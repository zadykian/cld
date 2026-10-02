package tests

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The tmux commands with which join attaches and detach detaches, and the environment tmux gets.

// The paths of VS Code's git helpers, as Cursor's terminal gives them, and a user's own.
const (
	vscodeDist    = "/home/u/.cursor-server/bin/1/extensions/git/dist/"
	vscodeNode    = "/home/u/.cursor-server/bin/1/node"
	vscodeHandle  = "/run/user/1000/vscode-git-1.sock"
	vscodeEditor  = `"` + vscodeDist + `git-editor.sh"`
	vscodeAskpass = vscodeDist + "askpass.sh"
	yourAskpass   = "/usr/lib/ssh/x11-ssh-askpass"
	yourEditor    = "/usr/bin/vim"
)

// vscodeAskpassVariables and vscodeEditorVariables come with VS Code's GIT_ASKPASS and
// GIT_EDITOR.
var (
	vscodeAskpassVariables = map[string]string{
		"VSCODE_GIT_ASKPASS_MAIN":       vscodeDist + "askpass-main.js",
		"VSCODE_GIT_ASKPASS_NODE":       vscodeNode,
		"VSCODE_GIT_ASKPASS_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":         vscodeHandle,
	}
	vscodeEditorVariables = map[string]string{
		"VSCODE_GIT_EDITOR_MAIN":       vscodeDist + "git-editor-main.js",
		"VSCODE_GIT_EDITOR_NODE":       vscodeNode,
		"VSCODE_GIT_EDITOR_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":        vscodeHandle,
	}
)

// vscodeGitCases are the git helpers of a terminal, VS Code's or the user's own.
var vscodeGitCases = []struct {
	name string
	// vscode are VS Code's variables that come with GIT_ASKPASS and GIT_EDITOR, and kept those of
	// the two that reach tmux
	vscode          []map[string]string
	askpass, editor string
	kept            []string
}{
	{"VS Code's", []map[string]string{vscodeAskpassVariables, vscodeEditorVariables},
		vscodeAskpass, vscodeEditor, nil},
	{"VS Code's without its window",
		[]map[string]string{vscodeAskpassVariables, vscodeEditorVariables},
		vscodeDist + "askpass-empty.sh", `"` + vscodeDist + `git-editor-empty.sh"`, nil},
	{"VS Code's askpass and your own editor", []map[string]string{vscodeAskpassVariables},
		vscodeAskpass, yourEditor, []string{"GIT_EDITOR"}},
	{"your own beside VS Code's",
		[]map[string]string{vscodeAskpassVariables, vscodeEditorVariables},
		yourAskpass, yourEditor, []string{"GIT_ASKPASS", "GIT_EDITOR"}},
	{"your own", nil, yourAskpass, yourEditor, []string{"GIT_ASKPASS", "GIT_EDITOR"}},
}

// VS Code's terminals, and its forks', give git an askpass and an editor that ask in its window,
// each a script beside its MAIN variable. So join leaves each helper out whole, with its variables
// and VSCODE_GIT_IPC_HANDLE, and keeps a GIT_ASKPASS or GIT_EDITOR of the user's (decision 33.2).
func TestVSCodeGit(t *testing.T) {
	t.Parallel()
	for _, test := range vscodeGitCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			given := fakeTmuxEnv(s, map[string]string{
				"GIT_ASKPASS": test.askpass,
				"GIT_EDITOR":  test.editor,
			})
			var dropped []string
			for _, vscode := range test.vscode {
				maps.Copy(given, vscode)
				dropped = slices.AppendSeq(dropped, maps.Keys(vscode))
			}
			for _, name := range []string{"GIT_ASKPASS", "GIT_EDITOR"} {
				if !slices.Contains(test.kept, name) {
					dropped = append(dropped, name)
				}
			}
			result := s.RunCldOnTerminal(given, "join", "-s", "x")
			checkTitled(t, result, "x")
			checkEnv(t, s.FakeTmuxRecord().Env, passedOn(s, given, dropped...))
		})
	}
}

