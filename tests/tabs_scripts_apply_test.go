package tests

import (
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
)

// apply.js against the mock Ghostty, in TestTabsScriptsMocked: apply.js's own cases, with their
// results and the Apple Events it sends, in their order.

// mockedApply is a case of apply.js's own against fakeGhostty(2, 1). It has a plan's typed and
// builds, what changes the model first, the mock's settings, the result, the shape, and every
// Apple Event apply.js sends.
type mockedApply struct {
	name, typed, builds string
	model               func(m *mac.Model) // nil for none
	mock                map[string]any
	result, shape       string
	log                 []string
}

// mockedApplies are apply.js's own cases as it works.
var mockedApplies = []mockedApply{
	{"Ctrl+U, the paste, then Enter", `{"pane": "@1", "pid": 50003, "line": "ls"},
	  {"pane": "@4", "pid": 1, "line": "ls"}`, "", nil, nil,
		`{"typed": [{"pane": "@1"}, {"pane": "@4", "busy": true}], "builds": []}`,
		"*[1] [4] | *[7]", []string{"get terminals.id", "get pid of @1", "send key u control to @1",
			"input text to @1", "send key enter to @1", "delay 0.3", "get pid of @4"}},
	{"a window found by its terminal", "", `{"kind": "window", "tabs": [
	   {"focused": "b", "tree": {"split": "right", "first": {"leaf": {"key": "a", "line": "ssh ws"}},
	     "second": {"leaf": {"key": "b"}}}},
	   {"selected": true, "tree": {"leaf": {"key": "c"}}}]}`, nil, nil,
		`{"typed": [], "builds": [{"keys": {"a": "@10", "b": "@13", "c": "@16"}}]}`,
		"right(10,[13]) *[16] | *[1] [4] | *[7]", []string{"get terminals.id", "get terminals.id",
			"new window", "get terminals.id", "split @10 right", "get id of @13",
			"perform action equalize_splits on @10", "focus @13", "get terminals.id",
			"get windows.terminals.id",
			"new tab in windows[0]", "get terminals.id", "focus @16", "get windows.terminals.id",
			"get windows[0].tabs.terminals.id", "select tab windows[0].tabs[1]", "focus @16"}},
	{"a window the user opens meanwhile", "", `{"kind": "window", "tabs": [{"focused": "b",
	   "tree": {"split": "right", "first": {"leaf": {"key": "a"}},
	     "second": {"leaf": {"key": "b"}}}}]}`,
		nil, map[string]any{"opens": []string{"split"}},
		`{"typed": [], "builds": [{"keys": {"a": "@10", "b": "@16"}}]}`,
		"*right(10,[16]) | *[13] | *[1] [4] | *[7]", []string{"get terminals.id",
			"get terminals.id", "new window", "get terminals.id", "split @10 right",
			"get id of @16", "perform action equalize_splits on @10", "focus @16",
			"get windows.terminals.id",
			"get windows[0].tabs.terminals.id", "select tab windows[0].tabs[0]", "focus @16"}},
	{"a split in a split", "", `{"kind": "window", "tabs": [{"focused": "c", "tree": {
	   "split": "right", "first": {"leaf": {"key": "a"}}, "second": {"split": "down",
	     "first": {"leaf": {"key": "b"}}, "second": {"leaf": {"key": "c"}}}}}]}`, nil, nil,
		`{"typed": [], "builds": [{"keys": {"a": "@10", "b": "@13", "c": "@16"}}]}`,
		"*right(10,down(13,[16])) | *[1] [4] | *[7]", []string{"get terminals.id",
			"get terminals.id", "new window", "get terminals.id", "split @10 right",
			"get id of @13", "split @13 down", "get id of @16",
			"perform action equalize_splits on @10", "focus @16", "get windows.terminals.id",
			"get windows[0].tabs.terminals.id", "select tab windows[0].tabs[0]", "focus @16"}},
	{"a window the user opens before a build", `{"pane": "@1", "pid": 50003, "line": "ls"}`,
		`{"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a"}}}]}`, nil,
		map[string]any{"opens": []string{"send key enter"}},
		`{"typed": [{"pane": "@1"}], "builds": [{"keys": {"a": "@13"}}]}`,
		"*[13] | *[10] | *[1] [4] | *[7]", []string{"get terminals.id", "get pid of @1",
			"send key u control to @1", "input text to @1", "send key enter to @1",
			"get terminals.id", "new window", "get terminals.id", "focus @13",
			"get windows.terminals.id",
			"get windows[0].tabs.terminals.id", "select tab windows[0].tabs[0]", "focus @13"}},
}

