package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A tmux that fails, or that the system cannot run.

// A tmux that fails where cld expects it to work ends cld with tmux's exit status, after tmux's
// own message: cld adds none. Such are tmux -V, kill's kill-session and kill-server, and detach's
// detach-client. A signal ends cld with 128 and the signal's number, as a shell reports it.
func TestPassesTmuxFailuresThrough(t *testing.T) {
	t.Parallel()
	const (
		versionFails = `echo "tmux: broken" >&2; exit 3`
		versionDies  = `kill -TERM $$`
		killFails    = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld-x ;; ` +
			`*kill-server*) echo "tmux: cannot kill" >&2; exit 5 ;; esac`
		detachFails = `case "$*" in -V) echo "tmux 3.7c" ;; *list-sessions*) echo cld-x ;; ` +
			`*detach-client*) echo "tmux: cannot detach" >&2; exit 6 ;; esac`
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
		{"the detach exits 6", detachFails, []string{"detach", "-s", "x"}, 6,
			"tmux: cannot detach\n"},
	} {
		t.Run(strings.Join(test.args, " ")+", "+test.failure, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			fake := filepath.Join(s.Root, "fake")
			if err := os.Mkdir(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			s.WriteProgram(filepath.Join(fake, "tmux"), "#!/bin/sh\n"+test.script+"\n", 0o755)
			path := fake + string(os.PathListSeparator) + s.Env["PATH"]
			result := s.RunCld(map[string]string{"PATH": path}, test.args...)
			checkFailed(t, result, test.code, test.stderr)
		})
	}
}

// unrunnableProgram is a program the system cannot run, as what describes it, and how cld ends:
// status code, and why the system refused it.
type unrunnableProgram struct {
	what, content string
	mode          os.FileMode
	code          int
	reason        string
}

// unrunnable are the programs printing output that the system cannot run. Each ends cld with 127
// where the system reports no such file, here the interpreter, and 126 otherwise, as a shell does
// (decision 11.9). A file without the execute permission counts where the PATH has no executable
// one (decision 11.5).
func unrunnable(output string) []unrunnableProgram {
	return []unrunnableProgram{
		{"a missing interpreter", "#!/nonexistent/interpreter\n", 0o755, 127,
			"no such file or directory"},
		{"no #!", "echo " + output + "\n", 0o755, 126, "exec format error"},
		{"no execute permission", "#!/bin/sh\necho " + output + "\n", 0o644, 126,
			"permission denied"},
	}
}

// cannotRunCase is a command line that runs a tmux the system cannot run.
type cannotRunCase struct {
	args []string
	// answers is what tmux runs before it cannot: nothing, or a script that answers tmux -V and,
	// for any other command, the lookup. Where join's lookup finds no server, the script fails on the
	// way out, once the file has been moved.
	answers string
	// lookup is whether the command that cannot run is a session lookup.
	lookup bool
	stdout string
}

// stage says when the tmux of the case cannot run.
func (c cannotRunCase) stage() string {
	switch {
	case c.answers == "":
		return "at once"
	case c.lookup:
		return "after -V"
	}
	return "after the lookup"
}

var cannotRunCases = []cannotRunCase{
	{[]string{"list"}, "", false, ""},
	{[]string{"join", "-s", "x"}, "", false, ""},
	{[]string{"kill", "-s", "x"}, "", false, ""},
	{[]string{"detach", "-s", "x"}, "", false, ""},
	{[]string{"list"}, "echo 'tmux 3.7c'", true, ""},
	{[]string{"join", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
	{[]string{"kill", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
	{[]string{"detach", "-s", "x"}, "echo 'tmux 3.7c'", true, ""},
	{[]string{"join", "-s", "x", "--new"}, `case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; ` +
		`echo 'no server running on /fake' >&2; trap 'exit 1' EXIT`, false, cldTitle("x")},
	{[]string{"join", "-s", "x"},
		`case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, cldTitle("x")},
	{[]string{"kill", "-s", "x"},
		`case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, ""},
	{[]string{"detach", "-s", "x"},
		`case "$1" in -V) echo 'tmux 3.7c'; exit ;; esac; echo cld-x`, false, ""},
}

// A tmux the system cannot run ends cld as a shell would, after cld's message (decision 11.9).
// So does one that stops being runnable once it has answered tmux -V and the lookup, for join's,
// kill's or detach's command. A lookup it cannot run ends cld with status 1, as the script's did.
// list's lookup asks the server of a socket cld-x.
func TestCannotRunTmux(t *testing.T) {
	t.Parallel()
	for _, broken := range unrunnable("tmux 3.7c") {
		for _, test := range cannotRunCases {
			name := strings.Join(test.args, " ") + ", " + test.stage() + ", " + broken.what
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				socket(t, s, "cld-x")
				tmux := brokenTmux(t, s, broken, test.answers)
				// No other tmux on the PATH, so that cld takes one without the execute permission.
				path := filepath.Dir(tmux) + string(os.PathListSeparator) + s.Tools("claude", "mv")
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

// brokenTmux writes the tmux of broken, in a directory of its own, and returns its path. With
// answers, a script runs them first, then moves broken's file over itself.
func brokenTmux(t *testing.T, s *sandbox.Sandbox, broken unrunnableProgram, answers string) string {
	t.Helper()
	fake := filepath.Join(s.Root, "fake")
	if err := os.Mkdir(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	tmux := filepath.Join(fake, "tmux")
	if answers == "" {
		s.WriteProgram(tmux, broken.content, broken.mode)
		return tmux
	}
	s.WriteProgram(tmux+".broken", broken.content, broken.mode)
	script := "#!/bin/sh\n" + answers + "\nmv -f '" + tmux + ".broken' '" + tmux + "'\n"
	s.WriteProgram(tmux, script, 0o755)
	return tmux
}
