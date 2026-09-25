package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// How cld uses its tmux server, independent of the outer terminal: cld runs in the baseline
// terminal (a pane of an outer tmux server).

// remoteControl is the --settings every new claude gets: Remote Control on from the start.
const remoteControl = `{"remoteControlAtStartup":true}`

func TestSessionNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		session string
	}{
		{[]string{"new"}, "cld-main"},
		{[]string{"new", "-n", "review"}, "cld-review"},
		{[]string{"new", "--name", "Fix_42-b"}, "cld-Fix_42-b"},
		{[]string{"new", "--name=x"}, "cld-x"},
		// The spellings of pflag, which reads the options.
		{[]string{"new", "-ny"}, "cld-y"},
		{[]string{"new", "-n=z"}, "cld-z"},
	} {
		t.Run(test.session, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			startCld(t, s, "tmux", nil, test.args...)
			probe := s.WaitProbes(1)[0]
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{test.session}) {
				t.Errorf("sessions %q, want [%s]", sessions, test.session)
			}
			if want := []string{"--name", test.session, "--settings", remoteControl}; !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != s.Work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, s.Work)
			}
		})
	}
}

// tmux starts the claude that new checked. A relative PATH entry before the probe's, "." or an
// empty one, holds a claude of its own, which records that it ran and fails: cld skips it, and
// tmux, handed the probe by its path, does not take it either, as it would the bare word. A
// claude that is a script without #!, which starts the probe, passes the check through /bin/sh
// and starts as tmux's execvp runs it.
func TestStartsTheClaudeItChecks(t *testing.T) {
	t.Parallel()
	const relative = "#!/bin/sh\necho \"$*\" >\"$CLD_PROBE_DIR/relative claude ran\"\nexit 99\n"
	for _, test := range []struct {
		// entry is the PATH entry before the sandbox's; claude there is script, in the working
		// directory for a relative entry.
		name, entry, script string
	}{
		{"after '.'", ".", relative},
		{"after ''", "", relative},
		{"without #!", "bin", "exec '" + filepath.Join(sandbox.ProbeBin, "claude") + "' \"$@\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir, entry := s.Work, test.entry
			if entry == "bin" {
				dir = filepath.Join(s.Root, "bin")
				entry = dir
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			s.WriteProgram(filepath.Join(dir, "claude"), test.script, 0o755)
			startCld(t, s, "tmux", map[string]string{"PATH": entry + string(os.PathListSeparator) + s.Env["PATH"]}, "new")
			probe := s.WaitProbes(1)[0]
			if want := []string{"--name", "cld-main", "--settings", remoteControl}; !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if args, err := os.ReadFile(filepath.Join(s.ProbeDir, "relative claude ran")); err == nil {
				t.Errorf("the claude of the relative entry ran with %q", args)
			}
		})
	}
}

// new -w hands the worktree to claude: claude gets --worktree NAME, with settings that also make it
// branch a new worktree from HEAD, and starts where cld runs; it then makes or reopens the
// worktree itself and moves into it.
func TestNewWorktree(t *testing.T) {
	t.Parallel()
	const fromHead = `{"remoteControlAtStartup":true,"worktree":{"baseRef":"head"}}`
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"new", "-w"}, []string{"--name", "cld-main", "--settings", fromHead, "--worktree", "main"}},
		{[]string{"new", "-n", "feat", "--worktree"}, []string{"--name", "cld-feat", "--settings", fromHead, "--worktree", "feat"}},
		{[]string{"new", "-w", "--name=feat"}, []string{"--name", "cld-feat", "--settings", fromHead, "--worktree", "feat"}},
		{[]string{"new", "-wn", "feat"}, []string{"--name", "cld-feat", "--settings", fromHead, "--worktree", "feat"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			sub := filepath.Join(s.Work, "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", sub, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			if !slices.Equal(probe.Argv, test.want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, test.want)
			}
			if probe.Cwd != sub {
				t.Errorf("claude starts in %s, want %s", probe.Cwd, sub)
			}
		})
	}
}

// resume makes its session as new does, in the current directory, and claude gets new's arguments
// - never -w's - then --resume with the conversation: the one named like the session, or SESSION,
// as one word, whatever it holds. tmux would end its command at a word ending in ";" (see
// literal in internal/session), and a git repository makes no worktree session.
func TestResume(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args         []string
		name, resume string
	}{
		{[]string{"resume"}, "main", "cld-main"},
		{[]string{"resume", "-n", "x"}, "x", "cld-x"},
		{[]string{"resume", "--name=x"}, "x", "cld-x"},
		{[]string{"resume", "-n", "x", "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"}, "x", "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"},
		{[]string{"resume", "-n", "x", "a b"}, "x", "a b"},
		{[]string{"resume", "-n", "x", "fix;"}, "x", "fix;"},
		{[]string{"resume", "-n", "x", `fix\;`}, "x", `fix\;`},
		{[]string{"resume", "-n", "x", "#{session_name}"}, "x", "#{session_name}"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			startCld(t, s, "tmux", nil, test.args...)
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-" + test.name}) {
				t.Errorf("sessions %q, want [cld-%s]", sessions, test.name)
			}
			if want := []string{"--name", "cld-" + test.name, "--settings", remoteControl, "--resume", test.resume}; !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != s.Work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, s.Work)
			}
			list := fmt.Sprintf("NAME  STATE     DIRECTORY\n%-4s  attached  %s\n", test.name, s.Work)
			if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
			}
		})
	}
}

// tmux would change the directory cld runs in, which cld gives it with -c: tmux ends a command at
// a word ending in ";" (see literal in internal/session), and expands -c as a format (see
// unexpanded), where "#S" is the session's name and "#(touch ran)" runs touch. new and resume
// still make their session there, with claude in that directory, and run nothing. The session's
// path is -c as tmux expanded it: for a directory that does not exist, tmux starts claude in the
// home directory.
func TestDirectoryTmuxWouldChange(t *testing.T) {
	t.Parallel()
	for _, command := range []struct {
		args, argv []string
	}{
		{[]string{"new", "-n", "x"}, []string{"--name", "cld-x", "--settings", remoteControl}},
		{[]string{"resume", "-n", "x"}, []string{"--name", "cld-x", "--settings", remoteControl, "--resume", "cld-x"}},
	} {
		for _, name := range []string{"w;", "C#S", "x#(touch ran)", "#{session_name};"} {
			args, argv := command.args, command.argv
			t.Run(strings.Join(args, " ")+" in "+name, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				dir := filepath.Join(s.Work, name)
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				startCldIn(t, s, "tmux", dir, nil, args...)
				probe := s.WaitProbes(1)[0]
				if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
					t.Errorf("sessions %q, want [cld-x]", sessions)
				}
				if !slices.Equal(probe.Argv, argv) {
					t.Errorf("claude arguments %q, want %q", probe.Argv, argv)
				}
				if probe.Cwd != dir {
					t.Errorf("claude runs in %s, want %s", probe.Cwd, dir)
				}
				if path := s.Format("cld-x", "#{session_path}"); path != dir {
					t.Errorf("session path %s, want %s", path, dir)
				}
				if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
					t.Errorf("tmux ran touch from the directory's name")
				}
			})
		}
	}
}

// Outside a git work tree new -w fails before starting anything: claude would say so in a
// session left to kill.
func TestNewWorktreeRequiresRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: --worktree needs a git repository, and " + s.Work + " is not in one\n"
	if result := s.RunCld(nil, "new", "-n", "feat", "-w"); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// new and resume refuse a session that exists, before touching it: the attached client and its
// claude carry on.
func TestNewRefusesExistingSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "dup")
	s.WaitProbes(1)
	waitClients(t, s, 1)

	for _, args := range [][]string{{"new", "-n", "dup"}, {"resume", "-n", "dup"}, {"resume", "-n", "dup", "other"}} {
		result := s.RunCld(nil, args...)
		if want := "cld: session 'dup' exists; attach to it with cld join -n dup\n"; result.Code != 1 || result.Stderr != want {
			t.Errorf("%q: exit %d, stderr %q, want exit 1, stderr %q", args, result.Code, result.Stderr, want)
		}
		if result.Stdout != "" {
			t.Errorf("%q printed %q before failing", args, result.Stdout)
		}
	}
	if probes := s.Probes(); len(probes) != 1 || !first.Running() {
		t.Errorf("%d claude processes, first client running: %v; want the original one, attached", len(probes), first.Running())
	}
}

// join finds only the session it names: without its server there is none, and session review,
// on a server of its own, is not session rev, although its name starts with rev. A session like
// that on the named session's own server is TestSeesOnlyItsOwnSessions' to check.
func TestJoinRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev'; create it with cld new -n rev\n"
	if result := s.RunCld(nil, "join", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("without a server: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}

	startCld(t, s, "tmux", nil, "new", "-n", "review")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	if result := s.RunCld(nil, "join", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("beside cld-review: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	waitClients(t, s, 1)
}

// list shows cld's sessions: the name, whether a terminal is attached, and the directory claude
// is in now, also under a locale that is not UTF-8. It asks each server for the session named
// like it, and shows no other: none that claude made on its server, none renamed by hand. Without
// a server, or with none of cld's sessions on the servers, there is nothing to show, and it shows
// nothing, not even the header.
func TestList(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("without a server: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	// A server named like a session of cld's, holding a session of another name; a socket named
	// like no session of cld's can be; and the server cld 0.3.0 and earlier shared (see
	// TestLeavesTheSharedServerAlone).
	others := sandbox.New(t)
	others.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	others.MustTmux("cld-x.y", "-f", "/dev/null", "new-session", "-d", "-s", "cld-x.y", "sleep", "600")
	others.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	if result := others.RunCld(nil, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("with none of cld's sessions: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}

	elsewhere, moved := filepath.Join(s.Root, "elsewhere"), filepath.Join(s.Work, "café")
	for _, dir := range []string{elsewhere, moved} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	b := s.WaitProbes(1)[0]
	detached := startCldIn(t, s, "tmux", elsewhere, nil, "new", "-n", "long_name-1")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	detached.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detached.Running() })
	// A session that claude makes on its server is not cld's to show, even one listed before cld's:
	// tmux names a session it is given no name for with a number, as a bare tmux in claude's pane
	// would.
	s.MustTmux("cld-b", "new-session", "-d", "sleep", "60")
	b.Send("cd " + moved)
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		return s.Format("cld-b", "#{pane_current_path}") == moved
	})

	want := "NAME         STATE     DIRECTORY\n" +
		"b            attached  " + moved + "\n" +
		"long_name-1  detached  " + elsewhere + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	// tmux writes to a client whose locale is not UTF-8 with "_" for what it cannot print: the
	// tabs, and the "é".
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	if result := s.RunCld(notUTF8, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("LANG=C: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	// A session renamed by hand is no longer the one its server is named after, nor on the server
	// its new name would have.
	s.MustTmux("cld-long_name-1", "rename-session", "-t", "=cld-long_name-1", "cld-renamed")
	want = "NAME  STATE     DIRECTORY\n" + "b     attached  " + moved + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("renamed: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
}

// kill ends the session it names, the claude in it and its server; the terminal attached to it
// is left clean, and its cld exits with status 0, as when claude exits: the session goes before
// the server, which would tell the terminal that the server exited, with status 1. The other
// sessions, on their own servers, carry on.
func TestKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// The terminal runs cld a in a shell that writes down its exit status.
	status := filepath.Join(s.Root, "a.status")
	a := terminal.New(t, "tmux", s)
	a.Start(append([]string{"sh", "-c", `"$@"; echo $? >"$0"`, status}, s.CldArgv("new", "-n", "a")...), s.Env, s.Work)
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)

	if result := s.RunCld(nil, "kill", "-n", "a"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["cld-a"].Alive() })
	sandbox.WaitFor(t, 10*time.Second, "cld a to return", func() bool { return !a.Running() })
	if code, _ := os.ReadFile(status); string(code) != "0\n" {
		t.Errorf("cld a exited with status %q, want 0:\n%s", strings.TrimSpace(string(code)), strings.TrimSpace(a.Screen()))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if modes := a.Modes(); modes.AltScreen || modes.Mouse {
		t.Errorf("terminal modes after the kill %+v, want none", modes)
	}
	if !probes["cld-b"].Alive() {
		t.Error("claude b did not survive session a's kill")
	}
	if _, err := s.Tmux("cld-a", "list-sessions"); err == nil {
		t.Error("session a's server survived its kill")
	}

	if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
		t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "session b's server to exit", func() bool {
		_, err := s.Tmux("cld-b", "list-sessions")
		return err != nil
	})
}

// kill ends only the session it names: without its server there is none, and session review,
// on a server of its own, is not session rev, although its name starts with rev. A session like
// that on the named session's own server is TestSeesOnlyItsOwnSessions' to check.
func TestKillRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev' (see cld list)\n"
	if result := s.RunCld(nil, "kill", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("without a server: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}

	startCld(t, s, "tmux", nil, "new", "-n", "review")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if result := s.RunCld(nil, "kill", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("beside cld-review: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-review"}) || !probe.Alive() {
		t.Errorf("sessions %q, claude alive: %v; want cld-review running", sessions, probe.Alive())
	}
}

// cld sees only the sessions it started, each cld-NAME on its server cld-NAME. A bare tmux that
// claude runs reaches claude's own server through TMUX, and a session made that way has another
// name there, even named like a session of cld's: list leaves it out, join and kill act as for no
// session, and new and resume make a session of that name on a server of its own. Beside one
// whose name starts with claude's session's, as cld-a-x does with cld-a, cld still finds cld-a,
// by its whole name: new and resume say that a exists, join attaches to it, and kill ends it.
// kill ends the others with the server. What cld sets for a failed claude stays on claude's
// window: a session claude makes whose program fails closes, as tmux would close it.
func TestSeesOnlyItsOwnSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for _, session := range []string{"cld-inside", "cld-a-x"} {
		probe.Send("tmux new-session -d -s " + session + " sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make "+session, func() bool {
			return slices.Contains(s.Sessions(), "cld-a/"+session)
		})
	}
	// As claude's tmux would make it, but made once this returns.
	s.MustTmux("cld-a", "new-session", "-d", "-s", "cld-failing", "false")
	sandbox.WaitFor(t, 10*time.Second, "the failing session to close", func() bool {
		return !slices.Contains(s.Sessions(), "cld-a/cld-failing")
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-a/cld-a-x", "cld-a/cld-inside"}) {
		t.Fatalf("sessions %q, want [cld-a cld-a/cld-a-x cld-a/cld-inside]", sessions)
	}

	want := "NAME  STATE     DIRECTORY\n" + "a     attached  " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	exists := "cld: session 'a' exists; attach to it with cld join -n a\n"
	for _, command := range []string{"new", "resume"} {
		if result := s.RunCld(nil, command, "-n", "a"); result.Code != 1 || result.Stdout != "" || result.Stderr != exists {
			t.Errorf("%s -n a: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stdout, result.Stderr, exists)
		}
	}
	second := startCld(t, s, "tmux", nil, "join", "-n", "a")
	sandbox.WaitFor(t, 10*time.Second, "join -n a to detach the first client", func() bool { return !first.Running() })
	waitScreen(t, second, "probe --name cld-a")
	for _, test := range []struct{ command, want string }{
		{"join", "cld: no session 'inside'; create it with cld new -n inside\n"},
		{"kill", "cld: no session 'inside' (see cld list)\n"},
	} {
		if result := s.RunCld(nil, test.command, "-n", "inside"); result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n inside: exit %d, stderr %q, want exit 1, stderr %q", test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "new", "-n", "inside")
	if probe := s.WaitProbes(2)[1]; !slices.Equal(probe.Argv[:2], []string{"--name", "cld-inside"}) {
		t.Errorf("new -n inside started claude with %q", probe.Argv)
	}
	startCld(t, s, "tmux", nil, "resume", "-n", "a-x")
	resumed := []string{"--name", "cld-a-x", "--settings", remoteControl, "--resume", "cld-a-x"}
	if probes := s.WaitProbes(3); !slices.ContainsFunc(probes, func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }) {
		t.Errorf("resume -n a-x started no claude with %q", resumed)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-a-x", "cld-a/cld-a-x", "cld-a/cld-inside", "cld-inside"}) {
		t.Errorf("sessions %q, want [cld-a cld-a-x cld-a/cld-a-x cld-a/cld-inside cld-inside]", sessions)
	}

	if result := s.RunCld(nil, "kill", "-n", "a"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("kill -n a: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "the cld joined to a to return", func() bool { return !second.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a-x", "cld-inside"}) {
		t.Errorf("sessions %q after kill -n a, want [cld-a-x cld-inside]", sessions)
	}
}

// A server outlives its session when claude exits while the tmux sessions it made keep the
// server running. list shows nothing for it, and new, resume, join and kill refuse the name,
// pointing at the server: new and resume would start claude there with the environment of the
// cld that started the server, join finds no session to attach to, and kill leaves what claude
// made to the user. Once the server is gone new starts a fresh one.
func TestRefusesALingeringServer(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "new", "-n", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a/side"}) {
		t.Fatalf("sessions %q, want [cld-a/side]", sessions)
	}

	const lingering = "cld: session 'a' has ended, but its tmux server still runs (see tmux -L cld-a ls); end it with tmux -L cld-a kill-server\n"
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"list"}, 0, ""},
		{[]string{"join", "-n", "a"}, 1, lingering},
		{[]string{"kill", "-n", "a"}, 1, lingering},
		{[]string{"new", "-n", "a"}, 1, lingering},
		{[]string{"resume", "-n", "a"}, 1, lingering},
		{[]string{"resume", "-n", "a", "SESSION"}, 1, lingering},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.code, test.want)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a/side"}) || len(s.Probes()) != 1 {
		t.Errorf("sessions %q, %d claude processes; want [cld-a/side] and the first one", sessions, len(s.Probes()))
	}

	s.MustTmux("cld-a", "kill-server")
	startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
}

