package tests

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net"
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

// Everything cld does before it hands over to tmux; no terminal emulator needed. The tests that
// hand over run cld on a pseudo-terminal of their own (sandbox.RunCldOnTerminal), since join
// refuses to hand over without one.

var update = flag.Bool("update", false, "rewrite the help in testdata/help from what cld help prints")

// helpTopics are what cld help takes, "" for none, in the order the help lists them: a command
// of cld's, followed by the commands it has, "setup telemetry" for setup's telemetry, and theirs,
// "setup completion zsh" for zsh's.
var helpTopics = []string{"", "join", "detach", "kill", "list", "restore", "setup", "setup project", "setup telemetry",
	"setup completion", "setup completion bash", "setup completion zsh", "setup completion fish", "setup restore", "update", "completion",
	"help", "version"}

// goldenHelp is the file holding what cld help topic prints: testdata/help/cld.txt for cld help,
// testdata/help/COMMAND.txt for cld help COMMAND, setup-telemetry.txt for cld help setup
// telemetry, and setup-completion-zsh.txt for cld help setup completion zsh.
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
// the files. cobra wraps nothing, so the texts break their lines by hand, within 80 columns - all
// but cobra's own last line, which names the command - and a usage line names the options before
// the arguments, as cld reads them.
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
				// cobra's last line, which names the command, is its template's, not a text of cld's
				// to break: setup completion's is 81 columns.
				if strings.HasPrefix(line, `Use "cld `) && strings.HasSuffix(line, ` [command] --help" for more information about a command.`) {
					continue
				}
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
// [flags], one in brackets starting with "-", or a plain one starting with "-", as the -s of
// -s SUFFIX, whose value is the word after it. An argument is another word in brackets or in
// capitals, such as [COMMAND], or the words after a "--", which ends the options, such as
// [-- ARGS...]; the others name cld and its command.
func optionsAfterArguments(help string) []string {
	_, usage, _ := strings.Cut(help, "\nUsage:\n")
	usage, _, _ = strings.Cut(usage, "\n\n")
	var late []string
	for _, line := range strings.Split(usage, "\n") {
		argument, value := false, false
		for _, word := range usageWord.FindAllString(line, -1) {
			switch {
			case value:
				value = false
			case word == "--" || strings.HasPrefix(word, "[-- "):
				argument = true
			case word == "[flags]" || strings.HasPrefix(word, "[-") || strings.HasPrefix(word, "-"):
				if argument {
					late = append(late, word)
				}
				value = strings.HasPrefix(word, "-")
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
		{[]string{"help", "join"}, "join"},
		{[]string{"-h", "join"}, "join"},
		{[]string{"--help", "version"}, "version"},
		{[]string{"help", "help"}, "help"},
		{[]string{"join", "--help"}, "join"},
		{[]string{"join", "--resume", "x", "-h"}, "join"},
		{[]string{"join", "-h", "a", "b"}, "join"},
		{[]string{"join", "-s", "x", "-h"}, "join"},
		{[]string{"help", "detach"}, "detach"},
		{[]string{"detach", "-h"}, "detach"},
		{[]string{"detach", "-s", "x", "--help"}, "detach"},
		{[]string{"kill", "--help"}, "kill"},
		{[]string{"list", "-h"}, "list"},
		{[]string{"update", "-h"}, "update"},
		{[]string{"help", "update"}, "update"},
		{[]string{"update", "--help", "x"}, "update"},
		{[]string{"help", "-h"}, "help"},
		{[]string{"version", "--help"}, "version"},
		{[]string{"-V", "-h"}, "version"},
		{[]string{"--version", "--help"}, "version"},
		{[]string{"join", "-h", "-x"}, "join"},
		{[]string{"join", "-h", "-w"}, "join"},
		{[]string{"join", "-h", "--help=x"}, "join"},
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
		{[]string{"help", "setup", "project"}, "setup project"},
		{[]string{"-h", "setup", "project"}, "setup project"},
		{[]string{"setup", "-h", "project"}, "setup project"},
		{[]string{"setup", "project", "-h"}, "setup project"},
		{[]string{"setup", "project", "--help", "x"}, "setup project"},
		{[]string{"setup", "project", "--mcp", "idea", "-h"}, "setup project"},
		{[]string{"setup", "project", "-h", "--bogus"}, "setup project"},
		// setup completion's shells: after it, or after help setup completion; -h after setup
		// completion is its own.
		{[]string{"help", "setup", "completion"}, "setup completion"},
		{[]string{"setup", "-h", "completion"}, "setup completion"},
		{[]string{"setup", "completion", "-h"}, "setup completion"},
		{[]string{"setup", "completion", "--help", "x"}, "setup completion"},
		{[]string{"help", "setup", "completion", "zsh"}, "setup completion zsh"},
		{[]string{"-h", "setup", "completion", "bash"}, "setup completion bash"},
		{[]string{"setup", "completion", "-h", "fish"}, "setup completion fish"},
		{[]string{"setup", "completion", "zsh", "--help"}, "setup completion zsh"},
		{[]string{"setup", "completion", "bash", "-h", "x"}, "setup completion bash"},
		{[]string{"setup", "completion", "fish", "-h", "--no-descriptions"}, "setup completion fish"},
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
// after setup or completion, theirs, and after setup completion, its shells - each with its
// description; the options; the MCP servers of setup project --mcp, after a comma the others,
// and the sets of --permissions; and no file names where nothing is offered
// (":4", ShellCompDirectiveNoFileComp, which cobra reports on stderr), the root's default for an
// argument with nothing to complete, setup telemetry's included.
// Without the word to complete it fails, as cld's other command-line mistakes do.
func TestCompleteCommands(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	commands := "join\tattach to session NAME-SUFFIX, creating or resuming it first\n" +
		"detach\tdetach the terminals attached to session NAME-SUFFIX\n" +
		"kill\tend session NAME-SUFFIX and its tmux server\n" +
		"list\tlist cld's sessions; on a terminal, join or kill one\n" +
		"restore\tbring back the sessions that ran when the machine stopped\n" +
		"setup\tset up claude in a project, telemetry, shell completion or restore\n" +
		"update\tupdate cld to the latest release\n" +
		"version\tshow the version\n" +
		"completion\tprint the completion script for a shell\n" +
		"help\tshow this help, or the help of COMMAND\n"
	project := "project\tset claude up in the project in the current directory\n"
	telemetry := "telemetry\tsend claude's telemetry through a local OpenTelemetry collector\n"
	setupShells := "completion\tset up cld's completion in bash, zsh or fish\n" +
		"restore\thave your systemd run cld restore at login, or at boot\n"
	shellSetups := "bash\tset up cld's completion in bash\n" +
		"zsh\tset up cld's completion in zsh\n" +
		"fish\tset up cld's completion in fish\n"
	goland := "goland\tGoLand's MCP server, port $GOLAND_MCP_PORT or 64422\n"
	jbcontext := "jbcontext\tJetBrains Context's semantic code search, jbcontext mcp\n"
	rider := "rider\tRider's MCP server, port $RIDER_MCP_PORT or 64482\n"
	readOnlySet := "read-only\tread files and run commands that only read, the default\n"
	cldSet := "cld\tcld's own: edit files, run git, go, make, docker and more\n"
	noneSet := "none\tnothing more than claude allows by itself\n"
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
		{[]string{"__complete", "help", "j"}, "join\tattach to session NAME-SUFFIX, creating or resuming it first\n:4\n"},
		{[]string{"__complete", "help", "d"}, "detach\tdetach the terminals attached to session NAME-SUFFIX\n:4\n"},
		{[]string{"__complete", "help", "x"}, ":4\n"},
		{[]string{"__complete", "help", "join", ""}, ":4\n"},
		{[]string{"__complete", "help", "new"}, ":4\n"},
		// After a command with commands of its own, help takes one of those, and nothing after it.
		{[]string{"__complete", "help", "setup", ""}, project + telemetry + setupShells + ":4\n"},
		{[]string{"__complete", "help", "setup", "t"}, telemetry + ":4\n"},
		{[]string{"__complete", "help", "setup", "project", ""}, ":4\n"},
		{[]string{"__complete", "help", "setup", "x"}, ":4\n"},
		{[]string{"__complete", "help", "setup", "telemetry", ""}, ":4\n"},
		{[]string{"__complete", "help", "setup", "completion", ""}, shellSetups + ":4\n"},
		{[]string{"__complete", "help", "setup", "completion", "z"}, "zsh\tset up cld's completion in zsh\n:4\n"},
		{[]string{"__complete", "help", "setup", "completion", "zsh", ""}, ":4\n"},
		{[]string{"__complete", "help", "completion", ""}, shells + ":4\n"},
		{[]string{"__complete", "help", "nope", ""}, ":4\n"},
		{[]string{"__complete", "completion", ""}, shells + ":4\n"},
		{[]string{"__complete", "setup", ""}, project + telemetry + setupShells + ":4\n"},
		{[]string{"__complete", "setup", "p"}, project + ":4\n"},
		{[]string{"__complete", "setup", "c"}, "completion\tset up cld's completion in bash, zsh or fish\n:4\n"},
		{[]string{"__complete", "setup", "r"}, "restore\thave your systemd run cld restore at login, or at boot\n:4\n"},
		{[]string{"__complete", "setup", "restore", ""}, ":4\n"},
		{[]string{"__complete", "restore", ""}, ":4\n"},
		{[]string{"__complete", "setup", "completion", ""}, shellSetups + ":4\n"},
		{[]string{"__complete", "setup", "completion", "f"}, "fish\tset up cld's completion in fish\n:4\n"},
		{[]string{"__complete", "setup", "completion", "bash", ""}, ":4\n"},
		// --mcp offers its servers; after a comma, those the list does not have, after it.
		{[]string{"__complete", "setup", "project", "--m"}, "--mcp\tan MCP `SERVER` for claude in the project: goland or\n:4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", ""}, goland + jbcontext + rider + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "j"}, jbcontext + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp=r"}, rider + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "rider,"}, "rider," + goland + "rider," + jbcontext + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "goland,rider,j"}, "goland,rider," + jbcontext + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "goland,jbcontext,rider,"}, ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "x"}, ":4\n"},
		{[]string{"__completeNoDesc", "setup", "project", "--mcp", "goland,"}, "goland,jbcontext\ngoland,rider\n:4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "goland", ""}, ":4\n"},
		// --permissions offers its sets, the default first.
		{[]string{"__complete", "setup", "project", "--p"}, "--permissions\twhat claude may do in the project without asking:\n:4\n"},
		{[]string{"__complete", "setup", "project", "--permissions", ""}, readOnlySet + cldSet + noneSet + ":4\n"},
		{[]string{"__complete", "setup", "project", "--permissions", "n"}, noneSet + ":4\n"},
		{[]string{"__complete", "setup", "project", "--mcp", "goland", "--permissions=c"}, cldSet + ":4\n"},
		{[]string{"__complete", "setup", "project", "--permissions", "x"}, ":4\n"},
		{[]string{"__completeNoDesc", "setup", "project", "--permissions", ""}, "read-only\ncld\nnone\n:4\n"},
		{[]string{"__complete", "setup", "project", "--permissions", "cld", ""}, ":4\n"},
		// No URL, port or file name is offered, --collector-config's FILE included.
		{[]string{"__complete", "setup", "telemetry", "--l"}, "--local\twhere traces, metrics and logs go, such as\n:4\n"},
		{[]string{"__complete", "setup", "telemetry", "--local", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--port", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--collector-config", ""}, ":4\n"},
		{[]string{"__complete", "setup", "telemetry", "--remote", "https://otel.example.com:4317", ""}, ":4\n"},
		{[]string{"__complete", "completion", "bash", ""}, ":4\n"},
		// cobra describes an option by the first line of its usage, backquotes included.
		{[]string{"__complete", "join", "-"}, "--detach-others\tdetach any other terminal attached to the session\n" +
			"--fork\twith --resume, resume a copy of SESSION under a new\n" +
			"--help\thelp for join\n-h\thelp for join\n" +
			"--name\tthe session's `NAME`, before -SUFFIX: by default the\n" +
			"-n\tthe session's `NAME`, before -SUFFIX: by default the\n" +
			"--new\tcreate a session that has ended anew, with a new\n" +
			"--resume\tcreate the session with claude resuming `SESSION`,\n" +
			"--suffix\tthe session's `SUFFIX`, after NAME-: by default the index\n" +
			"-s\tthe session's `SUFFIX`, after NAME-: by default the index\n" +
			"--worktree\tcreate the session with claude in git worktree\n" +
			"-w\tcreate the session with claude in git worktree\n:4\n"},
		{[]string{"__complete", "detach", "-"}, "--help\thelp for detach\n-h\thelp for detach\n" +
			"--name\tthe session's `NAME`, before -SUFFIX: by default the\n" +
			"-n\tthe session's `NAME`, before -SUFFIX: by default the\n" +
			"--suffix\tthe session's `SUFFIX`, after NAME-\n" +
			"-s\tthe session's `SUFFIX`, after NAME-\n:4\n"},
		{[]string{"__complete", "join", "--f"}, "--fork\twith --resume, resume a copy of SESSION under a new\n:4\n"},
		{[]string{"__complete", "join", "--resume", ""}, ":4\n"},
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
	for _, args := range [][]string{{}, {""}, {"", "join"}} {
		result := s.RunCld(nil, args...)
		if result.Code != 2 || !strings.HasPrefix(result.Stderr, "cld: missing command") || result.Stdout != "" {
			t.Errorf("cld %q: exit %d, stdout %q, stderr %q", args, result.Code, result.Stdout, result.Stderr)
		}
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// "cld NAME" created or attached to session NAME before cld had commands, and new and resume were
// commands until join took them over: each is an unknown command now, with no pointer to join.
// The first argument is the command, whatever follows: an option before it is no command either.
// -v is a command only elsewhere.
func TestRejectsUnknownCommands(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"review"}, "cld: unknown command 'review' (see cld help)\n"},
		{[]string{"new"}, "cld: unknown command 'new' (see cld help)\n"},
		{[]string{"resume", "-s", "x"}, "cld: unknown command 'resume' (see cld help)\n"},
		{[]string{"-x"}, "cld: unknown command '-x' (see cld help)\n"},
		{[]string{"a.b"}, "cld: unknown command 'a.b' (see cld help)\n"},
		{[]string{"-v"}, "cld: unknown command '-v' (see cld help)\n"},
		{[]string{"-n", "x", "join"}, "cld: unknown command '-n' (see cld help)\n"},
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
		for _, args := range [][]string{{"join", "-n", name}, {"join", "--resume", "x", "-n", name}, {"join", "--name", name}, {"detach", "-n", name}, {"kill", "-n", name}, {"join", "--name=" + name}} {
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
			if result, want := s.RunCld(locale, "join", "-n", name), "cld: invalid name '"+name+"' (see cld help)\n"; result.Code != 2 || result.Stderr != want {
				t.Errorf("join -n %s: exit %d, stderr %q, want exit 2, stderr %q", name, result.Code, result.Stderr, want)
			}
			if result, want := s.RunCld(locale, name), "cld: unknown command '"+name+"' (see cld help)\n"; result.Code != 2 || result.Stderr != want {
				t.Errorf("%s: exit %d, stderr %q, want exit 2, stderr %q", name, result.Code, result.Stderr, want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
		})
	}
}

// A name has at most 64 characters, so that the path of its server's socket fits in sun_path (see
// MaxName in internal/session): one of 65 is refused, saying so, whatever its characters; one of 64
// goes as far as tmux, whose server is named like the session, with --resume and without. The
// length is counted in characters, not bytes: 33 "é" are 66 bytes, and invalid for the "é".
func TestNameLength(t *testing.T) {
	t.Parallel()
	longest, tooLong := strings.Repeat("n", 64), strings.Repeat("n", 65)
	accents, tooManyAccents := strings.Repeat("é", 33), strings.Repeat("é", 65)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-n", tooLong}, "cld: name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "--name", tooLong}, "cld: name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"kill", "-n", tooLong}, "cld: name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"detach", "--name", tooLong}, "cld: name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "-n", tooLong, "--resume", "SESSION"}, "cld: name '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "-n", tooLong[:60] + "a.b.c"}, "cld: name '" + tooLong[:60] + "a.b.c' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "-n", tooLong[:60] + "a.b"}, "cld: invalid name '" + tooLong[:60] + "a.b' (see cld help)\n"},
		{[]string{"join", "-n", accents}, "cld: invalid name '" + accents + "' (see cld help)\n"},
		{[]string{"join", "-n", tooManyAccents}, "cld: name '" + tooManyAccents + "' is longer than 64 characters (see cld help)\n"},
		{[]string{tooLong}, "cld: unknown command '" + tooLong + "' (see cld help)\n"},
		{[]string{longest}, "cld: unknown command '" + longest + "' (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, test.args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
		})
	}
	// Where the directory's name leaves nothing, -s SUFFIX is the whole name.
	for _, args := range [][]string{{"join", "-s", longest}, {"join", "-s", longest, "--resume", "SESSION"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCldOnTerminal(map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}, args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:3], []string{"-u", "-L", "cld-" + longest}) {
				t.Errorf("tmux arguments start %q, want -u -L cld-%s", argv[:3], longest)
			}
		})
	}
}

