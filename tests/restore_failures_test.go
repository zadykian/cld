package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The sessions cld restore leaves ended or alone, and those it cannot bring back (decisions 48.5
// and 48.8).

// restore leaves ended, with a note, a session whose run mark is older than CLD_IDLE_DAYS, and
// removes the mark. It brings back one used within the limit, keeping its mark's time, which
// claude's UserPromptSubmit hook touches. A CLD_IDLE_DAYS that is no number of days is refused.
func TestRestoreIdle(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	writeRestorable(t, s, "fresh", s.Work, firstID, probe, s.Environ(nil))
	writeRestorable(t, s, "stale", s.Work, secondID, probe, s.Environ(nil))
	age(t, time.Hour, companionFile(s, "fresh", ".run"))
	age(t, 3*24*time.Hour+time.Minute, companionFile(s, "stale", ".run"))

	checkExit(t, "restore, CLD_IDLE_DAYS=2d",
		s.RunCld(map[string]string{"CLD_IDLE_DAYS": "2d"}, "restore"), 1, "",
		"cld: CLD_IDLE_DAYS is not a number of days: '2d'\n")
	checkMarks(t, s, "after the refusal", map[string]bool{"fresh": true, "stale": true})

	checkExit(t, "restore", s.RunCld(map[string]string{"CLD_IDLE_DAYS": "2"}, "restore"), 0,
		"Restored session 'fresh' in "+s.Work+"\n",
		"cld: left session 'stale' ended, idle for 3 days\n")
	claude := s.WaitProbes(1)[0]
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-fresh"}) {
		t.Errorf("sessions %q, want [cld-fresh]", sessions)
	}
	checkMarks(t, s, "after restore", map[string]bool{"fresh": true, "stale": false})
	if readEntry(s, "stale") == "" {
		t.Error("restore forgot stale's entry")
	}
	markTime := func() time.Duration {
		t.Helper()
		info, err := os.Stat(companionFile(s, "fresh", ".run"))
		if err != nil {
			t.Fatal(err)
		}
		return time.Since(info.ModTime())
	}
	if since := markTime(); since < 30*time.Minute {
		t.Errorf("fresh's run mark made %v ago after restore, want it kept from an hour ago",
			since.Round(time.Second))
	}
	claude.Hook("UserPromptSubmit", `{"prompt":"go"}`)
	if since := markTime(); since > time.Minute {
		t.Errorf("fresh's run mark made %v ago after a prompt, want now", since.Round(time.Second))
	}
}

// A session restore cannot bring back is a warning, and the others come back, with status 1. The
// warnings: a directory gone, pointing at join, no environment kept, and a claude too old, checked
// where it starts. A session without a run mark stays ended.
func TestRestoreFailures(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	env := append(s.Environ(nil), "CLD_TEST_ORIGIN=recorded")
	gone := filepath.Join(s.Root, "gone")
	old := filepath.Join(s.Root, "old claude")
	s.WriteProgram(old, "#!/bin/sh\necho '2.1.231 (Claude Code)'\n", 0o755)
	writeRestorable(t, s, "gone", gone, firstID, probe, env)
	writeRestorable(t, s, "noenv", s.Work, "", probe, env)
	if err := os.Remove(companionFile(s, "noenv", ".env")); err != nil {
		t.Fatal(err)
	}
	writeRestorable(t, s, "ok", s.Work, secondID, probe, env)
	writeRestorable(t, s, "old", s.Work, "", old, env)
	writeEntry(t, s, "unmarked", s.Work, "")

	checkExit(t, "restore", s.RunCld(nil, "restore"), 1, "Restored session 'ok' in "+s.Work+"\n",
		"cld: warning: cannot restore session 'gone': session 'gone' ran in "+gone+
			", which no longer exists; resume it from here with cld join -s gone --resume "+
			firstID+"\n"+
			"cld: warning: cannot restore session 'noenv': cld keeps no environment of it: "+
			companionFile(s, "noenv", ".env")+": no such file or directory\n"+
			"cld: warning: cannot restore session 'old': claude 2.1.232 or newer is required, "+
			"found '2.1.231 (Claude Code)'\n")
	claude := s.WaitProbes(1)[0]
	want := []string{"--name", "cld-ok", "--settings", sessionSettings(s, "cld-ok", s.Work),
		"--resume", secondID}
	if !slices.Equal(claude.Argv, want) {
		t.Errorf("claude arguments %q, want %q", claude.Argv, want)
	}
	if claude.Env["CLD_TEST_ORIGIN"] != "recorded" {
		t.Errorf("claude got CLD_TEST_ORIGIN=%q, want recorded", claude.Env["CLD_TEST_ORIGIN"])
	}
	time.Sleep(time.Second)
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want ok's alone", len(probes))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-ok"}) {
		t.Errorf("sessions %q, want [cld-ok]", sessions)
	}
}