// Where tmux's socket directory ignores case, as macOS's does by default, names that differ only
// in case share one socket: tmux -L cld-A reaches the server of session a, which has no session
// cld-A. new, resume, join and kill refuse A and name session a, rather than take its server for
// one that outlived session A and point at a kill-server that would end a - also once a's claude
// has exited and a session it made keeps the server running. list shows a once. Where the
// sandbox's socket directory ignores case, as on macOS, the name cld-A finds a's socket already
// and the test runs against the real thing; elsewhere a symlink cld-A to a's socket plays such a
// directory, as a casefold tmpfs does on Linux (see docs/design.md, Findings).
func TestNamesDifferingInCase(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "new", "-n", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	socket := filepath.Join(s.SocketDir(), "cld-A")
	if _, err := os.Lstat(socket); errors.Is(err, os.ErrNotExist) {
		if err := os.Symlink("cld-a", socket); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	const clash = "cld: session name 'A' clashes with session 'a': tmux's socket directory ignores case here, so both names reach server cld-a (see tmux -L cld-a ls)\n"
	refused := func(when string) {
		t.Helper()
		for _, command := range []string{"new", "resume", "join", "kill"} {
			if result := s.RunCld(nil, command, "-n", "A"); result.Code != 1 || result.Stdout != "" || result.Stderr != clash {
				t.Errorf("%s: %s -n A: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", when, command, result.Code, result.Stdout, result.Stderr, clash)
			}
		}
	}
	refused("a running")
	want := "NAME  STATE     DIRECTORY\n" + "a     attached  " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	if !probe.Alive() || len(s.Probes()) != 1 {
		t.Fatalf("claude a alive: %v, %d claude processes; want it alive and alone", probe.Alive(), len(s.Probes()))
	}

	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	refused("a lingering")
	if sessions := s.MustTmux("cld-a", "list-sessions", "-F", "#{session_name}"); sessions != "side" {
		t.Errorf("sessions on a's server %q, want side", sessions)
	}
}

// A server that dies - SIGKILL - leaves its socket behind, as tmux leaves every socket: list
// passes over it and lists the other sessions, join and kill find no session, and new starts a
// fresh server on it.
func TestStaleSocket(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	pid, err := strconv.Atoi(s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld a to lose its server", func() bool { return !a.Running() })
	if _, err := os.Stat(filepath.Join(s.SocketDir(), "cld-a")); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}

	want := "NAME  STATE     DIRECTORY\n" + "b     attached  " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	for _, test := range []struct{ command, want string }{
		{"join", "cld: no session 'a'; create it with cld new -n a\n"},
		{"kill", "cld: no session 'a' (see cld list)\n"},
	} {
		if result := s.RunCld(nil, test.command, "-n", "a"); result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n a: exit %d, stderr %q, want exit 1, stderr %q", test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(3)
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// cld 0.3.0 and earlier ran every session on one server, -L cld. cld leaves it alone: list does
// not show its sessions, join and kill find none there, and new makes a session of that name on
// a server of its own.
func TestLeavesTheSharedServerAlone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"list"}, 0, ""},
		{[]string{"join", "-n", "old"}, 1, "cld: no session 'old'; create it with cld new -n old\n"},
		{[]string{"kill", "-n", "old"}, 1, "cld: no session 'old' (see cld list)\n"},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.code, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "new", "-n", "old")
	s.WaitProbes(1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-old"}) {
		t.Errorf("sessions on cld's servers %q, want [cld-old]", sessions)
	}
	if old := s.MustTmux("cld", "list-sessions", "-F", "#{session_name}"); old != "cld-old" {
		t.Errorf("sessions on the shared server %q, want the one there", old)
	}
}

// join -n completes the names of the sessions list shows - attached, detached, or with claude
// exited - that start with what was typed, in list's order, each with its state, and then offers
// no file names (":4", ShellCompDirectiveNoFileComp). That is cobra's __complete, which the
// completion scripts run on every TAB; it starts no server and no claude. As list does, it asks
// the server of each socket for its own session, and leaves out the sessions cld did not start -
// one that claude makes, on its own session's server under another name, one made there by hand,
// and one on the server that cld 0.3.0 and earlier shared - a session renamed by hand, whose
// server then runs without it, and a stale socket, whose server has died. new -n, resume -n and
// resume's SESSION offer nothing, and neither do the other arguments, file names included.
func TestCompleteNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "__complete", "join", "-n", ""); result.Code != 0 || result.Stdout != ":4\n" {
		t.Errorf("without a server: exit %d, stdout %q, want exit 0, stdout %q", result.Code, result.Stdout, ":4\n")
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after completing without a server, want none", sessions)
	}

	startCld(t, s, "tmux", nil, "new", "-n", "rev")
	rev := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for i, name := range []string{"review", "cafe", "bad", "gone"} {
		term := startCld(t, s, "tmux", nil, "new", "-n", name)
		s.WaitProbes(2 + i)
		waitClients(t, s, 2)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld "+name+" to detach", func() bool { return !term.Running() })
	}
	for _, probe := range s.Probes() {
		if probe.Argv[1] == "cld-bad" {
			probe.Send("exit 1")
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "claude bad to exit", func() bool { return s.Format("cld-bad", "#{pane_dead}") == "1" })
	// Renamed by hand, session cafe is no longer cld-cafe, which its server runs without.
	s.MustTmux("cld-cafe", "rename-session", "-t", "=cld-cafe", "cld-café")
	// A server that dies leaves its socket behind.
	pid, err := strconv.Atoi(s.MustTmux("cld-gone", "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "server cld-gone to die", func() bool {
		_, err := s.Tmux("cld-gone", "list-sessions")
		return err != nil
	})
	if _, err := os.Stat(filepath.Join(s.SocketDir(), "cld-gone")); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}
	rev.Send("tmux new-session -d -s cld-inside sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-rev/cld-inside")
	})
	s.MustTmux("cld-review", "new-session", "-d", "-s", "cld-by-hand", "sleep", "600")
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	want := []string{"cld-bad", "cld-cafe/cld-café", "cld-rev", "cld-rev/cld-inside", "cld-review", "cld-review/cld-by-hand"}
	if sessions := s.Sessions(); !slices.Equal(sessions, want) {
		t.Fatalf("sessions %q, want %q", sessions, want)
	}
	probes := len(s.Probes())

	listed := []string{"bad", "rev", "review"}
	names := []string{"bad\texited", "rev\tattached", "review\tdetached"}
	result := s.RunCld(nil, "list")
	var shown []string
	for _, line := range strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")[1:] {
		shown = append(shown, strings.Fields(line)[0])
	}
	if result.Code != 0 || !slices.Equal(shown, listed) {
		t.Errorf("list: exit %d, names %q, want %q:\n%s", result.Code, shown, listed, result.Stdout)
	}

	// offered is what __complete prints for names, each one line, and then the directive.
	offered := func(names ...string) string {
		return strings.Join(append(names, ":4"), "\n") + "\n"
	}
	all := offered(names...)
	var bare []string
	for _, name := range names {
		bare = append(bare, strings.Split(name, "\t")[0])
	}
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	for _, test := range []struct {
		args []string
		env  map[string]string
		want string
	}{
		{[]string{"__complete", "join", "-n", ""}, nil, all},
		{[]string{"__complete", "join", "--name", ""}, nil, all},
		{[]string{"__complete", "join", "--name="}, nil, all},
		{[]string{"__complete", "join", "-n", "re"}, nil, offered("rev\tattached", "review\tdetached")},
		{[]string{"__complete", "join", "-n", "revi"}, nil, offered("review\tdetached")},
		{[]string{"__complete", "join", "-n", "x"}, nil, offered()},
		{[]string{"__complete", "join", "-n", "caf"}, nil, offered()},
		{[]string{"__complete", "join", "-n", "g"}, nil, offered()},
		{[]string{"__complete", "join", "-n", "in"}, nil, offered()},
		{[]string{"__complete", "join", "-n", "b"}, nil, offered("bad\texited")},
		{[]string{"__complete", "join", "-n", "o"}, nil, offered()},
		// pflag's -n=NAME; in -nNAME cobra takes the word for options, and finds none.
		{[]string{"__complete", "join", "-n=re"}, nil, offered("rev\tattached", "review\tdetached")},
		{[]string{"__complete", "join", "-nre"}, nil, offered()},
		{[]string{"__completeNoDesc", "join", "-n", ""}, nil, offered(bare...)},
		{[]string{"__complete", "join", "-n", ""}, map[string]string{"CLD_COMPLETION_DESCRIPTIONS": "0"}, offered(bare...)},
		{[]string{"__complete", "join", "-n", ""}, notUTF8, all},
		{[]string{"__complete", "new", "-n", ""}, nil, offered()},
		{[]string{"__complete", "resume", "-n", ""}, nil, offered()},
		{[]string{"__complete", "resume", ""}, nil, offered()},
		{[]string{"__complete", "resume", "-n", "rev", ""}, nil, offered()},
		{[]string{"__complete", "kill", "-n", ""}, nil, offered()},
		{[]string{"__complete", "join", ""}, nil, offered()},
		{[]string{"__complete", "list", ""}, nil, offered()},
		// cobra answers these with ShellCompDirectiveDefault, and the shell would offer files.
		{[]string{"__complete", "joni", "-n", ""}, nil, offered()},
		{[]string{"__complete", "join", "-x", "-n", ""}, nil, offered()},
		{[]string{"__complete", "list", "-n", ""}, nil, offered()},
	} {
		if result := s.RunCld(test.env, test.args...); result.Code != 0 || result.Stdout != test.want {
			t.Errorf("cld %q, %v: exit %d, stdout\n%s\nwant\n%s", test.args, test.env, result.Code, result.Stdout, test.want)
		}
	}
	if count := len(s.Probes()); count != probes {
		t.Errorf("%d claude processes after completing, want %d", count, probes)
	}
}

func TestJoinDetachesOtherClient(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "shared")
	s.WaitProbes(1)
	waitClients(t, s, 1)

	second := startCld(t, s, "tmux", nil, "join", "-n", "shared")
	sandbox.WaitFor(t, 10*time.Second, "the first client to be detached", func() bool { return !first.Running() })
	waitScreen(t, second, "probe --name cld-shared")
	if !second.Running() {
		t.Error("the second client is not attached")
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want 1", len(probes))
	}
}

