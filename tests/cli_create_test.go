package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The tmux command with which join creates a session.

// createCase is a join that creates session x.
type createCase struct {
	args []string
	// dir is where in the work tree cld runs, and c what tmux gets with -c there
	dir, c string
	// home is the session's home tmux gets, under the work directory: dir where that is a
	// repository of its own. Where home is empty, the home is the work directory.
	home string
	// after is what claude gets after its settings, as tmux gets it; -w's settings also branch the
	// worktree from HEAD
	after []string
}

var createCases = []createCase{
	{[]string{"join", "-s", "x"}, "", "", "", nil},
	{[]string{"join", "-s", "x", "-w"}, "", "", "", []string{"--worktree", "cld-x"}},
	{[]string{"join", "-s", "x", "--new"}, "", "", "", nil},
	{[]string{"join", "-s", "x", "--resume", "cld-x"}, "", "", "", []string{"--resume", "cld-x"}},
	{[]string{"join", "-s", "x", "--resume", "a b"}, "", "", "", []string{"--resume", "a b"}},
	{[]string{"join", "-s", "x", "--resume", "a;"}, "", "", "", []string{"--resume", `a\;`}},
	{[]string{"join", "-s", "x", "--resume", `a\;`}, "", "", "", []string{"--resume", `a\\;`}},
	{[]string{"join", "-s", "x", "--", "--model", "a;", `a\;`, ""}, "", "", "",
		[]string{"--model", `a\;`, `a\\;`, ""}},
	{[]string{"join", "-s", "x", "-w", "--", "go"}, "", "", "",
		[]string{"--worktree", "cld-x", "go"}},
	{[]string{"join", "-s", "x", "--resume", "a", "--", "b;"}, "", "", "",
		[]string{"--resume", "a", `b\;`}},
	{[]string{"join", "-s", "x", "--fork", "--resume", "cld-a-0"}, "", "", "",
		[]string{"--resume", "cld-a-0", "--fork-session"}},
	{[]string{"join", "-s", "x", "--resume", "a;", "--fork"}, "", "", "",
		[]string{"--resume", `a\;`, "--fork-session"}},
	{[]string{"join", "-s", "x", "--fork", "--resume=a", "--", "b;"}, "", "", "",
		[]string{"--resume", "a", "--fork-session", `b\;`}},
	{[]string{"join", "--switched-from", "a-0", "-s", "x"}, "", "", "", nil},
	{[]string{"join", "-s", "x"}, "w;", `w\;`, "", nil},
	{[]string{"join", "-s", "x", "-w"}, "w;", `w\;`, "", []string{"--worktree", "cld-x"}},
	{[]string{"join", "-s", "x", "--resume", "cld-x"}, "w;", `w\;`, "", []string{"--resume", "cld-x"}},
	{[]string{"join", "-s", "x"}, `w\;`, `w\\;`, "", nil},
	{[]string{"join", "-s", "x"}, "C#S", "C##S", "", nil},
	{[]string{"join", "-s", "x", "-w"}, "x#(touch ran)", "x##(touch ran)", "",
		[]string{"--worktree", "cld-x"}},
	{[]string{"join", "-s", "x", "--resume", "cld-x"}, "#{session_name}#;",
		`##{session_name}##\;`, "", []string{"--resume", "cld-x"}},
	{[]string{"join", "-s", "x"}, "#;", `##\;`, `#\;`, nil},
}

// join hands tmux this command, word for word, where it creates a session: claude by its checked
// path, and a "\" before a word's final ";" (decision 16.8). The directory doubles each "#", the
// home does not (decision 37.1). The environment tmux gets is cld's, without the terminal's
// variables and with TMUX empty (decisions 33 and 11.6); see TestClientsTakeUTF8 for -u.
func TestCreateTmuxCommand(t *testing.T) {
	t.Parallel()
	for _, test := range createCases {
		name := strings.Join(test.args, " ")
		if test.dir != "" {
			name += " in " + test.dir
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := createDirectory(t, s, test)
			given := fakeTmuxEnv(s, map[string]string{
				"TERMINAL_EMULATOR":    "JetBrains-JediTerm",
				"__CFBundleIdentifier": "com.jetbrains.goland",
				"CURSOR_TRACE_ID":      "0123456789abcdef",
				"VisualStudioVersion":  "17.0",
				"TMUX":                 filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
				"PS1":                  `\u@\h$ `,
			})
			result := s.RunCldOnTerminalIn(dir, given, test.args...)
			checkTitled(t, result, "x")
			want := createCommand(t, s, test)
			record := s.FakeTmuxRecord()
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given,
				"TERMINAL_EMULATOR", "__CFBundleIdentifier", "CURSOR_TRACE_ID", "VisualStudioVersion"))
			if record.Cwd != dir {
				t.Errorf("tmux runs in %s, want %s", record.Cwd, dir)
			}
		})
	}
}

