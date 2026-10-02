package tests

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join --resume SESSION makes its session as join makes any, in the current directory. claude
// gets --resume SESSION, as one word whatever it holds (decision 16.8), with --fork-session for
// --fork, then the words after "--". A git repository makes no worktree session (decision 16.3).
// The work directory's name leaves nothing, so -s SUFFIX names the session SUFFIX.
func TestJoinResume(t *testing.T) {
	t.Parallel()
	for _, test := range joinResumeCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			startCld(t, s, "tmux", nil, test.args...)
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-" + test.name}) {
				t.Errorf("sessions %q, want [cld-%s]", sessions, test.name)
			}
			want := append([]string{"--name", "cld-" + test.name,
				"--settings", sessionSettings(s, "cld-"+test.name, s.Work),
				"--resume", test.resume}, test.after...)
			if !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != s.Work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, s.Work)
			}
			width := max(4, len(test.name))
			list := fmt.Sprintf("%-*s  STATE     LAST ACTIVE  DIRECTORY\n"+
				"%-*s  attached  now          %s\n", width, "NAME", width, test.name, s.Work)
			result := s.RunCld(nil, "list")
			if result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
					result.Code, result.Stderr, result.Stdout, list)
			}
		})
	}
}

// joinResumeCases are TestJoinResume's.
var joinResumeCases = []struct {
	args         []string
	name, resume string
	// after is what claude gets after --resume: with --fork, --fork-session, then the words
	// after "--"
	after []string
}{
	{[]string{"join", "-s", "x", "--resume", "cld-x"}, "x", "cld-x", nil},
	{[]string{"join", "--suffix=x", "--resume=cld-x"}, "x", "cld-x", nil},
	{[]string{"join", "-n", "a", "-s", "x", "--resume", "cld-a-x"}, "a-x", "cld-a-x", nil},
	{[]string{"join", "--resume", firstID}, "0", firstID, nil},
	{[]string{"join", "-n", "a", "--resume", firstID}, "a-0", firstID, nil},
	{[]string{"join", "-s", "x", "--resume", firstID}, "x", firstID, nil},
	{[]string{"join", "-s", "x", "--resume", "a b"}, "x", "a b", nil},
	{[]string{"join", "-s", "x", "--resume", "fix;"}, "x", "fix;", nil},
	{[]string{"join", "-s", "x", "--resume", `fix\;`}, "x", `fix\;`, nil},
	{[]string{"join", "-s", "x", "--resume", "#{session_name}"}, "x", "#{session_name}", nil},
	{[]string{"join", "-s", "x", "--resume", "cld-x", "--", "--mcp-config", "m.json",
		"--add-dir", "../y"}, "x", "cld-x", []string{"--mcp-config", "m.json", "--add-dir", "../y"}},
	{[]string{"join", "-s", "x", "--resume", "cld-x", "--"}, "x", "cld-x", nil},
	{[]string{"join", "--resume", "a", "--", "--fork-session", "go on;"}, "0", "a",
		[]string{"--fork-session", "go on;"}},
	{[]string{"join", "-s", "x", "--resume", "a", "--", "--"}, "x", "a", []string{"--"}},
	{[]string{"join", "--fork", "--resume", "cld-a-0"}, "0", "cld-a-0",
		[]string{"--fork-session"}},
	{[]string{"join", "-s", "b", "--fork", "--resume", "cld-a-0"}, "b", "cld-a-0",
		[]string{"--fork-session"}},
	{[]string{"join", "--fork", "-n", "a", "--resume", firstID}, "a-0", firstID,
		[]string{"--fork-session"}},
	{[]string{"join", "-s", "x", "--fork=true", "--resume", "fix;"}, "x", "fix;",
		[]string{"--fork-session"}},
	{[]string{"join", "-s", "x", "--fork", "--resume", "cld-x-0"}, "x", "cld-x-0",
		[]string{"--fork-session"}},
	{[]string{"join", "-s", "x", "--fork=false", "--resume", "cld-x"}, "x", "cld-x", nil},
	{[]string{"join", "-s", "x", "--fork", "--resume", "a", "--", "--add-dir", "../y"}, "x", "a",
		[]string{"--fork-session", "--add-dir", "../y"}},
}

// join --fork refuses a --resume SESSION that is the name its copy would take, as claude compares
// names (decision 45.1). Where a default makes that name, the refusal has status 1: in repository
// api, a copy of cld-api-0 would be session api-0 again. No session starts, nor claude but for its
// version.
func TestForkOwnName(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// repository is the git repository cld runs in, or where empty the work directory, whose
		// name leaves nothing; name is the session's
		repository, name string
	}{
		{[]string{"join", "--fork", "--resume", "cld-0"}, "", "0"},
		{[]string{"join", "-s", "x", "--fork", "--resume", "cld-x"}, "", "x"},
		{[]string{"join", "-s", "x", "--fork", "--resume", " CLD-X\t"}, "", "x"},
		{[]string{"join", "-n", "a", "--fork", "--resume", "cld-a-0"}, "", "a-0"},
		{[]string{"join", "--fork", "--resume", "cld-api-0"}, "api", "api-0"},
		{[]string{"join", "-s", "Fix", "--fork", "--resume", "cld-API-fix"}, "api", "api-Fix"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := s.Work
			if test.repository != "" {
				dir = repository(t, s, test.repository)
			}
			want := "cld: join: --fork would give the copy SESSION's own name, cld-" + test.name +
				"; give another -s SUFFIX (see cld help)\n"
			result := s.RunCldIn(dir, nil, test.args...)
			if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
					result.Code, result.Stdout, result.Stderr, want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
			if probes := s.Probes(); len(probes) != 0 {
				t.Errorf("%d claude processes, want none", len(probes))
			}
		})
	}
}
