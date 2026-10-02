package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The versions of tmux and claude that cld requires.

// cld requires the oldest tmux its tests run on, 3.5a (decision 6). A letter, a bug-fix release,
// counts after the major and minor version, so 3.5 is older. Development builds are read from
// what follows "next-", and pass without a version.
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
				started := fakeTmuxRan(s)
				if accepted && (result.Code != 0 || !started) {
					t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
				}
				want := "cld: tmux 3.5a or newer is required, found '" + version + "'\n"
				if !accepted && (result.Code != 1 || result.Stderr != want || started) {
					t.Errorf("accepted: exit %d, stderr %q, tmux started: %v",
						result.Code, result.Stderr, started)
				}
			})
		}
	}
}

// join requires claude 2.1.232 where it starts claude, comparing versions by their numbers
// (decision 6). So 2.1.30 is older, and output without a version passes. An older claude is
// refused before tmux starts. The probe answers --version leaving no record, as the fake tmux
// starts no claude.
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
				started := fakeTmuxRan(s)
				if accepted && (result.Code != 0 || !started) {
					t.Errorf("rejected: exit %d, stderr %q", result.Code, result.Stderr)
				}
				want := "cld: claude 2.1.232 or newer is required, found '" + version + "'\n"
				if !accepted && (result.Code != 1 || result.Stderr != want || result.Stdout != "" ||
					started) {
					t.Errorf("accepted: exit %d, stdout %q, stderr %q, tmux started: %v",
						result.Code, result.Stdout, result.Stderr, started)
				}
				if probes := s.Probes(); len(probes) != 0 {
					t.Errorf("claude --version left %d probe record(s)", len(probes))
				}
			})
		}
	}
}

// versionRequired starts the message for a claude whose --version fails.
const versionRequired = "cld: claude 2.1.232 or newer is required, but "

// claudeVersionCases are claudes whose --version fails or cannot run, and how join ends.
var claudeVersionCases = []struct {
	// script is claude, the system's false where empty
	name, script string
	code         int
	// want is stderr, CLAUDE standing for claude's path; with prefix, what stderr starts with.
	want   string
	prefix bool
}{
	{"false", "", 1, versionRequired + "claude --version exited with status 1", true},
	{"status 3", "#!/bin/sh\necho partial\necho 'claude: cannot load' >&2\nexit 3\n", 1,
		versionRequired + "claude --version exited with status 3: partial\nclaude: cannot load\n",
		false},
	{"signal", "#!/bin/sh\nkill -TERM $$\n", 1,
		versionRequired + "claude --version exited with signal 15\n", false},
	{"missing interpreter", "#!/nonexistent/interpreter\n", 127,
		"cld: cannot run CLAUDE: no such file or directory\n", false},
	{"no #!", "echo '2.1.100 (Claude Code)'\n", 1,
		"cld: claude 2.1.232 or newer is required, found '2.1.100 (Claude Code)'\n", false},
	{"no #!, status 3", "echo 'claude: cannot load' >&2\nexit 3\n", 1,
		versionRequired + "claude --version exited with status 3: claude: cannot load\n", false},
	// A binary the system will not execute is no script, so such a claude cannot run. Here that
	// is an ELF header alone, as of one cut short or for another machine, or a NUL in line one.
	{"ELF", "\x7fELF\x02\x01\x01" + strings.Repeat("\x00", 57), 126,
		"cld: cannot run CLAUDE: exec format error\n", false},
	{"NUL", "echo '2.1.300 (Claude Code)'\x00\n", 126,
		"cld: cannot run CLAUDE: exec format error\n", false},
}

// A claude whose --version fails is refused before tmux starts, with status 1 and its stdout and
// stderr (decision 6). As claude, false exits 1, and GNU's or uutils' prints something first. A
// claude that cannot run ends cld as such a tmux does (see TestCannotRunTmux). A script without
// #! runs with /bin/sh, as tmux's execvp runs it (see TestStartsTheClaudeItChecks).
func TestRequiresClaudeVersion(t *testing.T) {
	t.Parallel()
	for _, test := range claudeVersionCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux")
			claude := filepath.Join(tools, "claude")
			writeClaude(t, s, claude, test.script)
			env := map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}
			result := s.RunCld(env, "join")
			want := strings.ReplaceAll(test.want, "CLAUDE", claude)
			if result.Code != test.code || result.Stdout != "" ||
				test.prefix && !strings.HasPrefix(result.Stderr, want) ||
				!test.prefix && result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, want)
			}
			if fakeTmuxRan(s) {
				t.Error("tmux started")
			}
		})
	}
}

// writeClaude writes script as the claude at path, or links the system's false there where
// script is empty.
func writeClaude(t *testing.T, s *sandbox.Sandbox, path, script string) {
	t.Helper()
	if script != "" {
		s.WriteProgram(path, script, 0o755)
		return
	}
	target, err := exec.LookPath("false")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// A claude --version that leaves a process holding its output, as a wrapper's update check might,
// holds join up for a second at most (decision 6). The check takes what it printed before it
// exited. The process runs for a minute, and cld must be done well before.
func TestClaudeVersionLeavesAProcessBehind(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, script string
		code         int
		stderr       string
	}{
		{"accepted", "echo '2.1.300 (Claude Code)'", 0, ""},
		{"too old", "echo '2.1.100 (Claude Code)'", 1,
			"cld: claude 2.1.232 or newer is required, found '2.1.100 (Claude Code)'\n"},
		{"failing", "echo 'claude: cannot load' >&2; exit 3", 1,
			versionRequired + "claude --version exited with status 3: claude: cannot load\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux", "sleep")
			behind := filepath.Join(s.Root, "behind")
			script := "#!/bin/sh\nsleep 60 &\necho $! >'" + behind + "'\n" + test.script + "\n"
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
			t.Cleanup(func() { killBehind(behind) })
			start := time.Now()
			env := map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}
			result := s.RunCldOnTerminal(env, "join")
			if took := time.Since(start); took > 20*time.Second {
				t.Errorf("join took %v, waiting on the process claude left behind", took)
			}
			stdout := ""
			if test.code == 0 {
				stdout = cldTitle("0")
			}
			if result.Code != test.code || result.Stdout != stdout || result.Stderr != test.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, stdout, test.stderr)
			}
			if handedOver := fakeTmuxRan(s); handedOver != (test.code == 0) {
				t.Errorf("cld handed over to tmux: %v, want %v", handedOver, test.code == 0)
			}
		})
	}
}

// killBehind ends the process whose PID the file behind holds, if that still runs.
func killBehind(behind string) {
	if pid, err := os.ReadFile(behind); err == nil {
		_ = exec.Command("kill", strings.TrimSpace(string(pid))).Run() //nolint:errcheck // it may be gone
	}
}
