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

// Repositories of one name share NAME and its indexes, as do directories (decision 37): work/api,
// scratch/api, and pla\tin/api, in no repository. Without -n, join, detach and kill refuse a
// session made elsewhere, naming where, and -s offers only those join takes. A subdirectory and a
// worktree take their repository's; -n, the root directory and an older cld's session, any.
func TestSessionOfAnotherRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dirs := makeAPIRepositories(t, s)
	claudes := dirs.startSessions(t, s)

	list := "NAME   STATE     LAST ACTIVE  DIRECTORY\n" +
		"api-0  attached  now          " + dirs.work + "\n" +
		"api-1  attached  now          " + dirs.other + "\n" +
		"api-2  detached  now          " + s.Work + "\n" +
		"api-3  attached  now          " + dirs.plain + "\n"
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != list || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, list)
	}
	// Under a locale without UTF-8, tmux would write the tab before the home as "_", leaving the
	// home unread and the session taken. cld's -u has it write them as they are (decision 37.3).
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	for _, env := range []map[string]string{nil, notUTF8} {
		dirs.checkRefused(t, s, env)
	}
	for _, claude := range claudes {
		if !claude.Alive() {
			t.Errorf("claude %s has exited", claude.Argv[1])
		}
	}
	dirs.checkCompletions(t, s, notUTF8)
	dirs.checkTaken(t, s)
}

// apiRepositories are the directories of TestSessionOfAnotherRepository whose NAME is api: the
// repositories work, by a symbolic link linked, and other; a subdirectory and a linked worktree of
// work; and plain, in no repository.
type apiRepositories struct {
	work, linked, other, sub, worktree, plain string
}

// makeAPIRepositories makes the directories, in the sandbox's root.
func makeAPIRepositories(t *testing.T, s *sandbox.Sandbox) apiRepositories {
	t.Helper()
	d := apiRepositories{
		work:   filepath.Join(s.Root, "work", "api"),
		other:  filepath.Join(s.Root, "scratch", "api"),
		plain:  filepath.Join(s.Root, "pla\tin", "api"),
		linked: filepath.Join(s.Root, "link", "api"),
	}
	for _, dir := range []string{d.work, d.other} {
		runGit(t, s, s.Root, "init", "-q", dir)
	}
	runGit(t, s, d.work, "commit", "-q", "--allow-empty", "-m", "first")
	runGit(t, s, d.work, "worktree", "add", "-q", "-b", "worktree-x",
		filepath.Join(".claude", "worktrees", "x"))
	d.worktree = filepath.Join(d.work, ".claude", "worktrees", "x")
	d.sub = filepath.Join(d.work, "sub")
	for _, dir := range []string{d.plain, d.sub} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(s.Root, "work"), filepath.Dir(d.linked)); err != nil {
		t.Fatal(err)
	}
	return d
}

// startSessions starts api-0 by the link, as PWD names it, api-1 in other, an older cld's api-2,
// and api-3 in plain, checks the homes join records, and returns the claudes it started.
func (d apiRepositories) startSessions(t *testing.T, s *sandbox.Sandbox) []*sandbox.Probe {
	t.Helper()
	startCldIn(t, s, "tmux", d.linked, map[string]string{"PWD": d.linked}, "join")
	s.WaitProbes(1)
	startCldIn(t, s, "tmux", d.other, nil, "join")
	s.WaitProbes(2)
	// An older cld's server: no @cld-home, and no @cld, but its prefix, C-q, marks it as cld's
	// (see TestLeavesAForeignServerAlone).
	s.MustTmux("cld-api-2", "-f", "/dev/null", "set", "-g", "prefix", "C-q", ";",
		"new-session", "-d", "-s", "cld-api-2", "-c", s.Work, "sleep", "600")
	startCldIn(t, s, "tmux", d.plain, nil, "join")
	claudes := s.WaitProbes(3)
	waitClients(t, s, 3)
	homes := map[string]string{"cld-api-0": d.linked, "cld-api-1": d.other, "cld-api-3": d.plain}
	for session, home := range homes {
		made := s.MustTmux(session, "show", "-v", "-t", "="+session+":", "@cld-home")
		if made != home {
			t.Errorf("%s's @cld-home is %q, want %q", session, made, home)
		}
	}
	return claudes
}