// join hands tmux this command, word for word, where the session runs. It attaches with -d for
// --detach-others alone (decision 23.1), and with --switched-from records the session the
// terminal came from (decision 51.4). The environment is cld's with TMUX emptied; the terminal's
// variables stay, as only a server join starts goes without them (decision 33).
func TestJoinTmuxCommand(t *testing.T) {
	t.Parallel()
	for _, command := range []struct {
		args []string
		// attach is attach-session and its options, as tmux gets them, and after what follows
		// attach-session before the hint
		attach, after []string
	}{
		{[]string{"join", "-s", "x"}, []string{"attach-session"}, nil},
		{[]string{"join", "-s", "x", "--detach-others"}, []string{"attach-session", "-d"}, nil},
		{[]string{"join", "--detach-others", "-s", "x"}, []string{"attach-session", "-d"}, nil},
		{[]string{"join", "-s", "x", "--detach-others=false"}, []string{"attach-session"}, nil},
		{[]string{"join", "--switched-from", "a-0", "-s", "x"}, []string{"attach-session"},
			[]string{";", "set", "-t", "=cld-x:", "@cld-last", "a-0"}},
		{[]string{"join", "--switched-from", "x", "-s", "x"}, []string{"attach-session"}, nil},
	} {
		t.Run(strings.Join(command.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			given := fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": "cld-x",
				"TERMINAL_EMULATOR":      "JetBrains-JediTerm",
				"TERM_PROGRAM":           "iTerm.app",
				"LC_TERMINAL":            "iTerm2",
				"TMUX":                   filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
				"PS1":                    `\u@\h$ `,
			})
			result := s.RunCldOnTerminal(given, command.args...)
			checkTitled(t, result, "x")
			record := s.FakeTmuxRecord()
			want := slices.Concat([]string{"-u", "-L", "cld-x"}, command.attach,
				[]string{"-t", "=cld-x"}, command.after,
				[]string{";", "if", "-F", "#{pane_dead}", endHint("-s x")})
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given))
		})
	}
}

// serverMark is 1 on a server that cld started (see TestLeavesAForeignServerAlone).
const serverMark = "#{||:#{@cld},#{==:#{prefix},C-q}}"

// detachCase is a detach command line, in a server's pane or outside, and what tmux gets.
type detachCase struct {
	args []string
	// server is the server of the socket TMUX names; TMUX is unset where empty
	server string
	// session is the session the fake tmux finds, on whatever server cld asks
	session string
	// argv is what tmux gets, SOCKET standing for the socket TMUX names; nil where detach refuses
	// the command line
	argv []string
}

var detachCases = []detachCase{
	{[]string{"detach", "-s", "x"}, "", "cld-x", []string{"-L", "cld-x",
		"if", "-F", "-t", "=cld-x:", "#{session_attached}", "detach-client -s =cld-x"}},
	{[]string{"detach", "-n", "a", "-s", "b"}, "", "cld-a-b", []string{"-L", "cld-a-b",
		"if", "-F", "-t", "=cld-a-b:", "#{session_attached}", "detach-client -s =cld-a-b"}},
	{[]string{"detach"}, "cld-x", "", []string{"-S", "SOCKET",
		"display-message", "-p", serverMark, ";",
		"if", "-F", "#{&&:" + serverMark + ",#{session_attached}}", "detach-client"}},
	{[]string{"detach", "-s", "y"}, "cld-x", "cld-y", []string{"-L", "cld-y",
		"if", "-F", "-t", "=cld-y:", "#{session_attached}", "detach-client -s =cld-y"}},
	{[]string{"detach", "-n", "x"}, "cld-x", "", nil},
	{[]string{"detach"}, "default", "", nil},
	{[]string{"detach"}, "cld", "", nil},
	{[]string{"detach"}, "cld-x.y", "", nil},
}

// detach hands tmux this command, word for word, under an if, and prints nothing (decision 44).
// With -s it runs detach-client -s on the session the lookup finds. Where TMUX names one of cld's
// servers it runs a bare detach-client there, TMUX and TMUX_PANE passed on, asking for the mark
// first. Elsewhere, or with -n alone, it needs -s, and tmux gets nothing.
func TestDetachTmuxCommand(t *testing.T) {
	t.Parallel()
	for _, test := range detachCases {
		name := strings.Join(test.args, " ")
		if test.server != "" {
			name += " in " + test.server
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			given := fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": test.session,
				"TERMINAL_EMULATOR":      "JetBrains-JediTerm",
			})
			tmux := filepath.Join(s.SocketDir(), test.server)
			if test.server != "" {
				given["TMUX"] = tmux + ",123,0"
				given["TMUX_PANE"] = "%0"
			}
			result := s.RunCld(given, test.args...)
			if test.argv == nil {
				checkFailed(t, result, 2, "cld: detach: missing -s SUFFIX (see cld list)\n")
				if fakeTmuxRan(s) {
					t.Error("tmux ran")
				}
				return
			}
			if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0 and no output",
					result.Code, result.Stdout, result.Stderr)
			}
			checkDetached(t, s, test, given, tmux)
		})
	}
}

// checkDetached reports a tmux command other than test's, the socket tmux standing for SOCKET, and
// an environment other than given, its TMUX kept in a server's pane.
func checkDetached(
	t *testing.T, s *sandbox.Sandbox, test detachCase, given map[string]string, tmux string,
) {
	t.Helper()
	record := s.FakeTmuxRecord()
	want := slices.Clone(test.argv)
	if i := slices.Index(want, "SOCKET"); i >= 0 {
		want[i] = tmux
	}
	if !slices.Equal(record.Argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
	}
	env := passedOn(s, given)
	if test.server != "" {
		env["TMUX"] = given["TMUX"]
	}
	checkEnv(t, record.Env, env)
}