// Joining from another directory leaves claude, and the session, where they are.
func TestJoinFromElsewhereKeepsClaude(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "task")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	second := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "task")
	waitScreen(t, second, "probe --name cld-task")
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() || probe.Cwd != s.Work {
		t.Errorf("%d claude processes after joining, want the original one in %s", len(probes), s.Work)
	}
	if path := s.Format("cld-task", "#{session_path}"); path != s.Work {
		t.Errorf("session directory %s after joining, want %s", path, s.Work)
	}
}

// A reattach repaints claude's screen from tmux's own copy of it: the same text in the same
// attributes - also after claude pushed its keyboard modes again, as it does after an external
// editor, and in a terminal of another size - and claude has the alternate screen, mouse
// reporting and the wheel back.
func TestReattachRepaintsTheSameScreen(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "paint")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("rekey")
	waitScreen(t, first, "repainted")
	painted := cells(first.Styled())
	if underlined(painted) {
		t.Errorf("claude's screen is underlined:\n%s", strings.Join(painted, "\n"))
	}
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	for _, size := range [][2]int{{120, 40}, {100, 30}} {
		term := terminal.New(t, "tmux", s)
		term.Resize(size[0], size[1])
		term.Start(s.CldArgv("join", "-n", "paint"), s.Env, s.Work)
		waitScreen(t, term, "repainted")
		if repainted := cells(term.Styled()); !slices.Equal(repainted, painted) {
			t.Errorf("reattached at %dx%d:\n%s\nwant\n%s", size[0], size[1], strings.Join(repainted, "\n"), strings.Join(painted, "\n"))
		}
		if modes := term.Modes(); !modes.AltScreen || !modes.Mouse {
			t.Errorf("modes after reattaching at %dx%d %+v, want the alternate screen and mouse reporting on", size[0], size[1], modes)
		}
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
}

// claude exiting closes its own session only, and its server: the other session, its client and
// its claude carry on.
func TestClaudeExitClosesOnlyItsSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	b := startCld(t, s, "tmux", nil, "new", "-n", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)
	probes["cld-a"].Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld a to exit", func() bool { return !a.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if !b.Running() || !probes["cld-b"].Alive() {
		t.Error("session b did not survive session a's claude exiting")
	}
	sandbox.WaitFor(t, 10*time.Second, "session a's server to exit", func() bool {
		_, err := s.Tmux("cld-a", "list-sessions")
		return err != nil
	})
}

// A claude that fails keeps its session: the terminal stays attached and shows claude's last
// words and how to end the session, list says claude exited, and new and resume refuse the name
// until kill ends the session. A resume whose claude finds no conversation fails that way.
func TestFailedClaudeKeepsSession(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		// startup is what claude prints as it fails at startup; fail makes it fail later instead
		startup string
		// how the hint says claude exited: tmux names a signal where the C library has
		// sys_signame (macOS), and numbers it elsewhere
		how  []string
		fail func(t *testing.T, s *sandbox.Sandbox)
	}{
		{"start", []string{"new", "-n", "bad"}, "Error: cannot start", []string{"status 1"}, nil},
		{"resume", []string{"resume", "-n", "bad", "x"}, "No conversation found with session ID: x", []string{"status 1"}, nil},
		{"status", []string{"new", "-n", "bad"}, "", []string{"status 3"}, func(t *testing.T, s *sandbox.Sandbox) {
			s.WaitProbes(1)[0].Send("exit 3")
		}},
		{"signal", []string{"new", "-n", "bad"}, "", []string{"signal 15", "signal term"}, func(t *testing.T, s *sandbox.Sandbox) {
			if err := syscall.Kill(s.WaitProbes(1)[0].PID, syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			extra := map[string]string{}
			if test.startup != "" {
				extra["CLD_PROBE_FAIL"] = test.startup
			}
			term := startCld(t, s, "tmux", extra, test.args...)
			if test.fail != nil {
				waitClients(t, s, 1)
				test.fail(t, s)
			}
			if test.startup != "" {
				waitScreen(t, term, test.startup)
			}
			hint := ": C-q d detaches, cld kill -n bad ends the session"
			waitScreen(t, term, hint)
			if !slices.ContainsFunc(test.how, func(how string) bool {
				return strings.Contains(term.Screen(), "claude exited with "+how+hint)
			}) {
				t.Errorf("the hint does not say claude exited with %s:\n%s", strings.Join(test.how, " or "), term.Screen())
			}
			if !term.Running() {
				t.Error("the terminal was detached")
			}

			list := "NAME  STATE     DIRECTORY\n" +
				"bad   exited    " + s.Work + "\n"
			if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
			}
			want := "cld: session 'bad' exists, but its claude exited; end it with cld kill -n bad\n"
			for _, command := range []string{"new", "resume"} {
				if result := s.RunCld(nil, command, "-n", "bad"); result.Code != 1 || result.Stderr != want {
					t.Errorf("%s: exit %d, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stderr, want)
				}
			}
			if result := s.RunCld(nil, "kill", "-n", "bad"); result.Code != 0 {
				t.Errorf("kill: exit %d, stderr %q", result.Code, result.Stderr)
			}
			sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q after kill, want none", sessions)
			}
		})
	}
}

// A claude that fails with no terminal attached leaves the hint to join, which shows it on the
// message line: from the hook, tmux would keep it and show it in view-mode over the next session
// any terminal attaches to. Joining a live session shows no hint.
func TestClaudeFailingDetached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "bad")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })
	probe.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-bad", "#{pane_dead}") == "1" })

	other := startCld(t, s, "tmux", nil, "new", "-n", "other")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	if mode := s.Format("cld-other", "#{pane_mode}"); mode != "" {
		t.Errorf("a new session opens in %s:\n%s", mode, other.Screen())
	}

	hint := "claude exited with status 1: C-q d detaches, cld kill -n bad ends the session"
	joined := startCld(t, s, "tmux", nil, "join", "-n", "bad")
	waitScreen(t, joined, hint)
	if screen := joined.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
		t.Errorf("the hint is not on the message line:\n%s", screen)
	}
	if mode := s.Format("cld-bad", "#{pane_mode}"); mode != "" {
		t.Errorf("the joined session is in %s", mode)
	}

	live := startCld(t, s, "tmux", nil, "join", "-n", "other")
	waitScreen(t, live, "probe --name cld-other")
	if screen := live.Screen(); strings.Contains(screen, "claude exited") {
		t.Errorf("joining a live session shows the hint:\n%s", screen)
	}
}

// Each session runs on a server of its own, named like it, and each claude gets the environment of
// the shell that ran cld new or cld resume - where resume's claude looks for the conversation, in
// CLAUDE_CONFIG_DIR. On one server shared by every session, tmux started each pane with the
// environment of the client that had started the server - the first session's - but for PATH and
// the update-environment variables.
func TestServerPerSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	shells := map[string]map[string]string{
		"a": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-a")},
		"b": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-b"), "VIRTUAL_ENV": "/venv/b"},
		"c": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-c")},
	}
	startCld(t, s, "tmux", shells["a"], "new", "-n", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", shells["b"], "new", "-n", "b")
	s.WaitProbes(2)
	startCld(t, s, "tmux", shells["c"], "resume", "-n", "c")
	for _, probe := range s.WaitProbes(3) {
		shell := shells[strings.TrimPrefix(probe.Argv[1], "cld-")]
		for _, name := range []string{"CLAUDE_CONFIG_DIR", "VIRTUAL_ENV"} {
			if value, want := probe.Env[name], shell[name]; value != want {
				t.Errorf("claude %s sees %s=%q, want %q", probe.Argv[1], name, value, want)
			}
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-c], each on its own server", sessions)
	}
}

func TestIgnoresUserTmuxConfig(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new")
	s.WaitProbes(1)
	if left := s.MustTmux("cld-main", "show", "-gv", "status-left"); left == "POISONED" {
		t.Error("~/.tmux.conf was loaded")
	}
	socket := filepath.Join(s.Root, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	if _, err := os.Stat(socket); err == nil {
		t.Error("cld started the default tmux server")
	}
}

func TestServerOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new")
	s.WaitProbes(1)
	for _, option := range []struct{ scope, name, value string }{
		{"-sv", "extended-keys", "on"},
		{"-sv", "focus-events", "on"},
		{"-gv", "mouse", "on"},
		{"-gv", "allow-passthrough", "on"},
		{"-gv", "status", "off"},
		{"-gv", "prefix", "C-q"},
	} {
		if value := s.MustTmux("cld-main", "show", option.scope, option.name); value != option.value {
			t.Errorf("%s is %q, want %q", option.name, value, option.value)
		}
	}
	// What cld sets for a failed claude goes to claude's window; the sessions claude makes on its
	// server keep tmux's.
	for _, option := range []struct {
		args  []string
		value string
	}{
		{[]string{"-wv", "-t", "=cld-main:", "remain-on-exit"}, "failed"},
		{[]string{"-Awv", "-t", "=cld-main:", "remain-on-exit-format"}, ""},
		{[]string{"-gwv", "remain-on-exit"}, "off"},
	} {
		if value := s.MustTmux("cld-main", append([]string{"show"}, option.args...)...); value != option.value {
			t.Errorf("show %s is %q, want %q", strings.Join(option.args, " "), value, option.value)
		}
	}
	if hooks := s.MustTmux("cld-main", "show-hooks", "-g", "pane-died"); strings.Contains(hooks, "[") {
		t.Errorf("global pane-died hooks, want none: they go to claude's window\n%s", hooks)
	}
	if hooks := strings.Split(s.MustTmux("cld-main", "show-hooks", "-w", "-t", "=cld-main:", "pane-died"), "\n"); len(hooks) != 1 || !strings.Contains(hooks[0], "window_active_clients") {
		t.Errorf("pane-died hooks of claude's window, want one:\n%s", strings.Join(hooks, "\n"))
	}
	// Two cld new -n main at once can both set the options on one server: the lookup of each finds
	// no server, and the tmux command of the second reaches the server the first one started,
	// setting them again before its new-session fails. The extkeys feature goes to a fixed index,
	// so it is there once however often it is set. The fake tmux says no server is running, and
	// runs the real one for the rest.
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	second := s.RunCld(map[string]string{
		"PATH":               filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_REAL": realTmux,
	}, "new")
	if want := "duplicate session: cld-main\n"; second.Code != 1 || second.Stderr != want {
		t.Errorf("a second cld new: exit %d, stderr %q, want exit 1, stderr %q", second.Code, second.Stderr, want)
	}
	features := strings.Split(s.MustTmux("cld-main", "show", "-sv", "terminal-features"), "\n")
	if count := len(slices.DeleteFunc(features, func(f string) bool { return f != "xterm*:extkeys" })); count != 1 {
		t.Errorf("%d xterm*:extkeys entries in terminal-features, want 1", count)
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want the first one", len(probes))
	}
	// "list-keys -T prefix C-q" would be shorter, but tmux 3.7 prints nothing for it.
	bindings := strings.Split(s.MustTmux("cld-main", "list-keys", "-T", "prefix"), "\n")
	if !slices.ContainsFunc(bindings, func(binding string) bool {
		return slices.Equal(strings.Fields(binding), []string{"bind-key", "-T", "prefix", "C-q", "send-prefix"})
	}) {
		t.Errorf("C-q C-q is not bound to send-prefix:\n%s", strings.Join(bindings, "\n"))
	}
}

// claude trusts TERMINAL_EMULATOR over TERM_PROGRAM=tmux, and a server keeps the environment of
// the client that started it: unless cld left TERMINAL_EMULATOR out of the environment it runs
// tmux with, a session created in a JetBrains terminal would hand it to its claude, joined from
// anywhere, and to whatever claude starts through tmux on its server.
func TestClaudeNeverSeesTerminalEmulator(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", map[string]string{"TERMINAL_EMULATOR": "JetBrains-JediTerm"}, "new", "-n", "ide")
	probe := s.WaitProbes(1)[0]
	if value, found := probe.Env["TERMINAL_EMULATOR"]; found {
		t.Errorf("claude sees TERMINAL_EMULATOR=%s", value)
	}
	if global := s.MustTmux("cld-ide", "show-environment", "-g"); strings.Contains(global, "TERMINAL_EMULATOR") {
		t.Errorf("the server's environment has TERMINAL_EMULATOR:\n%s", global)
	}
}

