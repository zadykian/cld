package tests

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Everything cld does before it hands over to tmux; no terminal needed.

var update = flag.Bool("update", false, "rewrite the help in testdata/help from what cld help prints")

// helpTopics are what cld help takes, "" for none, in the order the help lists them: a command
// of cld's, followed by the commands it has, "setup telemetry" for setup's telemetry.
var helpTopics = []string{"", "new", "resume", "join", "kill", "list", "setup", "setup telemetry", "completion", "help", "version"}

// goldenHelp is the file holding what cld help topic prints: testdata/help/cld.txt for cld help,
// testdata/help/COMMAND.txt for cld help COMMAND, and setup-telemetry.txt for cld help setup
// telemetry.
func goldenHelp(topic string) string {
	if topic == "" {
		topic = "cld"
	}
	return filepath.Join("testdata", "help", strings.ReplaceAll(topic, " ", "-")+".txt")
}

// subtopics are the commands the help of topic lists: the topics that name one after it.
func subtopics(topic string) []string {
	var names []string
	for _, other := range helpTopics[1:] {
		parent, name := "", other
		if last := strings.LastIndex(other, " "); last >= 0 {
			parent, name = other[:last], other[last+1:]
		}
		if parent == topic {
			names = append(names, name)
		}
	}
	return names
}

// The help of cld and of each command, byte for byte: cobra generates it from each command's texts
// and options, with its default templates, so a change to either shows here. -update rewrites
// the files. cobra wraps nothing, so the texts break their lines by hand, within 80 columns, and
// a usage line names the options before the arguments, as cld reads them.
func TestHelpText(t *testing.T) {
	t.Parallel()
	for _, topic := range helpTopics {
		args := append([]string{"help"}, strings.Fields(topic)...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(nil, args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			if *update {
				if err := os.WriteFile(goldenHelp(topic), []byte(result.Stdout), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenHelp(topic))
			if err != nil {
				t.Fatal(err)
			}
			if result.Stdout != string(want) {
				t.Errorf("stdout\n%s\nwant, as in %s\n%s", result.Stdout, goldenHelp(topic), want)
			}
			for number, line := range strings.Split(result.Stdout, "\n") {
				if width := utf8.RuneCountInString(line); width > 80 {
					t.Errorf("line %d is %d columns wide: %q", number+1, width, line)
				}
			}
			// cld reads a command's options only up to its first argument.
			if late := optionsAfterArguments(result.Stdout); len(late) > 0 {
				t.Errorf("the usage line names %q after an argument, where cld reads no options", late)
			}
			// A command added to cld, or to one of its commands, gets its own file here. completion's
			// commands are cobra's, whose help is cobra's too (see TestCompletionScripts).
			if listed, want := listedCommands(result.Stdout), subtopics(topic); topic != "completion" && !slices.Equal(listed, want) {
				t.Errorf("the help lists %q, want %q: one file in testdata/help for each", listed, want)
			}
		})
	}
}

// listedCommands are the commands the help of cld lists, in its order.
func listedCommands(help string) []string {
	_, listing, _ := strings.Cut(help, "\nAvailable Commands:\n")
	listing, _, _ = strings.Cut(listing, "\n\n")
	var names []string
	for _, line := range strings.Split(listing, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// usageWord is a word of a usage line: one in brackets, such as [-n NAME], or a plain one.
var usageWord = regexp.MustCompile(`\[[^]]*\]|\S+`)

// optionsAfterArguments are the options that the usage lines of help name after an argument:
// [flags], or one in brackets starting with "-". An argument is a word in brackets or in
// capitals, such as [COMMAND]; the others name cld and its command.
func optionsAfterArguments(help string) []string {
	_, usage, _ := strings.Cut(help, "\nUsage:\n")
	usage, _, _ = strings.Cut(usage, "\n\n")
	var late []string
	for _, line := range strings.Split(usage, "\n") {
		argument := false
		for _, word := range usageWord.FindAllString(line, -1) {
			switch {
			case word == "[flags]" || strings.HasPrefix(word, "[-"):
				if argument {
					late = append(late, word)
				}
			case strings.HasPrefix(word, "[") || strings.ToUpper(word) == word:
				argument = true
			}
		}
	}
	return late
}

// help, -h and --help print the help of cld, or of the command they are given to: the command
// after them, or the one before -h. -h before a wrong argument shows the help too, since
// arguments are read left to right. completion alone shows its help, as cobra's does.
func TestHelp(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args  []string
		topic string
	}{
		{[]string{"help"}, ""},
		{[]string{"-h"}, ""},
		{[]string{"--help"}, ""},
		{[]string{"help", "new"}, "new"},
		{[]string{"help", "resume"}, "resume"},
		{[]string{"-h", "join"}, "join"},
		{[]string{"--help", "version"}, "version"},
		{[]string{"help", "help"}, "help"},
		{[]string{"new", "--help"}, "new"},
		{[]string{"resume", "--help"}, "resume"},
		{[]string{"resume", "-n", "x", "-h"}, "resume"},
		{[]string{"resume", "-h", "a", "b"}, "resume"},
		{[]string{"join", "-n", "x", "-h"}, "join"},
		{[]string{"kill", "--help"}, "kill"},
		{[]string{"list", "-h"}, "list"},
		{[]string{"help", "-h"}, "help"},
		{[]string{"version", "--help"}, "version"},
		{[]string{"-V", "-h"}, "version"},
		{[]string{"--version", "--help"}, "version"},
		{[]string{"new", "-h", "-x"}, "new"},
		{[]string{"join", "-h", "-w"}, "join"},
		{[]string{"new", "-h", "--help=x"}, "new"},
		{[]string{"list", "-h", "-h=no"}, "list"},
		{[]string{"join", "--help", "--help=maybe"}, "join"},
		{[]string{"kill", "-h", "a"}, "kill"},
		{[]string{"help", "-h", "nope"}, "help"},
		{[]string{"help", "--help", "-x"}, "help"},
		{[]string{"completion"}, "completion"},
		{[]string{"completion", "--help"}, "completion"},
		{[]string{"-h", "completion"}, "completion"},
		{[]string{"completion", "-h", "tcsh"}, "completion"},
		{[]string{"completion", "-h", "--help=x"}, "completion"},
		{[]string{"completion", "--help", "-h=no"}, "completion"},
		// setup's commands: after it, or after help setup; -h after setup is setup's.
		{[]string{"help", "setup"}, "setup"},
		{[]string{"-h", "setup"}, "setup"},
		{[]string{"setup", "-h"}, "setup"},
		{[]string{"setup", "--help"}, "setup"},
		{[]string{"setup", "-h", "x"}, "setup"},
		{[]string{"help", "setup", "telemetry"}, "setup telemetry"},
		{[]string{"--help", "setup", "telemetry"}, "setup telemetry"},
		{[]string{"setup", "-h", "telemetry"}, "setup telemetry"},
		{[]string{"setup", "telemetry", "-h"}, "setup telemetry"},
		{[]string{"setup", "telemetry", "--help"}, "setup telemetry"},
		{[]string{"setup", "telemetry", "-h", "x"}, "setup telemetry"},
		{[]string{"setup", "telemetry", "--local", "x", "-h"}, "setup telemetry"},
		{[]string{"setup", "telemetry", "-h", "--bogus"}, "setup telemetry"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(goldenHelp(test.topic))
			if err != nil {
				t.Fatal(err)
			}
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 0 || result.Stdout != string(want) || result.Stderr != "" {
				t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout as in %s", result.Code, result.Stderr, result.Stdout, goldenHelp(test.topic))
			}
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, command := range []string{"version", "-V", "--version"} {
		if result := s.RunCld(nil, command); result.Code != 0 || result.Stdout != "cld dev\n" {
			t.Errorf("cld %s: exit %d, stdout %q", command, result.Code, result.Stdout)
		}
	}
}

// completion SHELL prints cobra's completion script for SHELL, which asks cld __complete what to
// offer on every TAB, or __completeNoDesc with --no-descriptions. Its help is cobra's, which says
// where the script goes and what it needs; -h and --help are cld's, and win over a wrong value
// for either after them. completion alone shows its help, as in testdata/help. None of them needs
// tmux or claude.
func TestCompletionScripts(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	for _, test := range []struct{ shell, start, setup string }{
		{"bash", "# bash completion V2 for cld ", "This script depends on the 'bash-completion' package."},
		{"zsh", "#compdef cld\n", "autoload -U compinit; compinit"},
		{"fish", "# fish completion for cld ", "cld completion fish > ~/.config/fish/completions/cld.fish"},
	} {
		for _, args := range [][]string{{"completion", test.shell}, {"completion", test.shell, "--no-descriptions"}} {
			result := s.RunCld(none, args...)
			noDescriptions := len(args) == 3
			if result.Code != 0 || !strings.HasPrefix(result.Stdout, test.start) || result.Stderr != "" ||
				strings.Contains(result.Stdout, " __completeNoDesc ") != noDescriptions {
				t.Errorf("cld %q: exit %d, stderr %q, stdout\n%.300s", args, result.Code, result.Stderr, result.Stdout)
			}
		}
		for _, args := range [][]string{{"completion", test.shell, "--help"}, {"completion", test.shell, "-h", "-x"},
			{"completion", test.shell, "-h", "--help=x"}, {"completion", test.shell, "--help", "-h=no"},
			{"help", "completion", test.shell}} {
			result := s.RunCld(none, args...)
			if help := "Generate the autocompletion script for the " + test.shell + " shell.\n"; result.Code != 0 ||
				!strings.HasPrefix(result.Stdout, help) || !strings.Contains(result.Stdout, test.setup) || result.Stderr != "" {
				t.Errorf("cld %q: exit %d, stderr %q, stdout\n%s", args, result.Code, result.Stderr, result.Stdout)
			}
		}
	}
	want, err := os.ReadFile(goldenHelp("completion"))
	if err != nil {
		t.Fatal(err)
	}
	if result := s.RunCld(none, "completion"); result.Code != 0 || result.Stdout != string(want) || result.Stderr != "" {
		t.Errorf("cld completion: exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout as in %s", result.Code, result.Stderr, result.Stdout, goldenHelp("completion"))
	}
}

// __complete offers the commands, completion and setup among them, and the commands help takes -
// after setup or completion, theirs - each with its description; the options; and no file names
// where nothing is offered (":4", ShellCompDirectiveNoFileComp, which cobra reports on stderr),
// the root's default for an argument with nothing to complete, setup telemetry's included.
// Without the word to complete it fails, as cld's other command-line mistakes do.
func TestCompleteCommands(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	commands := "new\tcreate session NAME in the current directory and attach to it\n" +
		"resume\tcreate session NAME with claude resuming its conversation\n" +
		"join\tattach to session NAME, detaching any other terminal from it\n" +
		"kill\tend session NAME and its tmux server\n" +
		"list\tlist the sessions cld started; on a terminal, join or kill one\n" +
		"setup\tset up what claude runs with: so far, its telemetry\n" +
		"version\tshow the version\n" +
		"completion\tprint the completion script for a shell\n" +
		"help\tshow this help, or the help of COMMAND\n"
	telemetry := "telemetry\tsend claude's telemetry through a local OpenTelemetry collector\n"
	shells := "bash\tprint the completion script for bash\n" +
		"zsh\tprint the completion script for zsh\n" +
		"fish\tprint the completion script for fish\n" +
		"powershell\tprint the completion script for powershell\n"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"__complete", ""}, commands + ":4\n"},
		{[]string{"__complete", "c"}, "completion\tprint the completion script for a shell\n:4\n"},
		{[]string{"__complete", "help", ""}, commands + ":4\n"},
		{[]string{"__complete", "help", "j"}, "join\tattach to session NAME, detaching any other terminal from it\n:4\n"},
		{[]string{"__complete", "help", "x"}, ":4\n"},
		{[]string{"__complete", "help", "new", ""}, ":4\n"},
		// After a command with commands of its own, help takes one of those, and nothing after it.
		{[]string{"__complete", "help", "setup", ""}, telemetry + ":4\n"},
		{[]string{"__complete", "help", "setup", "t"}, telemetry + ":4\n"},
		{[]string{"__complete", "help", "setup", "x"}, ":4\n"},
		{[]string{"__complete", "help", "setup", "telemetry", ""}, ":4\n"},
		{[]string{"__complete", "help", "completion", ""}, shells + ":4\n"},
		{[]string{"__complete", "help", "nope", ""}, ":4\n"},
		{[]string{"__complete", "completion", ""}, shells + ":4\n"},
		{[]string{"__complete", "setup", ""}, telemetry + ":4\n"},
		// No URL, port or file name is offered, --collector-config's FILE included.
		{[]string{"__complete", "setup", "telemetry", "--l"}, "--local\twhere traces, metrics and logs go, such as\n:4\n"},
		{[]string{"__complete", "setup", "telemetry", "--local", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--port", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--collector-config", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--remote", "https://otel.example.com:4317", ""}, ":4\n"},
		{[]string{"__complete", "completion", "bash", ""}, ":4\n"},
		// cobra describes an option by the first line of its usage, backquotes included.
		{[]string{"__complete", "join", "-"}, "--help\thelp for join\n-h\thelp for join\n" +
			"--name\tthe session `NAME`: up to 64 letters, digits, \"_\" and \"-\",\n" +
			"-n\tthe session `NAME`: up to 64 letters, digits, \"_\" and \"-\",\n:4\n"},
		{[]string{"__complete", "new", "-n", ""}, ":4\n"},
		{[]string{"__complete", "list", ""}, ":4\n"},
	} {
		result := s.RunCld(none, test.args...)
		if stderr := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"; result.Code != 0 || result.Stdout != test.want || result.Stderr != stderr {
			t.Errorf("cld %q: exit %d, stderr %q, stdout\n%s\nwant exit 0, stderr %q, stdout\n%s", test.args, result.Code, result.Stderr, result.Stdout, stderr, test.want)
		}
	}
	for _, command := range []string{"__complete", "__completeNoDesc"} {
		result := s.RunCld(none, command)
		if want := "cld: " + command + ": missing the word to complete (see cld help)\n"; result.Code != 2 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("cld %s: exit %d, stdout %q, stderr %q, want exit 2, stderr %q", command, result.Code, result.Stdout, result.Stderr, want)
		}
	}
}

func TestRequiresCommand(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, args := range [][]string{{}, {""}, {"", "new"}} {
		result := s.RunCld(nil, args...)
		if result.Code != 2 || !strings.HasPrefix(result.Stderr, "cld: missing command") || result.Stdout != "" {
			t.Errorf("cld %q: exit %d, stdout %q, stderr %q", args, result.Code, result.Stdout, result.Stderr)
		}
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// "cld NAME" created or attached to session NAME before cld had commands; it now fails, naming
// the commands that do either. The first argument is the command, whatever follows: an option
// before it is no command either. -v is a command only elsewhere.
func TestRejectsUnknownCommands(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"review"}, "cld: unknown command 'review'; for session review: cld new -n review, cld join -n review\n"},
		{[]string{"-x"}, "cld: unknown command '-x' (see cld help)\n"},
		{[]string{"a.b"}, "cld: unknown command 'a.b' (see cld help)\n"},
		{[]string{"-v"}, "cld: unknown command '-v' (see cld help)\n"},
		{[]string{"-n", "x", "new"}, "cld: unknown command '-n' (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
		})
	}
}