// join, detach and kill name their session NAME-SUFFIX with -n NAME and -s SUFFIX; kill needs -s -
// detach too, outside a pane of cld's servers, as here, -n alone included. join's --fork needs
// --resume SESSION, whose copy would otherwise take the name of the conversation it copies, and
// one other than the name -n and -s make, in any case (TestForkOwnName has the name the
// directory or the index makes); --resume takes a SESSION that is not empty and does not start
// with "-", which claude would read as an option, and goes neither with --new nor with -w. -n is
// checked first, then -s, each its length first, whatever its characters, then its characters -
// an empty one and one of spaces are invalid too; SUFFIX is checked as NAME is, since where the
// directory's name leaves nothing it is the whole name; then join's options. Each is a mistake on
// the command line, exit status 2, found before any tool is looked for: the PATH has none here.
func TestNameOptions(t *testing.T) {
	t.Parallel()
	tooLong := strings.Repeat("s", 65)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"kill"}, "cld: kill: missing -s SUFFIX (see cld list)\n"},
		{[]string{"kill", "--name", "x"}, "cld: kill: missing -s SUFFIX (see cld list)\n"},
		{[]string{"detach"}, "cld: detach: missing -s SUFFIX (see cld list)\n"},
		{[]string{"detach", "-n", "x"}, "cld: detach: missing -s SUFFIX (see cld list)\n"},
		{[]string{"detach", "-s", "a.b"}, "cld: invalid suffix 'a.b' (see cld help)\n"},
		{[]string{"join", "--fork"}, "cld: join: --fork needs --resume SESSION, the conversation to copy (see cld help)\n"},
		{[]string{"join", "-s", "x", "--fork"}, "cld: join: --fork needs --resume SESSION, the conversation to copy (see cld help)\n"},
		{[]string{"join", "--fork", "-n", "x"}, "cld: join: --fork needs --resume SESSION, the conversation to copy (see cld help)\n"},
		{[]string{"join", "--fork", "-s", "a.b"}, "cld: invalid suffix 'a.b' (see cld help)\n"},
		{[]string{"join", "-n", "a", "-s", "x", "--fork", "--resume", "cld-a-x"},
			"cld: join: --fork would give the copy SESSION's own name, cld-a-x; give another -s SUFFIX (see cld help)\n"},
		{[]string{"join", "-s", "x", "-n", "A", "--fork", "--resume", " CLD-a-X "},
			"cld: join: --fork would give the copy SESSION's own name, cld-A-x; give another -s SUFFIX (see cld help)\n"},
		{[]string{"join", "--resume"}, "cld: option '--resume' needs a value (see cld help)\n"},
		{[]string{"join", "--resume", ""}, "cld: option '--resume' needs a value (see cld help)\n"},
		{[]string{"join", "--resume=", "-s", "x"}, "cld: option '--resume' needs a value (see cld help)\n"},
		{[]string{"join", "--resume", "-p"}, "cld: invalid SESSION '-p' for --resume: claude would read it as an option (see cld help)\n"},
		{[]string{"join", "--resume=-", "--fork"}, "cld: invalid SESSION '-' for --resume: claude would read it as an option (see cld help)\n"},
		{[]string{"join", "--switched-from", "a.b"}, "cld: invalid session 'a.b' for --switched-from (see cld help)\n"},
		{[]string{"join", "--moved=" + moved("/"), "-s", "y"}, "cld: join: --moved goes with --switched-from alone (see cld help)\n"},
		{[]string{"join", "--moved=!"}, "cld: invalid value '!' for --moved (see cld help)\n"},
		{[]string{"join", "--switched-from", "a", "--moved=" + moved("_", "-s", "x")}, "cld: invalid value '" + moved("_", "-s", "x") + "' for --moved (see cld help)\n"},
		{[]string{"join", "--switched-from", "a", "--moved=" + moved("/", "-s", "a.b")}, "cld: invalid suffix 'a.b' (see cld help)\n"},
		{[]string{"list", "--to", "next"}, "cld: list: --to needs --switch CLIENT (see cld help)\n"},
		{[]string{"list", "--switch", "/dev/pts/0", "--to", "up"}, "cld: invalid value 'up' for --to: previous, next or last (see cld help)\n"},
		{[]string{"list", "--switch", ""}, "cld: option '--switch' needs a value (see cld help)\n"},
		{[]string{"join", "--new", "--resume", "x"},
			"cld: join: --new and --resume exclude each other: each says which conversation claude starts with (see cld help)\n"},
		{[]string{"join", "--resume", "x", "-w", "-s", "y"},
			"cld: join: -w and --resume exclude each other: claude takes a conversation back to its worktree itself (see cld help)\n"},
		{[]string{"join", "--resume", "x", "--fork", "--new"},
			"cld: join: --new and --resume exclude each other: each says which conversation claude starts with (see cld help)\n"},
		{[]string{"kill", "-n", "", "-s", ""}, "cld: invalid name '' (see cld help)\n"},
		{[]string{"join", "-n", "a.b"}, "cld: invalid name 'a.b' (see cld help)\n"},
		{[]string{"join", "-n", "x", "-s", "a b"}, "cld: invalid suffix 'a b' (see cld help)\n"},
		{[]string{"join", "-s", ""}, "cld: invalid suffix '' (see cld help)\n"},
		{[]string{"join", "-s", " "}, "cld: invalid suffix ' ' (see cld help)\n"},
		{[]string{"join", "-s", "a b", "--resume", "-p"}, "cld: invalid suffix 'a b' (see cld help)\n"},
		{[]string{"join", "-s", "-x"}, "cld: invalid suffix '-x' (see cld help)\n"},
		{[]string{"join", "--suffix=_x"}, "cld: invalid suffix '_x' (see cld help)\n"},
		{[]string{"kill", "-s", "a.b"}, "cld: invalid suffix 'a.b' (see cld help)\n"},
		{[]string{"join", "-s", "café", "--resume", "SESSION"}, "cld: invalid suffix 'café' (see cld help)\n"},
		{[]string{"join", "-s", tooLong}, "cld: suffix '" + tooLong + "' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "-s", tooLong[:60] + "a.b.c"}, "cld: suffix '" + tooLong[:60] + "a.b.c' is longer than 64 characters (see cld help)\n"},
		{[]string{"join", "-s"}, "cld: option '-s' needs a value (see cld help)\n"},
		{[]string{"join", "--suffix"}, "cld: option '--suffix' needs a value (see cld help)\n"},
		{[]string{"join", "-n", strings.Repeat("n", 62), "-s", "ab"},
			"cld: session name '" + strings.Repeat("n", 62) + "-ab' is longer than 64 characters; give a shorter -n NAME or -s SUFFIX (see cld help)\n"},
		{[]string{"kill", "-s", strings.Repeat("s", 30), "-n", strings.Repeat("n", 34)},
			"cld: session name '" + strings.Repeat("n", 34) + "-" + strings.Repeat("s", 30) + "' is longer than 64 characters; give a shorter -n NAME or -s SUFFIX (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...)
			if result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
		})
	}
}

// moved is the value of join's --moved for a join run in directory dir with words after it, as
// the command a terminal runs as that join moves it has it: dir and the words, each after a NUL,
// in URL-safe base64 without padding (see TestJoinMovesTheTerminal).
func moved(dir string, words ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(append([]string{dir}, words...), "\x00")))
}

// The terminal that a join in a pane moved runs join --moved, which enters the directory that join
// ran in: where it has been removed since, the terminal's join refuses, status 1, naming it, before
// any tool is looked for - the PATH has none here - the terminal having left its session.
func TestMovedToARemovedDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Work, "removed")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(map[string]string{"PATH": s.Tools()}, "join", "--switched-from", "a", "--moved="+moved(dir, "-s", "x"))
	want := "cld: cannot enter " + dir + ", where cld join ran: no such file or directory\n"
	if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
}

// A session's name that the name of the repository, or outside one of the directory, or join's
// index makes with -n or -s has at most 64 characters too: past them it is refused, with exit
// status 1 - that name or the index decides it, not the command line alone - pointing at -n and
// -s. The fake tmux finds no server, and gets a name of 64 characters. (TestNameOptions has the
// names -n and -s make together, too long with status 2.)
func TestLongPrefix(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		// length is that of the name of the repository, or where repository is false of the
		// directory, in no repository
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
	} {
		repository := strings.Repeat("r", test.length)
		name := repository + test.suffix
		if test.suffix == "" {
			name = test.args[len(test.args)-1] + "-0"
		}
		where := "repository"
		if !test.repository {
			where = "directory"
		}
		t.Run(where+" of "+strconv.Itoa(test.length)+", "+strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.repository {
				runGit(t, s, s.Root, "init", "-q", repository)
			} else if err := os.Mkdir(filepath.Join(s.Root, repository), 0o755); err != nil {
				t.Fatal(err)
			}
			result := s.RunCldOnTerminalIn(filepath.Join(s.Root, repository), map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}, test.args...)
			if test.ok {
				if result.Code != 0 || result.Stderr != "" {
					t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
				}
				if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:3], []string{"-u", "-L", "cld-" + name}) {
					t.Errorf("tmux arguments start %q, want -u -L cld-%s", argv[:3], name)
				}
				return
			}
			want := "cld: session name '" + name + "' is longer than 64 characters; give a shorter -n NAME or -s SUFFIX (see cld help)\n"
			if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("tmux started")
			}
		})
	}
}