// Inside another tmux ($TMUX set) cld nests: its server is another one. Its client gets an empty
// TMUX, as join's has to (see TestNestsOnADeadPanesPty), with which tmux still takes the terminal,
// a pane of the other tmux, for UTF-8 whatever the locale says. cld looks for its own panes on its
// own servers only, cld-NAME: in a live pane of any other server it nests - the default one, the
// one server cld 0.3.0 and earlier shared, or one named like no session of cld's can be.
func TestNestsInsideAnotherTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	elsewhere := filepath.Join(s.Root, "elsewhere", "default") + ",1,0"
	startCld(t, s, "tmux", map[string]string{"TMUX": elsewhere, "LANG": "C"}, "new")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-main"}) {
		t.Errorf("sessions %q, want [cld-main]", sessions)
	}
	if utf8 := s.MustTmux("cld-main", "list-clients", "-F", "#{client_utf8}"); utf8 != "1" {
		t.Errorf("client_utf8 %q under LANG=C, want 1", utf8)
	}

	for i, server := range []string{"default", "cld", "cld-x.y"} {
		// A pane of that server runs cld on its own pty, with the TMUX tmux sets for it.
		name := "in" + strconv.Itoa(i)
		out := filepath.Join(s.Root, name)
		s.MustTmux(server, append([]string{"-f", "/dev/null", "new-session", "-d", "-s", "outer",
			"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, s.CldArgv("new", "-n", name)...)...)
		sandbox.WaitFor(t, 10*time.Second, "cld in a pane of server "+server+" to attach or return", func() bool {
			_, err := os.Stat(out + ".code")
			return err == nil || slices.Contains(s.Clients(), "cld-"+name)
		})
		if !slices.Contains(s.Clients(), "cld-"+name) {
			stderr, _ := os.ReadFile(out + ".err")
			t.Errorf("cld new in a pane of server %s did not attach: %q", server, stderr)
		}
	}
}

// A dead pane - one cld keeps for a failed claude, say - keeps the name of its closed pty, and the
// system hands the name to the next terminal opened. tmux takes a client with $TMUX set on a pty
// of that name for one inside its own pane, when the pane is on the server it attaches to; cld's
// client, with an empty TMUX, attaches. join, that is: new starts a server of its own, with no
// dead pane. Not parallel: a terminal another test opens could take the name first.
func TestNestsOnADeadPanesPty(t *testing.T) {
	s := sandbox.New(t)
	// Session main, and a dead pane on its server: remain-on-exit keeps the pane, dead, once false
	// has exited.
	s.MustTmux("cld-main", "-f", "/dev/null", "new-session", "-d", "-s", "cld-main", "sleep", "600")
	s.MustTmux("cld-main", "set", "-g", "remain-on-exit", "on", ";", "new-session", "-d", "-s", "dead", "false")
	sandbox.WaitFor(t, 10*time.Second, "the pane to die", func() bool {
		return s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_dead}") == "1"
	})
	dead := s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_tty}")

	// A pane of another tmux: the terminal writes down its pty and runs join.
	// printf ends the line: uutils' tty (0.8.0) prints the name without a newline.
	env := map[string]string{"TMUX": filepath.Join(s.Root, "elsewhere", "default") + ",1,0"}
	for name, value := range s.Env {
		env[name] = value
	}
	ttyFile := filepath.Join(s.Root, "tty")
	term := terminal.New(t, "tmux", s)
	term.Start(append([]string{"sh", "-c", `tty=$(tty) && printf '%s\n' "$tty" >"$0" && exec "$@" join`, ttyFile}, s.CldArgv()...), env, s.Work)
	var tty []byte
	sandbox.WaitFor(t, 10*time.Second, "the terminal's pty", func() bool {
		tty, _ = os.ReadFile(ttyFile)
		return strings.HasSuffix(string(tty), "\n")
	})
	if got := strings.TrimSuffix(string(tty), "\n"); got != dead {
		t.Skipf("the terminal got %s rather than the dead pane's %s", got, dead)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld join to attach", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-main"}) || !term.Running()
	})
	if !term.Running() {
		t.Fatalf("cld join on %s failed: %q", dead, term.Output())
	}
}

