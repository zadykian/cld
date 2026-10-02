package telemetry

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

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
	if !errors.As(err, new(*exec.ExitError)) {
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
		return state{}, d.failed(err, stderr.String(),
			"cannot inspect the collector "+container+"; "+unchanged)
	}
	fields := strings.Fields(stdout.String())
	s := state{exists: true}
	if len(fields) > 0 {
		s.status = fields[0]
	}
	if len(fields) > 1 {
		s.restarts, _ = strconv.Atoi(fields[1]) //nolint:errcheck // docker writes a number
	}
	if len(fields) > 2 {
		s.port, _ = ParsePort(fields[2])
	}
	return s, nil
}

// validate has the collector check the config that will run, writing what it says, and docker's
// own output - an image pull, say - to cld's stderr.
func (d docker) validate(passed, configs []string) error {
	cmd := d.command(slices.Concat([]string{"run", "--rm"}, passed, []string{image, "validate"},
		configs)...)
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
		return fail.Runtime("docker cannot run the collector to validate its config (see above); " +
			"nothing has changed")
	}
	return fail.Runtime("the collector refused its config (see above); nothing has changed")
}

// wait waits until the collector, running, takes connections on 127.0.0.1:port, where claude will
// send: the port tells what the log cannot (decision 18.5). Where the collector stops or
// restarts first, or readyWithin passes, wait fails as notReady does.
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
			return d.notReady(port, stopped, unchanged)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// notReady shows the end of the collector's log and fails, saying unchanged. It stops Docker
// restarting a collector that stopped, which Docker would do again and again, and at every boot
// (decision 18.5).
func (d docker) notReady(port int, stopped bool, unchanged string) error {
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
		return fail.Runtime(fmt.Sprintf(
			"the collector takes no connections on 127.0.0.1:%d after %d s (%s); %s",
			port, int(readyWithin/time.Second), log, unchanged))
	}
	if out, err := d.output("update", "--restart", "no", container); err != nil {
		return d.failed(err, out, "the collector stopped before it was ready ("+log+
			"), and cld cannot stop Docker restarting it: docker rm -f "+container+
			" removes it; "+unchanged)
	}
	return fail.Runtime("the collector stopped before it was ready (" + log +
		"), and Docker no longer restarts it; " + unchanged)
}

// accepts is whether something takes connections on 127.0.0.1:port.
func accepts(port int) bool {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp4", address, time.Second)
	if err == nil {
		_ = conn.Close() //nolint:errcheck // a probe's connection, which sent nothing
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