// Under a TMUX_TMPDIR longer than tmux's default directories, a name within the 64 characters can
// still make the socket path too long for sun_path. tmux says so, and cld ends with its message,
// rather than take it for no server running: join before it would create the session, which
// would fail the same way, and set the title. list finds no socket.
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
		{[]string{"join", "-s", name}, want},
		{[]string{"join", "-s", name, "--resume", "SESSION"}, want},
		{[]string{"kill", "-s", name}, want},
		{[]string{"detach", "-s", name}, want},
		{[]string{"list"}, ""},
	} {
		result := s.RunCld(map[string]string{"TMUX_TMPDIR": dir}, test.args...)
		if code := min(len(test.want), 1); result.Code != code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q", test.args[0], result.Code, result.Stdout, result.Stderr, code, test.want)
		}
	}
}

// list reads tmux's socket directory to find the servers, and one it cannot read ends it with
// the reason, rather than show no session: here a file where tmux-UID would be. So does join
// without -s, which reads it for the next index. join, detach and kill with -s SUFFIX end with
// tmux's message, as for a socket path too long (tmux 3.3a to 3.7c). Where
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
		{[]string{"join"}, "cld: cannot read " + s.SocketDir() + ": not a directory\n"},
		{[]string{"join", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"kill", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"detach", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", test.args[0], result.Code, result.Stdout, result.Stderr, test.want)
		}
	}
}

// tmux refuses a socket directory that others can use, before it connects to a socket there, and
// cld ends with its message: where tmux would not get as far, cld takes neither a stale socket nor
// a missing one for no server, as it does without running tmux elsewhere (see
// TestStaleSocketsRunNoTmux). Here tmux-UID, open to others, holds 12 stale sockets, cld-0 to
// cld-11: list asks no more servers once they fail, and so no more than the eight it asks at once,
// which a tmux first on the PATH writes down.
func TestUnsafeSocketDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for i := range 12 {
		staleSocket(t, s, "cld-"+strconv.Itoa(i))
	}
	if err := os.Chmod(s.SocketDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	logged, asked := loggedTmux(t, s)
	want := "cld: directory " + s.SocketDir() + " has unsafe permissions\n"
	for _, args := range [][]string{{"list"}, {"join"}, {"join", "-s", "a"}, {"join", "-s", "0"}, {"detach", "-s", "1"}, {"kill", "-s", "a"}} {
		if result := s.RunCld(logged, args...); result.Code != 1 || result.Stdout != "" || result.Stderr != want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", strings.Join(args, " "), result.Code, result.Stdout, result.Stderr, want)
		}
		if got := asked(); args[0] == "list" && len(got) > 8 {
			t.Errorf("list asked %d servers %q, want 8 at most", len(got), got)
		}
	}
}

// Arguments are read left to right, and the first wrong one decides the message: an option after
// an argument is not read, and -- ends nothing but the options of join, whose words after it are
// claude's (see TestRefusesClaudeOptions). help and version are named as typed, completion's
// commands with their SHELL. help takes one argument, a command of cld's, new and resume no longer
// among them; join takes none but the words after "--", its --resume SESSION an option's value;
// and completion none but a SHELL, as its command.
func TestRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "review"}, "cld: join: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "-x"}, "cld: join: unexpected argument '-x' (see cld help)\n"},
		{[]string{"join", "-n"}, "cld: option '-n' needs a value (see cld help)\n"},
		{[]string{"join", "--name"}, "cld: option '--name' needs a value (see cld help)\n"},
		{[]string{"kill", "a"}, "cld: kill: unexpected argument 'a' (see cld help)\n"},
		{[]string{"detach", "-w"}, "cld: detach: unexpected argument '-w' (see cld help)\n"},
		{[]string{"kill", "-n", "a", "--worktree"}, "cld: kill: unexpected argument '--worktree' (see cld help)\n"},
		{[]string{"kill", "--new"}, "cld: kill: unexpected argument '--new' (see cld help)\n"},
		{[]string{"detach", "--resume", "x"}, "cld: detach: unexpected argument '--resume' (see cld help)\n"},
		{[]string{"list", "-n", "a"}, "cld: list: unexpected argument '-n' (see cld help)\n"},
		{[]string{"update", "x"}, "cld: update: unexpected argument 'x' (see cld help)\n"},
		{[]string{"update", "--check"}, "cld: update: unexpected argument '--check' (see cld help)\n"},
		{[]string{"update", "--"}, "cld: update: unexpected argument '--' (see cld help)\n"},
		{[]string{"help", "nope"}, "cld: help: unknown command 'nope' (see cld help)\n"},
		{[]string{"help", "new"}, "cld: help: unknown command 'new' (see cld help)\n"},
		{[]string{"help", "resume"}, "cld: help: unknown command 'resume' (see cld help)\n"},
		{[]string{"help", "join", "kill"}, "cld: help: unexpected argument 'kill' (see cld help)\n"},
		{[]string{"help", "nope", "join"}, "cld: help: unknown command 'nope' (see cld help)\n"},
		{[]string{"help", "-V"}, "cld: help: unexpected argument '-V' (see cld help)\n"},
		{[]string{"help", ""}, "cld: help: unknown command '' (see cld help)\n"},
		{[]string{"help", "--", "join"}, "cld: help: unexpected argument '--' (see cld help)\n"},
		{[]string{"help", "join", "--"}, "cld: help: unexpected argument '--' (see cld help)\n"},
		{[]string{"help", "join", "-h"}, "cld: help: unexpected argument '-h' (see cld help)\n"},
		{[]string{"help", "join", "telemetry"}, "cld: help: unexpected argument 'telemetry' (see cld help)\n"},
		{[]string{"help", "setup", "nope"}, "cld: help: unknown command 'setup nope' (see cld help)\n"},
		{[]string{"help", "setup", "telemetry", "x"}, "cld: help: unexpected argument 'x' (see cld help)\n"},
		{[]string{"help", "setup", "project", "telemetry"}, "cld: help: unexpected argument 'telemetry' (see cld help)\n"},
		{[]string{"help", "project"}, "cld: help: unknown command 'project' (see cld help)\n"},
		{[]string{"help", "telemetry"}, "cld: help: unknown command 'telemetry' (see cld help)\n"},
		{[]string{"help", "completion", "tcsh"}, "cld: help: unknown command 'completion tcsh' (see cld help)\n"},
		{[]string{"help", "completion", "bash", "x"}, "cld: help: unexpected argument 'x' (see cld help)\n"},
		{[]string{"-h", "-n", "x"}, "cld: -h: unexpected argument '-n' (see cld help)\n"},
		{[]string{"--help", "x"}, "cld: --help: unknown command 'x' (see cld help)\n"},
		{[]string{"version", "-n", "a"}, "cld: version: unexpected argument '-n' (see cld help)\n"},
		{[]string{"-V", "x"}, "cld: -V: unexpected argument 'x' (see cld help)\n"},
		{[]string{"join", "review", "--", "-p"}, "cld: join: unexpected argument 'review' (see cld help)\n"},
		{[]string{"kill", "--", "-x"}, "cld: kill: unexpected argument '--' (see cld help)\n"},
		{[]string{"list", "--"}, "cld: list: unexpected argument '--' (see cld help)\n"},
		{[]string{"join", "a", "-x"}, "cld: join: unexpected argument 'a' (see cld help)\n"},
		{[]string{"join", "review", "-n"}, "cld: join: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "review", "-h"}, "cld: join: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "-d"}, "cld: join: unexpected argument '-d' (see cld help)\n"},
		{[]string{"join", "--detach-others=maybe"}, "cld: join: unexpected argument '--detach-others=maybe' (see cld help)\n"},
		{[]string{"join", "--detach-others", "a"}, "cld: join: unexpected argument 'a' (see cld help)\n"},
		{[]string{"join", "--new=maybe"}, "cld: join: unexpected argument '--new=maybe' (see cld help)\n"},
		{[]string{"join", "--resume", "a", "b"}, "cld: join: unexpected argument 'b' (see cld help)\n"},
		{[]string{"join", "--resume", "a", "b", "--", "-p"}, "cld: join: unexpected argument 'b' (see cld help)\n"},
		{[]string{"join", "x", "-n", "y"}, "cld: join: unexpected argument 'x' (see cld help)\n"},
		{[]string{"join", "x", "-h", "--"}, "cld: join: unexpected argument 'x' (see cld help)\n"},
		{[]string{"join", "-n", "x", ""}, "cld: join: unexpected argument '' (see cld help)\n"},
		{[]string{"join", "", "--", "-p"}, "cld: join: unexpected argument '' (see cld help)\n"},
		{[]string{"join", "-"}, "cld: join: unexpected argument '-' (see cld help)\n"},
		{[]string{"join", "x", "--fork"}, "cld: join: unexpected argument 'x' (see cld help)\n"},
		{[]string{"join", "--fork=maybe", "--resume", "x"}, "cld: join: unexpected argument '--fork=maybe' (see cld help)\n"},
		{[]string{"join", "-f", "x"}, "cld: join: unexpected argument '-f' (see cld help)\n"},
		{[]string{"detach", "x"}, "cld: detach: unexpected argument 'x' (see cld help)\n"},
		{[]string{"detach", "-s", "x", "y"}, "cld: detach: unexpected argument 'y' (see cld help)\n"},
		{[]string{"detach", "--detach-others"}, "cld: detach: unexpected argument '--detach-others' (see cld help)\n"},
		{[]string{"detach", "--others"}, "cld: detach: unexpected argument '--others' (see cld help)\n"},
		{[]string{"detach", "--"}, "cld: detach: unexpected argument '--' (see cld help)\n"},
		// An empty argument is one too (cld's shell script took it for none).
		{[]string{"list", ""}, "cld: list: unexpected argument '' (see cld help)\n"},
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
		{[]string{"setup", "completion", "zsh", "x"}, "cld: setup completion zsh: unexpected argument 'x' (see cld help)\n"},
		{[]string{"setup", "completion", "bash", "--"}, "cld: setup completion bash: unexpected argument '--' (see cld help)\n"},
		{[]string{"setup", "completion", "fish", "--no-descriptions"}, "cld: setup completion fish: unexpected argument '--no-descriptions' (see cld help)\n"},
		{[]string{"setup", "completion", "zsh", "x", "-h"}, "cld: setup completion zsh: unexpected argument 'x' (see cld help)\n"},
		{[]string{"help", "setup", "completion", "tcsh"}, "cld: help: unknown command 'setup completion tcsh' (see cld help)\n"},
		{[]string{"help", "setup", "completion", "zsh", "x"}, "cld: help: unexpected argument 'x' (see cld help)\n"},
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