func TestRejectsInvalidNames(t *testing.T) {
	t.Parallel()
	// tmux would rename "." and ":" to "_", a space would split claude's arguments.
	for _, name := range []string{"", "a b", "foo.bar", "a:b", "x/y", "-x", "_x", "café", "a\nb"} {
		for _, args := range [][]string{{"new", "-n", name}, {"resume", "-n", name}, {"join", "--name", name}, {"kill", "-n", name}, {"new", "--name=" + name}} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCld(nil, args...)
				if result.Code != 2 || !strings.Contains(result.Stderr, "invalid session name") {
					t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
				}
				if result.Stdout != "" {
					t.Errorf("printed %q before failing", result.Stdout)
				}
			})
		}
	}
}

// Names are ASCII whatever the locale: under en_US.UTF-8, glibc's bash took é, ß or ① for a
// letter or a digit in [A-Za-z0-9], and cld's shell script took names with them (see Findings in
// docs/design.md).
func TestNamesAreASCII(t *testing.T) {
	t.Parallel()
	locale := map[string]string{"LC_ALL": "en_US.UTF-8"}
	for _, name := range []string{"café", "ß", "①", "٣"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result, want := s.RunCld(locale, "new", "-n", name), "cld: invalid session name '"+name+"' (see cld help)\n"; result.Code != 2 || result.Stderr != want {
				t.Errorf("new -n %s: exit %d, stderr %q, want exit 2, stderr %q", name, result.Code, result.Stderr, want)
			}
			// No legacy hint either: NAME is no session name.
			if result, want := s.RunCld(locale, name), "cld: unknown command '"+name+"' (see cld help)\n"; result.Code != 2 || result.Stderr != want {
				t.Errorf("%s: exit %d, stderr %q, want exit 2, stderr %q", name, result.Code, result.Stderr, want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
		})
	}
}

// A name has at most 64 characters, so that the path of its server's socket fits in sun_path
// (see MaxName in internal/session): one of 65 is refused, saying so, whatever its characters, and
// gets no legacy hint; one of 64 goes as far as tmux, whose server is named like the session, with
// new and resume alike. The length is counted in characters, not bytes: 33 "é" are 66 bytes, and
// invalid for the "é".
func TestNameLength(t *testing.T) {
	t.Parallel()
	longest, tooLong := strings.Repeat("n", 64), strings.Repeat("n", 65)
	accents, tooManyAccents := strings.Repeat("é", 33), strings.Repeat("é", 65)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "-n", tooLong}, "cld: session name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "--name", tooLong}, "cld: session name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"kill", "-n", tooLong}, "cld: session name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"resume", "-n", tooLong, "SESSION"}, "cld: session name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"new", "-n", tooLong[:60] + "a.b.c"}, "cld: session name '" + tooLong[:60] + "a.b.c' is longer than 64 characters (see cld help)\n"},
		{[]string{"new", "-n", tooLong[:60] + "a.b"}, "cld: invalid session name '" + tooLong[:60] + "a.b' (see cld help)\n"},
		{[]string{"new", "-n", accents}, "cld: invalid session name '" + accents + "' (see cld help)\n"},
		{[]string{"new", "-n", tooManyAccents}, "cld: session name '" + tooManyAccents + "' is longer than 64 characters (see cld help)\n"},
		{[]string{tooLong}, "cld: unknown command '" + tooLong + "' (see cld help)\n"},
		{[]string{longest}, "cld: unknown command '" + longest + "'; for session " + longest + ": cld new -n " + longest + ", cld join -n " + longest + "\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
		})
	}
	for _, command := range []string{"new", "resume"} {
		t.Run(command+" -n "+longest, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}, command, "-n", longest)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:2], []string{"-L", "cld-" + longest}) {
				t.Errorf("tmux arguments start %q, want -L cld-%s", argv[:2], longest)
			}
		})
	}
}

