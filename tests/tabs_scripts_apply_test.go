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
}

// mockedFailures are apply.js's own cases as it fails.
var mockedFailures = []mockedApply{
	{"Ghostty not running", `{"pane": "@1", "pid": 50003, "line": "ls"}`, "",
		func(m *mac.Model) { m.Running = false }, nil, `{"error": {"step": "running",
		  "number": 0, "message": "Ghostty is not running", "version": ""}}`, "*[1] [4] | *[7]",
		nil},
	{"denied", `{"pane": "@1", "pid": 50003, "line": "ls"}`, "", nil,
		map[string]any{"fail": "denied"}, `{"error": {"step": "count", "number": -1743,
		  "message": "Not authorized to send Apple events to Ghostty.", "version": ""}}`,
		"*[1] [4] | *[7]", []string{"get terminals.id"}},
}

// testMockedApply runs apply.js against fakeGhostty(2, 1) as the mock Ghostty.
func testMockedApply(t *testing.T) {
	t.Parallel()
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

// apply.js waits before each line that starts an ssh but the first. It waits the pace's gap, and
// its pause too after every eighth line.
func testMockedPace(t *testing.T) {
	t.Parallel()
	typing := `{"pane": "@1", "pid": 50003, "line": "ls"}`
	got := runMocked(t, "apply", withIDs(`{`+pace+`, "typed": [`+
		strings.Repeat(typing+", ", 8)+typing+`], "builds": []}`), fakeGhostty(2, 1), nil)
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
