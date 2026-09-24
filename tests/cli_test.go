package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Everything cld does before it hands over to tmux; no terminal needed.

func TestHelp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}, {"new", "--help"}, {"join", "-n", "x", "-h"}, {"kill", "--help"}, {"list", "-h"}} {
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
	result := s.RunCld(nil)
	if result.Code != 2 || !strings.HasPrefix(result.Stderr, "cld: missing command") || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// "cld NAME" created or attached to session NAME before cld had commands; it now fails, naming
// the commands that do either.
func TestRejectsUnknownCommands(t *testing.T) {
	t.Parallel()
	for command, want := range map[string]string{
		"review": "cld: unknown command 'review'; for session review: cld new -n review, cld join -n review\n",
		"-x":     "cld: unknown command '-x' (see cld help)\n",
		"a.b":    "cld: unknown command 'a.b' (see cld help)\n",
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, command); result.Code != 2 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, want)
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
		{[]string{"list", "-n", "a"}, "cld: list: unexpected argument '-n' (see cld help)\n"},
		{[]string{"help", "new"}, "cld: help: unexpected argument 'new' (see cld help)\n"},
		{[]string{"version", "-n", "a"}, "cld: version: unexpected argument '-n' (see cld help)\n"},
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

// new needs tmux and claude, the other commands only tmux: the fake tmux finds no session, so
// join and kill get as far as saying so.
func TestRequiresTmuxAndClaude(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		command string
		present []string
		want    string
	}{
		{"new", []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{"new", []string{"bash", "env", "tmux"}, "cld: claude is not installed\n"},
		{"join", []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{"join", []string{"bash", "env", "tmux"}, "cld: no session 'main'; create it with cld new -n main\n"},
		{"kill", []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
		{"kill", []string{"bash", "env", "tmux"}, "cld: no session 'main' (see cld list)\n"},
		{"list", []string{"bash", "env", "claude"}, "cld: tmux is not installed\n"},
	} {
		t.Run(test.command+" "+strings.Join(test.present, ","), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools(test.present...)}, test.command)
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
