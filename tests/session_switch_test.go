package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// C-q ( and C-q ) move the terminal to the previous and next session that runs, by name, round.
// C-q L moves it back to the one it came from (decision 51.1). Where no session fits, the
// message line says why for three seconds, and the terminal stays. claude's pane is drawn
// meanwhile, but by tmux 3.5, which draws it once the message has gone.
func TestSwitchKeys(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "c")
	k := keyMover{s: s, term: startCld(t, s, "tmux", nil, "join", "-s", "b")}
	waitScreen(t, k.term, "probe --name cld-b")
	k.term.Keys("C-q", "L")
	k.told(t, "C-q L with no session to go back to", "cld: no session to go back to")
	claudeOf(t, s, "b").Send("link https://example.com/cld drawn meanwhile")
	waitScreen(t, k.term, "drawn meanwhile")
	k.on(t, ")", "c")
	k.on(t, ")", "a")
	k.on(t, "(", "c")
	k.on(t, "L", "a")
	if last := lastOf(s, "a"); last != "c" {
		t.Errorf("a records %q as the last session, want c", last)
	}
	k.on(t, "L", "c")
	k.on(t, "(", "b")
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want a, b and c, each running", sessions)
	}
	k.passOver(t)
	outside := "cld: list --switch moves a terminal on one of cld's sessions, and TMUX names none\n"
	result := s.RunCld(nil, "list", "--switch", "/dev/pts/0", "--to", "next")
	if result.Code != 1 || result.Stderr != outside {
		t.Errorf("list --switch outside cld's servers: exit %d, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stderr, outside)
	}
}

// keyMover is TestSwitchKeys' sandbox, and the terminal whose keys move it.
type keyMover struct {
	s    *sandbox.Sandbox
	term terminal.Terminal
}

// told waits, for what, until the message line on b shows want, and checks that the terminal
// stayed on b.
func (k keyMover) told(t *testing.T, what, want string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, what, func() bool {
		return slices.Contains(shownMessages(k.s, "cld-b"), want)
	})
	if clients := k.s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("%s: clients attached to %q, want [cld-b]", what, clients)
	}
}

// on types C-q and key, and waits for the terminal to reach session name, alone there.
func (k keyMover) on(t *testing.T, key, name string) {
	t.Helper()
	k.term.Keys("C-q", key)
	waitScreen(t, k.term, "probe --name cld-"+name)
	sandbox.WaitFor(t, 10*time.Second, "the terminal on cld-"+name+" alone", func() bool {
		return slices.Equal(k.s.Clients(), []string{"cld-" + name})
	})
}

// passOver checks that C-q L refuses c once it has ended, and once its entry is forgotten, and
// that C-q ) refuses with no other session running.
func (k keyMover) passOver(t *testing.T) {
	t.Helper()
	k.s.RunCld(nil, "kill", "-s", "c")
	k.term.Keys("C-q", "L")
	k.told(t, "C-q L to c, which has ended", "cld: session 'c' has ended")
	forget(t, k.s, "c")
	k.term.Keys("C-q", "L")
	k.told(t, "C-q L to c, whose entry is forgotten", "cld: no session 'c'")
	k.on(t, ")", "a")
	k.on(t, ")", "b")
	k.s.RunCld(nil, "kill", "-s", "a")
	k.term.Keys("C-q", ")")
	k.told(t, "C-q ) with no other session", "cld: no other session runs")
}

// The keys and the move name cld by its file, quoted for run-shell's format, tmux's parser and the
// session's default-shell (decision 51.3). Here that is "cld;", in a directory ' #\;. The keys, and
// a join as claude runs it, move the terminal alike under sh, bash, zsh and fish, where installed.
// Words fish or tmux would read otherwise reach claude as given.
func TestSwitchQuoting(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sh", "bash", "zsh", "fish"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skipf("%s is not installed", name)
			}
			checkSwitchQuoting(t, shell)
		})
	}
}

// quotingRig is a sandbox of TestSwitchQuoting's, with cld copied to a path that needs quoting,
// in dir, and the shell each session's server runs the move with.
type quotingRig struct {
	s               *sandbox.Sandbox
	dir, cld, shell string
}

