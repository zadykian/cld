package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// Tmux is the tmux cld runs. Find finds it on the PATH, which is all completion needs to read the
// sessions, and Check checks it, as every command needs before it runs tmux.
type Tmux struct {
	path string
	// version is the release tmux -V reported, as Check read it: nil for a tmux that Find found,
	// or that reported none ("master"), which older takes for the newest.
	version version
}

// Find finds tmux on the PATH (see tool.LookPath) and checks nothing else. Completion reads the
// sessions with it on every TAB, where Check would cost a tmux -V each time.
func Find() (*Tmux, error) {
	path, err := tool.LookPath("tmux")
	if err != nil {
		return nil, fail.Runtime("tmux is not installed")
	}
	return &Tmux{path: path}, nil
}

// Check makes the checks every command makes before it runs tmux, in this order: tmux, then
// each of tools, on the PATH (see tool.LookPath), and tmux's version. A tmux -V that fails ends
// cld with its status, after its own message (see exitStatus). Completion makes none of them.
func Check(tools ...string) (*Tmux, error) {
	t, err := Find()
	if err != nil {
		return nil, err
	}
	for _, name := range tools {
		if _, err := tool.LookPath(name); err != nil {
			return nil, fail.Runtime(name + " is not installed")
		}
	}
	// Development builds pass: "tmux next-3.9" reads as 3.9, "tmux 3.8-rc2" as 3.8, and
	// "tmux master" has no version to compare.
	out, err := t.command("-V").Output()
	if err != nil {
		return nil, t.exitStatus(err)
	}
	found := strings.TrimRight(string(out), "\n")
	reported := strings.TrimPrefix(found[strings.LastIndex(found, " ")+1:], "next-")
	if v, ok := parseVersion(tmuxVersion, reported); ok {
		if v.before(minTmux) {
			return nil, fail.Runtime(fmt.Sprintf("tmux %s or newer is required, found '%s'",
				tmuxRelease(minTmux), found))
		}
		t.version = v
	}
	return t, nil
}

// older reports whether t is a tmux older than release, as Check read its version; a tmux of no
// version known is not.
func (t *Tmux) older(release version) bool { return t.version != nil && t.version.before(release) }

// command is a tmux command run with cld's stdin and stderr, as tmux.
func (t *Tmux) command(args ...string) *exec.Cmd {
	return t.commandContext(context.Background(), args...)
}

// commandContext is command, killed once ctx is done: the interactive list abandons a lookup, a
// kill or a read of the sessions that way (see internal/picker). A program the killed tmux left
// behind with its output open then holds cld up for a second at most.
func (t *Tmux) commandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.path, args...)
	cmd.Args[0] = "tmux"
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if ctx.Done() != nil {
		cmd.WaitDelay = time.Second
	}
	return cmd
}

// server is a tmux command run against the server of session cld-SUFFIX, named like it.
func (t *Tmux) server(suffix string, args ...string) *exec.Cmd {
	return t.serverContext(context.Background(), suffix, args...)
}

// serverContext is server, killed once ctx is done (see commandContext).
func (t *Tmux) serverContext(ctx context.Context, suffix string, args ...string) *exec.Cmd {
	return t.commandContext(ctx, append([]string{"-L", "cld-" + suffix}, args...)...)
}

// combinedOutput runs cmd and returns its stdout and stderr together, without the newlines at
// the end; when cmd cannot run at all, tool.CannotRun's message instead. A tmux gets that far only
// where it stops being runnable after answering tmux -V.
func combinedOutput(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) && out.Len() == 0 {
		out.WriteString(tool.CannotRun(cmd.Path, err).Error())
	}
	return strings.TrimRight(out.String(), "\n"), err
}

// become replaces cld with tmux, run with argv and env: the terminal's process is tmux from
// then on, and tmux's exit status is cld's. It returns only when that fails.
func (t *Tmux) become(argv, env []string) error {
	return tool.CannotRun(t.path, syscall.Exec(t.path, argv, env))
}

// exitStatus is the exit status of a tmux command that failed, which has said why, or how cld
// ends when it could not run tmux at all.
func (t *Tmux) exitStatus(err error) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return tool.CannotRun(t.path, err)
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return fail.Status(128 + int(status.Signal()))
	}
	return fail.Status(exit.ExitCode())
}