// mockedFailures are apply.js's own cases as it fails.
var mockedFailures = []mockedApply{
	{"Ghostty not running", `{"pane": "@1", "pid": 50003, "line": "ls"}`, "",
		func(m *mac.Model) { m.Running = false }, nil, `{"error": {"step": "running",
		  "number": 0, "message": "Ghostty is not running", "version": ""}}`, "*[1] [4] | *[7]",
		nil},
	{"a build after one that fails", "", `{"kind": "window", "tabs": [{"tree": {"split": "right",
	   "first": {"leaf": {"key": "a"}}, "second": {"leaf": {"key": "b"}}}}]},
	  {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "c"}}}]}`,
		func(m *mac.Model) { m.FailAt = 2 }, nil,
		`{"typed": [], "builds": [{"keys": {"a": "@10"},
		  "error": "Failed to create split. (-10000)"}, {"keys": {"c": "@13"}}]}`,
		"*[13] | *[10] | *[1] [4] | *[7]", []string{"get terminals.id",
			"get terminals.id", "new window", "get terminals.id", "split @10 right",
			"get terminals.id", "new window", "get terminals.id", "focus @13",
			"get windows.terminals.id",
			"get windows[0].tabs.terminals.id", "select tab windows[0].tabs[0]", "focus @13"}},
	{"Ghostty quits meanwhile", "", `{"kind": "window",
	   "tabs": [{"tree": {"leaf": {"key": "a"}}}]},
	  {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "b"}}}]}`, nil,
		map[string]any{"quits": "new window"}, `{"error": {"step": "running",
		  "number": 0, "message": "Ghostty is not running", "version": ""}}`,
		"*[10] | *[1] [4] | *[7]", []string{"get terminals.id", "get terminals.id", "new window"}},
	{"denied", `{"pane": "@1", "pid": 50003, "line": "ls"}`, "", nil,
		map[string]any{"fail": "denied"}, `{"error": {"step": "count", "number": -1743,
		  "message": "Not authorized to send Apple events to Ghostty.", "version": ""}}`,
		"*[1] [4] | *[7]", []string{"get terminals.id"}},
}

// builtSoFar are the fake osascript's apply cases that apply.js builds so far: windows and their
// splits.
var builtSoFar = []string{"a window", "a window with no tab selected", "a tree split left",
	"a split that fails"}

// testMockedApply runs apply.js against fakeGhostty(2, 1) as the mock Ghostty. The fake
// osascript's apply cases give the same results and shapes. apply.js's own cases follow.
func testMockedApply(t *testing.T) {
	t.Parallel()
	for _, test := range applyTests {
		if !slices.Contains(builtSoFar, test.name) {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			m := fakeGhostty(2, 1)
			m.FailAt = test.failAt
			checkMockedApply(t, test.typed, test.builds, m, nil,
				`{"typed": [], "builds": [`+test.result+`]}`, test.shape)
		})
	}
	for _, test := range slices.Concat(mockedApplies, mockedFailures) {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			m := fakeGhostty(2, 1)
			if test.model != nil {
				test.model(m)
			}
			got := checkMockedApply(t, test.typed, test.builds, m, test.mock, test.result,
				test.shape)
			if sent, want := strings.Join(got.Log, "\n"),
				withIDs(strings.Join(test.log, "\n")); sent != want {
				t.Errorf("sent\n%s\nwant\n%s", sent, want)
			}
		})
	}
	t.Run("the pace", testMockedPace)
}

// checkMockedApply runs apply.js on a plan of cld's pace with typed and builds, against the mock
// Ghostty of m, with mock's settings. It reports a result other than want, or a model whose shape
// is not drawn.
func checkMockedApply(
	t *testing.T, typed, builds string, m *mac.Model, mock map[string]any, want, drawn string,
) mockedResult {
	t.Helper()
	got := runMocked(t, "apply", withIDs(`{`+pace+`, "typed": [`+typed+`], "builds": [`+builds+
		`]}`), m, mock)
	if !sameJSON(t, got.Result, []byte(withIDs(want))) {
		t.Errorf("printed %s, want %s", got.Result, withIDs(want))
	}
	if got.Model == nil || shape(got.Model) != drawn {
		t.Errorf("left the model %+v, want the shape %s", got.Model, drawn)
	}
	return got
}

// apply.js waits before each line that starts an ssh but the first, typed or a new terminal's.
// It waits the pace's gap, and its pause too after every eighth line.
func testMockedPace(t *testing.T) {
	t.Parallel()
	typing := `{"pane": "@1", "pid": 50003, "line": "ls"}`
	got := runMocked(t, "apply", withIDs(`{`+pace+`, "typed": [`+
		strings.Repeat(typing+", ", 7)+typing+`], "builds": [{"kind": "window", "tabs": [
	    {"tree": {"leaf": {"key": "a", "line": "ssh ws"}}}]}]}`), fakeGhostty(2, 1), nil)
	var delays []string
	for _, event := range got.Log {
		if strings.HasPrefix(event, "delay ") {
			delays = append(delays, event)
		}
	}
	want := append(slices.Repeat([]string{"delay 0.3"}, 7), "delay 2.3")
	if !slices.Equal(delays, want) {
		t.Errorf("waited %q, want %q", delays, want)
	}
}
