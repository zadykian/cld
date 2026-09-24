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
	for _, flag := range []string{"-h", "--help"} {
		result := s.RunCld(nil, flag)
		if result.Code != 0 || !strings.HasPrefix(result.Stdout, "usage: cld [NAME]\n") {
			t.Errorf("cld %s: exit %d, stdout %q", flag, result.Code, result.Stdout)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, flag := range []string{"-V", "--version"} {
		if result := s.RunCld(nil, flag); result.Code != 0 || result.Stdout != "cld dev\n" {
			t.Errorf("cld %s: exit %d, stdout %q", flag, result.Code, result.Stdout)
		}
	}
}

func TestRejectsInvalidNames(t *testing.T) {
	t.Parallel()
	// tmux would rename "." and ":" to "_", a space would split claude's arguments.
	for _, name := range []string{"a b", "foo.bar", "a:b", "x/y", "-x", "_x", "café", "a\nb"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(nil, name)
			if result.Code != 2 || !strings.Contains(result.Stderr, "invalid session name") {
				t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			if result.Stdout != "" {
				t.Errorf("printed %q before failing", result.Stdout)
			}
		})
	}
}

func TestRejectsExtraArguments(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCld(nil, "a", "b")
	if result.Code != 2 || !strings.Contains(result.Stderr, "too many arguments") {
		t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
	}
}

func TestRequiresTmuxAndClaude(t *testing.T) {
	t.Parallel()
	for missing, present := range map[string][]string{
		"tmux":   {"bash", "env", "claude"},
		"claude": {"bash", "env", "tmux"},
	} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools(present...)})
			if want := "cld: " + missing + " is not installed\n"; result.Code != 1 || result.Stderr != want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
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
			})
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