// In a live pane of one of cld's servers - claude's external editor, say - a session attached
// would show inside a session of cld's, itself or another, both taking C-q: new, resume and join
// refuse, saying how to get out, and the terminals attached before stay. cld finds the server
// through the socket the pane's TMUX names, also where TMUX_TMPDIR has changed since. new and
// resume check claude's version before any tmux command but tmux -V, this check's list-panes
// included: a claude too old is what they report there (TestOnlyNewAndResumeRunClaude has no
// terminal, so cld makes no such check there).
func TestRefusesToNestInItsOwnPane(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	nested := "cld: this terminal is a pane of the tmux server of session 'a'; detach with C-q d first\n"
	moved := filepath.Join(s.Root, "moved")
	if err := os.Mkdir(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, test := range []struct {
		// env is what the pane runs cld with besides the server's environment.
		env  []string
		args []string
		want string
	}{
		{nil, []string{"join", "-n", "a"}, nested},
		{nil, []string{"join", "-n", "b"}, nested},
		{nil, []string{"new", "-n", "c"}, nested},
		{nil, []string{"resume", "-n", "c"}, nested},
		// tmux -L cld-a would look for the socket in the directory TMUX_TMPDIR names now.
		{[]string{"TMUX_TMPDIR=" + moved}, []string{"join", "-n", "a"}, nested},
		{[]string{"CLD_FAKE_CLAUDE_VERSION=2.1.231 (Claude Code)"}, []string{"new", "-n", "d"},
			"cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n"},
		{[]string{"CLD_FAKE_CLAUDE_VERSION=2.1.231 (Claude Code)"}, []string{"resume", "-n", "d"},
			"cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n"},
	} {
		// A pane on session a's server runs cld on its own pty, with the TMUX tmux sets for it.
		out := filepath.Join(s.Root, strconv.Itoa(i))
		argv := append(append([]string{"env"}, test.env...), s.CldArgv(test.args...)...)
		s.MustTmux("cld-a", append([]string{"new-session", "-d", "-s", "in-" + strconv.Itoa(i),
			"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, argv...)...)
		command := strings.Join(append(append(slices.Clone(test.env), "cld"), test.args...), " ")
		var code []byte
		sandbox.WaitFor(t, 10*time.Second, command+" to return", func() bool {
			code, _ = os.ReadFile(out + ".code")
			return strings.HasSuffix(string(code), "\n")
		})
		if stderr, _ := os.ReadFile(out + ".err"); string(code) != "1\n" || string(stderr) != test.want {
			t.Errorf("%s: exit %s, stderr %q, want exit 1, stderr %q", command, strings.TrimSpace(string(code)), stderr, test.want)
		}
	}
	if sessions := s.Sessions(); slices.ContainsFunc(sessions, func(session string) bool {
		return strings.HasSuffix(session, "cld-c") || strings.HasSuffix(session, "cld-d")
	}) {
		t.Errorf("sessions %q, want no cld-c or cld-d", sessions)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a", "cld-b"}) {
		t.Errorf("clients attached to %q, want the first ones, to cld-a and cld-b", clients)
	}
}

// The footer of the interactive list, on a detached row and on an attached one, and once Ctrl+X
// has armed the kill.
const (
	listHints         = "↑/↓ to navigate · enter to join · ctrl+x to kill · esc to quit"
	listHintsAttached = "↑/↓ to navigate · enter to join and detach its terminal · ctrl+x to kill · esc to quit"
	killArmed         = "ctrl+x again to kill · esc to keep"
	killArmedAttached = "ctrl+x again to kill and detach its terminal · esc to keep"
)

// On a terminal, cld list shows cld's sessions on the alternate screen, the first one selected:
// the arrows move the selection, Enter joins the selected session as cld join does, and Esc or
// Ctrl+C leave, printing the plain table. Elsewhere it prints the table, as it did before.
func TestListJoin(t *testing.T) {
	t.Parallel()
	// Enter joins the selected session, as cld join -n NAME does. tmux throws away what the
	// terminal has not read yet as its client starts, so the list's last output - the main screen
	// back, the cursor shown and the session's title - comes before a question the terminal
	// answers once it has read it (see busy terminal). tmux gives the terminal back, after a
	// detach, as it got it: as it was before the list.
	t.Run("enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			listHints)
		// The selected row is drawn in inverse video, and only that one; the footer is dim.
		styled := cells(term.Styled())
		for i, want := range map[int]string{
			0: "[]  NAME  STATE     DIRECTORY",
			1: "[inverse=7]> a     detached  " + s.Work,
			2: "[]  b     detached  " + s.Work,
			5: "[intensity=2]" + listHints,
		} {
			if styled[i] != want {
				t.Errorf("line %d is %q, want %q", i+1, styled[i], want)
			}
		}
		term.Keys("Down", "Enter")
		waitScreen(t, term, "probe --name cld-b")
		if title := term.Title(); title != "✳ cld-b" {
			t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
		}
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
		if probes := s.Probes(); len(probes) != 3 {
			t.Errorf("%d claude processes, want 3", len(probes))
		}
		const handOver = "\x1b[?25h\x1b[?1049l" + "\x1b]0;✳ cld-b\a" + "\x1b[c"
		sandbox.WaitFor(t, 10*time.Second, "the main screen, the cursor, the title and the question, in that order", func() bool {
			return bytes.Contains(term.Output(), []byte(handOver))
		})
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// A terminal too busy to answer at once - over a slow link, say - holds the join back until
	// it answers, and its answer does not reach claude as keys: with the terminal frozen as the
	// lookup ends, and thawed a second and a half later, the list becomes tmux only then.
	t.Run("busy terminal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid := list.read(t, "pid")
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Enter")
		lookup.held(t)
		thaw := term.Freeze()
		lookup.release(t)
		time.Sleep(1500 * time.Millisecond)
		if strings.HasPrefix(program(pid), "tmux") {
			t.Error("cld became tmux before the terminal answered")
		}
		thaw()
		waitScreen(t, term, "probe --name cld-b")
		// The terminal answered before it drew claude's screen: a key typed now comes after it.
		term.Keys("z")
		probes["b"].WaitInput(0, "z")
		if answer := attributesAnswer.Find(probes["b"].Input()); answer != nil {
			t.Errorf("claude read the terminal's answer %q as keys", answer)
		}
	})

	// A terminal that does not answer holds the join back five seconds, and no longer.
	t.Run("terminal that does not answer", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		lookup := holdLookup(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid := list.read(t, "pid")
		term.Keys("Enter")
		lookup.held(t)
		thaw := term.Freeze()
		released := time.Now()
		lookup.release(t)
		sandbox.WaitFor(t, 20*time.Second, "cld to become tmux", func() bool { return strings.HasPrefix(program(pid), "tmux") })
		if waited := time.Since(released); waited < 5*time.Second {
			t.Errorf("cld became tmux %v after its lookup, with no answer from the terminal; want five seconds", waited)
		}
		thaw()
		waitScreen(t, term, "probe --name cld-a")
	})

	// An attached row joins too, detaching the other terminal, and the footer says so first.
	t.Run("attached", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		other := startCld(t, s, "tmux", nil, "new", "-n", "b")
		s.WaitProbes(2)
		waitClients(t, s, 1)
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     attached  "+s.Work,
			"",
			listHints)
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> b     attached  "+s.Work,
			"",
			listHintsAttached)
		term.Keys("Enter")
		sandbox.WaitFor(t, 10*time.Second, "the other terminal to be detached", func() bool { return !other.Running() })
		waitScreen(t, term, "probe --name cld-b")
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
	})

	// An exited row joins, and the terminal shows claude's last words and the hint, as cld join
	// does.
	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		probes["b"].Send("exit 1")
		sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-b", "#{pane_dead}") == "1" })
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     exited    "+s.Work,
			"",
			listHints)
		term.Keys("Down", "Enter")
		hint := "claude exited with status 1: C-q d detaches, cld kill -n b ends the session"
		waitScreen(t, term, hint)
		if screen := term.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
			t.Errorf("the hint is not on the message line:\n%s", screen)
		}
	})

	// A row reads exited once claude has, whether a terminal is attached or not; Enter detaches
	// that terminal all the same, and the footer says so first.
	t.Run("exited and attached", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		other := startCld(t, s, "tmux", nil, "new", "-n", "b")
		s.WaitProbes(2)
		waitClients(t, s, 1)
		for _, probe := range s.Probes() {
			if probe.Argv[1] == "cld-b" {
				probe.Send("exit 1")
			}
		}
		sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-b", "#{pane_dead}") == "1" })
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     exited    "+s.Work,
			"",
			listHints)
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> b     exited    "+s.Work,
			"",
			listHintsAttached)
		term.Keys("Enter")
		sandbox.WaitFor(t, 10*time.Second, "the other terminal to be detached", func() bool { return !other.Running() })
		waitScreen(t, term, "claude exited with status 1")
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
	})

	// Esc and Ctrl+C leave, joining nothing: cld exits 0, and the terminal is as it was, with the
	// plain table printed, from the rows the list read. So does Esc typed twice at once, as Alt+Esc
	// comes too, and Esc twice followed at once by a letter: the first Esc stands alone unless a
	// sequence follows the second (see keys that do nothing).
	for _, quit := range []struct {
		name string
		keys func(terminal.Terminal)
	}{
		{"Escape", func(term terminal.Terminal) { term.Keys("Escape") }},
		{"C-c", func(term terminal.Terminal) { term.Keys("C-c") }},
		{"M-Escape", func(term terminal.Terminal) { term.Keys("M-Escape") }},
		// In one write, so that the letter comes within the wait for a lone Esc.
		{"Escape Escape j", func(term terminal.Terminal) { term.Paste("\x1b\x1bj") }},
	} {
		t.Run("quit with "+quit.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			probes := detachedSessions(t, s, "a", "b")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, nil)
			waitLines(t, term,
				"  NAME  STATE     DIRECTORY",
				"> a     detached  "+s.Work,
				"  b     detached  "+s.Work,
				"",
				listHints)
			if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
				t.Errorf("modes while the list is open %+v, want the alternate screen and the cursor hidden", modes)
			}
			quit.keys(term)
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			table := "NAME  STATE     DIRECTORY\n" +
				"a     detached  " + s.Work + "\n" +
				"b     detached  " + s.Work + "\n"
			waitLines(t, term, strings.Split(strings.TrimSuffix(table, "\n"), "\n")...)
			afterList(t, term, table)
			list.checkRestored(t, term)
			if clients := s.Clients(); len(clients) != 0 {
				t.Errorf("clients attached to %q, want none", clients)
			}
			for name, probe := range probes {
				if !probe.Alive() {
					t.Errorf("claude %s exited", name)
				}
			}
		})
	}

	// The selection stops at the first and the last row, and other keys do nothing: letters, an
	// arrow with Shift, and keys with Alt, which terminals send as Esc and the key - Alt+j, and
	// Alt+Up as the terminals that send any key with Alt that way send it (ESC ESC [ A). Each
	// step ends on a row that a key taken for another would not have left selected.
	t.Run("keys that do nothing", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		selected := func(name string) {
			t.Helper()
			lines := []string{"  NAME  STATE     DIRECTORY"}
			for _, row := range []string{"a", "b", "c"} {
				marker := " "
				if row == name {
					marker = ">"
				}
				lines = append(lines, marker+" "+row+"     detached  "+s.Work)
			}
			waitLines(t, term, append(lines, "", listHints)...)
		}
		selected("a")
		term.Keys("Up", "Down")
		selected("b")
		term.Keys("M-j", "S-Up", "M-Up", "k", "q", "Down")
		selected("c")
		term.Keys("Down", "Up")
		selected("b")
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// The list draws once for a key, not once for each of its bytes: an arrow comes as three.
	t.Run("a frame a key", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		term.Keys("Down", "Down")
		// Each frame starts at the top left; the output log trails the screen.
		var frames int
		sandbox.WaitFor(t, 10*time.Second, "the frame with c selected in the output log", func() bool {
			output := term.Output()
			frames = bytes.Count(output, []byte("\x1b[1;1H"))
			return bytes.Contains(output, []byte("\x1b[7m> c "))
		})
		if frames != 3 {
			t.Errorf("%d frames for the list and two arrows, want 3", frames)
		}
	})

	// The list reads only the keys it takes: what comes with Ctrl+C, in the same write, stays
	// with the terminal for the program that reads it next.
	t.Run("keys after leaving", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		// Out of raw mode, the terminal holds the keys for a line; dd takes them as they are.
		list := startList(t, s, term, `"$@"; echo $? >"$0.code"; stty raw; dd bs=64 count=1 of="$0.typed" 2>/dev/null`, nil)
		waitScreen(t, term, listHints)
		term.Paste("\x03typed")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		var typed []byte
		sandbox.WaitFor(t, 10*time.Second, "the keys after Ctrl+C to reach dd", func() bool {
			typed, _ = os.ReadFile(string(list) + ".typed")
			return len(typed) > 0
		})
		if string(typed) != "typed" {
			t.Errorf("dd read %q, want %q", typed, "typed")
		}
	})

	// SIGTERM, SIGHUP, SIGINT and SIGQUIT end the list as they end cld, with 128 and the signal's
	// number, but only once the terminal is as it was.
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT} {
		t.Run("signal "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, pidScript, nil)
			waitScreen(t, term, listHints)
			pid, err := strconv.Atoi(list.read(t, "pid"))
			if err != nil {
				t.Fatal(err)
			}
			if err := syscall.Kill(pid, sig); err != nil {
				t.Fatal(err)
			}
			if code, want := list.code(t), strconv.Itoa(128+int(sig)); code != want {
				t.Errorf("exit %s, want %s", code, want)
			}
			list.checkRestored(t, term)
		})
	}

	// A signal cld was started with ignored stays ignored, as under nohup: SIGHUP leaves the list
	// open.
	t.Run("ignored signal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, "trap '' HUP; "+pidScript, nil)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// SIGTSTP - from outside: Ctrl+Z is a key in raw mode - stops cld with the terminal as it was,
	// and once the shell has cld go on (fg), the list takes the terminal again and draws it all.
	// SIGSTOP leaves the list on the screen, where the shell writes over it, and the terminal in
	// raw mode, which the shell may put back to its own (bash does); the list takes the terminal
	// again all the same.
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGSTOP} {
		t.Run("stopped with "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a", "b")
			term := terminal.New(t, "tmux", s)
			list := startJob(t, s, term)
			rows := func(selected string) []string {
				lines := []string{"  NAME  STATE     DIRECTORY"}
				for _, row := range []string{"a", "b"} {
					marker := " "
					if row == selected {
						marker = ">"
					}
					lines = append(lines, marker+" "+row+"     detached  "+s.Work)
				}
				return append(lines, "", listHints)
			}
			waitLines(t, term, rows("a")...)
			pid, err := strconv.Atoi(list.read(t, "pid"))
			if err != nil {
				t.Fatal(err)
			}
			if err := syscall.Kill(pid, sig); err != nil {
				t.Fatal(err)
			}
			// cld stops itself with SIGSTOP for SIGTSTP (see pause in internal/picker).
			if status, want := list.read(t, "stopped"), strconv.Itoa(128+int(syscall.SIGSTOP)); status != want {
				t.Errorf("the shell reports cld stopped with status %s, want %s", status, want)
			}
			if sig == syscall.SIGTSTP {
				if before, during := list.read(t, "before"), list.read(t, "during"); before != during {
					t.Errorf("stty -g while cld is stopped\n%s\nwant as before\n%s", during, before)
				}
				waitScreen(t, term, "the shell's line")
				sandbox.WaitFor(t, 10*time.Second, "the main screen and the cursor while cld is stopped", func() bool {
					modes := term.Modes()
					return !modes.AltScreen && modes.Cursor
				})
			}
			s.WriteFile(string(list)+".go", "")
			waitLines(t, term, rows("a")...)
			if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
				t.Errorf("modes once cld goes on %+v, want the alternate screen and the cursor hidden", modes)
			}
			list.checkRaw(t)
			term.Keys("Down")
			waitLines(t, term, rows("b")...)
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			afterList(t, term, "NAME  STATE     DIRECTORY\n"+"a     detached  "+s.Work+"\n"+"b     detached  "+s.Work+"\n")
			list.checkRestored(t, term)
		})
	}

	// Where no shell with job control would have cld go on - its process group is its session
	// leader's, an orphaned one - SIGTSTP stops nothing, as without the list: the list puts the
	// terminal back, takes it again and goes on.
	t.Run("SIGTSTP without job control", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, nil)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, syscall.SIGTSTP); err != nil {
			t.Fatal(err)
		}
		const backAndAgain = "\x1b[?25h\x1b[?1049l" + "\x1b[?1049h\x1b[?25l"
		sandbox.WaitFor(t, 10*time.Second, "the terminal put back and taken again", func() bool {
			return bytes.Contains(term.Output(), []byte(backAndAgain))
		})
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// A signal that comes while Enter looks the session up ends cld at once, however long the
	// lookup takes - on a server that hangs, say - joining nothing: the lookup's tmux is killed,
	// and the tab keeps its title. A tmux first on the PATH holds the lookup.
	t.Run("signal at enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		lookup := holdLookup(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		term.Keys("Enter")
		held := lookup.held(t)
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code := list.code(t); code != "143" {
			t.Errorf("exit %s, want 143", code)
		}
		if !gone(held) {
			t.Error("the lookup's tmux outlived cld")
		}
		list.checkRestored(t, term)
		if clients := s.Clients(); len(clients) != 0 {
			t.Errorf("clients attached to %q, want none", clients)
		}
		if title := term.Title(); title == "✳ cld-a" {
			t.Errorf("terminal title %q, for a session not joined", title)
		}
	})

	// Esc and Ctrl+C leave while Enter looks the session up too, printing the table: the lookup's
	// tmux is killed.
	for _, key := range []string{"Escape", "C-c"} {
		t.Run("quit at enter with "+key, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			lookup := holdLookup(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, lookup.env)
			waitScreen(t, term, listHints)
			term.Keys("Enter")
			held := lookup.held(t)
			term.Keys(key)
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			if !gone(held) {
				t.Error("the lookup's tmux outlived cld")
			}
			afterList(t, term, "NAME  STATE     DIRECTORY\n"+"a     detached  "+s.Work+"\n")
			list.checkRestored(t, term)
			if clients := s.Clients(); len(clients) != 0 {
				t.Errorf("clients attached to %q, want none", clients)
			}
		})
	}

	// While Enter looks the session up, other keys do nothing: Down leaves the selection where it
	// is, and a second Enter looks nothing up, so the first lookup joins its session once it ends.
	// Each key the list takes draws a frame, which tells the test that the list has taken it.
	t.Run("keys during the lookup", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "a")
		term := startCld(t, s, "tmux", lookup.env, "list")
		waitScreen(t, term, listHints)
		term.Keys("Enter")
		lookup.held(t)
		waitFrames(t, term, 2)
		term.Keys("Down")
		waitFrames(t, term, 3)
		if row := selectedRow(term); row != "a" {
			t.Errorf("row %q selected during the lookup, want a", row)
		}
		term.Keys("Enter")
		waitFrames(t, term, 4)
		lookup.release(t)
		waitScreen(t, term, "probe --name cld-a")
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a"}) {
			t.Errorf("clients attached to %q, want one, to cld-a", clients)
		}
		if count := lookup.begun(t); count != 1 {
			t.Errorf("cld-a looked up %d times, want once: the second Enter looked it up again", count)
		}
	})

	// A session gone when Enter is pressed stays unjoined: the footer says so, and the list,
	// still in raw mode, reads the sessions again and selects the row that took its place.
	t.Run("gone", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			"no session 'b'")
		if clients := s.Clients(); len(clients) != 0 {
			t.Errorf("clients attached to %q, want none", clients)
		}
		list.checkRaw(t)
		term.Keys("Up")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			listHints)
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// The selection follows its session's place rather than its row's number: with the rows above
	// it gone too, it goes to the next row the list showed, which is now the first...
	t.Run("gone with the rows above", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c", "d")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		for _, name := range []string{"a", "b"} {
			if result := s.RunCld(nil, "kill", "-n", name); result.Code != 0 {
				t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
			}
		}
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> c     detached  "+s.Work,
			"  d     detached  "+s.Work,
			"",
			"no session 'b'")
	})

	// ... and with no row after it left, to the one above, although a session made meanwhile now
	// has its row's number.
	t.Run("gone from the last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-n", "c"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		detachedSessions(t, s, "z")
		term.Keys("Down", "Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> b     detached  "+s.Work,
			"  z     detached  "+s.Work,
			"",
			"no session 'c'")
	})

	// A session whose server runs on without it - claude exited, and a tmux session it made keeps
	// the server running - stays unjoined too, as cld join refuses it, and leaves the list.
	t.Run("lingering server", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		probes["b"].Send("tmux new-session -d -s side sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
			return slices.Contains(s.Sessions(), "cld-b/side")
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		probes["b"].Send("exit")
		sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
			return !slices.Contains(s.Sessions(), "cld-b")
		})
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			"session 'b' has ended, but its tmux server still runs")
		if clients := s.Clients(); len(clients) != 0 {
			t.Errorf("clients attached to %q, want none", clients)
		}
	})

	// When the sessions cannot be read again after a failed Enter, the list says why as well, and
	// keeps its rows. A tmux first on the PATH fails to read them once the test says so.
	t.Run("read again fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		broken := filepath.Join(s.Root, "broken")
		env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
			"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
			"esac\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		s.WriteFile(broken, "")
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			"no session 'b' · lost the server")
	})

	// With its last row gone, the list shows that there are no sessions, under its header, and
	// leaving prints nothing.
	t.Run("last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-n", "a"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		// cld kill ends the server, which takes a moment to exit: waiting for it keeps Enter's lookup
		// from reaching it as it goes (see Findings in docs/design.md).
		sandbox.WaitFor(t, 10*time.Second, "a's server to exit", func() bool {
			_, err := s.Tmux("cld-a", "list-sessions")
			return err != nil && strings.Contains(err.Error(), "no server running")
		})
		term.Keys("Enter")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "no sessions", "", "no session 'a'")
		term.Keys("Down")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "no sessions", "", "esc to quit")
		// Enter has nothing to join.
		term.Keys("Enter")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "no sessions", "", "esc to quit")
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "")
		waitLines(t, term)
	})

	// With no sessions, there is nothing to pick: cld list prints nothing and exits 0 at once.
	t.Run("no sessions", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
			t.Errorf("cld opened the alternate screen: %q", output)
		}
		waitLines(t, term)
	})

	// Every line is cut at the terminal's width, counted in cells, so that none wraps: a wide
	// character (日) where a row reaches the edge is left out whole, an é takes one cell, and so
	// do the arrows and dots of the footer. Leaving prints the whole directory.
	t.Run("narrow", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		// The row shows 22 cells of the directory, so it lives outside the sandbox, where its é
		// shows: /tmp/éN, or /private/tmp/éN on macOS. Its first 日 takes the row's cells 40 and 41.
		const prefix = "> a     attached  "
		base := shortDirectory(t, "/tmp/é")
		shown := base + "/" + strings.Repeat("x", 39-len(prefix)-utf8.RuneCountInString(base+"/"))
		dir := shown + "日本日本"
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		startCldIn(t, s, "tmux", dir, nil, "new", "-n", "a")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		detachedSessions(t, s, "b")
		term := terminal.New(t, "tmux", s)
		term.Resize(40, 40)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			prefix+shown,
			cutTo("  b     detached  "+s.Work, 40),
			"",
			footerIn(listHintsAttached, 40))
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     attached  "+shown,
			cutTo("> b     detached  "+s.Work, 40),
			"",
			footerIn(listHints, 40))
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		table := "NAME  STATE     DIRECTORY\n" +
			"a     attached  " + dir + "\n" +
			"b     detached  " + s.Work + "\n"
		afterList(t, term, table)
	})

	// A terminal too short for every row keeps the selected one in view, with the header and the
	// footer.
	t.Run("short", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c", "d")
		term := terminal.New(t, "tmux", s)
		term.Resize(120, 6)
		term.Start(s.CldArgv("list"), s.Env, s.Work)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			listHints)
		term.Keys("Down", "Down", "Down")
		scrolled := []string{
			"  NAME  STATE     DIRECTORY",
			"  b     detached  " + s.Work,
			"  c     detached  " + s.Work,
			"> d     detached  " + s.Work,
			"",
			listHints,
		}
		waitLines(t, term, scrolled...)
		// Back up, the rows scroll back with the selection.
		term.Keys("Up", "Up", "Up")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"  b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			listHints)
		// A taller terminal shows the rows scrolled out above, now that they fit.
		term.Keys("Down", "Down", "Down")
		waitLines(t, term, scrolled...)
		term.Resize(120, 10)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"  b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"> d     detached  "+s.Work,
			"",
			listHints)
	})

	// A resize redraws the list at the new size. It also takes lines away, so that a list still
	// drawing for the old size would push its footer off the screen: a line wider than the
	// terminal does not show as such, since the next line drawn clears what it wrapped onto.
	t.Run("resize", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		long := filepath.Join(s.Work, "a-directory-longer-than-thirty-cells")
		if err := os.Mkdir(long, 0o755); err != nil {
			t.Fatal(err)
		}
		probes["a"].Send("cd " + long)
		sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
			return s.Format("cld-a", "#{pane_current_path}") == long
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+long,
			"  b     detached  "+s.Work,
			"",
			listHints)
		term.Resize(30, 5)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			cutTo("> a     detached  "+long, 30),
			cutTo("  b     detached  "+s.Work, 30),
			"",
			footerIn(listHints, 30))
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			cutTo("  a     detached  "+long, 30),
			cutTo("> b     detached  "+s.Work, 30),
			"",
			footerIn(listHints, 30))
	})

	// A control character in a directory shows as "?", so that none moves the cursor or changes
	// the terminal: here an ESC would clear the screen. tmux 3.7c passes them on as they are to
	// a UTF-8 client - cld's -u, or the sandbox's LANG for s.Format.
	t.Run("control characters", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a")
		dir := filepath.Join(s.Work, "e\x01f\x1b[2Jg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		probes["a"].Send("cd " + dir)
		var reported string
		sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
			reported = s.Format("cld-a", "#{pane_current_path}")
			return reported != s.Work
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+strings.NewReplacer("\x01", "?", "\x1b", "?").Replace(reported),
			"",
			listHints)
	})

	// The list keeps arrows working after a program left application cursor keys on (CSI ?1h),
	// when the terminal sends ESC O A and ESC O B for them.
	t.Run("application cursor keys", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		term.Start(append([]string{"sh", "-c", `printf '\033[?1h' && exec "$@"`, "sh"}, s.CldArgv("list")...), s.Env, s.Work)
		waitScreen(t, term, listHints)
		term.Keys("Down", "Enter")
		waitScreen(t, term, "probe --name cld-b")
	})

	// In a live pane of one of cld's servers - claude's external editor, say - join would refuse the
	// session picked: the list prints its table there, and exits 0 with no key typed.
	t.Run("own pane", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		startCld(t, s, "tmux", nil, "new", "-n", "a")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		// A pane on session a's server; sh keeps it, with the TMUX tmux sets for it, for capture-pane.
		out := filepath.Join(s.Root, "own")
		s.MustTmux("cld-a", append([]string{"new-session", "-d", "-s", "in-list",
			"sh", "-c", `"$@"; echo $? >"$0.code"; sleep 600`, out}, s.CldArgv("list")...)...)
		var code []byte
		sandbox.WaitFor(t, 10*time.Second, "cld list to return", func() bool {
			code, _ = os.ReadFile(out + ".code")
			return strings.HasSuffix(string(code), "\n")
		})
		if string(code) != "0\n" {
			t.Errorf("exit %s, want 0", strings.TrimSpace(string(code)))
		}
		want := "NAME  STATE     DIRECTORY\n" + "a     attached  " + s.Work
		if screen := strings.TrimRight(s.MustTmux("cld-a", "capture-pane", "-p", "-t", "=in-list:"), "\n"); screen != want {
			t.Errorf("the pane shows\n%s\nwant\n%s", screen, want)
		}
	})

	// Output that goes to a pipe, input that is not the terminal, a terminal that cannot move the
	// cursor, and a job in the background get the plain table, with no key typed.
	t.Run("not a terminal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		table := []string{
			"NAME  STATE     DIRECTORY",
			"a     detached  " + s.Work,
			"b     detached  " + s.Work,
		}
		for _, test := range []struct {
			name, script string
			extra        map[string]string
		}{
			{"a pipe", `{ "$@"; echo $? >"$0.code"; } | cat`, nil},
			{"stdin from /dev/null", `"$@" </dev/null; echo $? >"$0.code"`, nil},
			{"TERM=dumb", `"$@"; echo $? >"$0.code"`, map[string]string{"TERM": "dumb"}},
			{"TERM unset", `env -u TERM "$@"; echo $? >"$0.code"`, nil},
			// It would stop (SIGTTOU) as it set the terminal up. Job control puts it in a process
			// group of its own, and goes off again before it ends: bash 3.2, macOS's sh, reports a
			// job's end on the terminal while job control is on, in a script too.
			{"a background job", `set -m; "$@" & set +m; wait $!; echo $? >"$0.code"`, nil},
		} {
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, test.script, test.extra)
			if code := list.code(t); code != "0" {
				t.Errorf("%s: exit %s, want 0", test.name, code)
			}
			waitLines(t, term, table...)
			if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
				t.Errorf("%s: cld opened the alternate screen: %q", test.name, output)
			}
		}
	})
}

