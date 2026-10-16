package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The fakes of the Mac that cld restore tabs runs (#176), driven directly. The probe plays
// osascript, ps and ssh against the model Ghostty of internal/mac, and sh fakes play open and
// ghostty. The helpers the tabs_fakes_* files share are here.

// Stand-ins for cld's scripts: the fake osascript reads a script's first line, its marker, alone.
const (
	snapshotScript = "// cld: snapshot\nfunction run(argv) {}\n"
	applyScript    = "// cld: apply\nfunction run(argv) {}\n"
)

// The model Ghostty's process, as snapshot.js reads it. Its executable is a path no Mac has, so
// that no test runs the user's Ghostty by it.
const (
	ghosttyPID        = 812
	ghosttyLaunched   = 1792141920.25
	ghosttyExecutable = "/nonexistent/Ghostty.app/Contents/MacOS/ghostty"
)

// ghosttyAppJSON is the model Ghostty's app in a snapshot.
var ghosttyAppJSON = fmt.Sprintf(`{"pid": %d, "launched": %s, "executable": %q}`, ghosttyPID,
	strconv.FormatFloat(ghosttyLaunched, 'f', -1, 64), ghosttyExecutable)

// fakeGhostty is a running Ghostty 1.4.0 of windows front to back, window w of tabs[w] tabs, each
// of one pane in /Users/me, the first selected. Its terminals are AddTerminal's, from 1.
func fakeGhostty(tabs ...int) *mac.Model {
	m := &mac.Model{Running: true, PID: ghosttyPID, Launched: ghosttyLaunched,
		Executable: ghosttyExecutable, Version: "1.4.0",
		Processes: []mac.Process{{PID: ghosttyPID, PPID: 1, PGID: ghosttyPID,
			Args: []string{ghosttyExecutable}}}}
	for _, count := range tabs {
		window := mac.Window{}
		for i := range count {
			id := m.AddTerminal("/Users/me", "")
			window.Tabs = append(window.Tabs,
				mac.Tab{Selected: i == 0, Focused: id, Tree: &mac.Node{Pane: id}})
		}
		m.Windows = append(m.Windows, window)
	}
	return m
}

// terminalRef is @N in a test's JSON, for the id of the model's terminal N.
var terminalRef = regexp.MustCompile(`@([0-9]+)`)

// withIDs is text with each @N replaced by the id of the model's terminal N.
func withIDs(text string) string {
	return terminalRef.ReplaceAllStringFunc(text, func(ref string) string {
		n, err := strconv.Atoi(ref[1:])
		if err != nil {
			panic(err) // the expression matched digits alone
		}
		return mac.TerminalID(n)
	})
}

// saveModel saves m as the sandbox's model Mac.
func saveModel(t *testing.T, s *sandbox.Sandbox, m *mac.Model) {
	t.Helper()
	if err := mac.Save(s.ProbeDir, m); err != nil {
		t.Fatal(err)
	}
}

// loadModel is the sandbox's model Mac, as the fakes left it.
func loadModel(t *testing.T, s *sandbox.Sandbox) *mac.Model {
	t.Helper()
	m, err := mac.Load(s.ProbeDir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// shape draws the model's windows front to back, split by " | ", and their tabs. * marks a
// window's selected tab, [N] a tab's focused terminal, and right(A,B) or down(A,B) a split. A
// terminal is drawn by its number, as TerminalID numbers it.
func shape(m *mac.Model) string {
	windows := make([]string, 0, len(m.Windows))
	for _, window := range m.Windows {
		tabs := make([]string, 0, len(window.Tabs))
		for _, tab := range window.Tabs {
			mark := ""
			if tab.Selected {
				mark = "*"
			}
			tabs = append(tabs, mark+drawTree(tab.Tree, tab.Focused))
		}
		windows = append(windows, strings.Join(tabs, " "))
	}
	return strings.Join(windows, " | ")
}

// drawTree draws a tab's tree for shape, focused in brackets.
func drawTree(n *mac.Node, focused string) string {
	switch {
	case n == nil:
		return "nil"
	case n.Pane == "":
		return n.Split + "(" + drawTree(n.First, focused) + "," + drawTree(n.Second, focused) + ")"
	case n.Pane == focused:
		return "[" + drawTree(n, "") + "]"
	}
	return strings.TrimLeft(n.Pane[strings.LastIndexByte(n.Pane, '-')+1:], "0")
}

// fakeCommand is the program path with args, in s's environment plus extra and s's work
// directory, killed once ctx ends.
func fakeCommand(
	ctx context.Context, s *sandbox.Sandbox, extra map[string]string, path string, args ...string,
) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Dir = s.Environ(extra), s.Work
	return cmd
}

// runFake runs the fake name of ProbeBin with args, as fakeCommand does.
func runFake(
	t *testing.T, s *sandbox.Sandbox, extra map[string]string, name string, args ...string,
) sandbox.Result {
	t.Helper()
	return runCommand(t, fakeCommand(t.Context(), s, extra, filepath.Join(sandbox.ProbeBin, name),
		args...))
}

// osascript runs the fake osascript as cld runs a script: with its text and one JSON input.
func osascript(
	t *testing.T, s *sandbox.Sandbox, extra map[string]string, script, input string,
) sandbox.Result {
	t.Helper()
	return runFake(t, s, extra, "osascript", "-l", "JavaScript", "-e", script, input)
}

// sameJSON reports whether got and want are JSON of the same value, whatever their spacing and
// the order of their keys.
func sameJSON(t *testing.T, got, want []byte) bool {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("want %s: %v", want, err)
	}
	return json.Unmarshal(got, &gotValue) == nil && reflect.DeepEqual(gotValue, wantValue)
}

// checkPrinted reports output, what a fake printed, unless that is a line of JSON whose value
// is want's.
func checkPrinted(t *testing.T, what, output, want string) {
	t.Helper()
	line, ended := strings.CutSuffix(output, "\n")
	if !ended || strings.Contains(line, "\n") || !sameJSON(t, []byte(line), []byte(want)) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(want)); err != nil {
			t.Fatal(err)
		}
		t.Errorf("%s: printed %q, want %s and a newline", what, output, compact.String())
	}
}

