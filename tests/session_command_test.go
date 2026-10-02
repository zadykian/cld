package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// tmuxCommandLimit is the longest command tmux takes, in bytes (decision 41.5).
const tmuxCommandLimit = 16364

// tmux fails on a command longer than it takes once its server has started. join refuses words
// for claude, or --resume's SESSION, that make it longer, naming its size, before it writes or
// starts anything (decision 41.5). At the limit, counted with the record's hooks, claude starts.
func TestCommandLimit(t *testing.T) {
	t.Parallel()
	const limit = tmuxCommandLimit
	s := sandbox.New(t)
	refused := func(args ...string) int {
		t.Helper()
		result := s.RunCld(nil, args...)
		size, want, err := commandTooLong(result.Stderr, "claude's arguments")
		if err != nil || result.Code != 2 || result.Stdout != "" || size <= limit ||
			result.Stderr != want {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q, want exit 2, stderr %q",
				args[0], result.Code, result.Stdout, result.Stderr, want)
		}
		return size
	}
	long := strings.Repeat("a", 20000)
	fits := long[:len(long)-(refused("join", "-s", "x", "--", "go", long)-limit)]
	if size := refused("join", "-s", "x", "--", "go", fits+"a"); size != limit+1 {
		t.Errorf("a word one byte longer makes the command %d bytes, want %d", size, limit+1)
	}
	refused("join", "-s", "x", "--resume", long)
	refused("join", "-s", "x", "--resume", "a", "--", long)
	if _, err := os.Lstat(filepath.Join(s.SocketDir(), "cld-x")); err == nil {
		t.Errorf("a socket cld-x is left")
	}
	if entry := readEntry(s, "x"); entry != "" {
		t.Errorf("an entry of x is written: %q", entry)
	}
	startCld(t, s, "tmux", nil, "join", "-s", "x", "--", "go", fits)
	probe := s.WaitProbes(1)[0]
	want := []string{"--name", "cld-x", "--settings", sessionSettings(s, "cld-x", s.Work), "go", fits}
	if !slices.Equal(probe.Argv, want) {
		t.Errorf("claude arguments %d words, want %d: the last %.20q, want %.20q",
			len(probe.Argv), len(want), probe.Argv[len(probe.Argv)-1], fits)
	}
}

// commandTooLong reads the size of tmux's command that stderr names, in cld's refusal of what, and
// returns it with the whole refusal cld gives for that size.
func commandTooLong(stderr, what string) (int, string, error) {
	var size int
	_, err := fmt.Sscanf(stderr, "cld: "+what+" make tmux's command %d bytes", &size)
	want := fmt.Sprintf("cld: %s make tmux's command %d bytes, and tmux takes %d at most: "+
		"give claude long text in a file, as with --append-system-prompt-file\n",
		what, size, tmuxCommandLimit)
	return size, want, err
}

// tmux starts the claude join checked, by its path (decision 6). A claude in a relative PATH entry
// before the probe's, "." or an empty one, records that it ran: neither cld nor tmux takes it. A
// script without #! passes the check through /bin/sh, and starts as execvp runs it.
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
			path := entry + string(os.PathListSeparator) + s.Env["PATH"]
			startCld(t, s, "tmux", map[string]string{"PATH": path}, "join")
			probe := s.WaitProbes(1)[0]
			want := []string{"--name", "cld-0", "--settings", sessionSettings(s, "cld-0", s.Work)}
			if !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			ran := filepath.Join(s.ProbeDir, "relative claude ran")
			if args, err := os.ReadFile(ran); err == nil {
				t.Errorf("the claude of the relative entry ran with %q", args)
			}
		})
	}
}

// join makes its session in a directory whose name tmux would change (decision 16.8). tmux ends a
// command at a word ending in ";", and expands -c as a format, where #(...) runs a command. claude
// starts there, with --resume and without, and tmux runs nothing. The session's path is -c as tmux
// expanded it: for a directory that does not exist, tmux starts claude in the home one.
func TestDirectoryTmuxWouldChange(t *testing.T) {
	t.Parallel()
	for _, command := range []struct {
		// after is what claude gets after its settings
		args, after []string
	}{
		{[]string{"join", "-n", "a", "-s", "x"}, nil},
		{[]string{"join", "-n", "a", "-s", "x", "--resume", "cld-a-x"},
			[]string{"--resume", "cld-a-x"}},
	} {
		for _, name := range []string{"w;", "C#S", "x#(touch ran)", "#{session_name};"} {
			args, after := command.args, command.after
			t.Run(strings.Join(args, " ")+" in "+name, func(t *testing.T) {
				t.Parallel()
				checkDirectoryKept(t, name, args, after)
			})
		}
	}
}

// checkDirectoryKept runs cld with args in the directory name, and checks that claude starts
// there, getting after once its settings, and that tmux runs nothing.
func checkDirectoryKept(t *testing.T, name string, args, after []string) {
	t.Helper()
	s := sandbox.New(t)
	dir := filepath.Join(s.Work, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	argv := append([]string{"--name", "cld-a-x", "--settings", sessionSettings(s, "cld-a-x", dir)},
		after...)
	startCldIn(t, s, "tmux", dir, nil, args...)
	probe := s.WaitProbes(1)[0]
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a-x"}) {
		t.Errorf("sessions %q, want [cld-a-x]", sessions)
	}
	if !slices.Equal(probe.Argv, argv) {
		t.Errorf("claude arguments %q, want %q", probe.Argv, argv)
	}
	if probe.Cwd != dir {
		t.Errorf("claude runs in %s, want %s", probe.Cwd, dir)
	}
	if path := s.Format("cld-a-x", "#{session_path}"); path != dir {
		t.Errorf("session path %s, want %s", path, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Errorf("tmux ran touch from the directory's name")
	}
}

// A join in a pane hands its words to the terminal's cld join encoded, a third longer, in the tmux
// command that moves the terminal (decision 51.3). Where that command is longer than tmux takes,
// join refuses the words with status 2, and the terminal stays on its session.
func TestJoinMoveCommandLimit(t *testing.T) {
	t.Parallel()
	const limit = tmuxCommandLimit
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitScreen(t, term, "probe --name cld-b")
	result := s.RunCld(paneOf(t, s, "b"), "join", "-s", "x", "--", strings.Repeat("a", 13000))
	size, want, err := commandTooLong(result.Stderr, "join's words")
	if err != nil || result.Code != 2 || result.Stdout != "" || size <= limit ||
		result.Stderr != want {
		t.Errorf("join -s x in b's pane: exit %d, stdout %q, stderr %q, want exit 2, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) || !term.Running() {
		t.Errorf("clients attached to %q, want the terminal on cld-b", clients)
	}
	if _, err := os.Lstat(filepath.Join(s.SocketDir(), "cld-x")); err == nil {
		t.Errorf("a socket cld-x is left")
	}
}
