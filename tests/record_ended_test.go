package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Sessions that have ended, which list shows, kill refuses and join brings back by the record's
// entry (decisions 40.3 to 40.5).

// makeDirs makes the directories dirs.
func makeDirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// optionFollowedBy reports whether args hold option, followed by value.
func optionFollowedBy(args []string, option, value string) bool {
	i := slices.Index(args, option)
	return i >= 0 && i+1 < len(args) && args[i+1] == value
}

// checkJoinRefused reports a run of cld, named by what, that did not exit 1 with stderr alone.
func checkJoinRefused(t *testing.T, what string, result sandbox.Result, stderr string) {
	t.Helper()
	if result.Code != 1 || result.Stdout != "" || result.Stderr != stderr {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			what, result.Code, result.Stdout, result.Stderr, stderr)
	}
}

// A session whose server no longer runs, killed here, has ended. join brings its conversation
// back by the recorded ID, in the directory it ran in, from wherever join runs. Once that
// directory has gone, join still attaches while the session runs. Once it has ended, join refuses
// it, naming a join --resume that works from here.
func TestJoinRecorded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir, elsewhere := filepath.Join(s.Root, "project"), filepath.Join(s.Root, "elsewhere")
	makeDirs(t, dir, elsewhere)
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	first := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	killAndWait(t, s, elsewhere, first, "-n", "api", "-s", "1")
	checkEnded(t, s, dir, elsewhere)
	resumed := resumeRecorded(t, s, dir, elsewhere)
	checkDirectoryGone(t, s, dir, elsewhere, resumed)
	startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "api", "-s", "1", "--resume", firstID)
	here := waitResumedHere(t, s, first, resumed)
	want := []string{"--name", "cld-api-1", "--settings",
		sessionSettings(s, "cld-api-1", elsewhere), "--resume", firstID}
	if !slices.Equal(here.Argv, want) || here.Cwd != elsewhere {
		t.Errorf("claude arguments %q in %s, want %q in %s", here.Argv, here.Cwd, want, elsewhere)
	}
}

// checkEnded checks, from elsewhere, that list shows session api-1 ended in dir, and that kill
// and detach refuse it, pointing at join.
func checkEnded(t *testing.T, s *sandbox.Sandbox, dir, elsewhere string) {
	t.Helper()
	want := "NAME   STATE     LAST ACTIVE  DIRECTORY\n" + "api-1  ended     -            " + dir + "\n"
	result := s.RunCldIn(elsewhere, nil, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	refused := "cld: session 'api-1' has ended; resume it with cld join -n api -s 1\n"
	for _, command := range []string{"kill", "detach"} {
		result := s.RunCldIn(elsewhere, nil, command, "-n", "api", "-s", "1")
		if result.Code != 1 || result.Stderr != refused {
			t.Errorf("%s: exit %d, stderr %q, want exit 1, stderr %q",
				command, result.Code, result.Stderr, refused)
		}
	}
}

// resumeRecorded joins session api-1, which has ended, from elsewhere with words for claude after
// --. It checks that claude resumes firstID in dir, and the entry, and returns that claude.
func resumeRecorded(t *testing.T, s *sandbox.Sandbox, dir, elsewhere string) *sandbox.Probe {
	t.Helper()
	startCldIn(t, s, "tmux", elsewhere, nil,
		"join", "-n", "api", "-s", "1", "--", "--model", "opus", "--add-dir", "../y")
	resumed := s.WaitProbes(2)[1]
	want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", dir),
		"--resume", firstID, "--model", "opus", "--add-dir", "../y"}
	if !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
	if resumed.Cwd != dir || resumed.Env["PWD"] != dir {
		t.Errorf("claude runs in %s, PWD %q, want %s", resumed.Cwd, resumed.Env["PWD"], dir)
	}
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, firstID); got != want {
		t.Errorf("entry %q, want %q", got, want)
	}
	return resumed
}

// checkDirectoryGone removes dir, where session api-1 runs claude. join from elsewhere still
// attaches to the session while it runs, and once it has ended refuses it, naming the join that
// resumes its conversation from there.
func checkDirectoryGone(
	t *testing.T, s *sandbox.Sandbox, dir, elsewhere string, claude *sandbox.Probe,
) {
	t.Helper()
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	checkJoinRefused(t, "join of a session that runs",
		s.RunCldIn(elsewhere, nil, "join", "-n", "api", "-s", "1"),
		"cld: join needs a terminal, and its input is not one\n")
	killAndWait(t, s, elsewhere, claude, "-n", "api", "-s", "1")
	checkJoinRefused(t, "join without its directory",
		s.RunCldIn(elsewhere, nil, "join", "-n", "api", "-s", "1"),
		"cld: session 'api-1' ran in "+dir+", which no longer exists; "+
			"resume it from here with cld join -n api -s 1 --resume "+firstID+"\n")
}

// waitResumedHere waits for a claude in s other than first and resumed, and returns it.
func waitResumedHere(
	t *testing.T, s *sandbox.Sandbox, first, resumed *sandbox.Probe,
) *sandbox.Probe {
	t.Helper()
	var here *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude to resume here", func() bool {
		for _, probe := range s.Probes() {
			if probe.PID != first.PID && probe.PID != resumed.PID {
				here = probe
			}
		}
		return here != nil
	})
	return here
}

// join --resume SESSION --fork writes its session's entry with no conversation, and claude's
// SessionStart hook gives it the copy's ID. Once the session has ended, join brings the copy back
// by that ID, without --fork-session (decision 45.3).
func TestForkRecorded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, "project")
	makeDirs(t, dir)
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1", "--fork", "--resume", firstID)
	copied := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Errorf("entry as claude starts %q, want %q", got, want)
	}
	copied.Hook("SessionStart", `{"session_id":"`+secondID+`","source":"resume"}`)
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, secondID); got != want {
		t.Errorf("entry after SessionStart %q, want %q", got, want)
	}
	killAndWait(t, s, dir, copied, "-n", "api", "-s", "1")
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	resumed := s.WaitProbes(2)[1]
	want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", dir),
		"--resume", secondID}
	if !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
}

// A session whose entry has no conversation, as its claude or tmux failed before claude started
// one, is resumed by its name, cld-NAME. From wherever join runs, it starts in the directory it
// ran in. The fake tmux records the command.
func TestJoinWithoutConversation(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir, elsewhere := filepath.Join(s.Root, "project"), filepath.Join(s.Root, "elsewhere")
	makeDirs(t, dir, elsewhere)
	result := s.RunCldOnTerminalIn(dir, fakeTmux(s), "join", "-n", "api", "-s", "1")
	if result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join creating api-1: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Fatalf("entry %q, want %q", got, want)
	}
	result = s.RunCldOnTerminalIn(elsewhere, fakeTmux(s), "join", "-n", "api", "-s", "1")
	if result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join resuming api-1: exit %d, stderr %q", result.Code, result.Stderr)
	}
	record := s.FakeTmuxRecord()
	if !optionFollowedBy(record.Argv, "--resume", "cld-api-1") {
		t.Errorf("tmux arguments %q, want --resume cld-api-1", record.Argv)
	}
	if !optionFollowedBy(record.Argv, "-c", dir) || record.Cwd != dir {
		t.Errorf("tmux runs in %s, arguments %q, want -c %s and to run there",
			record.Cwd, record.Argv, dir)
	}
}
