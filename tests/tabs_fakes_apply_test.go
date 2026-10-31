package tests

import (
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The fake osascript's apply: what it types and builds in the model, and the result it prints.

// pace is the pace cld gives a plan.
const pace = `"pace": {"gap": 0.3, "every": 8, "pause": 2}`

// applyTests apply builds, typed in a plan of cld's pace, to fakeGhostty(2, 1), whose shape is
// "*[1] [4] | *[7]". Where failAt is not 0, Ghostty fails to make the failAt-th terminal. Each
// case's result and shape use @N, as the plans do. Terminals made take 10, 13, 16 and on.
var applyTests = []struct {
	name, typed, builds, result, shape string
	failAt                             int
}{
	{"a window", "", `{"kind": "window", "tabs": [
	   {"focused": "b", "tree": {"split": "right",
	     "first": {"leaf": {"key": "a", "cwd": "/a", "line": "ssh ws"}},
	     "second": {"leaf": {"key": "b", "cwd": "/b"}}}},
	   {"selected": true, "focused": "c", "tree": {"leaf": {"key": "c", "cwd": "/c"}}}]}`,
		`{"keys": {"a": "@10", "b": "@13", "c": "@16"}}`, "right(10,[13]) *[16] | *[1] [4] | *[7]", 0},
	{"a window with no tab selected", "", `{"kind": "window", "tabs": [
	   {"tree": {"leaf": {"key": "a"}}}, {"tree": {"leaf": {"key": "b"}}}]}`,
		`{"keys": {"a": "@10", "b": "@13"}}`, "*[10] [13] | *[1] [4] | *[7]", 0},
	{"a tab after the selected one", "", `{"kind": "tab", "anchor": "@4",
	   "tabs": [{"selected": true, "tree": {"leaf": {"key": "t"}}}]}`,
		`{"keys": {"t": "@10"}, "index": 2}`, "[1] *[10] [4] | *[7]", 0},
	{"a tab moved first", "", `{"kind": "tab", "anchor": "@1", "index": 1,
	   "tabs": [{"selected": true, "tree": {"leaf": {"key": "t"}}}]}`,
		`{"keys": {"t": "@10"}, "index": 1}`, "*[10] [1] [4] | *[7]", 0},
	{"a tab's index clamped", "", `{"kind": "tab", "anchor": "@1", "index": 9,
	   "tabs": [{"selected": true, "tree": {"leaf": {"key": "t"}}}]}`,
		`{"keys": {"t": "@10"}, "index": 3}`, "[1] [4] *[10] | *[7]", 0},
	{"a split tab in a window behind", "", `{"kind": "tab", "anchor": "@7", "index": 2,
	   "tabs": [{"selected": true, "focused": "u", "tree": {"split": "down",
	     "first": {"leaf": {"key": "t"}}, "second": {"leaf": {"key": "u"}}}}]}`,
		`{"keys": {"t": "@10", "u": "@13"}, "index": 2}`, "[7] *down(10,[13]) | *[1] [4]", 0},
	{"a tab whose window is gone", "", `{"kind": "tab", "anchor": "@99", "index": 2,
	   "tabs": [{"selected": true, "tree": {"leaf": {"key": "t"}}}]}`,
		`{"keys": {"t": "@10"}, "window": true}`, "*[10] | *[1] [4] | *[7]", 0},
	{"a pane right", "", `{"kind": "pane", "anchor": "@4", "split": "right",
	   "tabs": [{"focused": "p", "tree": {"leaf": {"key": "p"}}}]}`,
		`{"keys": {"p": "@10"}}`, "[1] *right(4,[10]) | *[7]", 0},
	{"two panes down", "", `{"kind": "pane", "anchor": "@1", "split": "down",
	   "tabs": [{"focused": "q", "tree": {"split": "right",
	     "first": {"leaf": {"key": "p"}}, "second": {"leaf": {"key": "q"}}}}]}`,
		`{"keys": {"p": "@10", "q": "@13"}}`, "*down(1,right(10,[13])) [4] | *[7]", 0},
	{"a pane left", "", `{"kind": "pane", "anchor": "@7", "split": "left",
	   "tabs": [{"focused": "p", "tree": {"leaf": {"key": "p"}}}]}`,
		`{"keys": {"p": "@10"}}`, "*right([10],7) | *[1] [4]", 0},
	{"a pane up", "", `{"kind": "pane", "anchor": "@4", "split": "up",
	   "tabs": [{"focused": "p", "tree": {"leaf": {"key": "p"}}}]}`,
		`{"keys": {"p": "@10"}}`, "[1] *down([10],4) | *[7]", 0},
	{"a pane whose anchor is gone", "", `{"kind": "pane", "anchor": "@99", "split": "right",
	   "tabs": [{"tree": {"leaf": {"key": "p"}}}]}`,
		`{"keys": {}, "error": "Can't get object. (-1728)"}`, "*[1] [4] | *[7]", 0},
	{"a tree split left", "", `{"kind": "window", "tabs": [{"tree": {"split": "left",
	   "first": {"leaf": {"key": "l"}}, "second": {"leaf": {"key": "r"}}}}]}`,
		`{"keys": {"l": "@10", "r": "@13"}}`, "*right(13,[10]) | *[1] [4] | *[7]", 0},
	{"builds apply.js fails", "", `{"kind": "grid", "tabs": [{"tree": {"leaf": {"key": "g"}}}]},
	  {"kind": "window", "tabs": [{"tree": {"split": "x",
	    "first": {"leaf": {"key": "l"}}, "second": {"leaf": {"key": "r"}}}}]},
	  {"kind": "tab", "anchor": "@1", "tabs": []},
	  {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a"}}}]}`,
		`{"keys": {}, "error": "BUILDERS[b.kind] is not a function"},
		 {"keys": {"l": "@10"}, "error": "Missing or unknown split direction. (-1715)"},
		 {"keys": {}, "error": "undefined is not an object (evaluating 'tab.tree')"},
		 {"keys": {"a": "@13"}}`, "*[13] | *[10] | *[1] [4] | *[7]", 0},
	{"one of each", `{"pane": "@7", "pid": 50009, "line": "cld join -s x"}`,
		`{"kind": "pane", "anchor": "@4", "split": "right", "tabs": [{"tree": {"leaf": {"key": "p"}}}]},
	  {"kind": "tab", "anchor": "@1", "index": 3, "tabs": [{"tree": {"leaf": {"key": "t"}}}]},
	  {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "w"}}}]}`,
		`{"keys": {"p": "@10"}}, {"keys": {"t": "@13"}, "index": 3}, {"keys": {"w": "@16"}}`,
		"*[16] | [1] right(4,[10]) *[13] | *[7]", 0},
	{"a window that fails on its second tab", `{"pane": "@7", "pid": 50009, "line": "ls"}`,
		`{"kind": "pane", "anchor": "@1", "split": "down",
	   "tabs": [{"focused": "b0l0", "tree": {"leaf": {"key": "b0l0", "line": "ssh ws"}}}]},
	  {"kind": "tab", "anchor": "@1", "index": 2, "tabs": [{"tree": {"leaf": {"key": "b1l0"}}}]},
	  {"kind": "window", "tabs": [{"selected": true, "focused": "b2l1", "tree": {"split": "right",
	      "first": {"leaf": {"key": "b2l0", "line": "ssh ws"}},
	      "second": {"leaf": {"key": "b2l1", "line": "ssh ws"}}}},
	    {"focused": "b2l2", "tree": {"leaf": {"key": "b2l2"}}}]}`,
		`{"keys": {"b0l0": "@10"}}, {"keys": {"b1l0": "@13"}, "index": 2},
		 {"keys": {"b2l0": "@16", "b2l1": "@19"}, "error": "no terminal appeared"}`,
		"*right(16,[19]) | down(1,[10]) *[13] [4] | *[7]", 5},
	{"a split that fails", "", `{"kind": "window", "tabs": [{"tree": {"split": "right",
	   "first": {"leaf": {"key": "a"}}, "second": {"split": "down",
	     "first": {"leaf": {"key": "b"}}, "second": {"leaf": {"key": "c"}}}}}]}`,
		`{"keys": {"a": "@10", "b": "@13"}, "error": "Failed to create split. (-10000)"}`,
		"*right(10,[13]) | *[1] [4] | *[7]", 3},
}

// The apply builds windows, tabs and panes where Ghostty puts them, and types into panes in place.
// Its result names each leaf's terminal by key. A build that fails keeps the keys of what it made,
// which stays in the model. The plan goes to osascript.jsonl.
func TestFakeOsascriptApply(t *testing.T) {
	t.Parallel()
	for _, test := range applyTests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := fakeGhostty(2, 1)
			m.FailAt = test.failAt
			saveModel(t, s, m)
			plan := withIDs(`{` + pace + `, "typed": [` + test.typed + `], "builds": [` +
				test.builds + `]}`)
			result := osascript(t, s, nil, applyScript, plan)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
			}
			typed := "" // a case types only into @7's shell, at its prompt
			if test.typed != "" {
				typed = `{"pane": "@7"}`
			}
			checkPrinted(t, "the result", result.Stdout,
				withIDs(`{"typed": [`+typed+`], "builds": [`+test.result+`]}`))
			if got := shape(loadModel(t, s)); got != test.shape {
				t.Errorf("the model's shape is %s, want %s", got, test.shape)
			}
			checkCalls(t, s, "apply", plan)
		})
	}
	t.Run("the terminals made", testApplyTerminals)
	t.Run("typed", testApplyTyped)
	t.Run("plans apply.js throws on", testApplyThrows)
	t.Run("no pace", testApplyNoPace)
	t.Run("moves dropped", testApplyDropped)
}

