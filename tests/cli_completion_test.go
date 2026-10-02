package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The completion scripts and their help, and completion making none of the startup checks.

// printedScript is a shell completion prints a script for: how the script starts, and what its
// help says to set it up.
type printedScript struct{ shell, start, setup string }

var printedScripts = []printedScript{
	{"bash", "# bash completion V2 for cld ",
		"This script depends on the 'bash-completion' package."},
	{"zsh", "#compdef cld\n", "autoload -U compinit; compinit"},
	{"fish", "# fish completion for cld ",
		"cld completion fish > ~/.config/fish/completions/cld.fish"},
}

// completion SHELL prints cobra's script for SHELL, which asks cld __complete what to offer, or
// __completeNoDesc with --no-descriptions. Its help is cobra's; -h and --help are cld's, and win
// over a wrong value after them. Alone, completion shows its help. None of them needs tmux or
// claude (decisions 17.1, 17.4 and 17.5).
func TestCompletionScripts(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	for _, test := range printedScripts {
		checkCompletionScript(t, s, none, test)
		checkCompletionHelp(t, s, none, test)
	}
	want, err := os.ReadFile(goldenHelp("completion"))
	if err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(none, "completion")
	if result.Code != 0 || result.Stdout != string(want) || result.Stderr != "" {
		t.Errorf("cld completion: exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout as in %s",
			result.Code, result.Stderr, result.Stdout, goldenHelp("completion"))
	}
}

// checkCompletionScript reports a script of test's shell, with descriptions and without, that
// starts otherwise or asks for descriptions otherwise.
func checkCompletionScript(
	t *testing.T, s *sandbox.Sandbox, env map[string]string, test printedScript,
) {
	t.Helper()
	for _, args := range [][]string{
		{"completion", test.shell}, {"completion", test.shell, "--no-descriptions"},
	} {
		result := s.RunCld(env, args...)
		noDescriptions := len(args) == 3
		if result.Code != 0 || !strings.HasPrefix(result.Stdout, test.start) || result.Stderr != "" ||
			strings.Contains(result.Stdout, " __completeNoDesc ") != noDescriptions {
			t.Errorf("cld %q: exit %d, stderr %q, stdout\n%.300s",
				args, result.Code, result.Stderr, result.Stdout)
		}
	}
}

// checkCompletionHelp reports a help of the script of test's shell that is not cobra's.
func checkCompletionHelp(
	t *testing.T, s *sandbox.Sandbox, env map[string]string, test printedScript,
) {
	t.Helper()
	help := "Generate the autocompletion script for the " + test.shell + " shell.\n"
	for _, args := range [][]string{
		{"completion", test.shell, "--help"}, {"completion", test.shell, "-h", "-x"},
		{"completion", test.shell, "-h", "--help=x"}, {"completion", test.shell, "--help", "-h=no"},
		{"help", "completion", test.shell},
	} {
		result := s.RunCld(env, args...)
		if result.Code != 0 || !strings.HasPrefix(result.Stdout, help) ||
			!strings.Contains(result.Stdout, test.setup) || result.Stderr != "" {
			t.Errorf("cld %q: exit %d, stderr %q, stdout\n%s",
				args, result.Code, result.Stderr, result.Stdout)
		}
	}
}

// Completion makes none of the checks other commands make before tmux (decision 17.4): join -s
// offers what a tmux they refuse lists. With no tmux, a failing one or an unreadable socket
// directory, it offers nothing, exits 0 and says why on stderr, which the scripts discard.
// TestOnlyJoinRunsClaude pins that it never runs claude.
func TestCompletionSkipsChecks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		what string
		// script is the tmux on the PATH, with mode: none where empty, and the fake tmux for
		// "fake", or for "unreadable" with a file where the socket directory would be.
		script string
		mode   os.FileMode
		stdout string
		stderr string
	}{
		{"a tmux the check refuses", "fake", 0, "x\tdetached\n:4\n", ""},
		{"no tmux", "", 0, ":4\n", "tmux is not installed"},
		{"a tmux that fails", "#!/bin/sh\necho 'tmux: broken' >&2\nexit 3\n", 0o755, ":4\n",
			"tmux: broken"},
		{"a tmux that cannot run", "#!/bin/sh\necho 'tmux 3.7c'\n", 0o644, ":4\n",
			"permission denied"},
		{"a socket directory it cannot read", "unreadable", 0, ":4\n", "cannot read"},
	} {
		t.Run(test.what, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			env := completionTmux(t, s, test.script, test.mode)
			result := s.RunCld(env, "__complete", "join", "-s", "")
			if result.Code != 0 || result.Stdout != test.stdout ||
				!strings.Contains(result.Stderr, test.stderr) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr with %q",
					result.Code, result.Stdout, result.Stderr, test.stdout, test.stderr)
			}
			if fakeTmuxRan(s) {
				t.Error("cld ran tmux for more than its list of sessions")
			}
		})
	}
}

// completionTmux is the environment with the tmux of script and mode on the PATH, as
// TestCompletionSkipsChecks has them. Its socket is cld-x, as list asks it.
func completionTmux(
	t *testing.T, s *sandbox.Sandbox, script string, mode os.FileMode,
) map[string]string {
	t.Helper()
	env := map[string]string{"PATH": s.Tools()}
	switch script {
	case "":
	case "fake", "unreadable":
		env["PATH"] = s.Tools("tmux")
		env["CLD_FAKE_TMUX_VERSION"] = "tmux 3.2a"
		env["CLD_FAKE_TMUX_SESSIONS"] = fakeSession("x", 0)
	default:
		s.WriteProgram(filepath.Join(env["PATH"], "tmux"), script, mode)
	}
	if script == "unreadable" {
		s.WriteFile(s.SocketDir(), "")
	} else {
		socket(t, s, "cld-x")
	}
	return env
}
