package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The run mark, the busy mark and the environment beside a session's entry, which restore acts on
// (decisions 48.1 to 48.4).

// join writes the environment beside the session's entry, and its tmux makes the run mark once it
// has made the session. kill and claude's exit with status 0 remove the mark, which a claude that
// fails keeps, and join makes it again as it brings the session back.
func TestRunMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	extra := map[string]string{"CLD_TEST_ORIGIN": "join", "TERMINAL_EMULATOR": "JetBrains-JediTerm"}
	term := startCld(t, s, "tmux", extra, "join", "-s", "a", "--", "--model", "opus")
	a := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join", map[string]bool{"a": true})
	checkEnvironment(t, s, a)

	mustKill(t, s, "a")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after kill", map[string]bool{"a": false})
	if readEntry(s, "a") == "" || !exists(companionFile(s, "a", ".env")) {
		t.Error("kill removed the entry or the environment")
	}

	term = startCld(t, s, "tmux", nil, "join", "-s", "a")
	resumed := s.WaitProbes(2)[1]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join resumed a", map[string]bool{"a": true})
	resumed.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !resumed.Alive() })
	waitScreen(t, term, "claude exited with status 1")
	checkMarks(t, s, "after claude failed", map[string]bool{"a": true})
	mustKill(t, s, "a")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })

	term = startCld(t, s, "tmux", nil, "join", "-s", "a")
	again := s.WaitProbes(3)[2]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join resumed a again", map[string]bool{"a": true})
	again.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after claude exited with status 0", map[string]bool{"a": false})
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after claude exited with status 0, want none", sessions)
	}
}

// checkEnvironment checks the environment file beside a's entry, which the user alone may read:
// the claude join checked, and the environment claude a got, without the terminal's variables.
func checkEnvironment(t *testing.T, s *sandbox.Sandbox, a *sandbox.Probe) {
	t.Helper()
	info, err := os.Stat(companionFile(s, "a", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("environment file's mode %v, want 0600", mode)
	}
	var env recorded
	data, err := os.ReadFile(companionFile(s, "a", ".env"))
	if err != nil || json.Unmarshal(data, &env) != nil {
		t.Fatalf("environment file %q: %v", data, err)
	}
	if want := filepath.Join(sandbox.ProbeBin, "claude"); env.Claude != want {
		t.Errorf("environment file names claude %q, want %q", env.Claude, want)
	}
	for _, variable := range []string{
		"CLD_TEST_ORIGIN=join", "CLD_PROBE_DIR=" + s.ProbeDir, "HOME=" + s.Home,
	} {
		if !slices.Contains(env.Environment, variable) {
			t.Errorf("environment file lacks %s:\n%q", variable, env.Environment)
		}
	}
	if slices.ContainsFunc(env.Environment, func(v string) bool {
		return strings.HasPrefix(v, "TERMINAL_EMULATOR=")
	}) {
		t.Errorf("environment file keeps TERMINAL_EMULATOR:\n%q", env.Environment)
	}
	if origin := a.Env["CLD_TEST_ORIGIN"]; origin != "join" {
		t.Errorf("claude got CLD_TEST_ORIGIN=%q, want join, as the environment file has it", origin)
	}
}

// claude's hooks keep the busy mark beside the session's entry (decision 48.3): UserPromptSubmit
// makes it, and the hooks that end a turn remove it. A forgotten entry gets no mark.
func TestBusyMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	busy := companionFile(s, "a", ".busy")
	for _, step := range []struct {
		event, input string
		busy         bool
	}{
		{"UserPromptSubmit", `{"prompt":"go"}`, true},
		{"Stop", "", false},
		{"UserPromptSubmit", "", true},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":false}`, true},
		{"Notification", `{"notification_type":"permission_prompt"}`, true},
		{"StopFailure", "", false},
		{"UserPromptSubmit", "", true},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt": true}`, false},
		{"UserPromptSubmit", "", true},
		{"Notification", `{"notification_type":"idle_prompt"}`, false},
	} {
		probe.Hook(step.event, step.input)
		if got := exists(busy); got != step.busy {
			t.Errorf("busy mark after %s %s: %v, want %v", step.event, step.input, got, step.busy)
		}
	}
	forget(t, s, "a")
	probe.Hook("UserPromptSubmit", "")
	if exists(busy) {
		t.Error("UserPromptSubmit made a busy mark for a forgotten entry")
	}
}

// A join that makes no session leaves no run mark (decision 48.1). Refused for want of a terminal
// it writes nothing, and where tmux cannot open the terminal it leaves the entry, ended, for a new
// session and one that kill ended. restore then brings none of them back.
func TestNoSessionNoMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "join", "-s", "x"); result.Code != 1 {
		t.Errorf("join without a terminal: exit %d, stderr %q, want exit 1", result.Code, result.Stderr)
	}
	for _, ext := range []string{".json", ".env", ".run", ".start"} {
		if exists(companionFile(s, "x", ext)) {
			t.Errorf("join without a terminal left x%s", ext)
		}
	}
	unknown := map[string]string{"TERM": "cld-no-such-terminal"}
	if result := s.RunCldOnTerminal(unknown, "join", "-s", "y"); result.Code == 0 {
		t.Errorf("join with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail", result.Stderr)
	}
	if got, want := readEntry(s, "y"), entry("y", s.Work, ""); got != want {
		t.Errorf("y's entry %q, want %q", got, want)
	}
	startCld(t, s, "tmux", nil, "join", "-s", "z")
	z := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join", map[string]bool{"z": true})
	mustKill(t, s, "z")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !z.Alive() })
	if result := s.RunCldOnTerminal(unknown, "join", "-s", "z"); result.Code == 0 {
		t.Errorf("join of z with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail",
			result.Stderr)
	}
	checkMarks(t, s, "after tmux failed", map[string]bool{"x": false, "y": false, "z": false})
	checkExit(t, "restore", s.RunCld(nil, "restore"), 0, "", "")
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want z's alone", len(probes))
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// A join that cannot write the environment beside the entry, a directory in its place here, makes
// no run mark. claude's exit with status 0 still removes the mark a reboot left, which restore
// would otherwise act on (decision 48.2).
func TestRunMarkWithoutEnvironment(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	claudePath := filepath.Join(sandbox.ProbeBin, "claude")
	writeRestorable(t, s, "a", s.Work, firstID, claudePath, s.Environ(nil))
	env := companionFile(s, "a", ".env")
	if err := os.Remove(env); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(env, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, time.Hour, companionFile(s, "a", ".run"))
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	claude := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if info, err := os.Stat(env); err != nil || !info.IsDir() {
		t.Fatalf("join wrote the environment in place of the directory: %v", err)
	}
	info, err := os.Stat(companionFile(s, "a", ".run"))
	if err != nil {
		t.Fatal(err)
	}
	if since := time.Since(info.ModTime()); since < 30*time.Minute {
		t.Errorf("run mark made %v ago after join, want it kept from an hour ago",
			since.Round(time.Second))
	}
	claude.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after claude exited with status 0", map[string]bool{"a": false})
	checkExit(t, "restore", s.RunCld(nil, "restore"), 0, "", "")
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want the resumed one alone", len(probes))
	}
}
