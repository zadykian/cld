package tests

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// Running the interactive cld list on a terminal under sh, and reading what it shows and leaves.

// The interactive list's footers: its hints, and the kill that Ctrl+X arms on a detached row and on
// an attached one. On a row that has ended, Ctrl+X arms the forget.
const (
	listHints         = "↑/↓ to navigate · enter to join · ctrl+x to kill · esc to quit"
	killArmed         = "ctrl+x again to kill · esc to keep"
	killArmedAttached = "ctrl+x again to kill and detach its terminal · esc to keep"
	endedHints        = "↑/↓ to navigate · enter to resume · ctrl+x to forget · esc to quit"
	forgetArmed       = "ctrl+x again to forget · esc to keep"
)

// listScript runs cld list, as "$@", recording what listRun reads.
const listScript = `tty >"$0.tty"; stty -g >"$0.before"; "$@"; ` +
	`echo $? >"$0.code"; stty -g >"$0.after"`

// listRun is cld list run under sh, with the files sh writes: the terminal's name (tty), its
// mode before and after cld (stty -g), and cld's exit status. The script gets the files' common
// path as $0 and cld list as "$@".
type listRun string

// startList runs script in term, in the sandbox's environment with extra variables added. sh
// then sleeps, so that the terminal shows what cld left behind rather than tmux's "Pane is dead".
func startList(
	t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, script string,
	extra map[string]string,
) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh"}, script, extra)
}

// startListIn is startList with shell, a command line, in place of sh.
func startListIn(
	t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, shell []string, script string,
	extra map[string]string,
) listRun {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "list.") //nolint:usetesting // sh outlives t.TempDir's cleanup
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	maps.Copy(env, s.Env)
	maps.Copy(env, extra)
	run := listRun(filepath.Join(dir, "cld"))
	argv := append(slices.Clone(shell), "-c", script+"; exec sleep 600", string(run))
	term.Start(append(argv, s.CldArgv("list")...), env, s.Work)
	return run
}

// exited reports whether cld has exited.
func (r listRun) exited() bool {
	_, err := os.Stat(string(r) + ".code")
	return err == nil
}

// read waits for the named file to hold a line and returns it.
func (r listRun) read(t *testing.T, name string) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write "+name, func() bool {
		var err error
		data, err = os.ReadFile(string(r) + "." + name)
		return err == nil && len(data) > 0 && data[len(data)-1] == '\n'
	})
	return strings.TrimSuffix(string(data), "\n")
}

// code is cld's exit status, once it has exited.
func (r listRun) code(t *testing.T) string {
	t.Helper()
	return r.read(t, "code")
}

// checkRaw checks that the terminal is in raw mode: no line editing, no echo, and Ctrl+C a key.
func (r listRun) checkRaw(t *testing.T) {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close() //nolint:errcheck // opened only to read its mode
	stty := exec.Command("stty", "-a")
	stty.Stdin = tty
	out, err := stty.Output()
	if err != nil {
		t.Fatalf("stty -a: %v", err)
	}
	for _, flag := range []string{"-icanon", "-echo", "-isig"} {
		if !slices.Contains(strings.Fields(strings.ReplaceAll(string(out), ";", " ")), flag) {
			t.Errorf("the terminal is not in raw mode: no %s in\n%s", flag, out)
		}
	}
}

// unread reports whether the terminal has input that cld has not read, as cld itself tells: in
// raw mode, the terminal is readable once it has a byte.
func (r listRun) unread(t *testing.T) bool {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close() //nolint:errcheck // opened only to poll it
	fd := int(tty.Fd())
	var set unix.FdSet
	set.Set(fd)
	n, err := unix.Select(fd+1, &set, nil, nil, &unix.Timeval{})
	return err == nil && n > 0
}

// readTTY is the terminal's name; uutils' tty (0.8.0) prints it without a newline.
func (r listRun) readTTY(t *testing.T) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write the terminal's name", func() bool {
		var err error
		data, err = os.ReadFile(string(r) + ".tty")
		return err == nil && len(data) > 0
	})
	return string(data)
}

// checkRestored checks that cld left the terminal as it found it: the same mode (stty -g), the
// main screen, no mouse reporting and the cursor visible. The terminal may still be reading
// what cld wrote last when sh has written its exit status.
func (r listRun) checkRestored(t *testing.T, term terminal.Terminal) {
	t.Helper()
	if before, after := r.read(t, "before"), r.read(t, "after"); before != after {
		t.Errorf("stty -g after cld\n%s\nwant as before\n%s", after, before)
	}
	waitModes(t, term, "after cld", "the main screen, no mouse and the cursor visible",
		func(modes terminal.Modes) bool {
			return !modes.AltScreen && !modes.Mouse && modes.Cursor
		})
}

// armThen presses Ctrl+X and runs armed, which waits for the arm and checks what it will. Then it
// presses then, such as C-x or Escape, within the arm's two seconds (decision 15). Where armed took
// a second or more under load, a letter disarms a kill still armed, Ctrl+X arms it again, and then
// follows.
func armThen(t *testing.T, term terminal.Terminal, armed func(), then string) {
	t.Helper()
	pressed := time.Now()
	term.Keys("C-x")
	armed()
	if time.Since(pressed) < time.Second {
		term.Keys(then)
		return
	}
	term.Keys("k", "C-x", then)
}

// afterList waits for cld to leave the alternate screen and print want after it, with the
// terminal's CR LF line ends as LF. The output log trails the screen and cld's exit status: the
// outer terminal writes it as it goes, through a pipe.
func afterList(t *testing.T, term terminal.Terminal, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		output := string(term.Output())
		end := strings.LastIndex(output, "\x1b[?1049l")
		printed := ""
		if end >= 0 {
			printed = strings.ReplaceAll(output[end+len("\x1b[?1049l"):], "\r\n", "\n")
			if printed == want {
				return
			}
		}
		if time.Now().After(deadline) {
			if end < 0 {
				t.Fatalf("timed out after 10s: cld never left the alternate screen: %q", output)
			}
			t.Fatalf("timed out after 10s: printed on leaving\n%q\nwant\n%q", printed, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// selectedRow is the name on the row the list marks as selected, or "" when it marks none.
func selectedRow(term terminal.Terminal) string {
	for line := range strings.SplitSeq(term.Screen(), "\n") {
		if row, marked := strings.CutPrefix(line, "> "); marked {
			if fields := strings.Fields(row); len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}
