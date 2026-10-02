package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// bash with ble.sh (https://github.com/akinomyoga/ble.sh) completes cld through the script setup
// completion bash wrote, typed into a tmux pane; without ble.sh the test skips (decision 27.5).

// blesh is the ~/.bashrc given bash-completion's main script and ble.sh, which it loads as Ubuntu's
// ~/.bashrc and ble.sh's instructions do. It turns off ble.sh's suggestions as you type. TAB
// completes, then writes its status and the line to ~/tab.PID, as any key would cancel it.
const blesh = `. %s
[[ $- == *i* ]] && source -- %s
PS1='$ '
if [[ ${BLE_VERSION-} ]]; then
    bleopt complete_auto_complete=
    function ble/widget/cld-test/complete {
        ble/widget/complete
        printf '%%s\n%%s' "$?" "$_ble_edit_str" >"$HOME/tab.$$.new" &&
            mv "$HOME/tab.$$.new" "$HOME/tab.$$"
    }
    ble-bind -f C-i cld-test/complete
fi
`

// bleshLines are what TestSetupCompletionBashBleSh types, and the line a TAB leaves: cld's
// completions, and where cld offers nothing, the line unchanged.
var bleshLines = []struct{ typed, want string }{
	{"cld jo", "cld join "},
	{"cld join --wo", "cld join --worktree "},
	{"cld join -n al", "cld join -n alpha "},
	{"cld join -n alpha -s ", "cld join -n alpha -s 1 "},
	{"cld det", "cld detach "},
	{"cld detach -n al", "cld detach -n alpha "},
	{"cld join f", "cld join f"},
	{"cld kill -s f", "cld kill -s f"},
	{"cld setup telemetry --collector-config f", "cld setup telemetry --collector-config f"},
	{"cld joni f", "cld joni f"},
}

// bash with ble.sh completes cld as bash does: a command, an option, a session's NAME and SUFFIX.
// Where cld offers nothing, ble.sh offers nothing either, not the file in the directory (decision
// 27). Each line is typed into a new bash in a tmux pane, then a TAB; with more than one command,
// the first TAB lists them with their descriptions.
func TestSetupCompletionBashBleSh(t *testing.T) {
	t.Parallel()
	bash := lookShell(t, "bash")
	main := bashCompletion(t)
	ble := findBleSh(t)
	s := sandbox.New(t)
	if result := setupCompletion(s, "bash", nil); result.Code != 0 {
		t.Fatalf("setup completion bash: %+v", result)
	}
	s.WriteFile(filepath.Join(s.Home, ".bashrc"), fmt.Sprintf(blesh, main, ble))
	// ble.sh loads only where it can keep its cache, in ~/.cache if that is a directory (decision
	// 27.5).
	if err := os.Mkdir(filepath.Join(s.Home, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(filepath.Join(s.Work, "file"), "")
	// Session alpha-1, as list reads it: session cld-alpha-1 on its own server, marked as cld's.
	s.MustTmux("cld-alpha-1", "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
		"new-session", "-d", "-s", "cld-alpha-1", "sleep", "600")
	for i, test := range bleshLines {
		name := "case" + strconv.Itoa(i)
		if line := bleshTab(t, s, bash, name, test.typed); line != test.want {
			t.Errorf("%q, TAB: %q, want %q; the pane:\n%s",
				test.typed, line, test.want, bleshScreen(s, name))
		}
	}
	if line := bleshTab(t, s, bash, "commands", "cld "); line != "cld " {
		t.Errorf(`"cld ", TAB: %q, want "cld "`, line)
	}
	sandbox.WaitFor(t, 10*time.Second, "the commands with their descriptions", func() bool {
		commands := bleshScreen(s, "commands")
		return strings.Contains(commands, "attach to session NAME-SUFFIX") &&
			strings.Contains(commands, "update cld to the latest release")
	})
}

// findBleSh is ble.sh's main script: CLD_BLESH, where the test must run (make
// docker-blesh-test), or else the first installed; without either, the test skips.
func findBleSh(t *testing.T) string {
	t.Helper()
	ble := os.Getenv("CLD_BLESH")
	if ble == "" {
		paths := []string{"/usr/share/blesh/ble.sh", "/usr/local/share/blesh/ble.sh"}
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".local/share/blesh/ble.sh"))
		}
		return lookFile(t, "ble.sh", paths...)
	}
	if _, err := os.Stat(ble); err != nil {
		t.Fatalf("CLD_BLESH: %v", err)
	}
	return ble
}

// bleshScreen is what the pane of session name on the test's tmux server, bash, shows.
func bleshScreen(s *sandbox.Sandbox, name string) string {
	return s.MustTmux("bash", "capture-pane", "-p", "-t", "="+name+":")
}

// bleshTab types typed into a new bash in session name, then a TAB, and returns the line that
// ble.sh's completion leaves.
func bleshTab(t *testing.T, s *sandbox.Sandbox, bash, name, typed string) string {
	t.Helper()
	startBash(t, s, bash, name)
	pid := s.MustTmux("bash", "list-panes", "-t", "="+name, "-F", "#{pane_pid}")
	s.MustTmux("bash", "send-keys", "-t", "="+name+":", "-l", typed)
	s.MustTmux("bash", "send-keys", "-t", "="+name+":", "C-i")
	data := readTab(t, s, name, typed, pid)
	// ble.sh ends a completion that a key cancels with 148, leaving the line unchanged.
	status, line, _ := strings.Cut(string(data), "\n")
	if status == "148" {
		t.Fatalf("%q: ble.sh cancelled the completion; the pane:\n%s", typed, bleshScreen(s, name))
	}
	return line
}

// startBash starts bash in session name on the test's tmux server, bash, and waits for ble.sh's
// prompt, as keys that come before it are lost. The pane gets this client's PATH, with the cld
// under test first, whatever -e says (decision 27.5).
func startBash(t *testing.T, s *sandbox.Sandbox, bash, name string) {
	t.Helper()
	cmd := exec.Command("tmux", "-L", "bash", "-f", "/dev/null", "new-session", "-d", "-s", name,
		"-x", "100", "-y", "20", "-c", s.Work, bash, "-i")
	cmd.Env = shellEnv(s, nil)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v: %s", err, out)
	}
	sandbox.WaitFor(t, 30*time.Second, "ble.sh's prompt", func() bool {
		lines := strings.Split(bleshScreen(s, name), "\n")
		return lines[len(lines)-1] == "$"
	})
}

// readTab waits for ~/tab.PID, which TAB writes in the bash of process pid, and returns it.
func readTab(t *testing.T, s *sandbox.Sandbox, name, typed, pid string) []byte {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if data, err := os.ReadFile(filepath.Join(s.Home, "tab."+pid)); err == nil {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q: ble.sh wrote no line in 20s; the pane:\n%s", typed, bleshScreen(s, name))
		}
	}
}