// In the interactive list, Ctrl+X arms the kill of the selected session and a second Ctrl+X
// within two seconds kills it, as cld kill -n NAME does; Esc keeps it. The list then reads the
// sessions again and stays open, the selection on the row that took the killed row's place.
func TestListKill(t *testing.T) {
	t.Parallel()
	// A terminal attached to the session is detached and left clean, as by cld kill, and the
	// footer says so first; the other sessions carry on.
	t.Run("kill", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "c")
		other := startCld(t, s, "tmux", nil, "new", "-n", "b")
		waitClients(t, s, 1)
		for _, probe := range s.WaitProbes(3) {
			if probe.Argv[1] == "cld-b" {
				probes["b"] = probe
			}
		}
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		rows := []string{
			"  NAME  STATE     DIRECTORY",
			"  a     detached  " + s.Work,
			"> b     attached  " + s.Work,
			"  c     detached  " + s.Work,
			"",
		}
		waitScreen(t, term, listHints)
		term.Keys("Down")
		waitLines(t, term, append(rows, listHintsAttached)...)
		armThen(t, term, func() {
			waitLines(t, term, append(rows, killArmedAttached)...)
			if footer := cells(term.Styled())[5]; footer != "[intensity=2]"+killArmedAttached {
				t.Errorf("the footer is %q, want it dim", footer)
			}
			if !probes["b"].Alive() || !other.Running() {
				t.Error("the first Ctrl+X killed b")
			}
		}, "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			listHints)
		sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool { return !probes["b"].Alive() })
		sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool { return !other.Running() })
		if modes := other.Modes(); modes.AltScreen || modes.Mouse {
			t.Errorf("modes of b's terminal after the kill %+v, want none", modes)
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
		}
		for _, name := range []string{"a", "c"} {
			if !probes[name].Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
		list.checkRaw(t)
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     DIRECTORY\n"+"a     detached  "+s.Work+"\n"+"c     detached  "+s.Work+"\n")
		list.checkRestored(t, term)
	})

	// Esc disarms the kill and does nothing more: the list stays open, and the next Ctrl+X arms
	// the kill again.
	t.Run("esc", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		rows := []string{
			"  NAME  STATE     DIRECTORY",
			"> a     detached  " + s.Work,
			"  b     detached  " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		armThen(t, term, func() { waitLines(t, term, append(rows, killArmed)...) }, "Escape")
		waitLines(t, term, append(rows, listHints)...)
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		if list.exited() {
			t.Error("the list closed")
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
	})

	// Two seconds after the first Ctrl+X, the kill disarms; a Ctrl+X after that arms it again.
	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := startCld(t, s, "tmux", nil, "list")
		rows := []string{
			"  NAME  STATE     DIRECTORY",
			"> a     detached  " + s.Work,
			"  b     detached  " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		pressed := time.Now()
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		waitLines(t, term, append(rows, listHints)...)
		// Seeing the footer change takes a while: two seconds more is slack for load.
		if waited := time.Since(pressed); waited < 2*time.Second || waited > 4*time.Second {
			t.Errorf("the kill disarmed %v after Ctrl+X, want two seconds", waited)
		}
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
	})

	// A key that has come by the end of the two seconds counts as typed within them, although cld
	// has not read it yet - held up, stopped here, over their end: Esc keeps the session and the
	// list open, and a second Ctrl+X kills it. Once cld goes on, the key and the end of the wait
	// are there to take together, and cld may take either first: Esc is typed three times.
	t.Run("read late", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, nil)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		rows := []string{
			"  NAME  STATE     DIRECTORY",
			"> a     detached  " + s.Work,
			"  b     detached  " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		// readLate arms the kill, stops cld, types key and has cld go on once the two seconds are
		// over and key is there to read. When cld stopped too late to be sure that the two seconds
		// were not over by then, it types nothing, and reports false once the kill has disarmed;
		// the third time, the test is skipped.
		late := 0
		readLate := func(key string) bool {
			t.Helper()
			pressed := time.Now()
			term.Keys("C-x")
			waitLines(t, term, append(rows, killArmed)...)
			armed := time.Now()
			if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
				t.Fatal(err)
			}
			sandbox.WaitFor(t, 10*time.Second, "cld to stop", func() bool { return isStopped(pid) })
			inTime := time.Since(pressed) < 2*time.Second
			if inTime {
				term.Keys(key)
				sandbox.WaitFor(t, 10*time.Second, "the key to reach the terminal", func() bool { return list.unread(t) })
				time.Sleep(time.Until(armed.Add(2200 * time.Millisecond)))
			}
			if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
				t.Fatal(err)
			}
			if !inTime {
				waitLines(t, term, append(rows, listHints)...)
				if late++; late == 3 {
					t.Skip("cld stopped two seconds after Ctrl+X or later three times: too loaded to tell")
				}
			}
			return inTime
		}
		for kept := 0; kept < 3; {
			if !readLate("Escape") {
				continue
			}
			sandbox.WaitFor(t, 10*time.Second, "the list to take Esc", func() bool {
				return list.exited() || strings.Contains(term.Screen(), listHints)
			})
			if list.exited() {
				t.Fatalf("Esc %d of 3, read late, closed the list", kept+1)
			}
			waitLines(t, term, append(rows, listHints)...)
			kept++
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
		for !readLate("C-x") {
		}
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "> b     detached  "+s.Work, "", listHints)
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     DIRECTORY\n"+"b     detached  "+s.Work+"\n")
		if !probes["b"].Alive() {
			t.Error("claude b exited")
		}
	})

	// Any other key disarms the kill, and then does what it does: an arrow moves the selection, a
	// letter does nothing more, and Ctrl+C leaves. A Ctrl+X right after the key, within the two
	// seconds, then arms the kill again rather than kill.
	t.Run("other keys", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		lines := func(selected, footer string) []string {
			lines := []string{"  NAME  STATE     DIRECTORY"}
			for _, row := range []string{"a", "b"} {
				marker := " "
				if row == selected {
					marker = ">"
				}
				lines = append(lines, marker+" "+row+"     detached  "+s.Work)
			}
			return append(lines, "", footer)
		}
		waitLines(t, term, lines("a", listHints)...)
		term.Keys("C-x")
		waitLines(t, term, lines("a", killArmed)...)
		term.Keys("Down", "C-x")
		waitLines(t, term, lines("b", killArmed)...)
		term.Keys("k", "C-x")
		waitLines(t, term, lines("b", killArmed)...)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
			t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
		}
		term.Keys("C-c")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     DIRECTORY\n"+"a     detached  "+s.Work+"\n"+"b     detached  "+s.Work+"\n")
		for name, probe := range probes {
			if !probe.Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// Enter, once Ctrl+X has armed the kill, disarms it and joins the selected session, which the
	// kill leaves alone.
	t.Run("enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "Enter")
		waitScreen(t, term, "probe --name cld-b")
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
			t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
		}
		for name, probe := range probes {
			if !probe.Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// A key held down repeats: the terminal types it again after a delay, and then many times a
	// second. After the Ctrl+X that killed, Ctrl+X does nothing until none has come for a second,
	// so that the repeats of that Ctrl+X, held a little too long, do not arm and kill the session
	// that took the killed one's place, and the next; another key ends the wait at once.
	t.Run("held down", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		header := "  NAME  STATE     DIRECTORY"
		row := func(marker, name string) string { return marker + " " + name + "     detached  " + s.Work }
		waitScreen(t, term, listHints)
		term.Keys("C-x")
		waitScreen(t, term, killArmed)
		// The second Ctrl+X, held down: it repeats after half a second, a common delay, then 20
		// times a second for a second, past the second after the kill. Two that reach cld a second
		// apart are two presses rather than a key held down, and end the wait. The terminal times
		// the keys, but the time it took beyond their waits may have come between any two: where
		// that may have made a second, the case cannot tell, and is skipped.
		holding := time.Now()
		term.Hold("C-x", 500*time.Millisecond, 50*time.Millisecond, 20)
		if over := time.Since(holding) - 500*time.Millisecond - 20*50*time.Millisecond; over >= 400*time.Millisecond {
			t.Skipf("typing Ctrl+X held down took %v beyond its waits: two may have reached cld a second apart", over.Round(time.Millisecond))
		}
		// Down, after the repeats, moves the selection from b, which took a's place, to c, and ends
		// the wait: a Ctrl+X right after it arms the kill, and a second one kills c.
		term.Keys("Down")
		armThen(t, term, func() {
			waitLines(t, term, header, row(" ", "b"), row(">", "c"), "", killArmed)
			sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b", "cld-c"}) {
				t.Errorf("sessions %q, want [cld-b cld-c]", sessions)
			}
			for _, name := range []string{"b", "c"} {
				if !probes[name].Alive() {
					t.Errorf("claude %s exited", name)
				}
			}
		}, "C-x")
		waitLines(t, term, header, row(">", "b"), "", listHints)
		// A second without Ctrl+X ends the wait too; the test leaves it half a second more.
		time.Sleep(1500 * time.Millisecond)
		term.Keys("C-x")
		waitLines(t, term, header, row(">", "b"), "", killArmed)
		if !probes["b"].Alive() {
			t.Error("claude b exited")
		}
	})

	// Killing the last session leaves the list with no sessions, as when its last row has gone
	// elsewhere: the header over "no sessions", and leaving prints nothing.
	t.Run("last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "no sessions", "", "esc to quit")
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		// Ctrl+X has nothing to arm, or to kill; a letter first ends the wait after a kill (see
		// held down).
		term.Keys("k", "C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", "no sessions", "", "esc to quit")
		if list.exited() {
			t.Error("the list closed")
		}
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "")
	})

	// An exited session is killed the same way, and the terminal still attached to it is
	// detached.
	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		failed := startCld(t, s, "tmux", nil, "new", "-n", "b")
		waitClients(t, s, 1)
		for _, probe := range s.WaitProbes(2) {
			if probe.Argv[1] == "cld-b" {
				probe.Send("exit 1")
			}
		}
		waitScreen(t, failed, "claude exited with status 1: C-q d detaches, cld kill -n b ends the session")
		term := startCld(t, s, "tmux", nil, "list")
		rows := []string{
			"  NAME  STATE     DIRECTORY",
			"  a     detached  " + s.Work,
			"> b     exited    " + s.Work,
			"",
		}
		waitScreen(t, term, listHints)
		term.Keys("Down")
		waitLines(t, term, append(rows, listHintsAttached)...)
		armThen(t, term, func() { waitLines(t, term, append(rows, killArmedAttached)...) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> a     detached  "+s.Work,
			"",
			listHints)
		sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool { return !failed.Running() })
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
			t.Errorf("sessions %q, want [cld-a]", sessions)
		}
	})

	// A session gone by the second Ctrl+X - killed elsewhere here - is reported, and the list reads
	// the sessions again and stays open. The steps outside come before the first Ctrl+X, so that
	// they need not fit in the two seconds.
	t.Run("gone", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			"no session 'b'")
		list.checkRaw(t)
		if list.exited() {
			t.Error("the list closed")
		}
		for _, name := range []string{"a", "c"} {
			if !probes[name].Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// A session ended and made again under the same name is another session, with another
	// claude: the kill ends nothing, as for a session gone, and the list shows the new one.
	t.Run("replaced", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		old := detachedSessions(t, s, "a", "b", "c")["b"]
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		sandbox.WaitFor(t, 10*time.Second, "the first claude b to exit", func() bool { return !old.Alive() })
		detachedSessions(t, s, "b")
		var replaced *sandbox.Probe
		for _, probe := range s.Probes() {
			if probe.Argv[1] == "cld-b" && probe.PID != old.PID {
				replaced = probe
			}
		}
		if replaced == nil {
			t.Fatal("no second claude b")
		}
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> b     detached  "+s.Work,
			"  c     detached  "+s.Work,
			"",
			"no session 'b'")
		if !replaced.Alive() {
			t.Error("the second claude b exited")
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-b cld-c]", sessions)
		}
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// A session whose server runs on without it - claude exited, and the tmux session it made keeps
	// the server running - is refused as cld kill refuses it: the server stays, and the list says
	// why without the command line's advice.
	t.Run("lingering server", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		probes["b"].Send("tmux new-session -d -s side sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
			return slices.Contains(s.Sessions(), "cld-b/side")
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		probes["b"].Send("exit")
		sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
			return !slices.Contains(s.Sessions(), "cld-b")
		})
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			"session 'b' has ended, but its tmux server still runs")
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b/side", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-b/side cld-c]", sessions)
		}
	})

	// A kill-session that fails leaves the session listed, with what tmux said in the footer, or
	// its exit status when it said nothing. A tmux first on the PATH fails it.
	t.Run("kill-session fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		silent := filepath.Join(s.Root, "silent")
		env := wrapTmux(t, s, "case \"$*\" in *kill-session*)\n"+
			"\tif [ -e '"+silent+"' ]; then exit 5; fi\n"+
			"\techo 'tmux: cannot kill' >&2; exit 5 ;;\n"+
			"esac\n")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, env)
		waitScreen(t, term, listHints)
		row := "> a     detached  " + s.Work
		term.Keys("C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", row, "", "tmux: cannot kill")
		s.WriteFile(silent, "")
		// A letter first ends the wait after a kill (see held down).
		term.Keys("k", "C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     DIRECTORY", row, "", "tmux kill-session: exit status 5")
		list.checkRaw(t)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
			t.Errorf("sessions %q, want [cld-a]", sessions)
		}
	})

	// When the sessions cannot be read again after a kill, the list says why and keeps its rows,
	// but for the one it killed. A tmux first on the PATH fails to read them once the test says so.
	t.Run("read again fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		broken := filepath.Join(s.Root, "broken")
		env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
			"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
			"esac\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		s.WriteFile(broken, "")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"  a     detached  "+s.Work,
			"> c     detached  "+s.Work,
			"",
			"lost the server")
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
		}
	})

	// The killed session's server exits after the kill, and a read of the sessions that meets it
	// exiting is told now and then that the server exited unexpectedly (see Findings in
	// docs/design.md): the list passes over that server, as cld list does. A tmux first on the PATH
	// fails the first read of the killed session's server after the kill as tmux does then.
	t.Run("server exiting", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		exiting := filepath.Join(s.Root, "exiting")
		env := wrapTmux(t, s, "case \"$*\" in *'-L cld-a list-sessions '*'#{?pane_dead,exited'*)\n"+
			"\tif [ -e '"+exiting+"' ]; then rm '"+exiting+"'; echo 'server exited unexpectedly' >&2; exit 1; fi ;;\n"+
			"esac\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		s.WriteFile(exiting, "")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> b     detached  "+s.Work,
			"",
			listHints)
		if _, err := os.Stat(exiting); err == nil {
			t.Error("the list did not read the killed session's server after the kill")
		}
	})

	// While the kill runs, keys other than those that leave do nothing: Down leaves the selection
	// where it is, and Ctrl+X arms nothing - Down has ended the wait after the kill (see held
	// down), which would keep Ctrl+X from doing anything too. Each key the list takes draws a
	// frame, which tells the test that the list has taken it. A tmux first on the PATH holds the
	// kill's lookup.
	t.Run("keys during the kill", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "a")
		term := startCld(t, s, "tmux", lookup.env, "list")
		waitScreen(t, term, listHints)
		term.Keys("C-x", "C-x")
		lookup.held(t)
		waitFrames(t, term, 3)
		term.Keys("Down")
		waitFrames(t, term, 4)
		term.Keys("C-x")
		waitFrames(t, term, 5)
		if row := selectedRow(term); row != "a" {
			t.Errorf("row %q selected during the kill, want a", row)
		}
		if strings.Contains(term.Screen(), killArmed) {
			t.Error("Ctrl+X armed the kill while the kill ran")
		}
		lookup.release(t)
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> b     detached  "+s.Work,
			"",
			listHints)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		if !probes["b"].Alive() {
			t.Error("claude b exited")
		}
	})

	// Esc leaves while the kill runs, printing the table: the kill's tmux is killed, and the table
	// shows what it did by then. Held in its lookup or in kill-session, the kill ends nothing;
	// held as it reads the sessions again, it has ended the session, which the table leaves out.
	// A tmux first on the PATH holds the step, once the list has read the sessions it opens with.
	for _, step := range []struct {
		name, pattern string
		ends          bool
	}{
		{"lookup", "*'#{==:#{session_name},cld-a} -F #{session_name} #{W:#{P:#{pane_pid} }}'", false},
		{"kill-session", "*kill-session*", false},
		{"read", "*'#{?pane_dead,exited'*", true},
	} {
		t.Run("quit during the kill's "+step.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			probes := detachedSessions(t, s, "a", "b")
			hold := holdTmux(t, s, "the kill's "+step.name, step.pattern)
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, hold.env)
			waitScreen(t, term, listHints)
			hold.start(t)
			term.Keys("C-x", "C-x")
			held := hold.held(t)
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			if !gone(held) {
				t.Error("the kill's tmux outlived cld")
			}
			table := "NAME  STATE     DIRECTORY\n" + "a     detached  " + s.Work + "\n" + "b     detached  " + s.Work + "\n"
			sessions := []string{"cld-a", "cld-b"}
			if step.ends {
				table, sessions = "NAME  STATE     DIRECTORY\n"+"b     detached  "+s.Work+"\n", []string{"cld-b"}
			}
			afterList(t, term, table)
			list.checkRestored(t, term)
			if got := s.Sessions(); !slices.Equal(got, sessions) {
				t.Errorf("sessions %q, want %q", got, sessions)
			}
			if probes["a"].Alive() == step.ends {
				t.Errorf("claude a alive: %v, want %v", !step.ends, step.ends)
			}
		})
	}

	// The kill goes by the pids of all the session's panes, as tmux reports only the active one's
	// for a session: claude's window split by hand, its other pane selected since the list read
	// the sessions, is still the session on the row.
	t.Run("split window", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		s.MustTmux("cld-a", "split-window", "-d", "-t", "=cld-a:", "sleep", "600")
		s.MustTmux("cld-a", "select-pane", "-t", "=cld-a:.1")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     DIRECTORY",
			"> b     detached  "+s.Work,
			"",
			listHints)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
			t.Errorf("sessions %q, want [cld-b]", sessions)
		}
	})

	// With the kill's hint, the hints on a row with a terminal attached take 86 cells: in 80
	// columns the arrows' hint goes, so that the others, esc to quit last, show whole.
	t.Run("80 columns", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		startCld(t, s, "tmux", nil, "new", "-n", "b")
		waitClients(t, s, 1)
		s.WaitProbes(2)
		term := terminal.New(t, "tmux", s)
		term.Resize(80, 24)
		term.Start(s.CldArgv("list"), s.Env, s.Work)
		rows := func(selected string) []string {
			lines := []string{"  NAME  STATE     DIRECTORY"}
			for _, row := range []string{"a     detached  ", "b     attached  "} {
				marker := " "
				if row[:1] == selected {
					marker = ">"
				}
				lines = append(lines, cutTo(marker+" "+row+s.Work, 80))
			}
			return append(lines, "")
		}
		waitLines(t, term, append(rows("a"), listHints)...)
		term.Keys("Down")
		waitLines(t, term, append(rows("b"), "enter to join and detach its terminal · ctrl+x to kill · esc to quit")...)
		term.Keys("C-x")
		waitLines(t, term, append(rows("b"), killArmedAttached)...)
	})
}

