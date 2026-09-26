// Package telemetry is cld setup telemetry: it runs a local OpenTelemetry Collector in Docker and
// points Claude Code's user settings at it.
//
// Claude Code exports its telemetry to one OTLP endpoint per signal, so it cannot send the same
// signal to two places; the collector can. It receives everything claude sends and forwards
// traces, metrics and logs to a local endpoint (--local), such as the JetBrains OpenTelemetry
// plugin in the IDE, and metrics only to a remote one (--remote), such as a team's collector.
// Setup runs these steps in this order, so that a mistake is caught before anything that runs is
// replaced:
//
//   - checks: docker on the PATH (see tool.LookPath), then $CLAUDE_CONFIG_DIR/settings.json (by
//     default ~/.claude/settings.json) read and parsed - invalid JSON stops cld here - and the
//     --collector-config file read, and refused if no environment variable can hold it (see
//     extraConfig)
//   - the port the collector listens on, on 127.0.0.1 (see choosePort): --port, else the port of
//     the collector that runs, or is paused, kept in its label cld.port, so that claude sessions
//     already running keep sending to it, else one the kernel picks. Not 4317: another collector
//     or Jaeger is likely to have it. Nor the port of a --local or --remote URL that leads to
//     127.0.0.1: the collector would send what it receives to itself, again and again
//   - validate: docker run --rm IMAGE validate, on exactly the config that will run; if the
//     collector refuses it, nothing changes
//   - replace: docker rm -f cld-telemetry, then docker run -d, with --restart unless-stopped, so
//     that the collector is back after a reboot or a restart of Docker, where claude's settings
//     still send to it, and a docker stop stays stopped; and with --network host, so that
//     127.0.0.1 in a URL is the host itself and reaches a receiver that listens on loopback only,
//     such as the plugin's. Hence Linux only: Docker Desktop on macOS offers host networking only
//     as an opt-in, not probed
//   - wait up to 10 s for the collector to be ready (see docker.wait): for its receiver to take
//     connections on 127.0.0.1 and the port, where claude will send, whatever its log says. If it
//     stops or restarts first, or the time runs out, cld shows the end of the log and leaves the
//     settings alone; Docker is to stop restarting one that stopped
//   - settings: the env keys cld manages (see Setup) are rewritten in place, the others and
//     everything around them kept, the file replaced atomically (see settings.go)
//   - report: the port, where each signal goes, and the keys changed; claude reads its settings as
//     a session starts, so sessions running now keep what they started with
//
// The collector config (see collectorConfig) goes to the container as a copy, in an environment
// variable that --config=env:NAME reads, and so does the --collector-config file, as a second
// config that the collector merges over cld's: maps merge, lists are replaced. The container then
// needs no file of the host, validate checks what runs, and edits to the file apply when setup
// runs again; docker inspect shows both, secrets in the file included. cld's docker gets them
// through its environment (docker run -e NAME), not its command line, which any user can list.
// The image is pinned and bumped deliberately.
package telemetry

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

const (
	// image is the collector cld runs: the core distribution, pinned.
	image = "otel/opentelemetry-collector:0.161.0"
	// container is the name of the collector's container, one per Docker daemon.
	container = "cld-telemetry"
	// portLabel is the container's label that holds the port the collector listens on.
	portLabel = "cld.port"
	// configVariable and extraVariable are the environment variables that take cld's collector
	// config and the --collector-config file to the container.
	configVariable = "CLD_TELEMETRY_CONFIG"
	extraVariable  = "CLD_TELEMETRY_EXTRA"
	// readyWithin is how long cld waits for the collector to take connections (the collector
	// 0.161.0 does within 3 s).
	readyWithin = 10 * time.Second
	// logTail is how many lines of the collector's log cld shows when it does not get ready.
	logTail = 20
)

// Supported refuses setup telemetry on any system but Linux (see the package comment).
func Supported() error {
	if runtime.GOOS != "linux" {
		return fail.Runtime("setup telemetry works on Linux only")
	}
	return nil
}

