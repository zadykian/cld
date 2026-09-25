package tests

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Everything cld does before it hands over to tmux; no terminal needed.

var update = flag.Bool("update", false, "rewrite the help in testdata/help from what cld help prints")

// helpTopics are what cld help takes, "" for none, in the order the help lists them.
var helpTopics = []string{"", "new", "join", "kill", "list", "help", "version"}

// goldenHelp is the file holding what cld help topic prints: testdata/help/cld.txt for cld help,
// testdata/help/COMMAND.txt for cld help COMMAND.
func goldenHelp(topic string) string {
	if topic == "" {
		topic = "cld"
	}
	return filepath.Join("testdata", "help", topic+".txt")
}

// The help of cld and of each command, byte for byte: cobra generates it from each command's texts
// and options, with its default templates, so a change to either shows here. -update rewrites
// the files. cobra wraps nothing, so the texts break their lines by hand, within 80 columns, and
// a usage line names the options before the arguments, as cld reads them.
func TestHelpText(t *testing.T) {
	t.Parallel()
	for _, topic := range helpTopics {
		args := []string{"help"}
		if topic != "" {
			args = append(args, topic)
		}
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
			// A command added to cld gets its own file here.
			if topic == "" {
				if listed := listedCommands(result.Stdout); !slices.Equal(listed, helpTopics[1:]) {
					t.Errorf("the help lists %q, want %q: one file in testdata/help for each", listed, helpTopics[1:])
				}
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
// arguments are read left to right.
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
		{[]string{"-h", "join"}, "join"},
		{[]string{"--help", "version"}, "version"},
		{[]string{"help", "help"}, "help"},
		{[]string{"new", "--help"}, "new"},
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
// before it is no command either. -v, completion and __complete are commands only elsewhere.
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
		{[]string{"completion", "bash"}, "cld: unknown command 'completion'; for session completion: cld new -n completion, cld join -n completion\n"},
		{[]string{"__complete", "new", "-n", ""}, "cld: unknown command '__complete' (see cld help)\n"},
		{[]string{"__completeNoDesc", "join", ""}, "cld: unknown command '__completeNoDesc' (see cld help)\n"},
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
		for _, args := range [][]string{{"new", "-n", name}, {"join", "--name", name}, {"kill", "-n", name}, {"new", "--name=" + name}} {
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

// Arguments are read left to right, and the first wrong one decides the message: an option after
// an argument is not read, and -- ends nothing. help and version are named as typed. help takes
// one argument, a command of cld's.
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

// new needs tmux and claude, and git for -w; the other commands only tmux: the fake tmux finds no
// session, so join and kill get as far as saying so.
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
// working directory has a tmux and a git of its own, which fail, saying so, if run.
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
		{[]string{"new", "-w"}, []string{"tmux", "claude"}, 1, "cld: git is not installed\n"},
		// tmux and git, run from the absolute entry, find no session and the repository.
		{[]string{"new", "-w"}, []string{"tmux", "claude", "git"}, 0, ""},
	} {
		for _, relative := range []string{".", ""} {
			t.Run(strings.Join(test.args, " ")+" "+strings.Join(test.present, ",")+" after '"+relative+"'", func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				gitInit(t, s.Work)
				for _, name := range []string{"tmux", "git"} {
					script := "#!/bin/sh\necho \"relative " + name + " ran\" >&2\nexit 99\n"
					if err := os.WriteFile(filepath.Join(s.Work, name), []byte(script), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				path := relative + string(os.PathListSeparator) + s.Tools(test.present...)
				if result := s.RunCld(map[string]string{"PATH": path}, test.args...); result.Code != test.code || result.Stderr != test.want {
					t.Errorf("PATH %s: exit %d, stderr %q, want exit %d, stderr %q", path, result.Code, result.Stderr, test.code, test.want)
				}
			})
		}
	}
}

func TestRequiresTmux33(t *testing.T) {
	t.Parallel()
	for version, accepted := range map[string]bool{
		"tmux 3.3a":     true,
		"tmux 3.4":      true,
		"tmux 3.10":     true,
		"tmux 4.0":      true,
		"tmux next-3.6": true,
		"tmux master":   true,
		"tmux 3.2a":     false,
		"tmux 2.9a":     false,
		"tmux next-3.2": false,
	} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{
				"PATH":                  s.Tools("tmux", "claude"),
				"CLD_FAKE_TMUX_VERSION": version,
			}, "new")
			_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
			started := err == nil
			if accepted && (result.Code != 0 || !started) {
				t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
			}
			if want := "cld: tmux 3.3 or newer is required, found '" + version + "'\n"; !accepted &&
				(result.Code != 1 || result.Stderr != want || started) {
				t.Errorf("accepted: exit %d, stderr %q, tmux started: %v", result.Code, result.Stderr, started)
			}
		})
	}
}

// endHint is how the pane-died hook that new sets, and join, show how to end a session whose
// claude failed.
const endHint = "display-message -d 0 'claude exited with " +
	"#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}: " +
	"C-q d detaches, cld kill -n #{window_name} ends the session'"

