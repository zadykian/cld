package tests

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
)

// cld's scripts run by macOS's osascript against a mock Ghostty, testdata/ghostty/mock.js, so that
// their own logic runs where no Ghostty does (decision 59.12). Only the macos job has osascript.

// realOsascript is macOS's osascript, by its path: a sandbox's PATH finds the fake first.
const realOsascript = "/usr/bin/osascript"

// mockedResult is what a script run against the mock Ghostty printed, with the Apple Events it
// sent.
type mockedResult struct {
	Result json.RawMessage `json:"result"`
	Log    []string        `json:"log"`
}

// runMocked runs the script name of internal/ghostty/scripts with input against the mock Ghostty
// of model m, with mock's settings (see mock.js).
func runMocked(t *testing.T, name, input string, m *mac.Model, mock map[string]any) mockedResult {
	t.Helper()
	var text []string
	for _, path := range []string{filepath.Join("testdata", "ghostty", "mock.js"),
		filepath.Join(scriptsDir, name+".js"), filepath.Join("testdata", "ghostty", "mock-end.js")} {
		text = append(text, readFile(t, path))
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var model map[string]any
	if err := json.Unmarshal(data, &model); err != nil {
		t.Fatal(err)
	}
	model["mock"] = mock
	if data, err = json.Marshal(model); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), realOsascript, "-l", "JavaScript", "-e",
		strings.Join(text, "\n"), input, string(data)).Output()
	var result mockedResult
	if err != nil || json.Unmarshal(out, &result) != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			out = append(out, exit.Stderr...)
		}
		t.Fatalf("osascript: %v\n%s", err, out)
	}
	return result
}

// snapshotReads are the Apple Events snapshot.js sends to read windowsModel's Ghostty. Each read
// of a property is one event for every terminal, nested by window and tab. A tab with no focused
// terminal fails the bulk read of focus, which is then read tab by tab.
var snapshotReads = []string{"count windows", "new surface configuration", "get version",
	"get terminals.tty", "get windows.tabs.terminals.id", "get windows.tabs.terminals.name",
	"get windows.tabs.terminals.workingDirectory", "get windows.tabs.terminals.pid",
	"get windows.tabs.terminals.tty", "get windows.tabs.selected",
	"get windows.tabs.focusedTerminal.id", "get windows[0].tabs[0].focusedTerminal.id",
	"get windows[0].tabs[1].focusedTerminal.id", "get windows[1].tabs[0].focusedTerminal.id",
	"get windows.tabs.terminals.id", "get terminals.id"}

// snapshot.js reads Ghostty's windows in bulk, nested by window and tab, and reads no Ghostty that
// does not run.
func TestTabsScriptsMocked(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(realOsascript); err != nil {
		t.Skip("no osascript: the macos job runs it")
	}
	for _, test := range mockedSnapshots() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := runMocked(t, "snapshot", "{}", test.model, test.mock)
			if !sameJSON(t, got.Result, []byte(withIDs(test.want))) {
				t.Errorf("printed %s, want %s", got.Result, withIDs(test.want))
			}
			if test.name == "windows" && strings.Join(got.Log, "\n") !=
				strings.Join(snapshotReads, "\n") {
				t.Errorf("sent %q, want %q", got.Log, snapshotReads)
			}
			if test.name == "not running" && len(got.Log) != 0 {
				t.Errorf("sent %q to a Ghostty that does not run", got.Log)
			}
		})
	}
}

// mockedSnapshot is a case of TestTabsScriptsMocked: a model, the mock's settings, and the
// document snapshot.js prints, @N for the id of terminal N.
type mockedSnapshot struct {
	name  string
	model *mac.Model
	mock  map[string]any
	want  string
}

// mockedSnapshots are TestTabsScriptsMocked's cases for snapshot.js. Between its reads, the
// user moves @6 into @2's window, then splits @2's tab.
func mockedSnapshots() []mockedSnapshot {
	moved := []mac.Window{windowOf(tabOf(mac.TerminalID(2)), tabOf(mac.TerminalID(6)))}
	grown := []mac.Window{windowOf(tabOf(mac.TerminalID(2), mac.TerminalID(4)))}
	several := fakeGhostty()
	several.Apps = 2
	stopped := fakeGhostty()
	stopped.Running = false
	return []mockedSnapshot{
		{"windows", windowsModel(), nil, windowsSnapshot},
		{"changed between reads", windowsModel(), map[string]any{"then": [][]mac.Window{moved}},
			`{"running": true, "apps": [` + ghosttyAppJSON + `], "version": "1.4.0",
			  "terminals": ["@2", "@6", "@1"], "windows": [{"tabs": [
			    {"selected": true, "focused": "@2", "terminals": [{"id": "@2",
			      "name": "◐ cld-app-3", "cwd": "/Users/me", "pid": 4250, "tty": "/dev/ttys016"}]},
			    {"selected": false, "focused": "@6", "terminals": [{"id": "@6", "name": "~",
			      "cwd": "/", "pid": 4500, "tty": "/dev/ttys020"}]}]}]}`},
		{"changed twice", windowsModel(), map[string]any{"then": [][]mac.Window{moved, grown}},
			`{"error": {"step": "changed", "number": 0,
			  "message": "the windows changed while cld read them", "version": "1.4.0"}}`},
		{"several", several, nil, `{"running": true, "apps": [` + ghosttyAppJSON + `, ` +
			ghosttyAppJSON + `], "version": "1.4.0", "terminals": [], "windows": []}`},
		{"not running", stopped, nil, `{"running": false, "apps": []}`},
	}
}
