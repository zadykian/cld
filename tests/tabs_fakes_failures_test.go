package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The fake osascript's failure modes, the delay file that holds a snapshot back, and the arguments
// it refuses.

// answer is what the fake osascript answers a call with: its status, a line of JSON on stdout as
// a test writes it, with @N for terminal N, and stderr.
type answer struct {
	code           int
	stdout, stderr string
}

// check reports result, named by what, unless the fake answered want.
func (want answer) check(t *testing.T, what string, result sandbox.Result) {
	t.Helper()
	if result.Code != want.code || result.Stderr != want.stderr ||
		(want.stdout == "" && result.Stdout != "") {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d and stderr %q", what,
			result.Code, result.Stdout, result.Stderr, want.code, want.stderr)
	}
	if want.stdout != "" {
		checkPrinted(t, what, result.Stdout, withIDs(want.stdout))
	}
}

// failurePlan types into @1, at its prompt, and builds a window of one pane.
const failurePlan = `{` + pace + `, "typed": [{"pane": "@1", "pid": 50003, "line": "ls"}],
  "builds": [{"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a", "cwd": "/a"}}}]}]}`

// failureAnswers are what the fake osascript answers in each failure mode, to a snapshot and to
// failurePlan's apply, and the shape that apply leaves fakeGhostty(1) in.
var failureAnswers = []struct {
	mode            string
	snapshot, apply answer
	shape           string
}{
	{"denied", answer{0, `{"error": {"step": "count", "number": -1743,
		"message": "Not authorized to send Apple events to Ghostty.", "version": ""}}`, ""},
		answer{0, `{"error": {"step": "count", "number": -1743,
		"message": "Not authorized to send Apple events to Ghostty.", "version": ""}}`, ""},
		"*[1]"},
	{"off", answer{0, `{"error": {"step": "probe", "number": -1743,
		"message": "AppleScript is disabled by the macos-applescript configuration.",
		"version": ""}}`, ""},
		answer{0, `{"typed": [{"pane": "@1", "error": "Can't get object. (-1728)"}],
		"builds": [{"keys": {},
		  "error": "AppleScript is disabled by the macos-applescript configuration. (-1743)"}]}`,
			""},
		"*[1]"},
	{"old", answer{0, `{"error": {"step": "tty", "number": 0,
		"message": "TypeError: G.terminals.tty is not a function", "version": "1.3.1"}}`, ""},
		answer{2, "", "fake osascript: the failure mode old answers no apply: " +
			"cld runs none on Ghostty 1.3.1\n"},
		"*[1]"},
	{"changed", answer{0, `{"error": {"step": "changed", "number": 0,
		"message": "the windows changed while cld read them", "version": "1.4.0"}}`, ""},
		answer{0, `{"typed": [{"pane": "@1"}], "builds": [{"keys": {"a": "@4"}}]}`, ""},
		"*[4] | *[1]"},
	{"gone", answer{0, `{"error": {"step": "read", "number": -600,
		"message": "Application isn't running.", "version": "1.4.0"}}`, ""},
		answer{0, `{"error": {"step": "count", "number": -600,
		"message": "Application isn't running.", "version": ""}}`, ""},
		"*[1]"},
	{"garbage", answer{1, "", "fake osascript: execution error\n"},
		answer{1, "", "fake osascript: execution error\n"}, "*[1]"},
}

// Each failure mode answers a snapshot and an apply as the scripts do against such a Ghostty. The
// mode comes from osascript-fail's first line, else from CLD_FAKE_OSASCRIPT_FAIL.
func TestFakeOsascriptFailures(t *testing.T) {
	t.Parallel()
	for _, test := range failureAnswers {
		for _, from := range []string{"variable", "file"} {
			t.Run(test.mode+" from the "+from, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				saveModel(t, s, fakeGhostty(1))
				extra := map[string]string{"CLD_FAKE_OSASCRIPT_FAIL": test.mode}
				if from == "file" {
					s.WriteProbeFile(mac.FailFile, test.mode+"\n")
					extra = nil
				}
				test.snapshot.check(t, "a snapshot", osascript(t, s, extra, snapshotScript, "{}"))
				test.apply.check(t, "an apply",
					osascript(t, s, extra, applyScript, withIDs(failurePlan)))
				if got := shape(loadModel(t, s)); got != test.shape {
					t.Errorf("the apply left %s, want %s", got, test.shape)
				}
			})
		}
	}
	t.Run("the file first", testFailFile)
	t.Run("not running", testFailNotRunning)
	t.Run("timeout", testTimeout)
	t.Run("partial", testPartial)
	t.Run("delay", testDelay)
}

