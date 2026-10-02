package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A working directory that is gone, or that cld can no longer enter.

// The terminal that a join in a pane moved runs join --moved, which enters the directory that join
// ran in. Where that has been removed since, the terminal's join refuses, naming it, with status 1
// and before any tool is looked for.
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
	result := s.RunCld(map[string]string{"PATH": s.Tools()},
		"join", "--switched-from", "a", "--moved="+moved(dir, "-s", "x"))
	want := "cld: cannot enter " + dir + ", where cld join ran: no such file or directory\n"
	checkFailed(t, result, 1, want)
}

// join refuses a working directory that no longer exists where it would create the session
// (decision 11.10). There claude --version fails, so cld refuses before running it. Linux only:
// what macOS's getcwd does in a removed directory has not been checked.
func TestJoinRefusesARemovedDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("getcwd in a removed directory is checked on Linux only")
	}
	for _, args := range [][]string{
		{"join", "-s", "x"}, {"join", "-s", "x", "-w"}, {"join", "-s", "x", "--resume", "SESSION"},
		{"join"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			// sh makes the directory, moves into it and removes it, then runs cld there.
			script := `mkdir "$1" && cd "$1" && rmdir "$1" && shift && exec "$0" "$@"`
			removed := filepath.Join(s.Work, "removed")
			cmd := exec.Command("/bin/sh",
				append([]string{"-c", script, sandbox.Cld, removed}, args...)...)
			cmd.Env = s.Environ(fakeTmuxEnv(s, nil))
			checkFailed(t, runCommand(t, cmd), 1, "cld: the current directory no longer exists\n")
			if fakeTmuxRan(s) {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// join refuses a working directory it can no longer enter, or whose parent it cannot, with PWD set
// or not (decision 11.10). Otherwise tmux would start claude in the home directory, where claude
// --version fails. The test's sh enters the directory, takes the permissions away and runs cld
// there, as nobody under root. Linux only, as TestJoinRefusesARemovedDirectory.
func TestJoinRefusesADirectoryItCannotEnter(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("a directory that cannot be entered is checked on Linux only")
	}
	for _, test := range []struct {
		name string
		// locked is the directory whose permissions go, relative to the current one.
		locked string
		// pwd is how env passes PWD on to cld, whose Getwd reads PWD first.
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
			dir := lockableDirectory(t, s)
			script := `cd "$1" && chmod 000 "$2" && shift 2 && exec env ` + test.pwd + ` "$0" "$@"`
			cmd := exec.Command("/bin/sh", "-c", script, sandbox.Cld, dir, test.locked,
				"join", "-s", "x")
			if os.Geteuid() == 0 {
				runAsNobody(t, cmd, s.Root, filepath.Dir(dir), dir)
			}
			cmd.Env = s.Environ(fakeTmuxEnv(s, nil))
			checkFailed(t, runCommand(t, cmd), 1,
				"cld: cannot enter the current directory: permission denied\n")
			if fakeTmuxRan(s) {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// lockableDirectory makes s's directory above/dir, and returns it. Both get their permissions back
// as the test ends, for the sandbox to go.
func lockableDirectory(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	above := filepath.Join(s.Root, "above")
	dir := filepath.Join(above, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, path := range []string{above, dir} {
			_ = os.Chmod(path, 0o755) //nolint:errcheck // best effort, as the sandbox's removal
		}
	})
	return dir
}

// runAsNobody has cmd run as nobody (65534), who then owns paths: root enters any directory.
func runAsNobody(t *testing.T, cmd *exec.Cmd, paths ...string) {
	t.Helper()
	const nobody = 65534
	for _, path := range paths {
		if err := os.Chown(path, nobody, nobody); err != nil {
			t.Fatal(err)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: nobody, Gid: nobody}}
}