// new hands over to tmux with this command, word for word: the server options, claude and its
// arguments as separate words, the mark, and what goes on claude's window. The fake tmux records
// it, and the environment it gets: cld's own, without TERMINAL_EMULATOR and with an empty TMUX
// where TMUX was set (see TestNestsOnADeadPanesPty); a PS1, which the script's bash dropped,
// passes too (decision 11 in docs/design.md). Below tmux 3.5 remain-on-exit stays off.
func TestNewTmuxCommand(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		version, remain string
		worktree        bool
	}{
		{"tmux 3.4", "off", false},
		{"tmux 3.4", "off", true},
		{"tmux 3.7c", "failed", false},
		{"tmux 3.7c", "failed", true},
	} {
		args := []string{"new", "-n", "x"}
		claude := []string{"claude", "--name", "cld-x", "--settings", remoteControl}
		if test.worktree {
			args = append(args, "-w")
			claude = []string{"claude", "--name", "cld-x", "--settings",
				`{"remoteControlAtStartup":true,"worktree":{"baseRef":"head"}}`, "--worktree", "x"}
		}
		t.Run(test.version+" "+strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.worktree {
				gitInit(t, s.Work)
			}
			given := map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": test.version,
				"TERMINAL_EMULATOR":     "JetBrains-JediTerm",
				"TMUX":                  filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
				"PS1":                   `\u@\h$ `,
			}
			result := s.RunCld(given, args...)
			if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
			}
			want := []string{"-L", "cld", "-f", "/dev/null",
				"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys", ";",
				"set", "-s", "focus-events", "on", ";",
				"set", "-g", "mouse", "on", ";", "set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
				"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
				"new-session", "-s", "cld-x", "-n", "x", "-c", s.Work}
			want = append(want, claude...)
			want = append(want, ";",
				"set", "-F", "-t", "=cld-x:", "@cld", "#{session_id}", ";",
				"set", "-w", "-t", "=cld-x:", "remain-on-exit", test.remain, ";",
				"set", "-w", "-t", "=cld-x:", "remain-on-exit-format", "", ";",
				"set-hook", "-w", "-t", "=cld-x:", "pane-died", `if -F '#{window_active_clients}' "`+endHint+`"`)
			record := s.FakeTmuxRecord()
			if !slices.Equal(record.Argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
			}
			checkEnv(t, record.Env, passedOn(s, given, "TERMINAL_EMULATOR"))
			if record.Cwd != s.Work {
				t.Errorf("tmux runs in %s, want %s", record.Cwd, s.Work)
			}
		})
	}
}

// join hands over to tmux with this command, word for word, and with the environment it got but
// for an empty TMUX where TMUX was set: TERMINAL_EMULATOR too, which only new leaves out, and a
// PS1, which the script's bash dropped. The fake tmux finds session x among cld's, then records
// the command.
func TestJoinTmuxCommand(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	given := map[string]string{
		"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
		"CLD_FAKE_TMUX_SESSIONS": "cld",
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
	if want := []string{"-L", "cld", "attach-session", "-d", "-t", "=cld-x", ";", "if", "-F", "#{pane_dead}", endHint}; !slices.Equal(record.Argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
	}
	checkEnv(t, record.Env, passedOn(s, given))
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

// A tmux that fails where cld expects it to work - tmux -V, or kill-session - ends cld with tmux's
// exit status, after tmux's own message: cld adds none. A signal ends it with 128 and the
// signal's number, as a shell reports it.
func TestPassesTmuxFailuresThrough(t *testing.T) {
	t.Parallel()
	const (
		versionFails = `echo "tmux: broken" >&2; exit 3`
		versionDies  = `kill -TERM $$`
		killFails    = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld ;; *kill-session*) echo "tmux: cannot kill" >&2; exit 5 ;; esac`
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
		{"kill-session exits 5", killFails, []string{"kill", "-n", "x"}, 5, "tmux: cannot kill\n"},
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
// the script's lookups ended it with bash's.
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
			// -V and, if it is not -V, the lookup.
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
			{[]string{"new", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac`, false, title},
			{[]string{"join", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld`, false, title},
			{[]string{"kill", "-n", "x"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld`, false, ""},
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
// one, is found all the same, as bash's search found it: new goes on and hands tmux the claude it
// cannot run, as the script did, and git cannot say that the directory is in a repository. A
// tool without the execute permission before an executable one does not hide it.
func TestToolsWithoutExecutePermission(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// denied are the tools without the execute permission, in a PATH entry before present.
		denied, present []string
		code            int
		stderr          string
	}{
		{[]string{"new", "-n", "x"}, []string{"claude"}, []string{"tmux"}, 0, ""},
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
			stdout, stderr := "", strings.ReplaceAll(test.stderr, "WORK", s.Work)
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
// and from -h - and the version, and new and join, which then do not hand over to tmux. stdout is
// open for reading only here, so that every write to it fails.
func TestFailedWriteEndsCld(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// sessions is what the fake tmux lists.
		sessions string
	}{
		{[]string{"list"}, "cld-a\tdetached\t/w"},
		{[]string{"help"}, ""},
		{[]string{"help", "new"}, ""},
		{[]string{"new", "-h"}, ""},
		{[]string{"join", "-h"}, ""},
		{[]string{"kill", "-h", "-x"}, ""},
		{[]string{"version"}, ""},
		{[]string{"new", "-n", "x"}, ""},
		{[]string{"join", "-n", "x"}, "cld"},
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
			cmd.Env = s.Environ(map[string]string{
				"PATH":                   filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION":  "tmux 3.7c",
				"CLD_FAKE_TMUX_SESSIONS": test.sessions,
			})
			cmd.Dir = s.Work
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = stdout, &stderr
			_ = cmd.Run()
			if code, want := cmd.ProcessState.ExitCode(), "cld: write error: bad file descriptor\n"; code != 1 || stderr.String() != want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", code, stderr.String(), want)
			}
			if _, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json")); err == nil {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// new refuses a working directory that no longer exists, with or without -w: the script went on
// with the PWD it got, and tmux started claude in the home directory instead. On Linux only: what
// macOS's getcwd does in a removed directory has not been checked.
func TestNewRefusesARemovedDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("getcwd in a removed directory is checked on Linux only")
	}
	for _, args := range [][]string{{"new", "-n", "x"}, {"new", "-n", "x", "-w"}} {
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
