package tests

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join -C makes its session in DIR, where claude starts, the home and NAME's default coming from
// DIR (decision 61.1). In a pane, join refuses a DIR it cannot enter, or an ended session's that is
// gone, and the terminal stays. The terminal's join runs the words where join ran before -C: sub,
// never sub/sub (decision 61.3).
func TestJoinDirectoryMoves(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	orig := filepath.Join(s.Root, "orig")
	sub := filepath.Join(orig, "sub")
	makeDirs(t, orig, sub, filepath.Join(sub, "sub"))
	term := startCld(t, s, "tmux", nil, "join", "-C", "../orig")
	waitScreen(t, term, "probe --name cld-orig-0")
	checkMadeIn(t, s, "orig-0", orig)
	inOrig := paneOf(t, s, "orig-0")
	checkJoinRefused(t, "join -C missing in orig-0's pane",
		s.RunCldIn(orig, inOrig, "join", "-C", "missing", "-s", "x"),
		"cld: cannot enter missing: no such file or directory\n")
	// An ended sub-g whose directory is gone: the advice names sub by -C where -C was given.
	gone := filepath.Join(s.Root, "gone")
	writeEntry(t, s, "sub-g", gone, firstID)
	refused := "cld: session 'sub-g' ran in " + gone + ", which no longer exists; resume it "
	checkJoinRefused(t, "join -C sub -s g in orig-0's pane",
		s.RunCldIn(orig, inOrig, "join", "-C", "sub", "-s", "g"),
		refused+"with cld join -C "+sub+" -n sub -s g --resume "+firstID+"\n")
	checkJoinRefused(t, "join -s g in orig-0's pane, in sub",
		s.RunCldIn(sub, inOrig, "join", "-s", "g"),
		refused+"from here with cld join -n sub -s g --resume "+firstID+"\n")
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-orig-0"}) || !term.Running() {
		t.Errorf("clients attached to %q, want the terminal on cld-orig-0 still", clients)
	}

	checkMoved(t, "join -C sub -s b in orig-0's pane",
		s.RunCldIn(orig, inOrig, "join", "-C", "sub", "-s", "b"))
	waitScreen(t, term, "probe --name cld-sub-b")
	checkMadeIn(t, s, "sub-b", sub)
	if last := lastOf(s, "sub-b"); last != "orig-0" {
		t.Errorf("sub-b records %q as the last session, want orig-0", last)
	}
	// Without -s, the terminal's join takes the index, NAME coming from sub.
	checkMoved(t, "join -C sub in sub-b's pane",
		s.RunCldIn(orig, paneOf(t, s, "sub-b"), "join", "-C", "sub"))
	waitScreen(t, term, "probe --name cld-sub-0")
	checkMadeIn(t, s, "sub-0", sub)
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-sub-0"}) {
		t.Errorf("clients attached to %q, want the terminal on cld-sub-0", clients)
	}
	want := []string{"cld-orig-0", "cld-sub-0", "cld-sub-b"}
	if sessions := s.Sessions(); !slices.Equal(sessions, want) {
		t.Errorf("sessions %q, want %q", sessions, want)
	}
}

// checkMadeIn reports a session name whose claude does not run in dir, or whose home is not dir.
func checkMadeIn(t *testing.T, s *sandbox.Sandbox, name, dir string) {
	t.Helper()
	claude := claudeOf(t, s, name)
	home := s.MustTmux("cld-"+name, "show", "-v", "-t", "=cld-"+name+":", "@cld-home")
	if claude.Cwd != dir || home != dir {
		t.Errorf("claude %s runs in %s, @cld-home %s, want both %s", name, claude.Cwd, home, dir)
	}
}

// -C's DIR gives NAME's default and the home, as the current directory would, to -s's completion
// and to join --existing (decision 61.1). From web, -C ../api names api-a, not web-a, and not api-o
// of another directory named api. -C ../api/sub takes its repository's.
func TestDirectoryNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	api, web, other := filepath.Join(s.Root, "api"), filepath.Join(s.Root, "web"),
		filepath.Join(s.Root, "other", "api")
	runGit(t, s, s.Root, "init", "-q", api)
	makeDirs(t, filepath.Join(api, "sub"), web, filepath.Dir(other), other)
	for i, run := range []struct{ dir, suffix string }{{api, "a"}, {web, "w"}, {other, "o"}} {
		startCldIn(t, s, "tmux", run.dir, nil, "join", "-s", run.suffix)
		waitClients(t, s, i+1)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"-s", ""}, "w"},
		{[]string{"-C", "../api", "-s", ""}, "a"},
		{[]string{"--directory=../api/sub", "-s", ""}, "a"},
		{[]string{"-C", other, "-s", ""}, "o"},
	} {
		args := append([]string{"__complete", "join"}, test.args...)
		want := test.want + "\tattached\n:4\n"
		if result := s.RunCldIn(web, nil, args...); result.Code != 0 || result.Stdout != want {
			t.Errorf("cld %q in web: exit %d, stdout %q, want %q", args, result.Code, result.Stdout,
				want)
		}
	}
	checkJoinRefused(t, "join --existing -s a in web",
		s.RunCldOnTerminalIn(web, unknownTerminal, "join", "--existing", "-s", "a"),
		"cld: no session 'web-a' (see cld list)\n")
	term := startCldIn(t, s, "tmux", web, nil, "join", "--existing", "-C", "../api", "-s", "a")
	waitScreen(t, term, "probe --name cld-api-a")
	waitClients(t, s, 4)
	want := []string{"cld-api-a", "cld-api-a", "cld-api-o", "cld-web-w"}
	if clients := s.Clients(); !slices.Equal(clients, want) || len(s.Probes()) != 3 {
		t.Errorf("clients attached to %q, %d claudes, want %q and 3", clients, len(s.Probes()), want)
	}
}