// join gives claude the words after "--", but for the options of claude's that would undo what
// cld gives claude - its name, its worktree, its settings - those that resume a conversation, and
// those with which claude would not stay in the session, printing and exiting or leaving the pane:
// each is refused, named as given, with why, whether join would create the session, bring it back
// or attach to it, which it cannot tell before it looks the session up. A short option counts at
// the start of a word, with a value or more options after it, a long one with a value after "="
// too, and each word counts, whatever comes before it, a second "--" too. These are mistakes on
// the command line, exit status 2, found before any tool is looked for: the PATH has none here.
func TestRefusesClaudeOptions(t *testing.T) {
	t.Parallel()
	const (
		naming      = "cld gives claude the session's name, which -n and -s make"
		worktree    = "cld gives claude --worktree with -w, before --"
		resumes     = "cld gives claude --resume with --resume SESSION, before --"
		resumed     = "cld join resumes the session's conversation, or --resume SESSION"
		ownSettings = "cld gives claude --settings, which this one would replace"
		answer      = "claude would print its answer and exit, ending the session"
		background  = "claude would start in the background and exit, ending the session"
		ownTmux     = "claude would move to a tmux session of its own"
		teleport    = "claude would resume a session from Claude Code on the web instead"
		initOnly    = "claude would run its startup hooks and exit, ending the session"
		rewind      = "claude would restore files and exit, ending the session"
		helps       = "claude would print its help and exit, ending the session"
		versions    = "claude would print its version and exit, ending the session"
	)
	for _, test := range []struct {
		args []string
		// word is the word refused, and why the reason
		word, why string
	}{
		{[]string{"join", "--", "-n", "x"}, "-n", naming},
		{[]string{"join", "--", "-nx"}, "-nx", naming},
		{[]string{"join", "--", "--name", "x"}, "--name", naming},
		{[]string{"join", "--", "--name=x"}, "--name=x", naming},
		{[]string{"join", "-s", "x", "--", "-n", "x"}, "-n", naming},
		{[]string{"join", "--", "-w"}, "-w", worktree},
		{[]string{"join", "-w", "--", "--worktree=y"}, "--worktree=y", worktree},
		{[]string{"join", "-s", "x", "--", "--worktree"}, "--worktree", worktree},
		{[]string{"join", "--resume", "a", "--", "-wy"}, "-wy", worktree},
		{[]string{"join", "--", "-r", "x"}, "-r", resumes},
		{[]string{"join", "--", "--resume=x"}, "--resume=x", resumes},
		{[]string{"join", "--", "-c"}, "-c", resumed},
		{[]string{"join", "--new", "--", "--continue"}, "--continue", resumed},
		{[]string{"join", "-s", "x", "--", "-r", "y"}, "-r", resumes},
		{[]string{"join", "--resume", "a", "--", "--resume"}, "--resume", resumes},
		{[]string{"join", "-s", "x", "--", "-c"}, "-c", resumed},
		{[]string{"join", "--detach-others", "--", "--continue"}, "--continue", resumed},
		{[]string{"join", "--", "--from-pr", "12"}, "--from-pr", resumed},
		{[]string{"join", "-s", "x", "--", "--from-pr=12"}, "--from-pr=12", resumed},
		{[]string{"join", "--", "--settings", "{}"}, "--settings", ownSettings},
		{[]string{"join", "--resume", "a", "--", "--settings=s.json"}, "--settings=s.json", ownSettings},
		{[]string{"join", "--", "-p", "hi"}, "-p", answer},
		{[]string{"join", "--", "--print"}, "--print", answer},
		{[]string{"join", "--", "--model", "opus", "-pc"}, "-pc", answer},
		{[]string{"join", "-n", "x", "--", "-p"}, "-p", answer},
		{[]string{"join", "--", "--bg"}, "--bg", background},
		{[]string{"join", "--", "--background"}, "--background", background},
		{[]string{"join", "--resume", "a", "--", "--bg=1"}, "--bg=1", background},
		{[]string{"join", "-w", "--", "--tmux"}, "--tmux", ownTmux},
		{[]string{"join", "--", "--tmux=classic"}, "--tmux=classic", ownTmux},
		{[]string{"join", "--", "--teleport"}, "--teleport", teleport},
		{[]string{"join", "-s", "x", "--", "--teleport=id"}, "--teleport=id", teleport},
		{[]string{"join", "--", "--init-only"}, "--init-only", initOnly},
		{[]string{"join", "--resume", "a", "--", "--rewind-files", "id"}, "--rewind-files", rewind},
		{[]string{"join", "--", "--rewind-files=id"}, "--rewind-files=id", rewind},
		{[]string{"join", "--", "-h"}, "-h", helps},
		{[]string{"join", "--", "--help"}, "--help", helps},
		{[]string{"join", "--resume", "a", "--", "-v"}, "-v", versions},
		{[]string{"join", "--", "--version"}, "--version", versions},
		// The first word refused decides, wherever it is.
		{[]string{"join", "--", "--append-system-prompt", "-p", "-n", "x"}, "-p", answer},
		{[]string{"join", "--", "prompt", "--", "--tmux"}, "--tmux", ownTmux},
		{[]string{"join", "--resume", "a", "--", "--", "--bg"}, "--bg", background},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			want := "cld: " + test.args[0] + ": '" + test.word + "' after --: " + test.why + " (see cld help)\n"
			if result := s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...); result.Code != 2 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
		})
	}
}