// Under a TMUX_TMPDIR longer than tmux's default directories, a name within the 64 characters can
// still make the socket path too long for sun_path. tmux says so, and cld ends with its message,
// rather than take it for no server running: new before it sets the title, and join without
// pointing at cld new, which would fail the same way. list finds no socket.
func TestSocketPathTooLong(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, strings.Repeat("d", 60))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("n", 64)
	path := filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), "cld-"+name)
	want := "cld: error connecting to " + path + " (File name too long)\n"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "-n", name}, want},
		{[]string{"join", "-n", name}, want},
		{[]string{"kill", "-n", name}, want},
		{[]string{"list"}, ""},
	} {
		result := s.RunCld(map[string]string{"TMUX_TMPDIR": dir}, test.args...)
		if code := min(len(test.want), 1); result.Code != code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q", test.args[0], result.Code, result.Stdout, result.Stderr, code, test.want)
		}
	}
}

// list reads tmux's socket directory to find the servers, and one it cannot read ends it with
// the reason, rather than show no session: here a file where tmux-UID would be. new, join and
// kill end with tmux's message, as for a socket path too long (tmux 3.3a to 3.7c). Where
// TMUX_TMPDIR is unset, empty or names nothing, cld reads /tmp/tmux-UID, as tmux falls back to it;
// no test reaches that, which would read the user's own sockets.
func TestUnreadableSocketDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteFile(s.SocketDir(), "")
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "cld: cannot read " + s.SocketDir() + ": not a directory\n"},
		{[]string{"new", "-n", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"join", "-n", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"kill", "-n", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", test.args[0], result.Code, result.Stdout, result.Stderr, test.want)
		}
	}
}

// Arguments are read left to right, and the first wrong one decides the message: an option after
// an argument is not read, and -- ends nothing. help and version are named as typed, completion's
// commands with their SHELL. help takes one argument, a command of cld's; resume takes one,
// SESSION, after its options: never empty, never one claude would take for an option, and nothing
// after it; and completion none but a SHELL, as its command.
func TestRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "review"}, "cld: new: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "-x"}, "cld: join: unexpected argument '-x' (see cld help)\n"},
		{[]string{"new", "-n"}, "cld: option '-n' needs a value (see cld help)\n"},
		{[]string{"join", "--name"}, "cld: option '--name' needs a value (see cld help)\n"},
		{[]string{"kill", "a"}, "cld: kill: unexpected argument 'a' (see cld help)\n"},
		{[]string{"join", "-w"}, "cld: join: unexpected argument '-w' (see cld help)\n"},
		{[]string{"kill", "-n", "a", "--worktree"}, "cld: kill: unexpected argument '--worktree' (see cld help)\n"},
		{[]string{"list", "-n", "a"}, "cld: list: unexpected argument '-n' (see cld help)\n"},
		{[]string{"help", "nope"}, "cld: help: unknown command 'nope' (see cld help)\n"},
		{[]string{"help", "new", "join"}, "cld: help: unexpected argument 'join' (see cld help)\n"},
		{[]string{"help", "nope", "join"}, "cld: help: unknown command 'nope' (see cld help)\n"},
		{[]string{"help", "-V"}, "cld: help: unexpected argument '-V' (see cld help)\n"},
		{[]string{"help", ""}, "cld: help: unknown command '' (see cld help)\n"},
		{[]string{"help", "--", "new"}, "cld: help: unexpected argument '--' (see cld help)\n"},
		{[]string{"help", "new", "--"}, "cld: help: unexpected argument '--' (see cld help)\n"},
		{[]string{"help", "new", "-h"}, "cld: help: unexpected argument '-h' (see cld help)\n"},
		{[]string{"help", "new", "telemetry"}, "cld: help: unexpected argument 'telemetry' (see cld help)\n"},
		{[]string{"help", "setup", "nope"}, "cld: help: unknown command 'setup nope' (see cld help)\n"},
		{[]string{"help", "setup", "telemetry", "x"}, "cld: help: unexpected argument 'x' (see cld help)\n"},
		{[]string{"help", "telemetry"}, "cld: help: unknown command 'telemetry' (see cld help)\n"},
		{[]string{"help", "completion", "tcsh"}, "cld: help: unknown command 'completion tcsh' (see cld help)\n"},
		{[]string{"help", "completion", "bash", "x"}, "cld: help: unexpected argument 'x' (see cld help)\n"},
		{[]string{"-h", "-n", "x"}, "cld: -h: unexpected argument '-n' (see cld help)\n"},
		{[]string{"--help", "x"}, "cld: --help: unknown command 'x' (see cld help)\n"},
		{[]string{"version", "-n", "a"}, "cld: version: unexpected argument '-n' (see cld help)\n"},
		{[]string{"-V", "x"}, "cld: -V: unexpected argument 'x' (see cld help)\n"},
		{[]string{"new", "--"}, "cld: new: unexpected argument '--' (see cld help)\n"},
		{[]string{"join", "--", "-x"}, "cld: join: unexpected argument '--' (see cld help)\n"},
		{[]string{"list", "--"}, "cld: list: unexpected argument '--' (see cld help)\n"},
		{[]string{"join", "a", "-x"}, "cld: join: unexpected argument 'a' (see cld help)\n"},
		{[]string{"new", "review", "-n"}, "cld: new: unexpected argument 'review' (see cld help)\n"},
		{[]string{"new", "review", "-h"}, "cld: new: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "-w", "-h"}, "cld: join: unexpected argument '-w' (see cld help)\n"},
		// An empty argument is one too (cld's shell script took it for none).
		{[]string{"list", ""}, "cld: list: unexpected argument '' (see cld help)\n"},
		{[]string{"resume", "-w"}, "cld: resume: unexpected argument '-w' (see cld help)\n"},
		{[]string{"resume", "-n", "x", "--worktree"}, "cld: resume: unexpected argument '--worktree' (see cld help)\n"},
		{[]string{"resume", "-x"}, "cld: resume: unexpected argument '-x' (see cld help)\n"},
		{[]string{"resume", "--", "-p"}, "cld: resume: unexpected argument '--' (see cld help)\n"},
		{[]string{"resume", "-n", "x", "--", "-p"}, "cld: resume: unexpected argument '--' (see cld help)\n"},
		{[]string{"resume", "a", "b"}, "cld: resume: unexpected argument 'b' (see cld help)\n"},
		{[]string{"resume", "x", "-n", "y"}, "cld: resume: unexpected argument '-n' (see cld help)\n"},
		{[]string{"resume", "x", "-h"}, "cld: resume: unexpected argument '-h' (see cld help)\n"},
		{[]string{"resume", "x", "--"}, "cld: resume: unexpected argument '--' (see cld help)\n"},
		{[]string{"resume", "-n", "x", ""}, "cld: resume: unexpected argument '' (see cld help)\n"},
		{[]string{"resume", "-"}, "cld: resume: unexpected argument '-' (see cld help)\n"},
		// cobra would show its help and exit 0 for an unknown shell, fail with exit status 1 for
		// an argument after it, and read an option after an argument.
		{[]string{"completion", "tcsh"}, "cld: completion: unknown shell 'tcsh' (see cld help)\n"},
		{[]string{"completion", ""}, "cld: completion: unknown shell '' (see cld help)\n"},
		{[]string{"completion", "bash", "x"}, "cld: completion bash: unexpected argument 'x' (see cld help)\n"},
		{[]string{"completion", "zsh", "x", "--bogus"}, "cld: completion zsh: unexpected argument 'x' (see cld help)\n"},
		{[]string{"completion", "fish", "-x"}, "cld: completion fish: unexpected argument '-x' (see cld help)\n"},
		{[]string{"completion", "--no-descriptions", "bash"}, "cld: completion: unexpected argument '--no-descriptions' (see cld help)\n"},
		{[]string{"completion", "--", "bash"}, "cld: completion: unexpected argument '--' (see cld help)\n"},
		{[]string{"completion", "bash", "--"}, "cld: completion bash: unexpected argument '--' (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
		})
	}
}

// setup takes one of its commands, telemetry, as its first argument, which run checks as it checks
// cld's first: no option comes before it (cobra would run telemetry for setup --local URL
// telemetry), and help can be asked for with -h or --help only, as for cld.
func TestSetupRequiresCommand(t *testing.T) {
	t.Parallel()
	const telemetry = "cld setup telemetry runs a collector for claude's telemetry (see cld help)\n"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"setup"}, "cld: setup: missing command: " + telemetry},
		{[]string{"setup", ""}, "cld: setup: missing command: " + telemetry},
		{[]string{"setup", "", "telemetry"}, "cld: setup: missing command: " + telemetry},
		{[]string{"setup", "other"}, "cld: setup: unknown command 'other': " + telemetry},
		{[]string{"setup", "other", "telemetry"}, "cld: setup: unknown command 'other': " + telemetry},
		{[]string{"setup", "--local", "http://127.0.0.1:4319", "telemetry"}, "cld: setup: unknown command '--local': " + telemetry},
		{[]string{"setup", "-x"}, "cld: setup: unknown command '-x': " + telemetry},
		{[]string{"setup", "--help=false", "telemetry"}, "cld: setup: unknown command '--help=false': " + telemetry},
		{[]string{"setup", "--"}, "cld: setup: unknown command '--': " + telemetry},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
}

