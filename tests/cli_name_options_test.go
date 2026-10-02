package tests

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The options that name a session, -n and -s, and join's options that go with them.

// invalidName is the message for a NAME or SUFFIX, as kind says, that is no name.
func invalidName(kind, value string) string {
	return "cld: invalid " + kind + " '" + value + "' (see cld help)\n"
}

// longName is the message for a NAME or SUFFIX, as kind says, of more than 64 characters.
func longName(kind, value string) string {
	return "cld: " + kind + " '" + value + "' is longer than 64 characters (see cld help)\n"
}

// sessionTooLong is the message for a session's name, NAME-SUFFIX, of more than 64 characters.
func sessionTooLong(name string) string {
	return "cld: session name '" + name + "' is longer than 64 characters; " +
		"give a shorter -n NAME or -s SUFFIX (see cld help)\n"
}

// forkOwnName is the message for a --fork whose copy would take the name of SESSION, name.
func forkOwnName(name string) string {
	return "cld: join: --fork would give the copy SESSION's own name, " + name +
		"; give another -s SUFFIX (see cld help)\n"
}

// The messages for join's options that go only with others, or never together.
const (
	forkNeedsResume = "cld: join: --fork needs --resume SESSION, the conversation to copy " +
		"(see cld help)\n"
	resumeNeedsValue = "cld: option '--resume' needs a value (see cld help)\n"
	newWithResume    = "cld: join: --new and --resume exclude each other: " +
		"each says which conversation claude starts with (see cld help)\n"
	worktreeWithResume = "cld: join: -w and --resume exclude each other: " +
		"claude takes a conversation back to its worktree itself (see cld help)\n"
)

// readAsOption is the message for a SESSION that claude would read as an option.
func readAsOption(session string) string {
	return "cld: invalid SESSION '" + session + "' for --resume: " +
		"claude would read it as an option (see cld help)\n"
}

// tooLongSuffix is a SUFFIX of 65 characters.
var tooLongSuffix = strings.Repeat("s", 65)

// nameOptionCases are command lines with a mistake in the session's name or join's options, and
// the message.
var nameOptionCases = []struct {
	args []string
	want string
}{
	{[]string{"kill"}, "cld: kill: missing -s SUFFIX (see cld list)\n"},
	{[]string{"kill", "--name", "x"}, "cld: kill: missing -s SUFFIX (see cld list)\n"},
	{[]string{"detach"}, "cld: detach: missing -s SUFFIX (see cld list)\n"},
	{[]string{"detach", "-n", "x"}, "cld: detach: missing -s SUFFIX (see cld list)\n"},
	{[]string{"detach", "-s", "a.b"}, invalidName("suffix", "a.b")},
	{[]string{"join", "--fork"}, forkNeedsResume},
	{[]string{"join", "-s", "x", "--fork"}, forkNeedsResume},
	{[]string{"join", "--fork", "-n", "x"}, forkNeedsResume},
	{[]string{"join", "--fork", "-s", "a.b"}, invalidName("suffix", "a.b")},
	{[]string{"join", "-n", "a", "-s", "x", "--fork", "--resume", "cld-a-x"}, forkOwnName("cld-a-x")},
	{[]string{"join", "-s", "x", "-n", "A", "--fork", "--resume", " CLD-a-X "},
		forkOwnName("cld-A-x")},
	{[]string{"join", "--resume"}, resumeNeedsValue},
	{[]string{"join", "--resume", ""}, resumeNeedsValue},
	{[]string{"join", "--resume=", "-s", "x"}, resumeNeedsValue},
	{[]string{"join", "--resume", "-p"}, readAsOption("-p")},
	{[]string{"join", "--resume=-", "--fork"}, readAsOption("-")},
	{[]string{"join", "--switched-from", "a.b"},
		"cld: invalid session 'a.b' for --switched-from (see cld help)\n"},
	{[]string{"join", "--moved=" + moved("/"), "-s", "y"},
		"cld: join: --moved goes with --switched-from alone (see cld help)\n"},
	{[]string{"join", "--moved=!"}, "cld: invalid value '!' for --moved (see cld help)\n"},
	{[]string{"join", "--switched-from", "a", "--moved=" + moved("_", "-s", "x")},
		"cld: invalid value '" + moved("_", "-s", "x") + "' for --moved (see cld help)\n"},
	{[]string{"join", "--switched-from", "a", "--moved=" + moved("/", "-s", "a.b")},
		invalidName("suffix", "a.b")},
	{[]string{"list", "--to", "next"}, "cld: list: --to needs --switch CLIENT (see cld help)\n"},
	{[]string{"list", "--switch", "/dev/pts/0", "--to", "up"},
		"cld: invalid value 'up' for --to: previous, next or last (see cld help)\n"},
	{[]string{"list", "--switch", ""}, "cld: option '--switch' needs a value (see cld help)\n"},
	{[]string{"join", "--new", "--resume", "x"}, newWithResume},
	{[]string{"join", "--resume", "x", "-w", "-s", "y"}, worktreeWithResume},
	{[]string{"join", "--resume", "x", "--fork", "--new"}, newWithResume},
	{[]string{"kill", "-n", "", "-s", ""}, invalidName("name", "")},
	{[]string{"join", "-n", "a.b"}, invalidName("name", "a.b")},
	{[]string{"join", "-n", "x", "-s", "a b"}, invalidName("suffix", "a b")},
	{[]string{"join", "-s", ""}, invalidName("suffix", "")},
	{[]string{"join", "-s", " "}, invalidName("suffix", " ")},
	{[]string{"join", "-s", "a b", "--resume", "-p"}, invalidName("suffix", "a b")},
	{[]string{"join", "-s", "-x"}, invalidName("suffix", "-x")},
	{[]string{"join", "--suffix=_x"}, invalidName("suffix", "_x")},
	{[]string{"kill", "-s", "a.b"}, invalidName("suffix", "a.b")},
	{[]string{"join", "-s", "café", "--resume", "SESSION"}, invalidName("suffix", "café")},
	{[]string{"join", "-s", tooLongSuffix}, longName("suffix", tooLongSuffix)},
	{[]string{"join", "-s", tooLongSuffix[:60] + "a.b.c"},
		longName("suffix", tooLongSuffix[:60]+"a.b.c")},
	{[]string{"join", "-s"}, "cld: option '-s' needs a value (see cld help)\n"},
	{[]string{"join", "--suffix"}, "cld: option '--suffix' needs a value (see cld help)\n"},
	{[]string{"join", "-n", strings.Repeat("n", 62), "-s", "ab"},
		sessionTooLong(strings.Repeat("n", 62) + "-ab")},
	{[]string{"kill", "-s", strings.Repeat("s", 30), "-n", strings.Repeat("n", 34)},
		sessionTooLong(strings.Repeat("n", 34) + "-" + strings.Repeat("s", 30))},
}

// Mistakes with -n, -s and join's options exit with status 2 before any tool is looked for: the
// PATH has none here. Both kill and detach, outside cld's panes, need -s. Names are checked -n
// first, each by its length and then its characters (decision 24.2). Decisions 50.2 and 45.1 say
// what join's --resume and --fork take; TestForkOwnName has the names a default makes.
func TestNameOptions(t *testing.T) {
	t.Parallel()
	for _, test := range nameOptionCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...), 2, test.want)
		})
	}
}

// moved is the value of join's --moved for a join run in directory dir with words after it (see
// TestJoinMovesTheTerminal). That value holds dir and the words, each after a NUL, in URL-safe
// base64 without padding.
func moved(dir string, words ...string) string {
	value := strings.Join(append([]string{dir}, words...), "\x00")
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
