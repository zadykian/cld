package remote

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/tool"
)

// dropped is ssh's status where the connection fails, as on a drop, or the login does.
const dropped = fail.Status(255)

// pause is the wait before a try after a drop.
const pause = 5 * time.Second

// refusal matches the last line ssh logs where the host turns the login away: USER@HOST:
// Permission denied (METHODS). or Host key verification failed. No try follows one, as each would
// count towards a fail2ban's ban. Lines of a login that went on, such as "Permission denied,
// please try again." after a mistyped password, do not match (decision 68.3).
var refusal = regexp.MustCompile(
	`(?m)^(?:\S+@\S+(?: port \d+)?: Permission denied \([^)]*\)\.|Host key verification failed\.)\s*$`)

// unreached matches what ssh logs where it never connected: no address, or no answer. The host's
// cld has then seen no join, so the next try runs the first words (decision 68.2).
var unreached = regexp.MustCompile(
	`(?m)^ssh: (?:connect to host |Could not resolve hostname )`)

// modes turns off what the session's programs left on. Those are the reports of the mouse, focus
// and the colour scheme, modifyOtherKeys and kitty's flags, the cursor and keypad keys, and the
// cursor's style and visibility. reset leaves the alternate screen between two of them, as kitty's
// flags are each screen's own. Its ESC 7 keeps the main screen's cursor in place, which leaving
// restores even where that screen is on (decision 68.4, after zadykian/ws#793).
const (
	modes = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1005l\x1b[?1006l\x1b[?1015l\x1b[?1004l" +
		"\x1b[?2031l\x1b[>4m\x1b[<99u\x1b[=0;1u\x1b[?1l\x1b>\x1b[0 q\x1b[?25h"
	reset = modes + "\x1b7\x1b[?1049l" + modes
)

// Join runs a join's words on host with a terminal, as Exec does, but with ssh as cld's child: it
// joins again after a drop (decision 68). Where ssh ends with 255, and its log shows no refusal,
// cld resets the terminal and runs again's words 5 s later, or words again where ssh never
// connected. With again nil, no try follows a connection that dropped. A signal to cld ends the
// tries: Ctrl-C in the wait, or the pane closing.
func Join(host string, words, again []string) error {
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
	for first := true; ; first = false {
		said, err := j.try(words, !first)
		reached := !unreached.MatchString(said)
		switch {
		case !errors.Is(err, dropped) || refused(said):
			return err
		case reached && again == nil:
			output.Note("ssh to " + host + " ended with status 255: join again with -s SUFFIX " +
				"(see cld list --host " + host + ")")
			return err
		}
		output.Note("ssh to " + host + " ended with status 255: trying again in 5 s " +
			"(Ctrl-C to stop)")
		if err := j.wait(); err != nil {
			return err
		}
		if reached {
			words = again
		}
	}
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
// stderr, and how cld ends where no try follows. After a drop, it resets the terminal, so that the
// log and any try after show on the main screen. Where again, it first flushes the terminal's
// input. ssh keeps what the user typed in the wait when it enters raw mode, and the host's claude
// would take it as a prompt.
func (j *joiner) try(words []string, again bool) (string, error) {
	if again {
		flushInput(os.Stdin)
	}
	if err := os.WriteFile(j.log, nil, 0o600); err != nil {
		return "", fail.Runtime("cannot empty ssh's log: " + err.Error())
	}
	argv := argv(j.host, words, true, "-o", "ConnectTimeout=10", "-E", j.log)
	cmd := exec.Command(j.path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	ended := j.run(cmd)
	if errors.Is(ended, dropped) {
		if err := output.Print(reset); err != nil {
			return "", err
		}
	}
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

// wait waits 5 s before a try, or ends cld with 128 plus the number of a signal that comes first.
func (j *joiner) wait() error {
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case caught := <-j.signals:
		return fail.Status(128 + int(caught.(syscall.Signal)))
	}
}

// refused reports whether ssh's log says the host turned the login away.
func refused(said string) bool {
	return refusal.MatchString(said)
}
