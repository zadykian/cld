package tests

import (
	"encoding/json"
	"testing"

	"github.com/zadykian/cld/internal/ghostty"
)

// apply.js opens a rebuilt pane in its recorded directory. Where that is gone, it opens the pane
// in the home directory, as Ghostty ignores a directory that does not exist (decision 59.7).
func testMockedCwd(t *testing.T) {
	t.Parallel()
	got := runMocked(t, "apply", withIDs(`{`+pace+`, "typed": [], "builds": [{"kind": "window",
	  "tabs": [{"tree": {"split": "right", "first": {"leaf": {"key": "a", "cwd": "/Users/me/src"}},
	    "second": {"leaf": {"key": "b", "cwd": "/Users/me/gone"}}}}]}]}`),
		fakeGhostty(2, 1), map[string]any{"gone": []string{"/Users/me/gone"}})
	var result ghostty.Result
	if err := json.Unmarshal(got.Result, &result); err != nil || len(result.Builds) != 1 ||
		got.Model == nil {
		t.Fatalf("printed %s, want one build", got.Result)
	}
	keys := result.Builds[0].Keys
	for key, want := range map[string]string{"a": "/Users/me/src", "b": "/Users/me"} {
		if terminal := got.Model.Terminals[keys[key]]; terminal == nil {
			t.Errorf("printed %s, want pane %s made", got.Result, key)
		} else if terminal.Cwd != want {
			t.Errorf("pane %s opened in %q, want %q", key, terminal.Cwd, want)
		}
	}
}
