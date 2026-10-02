package tests

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld restore after a reboot, each server killed with kill-server, which runs no hook; and the
// helpers the restore_*_test.go files share (decision 48).

// continuePrompt is what restore gives the claude of a session that was in a turn.
const continuePrompt = "The machine restarted while you were working; " +
	"continue where you left off."

// startCldAsync starts cld with args in dir, without a terminal, as RunCldIn runs it, and returns
// what waits for it to exit.
func startCldAsync(
	t *testing.T, s *sandbox.Sandbox, dir string, extra map[string]string, args ...string,
) func() sandbox.Result {
	t.Helper()
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Dir = s.Environ(extra), dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return func() sandbox.Result {
		t.Helper()
		err := cmd.Wait()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatalf("run cld: %v", err)
		}
		return sandbox.Result{
			Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String(),
		}
	}
}

// checkExit reports a run of cld, named by what, that did not exit with code and write stdout
// and stderr.
func checkExit(t *testing.T, what string, result sandbox.Result, code int, stdout, stderr string) {
	t.Helper()
	if result.Code != code || result.Stdout != stdout || result.Stderr != stderr {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
			what, result.Code, result.Stdout, result.Stderr, code, stdout, stderr)
	}
}

// mustKill kills session name, and ends the test where kill fails.
func mustKill(t *testing.T, s *sandbox.Sandbox, name string) {
	t.Helper()
	if result := s.RunCld(nil, "kill", "-s", name); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
}

// waitServerExit waits for the server of session name to exit.
func waitServerExit(t *testing.T, s *sandbox.Sandbox, name string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, "server cld-"+name+" to exit", func() bool {
		_, err := s.Tmux("cld-"+name, "list-sessions")
		return err != nil
	})
}

// waitForLock waits, as what says, for a second cld to wait for the lock of cld's record in s. On
// Linux that is two cld processes with the lock file open, which Lock keeps open while it waits.
// A cld that takes no lock then fails, where a sleep would pass one too slow to get there.
// Elsewhere it gives the second cld a second.
func waitForLock(t *testing.T, s *sandbox.Sandbox, what string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		time.Sleep(time.Second)
		return
	}
	cld, err := filepath.EvalSymlinks(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(s.Home)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(home, ".local", "state", "cld", "lock")
	sandbox.WaitFor(t, 10*time.Second, what, func() bool { return openedBy(cld, lock) >= 2 })
}

// openedBy counts the processes running the executable cld that have the file lock open.
func openedBy(cld, lock string) int {
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	opened := 0
	for _, proc := range procs {
		if hasOpen(filepath.Join("/proc", proc.Name()), cld, lock) {
			opened++
		}
	}
	return opened
}

// hasOpen reports whether the process of the directory dir in /proc runs the executable cld and
// has the file lock open.
func hasOpen(dir, cld, lock string) bool {
	if exe, err := os.Readlink(filepath.Join(dir, "exe")); err != nil || exe != cld {
		return false
	}
	fds, err := os.ReadDir(filepath.Join(dir, "fd"))
	if err != nil {
		return false
	}
	return slices.ContainsFunc(fds, func(fd os.DirEntry) bool {
		target, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
		return err == nil && target == lock
	})
}

// After a reboot, restore brings back detached each session with a run mark, as join would, from
// wherever it runs. It leaves ended the sessions that kill and claude's /exit ended, and run again
// it brings back none.
func TestRestore(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// dir's name, as the work directory's, leaves nothing of a session's name.
	dir, elsewhere := filepath.Join(s.Root, "project", "_"), filepath.Join(s.Root, "elsewhere")
	for _, d := range []string{dir, elsewhere} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	claudes := joinDetached(t, s, dir, "a", "b", "c", "d")
	reboot(t, s, claudes)
	marks := map[string]bool{"a": true, "b": true, "c": false, "d": false}
	checkMarks(t, s, "before restore", marks)

	result := s.RunCldIn(elsewhere, map[string]string{"CLD_TEST_ORIGIN": "restore"}, "restore")
	checkExit(t, "restore", result, 0, "Restored session 'a' in "+dir+"\n"+
		"Restored session 'b' in "+dir+", continuing its turn\n", "")
	checkRestoredClaudes(t, s, dir, claudes)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
	if clients := s.Clients(); len(clients) != 0 {
		t.Errorf("terminals attached %q, want none", clients)
	}
	checkMarks(t, s, "after restore", marks)
	if exists(companionFile(s, "b", ".busy")) {
		t.Error("b's busy mark is left as restore brought it back")
	}
	if got, want := readEntry(s, "b"), entry("b", dir, secondID); got != want {
		t.Errorf("b's entry %q, want %q", got, want)
	}
	checkExit(t, "restore again", s.RunCldIn(elsewhere, nil, "restore"), 0, "", "")
	if probes := s.Probes(); len(probes) != 6 {
		t.Errorf("%d claudes started after restore again, want 6", len(probes))
	}
}