// setup telemetry's mistakes are usage errors, read left to right as other commands' are: a
// value that is no URL or no port, neither --local nor --remote, a URL where the collector would
// listen with --port, and so send to itself, an argument, an option cld does not know. None gets
// as far as docker.
func TestSetupTelemetryRejectsArguments(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	const (
		needed = "cld: setup telemetry: --local URL, --remote URL or both are needed (see cld help)\n"
		form   = ": http://HOST:PORT or https://HOST:PORT (see cld help)\n"
		port   = "': a number from 1 to 65535 (see cld help)\n"
	)
	collector := func(port string) string {
		return " is where the collector would listen (--port " + port + "): give the receiver's port, or another --port (see cld help)\n"
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, needed},
		{[]string{"--port", "4317"}, needed},
		{[]string{"--collector-config", "extra.yaml"}, needed},
		{[]string{"--local", "127.0.0.1:4319"}, "cld: invalid URL '127.0.0.1:4319' for --local" + form},
		{[]string{"--local", "http://127.0.0.1"}, "cld: invalid URL 'http://127.0.0.1' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:4319/"}, "cld: invalid URL 'http://127.0.0.1:4319/' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:4319/v1/traces"}, "cld: invalid URL 'http://127.0.0.1:4319/v1/traces' for --local" + form},
		{[]string{"--local", "grpc://127.0.0.1:4319"}, "cld: invalid URL 'grpc://127.0.0.1:4319' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:0"}, "cld: invalid URL 'http://127.0.0.1:0' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:65536"}, "cld: invalid URL 'http://127.0.0.1:65536' for --local" + form},
		{[]string{"--local", "http://user@127.0.0.1:4319"}, "cld: invalid URL 'http://user@127.0.0.1:4319' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:4319?x=1"}, "cld: invalid URL 'http://127.0.0.1:4319?x=1' for --local" + form},
		{[]string{"--local", "http://a b:4319"}, "cld: invalid URL 'http://a b:4319' for --local" + form},
		{[]string{"--local", "http://a!b:4319"}, "cld: invalid URL 'http://a!b:4319' for --local" + form},
		{[]string{"--local", "http://[fe80::1%25eth0]:4319"}, "cld: invalid URL 'http://[fe80::1%25eth0]:4319' for --local" + form},
		// A label of a host name starts and ends with a letter or digit, the last is not all digits,
		// and a port has no leading zero.
		{[]string{"--local", "http://-ide:4319"}, "cld: invalid URL 'http://-ide:4319' for --local" + form},
		{[]string{"--local", "http://ide-.local:4319"}, "cld: invalid URL 'http://ide-.local:4319' for --local" + form},
		{[]string{"--local", "http://ide._x:4319"}, "cld: invalid URL 'http://ide._x:4319' for --local" + form},
		{[]string{"--local", "http://a..b:4319"}, "cld: invalid URL 'http://a..b:4319' for --local" + form},
		{[]string{"--local", "http://127.1:4319"}, "cld: invalid URL 'http://127.1:4319' for --local" + form},
		{[]string{"--local", "http://2130706433:4319"}, "cld: invalid URL 'http://2130706433:4319' for --local" + form},
		{[]string{"--local", "http://127.0.0.1:04319"}, "cld: invalid URL 'http://127.0.0.1:04319' for --local" + form},
		{[]string{"--local="}, "cld: invalid URL '' for --local" + form},
		{[]string{"--remote", "otel.example.com:4317"}, "cld: invalid URL 'otel.example.com:4317' for --remote" + form},
		{[]string{"--local", "http://127.0.0.1:4319", "--remote", "https://otel.example.com"}, "cld: invalid URL 'https://otel.example.com' for --remote" + form},
		{[]string{"--remote", "https://otel.example.com:4317", "--port", "0"}, "cld: invalid port '0" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--port", "65536"}, "cld: invalid port '65536" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--port", "x"}, "cld: invalid port 'x" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--port", "-1"}, "cld: invalid port '-1" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--port="}, "cld: invalid port '" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--port", "04317"}, "cld: invalid port '04317" + port},
		// A URL that leads to 127.0.0.1, where the collector listens, on its --port.
		{[]string{"--local", "http://127.0.0.1:4319", "--port", "4319"}, "cld: --local http://127.0.0.1:4319" + collector("4319")},
		{[]string{"--port", "4317", "--remote", "http://localhost:4317"}, "cld: --remote http://localhost:4317" + collector("4317")},
		{[]string{"--local", "https://0.0.0.0:4319", "--port", "4319"}, "cld: --local https://0.0.0.0:4319" + collector("4319")},
		{[]string{"--local", "http://[::]:4319", "--port", "4319"}, "cld: --local http://[::]:4319" + collector("4319")},
		{[]string{"--local", "http://[::ffff:127.0.0.1]:4319", "--port", "4319"}, "cld: --local http://[::ffff:127.0.0.1]:4319" + collector("4319")},
		{[]string{"--local", "http://IDE.LocalHost.:4319", "--port", "4319"}, "cld: --local http://IDE.LocalHost.:4319" + collector("4319")},
		{[]string{"--local", "http://127.0.0.1:4318", "--remote", "http://localhost:4319", "--port", "4319"}, "cld: --remote http://localhost:4319" + collector("4319")},
		{[]string{"--remote", "http://localhost:4319", "--local", "http://127.0.0.1:4319", "--port", "4319"}, "cld: --local http://127.0.0.1:4319" + collector("4319")},
		// The URLs are checked before the port, and both before what is missing.
		{[]string{"--port", "x", "--local", "x"}, "cld: invalid URL 'x' for --local" + form},
		{[]string{"--port", "x"}, "cld: invalid port 'x" + port},
		{[]string{"--remote", "https://otel.example.com:4317", "--collector-config="}, "cld: option '--collector-config' needs a value (see cld help)\n"},
		{[]string{"--local"}, "cld: option '--local' needs a value (see cld help)\n"},
		{[]string{"--remote", "https://otel.example.com:4317", "x"}, "cld: setup telemetry: unexpected argument 'x' (see cld help)\n"},
		{[]string{"x", "--remote", "https://otel.example.com:4317"}, "cld: setup telemetry: unexpected argument 'x' (see cld help)\n"},
		{[]string{"--remote", "https://otel.example.com:4317", "--"}, "cld: setup telemetry: unexpected argument '--' (see cld help)\n"},
		{[]string{"--bogus"}, "cld: setup telemetry: unexpected argument '--bogus' (see cld help)\n"},
		{[]string{"-n", "x"}, "cld: setup telemetry: unexpected argument '-n' (see cld help)\n"},
	} {
		args := append([]string{"setup", "telemetry"}, test.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
}

// setup telemetry needs docker, which it looks for first, as the other commands look for tmux:
// before it reads the settings, here not valid JSON.
func TestSetupTelemetryRequiresDocker(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	s := sandbox.New(t)
	settings := writeSettings(t, s, "{")
	result := s.RunCld(map[string]string{"PATH": s.Tools()}, "setup", "telemetry", "--remote", "https://otel.example.com:4317")
	if want := "cld: docker is not installed\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	checkSettings(t, settings, "{")
}

// A docker the system cannot run ends cld as a tmux that cannot run does (see TestCannotRunTmux),
// from its first call, which looks for the collector: nothing has changed.
func TestCannotRunDocker(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, broken := range []struct {
		what, content string
		mode          os.FileMode
		code          int
		reason        string
	}{
		{"a missing interpreter", "#!/nonexistent/interpreter\n", 0o755, 127, "no such file or directory"},
		{"no #!", "echo docker\n", 0o755, 126, "exec format error"},
		{"no execute permission", "#!/bin/sh\necho docker\n", 0o644, 126, "permission denied"},
	} {
		t.Run(broken.what, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			fake := filepath.Join(s.Root, "fake")
			if err := os.Mkdir(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			docker := filepath.Join(fake, "docker")
			if err := os.WriteFile(docker, []byte(broken.content), broken.mode); err != nil {
				t.Fatal(err)
			}
			result := s.RunCld(map[string]string{"PATH": fake}, "setup", "telemetry", "--remote", "https://otel.example.com:4317")
			if want := "cld: cannot run " + docker + ": " + broken.reason + "\n"; result.Code != broken.code || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stderr %q", result.Code, result.Stdout, result.Stderr, broken.code, want)
			}
			if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("~/.claude: %v, want none", err)
			}
		})
	}
}

// new and resume need tmux and claude, and new -w git; the other commands only tmux: the fake tmux
// finds no session, so join and kill get as far as saying so.
func TestRequiresTools(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		present []string
		want    string
	}{
		{[]string{"new"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"new"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"new", "-w"}, []string{"tmux", "claude"}, "cld: git is not installed\n"},
		{[]string{"resume"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"resume", "-n", "x", "SESSION"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"join"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, "cld: no session 'main'; create it with cld new -n main\n"},
		{[]string{"kill"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"kill"}, []string{"tmux"}, "cld: no session 'main' (see cld list)\n"},
		{[]string{"list"}, []string{"claude"}, "cld: tmux is not installed\n"},
	} {
		t.Run(strings.Join(test.args, " ")+" "+strings.Join(test.present, ","), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools(test.present...)}, test.args...)
			if result.Code != 1 || result.Stderr != test.want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, test.want)
			}
		})
	}
}