// Endpoint is where the collector sends signals: an OTLP/gRPC receiver.
type Endpoint struct {
	// URL is the endpoint as given, http://HOST:PORT or https://HOST:PORT.
	URL string
	// Address is HOST:PORT, the collector's endpoint for it.
	Address string
	// Plaintext is set for http://: gRPC without TLS.
	Plaintext bool
	// port is PORT; local is whether HOST leads to 127.0.0.1, where the collector listens (see
	// Collector).
	port  int
	local bool
}

// Collector is whether e is where the collector listens, when it listens on port: it would send
// what it receives to itself, again and again. So it is when e has that port and a HOST that
// leads to 127.0.0.1: that address; 0.0.0.0 or ::, which are dialled as the host itself; or
// localhost or a name under it (RFC 6761). The collector's receiver listens on 127.0.0.1 alone:
// ::1 and 127.0.0.2 do not reach it.
func (e Endpoint) Collector(port int) bool {
	return e.local && e.port == port
}

// hostLabel matches a label of a host name: letters, digits, "-" and "_", starting and ending
// with a letter or digit. A name is labels separated by dots, with a dot at the end at most.
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`)

// validHost is whether host is a host name whose last label is not all digits: no top-level
// domain is, and such a name would be an IPv4 address written some other way, such as 127.1,
// which the collector would look up as a name.
func validHost(host string) bool {
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	for _, label := range labels {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

// ParseEndpoint reads a URL given with --local or --remote as OTEL_EXPORTER_OTLP_ENDPOINT takes
// it: http://HOST:PORT (plaintext gRPC) or https://HOST:PORT (TLS), and nothing more - no path,
// user, query or fragment. HOST is a name, an IPv4 address or an IPv6 one in brackets; PORT is
// read as ParsePort reads it.
func ParseEndpoint(raw string) (Endpoint, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Scheme+"://"+u.Host != raw {
		return Endpoint{}, false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil && (strings.HasPrefix(u.Host, "[") || !validHost(host)) {
		return Endpoint{}, false
	}
	port, ok := ParsePort(u.Port())
	if !ok {
		return Endpoint{}, false
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	local := name == "localhost" || strings.HasSuffix(name, ".localhost")
	if ip != nil {
		local = ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.IsUnspecified()
	}
	return Endpoint{URL: raw, Address: net.JoinHostPort(host, strconv.Itoa(port)), Plaintext: u.Scheme == "http",
		port: port, local: local}, true
}

// ParsePort reads a TCP port, from 1 to 65535, in decimal digits without a leading zero: cld would
// otherwise repeat a URL's 04317 where the collector has 4317.
func ParsePort(raw string) (int, bool) {
	if raw == "" || raw[0] == '0' || strings.Trim(raw, "0123456789") != "" {
		return 0, false
	}
	port, err := strconv.Atoi(raw)
	return port, err == nil && port >= 1 && port <= 65535
}

// Options are what cld setup telemetry is given.
type Options struct {
	// Local gets traces, metrics and logs; Remote metrics only. One at least is set.
	Local, Remote *Endpoint
	// Port is the port the collector listens on, 0 to have cld choose one.
	Port int
	// CollectorConfig is the file merged over cld's collector config, "" for none.
	CollectorConfig string
}

// Collector is the first of Local and Remote whose endpoint is where the collector listens, when
// it listens on port (see Endpoint.Collector), as its option and URL, "--local URL"; "" for
// neither. The collector must not have such a port: the command line refuses it as --port, and
// choosePort refuses it as the running collector's and passes over any other.
func (o Options) Collector(port int) string {
	for _, option := range []struct {
		name     string
		endpoint *Endpoint
	}{{"--local", o.Local}, {"--remote", o.Remote}} {
		if option.endpoint != nil && option.endpoint.Collector(port) {
			return option.name + " " + option.endpoint.URL
		}
	}
	return ""
}

// Setup runs the collector for o and points claude's settings at it, in the order the package
// comment gives. It sets these keys in the settings' env and removes the others it manages:
//
//   - CLAUDE_CODE_ENABLE_TELEMETRY=1, OTEL_METRICS_EXPORTER=otlp, OTEL_EXPORTER_OTLP_PROTOCOL=grpc
//     and OTEL_EXPORTER_OTLP_ENDPOINT, the collector
//   - with Local: OTEL_TRACES_EXPORTER=otlp and OTEL_LOGS_EXPORTER=otlp;
//     CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1, which turns claude's traces on, and
//     OTEL_LOG_TOOL_DETAILS=1, which puts Bash commands and MCP server and tool names on events and
//     spans - only the local endpoint gets those. Without Local these four are removed
//   - OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_{ENDPOINT,PROTOCOL} are removed: they would send a
//     signal past the collector
func Setup(o Options) error {
	if err := Supported(); err != nil {
		return err
	}
	// Checks.
	path, err := tool.LookPath("docker")
	if err != nil {
		return fail.Runtime("docker is not installed")
	}
	settingsPath, err := settingsFile()
	if err != nil {
		return err
	}
	if _, err := readSettings(settingsPath); err != nil {
		return err
	}
	configs := []string{"--config=env:" + configVariable}
	env := slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, configVariable+"=") || strings.HasPrefix(variable, extraVariable+"=")
	})
	passed := []string{"-e", configVariable}
	if o.CollectorConfig != "" {
		extra, err := extraConfig(o.CollectorConfig)
		if err != nil {
			return err
		}
		configs = append(configs, "--config=env:"+extraVariable)
		passed = append(passed, "-e", extraVariable)
		env = append(env, extra)
	}
	d := docker{path: path}

	// Port.
	running, err := d.inspect("nothing has changed")
	if err != nil {
		return err
	}
	port, held, err := choosePort(o, running)
	if err != nil {
		return err
	}
	defer release(held)
	d.env = append(env, configVariable+"="+collectorConfig(port, o.Local, o.Remote))

	// Validate.
	if err := d.validate(passed, configs); err != nil {
		return err
	}

	// Replace the collector.
	unchanged := "the settings are unchanged"
	if out, err := d.output("rm", "-f", container); err != nil && !strings.Contains(out, "No such container") {
		return d.failed(err, out, "cannot remove the collector "+container+"; "+unchanged)
	}
	if running.exists {
		unchanged = "the collector that ran before is gone, and " + unchanged
	}
	release(held)
	run := []string{"run", "-d", "--name", container, "--restart", "unless-stopped", "--network", "host",
		"--label", portLabel + "=" + strconv.Itoa(port)}
	run = append(append(append(run, passed...), image), configs...)
	if out, err := d.output(run...); err != nil {
		return d.failed(err, out, "cannot start the collector; "+unchanged)
	}

	// Wait.
	if err := d.wait(port, unchanged); err != nil {
		return err
	}

	// Settings.
	s, err := readSettings(settingsPath)
	if err != nil {
		return fail.Runtime(err.Error() + " (the collector " + container + " runs, but the settings are unchanged)")
	}
	changes := s.apply(envSettings(port, o.Local != nil))
	if len(changes) > 0 {
		if err := s.write(); err != nil {
			return fail.Runtime(err.Error() + " (the collector " + container + " runs, but the settings are unchanged)")
		}
	}

	// Report.
	return fail.Print(report(port, o, s.Path, changes))
}

// extraConfig reads the --collector-config file at path into the variable that takes it to the
// container, CLD_TELEMETRY_EXTRA=CONTENTS. It refuses a file that no environment variable can
// hold, where docker would fail to run: one with a NUL byte, which would end the variable, and
// one whose variable is longer than Linux takes a string of a program's environment,
// MAX_ARG_STRLEN - 32 pages, the NUL that ends it included - for docker as for the collector in
// its container.
func extraConfig(path string) (string, error) {
	extra, err := os.ReadFile(path)
	if err != nil {
		return "", fail.Runtime(fmt.Sprintf("cannot read the collector config %s: %v", path, reason(err)))
	}
	if bytes.IndexByte(extra, 0) >= 0 {
		return "", fail.Runtime("the collector config " + path + " is not text: it holds a NUL byte")
	}
	variable := extraVariable + "=" + string(extra)
	if most := 32*os.Getpagesize() - 1 - len(extraVariable+"="); len(extra) > most {
		return "", fail.Runtime(fmt.Sprintf("the collector config %s is too large for an environment variable: %d bytes, at most %d",
			path, len(extra), most))
	}
	return variable, nil
}

// docker runs docker at path, with env as its environment.
type docker struct {
	path string
	env  []string
}

// command is docker with args, stdin empty.
func (d docker) command(args ...string) *exec.Cmd {
	cmd := exec.Command(d.path, args...)
	cmd.Args[0] = "docker"
	cmd.Env = d.env
	return cmd
}

// output runs docker with args and returns what it wrote, stdout and stderr together. A docker
// that cannot run at all ends cld as a tmux that cannot run does (see tool.CannotRun).
func (d docker) output(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := d.command(args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := d.ran(cmd.Run())
	return out.String(), err
}

// ran is err from running docker as cld reports it: an *exec.ExitError when docker ran and
// failed, the end cld comes to when it could not run docker at all.
func (d docker) ran(err error) error {
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return tool.CannotRun(d.path, err)
	}
	return err
}

// failed is how cld ends when docker failed, after err, having written out: out, then cld's
// message; and when docker could not run at all, as that ends it.
func (d docker) failed(err error, out, message string) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return err
	}
	fmt.Fprint(os.Stderr, out)
	return fail.Runtime(message)
}

// state is the collector's container as docker inspect reports it.
type state struct {
	exists bool
	// status is docker's: created, running, paused, restarting, removing, exited or dead.
	status   string
	restarts int
	// port is the port in the label cld.port, 0 without one.
	port int
}

// inspect reads the state of the collector's container; none is no error. When docker fails,
// cld ends saying unchanged.
func (d docker) inspect(unchanged string) (state, error) {
	var stdout, stderr bytes.Buffer
	cmd := d.command("container", "inspect", "--format",
		`{{.State.Status}} {{.RestartCount}} {{index .Config.Labels "`+portLabel+`"}}`, container)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := d.ran(cmd.Run()); err != nil {
		if strings.Contains(stderr.String(), "No such container") {
			return state{}, nil
		}
		return state{}, d.failed(err, stderr.String(), "cannot inspect the collector "+container+"; "+unchanged)
	}
	fields := strings.Fields(stdout.String())
	s := state{exists: true}
	if len(fields) > 0 {
		s.status = fields[0]
	}
	if len(fields) > 1 {
		s.restarts, _ = strconv.Atoi(fields[1])
	}
	if len(fields) > 2 {
		s.port, _ = ParsePort(fields[2])
	}
	return s, nil
}

// validate has the collector check the config that will run, writing what it says, and docker's
// own output - an image pull, say - to cld's stderr.
func (d docker) validate(passed, configs []string) error {
	cmd := d.command(append(append(append([]string{"run", "--rm"}, passed...), image, "validate"), configs...)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	err := d.ran(cmd.Run())
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case !errors.As(err, &exit):
		return err
	// docker run exits with 125 when docker fails, and with the program's status otherwise.
	case exit.ExitCode() == 125:
		return fail.Runtime("docker cannot run the collector to validate its config (see above); nothing has changed")
	}
	return fail.Runtime("the collector refused its config (see above); nothing has changed")
}

// wait waits until the collector on port is ready: until, running, it takes connections on
// 127.0.0.1 and port, where claude will send. The collector starts its receivers after
// everything else, just before it logs "Everything is ready", so the port is as quick to tell,
// and it tells what the log cannot: a --collector-config may raise the level of the log, or send
// it elsewhere, and it may move the receiver to another port, where the collector is ready but
// out of claude's reach. When the container stops or restarts first, or readyWithin passes,
// wait shows the end of the log and fails, saying unchanged. One that stopped is left where
// docker logs finds it, but not restarting: under --restart unless-stopped Docker would restart
// it again and again, and at every boot.
func (d docker) wait(port int, unchanged string) error {
	deadline := time.Now().Add(readyWithin)
	for {
		s, err := d.inspect(unchanged)
		if err != nil {
			return err
		}
		stopped := !s.exists || s.status != "running" || s.restarts > 0
		if !stopped && accepts(port) {
			return nil
		}
		if stopped || time.Now().After(deadline) {
			// A collector that stopped may have written its reason last.
			out, err := d.output("logs", container)
			if err != nil {
				return d.failed(err, out, "cannot read the log of the collector; "+unchanged)
			}
			end := tail(out, logTail)
			fmt.Fprint(os.Stderr, end)
			log := "its log is empty"
			if end != "" {
				log = "the end of its log is above; docker logs " + container + " shows it all"
			}
			if !stopped {
				return fail.Runtime(fmt.Sprintf("the collector takes no connections on 127.0.0.1:%d after %d s (%s); %s",
					port, int(readyWithin/time.Second), log, unchanged))
			}
			if out, err := d.output("update", "--restart", "no", container); err != nil {
				return d.failed(err, out, "the collector stopped before it was ready ("+log+"), and cld cannot stop Docker restarting it: "+
					"docker rm -f "+container+" removes it; "+unchanged)
			}
			return fail.Runtime("the collector stopped before it was ready (" + log + "), and Docker no longer restarts it; " + unchanged)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// accepts is whether something takes connections on 127.0.0.1:port.
func accepts(port int) bool {
	conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err == nil {
		_ = conn.Close()
	}
	return err == nil
}

// tail is the last n lines of text, ending in a newline; nothing when text is empty.
func tail(text string, n int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}

// choosePort chooses the port the collector listens on, on 127.0.0.1: o.Port, if set; else the
// port of the running collector, from its label; else, if the collector is not running, the
// port in its label if nothing holds it - after a reboot another program may, the port being in
// the ephemeral range; else one the kernel picks. A paused collector counts as running: docker
// pause freezes it, but its receiver keeps the port, which claude sessions send to. Never a port
// at which an endpoint of o is the collector itself (see Options.Collector): the command line
// refuses such a --port, choosePort the running collector's, which claude sessions send to, and
// it passes over a stopped collector's and one the kernel picks - the kernel knows nothing of a
// receiver that is not running now. cld holds a port it checks, with a listener that release
// closes just before the collector starts, so that nothing takes it meanwhile; the port of the
// running collector is the collector's until docker rm -f.
func choosePort(o Options, running state) (int, net.Listener, error) {
	ours := (running.status == "running" || running.status == "paused") && running.port != 0
	if given := o.Port; given != 0 {
		if ours && running.port == given {
			return given, nil, nil
		}
		held, err := listen(given)
		if errors.Is(err, syscall.EADDRINUSE) {
			return 0, nil, fail.Runtime(fmt.Sprintf("port %d on 127.0.0.1 is in use: choose another --port, or leave it out", given))
		}
		if err != nil {
			return 0, nil, fail.Runtime(fmt.Sprintf("cannot listen on 127.0.0.1:%d: %v", given, reason(err)))
		}
		return given, held, nil
	}
	if ours {
		if option := o.Collector(running.port); option != "" {
			return 0, nil, fail.Runtime(fmt.Sprintf("%s is where the collector listens (%s has port %d): give the receiver's port, or another --port",
				option, container, running.port))
		}
		return running.port, nil, nil
	}
	if running.port != 0 && o.Collector(running.port) == "" {
		if held, err := listen(running.port); err == nil {
			return running.port, held, nil
		}
	}
	// A port passed over stays held until the kernel has picked another.
	var passed []net.Listener
	defer func() {
		for _, held := range passed {
			release(held)
		}
	}()
	for {
		held, err := listen(0)
		if err != nil {
			return 0, nil, fail.Runtime("cannot listen on 127.0.0.1: " + reason(err).Error())
		}
		port := held.Addr().(*net.TCPAddr).Port
		if o.Collector(port) == "" {
			return port, held, nil
		}
		passed = append(passed, held)
	}
}

// listen listens on 127.0.0.1:port, where the collector will: something listening on
// 0.0.0.0 or [::] holds the port too.
func listen(port int) (net.Listener, error) {
	return net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// release closes held, if cld holds a port.
func release(held net.Listener) {
	if held != nil {
		_ = held.Close()
	}
}

// reason is err without the operation and path that *os.PathError, *os.SyscallError and
// *net.OpError add to it.
func reason(err error) error {
	for {
		switch e := err.(type) {
		case *os.PathError:
			err = e.Err
		case *os.SyscallError:
			err = e.Err
		case *net.OpError:
			err = e.Err
		default:
			return err
		}
	}
}

// collectorConfig is cld's collector config: an OTLP/gRPC receiver on 127.0.0.1:port and an
// exporter per endpoint, otlp_grpc/local and otlp_grpc/remote, which --collector-config can
// refer to by these names; otlp, their type's old name, is deprecated in the collector 0.161.0.
// Each exporter queues and retries on its own, so the remote gets metrics while the IDE is
// closed; the local one gives up on data after 30 s rather than the default 5 min. The
// collector's own metrics are off: with --network host their server would take localhost:8888
// on the host, and the collector exits when another collector has it.
func collectorConfig(port int, local, remote *Endpoint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: %s\n",
		quote(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))))
	b.WriteString("exporters:\n")
	var metrics []string
	for _, exporter := range []struct {
		name     string
		endpoint *Endpoint
		retry    string
	}{
		{"otlp_grpc/local", local, "    retry_on_failure:\n      max_elapsed_time: 30s\n"},
		{"otlp_grpc/remote", remote, ""},
	} {
		if exporter.endpoint == nil {
			continue
		}
		metrics = append(metrics, exporter.name)
		fmt.Fprintf(&b, "  %s:\n    endpoint: %s\n", exporter.name, quote(exporter.endpoint.Address))
		if exporter.endpoint.Plaintext {
			b.WriteString("    tls:\n      insecure: true\n")
		}
		b.WriteString(exporter.retry)
	}
	b.WriteString("service:\n  pipelines:\n")
	fmt.Fprintf(&b, "    metrics:\n      receivers: [otlp]\n      exporters: [%s]\n", strings.Join(metrics, ", "))
	if local != nil {
		for _, signal := range []string{"traces", "logs"} {
			fmt.Fprintf(&b, "    %s:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n", signal)
		}
	}
	b.WriteString("  telemetry:\n    metrics:\n      level: none\n")
	return b.String()
}

// quote is s as a YAML scalar in double quotes, which a JSON string is: an IPv6 address in
// brackets would otherwise be a list.
func quote(s string) string {
	return strconv.Quote(s)
}

// envSettings are the env keys Setup manages, in the order it adds them, for a collector on port
// (see Setup).
func envSettings(port int, local bool) []setting {
	settings := []setting{
		{key: "CLAUDE_CODE_ENABLE_TELEMETRY", value: "1"},
		{key: "OTEL_METRICS_EXPORTER", value: "otlp"},
		{key: "OTEL_EXPORTER_OTLP_PROTOCOL", value: "grpc"},
		{key: "OTEL_EXPORTER_OTLP_ENDPOINT", value: "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))},
	}
	for _, s := range []setting{
		{key: "OTEL_TRACES_EXPORTER", value: "otlp"},
		{key: "OTEL_LOGS_EXPORTER", value: "otlp"},
		{key: "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA", value: "1"},
		{key: "OTEL_LOG_TOOL_DETAILS", value: "1"},
	} {
		s.remove = !local
		settings = append(settings, s)
	}
	for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
		for _, what := range []string{"ENDPOINT", "PROTOCOL"} {
			settings = append(settings, setting{key: "OTEL_EXPORTER_OTLP_" + signal + "_" + what, remove: true})
		}
	}
	return settings
}

// report is what Setup prints once it is done: where the collector listens and sends each
// signal, and the keys it changed in the settings at path.
func report(port int, o Options, path string, changes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The collector %s listens on 127.0.0.1:%d and sends\n", container, port)
	var metrics []string
	if o.Local != nil {
		fmt.Fprintf(&b, "  traces to   %s\n", o.Local.URL)
		metrics = append(metrics, o.Local.URL)
	}
	if o.Remote != nil {
		metrics = append(metrics, o.Remote.URL)
	}
	fmt.Fprintf(&b, "  metrics to  %s\n", strings.Join(metrics, ", "))
	if o.Local != nil {
		fmt.Fprintf(&b, "  logs to     %s\n", o.Local.URL)
	}
	if len(changes) == 0 {
		fmt.Fprintf(&b, "%s has these settings already.\n", path)
		return b.String()
	}
	fmt.Fprintf(&b, "Changed in %s, under env:\n", path)
	for _, change := range changes {
		fmt.Fprintf(&b, "  %s\n", change)
	}
	b.WriteString("claude reads these settings as a session starts: sessions running now keep theirs.\n")
	return b.String()
}
