// Package telemetry is cld setup telemetry: it runs a local OpenTelemetry Collector in Docker and
// points Claude Code's user settings at it (decision 18).
//
// claude cannot send a signal to two OTLP endpoints; the collector can. It sends traces, metrics
// and logs to --local, such as the IDE, and metrics alone to --remote. Setup reads and checks
// everything before it replaces anything that runs (decision 18.5).
//
// Linux only, for --network host (decision 18.1). Both collector configs reach the container in
// environment variables, which docker gets in its environment rather than on its command line,
// where any user can list them (decision 18.4). The image is pinned and bumped deliberately.
package telemetry

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
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

// Supported refuses setup telemetry on any system but Linux (decision 18.1).
func Supported() error {
	if runtime.GOOS != "linux" {
		return fail.Runtime("setup telemetry works on Linux only")
	}
	return nil
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

// Setup runs the collector for o and points claude's settings at it, in the steps of decision
// 18.5. It sets the env keys that send claude's signals to the collector, and removes those that
// would bypass it (see envSettings).
func Setup(o Options) error {
	if err := Supported(); err != nil {
		return err
	}
	p, err := check(o)
	if err != nil {
		return err
	}
	running, err := p.docker.inspect("nothing has changed")
	if err != nil {
		return err
	}
	port, held, err := choosePort(o, running)
	if err != nil {
		return err
	}
	defer release(held)
	config := configVariable + "=" + collectorConfig(port, o.Local, o.Remote)
	p.docker.env = slices.Concat(p.environ, []string{config})
	if err := p.docker.validate(p.passed, p.configs); err != nil {
		return err
	}
	unchanged, err := p.replace(port, held, running.exists)
	if err != nil {
		return err
	}
	if err := p.docker.wait(port, unchanged); err != nil {
		return err
	}
	changes, err := pointSettings(p.settings, port, o.Local != nil)
	if err != nil {
		return err
	}
	return output.Print(report(port, o, p.settings, changes))
}

// plan is what Setup's checks find: docker, claude's settings file, and what takes the
// collector's configs to its container (decision 18.4).
type plan struct {
	docker   docker
	settings string
	// environ is docker's environment but for cld's config, which Setup adds once it has the port.
	// passed are docker run's -e options for the configs, and configs the collector's --config
	// options that read them.
	environ, passed, configs []string
}

// check makes Setup's checks, which change nothing: docker on the PATH, claude's settings read
// and parsed, and the --collector-config file read.
func check(o Options) (plan, error) {
	path, err := tool.LookPath("docker")
	if err != nil {
		return plan{}, fail.Runtime("docker is not installed")
	}
	settingsPath, err := settingsFile()
	if err != nil {
		return plan{}, err
	}
	if _, err := readSettings(settingsPath); err != nil {
		return plan{}, err
	}
	p := plan{
		docker:   docker{path: path},
		settings: settingsPath,
		environ: slices.DeleteFunc(os.Environ(), func(variable string) bool {
			return strings.HasPrefix(variable, configVariable+"=") ||
				strings.HasPrefix(variable, extraVariable+"=")
		}),
		passed:  []string{"-e", configVariable},
		configs: []string{"--config=env:" + configVariable},
	}
	if o.CollectorConfig == "" {
		return p, nil
	}
	extra, err := extraConfig(o.CollectorConfig)
	if err != nil {
		return plan{}, err
	}
	p.configs = append(p.configs, "--config=env:"+extraVariable)
	p.passed = append(p.passed, "-e", extraVariable)
	p.environ = append(p.environ, extra)
	return p, nil
}

// replace removes the collector and starts one on port, letting go of the port cld holds just
// before. It returns what a later failure says is unchanged.
func (p plan) replace(port int, held net.Listener, existed bool) (string, error) {
	unchanged := "the settings are unchanged"
	out, err := p.docker.output("rm", "-f", container)
	if err != nil && !strings.Contains(out, "No such container") {
		return "", p.docker.failed(err, out,
			"cannot remove the collector "+container+"; "+unchanged)
	}
	if existed {
		unchanged = "the collector that ran before is gone, and " + unchanged
	}
	release(held)
	run := slices.Concat([]string{"run", "-d", "--name", container,
		"--restart", "unless-stopped", "--network", "host",
		"--label", portLabel + "=" + strconv.Itoa(port)}, p.passed, []string{image}, p.configs)
	if out, err := p.docker.output(run...); err != nil {
		return "", p.docker.failed(err, out, "cannot start the collector; "+unchanged)
	}
	return unchanged, nil
}

// pointSettings points claude's settings at path to the collector on port, and returns the keys
// it changed (see settings.apply).
func pointSettings(path string, port int, local bool) ([]string, error) {
	const runs = " (the collector " + container + " runs, but the settings are unchanged)"
	s, err := readSettings(path)
	if err != nil {
		return nil, fail.Runtime(err.Error() + runs)
	}
	changes := s.apply(envSettings(port, local))
	if len(changes) > 0 {
		if err := s.write(); err != nil {
			return nil, fail.Runtime(err.Error() + runs)
		}
	}
	return changes, nil
}

// report is what Setup prints at the end: where the collector listens and sends each signal, and
// the keys it changed in the settings at path.
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
	b.WriteString("claude reads these settings as a session starts: " +
		"sessions running now keep theirs.\n")
	return b.String()
}

// reason is err without the operation and path that *os.PathError, *os.SyscallError and
// *net.OpError add to it.
func reason(err error) error {
	for {
		if e, ok := errors.AsType[*os.PathError](err); ok {
			err = e.Err
		} else if e, ok := errors.AsType[*os.SyscallError](err); ok {
			err = e.Err
		} else if e, ok := errors.AsType[*net.OpError](err); ok {
			err = e.Err
		} else {
			return err
		}
	}
}