// restore leaves alone, silently, a session whose server runs without it: a server of cld's the
// session outlived, or the user's own tmux -L cld-NAME. A server that starts just before restore's
// new-session fails it: a warning, status 1, and the session keeps both marks for the next restore.
func TestRestoreBesideServers(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	for _, name := range []string{"foreign", "lingering", "x"} {
		writeRestorable(t, s, name, s.Work, firstID, probe, s.Environ(nil))
		s.WriteFile(companionFile(s, name, ".busy"), "")
	}
	s.MustTmux("cld-foreign", "-f", "/dev/null", "new-session", "-d", "-s", "cld-foreign")
	s.MustTmux("cld-lingering", "-f", "/dev/null", "new-session", "-d", "-s", "other",
		";", "set", "-s", "@cld", "1")

	tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
	tmux.start(t)
	first := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	s.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "cld-x")
	tmux.release(t)
	checkExit(t, "restore", first(), 1, "",
		"cld: warning: cannot restore session 'x': duplicate session: cld-x\n")
	if !exists(companionFile(s, "x", ".run")) || !exists(companionFile(s, "x", ".busy")) {
		t.Error("x lost its run mark or its busy mark as restore's tmux failed")
	}
	checkLeftAlone(t, s, "after restore")
	if probes := s.Probes(); len(probes) != 0 {
		t.Errorf("%d claudes started, want none", len(probes))
	}

	s.MustTmux("cld-x", "kill-server")
	waitServerExit(t, s, "x")
	checkExit(t, "restore again", s.RunCld(nil, "restore"), 0,
		"Restored session 'x' in "+s.Work+", continuing its turn\n", "")
	claude := s.WaitProbes(1)[0]
	want := []string{"--name", "cld-x", "--settings", sessionSettings(s, "cld-x", s.Work),
		"--resume", firstID, continuePrompt}
	if !slices.Equal(claude.Argv, want) {
		t.Errorf("claude arguments %q, want %q", claude.Argv, want)
	}
	if exists(companionFile(s, "x", ".busy")) {
		t.Error("x's busy mark is left as restore brought it back")
	}
	checkLeftAlone(t, s, "after restore again")
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want x's alone", len(probes))
	}
}

// checkLeftAlone checks that the servers cld-foreign and cld-lingering keep their sessions, and
// the sessions of those names their run and busy marks.
func checkLeftAlone(t *testing.T, s *sandbox.Sandbox, when string) {
	t.Helper()
	for server, sessions := range map[string]string{
		"cld-foreign": "cld-foreign", "cld-lingering": "other",
	} {
		got, err := s.Tmux(server, "list-sessions", "-F", "#{session_name}")
		if err != nil || got != sessions {
			t.Errorf("%s: sessions of server %s %q, %v, want %q", when, server, got, err, sessions)
		}
	}
	for _, name := range []string{"foreign", "lingering"} {
		if !exists(companionFile(s, name, ".run")) || !exists(companionFile(s, name, ".busy")) {
			t.Errorf("%s: %s lost its run mark or its busy mark", when, name)
		}
	}
}
