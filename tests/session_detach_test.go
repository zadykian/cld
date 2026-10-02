package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// detach detaches terminals without C-q d (decision 44). As claude runs it, it detaches the one a
// key was typed in last on claude's session; with -s, every one on the session named. With none
// there, either does nothing, and in a shell on the server it goes by that shell's terminal. Each
// cld detached returns 0, and claude runs on throughout.
func TestDetach(t *testing.T) {
	t.Parallel()
	r := &detachRig{s: sandbox.New(t)}
	first := r.attach(t, "join", "-s", "a")
	r.probe = r.s.WaitProbes(1)[0]
	waitClients(t, r.s, 1)
	second := r.attach(t, "join", "-s", "a")
	waitClients(t, r.s, 2)
	startCld(t, r.s, "tmux", nil, "join", "-s", "b")
	r.s.WaitProbes(2)
	waitClients(t, r.s, 3)
	r.pane = map[string]string{"TMUX": r.probe.Env["TMUX"], "TMUX_PANE": r.probe.Env["TMUX_PANE"]}
	if !strings.HasPrefix(r.pane["TMUX"], filepath.Join(r.s.SocketDir(), "cld-a")+",") ||
		r.pane["TMUX_PANE"] == "" {
		t.Fatalf("claude a has TMUX %q and TMUX_PANE %q", r.pane["TMUX"], r.pane["TMUX_PANE"])
	}

	third := r.byKeys(t, first, second)
	r.bySession(t, third)
	r.withoutTerminal(t)
	r.inShell(t)

	gone := filepath.Join(r.s.SocketDir(), "cld-gone")
	want := "error connecting to " + gone + " (No such file or directory)\n"
	result := r.s.RunCld(map[string]string{"TMUX": gone + ",1,0", "TMUX_PANE": "%0"}, "detach")
	if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("detach with no server: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if probes := r.s.Probes(); len(probes) != 2 || !r.probe.Alive() {
		t.Errorf("%d claude processes, claude a alive: %v; want the two, alive",
			len(probes), r.probe.Alive())
	}
}

// detachRig is TestDetach's sandbox, with claude a and the variables of its pane.
type detachRig struct {
	s     *sandbox.Sandbox
	probe *sandbox.Probe
	pane  map[string]string
	// statuses counts the terminals attach has started
	statuses int
}

// statusTerminal is a terminal running cld in a shell, which writes cld's exit status to status.
type statusTerminal struct {
	terminal.Terminal
	status string
}

// attach starts cld with args in a terminal of its own.
func (r *detachRig) attach(t *testing.T, args ...string) statusTerminal {
	t.Helper()
	r.statuses++
	status := filepath.Join(r.s.Root, strconv.Itoa(r.statuses)+".status")
	term := terminal.New(t, "tmux", r.s)
	term.Start(append([]string{"sh", "-c", `"$@"; echo $? >"$0"`, status},
		r.s.CldArgv(args...)...), r.s.Env, r.s.Work)
	return statusTerminal{term, status}
}

// detached waits for the cld in term, named what, to return, and checks that it exits 0.
func (r *detachRig) detached(t *testing.T, what string, term statusTerminal) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, what+" to return", func() bool { return !term.Running() })
	if code, err := os.ReadFile(term.status); err != nil || string(code) != "0\n" {
		t.Errorf("%s exited with status %q, want 0:\n%s",
			what, strings.TrimSpace(string(code)), strings.TrimSpace(term.Screen()))
	}
}

// quiet runs cld with args and the variables extra, and checks that it exits 0, printing nothing.
func (r *detachRig) quiet(t *testing.T, what string, extra map[string]string, args ...string) {
	t.Helper()
	result := r.s.RunCld(extra, args...)
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			what, result.Code, result.Stdout, result.Stderr)
	}
}

// clients checks the sessions the terminals are attached to.
func (r *detachRig) clients(t *testing.T, what string, want ...string) {
	t.Helper()
	if got := r.s.Clients(); !slices.Equal(got, want) {
		t.Errorf("%s: clients attached to %q, want %q", what, got, want)
	}
}

// onSide attaches a terminal to session side, which claude made on a's server.
func (r *detachRig) onSide(t *testing.T) {
	t.Helper()
	side := terminal.New(t, "tmux", r.s)
	side.Start([]string{sandbox.RealTmux, "-L", "cld-a", "attach-session", "-t", "=side"},
		r.s.Env, r.s.Work)
	sandbox.WaitFor(t, 10*time.Second, "a terminal on side", func() bool {
		return slices.Contains(r.s.Clients(), "side")
	})
}