// cld runs no program from a relative PATH entry, "." or an empty one: a tool found only there
// is not installed, and one that an absolute entry after it also has runs from that entry. The
// working directory has a tmux, a claude and a git of its own, which fail, saying so, if run. new
// hands tmux the claude of the absolute entry by its path, which tmux, looking the bare word up,
// would have taken from the relative one (see TestStartsTheClaudeItChecks).
func TestIgnoresRelativePathEntries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		present []string
		code    int
		want    string
	}{
		{[]string{"join"}, nil, 1, "cld: tmux is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, 1, "cld: no session 'main'; create it with cld new -n main\n"},
		{[]string{"new"}, []string{"tmux"}, 1, "cld: claude is not installed\n"},
		{[]string{"new", "-w"}, []string{"tmux", "claude"}, 1, "cld: git is not installed\n"},
		// tmux, claude --version and git, run from the absolute entry, find no session, a version
		// that passes and the repository.
		{[]string{"new", "-w"}, []string{"tmux", "claude", "git"}, 0, ""},
	} {
		for _, relative := range []string{".", ""} {
			t.Run(strings.Join(test.args, " ")+" "+strings.Join(test.present, ",")+" after '"+relative+"'", func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				gitInit(t, s.Work)
				for _, name := range []string{"tmux", "claude", "git"} {
					script := "#!/bin/sh\necho \"relative " + name + " ran\" >&2\nexit 99\n"
					if err := os.WriteFile(filepath.Join(s.Work, name), []byte(script), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				tools := s.Tools(test.present...)
				path := relative + string(os.PathListSeparator) + tools
				if result := s.RunCld(map[string]string{"PATH": path}, test.args...); result.Code != test.code || result.Stderr != test.want {
					t.Errorf("PATH %s: exit %d, stderr %q, want exit %d, stderr %q", path, result.Code, result.Stderr, test.code, test.want)
				}
				if test.code != 0 {
					return
				}
				if argv, claude := s.FakeTmuxRecord().Argv, filepath.Join(tools, "claude"); !slices.Contains(argv, claude) || slices.Contains(argv, "claude") {
					t.Errorf("tmux arguments %q name claude otherwise than as %s", argv, claude)
				}
			})
		}
	}
}

// cld requires the tmux its tests run on, 3.7. A letter, a bug-fix release, is not compared;
// development builds are read from what follows "next-", and pass without a version.
func TestRequiresTmux(t *testing.T) {
	t.Parallel()
	for version, accepted := range map[string]bool{
		"tmux 3.7":      true,
		"tmux 3.7c":     true,
		"tmux 3.10":     true,
		"tmux 4.0":      true,
		"tmux next-3.9": true,
		"tmux 3.8-rc2":  true,
		"tmux master":   true,
		"tmux 3.6b":     false,
		"tmux 3.5a":     false,
		"tmux 3.3a":     false,
		"tmux 2.9a":     false,
		"tmux next-3.6": false,
	} {
		for _, command := range []string{"new", "resume"} {
			t.Run(command+" "+version, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCld(map[string]string{
					"PATH":                  s.Tools("tmux", "claude"),
					"CLD_FAKE_TMUX_VERSION": version,
				}, command)
				_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
				started := err == nil
				if accepted && (result.Code != 0 || !started) {
					t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
				}
				if want := "cld: tmux 3.7 or newer is required, found '" + version + "'\n"; !accepted &&
					(result.Code != 1 || result.Stderr != want || started) {
					t.Errorf("accepted: exit %d, stderr %q, tmux started: %v", result.Code, result.Stderr, started)
				}
			})
		}
	}
}

// new and resume require claude 2.1.232, the first release that does what cld passes and relies
// on, resume included, comparing the numbers claude --version starts with as numbers: 2.1.30 is
// older. Output that does not start with a version passes. An older claude - 2.1.222, the minimum
// before resume, too - is refused before tmux starts. The probe answers --version without leaving
// a record of a claude: the fake tmux starts none.
func TestRequiresClaude(t *testing.T) {
	t.Parallel()
	for version, accepted := range map[string]bool{
		"2.1.232 (Claude Code)":   true,
		"2.1.282 (Claude Code)":   true,
		"2.2.0 (Claude Code)":     true,
		"2.10.0 (Claude Code)":    true,
		"3.0.0 (Claude Code)":     true,
		"Claude Code, version 42": true,
		"2.1.231 (Claude Code)":   false,
		"2.1.222 (Claude Code)":   false,
		"2.1.30 (Claude Code)":    false,
		"2.0.999 (Claude Code)":   false,
		"1.9.9 (Claude Code)":     false,
	} {
		for _, command := range []string{"new", "resume"} {
			t.Run(command+" "+version, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCld(map[string]string{
					"PATH":                    s.Tools("tmux", "claude"),
					"CLD_FAKE_TMUX_VERSION":   "tmux 3.7c",
					"CLD_FAKE_CLAUDE_VERSION": version,
				}, command)
				_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
				started := err == nil
				if accepted && (result.Code != 0 || !started) {
					t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
				}
				if want := "cld: claude 2.1.232 or newer is required, found '" + version + "'\n"; !accepted &&
					(result.Code != 1 || result.Stderr != want || result.Stdout != "" || started) {
					t.Errorf("accepted: exit %d, stdout %q, stderr %q, tmux started: %v", result.Code, result.Stdout, result.Stderr, started)
				}
				if probes := s.Probes(); len(probes) != 0 {
					t.Errorf("claude --version left %d probe record(s)", len(probes))
				}
			})
		}
	}
}

// A claude whose --version fails is refused before tmux starts, with status 1 and what it printed
// - stdout, then stderr. false as claude exits 1, and may print something first: GNU's false its
// version, uutils' that it knows no program claude. A claude that cannot run at all ends cld as a
// tmux that cannot run does (see TestCannotRunTmux): 127 for a missing interpreter, 126
// otherwise, and why. A script without #! runs with /bin/sh, as tmux's execvp runs it, and is
// checked as any other claude (see TestStartsTheClaudeItChecks).
func TestRequiresClaudeVersion(t *testing.T) {
	t.Parallel()
	const required = "cld: claude 2.1.232 or newer is required, but "
	for _, test := range []struct {
		name, script string
		code         int
		// want is stderr, CLAUDE standing for claude's path; with prefix, what stderr starts with.
		want   string
		prefix bool
	}{
		{"false", "", 1, required + "claude --version exited with status 1", true},
		{"status 3", "#!/bin/sh\necho partial\necho 'claude: cannot load' >&2\nexit 3\n", 1,
			required + "claude --version exited with status 3: partial\nclaude: cannot load\n", false},
		{"signal", "#!/bin/sh\nkill -TERM $$\n", 1, required + "claude --version exited with signal 15\n", false},
		{"missing interpreter", "#!/nonexistent/interpreter\n", 127, "cld: cannot run CLAUDE: no such file or directory\n", false},
		{"no #!", "echo '2.1.100 (Claude Code)'\n", 1, "cld: claude 2.1.232 or newer is required, found '2.1.100 (Claude Code)'\n", false},
		{"no #!, status 3", "echo 'claude: cannot load' >&2\nexit 3\n", 1,
			required + "claude --version exited with status 3: claude: cannot load\n", false},
		// A binary the system will not execute - an ELF header with nothing after it, as of one
		// cut short or for another machine, and a file with a NUL in its first line - is no script:
		// such a claude cannot run, and /bin/sh does not read it as commands.
		{"ELF", "\x7fELF\x02\x01\x01" + strings.Repeat("\x00", 57), 126, "cld: cannot run CLAUDE: exec format error\n", false},
		{"NUL", "echo '2.1.300 (Claude Code)'\x00\n", 126, "cld: cannot run CLAUDE: exec format error\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux")
			claude := filepath.Join(tools, "claude")
			if test.script == "" {
				target, err := exec.LookPath("false")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, claude); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(claude, []byte(test.script), 0o755); err != nil {
				t.Fatal(err)
			}
			result := s.RunCld(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "new")
			want := strings.ReplaceAll(test.want, "CLAUDE", claude)
			if result.Code != test.code || result.Stdout != "" || test.prefix && !strings.HasPrefix(result.Stderr, want) || !test.prefix && result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stderr %q", result.Code, result.Stdout, result.Stderr, test.code, want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("tmux started")
			}
		})
	}
}

// A claude --version that leaves a process in the background holding its stdout and stderr - a
// wrapper's update check, say - holds new up for a second at most, not for as long as that runs,
// and is checked on what it printed before it exited: accepted, refused as too old, or refused as
// failing. The process runs for a minute; cld must be done well before.
func TestClaudeVersionLeavesAProcessBehind(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, script string
		code         int
		stderr       string
	}{
		{"accepted", "echo '2.1.300 (Claude Code)'", 0, ""},
		{"too old", "echo '2.1.100 (Claude Code)'", 1, "cld: claude 2.1.232 or newer is required, found '2.1.100 (Claude Code)'\n"},
		{"failing", "echo 'claude: cannot load' >&2; exit 3", 1,
			"cld: claude 2.1.232 or newer is required, but claude --version exited with status 3: claude: cannot load\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux", "sleep")
			behind := filepath.Join(s.Root, "behind")
			script := "#!/bin/sh\nsleep 60 &\necho $! >'" + behind + "'\n" + test.script + "\n"
			if err := os.WriteFile(filepath.Join(tools, "claude"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if pid, err := os.ReadFile(behind); err == nil {
					_ = exec.Command("kill", strings.TrimSpace(string(pid))).Run()
				}
			})
			start := time.Now()
			result := s.RunCld(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "new")
			if took := time.Since(start); took > 20*time.Second {
				t.Errorf("new took %v, waiting on the process claude left behind", took)
			}
			stdout := ""
			if test.code == 0 {
				stdout = "\x1b]0;✳ cld-main\x07"
			}
			if result.Code != test.code || result.Stdout != stdout || result.Stderr != test.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, stdout, test.stderr)
			}
			_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
			if handedOver := err == nil; handedOver != (test.code == 0) {
				t.Errorf("cld handed over to tmux: %v, want %v", handedOver, test.code == 0)
			}
		})
	}
}