// osascript-fail's first line overrides the variable, and an empty one answers. An unknown mode
// is refused with status 2.
func testFailFile(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		file, variable string
		code           int
		stderr         string
	}{
		{"garbage\n", "", 1, "fake osascript: execution error\n"},
		{"garbage\ndenied\n", "denied", 1, "fake osascript: execution error\n"},
		{"", "garbage", 0, ""},
		{"nope\n", "", 2, "fake osascript: unknown failure mode \"nope\"\n"},
		{"", "nope", 0, ""},
	} {
		s := sandbox.New(t)
		saveModel(t, s, &mac.Model{})
		s.WriteProbeFile(mac.FailFile, test.file)
		stdout := ""
		if test.code == 0 {
			stdout = `{"running":false,"apps":[]}` + "\n"
		}
		result := osascript(t, s, map[string]string{"CLD_FAKE_OSASCRIPT_FAIL": test.variable},
			snapshotScript, "{}")
		checkExit(t, "osascript-fail "+test.file+", the variable "+test.variable, result,
			test.code, stdout, test.stderr)
	}
}

// Where Ghostty does not run, both scripts answer before any Apple Event, so no mode but garbage
// fails them.
func testFailNotRunning(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, mode := range []string{
		"denied", "off", "old", "changed", "gone", "partial", "short", "timeout",
	} {
		extra := map[string]string{"CLD_FAKE_OSASCRIPT_FAIL": mode}
		checkExit(t, mode+": a snapshot", osascript(t, s, extra, snapshotScript, "{}"), 0,
			`{"running":false,"apps":[]}`+"\n", "")
		answer{0, `{"error": {"step": "running", "number": 0, "message": "Ghostty is not running",
		  "version": ""}}`, ""}.check(t, mode+": an apply",
			osascript(t, s, extra, applyScript, withIDs(failurePlan)))
	}
}

// timeout sleeps until cld's deadline kills the fake, here a second's.
func testTimeout(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, fakeGhostty(1))
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	start := time.Now()
	extra := map[string]string{"CLD_FAKE_OSASCRIPT_FAIL": "timeout"}
	result := runCommand(t, fakeCommand(ctx, s, extra, filepath.Join(sandbox.ProbeBin, "osascript"),
		"-l", "JavaScript", "-e", snapshotScript, "{}"))
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || result.Code != -1 ||
		result.Stdout != "" || time.Since(start) < time.Second {
		t.Errorf("after %v: exit %d, stdout %q, want a kill after 1s and nothing printed",
			time.Since(start), result.Code, result.Stdout)
	}
}

// partial types and makes the first build of an apply, then fails as garbage does, printing no
// result; short prints the result so far. A snapshot answers.
func testPartial(t *testing.T) {
	t.Parallel()
	plan := withIDs(`{` + pace + `, "typed": [{"pane": "@1", "pid": 50003, "line": "ls"}],
	  "builds": [{"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a"}}}]},
	    {"kind": "window", "tabs": [{"tree": {"leaf": {"key": "b"}}}]}]}`)
	for _, test := range []struct {
		mode  string
		apply answer
	}{
		{"partial", answer{1, "", "fake osascript: execution error\n"}},
		{"short", answer{0, `{"typed": [{"pane": "@1"}], "builds": [{"keys": {"a": "@4"}}]}`, ""}},
	} {
		s := sandbox.New(t)
		saveModel(t, s, fakeGhostty(1))
		extra := map[string]string{"CLD_FAKE_OSASCRIPT_FAIL": test.mode}
		test.apply.check(t, test.mode, osascript(t, s, extra, applyScript, plan))
		m := loadModel(t, s)
		if got := shape(m); got != "*[4] | *[1]" {
			t.Errorf("%s: the apply built %s, want *[4] | *[1]", test.mode, got)
		}
		checkTyped(t, m, map[int][]string{1: {"^U", "ls", "\r"}})
		result := osascript(t, s, extra, snapshotScript, "{}")
		if result.Code != 0 || len(snapshotIDs(t, result.Stdout)) != 2 {
			t.Errorf("%s: the snapshot: exit %d, stdout %q, want the two terminals", test.mode,
				result.Code, result.Stdout)
		}
	}
}