// byKeys runs detach in claude's pane after a key in first, attached before second, and then
// after one in second, attached before a third. Each time it detaches the terminal of the key. It
// returns the third.
func (r *detachRig) byKeys(t *testing.T, first, second statusTerminal) statusTerminal {
	t.Helper()
	mark := r.probe.Mark()
	first.Keys("x")
	r.probe.WaitInput(mark, "x")
	r.quiet(t, "detach in claude's pane", r.pane, "detach")
	r.detached(t, "the first terminal", first)
	r.clients(t, "after a key in the first terminal", "cld-a", "cld-b")
	if !second.Running() {
		t.Error("the second terminal was detached")
	}
	third := r.attach(t, "join", "-s", "a")
	waitClients(t, r.s, 3)
	mark = r.probe.Mark()
	second.Keys("y")
	r.probe.WaitInput(mark, "y")
	r.quiet(t, "detach in claude's pane", r.pane, "detach")
	r.detached(t, "the second terminal", second)
	r.clients(t, "after a key in the second terminal", "cld-a", "cld-b")
	return third
}

// bySession runs detach -s, which detaches every terminal on the session: from claude's pane, the
// terminal on b, and then the third terminal on a and a fourth.
func (r *detachRig) bySession(t *testing.T, third statusTerminal) {
	t.Helper()
	r.quiet(t, "detach -s b in claude's pane", r.pane, "detach", "-s", "b")
	waitClients(t, r.s, 1)
	r.clients(t, "after detach -s b", "cld-a")
	fourth := r.attach(t, "join", "-s", "a")
	waitClients(t, r.s, 2)
	r.quiet(t, "detach -s a", nil, "detach", "-s", "a")
	r.detached(t, "the third terminal", third)
	r.detached(t, "the fourth terminal", fourth)
	r.clients(t, "after detach -s a")
}

// withoutTerminal runs detach with no terminal on the server, then with one only on a session
// claude made there. tmux's detach-client would fail on the first, and detach that terminal on the
// second (decision 44.2).
func (r *detachRig) withoutTerminal(t *testing.T) {
	t.Helper()
	r.quiet(t, "detach -s a with no terminal", nil, "detach", "-s", "a")
	r.quiet(t, "detach in claude's pane with no terminal", r.pane, "detach")
	r.probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(r.s.Sessions(), "cld-a/side")
	})
	r.onSide(t)
	r.quiet(t, "detach in claude's pane with a terminal on side", r.pane, "detach")
	r.quiet(t, "detach -s a with a terminal on side", nil, "detach", "-s", "a")
	r.clients(t, "with a terminal on side", "side")
}

// inShell runs detach in a shell on a's server, in a window of its own, without TMUX_PANE. tmux
// finds its pane by its terminal, and not the session used last, side, which a terminal attaches
// to after one attaches to a.
func (r *detachRig) inShell(t *testing.T) {
	t.Helper()
	s := r.s
	s.MustTmux("cld-a", "detach-client", "-s", "=side")
	waitClients(t, s, 0)
	fifth := r.attach(t, "join", "-s", "a")
	waitClients(t, s, 1)
	r.onSide(t)
	shell := filepath.Join(s.Root, "shell")
	s.MustTmux("cld-a", append([]string{"new-window", "-d", "-t", "=cld-a:", "sh", "-c",
		`"$@" 2>"$0.err"; echo $? >"$0.code"`, shell, "env", "-u", "TMUX_PANE"},
		s.CldArgv("detach")...)...)
	code := waitExitFile(t, "cld detach in a shell to return", shell)
	stderr, err := os.ReadFile(shell + ".err")
	if err != nil || code != "0\n" || len(stderr) != 0 {
		t.Errorf("cld detach in a shell: exit %s, stderr %q, want exit 0 and no output",
			strings.TrimSpace(code), stderr)
	}
	r.detached(t, "the fifth terminal", fifth)
	r.clients(t, "after detach in a shell", "side")
}

// waitExitFile waits, for what, until a shell run with out as $0 has written its exit status to
// out.code, and returns that line.
func waitExitFile(t *testing.T, what, out string) string {
	t.Helper()
	var code []byte
	sandbox.WaitFor(t, 10*time.Second, what, func() bool {
		var err error
		code, err = os.ReadFile(out + ".code")
		return err == nil && strings.HasSuffix(string(code), "\n")
	})
	return string(code)
}