// new and resume run claude; join, kill, list and setup telemetry never do, and neither does
// completion - __complete and __completeNoDesc - which makes no check at all (see
// TestCompletionSkipsChecks), for new's, resume's and setup telemetry's arguments too and with a
// tmux the check refuses: with a claude too old for new and resume, which records that it ran,
// the others do as they do with any other - setup telemetry, which checks no tmux, looks for
// docker, not on the PATH here, and on a system other than Linux refuses to run. new and
// resume run claude --version last among the checks they make before tmux - what is not
// installed, and tmux's version, which join, kill and list check too, come first - and before the
// session lookup: with session main found (sessions "cld-main"), a check that came later would
// say the session exists, after a list-panes the fake tmux records. The fake tmux lists the
// sessions that sessions names, none if it is empty, and completion finds them through a socket
// cld-main. That it comes before the check for cld's own pane as well, which needs a terminal,
// TestRefusesToNestInItsOwnPane pins.
func TestOnlyNewAndResumeRunClaude(t *testing.T) {
	t.Parallel()
	noDocker := "cld: docker is not installed\n"
	if runtime.GOOS != "linux" {
		noDocker = "cld: setup telemetry works on Linux only\n"
	}
	for _, test := range []struct {
		args        []string
		tmuxVersion string
		sessions    string
		code        int
		stdout      string
		stderr      string
		// ran is whether claude --version runs.
		ran bool
	}{
		{[]string{"new"}, "tmux 3.7c", "", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"new"}, "tmux 3.7c", "cld-main", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"new", "-w"}, "tmux 3.7c", "", 1, "", "cld: git is not installed\n", false},
		{[]string{"new"}, "tmux 3.6b", "", 1, "", "cld: tmux 3.7 or newer is required, found 'tmux 3.6b'\n", false},
		{[]string{"resume"}, "tmux 3.7c", "", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"resume", "-n", "main", "SESSION"}, "tmux 3.7c", "cld-main", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"resume"}, "tmux 3.6b", "", 1, "", "cld: tmux 3.7 or newer is required, found 'tmux 3.6b'\n", false},
		{[]string{"join"}, "tmux 3.7c", "", 1, "", "cld: no session 'main'; create it with cld new -n main\n", false},
		{[]string{"kill"}, "tmux 3.7c", "", 1, "", "cld: no session 'main' (see cld list)\n", false},
		{[]string{"list"}, "tmux 3.7c", "", 0, "", "", false},
		{[]string{"__complete", "new", "-n", ""}, "tmux 3.6b", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"__complete", "resume", "-n", ""}, "tmux 3.6b", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"__completeNoDesc", "join", "-n", ""}, "tmux 3.6b", "cld-main\tdetached\t0\t100\t/w", 0, "main\n:4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"setup", "telemetry", "--remote", "https://otel.example.com:4317"}, "tmux 3.6b", "", 1, "", noDocker, false},
		{[]string{"__complete", "setup", "telemetry", "--local", ""}, "tmux 3.6b", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
	} {
		name := strings.Join(test.args, " ") + ", " + test.tmuxVersion
		if test.sessions != "" {
			name += ", session main"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.sessions != "" {
				socket(t, s, "cld-main")
			}
			tools := s.Tools("tmux")
			ran := filepath.Join(s.Root, "claude ran")
			script := "#!/bin/sh\necho \"$*\" >'" + ran + "'\necho '2.1.231 (Claude Code)'\n"
			if err := os.WriteFile(filepath.Join(tools, "claude"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			result := s.RunCld(map[string]string{
				"PATH":                   tools,
				"CLD_FAKE_TMUX_VERSION":  test.tmuxVersion,
				"CLD_FAKE_TMUX_SESSIONS": test.sessions,
			}, test.args...)
			if result.Code != test.code || result.Stdout != test.stdout || result.Stderr != test.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, test.stdout, test.stderr)
			}
			args, err := os.ReadFile(ran)
			if test.ran && string(args) != "--version\n" {
				t.Errorf("claude ran with %q, want --version", args)
			}
			if !test.ran && err == nil {
				t.Errorf("claude ran with %q", args)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("tmux ran a command other than -V and list-sessions")
			}
		})
	}
}

// new checks the claude that tmux starts: claude --version runs in the directory claude starts
// in, where a version manager's shim - mise's, say - runs the claude that directory pins.
// The fake claude reports the version in the file .claude-version where the directory has one,
// and global otherwise, as such a shim would.
func TestChecksClaudeWhereItStarts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		global, pinned string
		accepted       bool
	}{
		{"2.1.282 (Claude Code)", "2.1.100 (Claude Code)", false},
		{"2.1.100 (Claude Code)", "2.1.282 (Claude Code)", true},
	} {
		t.Run("pinned "+test.pinned, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux")
			script := "#!/bin/sh\nif [ -f .claude-version ]; then read -r v <.claude-version; echo \"$v\"; else echo '" + test.global + "'; fi\n"
			if err := os.WriteFile(filepath.Join(tools, "claude"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.Work, ".claude-version"), []byte(test.pinned+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result := s.RunCld(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "new")
			if test.accepted {
				if title := "\x1b]0;\u2733 cld-main\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
					t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
				}
				if argv := s.FakeTmuxRecord().Argv; !slices.Contains(argv, s.Work) {
					t.Errorf("tmux arguments %q name no %s", argv, s.Work)
				}
				return
			}
			if want := "cld: claude 2.1.232 or newer is required, found '" + test.pinned + "'\n"; result.Code != 1 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("tmux started")
			}
		})
	}
}

// Completion makes none of the checks the other commands make before they run tmux, where they
// would end cld with status 1 and nothing on stdout: with a tmux whose version they refuse, join
// -n offers what that tmux lists. With no tmux, one that fails or cannot run, or a socket
// directory it cannot read, it offers no names, and no file names, and exits 0; what went wrong
// goes to stderr, which the completion scripts discard. The tmux is asked through a socket cld-x,
// as list asks it. That it never runs claude, whose version new and resume check,
// TestOnlyNewAndResumeRunClaude pins.
func TestCompletionSkipsChecks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		what string
		// script is the tmux on the PATH, with mode; none without one, the fake tmux when "fake",
		// and the fake tmux with a file where the socket directory would be when "unreadable".
		script string
		mode   os.FileMode
		stdout string
		stderr string
	}{
		{"a tmux the check refuses", "fake", 0, "x\tdetached\n:4\n", ""},
		{"no tmux", "", 0, ":4\n", "tmux is not installed"},
		{"a tmux that fails", "#!/bin/sh\necho 'tmux: broken' >&2\nexit 3\n", 0o755, ":4\n", "tmux: broken"},
		{"a tmux that cannot run", "#!/bin/sh\necho 'tmux 3.7c'\n", 0o644, ":4\n", "permission denied"},
		{"a socket directory it cannot read", "unreadable", 0, ":4\n", "cannot read"},
	} {
		t.Run(test.what, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			env := map[string]string{"PATH": s.Tools()}
			switch test.script {
			case "":
			case "fake", "unreadable":
				env["PATH"] = s.Tools("tmux")
				env["CLD_FAKE_TMUX_VERSION"] = "tmux 3.2a"
				env["CLD_FAKE_TMUX_SESSIONS"] = "cld-x\tdetached\t0\t100\t/w"
			default:
				if err := os.WriteFile(filepath.Join(env["PATH"], "tmux"), []byte(test.script), test.mode); err != nil {
					t.Fatal(err)
				}
			}
			if test.script == "unreadable" {
				s.WriteFile(s.SocketDir(), "")
			} else {
				socket(t, s, "cld-x")
			}
			result := s.RunCld(env, "__complete", "join", "-n", "")
			if result.Code != 0 || result.Stdout != test.stdout || !strings.Contains(result.Stderr, test.stderr) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr with %q", result.Code, result.Stdout, result.Stderr, test.stdout, test.stderr)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld ran tmux for more than its list of sessions")
			}
		})
	}
}

// endHint is how the pane-died hook that new and resume set, and join, show how to end a session
// whose claude failed.
const endHint = "display-message -d 0 'claude exited with " +
	"#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}: " +
	"C-q d detaches, cld kill -n #{window_name} ends the session'"