// setup takes one of its commands, project, telemetry, completion or restore, as its first
// argument, which
// run checks as it checks cld's first: no option comes before it (cobra would run telemetry for
// setup --local URL telemetry), and help can be asked for with -h or --help only, as for cld.
// setup completion takes a shell the same way, bash, zsh or fish (cobra would run zsh's for setup
// completion --help=false zsh). Nothing is written, in the work directory or the home directory.
func TestSetupRequiresCommand(t *testing.T) {
	t.Parallel()
	const (
		telemetry = "cld setup project, cld setup telemetry, cld setup completion SHELL or cld setup restore (see cld help)\n"
		shells    = "bash, zsh or fish (see cld help)\n"
	)
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
		{[]string{"setup", "--mcp", "goland", "project"}, "cld: setup: unknown command '--mcp': " + telemetry},
		{[]string{"setup", "Project"}, "cld: setup: unknown command 'Project': " + telemetry},
		{[]string{"setup", "completion"}, "cld: setup completion: missing shell: " + shells},
		{[]string{"setup", "completion", ""}, "cld: setup completion: missing shell: " + shells},
		{[]string{"setup", "completion", "", "zsh"}, "cld: setup completion: missing shell: " + shells},
		{[]string{"setup", "completion", "tcsh"}, "cld: setup completion: unknown shell 'tcsh': " + shells},
		{[]string{"setup", "completion", "powershell"}, "cld: setup completion: unknown shell 'powershell': " + shells},
		{[]string{"setup", "completion", "Zsh"}, "cld: setup completion: unknown shell 'Zsh': " + shells},
		{[]string{"setup", "completion", "--help=false", "zsh"}, "cld: setup completion: unknown shell '--help=false': " + shells},
		{[]string{"setup", "completion", "--no-descriptions", "bash"}, "cld: setup completion: unknown shell '--no-descriptions': " + shells},
		{[]string{"setup", "completion", "--", "fish"}, "cld: setup completion: unknown shell '--': " + shells},
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
			if entries, err := os.ReadDir(s.Work); err != nil || len(entries) != 0 {
				t.Errorf("the work directory holds %v (%v), want nothing", entries, err)
			}
			if entries, err := os.ReadDir(s.Home); err != nil || len(entries) != 1 {
				t.Errorf("the home directory holds %v (%v), want .tmux.conf alone", entries, err)
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
			s.WriteProgram(docker, broken.content, broken.mode)
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

// join needs tmux, claude where it starts claude, and with -w git; the other commands only tmux:
// the fake tmux finds no session, so detach and kill get as far as saying so, and join as far as
// the claude it would start.
func TestRequiresTools(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		present []string
		want    string
	}{
		{[]string{"join"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"join", "-w"}, []string{"tmux", "claude"}, "cld: git is not installed\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"tmux"}, "cld: git is not installed\n"},
		{[]string{"join", "--resume", "SESSION"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"kill", "-s", "x"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"kill", "-s", "x"}, []string{"tmux"}, "cld: no session 'x' (see cld list)\n"},
		{[]string{"detach", "-s", "x"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"detach", "-s", "x"}, []string{"tmux"}, "cld: no session 'x' (see cld list)\n"},
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
// working directory has a tmux, a claude and a git of its own, which fail, saying so, if run. join
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
		{[]string{"join", "-s", "main"}, nil, 1, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"tmux"}, 1, "cld: claude is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, 1, "cld: claude is not installed\n"},
		{[]string{"join", "-w"}, []string{"tmux", "claude"}, 1, "cld: git is not installed\n"},
		// tmux, claude --version and git, run from the absolute entry, find no session, a version
		// that passes and the repository.
		{[]string{"join", "-w"}, []string{"tmux", "claude", "git"}, 0, ""},
	} {
		for _, relative := range []string{".", ""} {
			t.Run(strings.Join(test.args, " ")+" "+strings.Join(test.present, ",")+" after '"+relative+"'", func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				gitInit(t, s)
				for _, name := range []string{"tmux", "claude", "git"} {
					script := "#!/bin/sh\necho \"relative " + name + " ran\" >&2\nexit 99\n"
					if err := os.WriteFile(filepath.Join(s.Work, name), []byte(script), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				tools := s.Tools(test.present...)
				path := relative + string(os.PathListSeparator) + tools
				if result := s.RunCldOnTerminal(map[string]string{"PATH": path}, test.args...); result.Code != test.code || result.Stderr != test.want {
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

// cld requires the oldest tmux its tests run on, 3.5a. A letter, a bug-fix release, counts after
// the major and minor version, so 3.5 is older; development builds are read from what follows
// "next-", and pass without a version.
func TestRequiresTmux(t *testing.T) {
	t.Parallel()
	for version, accepted := range map[string]bool{
		"tmux 3.5a":     true,
		"tmux 3.5b":     true,
		"tmux 3.6":      true,
		"tmux 3.6b":     true,
		"tmux 3.7c":     true,
		"tmux 3.10":     true,
		"tmux 4.0":      true,
		"tmux next-3.9": true,
		"tmux 3.8-rc2":  true,
		"tmux master":   true,
		"tmux 3.5":      false,
		"tmux 3.5-rc":   false,
		"tmux 3.4":      false,
		"tmux 3.3a":     false,
		"tmux 2.9a":     false,
		"tmux next-3.5": false,
	} {
		for _, args := range [][]string{{"join"}, {"join", "--resume", "SESSION"}} {
			t.Run(strings.Join(args, " ")+" "+version, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCldOnTerminal(map[string]string{
					"PATH":                  s.Tools("tmux", "claude"),
					"CLD_FAKE_TMUX_VERSION": version,
				}, args...)
				_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
				started := err == nil
				if accepted && (result.Code != 0 || !started) {
					t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
				}
				if want := "cld: tmux 3.5a or newer is required, found '" + version + "'\n"; !accepted &&
					(result.Code != 1 || result.Stderr != want || started) {
					t.Errorf("accepted: exit %d, stderr %q, tmux started: %v", result.Code, result.Stderr, started)
				}
			})
		}
	}
}

// join requires claude 2.1.232 where it starts claude, the first release that does what cld passes
// and relies on, resuming included, comparing the numbers claude --version starts with as numbers:
// 2.1.30 is older. Output that does not start with a version passes. An older claude - 2.1.222, the
// minimum before resume, too - is refused before tmux starts. The probe answers --version without
// leaving a record of a claude: the fake tmux starts none.
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
		for _, args := range [][]string{{"join"}, {"join", "--resume", "SESSION"}} {
			t.Run(strings.Join(args, " ")+" "+version, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				result := s.RunCldOnTerminal(map[string]string{
					"PATH":                    s.Tools("tmux", "claude"),
					"CLD_FAKE_TMUX_VERSION":   "tmux 3.7c",
					"CLD_FAKE_CLAUDE_VERSION": version,
				}, args...)
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
			} else {
				s.WriteProgram(claude, test.script, 0o755)
			}
			result := s.RunCld(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "join")
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
// wrapper's update check, say - holds join up for a second at most, not for as long as that runs,
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
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
			t.Cleanup(func() {
				if pid, err := os.ReadFile(behind); err == nil {
					_ = exec.Command("kill", strings.TrimSpace(string(pid))).Run()
				}
			})
			start := time.Now()
			result := s.RunCldOnTerminal(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "join")
			if took := time.Since(start); took > 20*time.Second {
				t.Errorf("join took %v, waiting on the process claude left behind", took)
			}
			stdout := ""
			if test.code == 0 {
				stdout = "\x1b]0;✳ cld-0\x07"
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

// join runs claude where it starts it: where it creates the session, or brings it back, and not
// where the session runs, which it attaches to; detach, kill, list and setup telemetry never do,
// nor does restore without a session to bring back - it checks the claude a session started with,
// in the session's directory (see TestRestoreFailures) - and neither does completion - __complete
// and __completeNoDesc - which makes no check at all (see TestCompletionSkipsChecks), for join's
// and setup telemetry's arguments too and with a tmux the check refuses: with a claude too old for
// join, which records that it ran, the others do as they do with any other - setup telemetry,
// which checks no tmux, looks for docker, not on the PATH here, and on a system other than Linux
// refuses to run. join runs claude --version last among the checks it makes before tmux - what is
// not installed, and tmux's version, which detach, kill and list check too, come first - after
// the session lookup, which decides whether it starts claude: with session main found (sessions
// "cld-main"), join attaches, and refuses there a terminal it does not have, or the --resume that
// would be lost. The fake tmux lists the sessions that sessions names, none if it is empty, and
// completion finds them through a socket cld-main. That it comes after the check for cld's own
// pane, which needs a terminal, TestJoinInItsOwnPaneWithNoTerminal pins.
func TestOnlyJoinRunsClaude(t *testing.T) {
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
		{[]string{"join"}, "tmux 3.7c", "", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"join", "-s", "main"}, "tmux 3.7c", "", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"join", "-s", "main"}, "tmux 3.7c", "cld-main", 1, "", "cld: join needs a terminal, and its input is not one\n", false},
		{[]string{"join", "-s", "main", "--resume", "SESSION"}, "tmux 3.7c", "cld-main", 1, "",
			"cld: session 'main' exists, and --resume would be lost: its claude has started; attach to it with cld join -s main, or give another -s SUFFIX\n", false},
		{[]string{"join", "-w"}, "tmux 3.7c", "", 1, "", "cld: git is not installed\n", false},
		{[]string{"join"}, "tmux 3.4", "", 1, "", "cld: tmux 3.5a or newer is required, found 'tmux 3.4'\n", false},
		{[]string{"join", "--resume", "SESSION"}, "tmux 3.7c", "", 1, "", "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n", true},
		{[]string{"join", "-s", "x"}, "tmux 3.4", "", 1, "", "cld: tmux 3.5a or newer is required, found 'tmux 3.4'\n", false},
		{[]string{"kill", "-s", "main"}, "tmux 3.7c", "", 1, "", "cld: no session 'main' (see cld list)\n", false},
		{[]string{"detach", "-s", "main"}, "tmux 3.7c", "", 1, "", "cld: no session 'main' (see cld list)\n", false},
		{[]string{"list"}, "tmux 3.7c", "", 0, "", "", false},
		{[]string{"restore"}, "tmux 3.7c", "", 0, "", "", false},
		{[]string{"restore"}, "tmux 3.4", "", 1, "", "cld: tmux 3.5a or newer is required, found 'tmux 3.4'\n", false},
		{[]string{"__complete", "join", "-s", ""}, "tmux 3.4", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"__complete", "join", "--resume", "x", "-n", ""}, "tmux 3.4", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"__completeNoDesc", "join", "-s", ""}, "tmux 3.4", fakeSession("main", 0), 0, "main\n:4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"setup", "telemetry", "--remote", "https://otel.example.com:4317"}, "tmux 3.4", "", 1, "", noDocker, false},
		{[]string{"__complete", "setup", "telemetry", "--local", ""}, "tmux 3.4", "", 0, ":4\n",
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n", false},
		{[]string{"setup", "project"}, "tmux 3.4", "", 0,
			"Created .claude/settings.json\nCreated .claude/settings.local.json\nCreated .gitignore\n", "", false},
		{[]string{"__complete", "setup", "project", "--mcp", "r"}, "tmux 3.4", "", 0,
			"rider\tRider's MCP server, port $RIDER_MCP_PORT or 64482\n:4\n",
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
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
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

// join checks the claude that tmux starts: claude --version runs in the directory claude starts
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
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
			if err := os.WriteFile(filepath.Join(s.Work, ".claude-version"), []byte(test.pinned+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result := s.RunCldOnTerminal(map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "join")
			if test.accepted {
				if title := "\x1b]0;\u2733 cld-0\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
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
// -s offers what that tmux lists. With no tmux, one that fails or cannot run, or a socket
// directory it cannot read, it offers no names, and no file names, and exits 0; what went wrong
// goes to stderr, which the completion scripts discard. The tmux is asked through a socket cld-x,
// as list asks it. That it never runs claude, whose version join checks, TestOnlyJoinRunsClaude
// pins.
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
				env["CLD_FAKE_TMUX_SESSIONS"] = fakeSession("x", 0)
			default:
				s.WriteProgram(filepath.Join(env["PATH"], "tmux"), test.script, test.mode)
			}
			if test.script == "unreadable" {
				s.WriteFile(s.SocketDir(), "")
			} else {
				socket(t, s, "cld-x")
			}
			result := s.RunCld(env, "__complete", "join", "-s", "")
			if result.Code != 0 || result.Stdout != test.stdout || !strings.Contains(result.Stderr, test.stderr) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr with %q", result.Code, result.Stdout, result.Stderr, test.stdout, test.stderr)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld ran tmux for more than its list of sessions")
			}
		})
	}
}

// endText says how claude exited and how to end its session, with cld kill and options, which
// name the session, and then how to detach, with C-q d or cld detach and options, as much of it
// as fits the pane's width whole: without C-q d and cld detach where all of it does not fit, and
// without the kill where that does not fit either. The widths count how claude exited as its
// widest, signal vtalrm, and the border line's spaces and 4 cells of border.
func endText(options string) string {
	exited := "claude exited with #{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}"
	width := func(text string) string {
		return strconv.Itoa(len("claude exited with signal vtalrm"+text) + 6)
	}
	kill := ": cld kill " + options + " ends the session"
	detach := " C-q d or cld detach " + options + " detaches"
	return "#{?#{e|<:#{pane_width}," + width(kill+","+detach) + "}," +
		"#{?#{e|<:#{pane_width}," + width(kill) + "}," + exited + "," + exited + kill + "}," +
		exited + kill + "#," + detach + "}"
}

// endHint is how the pane-died hook that join sets, and join attaching, show endText on the
// message line.
func endHint(options string) string {
	return "display-message -d 0 '" + endText(options) + "'"
}

// marksCommand is the run-shell that join has tmux run once it has made session name, in its
// command: it removes the session's busy mark and its start mark and makes its run mark, beside
// the entry in cld's record in s, printing nothing and exiting 0 however they fare, with each "#"
// doubled for run-shell's format.
func marksCommand(s *sandbox.Sandbox, name string) []string {
	quoted := func(path string) string { return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'" }
	base := strings.TrimSuffix(entryFile(s, name), ".json")
	sh := "{ rm -f " + quoted(base+".busy") + " " + quoted(base+".start") + "; touch " + quoted(base+".run") + "; } 2>/dev/null || true"
	return []string{"run-shell", strings.ReplaceAll(sh, "#", "##")}
}

// unmarkCommand is the command with which kill and the sweep of the idle sessions remove session
// name's run mark from cld's record in s, as run-shell takes it.
func unmarkCommand(s *sandbox.Sandbox, name string) string {
	run := strings.TrimSuffix(entryFile(s, name), ".json") + ".run"
	return strings.ReplaceAll("rm -f '"+strings.ReplaceAll(run, "'", `'\''`)+"' 2>/dev/null || true", "#", "##")
}

// endHook is the pane-died hook that join sets, on a pane that tmux keeps however claude
// exits: for status 0 it removes the session's run mark, the file run, where its path goes quoted
// for tmux, with each "#" doubled for run-shell's format, and quoted for sh within, and closes the
// pane, as tmux would; otherwise it keeps endText on a line of the pane's border, below the dead
// pane, and shows endHint to a terminal on claude's window.
func endHook(options, run string) string {
	sh := "rm -f '" + strings.ReplaceAll(run, "'", `'\''`) + "'"
	tmux := "'" + strings.ReplaceAll(strings.ReplaceAll(sh, "#", "##"), "'", `'\''`) + "'"
	return "if -F '#{==:#{pane_dead_status},0}' { run-shell " + tmux + " ; kill-pane } { " +
		"set -w pane-border-status bottom ; set -p pane-border-format ' " + endText(options) + " ' ; " +
		`if -F '#{window_active_clients}' "` + endHint(options) + `" }`
}

// switchKeys are the keys join binds on the server of session name, which tmux, at the path tmux,
// runs, for cld to move the terminal to another session (see TestSwitchKeys): C-q s shows cld list
// in a popup, by tmux on the server's socket, and C-q (, C-q ) and C-q L run cld list --to, each a
// run-shell in the background that prints nothing, naming the terminal that pressed the key, and
// cld by the file it runs from. The paths go quoted for sh, with each "#" doubled for run-shell's
// format.
func switchKeys(t *testing.T, s *sandbox.Sandbox, tmux, name string) []string {
	t.Helper()
	cld, err := filepath.EvalSymlinks(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	quoted := func(path string) string {
		return strings.ReplaceAll("'"+strings.ReplaceAll(path, "'", `'\''`)+"'", "#", "##")
	}
	list := quoted(cld) + " list --switch #{q:client_name}"
	popup := quoted(tmux) + " -S " + quoted(filepath.Join(s.SocketDir(), "cld-"+name)) +
		" display-popup -c #{q:client_name} -B -w 100% -h 100% -E " + list
	const quiet = " >/dev/null 2>&1 || true"
	return []string{
		"bind", "s", "run-shell", "-b", popup + quiet, ";",
		"bind", "(", "run-shell", "-b", list + " --to previous" + quiet, ";",
		"bind", ")", "run-shell", "-b", list + " --to next" + quiet, ";",
		"bind", "L", "run-shell", "-b", list + " --to last" + quiet, ";",
	}
}

// busyMarker is the marker join has tmux put before the session's name in the tab's
// title while claude is busy: ◐ in even seconds and ◑ in odd ones, with a job that, a second
// later, has tmux set the title again (see TestContractTitle).
const busyMarker = "#{?#{m:*[02468],%S},◐,◑}" +
	"#((sleep 1; #{q:@cld-tmux} -S #{q:socket_path} refresh-client -S -t #{q:client_name}) >/dev/null 2>&1 &)"

// join hands over to tmux with this command, word for word, where it creates a session: a client
// that takes the terminal for UTF-8 whatever the locale (see TestClientsTakeUTF8), the session's
// own server, its options, the directory, claude - by the path of the one it checked - and its
// arguments as separate words, what goes on claude's pane, and the tab's title on claude's session,
// naming the tmux cld checked, as claude's hooks do, and the keys that move the terminal to another
// session. With --switched-from, the session records the one the terminal came from. With
// --resume, claude gets --resume SESSION after its other arguments, never with -w's, and with
// --fork --fork-session; the words after "--" come last. A word ending in ";", which tmux would
// take for the end of its command, goes with a "\" before the ";", which tmux drops: SESSION, a
// word after "--", or the directory cld runs in. The directory goes with every "#" doubled, since
// tmux expands -c as a format, in which "##" is a "#". The session's home - the repository, the
// work directory, or a directory that is a repository of its own - goes with a "\" before a ";" at
// its end too, but with no "#" doubled: set does not expand it. The fake tmux, which finds no
// server running for the session, records the command, and the environment it gets: cld's own,
// without the variables that name the terminal to claude (see TestVSCodeGit for VS Code's) and
// with an empty TMUX where TMUX was set, which join's client needs (see TestNestsOnADeadPanesPty);
// a PS1, which the script's bash dropped, passes too (decision 11 in docs/design.md).
func TestCreateTmuxCommand(t *testing.T) {
	t.Parallel()
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	for _, command := range []struct {
		args []string
		// dir is where in the work tree cld runs, and c what tmux gets with -c there
		dir, c string
		// home is what tmux gets for the session's home, in the work directory where dir is a
		// repository of its own - whose name leaves nothing, as the work directory's does - and
		// the work directory itself where it is empty
		home string
		// after is what claude gets after its settings, as tmux gets it; -w's settings also
		// branch the worktree from HEAD
		after []string
	}{
		{[]string{"join", "-s", "x"}, "", "", "", nil},
		{[]string{"join", "-s", "x", "-w"}, "", "", "", []string{"--worktree", "cld-x"}},
		{[]string{"join", "-s", "x", "--new"}, "", "", "", nil},
		{[]string{"join", "-s", "x", "--resume", "cld-x"}, "", "", "", []string{"--resume", "cld-x"}},
		{[]string{"join", "-s", "x", "--resume", "a b"}, "", "", "", []string{"--resume", "a b"}},
		{[]string{"join", "-s", "x", "--resume", "a;"}, "", "", "", []string{"--resume", `a\;`}},
		{[]string{"join", "-s", "x", "--resume", `a\;`}, "", "", "", []string{"--resume", `a\\;`}},
		{[]string{"join", "-s", "x", "--", "--model", "a;", `a\;`, ""}, "", "", "", []string{"--model", `a\;`, `a\\;`, ""}},
		{[]string{"join", "-s", "x", "-w", "--", "go"}, "", "", "", []string{"--worktree", "cld-x", "go"}},
		{[]string{"join", "-s", "x", "--resume", "a", "--", "b;"}, "", "", "", []string{"--resume", "a", `b\;`}},
		{[]string{"join", "-s", "x", "--fork", "--resume", "cld-a-0"}, "", "", "", []string{"--resume", "cld-a-0", "--fork-session"}},
		{[]string{"join", "-s", "x", "--resume", "a;", "--fork"}, "", "", "", []string{"--resume", `a\;`, "--fork-session"}},
		{[]string{"join", "-s", "x", "--fork", "--resume=a", "--", "b;"}, "", "", "", []string{"--resume", "a", "--fork-session", `b\;`}},
		{[]string{"join", "--switched-from", "a-0", "-s", "x"}, "", "", "", nil},
		{[]string{"join", "-s", "x"}, "w;", `w\;`, "", nil},
		{[]string{"join", "-s", "x", "-w"}, "w;", `w\;`, "", []string{"--worktree", "cld-x"}},
		{[]string{"join", "-s", "x", "--resume", "cld-x"}, "w;", `w\;`, "", []string{"--resume", "cld-x"}},
		{[]string{"join", "-s", "x"}, `w\;`, `w\\;`, "", nil},
		{[]string{"join", "-s", "x"}, "C#S", "C##S", "", nil},
		{[]string{"join", "-s", "x", "-w"}, "x#(touch ran)", "x##(touch ran)", "", []string{"--worktree", "cld-x"}},
		{[]string{"join", "-s", "x", "--resume", "cld-x"}, "#{session_name}#;", `##{session_name}##\;`, "", []string{"--resume", "cld-x"}},
		{[]string{"join", "-s", "x"}, "#;", `##\;`, `#\;`, nil},
	} {
		args, dir, c, home, after := command.args, command.dir, command.c, command.home, command.after
		name := strings.Join(args, " ")
		if dir != "" {
			name += " in " + dir
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			if dir != "" {
				if err := os.Mkdir(filepath.Join(s.Work, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if home != "" {
				runGit(t, s, s.Work, "init", "-q", dir)
			}
			given := map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
				"TERMINAL_EMULATOR":     "JetBrains-JediTerm",
				"__CFBundleIdentifier":  "com.jetbrains.goland",
				"CURSOR_TRACE_ID":       "0123456789abcdef",
				"VisualStudioVersion":   "17.0",
				"TMUX":                  filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
				"PS1":                   `\u@\h$ `,
			}
			result := s.RunCldOnTerminalIn(filepath.Join(s.Work, dir), given, args...)
			if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
			}
			want := []string{"-u", "-L", "cld-x", "-f", "/dev/null",
				"set", "-s", "@cld", "1", ";",
				"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys:hyperlinks", ";",
				"set", "-s", "terminal-features[101]", "wezterm:hyperlinks", ";",
				"set", "-s", "terminal-features[102]", "alacritty:hyperlinks", ";",
				"set", "-s", "focus-events", "on", ";",
				"set", "-g", "mouse", "on", ";",
				"unbind", "-n", "C-MouseDown1Pane", ";", "unbind", "-n", "M-MouseDown3Pane", ";",
				"set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
				"set", "-g", "history-limit", "50000", ";",
				"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";"}
			want = append(want, switchKeys(t, s, sandbox.FakeTmux, "x")...)
			want = append(want, "new-session", "-s", "cld-x", "-n", "x", "-c", filepath.Join(s.Work, c))
			// cld finds the fake tmux, which the hooks then name.
			claude := settings(s, sandbox.FakeTmux, sandbox.RealGit, "cld-x", filepath.Join(s.Work, dir), slices.Contains(args, "-w"))
			want = append(append(want, probe, "--name", "cld-x", "--settings", claude), after...)
			want = append(want, ";",
				"set", "-p", "-t", "=cld-x:", "remain-on-exit", "on", ";",
				"set", "-p", "-t", "=cld-x:", "remain-on-exit-format", "", ";",
				"set-hook", "-p", "-t", "=cld-x:", "pane-died", endHook("-s x", strings.TrimSuffix(entryFile(s, "x"), ".json")+".run"), ";",
				"set", "-t", "=cld-x:", "@cld-tmux", sandbox.FakeTmux, ";",
				"set", "-t", "=cld-x:", "@cld-home", filepath.Join(s.Work, home), ";",
				"set", "-t", "=cld-x:", "@cld-busy", busyMarker, ";",
				"set", "-t", "=cld-x:", "set-titles-string", "#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-x#{?@cld-worktree, [w],}", ";",
				"set", "-t", "=cld-x:", "set-titles", "on", ";")
			if i := slices.Index(args, "--switched-from"); i >= 0 {
				want = append(want, "set", "-t", "=cld-x:", "@cld-last", args[i+1], ";")
			}
			want = append(want, marksCommand(s, "x")...)
			record := s.FakeTmuxRecord()
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given, "TERMINAL_EMULATOR", "__CFBundleIdentifier", "CURSOR_TRACE_ID", "VisualStudioVersion"))
			if want := filepath.Join(s.Work, dir); record.Cwd != want {
				t.Errorf("tmux runs in %s, want %s", record.Cwd, want)
			}
		})
	}
}

// A terminal of VS Code, or of one of its forks, gives git helpers that ask in its window: an
// askpass, GIT_ASKPASS naming a script beside VSCODE_GIT_ASKPASS_MAIN, which names Cursor,
// Windsurf or Antigravity to claude, and with git.terminalGitEditor an editor, GIT_EDITOR naming
// a script, quoted, beside VSCODE_GIT_EDITOR_MAIN; both ask through VSCODE_GIT_IPC_HANDLE. join
// leaves out each helper whole, with the other variables of its script and
// VSCODE_GIT_IPC_HANDLE, and keep a GIT_ASKPASS or GIT_EDITOR elsewhere, the user's own. The fake
// tmux records the environment it gets.
func TestVSCodeGit(t *testing.T) {
	t.Parallel()
	const (
		dist          = "/home/u/.cursor-server/bin/1/extensions/git/dist/"
		node          = "/home/u/.cursor-server/bin/1/node"
		handle        = "/run/user/1000/vscode-git-1.sock"
		yourAskpass   = "/usr/lib/ssh/x11-ssh-askpass"
		yourEditor    = "/usr/bin/vim"
		vscodeEditor  = `"` + dist + `git-editor.sh"`
		vscodeAskpass = dist + "askpass.sh"
	)
	askpass := map[string]string{
		"VSCODE_GIT_ASKPASS_MAIN":       dist + "askpass-main.js",
		"VSCODE_GIT_ASKPASS_NODE":       node,
		"VSCODE_GIT_ASKPASS_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":         handle,
	}
	editor := map[string]string{
		"VSCODE_GIT_EDITOR_MAIN":       dist + "git-editor-main.js",
		"VSCODE_GIT_EDITOR_NODE":       node,
		"VSCODE_GIT_EDITOR_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":        handle,
	}
	for _, test := range []struct {
		name string
		// vscode are VS Code's variables that come with GIT_ASKPASS and GIT_EDITOR, and kept
		// those of the two that reach tmux
		vscode          []map[string]string
		askpass, editor string
		kept            []string
	}{
		{"VS Code's", []map[string]string{askpass, editor}, vscodeAskpass, vscodeEditor, nil},
		{"VS Code's without its window", []map[string]string{askpass, editor},
			dist + "askpass-empty.sh", `"` + dist + `git-editor-empty.sh"`, nil},
		{"VS Code's askpass and your own editor", []map[string]string{askpass},
			vscodeAskpass, yourEditor, []string{"GIT_EDITOR"}},
		{"your own beside VS Code's", []map[string]string{askpass, editor},
			yourAskpass, yourEditor, []string{"GIT_ASKPASS", "GIT_EDITOR"}},
		{"your own", nil, yourAskpass, yourEditor, []string{"GIT_ASKPASS", "GIT_EDITOR"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			given := map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
				"GIT_ASKPASS":           test.askpass,
				"GIT_EDITOR":            test.editor,
			}
			var dropped []string
			for _, vscode := range test.vscode {
				maps.Copy(given, vscode)
				dropped = slices.AppendSeq(dropped, maps.Keys(vscode))
			}
			for _, name := range []string{"GIT_ASKPASS", "GIT_EDITOR"} {
				if !slices.Contains(test.kept, name) {
					dropped = append(dropped, name)
				}
			}
			result := s.RunCldOnTerminal(given, "join", "-s", "x")
			if title := "\x1b]0;✳ cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
			}
			checkEnv(t, s.FakeTmuxRecord().Env, passedOn(s, given, dropped...))
		})
	}
}

// join hands over to tmux with this command, word for word, where the session runs - a client that
// takes the terminal for UTF-8, attach-session with -d only for --detach-others, and with
// --switched-from the session the terminal came from recorded on the session - and with the
// environment it got but for an empty TMUX where TMUX was set: TERMINAL_EMULATOR too, which join
// leaves out only where it creates the session, and a PS1, which the script's bash dropped. The
// fake tmux finds session x on its server, then records the command.
func TestJoinTmuxCommand(t *testing.T) {
	t.Parallel()
	for _, command := range []struct {
		args []string
		// attach is attach-session and its options, as tmux gets them, and after what follows
		// attach-session before the hint
		attach, after []string
	}{
		{[]string{"join", "-s", "x"}, []string{"attach-session"}, nil},
		{[]string{"join", "-s", "x", "--detach-others"}, []string{"attach-session", "-d"}, nil},
		{[]string{"join", "--detach-others", "-s", "x"}, []string{"attach-session", "-d"}, nil},
		{[]string{"join", "-s", "x", "--detach-others=false"}, []string{"attach-session"}, nil},
		{[]string{"join", "--switched-from", "a-0", "-s", "x"}, []string{"attach-session"}, []string{";", "set", "-t", "=cld-x:", "@cld-last", "a-0"}},
		{[]string{"join", "--switched-from", "x", "-s", "x"}, []string{"attach-session"}, nil},
	} {
		args, attach, after := command.args, command.attach, command.after
		t.Run(strings.Join(args, " "), func(t *testing.T) {
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
			result := s.RunCldOnTerminal(given, args...)
			if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
			}
			record := s.FakeTmuxRecord()
			want := slices.Concat([]string{"-u", "-L", "cld-x"}, attach, []string{"-t", "=cld-x"}, after, []string{";", "if", "-F", "#{pane_dead}", endHint("-s x")})
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given))
		})
	}
}

// detach hands tmux this command, word for word, and prints nothing. With -s, once the fake tmux
// has found the session on its server: detach-client -s for the session, in an if that runs it
// only while a terminal is attached to the session. Without -n and -s, where TMUX names one of
// cld's servers - as in claude's pane, or a program claude runs there - a bare detach-client, by
// the socket TMUX names, under the same if, which also asks for the server's mark, printed first
// in the same command; no session lookup precedes it. tmux gets the environment cld got, TMUX and
// TMUX_PANE as they were, since it finds the pane through them. -n or -s there name the session
// as elsewhere; -n alone, or a TMUX that names another server - the default one, the one cld
// 0.3.0 and earlier shared, or one named like no session of cld's - is refused as outside cld's
// servers, and tmux gets nothing.
func TestDetachTmuxCommand(t *testing.T) {
	t.Parallel()
	// mark is 1 on a server that cld started (see TestLeavesAForeignServerAlone).
	const mark = "#{||:#{@cld},#{==:#{prefix},C-q}}"
	for _, test := range []struct {
		args []string
		// server is the server of the socket TMUX names; TMUX is unset where it is empty
		server string
		// session is the session the fake tmux finds, on whatever server it is asked
		session string
		// argv is what tmux gets, SOCKET standing for the socket TMUX names; nil where detach
		// refuses the command line
		argv []string
	}{
		{[]string{"detach", "-s", "x"}, "", "cld-x",
			[]string{"-L", "cld-x", "if", "-F", "-t", "=cld-x:", "#{session_attached}", "detach-client -s =cld-x"}},
		{[]string{"detach", "-n", "a", "-s", "b"}, "", "cld-a-b",
			[]string{"-L", "cld-a-b", "if", "-F", "-t", "=cld-a-b:", "#{session_attached}", "detach-client -s =cld-a-b"}},
		{[]string{"detach"}, "cld-x", "",
			[]string{"-S", "SOCKET", "display-message", "-p", mark, ";", "if", "-F", "#{&&:" + mark + ",#{session_attached}}", "detach-client"}},
		{[]string{"detach", "-s", "y"}, "cld-x", "cld-y",
			[]string{"-L", "cld-y", "if", "-F", "-t", "=cld-y:", "#{session_attached}", "detach-client -s =cld-y"}},
		{[]string{"detach", "-n", "x"}, "cld-x", "", nil},
		{[]string{"detach"}, "default", "", nil},
		{[]string{"detach"}, "cld", "", nil},
		{[]string{"detach"}, "cld-x.y", "", nil},
	} {
		name := strings.Join(test.args, " ")
		if test.server != "" {
			name += " in " + test.server
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			given := map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": test.session,
				"TERMINAL_EMULATOR":      "JetBrains-JediTerm",
			}
			tmux := filepath.Join(s.SocketDir(), test.server)
			if test.server != "" {
				given["TMUX"] = tmux + ",123,0"
				given["TMUX_PANE"] = "%0"
			}
			result := s.RunCld(given, test.args...)
			if test.argv == nil {
				if want := "cld: detach: missing -s SUFFIX (see cld list)\n"; result.Code != 2 || result.Stdout != "" || result.Stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, want)
				}
				if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
					t.Error("tmux ran")
				}
				return
			}
			if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
			}
			record := s.FakeTmuxRecord()
			want := slices.Clone(test.argv)
			if i := slices.Index(want, "SOCKET"); i >= 0 {
				want[i] = tmux
			}
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			env := passedOn(s, given)
			if test.server != "" {
				env["TMUX"] = given["TMUX"]
			}
			checkEnv(t, record.Env, env)
		})
	}
}