// The delay file holds the next snapshot back for its seconds, after it read the model and
// removed the file: a model saved meanwhile shows only in the snapshot after. An apply leaves it,
// and waits as the apply delay file says once it has built.
func testDelay(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, fakeGhostty(1))
	s.WriteProbeFile(mac.DelayFile, "2\n")
	s.WriteProbeFile(mac.ApplyDelayFile, "1\n")
	start := time.Now()
	checkPrinted(t, "an apply", osascript(t, s, nil, applyScript, `{`+pace+`, "typed": [],
	  "builds": [{"kind": "window", "tabs": [{"tree": {"leaf": {"key": "a"}}}]}]}`).Stdout,
		withIDs(`{"typed": [], "builds": [{"keys": {"a": "@4"}}]}`))
	if time.Since(start) < time.Second || exists(filepath.Join(s.ProbeDir, mac.ApplyDelayFile)) {
		t.Errorf("the apply answered after %v, want 1 s, and the apply delay file gone",
			time.Since(start))
	}
	start = time.Now()
	wait := startCommand(t, fakeCommand(t.Context(), s, nil, filepath.Join(sandbox.ProbeBin,
		"osascript"), "-l", "JavaScript", "-e", snapshotScript, "{}"))
	sandbox.WaitFor(t, 10*time.Second, "the snapshot to take the delay file", func() bool {
		_, err := os.Stat(filepath.Join(s.ProbeDir, mac.DelayFile))
		return errors.Is(err, os.ErrNotExist)
	})
	saveModel(t, s, fakeGhostty(1))
	held := wait()
	if ids := snapshotIDs(t, held.Stdout); len(ids) != 2 || time.Since(start) < 2*time.Second {
		t.Errorf("after %v the held snapshot printed %q, want 2 s and the two terminals",
			time.Since(start), held.Stdout)
	}
	if ids := snapshotIDs(t, osascript(t, s, nil, snapshotScript, "{}").Stdout); len(ids) != 1 {
		t.Errorf("the next snapshot held %v, want terminal 1 at once", ids)
	}
}

// snapshotIDs are the app-wide terminals of a snapshot that the fake osascript printed.
func snapshotIDs(t *testing.T, stdout string) []string {
	t.Helper()
	var snapshot struct {
		Terminals []string `json:"terminals"`
	}
	if err := json.Unmarshal([]byte(stdout), &snapshot); err != nil {
		t.Fatalf("snapshot %q: %v", stdout, err)
	}
	return snapshot.Terminals
}

// The fake osascript takes only the six words cld runs it with, a known marker and JSON input,
// a plan for an apply. It refuses anything else with status 2, logging no call.
func TestFakeOsascriptArguments(t *testing.T) {
	t.Parallel()
	const unexpected = "fake osascript: unexpected arguments\n"
	for _, test := range []struct {
		name   string
		args   []string
		stderr string
	}{
		{"none", nil, unexpected},
		{"AppleScript", []string{"-l", "AppleScript", "-e", snapshotScript, "{}"}, unexpected},
		{"no language", []string{"-e", snapshotScript, "{}"}, unexpected},
		{"a file", []string{"-l", "JavaScript", "snapshot.js", "{}"}, unexpected},
		{"two inputs", []string{"-l", "JavaScript", "-e", snapshotScript, "{}", "{}"}, unexpected},
		{"watch", []string{"-l", "JavaScript", "-e", "// cld: watch\n", `{"every": 5}`},
			"fake osascript: unexpected script \"// cld: watch\"\n"},
		{"no marker", []string{"-l", "JavaScript", "-e", "run()\n", "{}"},
			"fake osascript: unexpected script \"run()\"\n"},
		{"no JSON", []string{"-l", "JavaScript", "-e", snapshotScript, "{"},
			"fake osascript: the input is no JSON\n"},
		{"no plan", []string{"-l", "JavaScript", "-e", applyScript, "[]"},
			"fake osascript: the input is no plan\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			saveModel(t, s, fakeGhostty(1))
			checkExit(t, test.name, runFake(t, s, nil, "osascript", test.args...),
				2, "", test.stderr)
			if calls := s.OsascriptCalls(); calls != nil {
				t.Errorf("osascript.jsonl holds %+v, want nothing", calls)
			}
		})
	}
}