// new and resume hand over to tmux with this command, word for word: the session's own server,
// its options, the directory, claude - by the path of the one it checked - and its arguments as
// separate words, and what goes on claude's window. resume's claude gets new's arguments, never
// -w's, then --resume. A word ending in ";", which tmux would take for the end of its command,
// goes with a "\" before the ";", which tmux drops: SESSION, or the directory cld runs in. The
// directory goes with every "#" doubled, since tmux expands -c as a format, in which "##" is a
// "#". The fake tmux, which finds no server running for the session, records the command, and
// the environment it gets: cld's own, without TERMINAL_EMULATOR and with an empty TMUX where TMUX
// was set - join's client needs it (see TestNestsOnADeadPanesPty), and with it tmux still takes
// new's terminal for UTF-8 (see TestNestsInsideAnotherTmux); a PS1, which the script's bash
// dropped, passes too (decision 11 in docs/design.md).
func TestNewTmuxCommand(t *testing.T) {
	t.Parallel()
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	const fromHead = `{"remoteControlAtStartup":true,"worktree":{"baseRef":"head"}}`
	for _, command := range []struct {
		args []string
		// dir is where in the work tree cld runs, and c what tmux gets with -c there
		dir, c string
		// claude is claude and its arguments, as tmux gets them
		claude []string
	}{
		{[]string{"new", "-n", "x"}, "", "", []string{probe, "--name", "cld-x", "--settings", remoteControl}},
		{[]string{"new", "-n", "x", "-w"}, "", "", []string{probe, "--name", "cld-x", "--settings", fromHead, "--worktree", "x"}},
		{[]string{"resume", "-n", "x"}, "", "", []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", "cld-x"}},
		{[]string{"resume", "-n", "x", "a b"}, "", "", []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", "a b"}},
		{[]string{"resume", "-n", "x", "a;"}, "", "", []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", `a\;`}},
		{[]string{"resume", "-n", "x", `a\;`}, "", "", []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", `a\\;`}},
		{[]string{"new", "-n", "x"}, "w;", `w\;`, []string{probe, "--name", "cld-x", "--settings", remoteControl}},
		{[]string{"new", "-n", "x", "-w"}, "w;", `w\;`, []string{probe, "--name", "cld-x", "--settings", fromHead, "--worktree", "x"}},
		{[]string{"resume", "-n", "x"}, "w;", `w\;`, []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", "cld-x"}},
		{[]string{"new", "-n", "x"}, `w\;`, `w\\;`, []string{probe, "--name", "cld-x", "--settings", remoteControl}},
		{[]string{"new", "-n", "x"}, "C#S", "C##S", []string{probe, "--name", "cld-x", "--settings", remoteControl}},
		{[]string{"new", "-n", "x", "-w"}, "x#(touch ran)", "x##(touch ran)", []string{probe, "--name", "cld-x", "--settings", fromHead, "--worktree", "x"}},
		{[]string{"resume", "-n", "x"}, "#{session_name}#;", `##{session_name}##\;`, []string{probe, "--name", "cld-x", "--settings", remoteControl, "--resume", "cld-x"}},
	} {
		args, dir, c, claude := command.args, command.dir, command.c, command.claude
		name := strings.Join(args, " ")
		if dir != "" {
			name += " in " + dir
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			if dir != "" {
				if err := os.Mkdir(filepath.Join(s.Work, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			given := map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
				"TERMINAL_EMULATOR":     "JetBrains-JediTerm",
				"TMUX":                  filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
				"PS1":                   `\u@\h$ `,
			}
			result := s.RunCldIn(filepath.Join(s.Work, dir), given, args...)
			if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
			}
			want := []string{"-L", "cld-x", "-f", "/dev/null",
				"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys", ";",
				"set", "-s", "focus-events", "on", ";",
				"set", "-g", "mouse", "on", ";", "set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
				"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
				"new-session", "-s", "cld-x", "-n", "x", "-c", filepath.Join(s.Work, c)}
			want = append(want, claude...)
			want = append(want, ";",
				"set", "-w", "-t", "=cld-x:", "remain-on-exit", "failed", ";",
				"set", "-w", "-t", "=cld-x:", "remain-on-exit-format", "", ";",
				"set-hook", "-w", "-t", "=cld-x:", "pane-died", `if -F '#{window_active_clients}' "`+endHint+`"`)
			record := s.FakeTmuxRecord()
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given, "TERMINAL_EMULATOR"))
			if want := filepath.Join(s.Work, dir); record.Cwd != want {
				t.Errorf("tmux runs in %s, want %s", record.Cwd, want)
			}
		})
	}
}

// join hands over to tmux with this command, word for word, and with the environment it got but
// for an empty TMUX where TMUX was set: TERMINAL_EMULATOR too, which only new and resume leave
// out, and a PS1, which the script's bash dropped. The fake tmux finds session x on its server,
// then records the command.
func TestJoinTmuxCommand(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	given := map[string]string{
		"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
		"CLD_FAKE_TMUX_SESSIONS": "cld-x",
		"TERMINAL_EMULATOR":      "JetBrains-JediTerm",
		"TERM_PROGRAM":           "iTerm.app",
		"LC_TERMINAL":            "iTerm2",
		"TMUX":                   filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
		"PS1":                    `\u@\h$ `,
	}
	result := s.RunCld(given, "join", "-n", "x")
	if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
	}
	record := s.FakeTmuxRecord()
	if want := []string{"-L", "cld-x", "attach-session", "-d", "-t", "=cld-x", ";", "if", "-F", "#{pane_dead}", endHint}; !slices.Equal(record.Argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
	}
	checkEnv(t, record.Env, passedOn(s, given))
}

// A server can exit while cld asks it - its claude exits, a cld kill runs - and tmux then says that
// the server exited unexpectedly: list passes over it and lists the other sessions, and join and
// kill find no session there. The fake tmux answers every server but cld-b, which exits as it is
// asked.
func TestServerExitingWhileAsked(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	socket(t, s, "cld-a")
	socket(t, s, "cld-b")
	fake := map[string]string{
		"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
		"CLD_FAKE_TMUX_SESSIONS": "cld-a\tdetached\t0\t100\t/w",
		"CLD_FAKE_TMUX_EXITED":   "cld-b",
	}
	for _, test := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"list"}, 0, "NAME  STATE     DIRECTORY\n" + "a     detached  /w\n", ""},
		{[]string{"join", "-n", "b"}, 1, "", "cld: no session 'b'; create it with cld new -n b\n"},
		{[]string{"kill", "-n", "b"}, 1, "", "cld: no session 'b' (see cld list)\n"},
	} {
		if result := s.RunCld(fake, test.args...); result.Code != test.code || result.Stdout != test.stdout || result.Stderr != test.stderr {
			t.Errorf("%s: exit %d, stderr %q, stdout\n%s\nwant exit %d, stderr %q, stdout\n%s",
				strings.Join(test.args, " "), result.Code, result.Stderr, result.Stdout, test.code, test.stderr, test.stdout)
		}
	}
}

// passedOn is the environment cld runs in with extra, as cld hands it on to tmux: without the
// variables named in dropped, and with an empty TMUX where TMUX was set.
func passedOn(s *sandbox.Sandbox, extra map[string]string, dropped ...string) map[string]string {
	env := map[string]string{}
	for _, variable := range s.Environ(extra) {
		name, value, _ := strings.Cut(variable, "=")
		env[name] = value
	}
	for _, name := range dropped {
		delete(env, name)
	}
	if _, set := env["TMUX"]; set {
		env["TMUX"] = ""
	}
	return env
}

// checkEnv reports each variable of the environment tmux got that is not as in want.
func checkEnv(t *testing.T, got, want map[string]string) {
	t.Helper()
	for name, value := range want {
		if gotValue, found := got[name]; !found || gotValue != value {
			t.Errorf("tmux gets %s %q (set: %v), want %q", name, gotValue, found, value)
		}
	}
	for name, value := range got {
		if _, wanted := want[name]; !wanted {
			t.Errorf("tmux gets %s=%q, want it unset", name, value)
		}
	}
}

// A tmux that fails where cld expects it to work - tmux -V, or the kill-session and kill-server
// that kill runs - ends cld with tmux's exit status, after tmux's own message: cld adds none. A
// signal ends it with 128 and the signal's number, as a shell reports it.
func TestPassesTmuxFailuresThrough(t *testing.T) {
	t.Parallel()
	const (
		versionFails = `echo "tmux: broken" >&2; exit 3`
		versionDies  = `kill -TERM $$`
		killFails    = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld-x ;; *kill-server*) echo "tmux: cannot kill" >&2; exit 5 ;; esac`
	)
	for _, test := range []struct {
		failure, script string
		args            []string
		code            int
		stderr          string
	}{
		{"-V exits 3", versionFails, []string{"list"}, 3, "tmux: broken\n"},
		{"-V exits 3", versionFails, []string{"kill", "-n", "x"}, 3, "tmux: broken\n"},
		{"-V gets SIGTERM", versionDies, []string{"list"}, 128 + 15, ""},
		{"-V gets SIGTERM", versionDies, []string{"join"}, 128 + 15, ""},
		{"the kill exits 5", killFails, []string{"kill", "-n", "x"}, 5, "tmux: cannot kill\n"},
	} {
		t.Run(strings.Join(test.args, " ")+", "+test.failure, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			fake := filepath.Join(s.Root, "fake")
			if err := os.Mkdir(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fake, "tmux"), []byte("#!/bin/sh\n"+test.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			result := s.RunCld(map[string]string{"PATH": fake + string(os.PathListSeparator) + s.Env["PATH"]}, test.args...)
			if result.Code != test.code || result.Stderr != test.stderr || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stderr %q", result.Code, result.Stdout, result.Stderr, test.code, test.stderr)
			}
		})
	}
}

// A tmux the system cannot run at all ends cld with the status a shell gives, after cld's own
// message: 127 when the system reports no such file - here the interpreter the #! line names -
// and 126 otherwise - here a text file without #!, which bash ran as a script, and a file
// without the execute permission, which cld, like bash, takes when the PATH has no executable
// tmux. So does a tmux that stops being runnable once it has answered tmux -V and the session
// lookup: new and join cannot hand over to it, and kill cannot end the session with it. A lookup
// that cannot run, once tmux -V has answered, ends cld with status 1 and the same message, as
// the script's lookups ended it with bash's. list's lookup asks the server of a socket cld-x.
func TestCannotRunTmux(t *testing.T) {
	t.Parallel()
	const title = "\x1b]0;\u2733 cld-x\x07"
	for _, broken := range []struct {
		what, content string
		mode          os.FileMode
		code          int
		reason        string
	}{
		{"a missing interpreter", "#!/nonexistent/interpreter\n", 0o755, 127, "no such file or directory"},
		{"no #!", "echo tmux 3.7c\n", 0o755, 126, "exec format error"},
		{"no execute permission", "#!/bin/sh\necho tmux 3.7c\n", 0o644, 126, "permission denied"},
	} {
		for _, test := range []struct {
			args []string
			// answers is what tmux runs before it cannot: nothing, or a script that answers tmux
			// -V and, if it is not -V, the lookup - where new's finds no server, it fails on
			// the way out, once the file has been moved.
			answers string
			// lookup is whether the command that cannot run is a session lookup.
			lookup bool
			stdout string
		}{
			{[]string{"list"}, "", false, ""},
			{[]string{"join"}, "", false, ""},
			{[]string{"kill"}, "", false, ""},
			{[]string{"list"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"join", "-n", "x"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"kill", "-n", "x"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"new", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo 'no server running on /fake' >&2; trap 'exit 1' EXIT`, false, title},
			{[]string{"join", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, title},
			{[]string{"kill", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, ""},
		} {
			stage := "at once"
			if test.answers != "" {
				stage = "after the lookup"
				if test.lookup {
					stage = "after -V"
				}
			}
			t.Run(strings.Join(test.args, " ")+", "+stage+", "+broken.what, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				socket(t, s, "cld-x")
				fake := filepath.Join(s.Root, "fake")
				if err := os.Mkdir(fake, 0o755); err != nil {
					t.Fatal(err)
				}
				tmux := filepath.Join(fake, "tmux")
				if test.answers == "" {
					if err := os.WriteFile(tmux, []byte(broken.content), broken.mode); err != nil {
						t.Fatal(err)
					}
				} else {
					// The script answers, then moves the file that cannot run over itself.
					if err := os.WriteFile(tmux+".broken", []byte(broken.content), broken.mode); err != nil {
						t.Fatal(err)
					}
					script := "#!/bin/sh\n" + test.answers + "\nmv -f '" + tmux + ".broken' '" + tmux + "'\n"
					if err := os.WriteFile(tmux, []byte(script), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				// No other tmux on the PATH, so that cld takes one without the execute permission.
				path := fake + string(os.PathListSeparator) + s.Tools("claude", "mv")
				result := s.RunCld(map[string]string{"PATH": path}, test.args...)
				code := broken.code
				if test.lookup {
					code = 1
				}
				want := "cld: cannot run " + tmux + ": " + broken.reason + "\n"
				if result.Code != code || result.Stderr != want || result.Stdout != test.stdout {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
						result.Code, result.Stdout, result.Stderr, code, test.stdout, want)
				}
			})
		}
	}
}

