package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Everything cld does before it hands over to tmux; no terminal needed.

func TestHelp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// -h before a wrong argument shows the help too: arguments are read left to right.
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}, {"new", "--help"}, {"join", "-n", "x", "-h"}, {"kill", "--help"}, {"list", "-h"},
		{"new", "-h", "-x"}, {"join", "-h", "-w"}, {"new", "-h", "--help=x"}, {"list", "-h", "-h=no"}, {"join", "--help", "--help=maybe"}} {
		result := s.RunCld(nil, args...)
		if result.Code != 0 || !strings.HasPrefix(result.Stdout, "usage: cld COMMAND [OPTIONS]\n") {
			t.Errorf("cld %q: exit %d, stdout %q", args, result.Code, result.Stdout)
		}
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

// Arguments are read left to right, and the first wrong one decides the message: an option after
// an argument is not read, and -- ends nothing. help and version are named as typed.
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
		{[]string{"help", "new"}, "cld: help: unexpected argument 'new' (see cld help)\n"},
		{[]string{"version", "-n", "a"}, "cld: version: unexpected argument '-n' (see cld help)\n"},
		{[]string{"-V", "x"}, "cld: -V: unexpected argument 'x' (see cld help)\n"},
		{[]string{"--help", "x"}, "cld: --help: unexpected argument 'x' (see cld help)\n"},
		{[]string{"new", "--"}, "cld: new: unexpected argument '--' (see cld help)\n"},
		{[]string{"join", "--", "-x"}, "cld: join: unexpected argument '--' (see cld help)\n"},
		{[]string{"list", "--"}, "cld: list: unexpected argument '--' (see cld help)\n"},
		{[]string{"join", "a", "-x"}, "cld: join: unexpected argument 'a' (see cld help)\n"},
		{[]string{"new", "review", "-n"}, "cld: new: unexpected argument 'review' (see cld help)\n"},
		{[]string{"new", "review", "-h"}, "cld: new: unexpected argument 'review' (see cld help)\n"},
		{[]string{"join", "-w", "-h"}, "cld: join: unexpected argument '-w' (see cld help)\n"},
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
		{[]string{"new"}, []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{[]string{"new"}, []string{"bash", "env", "tmux"}, "cld: claude is not installed\n"},
		{[]string{"new", "-w"}, []string{"bash", "env", "tmux", "claude"}, "cld: git is not installed\n"},
		{[]string{"join"}, []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{[]string{"join"}, []string{"bash", "env", "tmux"}, "cld: no session 'main'; create it with cld new -n main\n"},
		{[]string{"kill"}, []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{[]string{"kill"}, []string{"bash", "env", "tmux"}, "cld: no session 'main' (see cld list)\n"},
		{[]string{"list"}, []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
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
				"PATH":                  s.Tools("bash", "env", "tmux", "claude"),
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
// it, and the environment it gets: without TERMINAL_EMULATOR, and with an empty TMUX where TMUX
// was set (see TestNestsOnADeadPanesPty). Below tmux 3.5 remain-on-exit stays off.
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
			result := s.RunCld(map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": test.version,
				"TERMINAL_EMULATOR":     "JetBrains-JediTerm",
				"TMUX":                  filepath.Join(s.Root, "elsewhere", "default") + ",1,0",
			}, args...)
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
			if value, found := record.Env["TERMINAL_EMULATOR"]; found {
				t.Errorf("tmux gets TERMINAL_EMULATOR=%s", value)
			}
			if value, found := record.Env["TMUX"]; !found || value != "" {
				t.Errorf("tmux gets TMUX %q (set: %v), want it set and empty", value, found)
			}
			if record.Cwd != s.Work {
				t.Errorf("tmux runs in %s, want %s", record.Cwd, s.Work)
			}
		})
	}
}

// join hands over to tmux with this command, word for word, and with the environment it got but
// for an empty TMUX where TMUX was set: TERMINAL_EMULATOR too, which only new leaves out. The fake
// tmux finds session x among cld's, then records the command.
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
	}
	result := s.RunCld(given, "join", "-n", "x")
	if title := "\x1b]0;\u2733 cld-x\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, title)
	}
	record := s.FakeTmuxRecord()
	if want := []string{"-L", "cld", "attach-session", "-d", "-t", "=cld-x", ";", "if", "-F", "#{pane_dead}", endHint}; !slices.Equal(record.Argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", record.Argv, want)
	}
	for _, name := range []string{"TERMINAL_EMULATOR", "TERM_PROGRAM", "LC_TERMINAL", "LANG", "TERM"} {
		want, isGiven := given[name]
		if !isGiven {
			want = s.Env[name]
		}
		if value, found := record.Env[name]; !found || value != want {
			t.Errorf("tmux gets %s %q (set: %v), want %q", name, value, found, want)
		}
	}
	if value, found := record.Env["TMUX"]; !found || value != "" {
		t.Errorf("tmux gets TMUX %q (set: %v), want it set and empty", value, found)
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