// checkRefused checks, with the variables env, that join, kill and detach refuse each session
// from a directory other than its home.
func (d apiRepositories) checkRefused(t *testing.T, s *sandbox.Sandbox, env map[string]string) {
	t.Helper()
	for _, test := range []struct {
		dir  string
		args []string
		want string
	}{
		{d.other, []string{"join", "-s", "0"}, belongsElsewhere("0", d.linked, "repository", "join")},
		{d.other, []string{"kill", "-s", "0"}, belongsElsewhere("0", d.linked, "repository", "kill")},
		{d.other, []string{"detach", "-s", "0"}, belongsElsewhere("0", d.linked, "repository", "detach")},
		{d.plain, []string{"kill", "-s", "0"}, belongsElsewhere("0", d.linked, "directory", "kill")},
		{d.work, []string{"join", "-s", "1"}, belongsElsewhere("1", d.other, "repository", "join")},
		{d.sub, []string{"kill", "-s", "3"}, belongsElsewhere("3", d.plain, "repository", "kill")},
		{d.other, []string{"join", "-s", "0", "--new"},
			belongsElsewhere("0", d.linked, "repository", "join")},
		{d.other, []string{"join", "-s", "0", "--resume", "x"},
			belongsElsewhere("0", d.linked, "repository", "join")},
		{d.other, []string{"join", "-s", "2", "--new"}, lostRefusal("api-2", "--new", "-n api -s 2")},
	} {
		result := s.RunCldIn(test.dir, env, test.args...)
		if result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s in %s, environment %q: exit %d, stdout %q, stderr %q, "+
				"want exit 1, stderr %q", strings.Join(test.args, " "), test.dir, env,
				result.Code, result.Stdout, result.Stderr, test.want)
		}
	}
}

// belongsElsewhere is the refusal of session api-SUFFIX, made in home, by command run in another
// repository or directory, as place says.
func belongsElsewhere(suffix, home, place, command string) string {
	return "cld: session 'api-" + suffix + "' belongs to " + home + ", not to this " + place +
		"; name it with cld " + command + " -n api -s " + suffix + "\n"
}

// checkCompletions checks the sessions join -s and detach -s offer in each directory, with the
// sandbox's variables and with notUTF8.
func (d apiRepositories) checkCompletions(
	t *testing.T, s *sandbox.Sandbox, notUTF8 map[string]string,
) {
	t.Helper()
	for _, test := range []struct {
		dir  string
		args []string
		want string
	}{
		{d.work, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{d.sub, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{d.worktree, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{d.other, []string{"-s", ""}, "1\tattached\n2\tdetached\n:4\n"},
		{d.plain, []string{"-s", ""}, "2\tdetached\n3\tattached\n:4\n"},
		{d.other, []string{"-n", "api", "-s", ""},
			"0\tattached\n1\tattached\n2\tdetached\n3\tattached\n:4\n"},
		{"/", []string{"-s", "api"},
			"api-0\tattached\napi-1\tattached\napi-2\tdetached\napi-3\tattached\n:4\n"},
	} {
		for _, command := range []string{"join", "detach"} {
			args := append([]string{"__complete", command}, test.args...)
			for _, env := range []map[string]string{nil, notUTF8} {
				result := s.RunCldIn(test.dir, env, args...)
				if result.Code != 0 || result.Stdout != test.want {
					t.Errorf("%s %q in %s, environment %q: exit %d, stdout\n%s\nwant\n%s",
						command, test.args, test.dir, env, result.Code, result.Stdout, test.want)
				}
			}
		}
	}
}

// checkTaken checks that a worktree's join, -n, the root directory and an older cld's session
// take a session from anywhere, ending each.
func (d apiRepositories) checkTaken(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	startCldIn(t, s, "tmux", d.worktree, nil, "join", "-s", "0")
	startCldIn(t, s, "tmux", d.other, nil, "join", "-n", "api", "-s", "3")
	sandbox.WaitFor(t, 10*time.Second, "the joins to attach", func() bool {
		return slices.Equal(s.Clients(),
			[]string{"cld-api-0", "cld-api-0", "cld-api-1", "cld-api-3", "cld-api-3"})
	})
	for _, test := range []struct {
		dir  string
		args []string
	}{
		{d.worktree, []string{"detach", "-s", "0"}},
		{d.other, []string{"detach", "-n", "api", "-s", "3"}},
		{d.other, []string{"kill", "-s", "2"}},
		{d.sub, []string{"kill", "-s", "0"}},
		{d.other, []string{"kill", "-n", "api", "-s", "3"}},
		{"/", []string{"kill", "-s", "api-1"}},
	} {
		result := s.RunCldIn(test.dir, nil, test.args...)
		if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
			t.Errorf("%s in %s: exit %d, stdout %q, stderr %q, want exit 0 and no output",
				strings.Join(test.args, " "), test.dir, result.Code, result.Stdout, result.Stderr)
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "the sessions to end", func() bool {
		return len(s.Sessions()) == 0
	})
}