// checkSwitchQuoting moves a terminal with C-q s, C-q ) and C-q L, and with a join in a pane, by a
// cld whose path needs quoting, under shell.
func checkSwitchQuoting(t *testing.T, shell string) {
	t.Helper()
	s := sandbox.New(t)
	r := quotingRig{s: s, dir: filepath.Join(s.Root, `' #\;`), shell: shell}
	if err := os.Mkdir(r.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	r.cld = filepath.Join(r.dir, "cld;")
	if err := os.WriteFile(r.cld, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	detach := r.start(t, "b")
	detach.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detach.Running() })
	term := r.start(t, "a")
	term.Keys("C-q", "s")
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Enter")
	r.on(t, term, "b", "a")
	term.Keys("C-q", ")")
	r.on(t, term, "a", "b")
	term.Keys("C-q", "L")
	r.on(t, term, "b", "a")

	words := []string{`x\'`, `C:\`, `; echo moved #`, "fix:\n    indented\n# heading\nend"}
	join := exec.Command(r.cld, append([]string{"join", "-s", "c", "--"}, words...)...)
	join.Env, join.Dir = s.Environ(paneOf(t, s, "b")), r.dir
	if out, err := join.CombinedOutput(); err != nil || len(out) > 0 {
		t.Fatalf("join -s c in b's pane: %v, output %q, want exit 0 and no output", err, out)
	}
	r.on(t, term, "c", "b")
	c := claudeOf(t, s, "c")
	if c.Cwd != r.dir {
		t.Errorf("claude c runs in %s, want %s", c.Cwd, r.dir)
	}
	if len(c.Argv) < len(words) || !slices.Equal(c.Argv[len(c.Argv)-len(words):], words) {
		t.Errorf("claude c has arguments %q, want %q last", c.Argv, words)
	}
}

// start starts session suffix with the copy of cld, in a terminal of its own, and has its server
// run the move with the shell.
func (r quotingRig) start(t *testing.T, suffix string) terminal.Terminal {
	t.Helper()
	term := terminal.New(t, "tmux", r.s)
	term.Start([]string{r.cld, "join", "-s", suffix}, r.s.Env, r.s.Work)
	waitScreen(t, term, "probe --name cld-"+suffix)
	r.s.MustTmux("cld-"+suffix, "set", "-g", "default-shell", r.shell)
	return term
}

// on waits for term to reach session name, alone there, and checks that name records last as the
// session the terminal came from.
func (r quotingRig) on(t *testing.T, term terminal.Terminal, name, last string) {
	t.Helper()
	waitScreen(t, term, "probe --name cld-"+name)
	sandbox.WaitFor(t, 10*time.Second, "the terminal on cld-"+name+" alone", func() bool {
		return slices.Equal(r.s.Clients(), []string{"cld-" + name})
	})
	if got := lastOf(r.s, name); got != last {
		t.Errorf("%s records %q as the last session, want %s", name, got, last)
	}
}

// cld writes the move for sh, bash, zsh, fish, ksh and csh: under another default-shell, nu here,
// the terminal stays, and cld says why (decision 51.3). It says so on the message line for a key,
// and to claude for ! cld join. The message shows unchanged, although the shell's directory,
// "50%done #x", holds what tmux would expand.
func TestMoveNeedsAKnownShell(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	term := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitScreen(t, term, "probe --name cld-b")
	nu := filepath.Join(s.Root, "50%done #x", "nu")
	if err := os.Mkdir(filepath.Dir(nu), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", nu); err != nil {
		t.Fatal(err)
	}
	s.MustTmux("cld-b", "set", "-g", "default-shell", nu)
	refused := "cld: cannot move the terminal with tmux's default-shell '" + nu +
		"': cld writes the move for sh, bash, zsh, fish, ksh and csh alone"
	term.Keys("C-q", ")")
	sandbox.WaitFor(t, 10*time.Second, "C-q ) to be refused", func() bool {
		return slices.Contains(shownMessages(s, "cld-b"), refused)
	})
	want := refused + "; detach with C-q d and run cld join (see cld help join)\n"
	result := s.RunCld(paneOf(t, s, "b"), "join", "-s", "a")
	if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("join -s a in b's pane: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want [cld-b]", clients)
	}
}