// Ghostty drops the model's first dropMoves move_tab actions. A tab whose move it drops once lands
// at its index all the same, as apply.js sends the move again; one dropped twice stays where new
// tab put it.
func testApplyDropped(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		drops         int
		result, shape string
	}{
		{1, `{"keys": {"t": "@10"}, "index": 1}`, "*[10] [1] [4] | *[7]"},
		{2, `{"keys": {"t": "@10"}, "index": 2}`, "[1] *[10] [4] | *[7]"},
	} {
		s := sandbox.New(t)
		m := fakeGhostty(2, 1)
		m.DropMoves = test.drops
		saveModel(t, s, m)
		result := osascript(t, s, nil, applyScript, withIDs(`{`+pace+`, "typed": [], "builds": [
		  {"kind": "tab", "anchor": "@1", "index": 1,
		    "tabs": [{"selected": true, "tree": {"leaf": {"key": "t"}}}]}]}`))
		checkPrinted(t, "the result", result.Stdout,
			withIDs(`{"typed": [], "builds": [`+test.result+`]}`))
		if got := shape(loadModel(t, s)); got != test.shape {
			t.Errorf("%d moves dropped: the model's shape is %s, want %s", test.drops, got,
				test.shape)
		}
	}
}

// A terminal made has its leaf's directory, and its line and a newline as initial input. Its
// login is Ghostty's child, on a tty no device backs, and runs -zsh at its prompt in front.
func testApplyTerminals(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, fakeGhostty(1))
	result := osascript(t, s, nil, applyScript, `{`+pace+`, "typed": [], "builds": [
	  {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a", "cwd": "/a", "line": "ssh ws"}}}]}]}`)
	checkPrinted(t, "the result", result.Stdout,
		withIDs(`{"typed": [], "builds": [{"keys": {"a": "@4"}}]}`))
	m := loadModel(t, s)
	terminal := m.Terminals[mac.TerminalID(4)]
	if terminal == nil || terminal.Cwd != "/a" || terminal.Input != "ssh ws\n" ||
		terminal.TTY != "/dev/ttys904" || terminal.PID != 50006 {
		t.Fatalf("terminal 4 is %+v, want /a, ssh ws, /dev/ttys904 and pid 50006", terminal)
	}
	want := []mac.Process{
		{PID: 50005, PPID: ghosttyPID, PGID: 50005, TTY: "ttys904", Args: []string{"/usr/bin/login",
			"-flp", "me", "/bin/bash", "--noprofile", "--norc", "-c", "exec -l /bin/zsh"}},
		{PID: 50006, PPID: 50005, PGID: 50006, TTY: "ttys904", Args: []string{"-zsh"}},
	}
	got := m.Processes[len(m.Processes)-2:]
	if !slices.EqualFunc(got, want, func(got, want mac.Process) bool {
		return got.PID == want.PID && got.PPID == want.PPID && got.PGID == want.PGID &&
			got.TTY == want.TTY && slices.Equal(got.Args, want.Args) && !got.Start.IsZero()
	}) {
		t.Errorf("the processes made are %+v, want %+v", got, want)
	}
}

