package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld's record of its sessions in the sandbox's home directory (decision 40): the entry join
// writes, the hooks that keep it, and where it goes. The helpers the record_*_test.go files share
// are here too.

// fakeTmux is the environment where cld finds the fake tmux first on the PATH (see probe). It
// passes the version check, says that no server runs, and records the command join would run. cld
// then makes no session, and prints the title of the one it would have made.
func fakeTmux(s *sandbox.Sandbox) map[string]string {
	path := filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"]
	return map[string]string{"PATH": path, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}
}

// touched reports an entry of session name in s that was not written or touched within a minute.
func touched(t *testing.T, s *sandbox.Sandbox, name, after string) {
	t.Helper()
	info, err := os.Stat(entryFile(s, name))
	if err != nil {
		t.Fatal(err)
	}
	if since := time.Since(info.ModTime()); since > time.Minute {
		t.Errorf("entry written %v ago after %s, want now", since.Round(time.Second), after)
	}
}

// killAndWait kills the session that args name, running cld in dir, and waits for its claude to
// exit.
func killAndWait(
	t *testing.T, s *sandbox.Sandbox, dir string, claude *sandbox.Probe, args ...string,
) {
	t.Helper()
	if result := s.RunCldIn(dir, nil, append([]string{"kill"}, args...)...); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !claude.Alive() })
}

// sessionStartInputs are inputs of claude's SessionStart hook, each with the conversation the
// entry has after it: an ID that is no conversation's, or none, leaves the entry unchanged.
var sessionStartInputs = []struct{ input, conversation string }{
	{`{"session_id":"` + firstID + `","transcript_path":"/t/` + firstID + `.jsonl",` +
		`"cwd":"/t","hook_event_name":"SessionStart","source":"startup"}`, firstID},
	{`{"session_id":"` + secondID + `","source":"clear"}`, secondID},
	{`{"session_id":"served:unknown","source":"startup"}`, secondID},
	{`{"source":"compact"}`, secondID},
}

// join writes the entry of the session it makes, with no conversation yet, before tmux starts it.
// claude's hooks then write the conversation's ID into it and touch it (decision 40.2), wherever
// claude has gone, for a directory whose name holds quotes too.
func TestRecordHooks(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, `it's "x"`)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	probe := s.WaitProbes(1)[0]
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Errorf("entry as claude starts %q, want %q", got, want)
	}
	probe.Send("cd " + s.Root)
	for _, step := range sessionStartInputs {
		probe.Hook("SessionStart", step.input)
		if got, want := readEntry(s, "api-1"), entry("api-1", dir, step.conversation); got != want {
			t.Errorf("entry after SessionStart %s: %q, want %q", step.input, got, want)
		}
	}
	for _, event := range []string{"Stop", "SessionEnd"} {
		age(t, time.Hour, entryFile(s, "api-1"))
		probe.Hook(event, `{"session_id":"`+secondID+`","reason":"other"}`)
		touched(t, s, "api-1", event)
		if got, want := readEntry(s, "api-1"), entry("api-1", dir, secondID); got != want {
			t.Errorf("entry after %s %q, want %q", event, got, want)
		}
	}
	// A forgotten entry stays forgotten.
	forget(t, s, "api-1")
	probe.Hook("SessionEnd", `{"session_id":"`+secondID+`","reason":"other"}`)
	if _, err := os.Stat(entryFile(s, "api-1")); !os.IsNotExist(err) {
		t.Errorf("SessionEnd made an entry that was forgotten: %v", err)
	}
}

// The record is in $XDG_STATE_HOME/cld where that is a whole path, and else in the home
// directory's .local/state/cld (decision 40.1). Where cld cannot write it, join warns and makes
// the session all the same.
func TestRecordPlace(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	state := filepath.Join(s.Root, "state")
	env := fakeTmux(s)
	env["XDG_STATE_HOME"] = state
	if result := s.RunCldOnTerminal(env, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Errorf("join: exit %d, stderr %q", result.Code, result.Stderr)
	}
	data, err := os.ReadFile(filepath.Join(state, "cld", "sessions", "x.json"))
	if err != nil || string(data) != entry("x", s.Work, "") {
		t.Errorf("entry in $XDG_STATE_HOME: %q, %v", data, err)
	}
	if _, err := os.Stat(entryFile(s, "x")); !os.IsNotExist(err) {
		t.Errorf("entry in the home directory: %v, want none", err)
	}
	env["XDG_STATE_HOME"] = "state"
	if result := s.RunCldOnTerminal(env, "join", "-s", "z"); result.Code != 0 || result.Stderr != "" {
		t.Errorf("join: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if entry := readEntry(s, "z"); entry == "" {
		t.Error("no entry in the home directory with a relative $XDG_STATE_HOME")
	}
	checkUnwritableRecord(t, s, env)
}

// checkUnwritableRecord points env's XDG_STATE_HOME at a file in s, where cld cannot write the
// record. join warns, and makes the session all the same, without the record's hooks and with
// agent view off (decisions 40.7 and 47); list reads no entries there.
func checkUnwritableRecord(t *testing.T, s *sandbox.Sandbox, env map[string]string) {
	t.Helper()
	blocked := filepath.Join(s.Root, "blocked")
	s.WriteFile(blocked, "")
	env["XDG_STATE_HOME"] = blocked
	result := s.RunCldOnTerminal(env, "join", "-s", "y")
	want := "cld: warning: cannot record session 'y': " + blocked + ": not a directory\n"
	if result.Code != 0 || result.Stderr != want {
		t.Errorf("join: exit %d, stderr %q, want exit 0, stderr %q",
			result.Code, result.Stderr, want)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	settings := argv[i+1]
	if strings.Contains(settings, "SessionEnd") || strings.Contains(settings, "session_id") {
		t.Errorf("settings with the record's hooks: %s", settings)
	} else if !strings.HasPrefix(settings, `{"disableAgentView":true,"hooks":{`) {
		t.Errorf("settings without agent view off: %s", settings)
	}
	result = s.RunCld(env, "list")
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("list: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			result.Code, result.Stdout, result.Stderr)
	}
}