// join hands tmux the terminal of its stdin, and refuses without one - as from cron, ssh without
// -t or a script whose input is not the terminal - or with TERM unset, empty or dumb, with status
// 1, saying so, and nothing on stdout, whether it would create the session or attach to it. tmux
// failed there instead, with no word of cld's ("open terminal failed: not a terminal", "terminal
// does not support clear", tmux 3.7c), after cld had printed the title, and where it created the
// session only once it had started the server, whose socket stayed behind for list and every TAB
// to ask. join runs the real tmux here where it creates session x, and leaves no socket; where it
// attaches, it finds session x on the fake tmux's server, which would record the attach. What it
// checks before - a lingering server, a claude too old, an option that would be lost - comes
// first, as TestJoinStates, TestLingeringServer and TestOnlyJoinRunsClaude, which run cld
// without a terminal, pin.
func TestRefusesWithoutATerminal(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// attach is whether the fake tmux finds session x, which join then attaches to
		attach bool
		// terminal is whether cld runs on a terminal; term is TERM, unset where it is "unset"
		terminal bool
		term     string
		want     string
	}{
		{[]string{"join", "-s", "x"}, false, false, "xterm-256color", "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, false, false, "xterm-256color", "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"join", "-s", "x"}, true, false, "xterm-256color", "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"join", "-s", "x"}, false, true, "dumb", "cld: join needs a terminal, and TERM is dumb\n"},
		{[]string{"join", "-s", "x"}, false, true, "unset", "cld: join needs a terminal, and TERM is not set\n"},
		{[]string{"join", "-s", "x"}, false, true, "", "cld: join needs a terminal, and TERM is empty\n"},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, false, true, "dumb", "cld: join needs a terminal, and TERM is dumb\n"},
		{[]string{"join", "-s", "x"}, true, true, "dumb", "cld: join needs a terminal, and TERM is dumb\n"},
		{[]string{"join", "-s", "x"}, true, true, "unset", "cld: join needs a terminal, and TERM is not set\n"},
	} {
		name := strings.Join(test.args, " ")
		if test.attach {
			name += ", attaching"
		}
		name += ", no terminal"
		if test.terminal {
			name = strings.TrimSuffix(name, ", no terminal") + ", TERM=" + test.term
			if test.term == "unset" {
				name = strings.TrimSuffix(name, ", TERM=unset") + ", TERM unset"
			}
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			extra := map[string]string{"TERM": test.term}
			if test.term == "unset" {
				delete(s.Env, "TERM")
				delete(extra, "TERM")
			}
			if test.attach {
				extra["PATH"] = filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"]
				extra["CLD_FAKE_TMUX_VERSION"] = "tmux 3.7c"
				extra["CLD_FAKE_TMUX_SESSIONS"] = "cld-x"
			}
			run := s.RunCld
			if test.terminal {
				run = s.RunCldOnTerminal
			}
			result := run(extra, test.args...)
			if result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
			if servers := s.Servers(); len(servers) != 0 {
				t.Errorf("sockets %q left behind, want none", servers)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// join prints the title only where stdout is a terminal: tmux draws on the terminal of its stdin,
// and a stdout that is no terminal - cld join | tee or $(cld join), say - would only take the
// escape in as text. With stdin a terminal and stdout a pipe, neither gets the title, and cld hands
// over to the fake tmux, which finds session x where join attaches, and none where it creates it.
func TestTitleOnlyToATerminal(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args     []string
		sessions string
	}{
		{[]string{"join", "-s", "x"}, ""},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, ""},
		{[]string{"join", "-s", "x"}, "cld-x"},
	} {
		args, sessions := test.args, test.sessions
		t.Run(strings.Join(args, " ")+" "+sessions, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			pty := sandbox.OpenPty(t)
			cmd := exec.Command(sandbox.Cld, args...)
			cmd.Env = s.Environ(map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": sessions,
			})
			cmd.Dir = s.Work
			var stdout, stderr bytes.Buffer
			cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.Terminal, &stdout, &stderr
			_ = cmd.Run()
			if code := cmd.ProcessState.ExitCode(); code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output", code, stdout.String(), stderr.String())
			}
			if written := pty.Output(); written != "" {
				t.Errorf("the terminal got %q, want nothing", written)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err != nil {
				t.Error("cld did not hand over to tmux")
			}
		})
	}
}