// A line is typed in place as apply.js types it, Ctrl+U first and Enter last, where the shell the
// plan names is still in front. A pane that ran another program since is busy, and one that
// closed is gone: neither is typed into.
func testApplyTyped(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, fakeGhostty(2))
	result := osascript(t, s, nil, applyScript, withIDs(`{`+pace+`, "typed": [
	  {"pane": "@1", "pid": 50003, "line": "cld join -s x"},
	  {"pane": "@4", "pid": 4400, "line": "ls"}, {"pane": "@99", "pid": 50003, "line": "ls"}],
	  "builds": []}`))
	checkPrinted(t, "the result", result.Stdout, withIDs(`{"typed": [{"pane": "@1"},
	  {"pane": "@4", "busy": true}, {"pane": "@99", "error": "Can't get object. (-1728)"}],
	  "builds": []}`))
	checkTyped(t, loadModel(t, s), map[int][]string{1: {"^U", "cld join -s x", "\r"}, 4: nil})
}

// checkTyped reports the terminals of m, by number, not typed what typed holds for them.
func checkTyped(t *testing.T, m *mac.Model, typed map[int][]string) {
	t.Helper()
	for n, want := range typed {
		if got := m.Terminals[mac.TerminalID(n)].Typed; !slices.Equal(got, want) {
			t.Errorf("terminal %d was typed %q, want %q", n, got, want)
		}
	}
}

