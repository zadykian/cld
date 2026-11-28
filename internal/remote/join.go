package remote

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// Join runs a join's words on host with a terminal, as Exec does, but with ssh as cld's child
// (decision 68). ssh gives up a connection after 10 s, and logs to a file, which cld prints once
// ssh ends.
func Join(host string, words []string) error {
	path, err := tool.LookPath("ssh")
	if err != nil {
		return fail.Runtime("ssh is not installed")
	}
	j, err := newJoiner(path, host)
	if err != nil {
		return err
	}
	defer os.Remove(j.log) //nolint:errcheck // a temporary file, left where it cannot go
	defer signal.Stop(j.signals)
	_, err = j.try(words)
	return err
}

// joiner runs a join's tries: ssh at path, to host, logging to the file log, and the signals
// that end it.
type joiner struct {
	path, host, log string
	signals         chan os.Signal
}

// newJoiner is a joiner whose log is a new temporary file, and which catches the signals that
// would end cld.
func newJoiner(path, host string) (*joiner, error) {
	file, err := os.CreateTemp("", "cld-ssh-*.log")
	if err != nil {
		return nil, fail.Runtime("cannot make ssh's log: " + err.Error())
	}
	if err := file.Close(); err != nil {
		return nil, fail.Runtime("cannot make ssh's log: " + err.Error())
	}
	j := &joiner{path: path, host: host, log: file.Name(), signals: make(chan os.Signal, 1)}
	signal.Notify(j.signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	return j, nil
}

// try runs ssh with words, with an empty log, and returns what the log then says, printed to
// stderr, and how cld ends where no try follows.
func (j *joiner) try(words []string) (string, error) {
	if err := os.WriteFile(j.log, nil, 0o600); err != nil {
		return "", fail.Runtime("cannot empty ssh's log: " + err.Error())
	}
	argv := argv(j.host, words, true, "-o", "ConnectTimeout=10", "-E", j.log)
	cmd := exec.Command(j.path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	ended := j.run(cmd)
	said, err := os.ReadFile(j.log)
	if err != nil {
		return "", fail.Runtime("cannot read ssh's log: " + err.Error())
	}
	_, _ = fmt.Fprint(os.Stderr, string(said))
	return string(said), ended
}

// run runs cmd, passing it the signals cld catches meanwhile, and returns how cld ends with its
// status: ssh's, or 128 plus the number of a signal cld caught.
func (j *joiner) run(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return tool.CannotRun(j.path, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var caught os.Signal
	for {
		select {
		case caught = <-j.signals:
			_ = cmd.Process.Signal(caught) //nolint:errcheck // ssh may have ended already
		case err := <-done:
			if caught != nil {
				return fail.Status(128 + int(caught.(syscall.Signal)))
			}
			return j.status(err)
		}
	}
}

// status is how cld ends where ssh's run ended with err: ssh's status, or 128 plus the number of
// the signal that ended it.
func (j *joiner) status(err error) error {
	exit, ok := errors.AsType[*exec.ExitError](err)
	switch {
	case err == nil:
		return nil
	case !ok:
		return tool.CannotRun(j.path, err)
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return fail.Status(128 + int(status.Signal()))
	}
	return fail.Status(exit.ExitCode())
}