// A session cld resume made is a session like any other in the interactive list: Enter joins it,
// and Ctrl+X twice kills it with its server. After such a kill - by mistake, say - cld resume -n
// NAME brings its conversation back in a new session.
func TestListResumedSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	resumed := startCld(t, s, "tmux", nil, "resume", "-n", "b")
	sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-b", func() bool {
		return slices.Contains(s.Clients(), "cld-b")
	})
	resumed.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !resumed.Running() })
	argv := []string{"--name", "cld-b", "--settings", remoteControl, "--resume", "cld-b"}
	claude := func(other *sandbox.Probe) *sandbox.Probe {
		for _, probe := range s.Probes() {
			if slices.Equal(probe.Argv, argv) && (other == nil || probe.PID != other.PID) {
				return probe
			}
		}
		return nil
	}
	var b *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude b to start", func() bool { b = claude(nil); return b != nil })
	rows := func(selected string) []string {
		lines := []string{"  NAME  STATE     DIRECTORY"}
		for _, name := range []string{"a", "b"} {
			mark := " "
			if name == selected {
				mark = ">"
			}
			lines = append(lines, mark+" "+name+"     detached  "+s.Work)
		}
		return append(lines, "")
	}

	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(rows("a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(rows("b"), listHints)...)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want one, to cld-b", clients)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("join: exit %s, want 0", code)
	}

	term = terminal.New(t, "tmux", s)
	list = startList(t, s, term, listScript, nil)
	waitLines(t, term, append(rows("a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(rows("b"), listHints)...)
	armThen(t, term, func() { waitLines(t, term, append(rows("b"), killArmed)...) }, "C-x")
	waitLines(t, term, "  NAME  STATE     DIRECTORY", "> a     detached  "+s.Work, "", listHints)
	sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool { return !b.Alive() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q after the kill, want [cld-a]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("kill: exit %s, want 0", code)
	}

	startCld(t, s, "tmux", nil, "resume", "-n", "b")
	sandbox.WaitFor(t, 10*time.Second, "claude b to resume again", func() bool { return claude(b) != nil })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// program is the name of the program process pid runs: from /proc where there is one, which is
// quicker than ps. A tmux client may name itself "tmux: client" there (prctl in tmux's
// setproctitle, where the system has no setproctitle of its own).
func program(pid string) string {
	if name, err := os.ReadFile("/proc/" + pid + "/comm"); err == nil {
		return strings.TrimSpace(string(name))
	}
	name, _ := exec.Command("ps", "-o", "comm=", "-p", pid).Output()
	return filepath.Base(strings.TrimSpace(string(name)))
}

// isStopped reports whether process pid is stopped, every thread of it: from /proc where there is
// one, or else from ps.
func isStopped(pid int) bool {
	if tasks, _ := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/task/*/stat"); len(tasks) > 0 {
		for _, task := range tasks {
			stat, err := os.ReadFile(task)
			if err != nil {
				return false
			}
			// The state follows the program's name, which is in parentheses and may hold any.
			if fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:])); len(fields) == 0 || fields[0] != "T" {
				return false
			}
		}
		return true
	}
	state, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return strings.HasPrefix(strings.TrimSpace(string(state)), "T")
}

// gitInit makes dir a git repository.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// waitClients waits until count clients are attached to cld's servers, all told.
func waitClients(t *testing.T, s *sandbox.Sandbox, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d attached client(s)", count), func() bool {
		return len(s.Clients()) == count
	})
}

func waitScreen(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%q on the screen", text), func() bool {
		return strings.Contains(term.Screen(), text)
	})
}

// waitLines waits until the screen shows exactly lines, from the top, and nothing below them;
// spaces at the end of a line do not count.
func waitLines(t *testing.T, term terminal.Terminal, lines ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := strings.Split(term.Screen(), "\n")
		for i := range got {
			got[i] = strings.TrimRight(got[i], " ")
		}
		for len(got) > 0 && got[len(got)-1] == "" {
			got = got[:len(got)-1]
		}
		if slices.Equal(got, lines) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 10s waiting for the screen to show\n%s\nit shows\n%s", strings.Join(lines, "\n"), strings.Join(got, "\n"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// shortDirectory creates a directory named prefix and a number, with the symbolic links in its
// path resolved, and removes it when the test ends.
func shortDirectory(t *testing.T, prefix string) string {
	t.Helper()
	for {
		dir := prefix + strconv.Itoa(rand.IntN(1000))
		err := os.Mkdir(dir, 0o755)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
}

// cutCells is text cut to columns cells, for text whose characters take one cell each but for
// Han ones, which take two.
func cutCells(text string, columns int) string {
	used := 0
	for i, r := range text {
		size := 1
		if unicode.Is(unicode.Han, r) {
			size = 2
		}
		if used+size > columns {
			return text[:i]
		}
		used += size
	}
	return text
}

// cutTo is text cut to columns characters, for text whose characters take one cell each.
func cutTo(text string, columns int) string {
	runes := []rune(text)
	return string(runes[:min(columns, len(runes))])
}

// footerIn is the list's hints as they show in columns cells: without the arrows' hint when the
// hints do not all fit, then cut, without the spaces the screen does not show at the end.
func footerIn(hints string, columns int) string {
	if utf8.RuneCountInString(hints) > columns {
		hints = strings.TrimPrefix(hints, "↑/↓ to navigate · ")
	}
	return strings.TrimRight(cutTo(hints, columns), " ")
}

// detachedSessions creates cld's sessions of the given names, each from a terminal of its own
// that then detaches with C-q d, and returns their claudes by name.
func detachedSessions(t *testing.T, s *sandbox.Sandbox, names ...string) map[string]*sandbox.Probe {
	t.Helper()
	for _, name := range names {
		term := startCld(t, s, "tmux", nil, "new", "-n", name)
		sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-"+name, func() bool {
			return slices.Contains(s.Clients(), "cld-"+name)
		})
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
	probes := map[string]*sandbox.Probe{}
	sandbox.WaitFor(t, 10*time.Second, "the claudes to start", func() bool {
		for _, probe := range s.Probes() {
			probes[strings.TrimPrefix(probe.Argv[1], "cld-")] = probe
		}
		return !slices.ContainsFunc(names, func(name string) bool { return probes[name] == nil })
	})
	return probes
}

// wrapTmux puts a tmux first on the PATH of the environment it returns: an sh script that runs
// script with tmux's arguments as "$@", and then the tmux the tests run.
func wrapTmux(t *testing.T, s *sandbox.Sandbox, script string) map[string]string {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.MkdirTemp(s.Root, "bin.")
	if err != nil {
		t.Fatal(err)
	}
	s.WriteProgram(filepath.Join(bin, "tmux"), "#!/bin/sh\n"+script+"exec '"+tmux+"' \"$@\"\n", 0o755)
	return map[string]string{"PATH": bin + string(os.PathListSeparator) + s.Env["PATH"]}
}

// heldTmux holds the tmux commands of a kind that cld run with env runs, from when the test holds
// them until it releases them or ends: a tmux first on the PATH (see wrapTmux) waits as long as
// the file hold is there. It also counts the commands that begin, held or not, a line each in
// hold.begun.
type heldTmux struct {
	hold, what string
	env        map[string]string
}

// holdLookup holds the lookup of session cld-NAME that Enter and the kill make, from the start. It
// tells the lookup from the read of the sessions, which asks server cld-NAME with the same filter,
// by the one format that follows.
func holdLookup(t *testing.T, s *sandbox.Sandbox, name string) heldTmux {
	t.Helper()
	lookup := holdTmux(t, s, "the lookup of cld-"+name, "*'#{==:#{session_name},cld-"+name+"} -F #{session_name} #{W:#{P:#{pane_pid} }}'")
	lookup.start(t)
	return lookup
}

// holdTmux is ready to hold the tmux commands, described by what, whose arguments match pattern,
// a pattern of sh's case, once the test calls start.
func holdTmux(t *testing.T, s *sandbox.Sandbox, what, pattern string) heldTmux {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "hold.")
	if err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(dir, "hold")
	t.Cleanup(func() { _ = os.Remove(hold) })
	return heldTmux{hold: hold, what: what, env: wrapTmux(t, s, "case \"$*\" in "+pattern+")\n"+
		"\techo $$ >>'"+hold+".begun'\n"+
		"\tif [ -e '"+hold+"' ]; then echo $$ >'"+hold+".held'; fi\n"+
		"\twhile [ -e '"+hold+"' ]; do sleep 0.05; done ;;\n"+
		"esac\n")}
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
		data, _ = os.ReadFile(h.hold + ".held")
		return len(data) > 0 && data[len(data)-1] == '\n'
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
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(data, []byte("\n"))
}

// gone reports whether process pid has ended and been waited for.
func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// listScript runs cld list, as "$@", recording what listRun reads.
const listScript = `tty >"$0.tty"; stty -g >"$0.before"; "$@"; echo $? >"$0.code"; stty -g >"$0.after"`

// pidScript is listScript that also records cld's pid: the inner sh writes its own, which exec
// hands on to cld.
var pidScript = strings.Replace(listScript, `"$@";`, `sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@";`, 1)

// jobScript is pidScript run as a job of a shell with job control (see startJob), which goes on
// with the script when cld stops: it records cld's status then (stopped) and the terminal's mode
// (during), and writes a line of its own. Once the test says so, with the file go, it puts its
// own mode back - as bash does when it takes the terminal back, and dash does not - and cld back
// in the foreground (fg).
const jobScript = `tty >"$0.tty"; stty -g >"$0.before"; ` +
	`sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@"; ` +
	`echo $? >"$0.stopped"; stty -g >"$0.during"; echo "the shell's line"; ` +
	`until [ -e "$0.go" ]; do sleep 0.05; done; stty "$(cat "$0.before")"; ` +
	`fg >/dev/null; echo $? >"$0.code"; stty -g >"$0.after"`

// attributesAnswer matches a terminal's answer to a question for its primary device attributes.
var attributesAnswer = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

// waitFrames waits until the list has drawn count frames: each starts at the top left. The
// output log trails the screen.
func waitFrames(t *testing.T, term terminal.Terminal, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d frames in the output log", count), func() bool {
		return bytes.Count(term.Output(), []byte("\x1b[1;1H")) >= count
	})
}

// armThen presses Ctrl+X, runs armed - which waits for the list to arm the kill, and checks what
// it will meanwhile - and presses then, such as C-x to kill or Escape to keep, which has to reach
// the list within the two seconds of the arm. Under load the checks can take them all: when a
// second has gone by since the first Ctrl+X, a letter disarms the kill, if it is still armed, and
// Ctrl+X arms it again, then follows at once.
func armThen(t *testing.T, term terminal.Terminal, armed func(), then string) {
	t.Helper()
	pressed := time.Now()
	term.Keys("C-x")
	armed()
	if time.Since(pressed) < time.Second {
		term.Keys(then)
		return
	}
	term.Keys("k", "C-x", then)
}

// listRun is cld list run under sh, with the files sh writes: the terminal's name (tty), its
// mode before and after cld (stty -g), and cld's exit status. The script gets the files' common
// path as $0 and cld list as "$@".
type listRun string

// startList runs script in term, in the sandbox's environment with extra variables added. sh
// then sleeps, so that the terminal shows what cld left behind rather than tmux's "Pane is dead".
func startList(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, script string, extra map[string]string) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh"}, script, extra)
}

// startJob runs jobScript in term under an interactive sh (-i), which has job control. macOS's
// sh, bash 3.2 as Apple builds it, hears of a job that stops (waitpid's WUNTRACED) only when it
// is interactive: under set -m in a script, it goes on waiting for a stopped job to end. There,
// bash puts its own mode back as the job stops, before the script reads it (during).
func startJob(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh", "-i"}, jobScript, nil)
}

// startListIn is startList with shell, a command line, in place of sh.
func startListIn(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, shell []string, script string, extra map[string]string) listRun {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "list.")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for name, value := range s.Env {
		env[name] = value
	}
	for name, value := range extra {
		env[name] = value
	}
	run := listRun(filepath.Join(dir, "cld"))
	argv := append(slices.Clone(shell), "-c", script+"; exec sleep 600", string(run))
	term.Start(append(argv, s.CldArgv("list")...), env, s.Work)
	return run
}

// exited reports whether cld has exited.
func (r listRun) exited() bool {
	_, err := os.Stat(string(r) + ".code")
	return err == nil
}

// read waits for the named file to hold a line and returns it.
func (r listRun) read(t *testing.T, name string) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write "+name, func() bool {
		data, _ = os.ReadFile(string(r) + "." + name)
		return len(data) > 0 && data[len(data)-1] == '\n'
	})
	return strings.TrimSuffix(string(data), "\n")
}

// code is cld's exit status, once it has exited.
func (r listRun) code(t *testing.T) string {
	t.Helper()
	return r.read(t, "code")
}

// checkRaw checks that the terminal is in raw mode: no line editing, no echo, and Ctrl+C a key.
func (r listRun) checkRaw(t *testing.T) {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	stty := exec.Command("stty", "-a")
	stty.Stdin = tty
	out, err := stty.Output()
	if err != nil {
		t.Fatalf("stty -a: %v", err)
	}
	for _, flag := range []string{"-icanon", "-echo", "-isig"} {
		if !slices.Contains(strings.Fields(strings.ReplaceAll(string(out), ";", " ")), flag) {
			t.Errorf("the terminal is not in raw mode: no %s in\n%s", flag, out)
		}
	}
}

// unread reports whether the terminal has input that cld has not read, as cld itself tells: in
// raw mode, the terminal is readable once it has a byte.
func (r listRun) unread(t *testing.T) bool {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	fd := int(tty.Fd())
	var set unix.FdSet
	set.Set(fd)
	n, err := unix.Select(fd+1, &set, nil, nil, &unix.Timeval{})
	return err == nil && n > 0
}

// readTTY is the terminal's name; uutils' tty (0.8.0) prints it without a newline.
func (r listRun) readTTY(t *testing.T) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write the terminal's name", func() bool {
		data, _ = os.ReadFile(string(r) + ".tty")
		return len(data) > 0
	})
	return string(data)
}

// checkRestored checks that cld left the terminal as it found it: the same mode (stty -g), the
// main screen, no mouse reporting and the cursor visible. The terminal may still be reading
// what cld wrote last when sh has written its exit status.
func (r listRun) checkRestored(t *testing.T, term terminal.Terminal) {
	t.Helper()
	if before, after := r.read(t, "before"), r.read(t, "after"); before != after {
		t.Errorf("stty -g after cld\n%s\nwant as before\n%s", after, before)
	}
	deadline := time.Now().Add(10 * time.Second)
	for modes := term.Modes(); modes.AltScreen || modes.Mouse || !modes.Cursor; modes = term.Modes() {
		if time.Now().After(deadline) {
			t.Errorf("modes after cld %+v, want the main screen, no mouse and the cursor visible", modes)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// afterList waits for cld to leave the alternate screen and print want after it, with the
// terminal's CR LF line ends as LF. The output log trails the screen and cld's exit status: the
// outer terminal writes it as it goes, through a pipe.
func afterList(t *testing.T, term terminal.Terminal, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		output := string(term.Output())
		end := strings.LastIndex(output, "\x1b[?1049l")
		printed := ""
		if end >= 0 {
			if printed = strings.ReplaceAll(output[end+len("\x1b[?1049l"):], "\r\n", "\n"); printed == want {
				return
			}
		}
		if time.Now().After(deadline) {
			if end < 0 {
				t.Fatalf("timed out after 10s: cld never left the alternate screen: %q", output)
			}
			t.Fatalf("timed out after 10s: printed on leaving\n%q\nwant\n%q", printed, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// attribute names the attribute each SGR code sets or clears.
var attribute = map[string]string{
	"1": "intensity", "2": "intensity", "22": "intensity",
	"3": "italic", "23": "italic",
	"4": "underline", "21": "underline", "24": "underline",
	"5": "blink", "6": "blink", "25": "blink",
	"7": "inverse", "27": "inverse",
	"8": "hidden", "28": "hidden",
	"9": "strike", "29": "strike",
	"53": "overline", "55": "overline",
	"38": "fg", "39": "fg", "48": "bg", "49": "bg", "58": "underline-colour", "59": "underline-colour",
}

// clearing lists the SGR codes that turn their attribute off.
var clearing = map[string]bool{
	"22": true, "23": true, "24": true, "4:0": true, "25": true, "27": true, "28": true,
	"29": true, "55": true, "39": true, "49": true, "59": true,
}

// cells turns a Styled screen into lines of runs, "[attribute=code ...]text", whatever the
// SGR sequences that happened to draw them: equal cells give equal lines. Trailing blank cells
// and lines are dropped, so that screens of different sizes compare by what is drawn on them.
func cells(styled string) []string {
	state := map[string]string{} // capture-pane -e carries attributes over line ends
	var lines []string
	for _, row := range strings.Split(styled, "\n") {
		var attrs, texts []string // one per run
		matches := sgr.FindAllStringSubmatch(row, -1)
		for i, text := range sgr.Split(row, -1) {
			if i > 0 {
				apply(state, matches[i-1][1])
			}
			if text == "" {
				continue
			}
			if n := len(attrs); n > 0 && attrs[n-1] == describe(state) {
				texts[n-1] += text
			} else {
				attrs, texts = append(attrs, describe(state)), append(texts, text)
			}
		}
		if n := len(attrs); n > 0 && attrs[n-1] == "" {
			if texts[n-1] = strings.TrimRight(texts[n-1], " "); texts[n-1] == "" {
				attrs, texts = attrs[:n-1], texts[:n-1]
			}
		}
		var line strings.Builder
		for i := range attrs {
			line.WriteString("[" + attrs[i] + "]" + texts[i])
		}
		lines = append(lines, line.String())
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// apply updates the attributes of the cells after an SGR sequence with parameters params.
func apply(state map[string]string, params string) {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		if code == "" || code == "0" {
			clear(state)
			continue
		}
		base, _, _ := strings.Cut(code, ":")
		// A colour in the ; form takes the codes after it: 5;N or 2;R;G;B.
		if (base == "38" || base == "48" || base == "58") && base == code {
			n := 4
			if i+1 < len(codes) && codes[i+1] == "5" {
				n = 2
			}
			code = strings.Join(codes[i:min(i+n+1, len(codes))], ";")
			i += n
		}
		name, known := attribute[base]
		switch {
		case !known:
		case clearing[code]:
			delete(state, name)
		default:
			state[name] = code
		}
	}
}

func describe(state map[string]string) string {
	var pairs []string
	for name, code := range state {
		pairs = append(pairs, name+"="+code)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// underlined reports whether any cell of a screen from cells is underlined.
func underlined(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, "underline=")
	})
}