// joinDetached joins the sessions names in dir, each with words after -- and detached once
// claude runs, and returns their claudes by name.
func joinDetached(
	t *testing.T, s *sandbox.Sandbox, dir string, names ...string,
) map[string]*sandbox.Probe {
	t.Helper()
	extra := map[string]string{"CLD_TEST_ORIGIN": "join"}
	claudes := map[string]*sandbox.Probe{}
	for i, name := range names {
		term := startCldIn(t, s, "tmux", dir, extra, "join", "-s", name, "--", "--model", "opus")
		claudes[name] = s.WaitProbes(i + 1)[i]
		waitClients(t, s, 1)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
	return claudes
}

// reboot gives a and b conversations, b in a turn, and ends c with kill and d with claude's
// /exit. It then ends a and b as a reboot would, with kill-server.
func reboot(t *testing.T, s *sandbox.Sandbox, claudes map[string]*sandbox.Probe) {
	t.Helper()
	claudes["a"].Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	claudes["b"].Hook("SessionStart", `{"session_id":"`+secondID+`","source":"startup"}`)
	claudes["b"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	mustKill(t, s, "c")
	claudes["d"].Send("exit")
	for _, name := range []string{"a", "b"} {
		s.MustTmux("cld-"+name, "kill-server")
	}
	for name, claude := range claudes {
		sandbox.WaitFor(t, 10*time.Second, "claude "+name+" to exit", func() bool {
			return !claude.Alive()
		})
		waitServerExit(t, s, name)
	}
}

// checkRestoredClaudes checks the claudes restore started for a and b, beside those of before.
// Each resumes its conversation, b continuing its turn, without the words after --, in dir and
// with the session's environment.
func checkRestoredClaudes(
	t *testing.T, s *sandbox.Sandbox, dir string, before map[string]*sandbox.Probe,
) {
	t.Helper()
	probes := s.WaitProbes(6)
	if len(probes) != 6 {
		t.Fatalf("%d claudes started, want 6: a and b brought back", len(probes))
	}
	restored := map[string]*sandbox.Probe{}
	for _, probe := range probes {
		if !slices.ContainsFunc(slices.Collect(maps.Values(before)), func(c *sandbox.Probe) bool {
			return c.PID == probe.PID
		}) {
			restored[probe.Argv[1]] = probe
		}
	}
	for name, after := range map[string][]string{
		"cld-a": {"--resume", firstID},
		"cld-b": {"--resume", secondID, continuePrompt},
	} {
		probe := restored[name]
		if probe == nil {
			t.Errorf("no claude of %s", name)
			continue
		}
		want := append([]string{"--name", name, "--settings", sessionSettings(s, name, dir)}, after...)
		if !slices.Equal(probe.Argv, want) {
			t.Errorf("claude arguments of %s %q, want %q", name, probe.Argv, want)
		}
		if probe.Cwd != dir || probe.Env["PWD"] != dir {
			t.Errorf("claude of %s runs in %s, PWD %q, want %s", name, probe.Cwd, probe.Env["PWD"], dir)
		}
		if origin := probe.Env["CLD_TEST_ORIGIN"]; origin != "join" {
			t.Errorf("claude of %s got CLD_TEST_ORIGIN=%q, want the session's own, join", name, origin)
		}
	}
}