// list shows how long ago each session was last active - now under a minute, and then in whole
// minutes, hours or days - and first ends each one idle for longer than CLD_IDLE_DAYS days, 30
// where it is unset or empty, none for 0, with a note that says how long in the largest whole
// unit. The fake tmux lists session a with its activity and its last attach, which a case gives
// apart where they differ, or leaves empty for no attach: the later counts. It records the kill:
// an if -F that has tmux kill the session and its server only where it is still idle - no
// terminal attached, and its activity and its last attach before the cutoff, the whole second at
// or after now less the limit - and otherwise print "kept". The fake prints nothing, as tmux does
// once the kill has ended the server.
func TestIdleSessionsWithFakeTmux(t *testing.T) {
	t.Parallel()
	const day = 24 * time.Hour
	// never is a last attach for a session no terminal has attached to.
	const never = -1
	for _, test := range []struct {
		idle time.Duration
		// attached, where it is not 0, is how long ago a terminal last attached, idle then being
		// the time since the activity alone
		attached time.Duration
		days     string
		// active is LAST ACTIVE, where the session stays
		active string
		// limit, where the session ends, is CLD_IDLE_DAYS as a time, and ended how long the note
		// says the session was idle
		limit time.Duration
		ended string
	}{
		{idle: 0, active: "now"},
		{idle: 55 * time.Second, active: "now"},
		{idle: 61 * time.Second, active: "1m"},
		{idle: 5*time.Minute + 30*time.Second, active: "5m"},
		{idle: 2*time.Hour + 30*time.Minute, active: "2h"},
		{idle: 3*day + 23*time.Hour, active: "3d"},
		{idle: 29*day + 23*time.Hour, active: "29d"},
		{idle: 5*time.Minute + 30*time.Second, attached: 40 * day, active: "5m"},
		{idle: 40 * day, attached: 2*time.Hour + 30*time.Minute, active: "2h"},
		{idle: 5*time.Minute + 30*time.Second, attached: never, active: "5m"},
		{idle: 40*day + time.Hour, days: "0", active: "40d"},
		{idle: 40*day + time.Hour, days: "45", active: "40d"},
		{idle: 40*day + time.Hour, limit: 30 * day, ended: "40 days"},
		{idle: 40 * day, attached: 31*day + time.Hour, limit: 30 * day, ended: "31 days"},
		{idle: 31*day + time.Hour, attached: 40 * day, limit: 30 * day, ended: "31 days"},
		{idle: 40*day + time.Hour, attached: never, limit: 30 * day, ended: "40 days"},
		{idle: 31*day + time.Hour, days: "30", limit: 30 * day, ended: "31 days"},
		{idle: 26 * time.Hour, days: "0.5", limit: 12 * time.Hour, ended: "1 day"},
		{idle: 90 * time.Minute, days: ".05", limit: 72 * time.Minute, ended: "1 hour"},
		{idle: 150 * time.Second, days: "0.001", limit: 86400 * time.Millisecond, ended: "2 minutes"},
	} {
		name := fmt.Sprintf("idle %v", test.idle)
		switch test.attached {
		case 0:
		case never:
			name += ", never attached"
		default:
			name += fmt.Sprintf(", attached %v ago", test.attached)
		}
		t.Run(fmt.Sprintf("%s, CLD_IDLE_DAYS %q", name, test.days), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			line := fakeSession("a", test.idle)
			switch test.attached {
			case 0:
			case never:
				line = fakeSessionAt("a", fakeTime(test.idle), "")
			default:
				line = fakeSessionAt("a", fakeTime(test.idle), fakeTime(test.attached))
			}
			started := time.Now()
			result := s.RunCld(map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": line,
				"CLD_IDLE_DAYS":          test.days,
			}, "list")
			if test.ended == "" {
				want := fmt.Sprintf("NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  %-11s  /w\n", test.active)
				if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
					t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s", result.Code, result.Stderr, result.Stdout, want)
				}
				if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
					t.Errorf("tmux ran %q", s.FakeTmuxRecord().Argv)
				}
				return
			}
			if want := "cld: ended session 'a', idle for " + test.ended + "\n"; result.Code != 0 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			argv := s.FakeTmuxRecord().Argv
			cutoff := regexp.MustCompile(`#\{e\|<:#\{session_activity\},([0-9]+)\}`).FindStringSubmatch(strings.Join(argv, " "))
			if cutoff == nil {
				t.Fatalf("tmux arguments %q compare no activity", argv)
			}
			idle := "#{&&:#{==:#{session_attached},0},#{&&:#{e|<:#{session_activity}," + cutoff[1] + "},#{e|<:#{session_last_attached}," + cutoff[1] + "}}}"
			kill := "run-shell '" + strings.ReplaceAll(unmarkCommand(s, "a"), "'", `'\''`) + "' ; if -F -t =cld-a: '" + idle + "' 'kill-session -t =cld-a ; kill-server' 'display-message -p kept'"
			if want := []string{"-L", "cld-a", "if", "-F", "-t", "=cld-a:", idle, kill, "display-message -p kept"}; !slices.Equal(argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", argv, want)
			}
			seconds, _ := strconv.ParseInt(cutoff[1], 10, 64)
			if earliest, latest := started.Add(-test.limit).Unix(), time.Now().Add(-test.limit).Unix()+1; seconds < earliest || seconds > latest {
				t.Errorf("cutoff %d, want from %d to %d: now less %v", seconds, earliest, latest, test.limit)
			}
		})
	}
}

// list shows claude's status after the session's state, from the one field tmux writes both in:
// the state, a space and busy, waiting or idle - or nothing, where no hook has set the status - and
// for a claude that has exited the state alone. STATE is as wide as its longest, and at least as
// wide as detached; completion describes the session by both. TestListShowsClaudesStatus has the
// real tmux write the field.
func TestClaudesStatusWithFakeTmux(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ field, state string }{
		{"detached ", "detached"},
		{"detached busy", "detached, busy"},
		{"detached waiting", "detached, waiting"},
		{"detached idle", "detached, idle"},
		{"attached waiting", "attached, waiting"},
		{"exited", "exited"},
	} {
		t.Run(test.field, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			fake := map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": fakeSessionIn("a", test.field, fakeTime(0), fakeTime(0)),
			}
			width := max(len(test.state), 8)
			want := fmt.Sprintf("NAME  %-*s  LAST ACTIVE  DIRECTORY\n"+"a     %-*s  now          /w\n", width, "STATE", width, test.state)
			if result := s.RunCld(fake, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s", result.Code, result.Stderr, result.Stdout, want)
			}
			want = "a\t" + test.state + "\n:4\n"
			if result := s.RunCld(fake, "__complete", "join", "-s", ""); result.Code != 0 || result.Stdout != want {
				t.Errorf("__complete join -s: exit %d, stdout %q, want %q", result.Code, result.Stdout, want)
			}
		})
	}
}

// list keeps the session whose server cld runs on - TMUX names its socket, as in any pane of that
// server, claude's Bash tool included - however long it has been idle: ending the server would end
// cld, and a claude that ran it. It goes by the socket's file, as TMUX gives the socket's path with
// the directory's symbolic links resolved, which list's own path to it need not have; a socket of
// the same name elsewhere is another server's, and the session ends.
func TestSweepKeepsTheSessionCldRunsIn(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// socket is TMUX's socket, relative to the sandbox's root, whose socket directory is
		// tmux-UID
		socket string
		kept   bool
	}{
		{name: "its socket", socket: "tmux-UID/cld-a", kept: true},
		{name: "its socket through a symbolic link", socket: "link/tmux-UID/cld-a", kept: true},
		{name: "a socket of that name elsewhere", socket: "other/tmux-UID/cld-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			if err := os.Symlink(s.Root, filepath.Join(s.Root, "link")); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(s.Root, "other", filepath.Base(s.SocketDir()))
			if err := os.MkdirAll(other, 0o700); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(filepath.Join(other, "cld-a"), "")
			own := filepath.Join(s.Root, strings.ReplaceAll(test.socket, "tmux-UID", filepath.Base(s.SocketDir())))
			result := s.RunCld(map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": fakeSession("a", 40*24*time.Hour+time.Hour),
				"TMUX":                   own + ",100,0",
			}, "list")
			if !test.kept {
				if want := "cld: ended session 'a', idle for 40 days\n"; result.Code != 0 || result.Stdout != "" || result.Stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stderr %q", result.Code, result.Stdout, result.Stderr, want)
				}
				if argv := s.FakeTmuxRecord().Argv; len(argv) < 3 || !slices.Equal(argv[:3], []string{"-L", "cld-a", "if"}) {
					t.Errorf("tmux arguments %q, want the kill of session a", argv)
				}
				return
			}
			want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  40d          /w\n"
			if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s", result.Code, result.Stderr, result.Stdout, want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Errorf("tmux ran %q", s.FakeTmuxRecord().Argv)
			}
		})
	}
}

