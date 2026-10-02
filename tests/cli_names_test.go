package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The names cld takes for a session: their characters and their length.

// A name is refused, not sanitised (decision 1): tmux would rename "." and ":" to "_", and a space
// would split claude's arguments.
func TestRejectsInvalidNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "a b", "foo.bar", "a:b", "x/y", "-x", "_x", "café", "a\nb"} {
		for _, args := range [][]string{
			{"join", "-n", name}, {"join", "--resume", "x", "-n", name}, {"join", "--name", name},
			{"detach", "-n", name}, {"kill", "-n", name}, {"join", "--name=" + name},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCld(nil, args...)
				if result.Code != 2 || !strings.Contains(result.Stderr, "invalid name") {
					t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
				}
				if result.Stdout != "" {
					t.Errorf("printed %q before failing", result.Stdout)
				}
			})
		}
	}
}

// Names are ASCII whatever the locale (decision 11.8). Under en_US.UTF-8, glibc's bash took é, ß
// or ① for a letter or digit in [A-Za-z0-9], so the script took such names. See the environment
// findings, docs/design/findings/environment.md.
func TestNamesAreASCII(t *testing.T) {
	t.Parallel()
	locale := map[string]string{"LC_ALL": "en_US.UTF-8"}
	for _, name := range []string{"café", "ß", "①", "٣"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			want := invalidName("name", name)
			if result := s.RunCld(locale, "join", "-n", name); result.Code != 2 || result.Stderr != want {
				t.Errorf("join -n %s: exit %d, stderr %q, want exit 2, stderr %q",
					name, result.Code, result.Stderr, want)
			}
			want = "cld: unknown command '" + name + "' (see cld help)\n"
			if result := s.RunCld(locale, name); result.Code != 2 || result.Stderr != want {
				t.Errorf("%s: exit %d, stderr %q, want exit 2, stderr %q",
					name, result.Code, result.Stderr, want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
		})
	}
}

// A name has at most 64 characters (decision 13.2): one of 65 is refused, saying so, whatever its
// characters. One of 64 goes as far as tmux, whose server is named like the session. The length
// counts characters, not bytes: 33 "é" are 66 bytes, and invalid for the "é".
func TestNameLength(t *testing.T) {
	t.Parallel()
	longest, tooLong := strings.Repeat("n", 64), strings.Repeat("n", 65)
	accents, tooManyAccents := strings.Repeat("é", 33), strings.Repeat("é", 65)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-n", tooLong}, longName("name", tooLong)},
		{[]string{"join", "--name", tooLong}, longName("name", tooLong)},
		{[]string{"kill", "-n", tooLong}, longName("name", tooLong)},
		{[]string{"detach", "--name", tooLong}, longName("name", tooLong)},
		{[]string{"join", "-n", tooLong, "--resume", "SESSION"}, longName("name", tooLong)},
		{[]string{"join", "-n", tooLong[:60] + "a.b.c"}, longName("name", tooLong[:60]+"a.b.c")},
		{[]string{"join", "-n", tooLong[:60] + "a.b"}, invalidName("name", tooLong[:60]+"a.b")},
		{[]string{"join", "-n", accents}, invalidName("name", accents)},
		{[]string{"join", "-n", tooManyAccents}, longName("name", tooManyAccents)},
		{[]string{tooLong}, "cld: unknown command '" + tooLong + "' (see cld help)\n"},
		{[]string{longest}, "cld: unknown command '" + longest + "' (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, test.args...), 2, test.want)
		})
	}
	// Where the directory's name leaves nothing, -s SUFFIX is the whole name.
	for _, args := range [][]string{
		{"join", "-s", longest}, {"join", "-s", longest, "--resume", "SESSION"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCldOnTerminal(fakeTmuxEnv(s, nil), args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			checkTmuxServer(t, s, longest)
		})
	}
}

// checkTmuxServer reports a command of the fake tmux's that is not for server cld-NAME.
func checkTmuxServer(t *testing.T, s *sandbox.Sandbox, name string) {
	t.Helper()
	if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:3], []string{"-u", "-L", "cld-" + name}) {
		t.Errorf("tmux arguments start %q, want -u -L cld-%s", argv[:3], name)
	}
}

// longPrefixCases are command lines run in a repository or a directory whose name is length
// characters long, and the session's name they make with suffix, ok where it fits.
var longPrefixCases = []struct {
	length     int
	repository bool
	args       []string
	suffix     string
	ok         bool
}{
	{62, true, []string{"join"}, "-0", true},
	{63, true, []string{"join"}, "-0", false},
	{63, true, []string{"join", "--resume", "SESSION"}, "-0", false},
	{60, true, []string{"join", "-s", "abc"}, "-abc", true},
	{60, true, []string{"join", "-s", "abcd"}, "-abcd", false},
	{60, true, []string{"join", "-s", "abcd"}, "-abcd", false},
	{60, true, []string{"kill", "-s", "abcd"}, "-abcd", false},
	{60, true, []string{"detach", "-s", "abcd"}, "-abcd", false},
	{62, false, []string{"join"}, "-0", true},
	{63, false, []string{"join"}, "-0", false},
	{60, false, []string{"join", "-s", "abcd"}, "-abcd", false},
	// -n's NAME with the index: the repository's name no longer counts.
	{63, true, []string{"join", "-n", strings.Repeat("n", 62)}, "", true},
	{60, true, []string{"join", "-n", strings.Repeat("n", 63)}, "", false},
}

// A name that the repository's or the directory's name, or join's index, makes with -n or -s has
// at most 64 characters too. A longer one is refused with status 1, pointing at -n and -s
// (decision 24.2). The fake tmux finds no server, and gets a name of 64 characters.
func TestLongPrefix(t *testing.T) {
	t.Parallel()
	for _, test := range longPrefixCases {
		repository := strings.Repeat("r", test.length)
		name := repository + test.suffix
		if test.suffix == "" {
			name = test.args[len(test.args)-1] + "-0"
		}
		where := "repository"
		if !test.repository {
			where = "directory"
		}
		t.Run(where+" of "+strconv.Itoa(test.length)+", "+strings.Join(test.args, " "),
			func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				if test.repository {
					runGit(t, s, s.Root, "init", "-q", repository)
				} else if err := os.Mkdir(filepath.Join(s.Root, repository), 0o755); err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(s.Root, repository)
				result := s.RunCldOnTerminalIn(dir, fakeTmuxEnv(s, nil), test.args...)
				if test.ok {
					if result.Code != 0 || result.Stderr != "" {
						t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
					}
					checkTmuxServer(t, s, name)
					return
				}
				checkFailed(t, result, 1, sessionTooLong(name))
				if fakeTmuxRan(s) {
					t.Error("tmux started")
				}
			})
	}
}
