package tests

import (
	"bytes"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The tmux that cld runs in the tests: wrapped first on its PATH to log or hold commands, a
// server's stale socket, and the title's busy marker.

// wrapTmux puts a tmux first on the PATH of the environment it returns. That tmux is an sh script
// that runs script with tmux's arguments as "$@", and then the tmux the tests run.
func wrapTmux(t *testing.T, s *sandbox.Sandbox, script string) map[string]string {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.MkdirTemp(s.Root, "bin.") //nolint:usetesting // cld's servers outlive a t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	s.WriteProgram(filepath.Join(bin, "tmux"),
		"#!/bin/sh\n"+script+"exec '"+tmux+"' \"$@\"\n", 0o755)
	return map[string]string{"PATH": bin + string(os.PathListSeparator) + s.Env["PATH"]}
}

// loggedTmux puts a tmux that writes down what it runs first on the PATH of the environment it
// returns (see wrapTmux). The asked it returns gives the servers that tmux ran list-sessions on
// since asked was last called, in the order it ran them.
func loggedTmux(
	t *testing.T, s *sandbox.Sandbox,
) (env map[string]string, asked func() []string) {
	t.Helper()
	ran := filepath.Join(s.Root, "tmux ran")
	env = wrapTmux(t, s, "echo \"$*\" >>'"+ran+"'\n")
	return env, func() []string {
		t.Helper()
		out, err := os.ReadFile(ran)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if err := os.Remove(ran); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		var servers []string
		for line := range strings.SplitSeq(string(out), "\n") {
			if words := strings.Fields(line); slices.Contains(words, "list-sessions") {
				servers = append(servers, words[slices.Index(words, "-L")+1])
			}
		}
		return servers
	}
}

// heldTmux holds the tmux commands of a kind that cld run with env runs, from when the test holds
// them until it releases them or ends. A tmux first on the PATH (see wrapTmux) waits as long as
// the file hold is there. It also counts the commands that begin, held or not, a line each in
// hold.begun.
type heldTmux struct {
	hold, what string
	env        map[string]string
}

// holdTmux is ready to hold the tmux commands, described by what, whose arguments match pattern,
// a pattern of sh's case, once the test calls start.
func holdTmux(t *testing.T, s *sandbox.Sandbox, what, pattern string) heldTmux {
	t.Helper()
	return holding(t, s, what, pattern, "", "")
}

// holdTmuxOutput is holdTmux, but for commands that run first. What they write, on stdout and
// without its last newlines, and their exit status are held back until the test releases them, as
// if tmux took that long to answer.
func holdTmuxOutput(t *testing.T, s *sandbox.Sandbox, what, pattern string) heldTmux {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	return holding(t, s, what, pattern, "\tout=$('"+tmux+"' \"$@\" 2>&1); status=$?\n",
		"\t[ -z \"$out\" ] || printf '%s\\n' \"$out\"\n\texit $status\n")
}

// holding is holdTmux, and holdTmuxOutput, with the lines of sh's run before the commands are held
// and then after.
func holding(t *testing.T, s *sandbox.Sandbox, what, pattern, run, then string) heldTmux {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "hold.") //nolint:usetesting // the wrapper outlives t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(dir, "hold")
	t.Cleanup(func() { _ = os.Remove(hold) }) //nolint:errcheck // the test may have released it
	script := "case \"$*\" in " + pattern + ")\n" + run +
		"\techo $$ >>'" + hold + ".begun'\n" +
		"\tif [ -e '" + hold + "' ]; then echo $$ >'" + hold + ".held'; fi\n" +
		"\twhile [ -e '" + hold + "' ]; do sleep 0.05; done\n" + then +
		"\t;;\n" +
		"esac\n"
	return heldTmux{hold: hold, what: what, env: wrapTmux(t, s, script)}
}

// start holds the commands from now on.
func (h heldTmux) start(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(h.hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// held waits for a command to be held, and returns the pid of the tmux holding it.
func (h heldTmux) held(t *testing.T) int {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, h.what, func() bool {
		var err error
		data, err = os.ReadFile(h.hold + ".held")
		return err == nil && len(data) > 0 && data[len(data)-1] == '\n'
	})
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// release lets the commands go on.
func (h heldTmux) release(t *testing.T) {
	t.Helper()
	if err := os.Remove(h.hold); err != nil {
		t.Fatal(err)
	}
}

// begun counts the commands that have begun, held or not.
func (h heldTmux) begun(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(h.hold + ".begun")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return bytes.Count(data, []byte("\n"))
}

// holdLookup holds the lookup of session cld-NAME that Enter, the kill and join make, from the
// start. It tells the lookup from the read of the sessions, which asks server cld-NAME with the
// same filter, by the one format that follows.
func holdLookup(t *testing.T, s *sandbox.Sandbox, name string) heldTmux {
	t.Helper()
	lookup := holdTmux(t, s, "the lookup of cld-"+name, lookupPattern(name))
	lookup.start(t)
	return lookup
}

// lookupPattern is the pattern of sh's case that the arguments of the lookup of session cld-NAME
// match (see holdLookup).
func lookupPattern(name string) string {
	return "*'#{==:#{session_name},cld-" + name + "},'*" +
		"' -F #{session_name} #{W:#{P:#{pane_pid} }}\t#{@cld-home}'"
}

// staleSocket makes the sandbox's socket of server as a server that has died leaves it. That is a
// socket nothing listens on, where the real tmux says that no server is running. A plain file will
// not do on macOS (see docs/design/testing.md).
func staleSocket(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	address := &net.UnixAddr{Name: filepath.Join(s.SocketDir(), server), Net: "unix"}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

// busyMarker is the marker join has tmux put before the session's name in the tab's title while
// claude is busy: ◐ in even seconds and ◑ in odd ones. A job in it has tmux set the title again a
// second later (see TestContractTitle).
const busyMarker = "#{?#{m:*[02468],%S},◐,◑}" +
	"#((sleep 1; #{q:@cld-tmux} -S #{q:socket_path} refresh-client -S -t #{q:client_name})" +
	" >/dev/null 2>&1 &)"