// apply.js throws out of run on a plan whose typed or builds is null or missing, once it knows
// Ghostty runs, after the typings where only builds is. osascript then fails with no result.
func testApplyThrows(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		plan, stderr string
		typed        []string
	}{
		{`{` + pace + `, "typed": null, "builds": []}`, "typed", nil},
		{`{}`, "typed", nil},
		{`null`, "typed", nil},
		{`{` + pace + `, "typed": [{"pane": "@1", "pid": 50003, "line": "ls"}]}`, "builds",
			[]string{"^U", "ls", "\r"}},
	} {
		s := sandbox.New(t)
		saveModel(t, s, fakeGhostty(1))
		checkExit(t, test.plan, osascript(t, s, nil, applyScript, withIDs(test.plan)), 1, "",
			"fake osascript: execution error: the plan's "+test.stderr+" is no list\n")
		checkTyped(t, loadModel(t, s), map[int][]string{1: test.typed})
		saveModel(t, s, &mac.Model{})
		checkPrinted(t, test.plan+" with Ghostty not running",
			osascript(t, s, nil, applyScript, withIDs(test.plan)).Stdout,
			`{"error": {"step": "running", "number": 0, "message": "Ghostty is not running",
			  "version": ""}}`)
	}
}

// With no pace in the plan, apply.js's step throws before each line but the first: the typing or
// the build of each later line fails.
func testApplyNoPace(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, fakeGhostty(2))
	result := osascript(t, s, nil, applyScript, withIDs(`{"typed": [
	    {"pane": "@1", "pid": 50003, "line": "ls"}, {"pane": "@4", "pid": 50006, "line": "ls"}],
	  "builds": [{"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a", "line": "ssh ws"}}}]},
	    {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "b"}}}]}]}`))
	const thrown = `"undefined is not an object (evaluating 'pace.every')"`
	checkPrinted(t, "the result", result.Stdout, withIDs(`{"typed": [{"pane": "@1"},
	  {"pane": "@4", "error": `+thrown+`}],
	  "builds": [{"keys": {}, "error": `+thrown+`}, {"keys": {"b": "@7"}}]}`))
	checkTyped(t, loadModel(t, s), map[int][]string{1: {"^U", "ls", "\r"}, 4: nil})
}