// createDirectory makes the directory where test's join runs, in s's work tree, a repository. Where
// test has a home, the directory is a repository of its own.
func createDirectory(t *testing.T, s *sandbox.Sandbox, test createCase) string {
	t.Helper()
	gitInit(t, s)
	if test.dir != "" {
		if err := os.Mkdir(filepath.Join(s.Work, test.dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if test.home != "" {
		runGit(t, s, s.Work, "init", "-q", test.dir)
	}
	return filepath.Join(s.Work, test.dir)
}

// serverOptions are the options join sets on the server it starts, as tmux gets them.
var serverOptions = []string{
	"set", "-s", "@cld", "1", ";",
	"set", "-s", "extended-keys", "on", ";",
	"set", "-s", "terminal-features[100]", "xterm*:extkeys:hyperlinks", ";",
	"set", "-s", "terminal-features[101]", "wezterm:hyperlinks", ";",
	"set", "-s", "terminal-features[102]", "alacritty:hyperlinks", ";",
	"set", "-s", "focus-events", "on", ";",
	"set", "-g", "mouse", "on", ";",
	"unbind", "-n", "C-MouseDown1Pane", ";", "unbind", "-n", "M-MouseDown3Pane", ";",
	"set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
	"set", "-g", "history-limit", "50000", ";",
	"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
}

// titlesString is the tab's title join sets on session cld-x (decision 25).
const titlesString = "#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} " +
	"cld-x#{?@cld-worktree, [w],}"

// createCommand is the command with which join, run as test says in s, creates session x. It starts
// the server with its options and keys and the session with claude, then sets claude's pane and
// session, then the marks.
func createCommand(t *testing.T, s *sandbox.Sandbox, test createCase) []string {
	t.Helper()
	want := slices.Concat([]string{"-u", "-L", "cld-x", "-f", "/dev/null"}, serverOptions,
		switchKeys(t, s, sandbox.FakeTmux, "x"),
		[]string{"new-session", "-s", "cld-x", "-n", "x", "-c", filepath.Join(s.Work, test.c)})
	// cld finds the fake tmux, which the hooks then name.
	claude := settings(s, sandbox.FakeTmux, sandbox.RealGit, "cld-x",
		filepath.Join(s.Work, test.dir), slices.Contains(test.args, "-w"))
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	want = slices.Concat(want, []string{probe, "--name", "cld-x", "--settings", claude}, test.after)
	run := strings.TrimSuffix(entryFile(s, "x"), ".json") + ".run"
	want = append(want, ";",
		"set", "-p", "-t", "=cld-x:", "remain-on-exit", "on", ";",
		"set", "-p", "-t", "=cld-x:", "remain-on-exit-format", "", ";",
		"set-hook", "-p", "-t", "=cld-x:", "pane-died", endHook("-s x", run), ";",
		"set", "-p", "-t", "=cld-x:", "allow-passthrough", "all", ";",
		"set", "-t", "=cld-x:", "@cld-tmux", sandbox.FakeTmux, ";",
		"set", "-t", "=cld-x:", "@cld-home", filepath.Join(s.Work, test.home), ";",
		"set", "-t", "=cld-x:", "@cld-busy", busyMarker, ";",
		"set", "-t", "=cld-x:", "set-titles-string", titlesString, ";",
		"set", "-t", "=cld-x:", "set-titles", "on", ";")
	if i := slices.Index(test.args, "--switched-from"); i >= 0 {
		want = append(want, "set", "-t", "=cld-x:", "@cld-last", test.args[i+1], ";")
	}
	return append(want, marksCommand(s, "x")...)
}
