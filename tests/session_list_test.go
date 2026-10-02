package tests

import (
	"bytes"
	"fmt"
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

// list shows cld's sessions: the name, the state and claude's directory now, under a locale that
// is not UTF-8 too. It asks each server for its own session only (decision 13), so a session
// renamed by hand shows as ended. With none of cld's sessions, it prints nothing, not even the
// header.
func TestList(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	checkListed(t, s, "without a server", nil, "")
	// None of cld's: server cld-x holds session other, and cld-x.y is a name no session of cld's
	// can have. Server cld is the one cld 0.3.0 and earlier shared (see
	// TestLeavesTheSharedServerAlone).
	others := sandbox.New(t)
	others.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	others.MustTmux("cld-x.y", "-f", "/dev/null", "new-session", "-d", "-s", "cld-x.y",
		"sleep", "600")
	others.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	checkListed(t, others, "with none of cld's sessions", nil, "")

	elsewhere, moved := filepath.Join(s.Root, "elsewhere"), filepath.Join(s.Work, "café")
	for _, dir := range []string{elsewhere, moved} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	b := s.WaitProbes(1)[0]
	detached := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "long_name", "-s", "1")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	detached.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detached.Running() })
	// A session claude makes on its server is not cld's, even one listed first: tmux names it with
	// a number, as a bare tmux in claude's pane would.
	s.MustTmux("cld-b", "new-session", "-d", "sleep", "60")
	b.Send("cd " + moved)
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		return s.Format("cld-b", "#{pane_current_path}") == moved
	})

	want := "NAME         STATE     LAST ACTIVE  DIRECTORY\n" +
		"b            attached  now          " + moved + "\n" +
		"long_name-1  detached  now          " + elsewhere + "\n"
	checkListed(t, s, "", nil, want)
	// tmux writes to a client whose locale is not UTF-8 with "_" for what it cannot print: the
	// tabs, and the "é" (decision 37).
	checkListed(t, s, "LANG=C", map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}, want)
	// A session renamed by hand is on no server of its name: it has ended, as cld's record has it,
	// in the directory it started in.
	s.MustTmux("cld-long_name-1", "rename-session", "-t", "=cld-long_name-1", "cld-renamed")
	want = "NAME         STATE     LAST ACTIVE  DIRECTORY\n" +
		"b            attached  now          " + moved + "\n" +
		"long_name-1  ended     -            " + elsewhere + "\n"
	checkListed(t, s, "renamed", nil, want)
}

// checkListed runs cld list with the variables extra, and checks that it prints want, or nothing
// where want is empty; when names the case in a failure.
func checkListed(
	t *testing.T, s *sandbox.Sandbox, when string, extra map[string]string, want string,
) {
	t.Helper()
	result := s.RunCld(extra, "list")
	if result.Code == 0 && result.Stdout == want && result.Stderr == "" {
		return
	}
	switch {
	case want == "":
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			when, result.Code, result.Stdout, result.Stderr)
	case when == "":
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	default:
		t.Errorf("%s: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			when, result.Code, result.Stderr, result.Stdout, want)
	}
}

// list asks the servers eight at a time, and shows the sessions in the order of their names
// (decision 38.2). A tmux first on the PATH holds every list-sessions until the test lets them go:
// eight of the 12 begin, and the rest only once they are let go.
func TestListAsksServersAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	var names []string
	// Each on a server marked as cld marks its own (see TestLeavesAForeignServerAlone).
	for i := range 12 {
		name := "cld-a" + strconv.Itoa(i)
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", name, "-c", s.Work, "sleep", "600")
		names = append(names, strings.TrimPrefix(name, "cld-"))
	}
	asks := holdTmux(t, s, "list's asks", "*list-sessions*")
	asks.start(t)
	var stdout, stderr bytes.Buffer
	argv := s.CldArgv("list")
	list := exec.Command(argv[0], argv[1:]...)
	list.Env, list.Dir, list.Stdout, list.Stderr = s.Environ(asks.env), s.Work, &stdout, &stderr
	if err := list.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = list.Process.Kill() //nolint:errcheck // list may have ended
		_ = list.Wait()         //nolint:errcheck // the test may have waited for it
	})
	sandbox.WaitFor(t, 10*time.Second, "eight asks to begin", func() bool {
		return asks.begun(t) >= 8
	})
	// The rest would begin within this, were they not held back.
	time.Sleep(time.Second)
	if begun := asks.begun(t); begun != 8 {
		t.Errorf("%d asks began at once, want 8", begun)
	}
	asks.release(t)
	err := list.Wait()
	slices.Sort(names)
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n"
	for _, name := range names {
		want += fmt.Sprintf("%-6sdetached  now          %s\n", name, s.Work)
	}
	if err != nil || stdout.String() != want || stderr.String() != "" {
		t.Errorf("list: %v, stderr %q, stdout\n%s\nwant\n%s",
			err, stderr.String(), stdout.String(), want)
	}
	if begun := asks.begun(t); begun != 12 {
		t.Errorf("%d asks began, want 12", begun)
	}
}