// join without -s makes its session although a server does not answer the read of the sessions
// for its sweep of the idle ones, and says so: the sweep is not what was asked. list, whose read
// it is, fails with tmux's message. The fake tmux fails on server cld-x, whose socket takes the
// connection, as tmux does where it may not connect; join's index passes over x, as x is no index,
// and finds no socket for session 0.
func TestJoinWarnsWhereTheSweepCannotRead(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	socket(t, s, "cld-x")
	fake := map[string]string{
		"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
		"CLD_FAKE_TMUX_DENIED":  "cld-x",
	}
	denied := "error connecting to /fake/tmux/cld-x (Permission denied)"
	result := s.RunCldOnTerminal(fake, "join")
	title := "\x1b]0;\u2733 cld-0\x07"
	if want := "cld: warning: cannot end the idle sessions: " + denied + "\n"; result.Code != 0 || result.Stdout != title || result.Stderr != want {
		t.Errorf("join: exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr, title, want)
	}
	if argv := s.FakeTmuxRecord().Argv; len(argv) < 3 || !slices.Equal(argv[:3], []string{"-u", "-L", "cld-0"}) || !slices.Contains(argv, "new-session") {
		t.Errorf("tmux arguments %q, want new-session on server cld-0", argv)
	}
	result = s.RunCld(fake, "list")
	if want := "cld: " + denied + "\n"; result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("list: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
}

// A server can exit while cld asks it - its claude exits, a cld kill runs - and tmux then says that
// the server exited unexpectedly: list passes over it and lists the other sessions, detach and
// kill find no session there, and join none to attach to - it would make session b, but has no
// terminal here. The fake tmux answers every server but cld-b, which exits as it is asked.
func TestServerExitingWhileAsked(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	socket(t, s, "cld-a")
	socket(t, s, "cld-b")
	fake := map[string]string{
		"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
		"CLD_FAKE_TMUX_SESSIONS": fakeSession("a", 0),
		"CLD_FAKE_TMUX_EXITED":   "cld-b",
	}
	for _, test := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"list"}, 0, "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  now          /w\n", ""},
		{[]string{"join", "-s", "b"}, 1, "", "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "b"}, 1, "", "cld: no session 'b' (see cld list)\n"},
		{[]string{"detach", "-s", "b"}, 1, "", "cld: no session 'b' (see cld list)\n"},
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

// A tmux that fails where cld expects it to work - tmux -V, the kill-session and kill-server that
// kill runs, or the detach-client that detach runs - ends cld with tmux's exit status, after
// tmux's own message: cld adds none. A signal ends it with 128 and the signal's number, as a shell
// reports it.
func TestPassesTmuxFailuresThrough(t *testing.T) {
	t.Parallel()
	const (
		versionFails = `echo "tmux: broken" >&2; exit 3`
		versionDies  = `kill -TERM $$`
		killFails    = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld-x ;; *kill-server*) echo "tmux: cannot kill" >&2; exit 5 ;; esac`
		detachFails  = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld-x ;; *detach-client*) echo "tmux: cannot detach" >&2; exit 6 ;; esac`
	)
	for _, test := range []struct {
		failure, script string
		args            []string
		code            int
		stderr          string
	}{
		{"-V exits 3", versionFails, []string{"list"}, 3, "tmux: broken\n"},
		{"-V exits 3", versionFails, []string{"kill", "-s", "x"}, 3, "tmux: broken\n"},
		{"-V gets SIGTERM", versionDies, []string{"list"}, 128 + 15, ""},
		{"-V gets SIGTERM", versionDies, []string{"join", "-s", "x"}, 128 + 15, ""},
		{"the kill exits 5", killFails, []string{"kill", "-s", "x"}, 5, "tmux: cannot kill\n"},
		{"-V exits 3", versionFails, []string{"detach", "-s", "x"}, 3, "tmux: broken\n"},
		{"the detach exits 6", detachFails, []string{"detach", "-s", "x"}, 6, "tmux: cannot detach\n"},
	} {
		t.Run(strings.Join(test.args, " ")+", "+test.failure, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			fake := filepath.Join(s.Root, "fake")
			if err := os.Mkdir(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			s.WriteProgram(filepath.Join(fake, "tmux"), "#!/bin/sh\n"+test.script+"\n", 0o755)
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
// lookup: join cannot hand over to it, kill cannot end the session with it, and detach
// cannot detach its terminals. A lookup
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
			// -V and, if it is not -V, the lookup - where join's finds no server, it fails on
			// the way out, once the file has been moved.
			answers string
			// lookup is whether the command that cannot run is a session lookup.
			lookup bool
			stdout string
		}{
			{[]string{"list"}, "", false, ""},
			{[]string{"join", "-s", "x"}, "", false, ""},
			{[]string{"kill", "-s", "x"}, "", false, ""},
			{[]string{"detach", "-s", "x"}, "", false, ""},
			{[]string{"list"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"join", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"kill", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"detach", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
			{[]string{"join", "-s", "x", "--new"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo 'no server running on /fake' >&2; trap 'exit 1' EXIT`, false, title},
			{[]string{"join", "-s", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, title},
			{[]string{"kill", "-s", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, ""},
			{[]string{"detach", "-s", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, ""},
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
					s.WriteProgram(tmux, broken.content, broken.mode)
				} else {
					// The script answers, then moves the file that cannot run over itself.
					s.WriteProgram(tmux+".broken", broken.content, broken.mode)
					script := "#!/bin/sh\n" + test.answers + "\nmv -f '" + tmux + ".broken' '" + tmux + "'\n"
					s.WriteProgram(tmux, script, 0o755)
				}
				// No other tmux on the PATH, so that cld takes one without the execute permission.
				path := fake + string(os.PathListSeparator) + s.Tools("claude", "mv")
				result := s.RunCldOnTerminal(map[string]string{"PATH": path}, test.args...)
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
// one, is found all the same, as bash's search found it: join cannot run the claude to check its
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
		{[]string{"join", "-s", "x"}, []string{"claude"}, []string{"tmux"}, 126,
			"cld: cannot run DENIED/claude: permission denied\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"git"}, []string{"tmux", "claude"}, 1,
			"cld: --worktree needs a git repository, and WORK is not in one\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"git"}, []string{"tmux", "claude", "git"}, 0, ""},
		{[]string{"join", "-s", "x"}, []string{"tmux", "claude"}, []string{"tmux", "claude"}, 0, ""},
	} {
		t.Run(strings.Join(test.args, " ")+", "+strings.Join(test.denied, ",")+" denied, "+strings.Join(test.present, ",")+" present", func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
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
			result := s.RunCldOnTerminal(map[string]string{
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
// and from -h - and the version, and join, which then does not hand over to tmux, whether it would
// create the session or attach to it; the completion scripts and the answers to __complete, which
// cobra prints for cld; and setup telemetry's report, once it is done. stdout is open for reading
// only here, so that every write to it fails - for join, which needs a terminal and prints the
// title only to one, a terminal that is its stdin too; list and __complete find session x through
// a socket cld-x.
func TestFailedWriteEndsCld(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// sessions is what the fake tmux lists.
		sessions string
		// before is what cobra writes to stderr first.
		before string
	}{
		{[]string{"list"}, fakeSession("x", 0), ""},
		{[]string{"help"}, "", ""},
		{[]string{"help", "join"}, "", ""},
		{[]string{"join", "-h"}, "", ""},
		{[]string{"detach", "-h"}, "", ""},
		{[]string{"kill", "-h", "-x"}, "", ""},
		{[]string{"version"}, "", ""},
		{[]string{"join", "-s", "x"}, "", ""},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, "", ""},
		{[]string{"join", "-s", "x", "--detach-others"}, "cld-x", ""},
		{[]string{"completion", "bash"}, "", ""},
		{[]string{"completion", "zsh", "--help"}, "", ""},
		{[]string{"completion"}, "", ""},
		{[]string{"__complete", "join", "-s", ""}, fakeSession("x", 0), "Completion ended with directive: ShellCompDirectiveNoFileComp\n"},
		{[]string{"help", "setup"}, "", ""},
		{[]string{"setup", "-h", "telemetry"}, "", ""},
		{[]string{"setup", "telemetry", "--remote", "https://otel.example.com:4317"}, "", ""},
		{[]string{"help", "setup", "project"}, "", ""},
		{[]string{"setup", "project", "--mcp", "goland"}, "", ""},
		{[]string{"help", "setup", "completion"}, "", ""},
		{[]string{"setup", "completion", "fish"}, "", ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			if test.args[0] == "setup" && test.args[1] == "telemetry" {
				linuxOnly(t)
			}
			s := sandbox.New(t)
			socket(t, s, "cld-x")
			cmd := exec.Command(sandbox.Cld, test.args...)
			written := os.DevNull
			if test.args[0] == "join" {
				pty := sandbox.OpenPty(t)
				cmd.Stdin, written = pty.Terminal, pty.Path
			}
			stdout, err := os.OpenFile(written, os.O_RDONLY|syscall.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
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
// reading only, where it would fail with status 1 once the session was gone; so does detach,
// which prints nothing either, once it has detached the session's terminals.
func TestNothingToPrintWritesNothing(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// argv is what tmux gets
		argv []string
	}{
		{[]string{"kill", "-s", "a"}, nil},
		{[]string{"detach", "-s", "a"}, []string{"-L", "cld-a", "if", "-F", "-t", "=cld-a:", "#{session_attached}", "detach-client -s =cld-a"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			stdout, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			cmd := exec.Command(sandbox.Cld, test.args...)
			// The fake tmux finds session a on its server, then records the command.
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
			want := test.argv
			if want == nil {
				want = []string{"-L", "cld-a", "run-shell", unmarkCommand(s, "a"), ";", "kill-session", "-t", "=cld-a", ";", "kill-server"}
			}
			if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", argv, want)
			}
		})
	}
}

// join refuses a working directory that no longer exists where it would create the session, with or
// without -w or --resume: the script went on with the PWD it got, and tmux started claude in the
// home directory instead. claude --version fails there, the probe's as claude 2.1.282's, so cld
// refuses the directory before it runs claude --version there. On Linux only: what macOS's getcwd
// does in a removed directory has not been checked.
func TestJoinRefusesARemovedDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("getcwd in a removed directory is checked on Linux only")
	}
	for _, args := range [][]string{{"join", "-s", "x"}, {"join", "-s", "x", "-w"}, {"join", "-s", "x", "--resume", "SESSION"}, {"join"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
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

// join refuses a working directory it can no longer enter - its search permission taken away since
// the shell entered it, or that of the directory above it - whether or not PWD is set, which Go's
// Getwd looks at first: given it with -c, tmux 3.7c started claude in the home directory instead,
// and claude --version could not start there, which cld had put down to claude. sh enters the
// directory, takes the permissions away and runs cld there. root enters any directory, so run as
// root the test runs sh and cld as nobody (65534), who owns the directories. On Linux only, as
// TestJoinRefusesARemovedDirectory.
func TestJoinRefusesADirectoryItCannotEnter(t *testing.T) {
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
			cmd := exec.Command("/bin/sh", "-c", script, sandbox.Cld, dir, test.locked, "join", "-s", "x")
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

// fakeSession is the line the fake tmux prints for list-sessions as Sessions asks it: session
// cld-NAME, detached, its claude's pid 100, the directory /w, and idle for idle - its activity and
// its last attach that long before now.
func fakeSession(name string, idle time.Duration) string {
	return fakeSessionAt(name, fakeTime(idle), fakeTime(idle))
}

// fakeSessionAt is fakeSession's line with the session's activity and its last attach as tmux
// gives them: seconds since the epoch, the last attach empty where no terminal has attached.
func fakeSessionAt(name, activity, attached string) string {
	return fakeSessionIn(name, "detached ", activity, attached)
}

// fakeSessionIn is fakeSessionAt's line with state, the field that holds the session's state and
// claude's status as tmux writes it: "detached " where no hook has set the status, as for
// fakeSession, "attached busy", "exited" (see session.Tmux.Sessions). A session attached has a
// terminal attached.
func fakeSessionIn(name, state, activity, attached string) string {
	clients := "0"
	if strings.HasPrefix(state, "attached") {
		clients = "1"
	}
	return "cld-" + name + "\t" + state + "\t" + clients + "\t100\t" + activity + " " + attached + "\t0\t/w"
}

// fakeTime is the time ago before now in whole seconds since the epoch, as tmux gives a session's
// times, rounded up: cld then reads a time since then of ago less up to a second, plus the time
// it takes to read it. Rounded down, it could be a second more on top, and 59 s read as a minute.
// So a case stays more than a second above the unit it shows, and some seconds below the next.
func fakeTime(ago time.Duration) string {
	since := time.Now().Add(-ago)
	seconds := since.Unix()
	if since.Nanosecond() > 0 {
		seconds++
	}
	return strconv.FormatInt(seconds, 10)
}

// socket makes the sandbox's socket of server as a running server has it, for list to find: a
// socket that takes connections, until the test ends, since cld connects to a socket before it
// runs tmux there and passes over one that refuses. Its lookups go to a fake tmux, which answers
// whatever the socket is. A stale one is staleSocket's.
func socket(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(s.SocketDir(), server), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
}

// staleSocket makes the sandbox's socket of server as a server that has died leaves it: a socket
// that nothing listens on, where the real tmux says that no server is running. A plain file will
// not do: tmux says so on Linux, whose connect refuses the connection there, but macOS's reports
// that the file is no socket, and tmux fails with that.
func staleSocket(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(s.SocketDir(), server), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}