// checkCalls reports the fake osascript's calls in s where they are not one of script, a
// marker's word, with input.
func checkCalls(t *testing.T, s *sandbox.Sandbox, script, input string) {
	t.Helper()
	calls := s.OsascriptCalls()
	if len(calls) != 1 || calls[0].Script != script ||
		!sameJSON(t, calls[0].Input, []byte(input)) {
		t.Errorf("osascript.jsonl holds %+v, want a call of %s with %s", calls, script, input)
	}
}

// The fake osascript answers snapshot.js's document of the model: its windows front to back, and
// each tab's terminals in its tree's order. Every terminal's id follows, those of no window last.
// It logs each call with its marker, its input and its environment.
func TestFakeOsascriptSnapshot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		model *mac.Model
		want  string
	}{
		{"windows", windowsModel(), windowsSnapshot},
		{"several", &mac.Model{Running: true, Apps: 2, PID: ghosttyPID, Launched: ghosttyLaunched,
			Executable: ghosttyExecutable, Version: "1.4.0"},
			`{"running": true, "apps": [` + ghosttyAppJSON + `, ` + ghosttyAppJSON + `],
			  "version": "1.4.0", "terminals": [], "windows": []}`},
		{"not running", &mac.Model{}, `{"running": false, "apps": []}`},
		{"no model", nil, `{"running": false, "apps": []}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.model != nil {
				saveModel(t, s, test.model)
			}
			result := osascript(t, s, map[string]string{"TZ": "UTC"}, snapshotScript, "{}")
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
			}
			checkPrinted(t, "the snapshot", result.Stdout, withIDs(test.want))
			checkCalls(t, s, "snapshot", "{}")
			if calls := s.OsascriptCalls(); len(calls) == 1 && calls[0].Env["TZ"] != "UTC" {
				t.Errorf("osascript.jsonl logged the environment %v, want TZ=UTC", calls[0].Env)
			}
		})
	}
}

// windowsModel is a Ghostty of two windows and a quick terminal, @1, in none. The front window
// has a tab of one pane, and a tab split in three whose leaves are not in their ids' order. The
// other's one tab has no focused terminal.
func windowsModel() *mac.Model {
	m := fakeGhostty()
	pane := func(n int) *mac.Node { return &mac.Node{Pane: mac.TerminalID(n)} }
	m.Windows = []mac.Window{
		{Tabs: []mac.Tab{
			{Focused: mac.TerminalID(2), Tree: pane(2)},
			{Selected: true, Focused: mac.TerminalID(3), Tree: &mac.Node{Split: "right",
				First:  &mac.Node{Split: "down", First: pane(4), Second: pane(3)},
				Second: pane(5)}},
		}},
		{Tabs: []mac.Tab{{Selected: true, Tree: pane(6)}}},
	}
	m.Hidden = []string{mac.TerminalID(1)}
	m.Terminals = map[string]*mac.Terminal{
		mac.TerminalID(1): {Cwd: "/Users/me", PID: 4100, TTY: "/dev/ttys010"},
		mac.TerminalID(2): {Name: "◐ cld-app-3", Cwd: "/Users/me", PID: 4250, TTY: "/dev/ttys016"},
		mac.TerminalID(3): {Name: "vim", Cwd: "/Users/me/src", PID: 4400, TTY: "/dev/ttys017"},
		mac.TerminalID(4): {Name: "me@mac: ~", Cwd: "/Users/me", PID: 4310, TTY: "/dev/ttys018"},
		mac.TerminalID(5): {Name: "ssh ws"},
		mac.TerminalID(6): {Name: "~", Cwd: "/", PID: 4500, TTY: "/dev/ttys020"},
	}
	return m
}

// windowsSnapshot is windowsModel's snapshot.
var windowsSnapshot = `{"running": true, "apps": [` + ghosttyAppJSON + `], "version": "1.4.0",
  "terminals": ["@2", "@4", "@3", "@5", "@6", "@1"],
  "windows": [
    {"tabs": [
      {"selected": false, "focused": "@2", "terminals": [{"id": "@2", "name": "◐ cld-app-3",
        "cwd": "/Users/me", "pid": 4250, "tty": "/dev/ttys016"}]},
      {"selected": true, "focused": "@3", "terminals": [
        {"id": "@4", "name": "me@mac: ~", "cwd": "/Users/me", "pid": 4310, "tty": "/dev/ttys018"},
        {"id": "@3", "name": "vim", "cwd": "/Users/me/src", "pid": 4400, "tty": "/dev/ttys017"},
        {"id": "@5", "name": "ssh ws", "cwd": "", "pid": 0, "tty": ""}]}]},
    {"tabs": [
      {"selected": true, "focused": "", "terminals": [
        {"id": "@6", "name": "~", "cwd": "/", "pid": 4500, "tty": "/dev/ttys020"}]}]}]}`