// A claude or git on the PATH without the execute permission, where the PATH has no executable
// one, is found all the same, as bash's search found it: new cannot run the claude to check its
// version, and ends as with such a tmux (126), where the script handed it to tmux; git cannot say
// that the directory is in a repository. A tool without the execute permission before an
// executable one does not hide it.
func TestToolsWithoutExecutePermission(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// denied are the tools without the execute permission, in a PATH entry before present.
		denied, present []string
		code            int
		stderr          string
	}{
		{[]string{"new", "-n", "x"}, []string{"claude"}, []string{"tmux"}, 126,
			"cld: cannot run DENIED/claude: permission denied\n"},
		{[]string{"new", "-n", "x", "-w"}, []string{"git"}, []string{"tmux", "claude"}, 1,
			"cld: --worktree needs a git repository, and WORK is not in one\n"},
		{[]string{"new", "-n", "x", "-w"}, []string{"git"}, []string{"tmux", "claude", "git"}, 0, ""},
		{[]string{"new", "-n", "x"}, []string{"tmux", "claude"}, []string{"tmux", "claude"}, 0, ""},
	} {
		t.Run(strings.Join(test.args, " ")+", "+strings.Join(test.denied, ",")+" denied, "+strings.Join(test.present, ",")+" present", func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			denied := filepath.Join(s.Root, "denied")
			if err := os.Mkdir(denied, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range test.denied {
				script := "#!/bin/sh\necho \"" + name + " without the execute permission ran\" >&2\nexit 99\n"
				if err := os.WriteFile(filepath.Join(denied, name), []byte(script), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			result := s.RunCld(map[string]string{
				"PATH":                  denied + string(os.PathListSeparator) + s.Tools(test.present...),
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}, test.args...)
			stdout, stderr := "", strings.NewReplacer("WORK", s.Work, "DENIED", denied).Replace(test.stderr)
			if test.code == 0 {
				stdout = "\x1b]0;\u2733 cld-x\x07"
			}
			if result.Code != test.code || result.Stdout != stdout || result.Stderr != stderr {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, stdout, stderr)
			}
			_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
			if handedOver := err == nil; handedOver != (test.code == 0) {
				t.Errorf("cld handed over to tmux: %v, want %v", handedOver, test.code == 0)
			}
		})
	}
}

// A write to stdout that fails ends cld with status 1, as the script's printf and cat failing
// under set -e did, so that output cut short does not pass for whole: list, the help - from help
// and from -h - and the version, and new, resume and join, which then do not hand over to tmux;
// the completion scripts and the answers to __complete, which cobra prints for cld; and setup
// telemetry's report, once it is done. stdout is open for reading only here, so that every write
// to it fails; list and __complete find session x through a socket cld-x.
func TestFailedWriteEndsCld(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// sessions is what the fake tmux lists.
		sessions string
		// before is what cobra writes to stderr first.
		before string
	}{
		{[]string{"list"}, "cld-x\tdetached\t0\t100\t/w", ""},
		{[]string{"help"}, "", ""},
		{[]string{"help", "new"}, "", ""},
		{[]string{"new", "-h"}, "", ""},
		{[]string{"join", "-h"}, "", ""},
		{[]string{"kill", "-h", "-x"}, "", ""},
		{[]string{"version"}, "", ""},
		{[]string{"new", "-n", "x"}, "", ""},
		{[]string{"resume", "-n", "x"}, "", ""},
		{[]string{"join", "-n", "x"}, "cld-x", ""},
		{[]string{"completion", "bash"}, "", ""},
		{[]string{"completion", "zsh", "--help"}, "", ""},
		{[]string{"completion"}, "", ""},
		{[]string{"__complete", "join", "-n", ""}, "cld-x\tdetached\t0\t100\t/w", "Completion ended with directive: ShellCompDirectiveNoFileComp\n"},
		{[]string{"help", "setup"}, "", ""},
		{[]string{"setup", "-h", "telemetry"}, "", ""},
		{[]string{"setup", "telemetry", "--remote", "https://otel.example.com:4317"}, "", ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			if test.args[0] == "setup" && test.args[1] == "telemetry" {
				linuxOnly(t)
			}
			s := sandbox.New(t)
			socket(t, s, "cld-x")
			stdout, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			cmd := exec.Command(sandbox.Cld, test.args...)
			cmd.Env = s.Environ(map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": test.sessions,
			})
			cmd.Dir = s.Work
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = stdout, &stderr
			_ = cmd.Run()
			if code, want := cmd.ProcessState.ExitCode(), test.before+"cld: write error: bad file descriptor\n"; code != 1 || stderr.String() != want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", code, stderr.String(), want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// With nothing to print, cld writes nothing, since even an empty write to a stdout that cannot
// take one fails: kill, which prints nothing, ends the session and exits 0 with stdout open for
// reading only, where it would fail with status 1 once the session was gone.
func TestNothingToPrintWritesNothing(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	stdout, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	cmd := exec.Command(sandbox.Cld, "kill", "-n", "a")
	// The fake tmux finds session a on its server, then records the kill.
	cmd.Env = s.Environ(map[string]string{
		"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
		"CLD_FAKE_TMUX_SESSIONS": "cld-a",
	})
	cmd.Dir = s.Work
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	_ = cmd.Run()
	if code := cmd.ProcessState.ExitCode(); code != 0 || stderr.Len() != 0 {
		t.Errorf("exit %d, stderr %q, want exit 0, no stderr", code, stderr.String())
	}
	if argv, want := s.FakeTmuxRecord().Argv, []string{"-L", "cld-a", "kill-session", "-t", "=cld-a", ";", "kill-server"}; !slices.Equal(argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", argv, want)
	}
}

// new, with or without -w, and resume refuse a working directory that no longer exists: the
// script went on with the PWD it got, and tmux started claude in the home directory instead.
// claude --version fails there, the probe's as claude 2.1.282's, so cld refuses the directory
// before it runs claude --version there. On Linux only: what macOS's getcwd does in a removed
// directory has not been checked.
func TestNewRefusesARemovedDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("getcwd in a removed directory is checked on Linux only")
	}
	for _, args := range [][]string{{"new", "-n", "x"}, {"new", "-n", "x", "-w"}, {"resume", "-n", "x"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			// sh makes the directory, moves into it and removes it, then runs cld there.
			script := `mkdir "$1" && cd "$1" && rmdir "$1" && shift && exec "$0" "$@"`
			cmd := exec.Command("/bin/sh", append([]string{"-c", script, sandbox.Cld, filepath.Join(s.Work, "removed")}, args...)...)
			cmd.Env = s.Environ(map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			})
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			_ = cmd.Run()
			if code, want := cmd.ProcessState.ExitCode(), "cld: the current directory no longer exists\n"; code != 1 || stderr.String() != want || stdout.Len() != 0 {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", code, stdout.String(), stderr.String(), want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// new refuses a working directory it can no longer enter - its search permission taken away since
// the shell entered it, or that of the directory above it - whether or not PWD is set, which Go's
// Getwd looks at first: given it with -c, tmux 3.7c started claude in the home directory instead,
// and claude --version could not start there, which cld had put down to claude. sh enters the
// directory, takes the permissions away and runs cld there. root enters any directory, so run as
// root the test runs sh and cld as nobody (65534), who owns the directories. On Linux only, as
// TestNewRefusesARemovedDirectory.
func TestNewRefusesADirectoryItCannotEnter(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("a directory that cannot be entered is checked on Linux only")
	}
	for _, test := range []struct {
		name string
		// locked is the directory whose permissions go, relative to the current one.
		locked string
		// pwd is how env passes PWD on to cld.
		pwd string
	}{
		{"PWD set", ".", `PWD="$PWD"`},
		{"PWD unset", ".", "-u PWD"},
		{"above, PWD set", "..", `PWD="$PWD"`},
		{"above, PWD unset", "..", "-u PWD"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			above := filepath.Join(s.Root, "above")
			dir := filepath.Join(above, "dir")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Chmod(above, 0o755)
				_ = os.Chmod(dir, 0o755)
			})
			script := `cd "$1" && chmod 000 "$2" && shift 2 && exec env ` + test.pwd + ` "$0" "$@"`
			cmd := exec.Command("/bin/sh", "-c", script, sandbox.Cld, dir, test.locked, "new", "-n", "x")
			if os.Geteuid() == 0 {
				const nobody = 65534
				for _, path := range []string{s.Root, above, dir} {
					if err := os.Chown(path, nobody, nobody); err != nil {
						t.Fatal(err)
					}
				}
				cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: nobody, Gid: nobody}}
			}
			cmd.Env = s.Environ(map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			})
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			_ = cmd.Run()
			if code, want := cmd.ProcessState.ExitCode(), "cld: cannot enter the current directory: permission denied\n"; code != 1 || stderr.String() != want || stdout.Len() != 0 {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", code, stdout.String(), stderr.String(), want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// socket makes a file where tmux keeps the sandbox's socket of server, for list to find: its
// lookups go to a fake tmux, which answers whatever the file is.
func socket(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(filepath.Join(s.SocketDir(), server), "")
}
