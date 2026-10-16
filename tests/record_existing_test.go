package tests

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// join --existing on sessions that do not run (decision 58.1): it resumes one that a stop ended,
// whose run mark remains, and refuses the others, writing nothing.

// A session whose server kill-server ended, as a reboot or a crash would, keeps its run mark. From
// elsewhere, join --existing resumes its conversation by the entry's ID, in the directory it ran
// in, and attaches, though the mark is older than CLD_IDLE_DAYS, unlike restore.
func TestJoinExistingStopped(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir, elsewhere := filepath.Join(s.Root, "project"), filepath.Join(s.Root, "elsewhere")
	makeDirs(t, dir, elsewhere)
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	first := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	checkMarks(t, s, "after join", map[string]bool{"api-1": true})
	s.MustTmux("cld-api-1", "kill-server")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !first.Alive() })
	waitServerExit(t, s, "api-1")
	checkMarks(t, s, "after kill-server", map[string]bool{"api-1": true})
	age(t, 2*24*time.Hour, companionFile(s, "api-1", ".run"))

	term := startCldIn(t, s, "tmux", elsewhere, map[string]string{"CLD_IDLE_DAYS": "1"},
		"join", "--existing", "-n", "api", "-s", "1")
	resumed := s.WaitProbes(2)[1]
	want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", dir),
		"--resume", firstID}
	if !slices.Equal(resumed.Argv, want) || resumed.Cwd != dir {
		t.Errorf("claude arguments %q in %s, want %q in %s", resumed.Argv, resumed.Cwd, want, dir)
	}
	waitClients(t, s, 1)
	if !term.Running() {
		t.Errorf("join --existing is not attached:\n%s", term.Screen())
	}
}

// The list's Ctrl+X ends my-a, cld kill my-b, and claude's exit with status 0 my-c: each removes
// the run mark. join --existing refuses them, naming the join that resumes each. It starts no
// claude, makes no server and changes no file of the record.
func TestJoinExistingEnded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "my-a", "my-b", "my-c")
	killFirstRow(t, s)
	mustKill(t, s, "my-b")
	probes["my-c"].Send("exit")
	for name, probe := range probes {
		sandbox.WaitFor(t, 10*time.Second, "claude "+name+" to exit", func() bool {
			return !probe.Alive()
		})
		waitServerExit(t, s, name)
	}
	ended := map[string]bool{"my-a": false, "my-b": false, "my-c": false}
	checkMarks(t, s, "after the sessions ended", ended)
	before, sockets := recordState(t, s), s.Servers()
	for _, suffix := range []string{"a", "b", "c"} {
		checkJoinRefused(t, "join --existing of my-"+suffix,
			s.RunCldOnTerminal(unknownTerminal, "join", "--existing", "-n", "my", "-s", suffix),
			"cld: session 'my-"+suffix+"' has ended: cld join -n my -s "+suffix+" resumes it\n")
	}
	checkNothingMade(t, s, before, sockets, 3)
	checkMarks(t, s, "after join --existing", ended)
}

// killFirstRow kills the session on the first row of the interactive list, with Ctrl+X twice, and
// leaves the list.
func killFirstRow(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
	waitScreen(t, term, endedHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}

// join --existing refuses a session there is none of, as -n and -s or -s alone name it, with
// "no session". So it refuses one whose entry has expired, run mark and all. It writes no entry,
// index or mark, and starts no server.
func TestJoinExistingMissing(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeRestorable(t, s, "api-7", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
		s.Environ(nil))
	age(t, 31*24*time.Hour, entryFile(s, "api-7"))
	before := recordState(t, s)
	for _, test := range []struct {
		args []string
		name string
	}{
		{[]string{"join", "--existing", "-n", "api", "-s", "9"}, "api-9"},
		{[]string{"join", "--existing", "-s", "api-9", "--detach-others"}, "api-9"},
		{[]string{"join", "--existing", "-n", "api", "-s", "7"}, "api-7"},
	} {
		checkJoinRefused(t, strings.Join(test.args, " "),
			s.RunCldOnTerminal(unknownTerminal, test.args...),
			"cld: no session '"+test.name+"' (see cld list)\n")
	}
	checkNothingMade(t, s, before, nil, 0)
}

// checkNothingMade reports what refused joins made: claudes beyond probes, a session, a socket
// not among sockets, or a file of the record other than before has it.
func checkNothingMade(
	t *testing.T, s *sandbox.Sandbox, before map[string]string, sockets []string, probes int,
) {
	t.Helper()
	if started := len(s.Probes()); started != probes {
		t.Errorf("%d claudes started, want %d", started, probes)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
	if servers := s.Servers(); !slices.Equal(servers, sockets) {
		t.Errorf("servers' sockets %q, want %q", servers, sockets)
	}
	if after := recordState(t, s); !maps.Equal(after, before) {
		t.Errorf("the record's files\n%q\nwant them as before\n%q", after, before)
	}
}

// recordState is each file of cld's record in s, but for its lock, by path: its time and size.
func recordState(t *testing.T, s *sandbox.Sandbox) map[string]string {
	t.Helper()
	state := map[string]string{}
	root := filepath.Dir(filepath.Dir(entryFile(s, "x")))
	err := filepath.WalkDir(root, func(path string, file fs.DirEntry, err error) error {
		if err != nil || file.IsDir() || file.Name() == "lock" {
			return err
		}
		info, err := file.Info()
		if err != nil {
			return err
		}
		state[path] = info.ModTime().String() + " " + strconv.FormatInt(info.Size(), 10)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return state
}
