package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// cld restore and cld setup restore: the run mark, the busy mark and the environment beside a
// session's entry in cld's record; the sessions restore brings back, as a reboot leaves them -
// their servers gone, killed here with kill-server, which runs no hook - and those it leaves
// ended; what refuses a session, and the servers it leaves alone; two restores, a restore and a
// resume, and a restore and the interactive list's forget, at once; and setup restore against the
// fake systemctl and loginctl (see probe).

// continuePrompt is what restore gives the claude of a session that was in a turn.
const continuePrompt = "The machine restarted while you were working; continue where you left off."

// companionFile is the file beside session name's entry in cld's record in s with the extension
// ext: .run, .busy or .env.
func companionFile(s *sandbox.Sandbox, name, ext string) string {
	return strings.TrimSuffix(entryFile(s, name), ".json") + ext
}

// exists reports whether there is a file at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// checkMarks reports the run marks of the sessions in s that are not as want has them, by name:
// true for a mark there. A mark that should be there is waited for: tmux makes it once it has made
// the session, in a run-shell that can end after the terminal has attached and claude has started.
func checkMarks(t *testing.T, s *sandbox.Sandbox, when string, want map[string]bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for name, marked := range want {
		got := exists(companionFile(s, name, ".run"))
		for marked && !got && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			got = exists(companionFile(s, name, ".run"))
		}
		if got != marked {
			t.Errorf("%s: session %s's run mark there: %v, want %v", when, name, got, marked)
		}
	}
}

// recorded is the environment file cld writes beside an entry: the claude it started, by its
// path, and the environment of its server.
type recorded struct {
	Claude      string   `json:"claude"`
	Environment []string `json:"environment"`
}

// writeRestorable writes session name's entry in s - claude started in dir, in the conversation
// of that ID - with its run mark and the environment file naming claude and env, as new would
// have left them for a session a reboot ended.
func writeRestorable(t *testing.T, s *sandbox.Sandbox, name, dir, conversation, claude string, env []string) {
	t.Helper()
	writeEntry(t, s, name, dir, conversation)
	data, err := json.Marshal(recorded{Claude: claude, Environment: env})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companionFile(s, name, ".env"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(companionFile(s, name, ".run"), "")
}

// startCldAsync starts cld with args in dir, without a terminal, as RunCldIn runs it, and returns
// what waits for it to exit.
func startCldAsync(t *testing.T, s *sandbox.Sandbox, dir string, extra map[string]string, args ...string) func() sandbox.Result {
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
		return sandbox.Result{Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
	}
}

// waitForLock waits, as what says, for a second cld to wait for the lock of cld's record in s,
// which a first holds: on Linux, for two cld processes to have the lock file open, as Lock opens
// it before it tries for the lock - every 20 ms - and keeps it open while it waits, as while it
// holds it, so that a cld that takes no lock fails the test, rather than passing where it is too
// slow to get there before the first is let go. Elsewhere, with no /proc to tell, it gives the
// second cld a second.
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
	sandbox.WaitFor(t, 10*time.Second, what, func() bool {
		procs, _ := os.ReadDir("/proc")
		opened := 0
		for _, proc := range procs {
			dir := filepath.Join("/proc", proc.Name())
			if exe, err := os.Readlink(filepath.Join(dir, "exe")); err != nil || exe != cld {
				continue
			}
			fds, _ := os.ReadDir(filepath.Join(dir, "fd"))
			if slices.ContainsFunc(fds, func(fd os.DirEntry) bool {
				target, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
				return err == nil && target == lock
			}) {
				opened++
			}
		}
		return opened >= 2
	})
}

// new and resume write the environment a session's server starts with beside its entry - the
// claude they checked and their own environment without the variables that name the terminal,
// readable by the user alone - and their tmux makes its run mark there, once it has made the
// session. kill removes the run mark and keeps the entry, and so does claude's exit with status 0,
// which the pane-died hook sees on a pane that tmux keeps, and closes; a claude that fails keeps
// it, with its session. resume makes it again.
func TestRunMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	extra := map[string]string{"CLD_TEST_ORIGIN": "new", "TERMINAL_EMULATOR": "JetBrains-JediTerm"}
	term := startCld(t, s, "tmux", extra, "new", "-s", "a", "--", "--model", "opus")
	a := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after new", map[string]bool{"a": true})
	info, err := os.Stat(companionFile(s, "a", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("environment file's mode %v, want 0600", mode)
	}
	var env recorded
	data, err := os.ReadFile(companionFile(s, "a", ".env"))
	if err != nil || json.Unmarshal(data, &env) != nil {
		t.Fatalf("environment file %q: %v", data, err)
	}
	if want := filepath.Join(sandbox.ProbeBin, "claude"); env.Claude != want {
		t.Errorf("environment file names claude %q, want %q", env.Claude, want)
	}
	for _, variable := range []string{"CLD_TEST_ORIGIN=new", "CLD_PROBE_DIR=" + s.ProbeDir, "HOME=" + s.Home} {
		if !slices.Contains(env.Environment, variable) {
			t.Errorf("environment file lacks %s:\n%q", variable, env.Environment)
		}
	}
	if slices.ContainsFunc(env.Environment, func(v string) bool { return strings.HasPrefix(v, "TERMINAL_EMULATOR=") }) {
		t.Errorf("environment file keeps TERMINAL_EMULATOR:\n%q", env.Environment)
	}
	if origin := a.Env["CLD_TEST_ORIGIN"]; origin != "new" {
		t.Errorf("claude got CLD_TEST_ORIGIN=%q, want new, as the environment file has it", origin)
	}

	if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after kill", map[string]bool{"a": false})
	if readEntry(s, "a") == "" || !exists(companionFile(s, "a", ".env")) {
		t.Error("kill removed the entry or the environment")
	}

	term = startCld(t, s, "tmux", nil, "resume", "-s", "a")
	resumed := s.WaitProbes(2)[1]
	waitClients(t, s, 1)
	checkMarks(t, s, "after resume", map[string]bool{"a": true})
	resumed.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !resumed.Alive() })
	waitScreen(t, term, "claude exited with status 1")
	checkMarks(t, s, "after claude failed", map[string]bool{"a": true})
	if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })

	term = startCld(t, s, "tmux", nil, "resume", "-s", "a")
	again := s.WaitProbes(3)[2]
	waitClients(t, s, 1)
	checkMarks(t, s, "after resume", map[string]bool{"a": true})
	again.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after claude exited with status 0", map[string]bool{"a": false})
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after claude exited with status 0, want none", sessions)
	}
}

// claude's hooks keep the busy mark beside the session's entry: UserPromptSubmit makes it, and
// Stop, StopFailure, an interrupt in a tool (PostToolUseFailure with is_interrupt) and idle_prompt
// remove it; another failure of a tool leaves it, as does a notification of another type. A
// forgotten entry gets no mark.
func TestBusyMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new", "-s", "a")
	probe := s.WaitProbes(1)[0]
	busy := companionFile(s, "a", ".busy")
	for _, step := range []struct {
		event, input string
		busy         bool
	}{
		{"UserPromptSubmit", `{"prompt":"go"}`, true},
		{"Stop", "", false},
		{"UserPromptSubmit", "", true},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":false}`, true},
		{"Notification", `{"notification_type":"permission_prompt"}`, true},
		{"StopFailure", "", false},
		{"UserPromptSubmit", "", true},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt": true}`, false},
		{"UserPromptSubmit", "", true},
		{"Notification", `{"notification_type":"idle_prompt"}`, false},
	} {
		probe.Hook(step.event, step.input)
		if got := exists(busy); got != step.busy {
			t.Errorf("busy mark after %s %s: %v, want %v", step.event, step.input, got, step.busy)
		}
	}
	forget(t, s, "a")
	probe.Hook("UserPromptSubmit", "")
	if exists(busy) {
		t.Error("UserPromptSubmit made a busy mark for a forgotten entry")
	}
}

// After a reboot - here, each server killed with kill-server, which ends claude and runs no hook -
// restore brings back the sessions with a run mark, detached, each as resume -n NAME -s SUFFIX
// would: claude resumes the recorded conversation in the directory the session ran in, from
// wherever restore runs, with the environment the session started with rather than restore's, and
// without the words given after --; a session whose claude was in a turn gets the prompt that
// has it continue, and loses its busy mark until claude takes that prompt. It leaves ended the
// sessions that kill and claude's own /exit ended. Run again, it finds them all running, and
// brings back none.
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
	extra := map[string]string{"CLD_TEST_ORIGIN": "new"}
	claudes := map[string]*sandbox.Probe{}
	for i, name := range []string{"a", "b", "c", "d"} {
		term := startCldIn(t, s, "tmux", dir, extra, "new", "-s", name, "--", "--model", "opus")
		claudes[name] = s.WaitProbes(i + 1)[i]
		waitClients(t, s, 1)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
	claudes["a"].Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	claudes["b"].Hook("SessionStart", `{"session_id":"`+secondID+`","source":"startup"}`)
	claudes["b"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	if result := s.RunCld(nil, "kill", "-s", "c"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	claudes["d"].Send("exit")
	for _, name := range []string{"a", "b"} {
		s.MustTmux("cld-"+name, "kill-server")
	}
	for name, claude := range claudes {
		sandbox.WaitFor(t, 10*time.Second, "claude "+name+" to exit", func() bool { return !claude.Alive() })
		sandbox.WaitFor(t, 10*time.Second, "server cld-"+name+" to exit", func() bool {
			_, err := s.Tmux("cld-"+name, "list-sessions")
			return err != nil
		})
	}
	checkMarks(t, s, "before restore", map[string]bool{"a": true, "b": true, "c": false, "d": false})

	result := s.RunCldIn(elsewhere, map[string]string{"CLD_TEST_ORIGIN": "restore"}, "restore")
	want := "Restored session 'a' in " + dir + "\n" + "Restored session 'b' in " + dir + ", continuing its turn\n"
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("restore: exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	probes := s.WaitProbes(6)
	if len(probes) != 6 {
		t.Fatalf("%d claudes started, want 6: a and b brought back", len(probes))
	}
	restored := map[string]*sandbox.Probe{}
	for _, probe := range probes {
		if !slices.ContainsFunc(slices.Collect(maps.Values(claudes)), func(c *sandbox.Probe) bool { return c.PID == probe.PID }) {
			restored[probe.Argv[1]] = probe
		}
	}
	for name, after := range map[string][]string{"cld-a": {"--resume", firstID}, "cld-b": {"--resume", secondID, continuePrompt}} {
		probe := restored[name]
		if probe == nil {
			t.Errorf("no claude of %s", name)
			continue
		}
		if want := append([]string{"--name", name, "--settings", sessionSettings(s, name, dir)}, after...); !slices.Equal(probe.Argv, want) {
			t.Errorf("claude arguments of %s %q, want %q", name, probe.Argv, want)
		}
		if probe.Cwd != dir || probe.Env["PWD"] != dir {
			t.Errorf("claude of %s runs in %s, PWD %q, want %s", name, probe.Cwd, probe.Env["PWD"], dir)
		}
		if origin := probe.Env["CLD_TEST_ORIGIN"]; origin != "new" {
			t.Errorf("claude of %s got CLD_TEST_ORIGIN=%q, want the session's own, new", name, origin)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
	if clients := s.Clients(); len(clients) != 0 {
		t.Errorf("terminals attached %q, want none", clients)
	}
	checkMarks(t, s, "after restore", map[string]bool{"a": true, "b": true, "c": false, "d": false})
	if exists(companionFile(s, "b", ".busy")) {
		t.Error("b's busy mark is left as restore brought it back")
	}
	if got, want := readEntry(s, "b"), entry("b", dir, secondID); got != want {
		t.Errorf("b's entry %q, want %q", got, want)
	}
	if result := s.RunCldIn(elsewhere, nil, "restore"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("restore again: exit %d, stdout %q, stderr %q, want exit 0 and nothing", result.Code, result.Stdout, result.Stderr)
	}
	if probes := s.Probes(); len(probes) != 6 {
		t.Errorf("%d claudes started after restore again, want 6", len(probes))
	}
}

// holdRm puts an rm first on the PATH of the environment it returns, for the servers that cld
// starts with it, and so for their run-shell: one that holds the removal of the file mark - a run
// mark - from start on, until release, and then runs the rm the tests run (see holdTmux).
func holdRm(t *testing.T, s *sandbox.Sandbox, what, mark string) heldTmux {
	t.Helper()
	rm, err := exec.LookPath("rm")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(s.Root, "hold.")
	if err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(dir, "hold")
	t.Cleanup(func() { _ = os.Remove(hold) })
	s.WriteProgram(filepath.Join(dir, "rm"), "#!/bin/sh\ncase \"$*\" in *'"+mark+"'*)\n"+
		"\techo $$ >>'"+hold+".begun'\n"+
		"\tif [ -e '"+hold+"' ]; then echo $$ >'"+hold+".held'; fi\n"+
		"\twhile [ -e '"+hold+"' ]; do sleep 0.05; done ;;\n"+
		"esac\nexec '"+rm+"' \"$@\"\n", 0o755)
	return heldTmux{hold: hold, what: what, env: map[string]string{"PATH": dir + string(os.PathListSeparator) + s.Env["PATH"]}}
}

// kill removes the session's run mark before its kill-session, in the one tmux command that ends
// the session, which tmux runs while it serves other clients: the session holds its name while sh
// removes the mark, held here, so that a session of the name that another cld's tmux makes just
// then - tmux itself here - is refused, duplicate session, where it would be made on the server
// that the kill-server then ends, and keep a run mark that restore would act on. The sweep of the
// idle sessions removes the mark so too, then checks again that the session is idle: a terminal
// that attaches while sh removes the mark keeps the session, without its mark.
func TestUnmarkBeforeTheKill(t *testing.T) {
	t.Parallel()
	t.Run("kill", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		rm := holdRm(t, s, "the kill's rm of x's run mark", companionFile(s, "x", ".run"))
		term := startCld(t, s, "tmux", rm.env, "new", "-s", "x")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		rm.start(t)
		kill := startCldAsync(t, s, s.Work, nil, "kill", "-s", "x")
		rm.held(t)
		if _, err := s.Tmux("cld-x", "new-session", "-d", "-s", "cld-x"); err == nil || !strings.Contains(err.Error(), "duplicate session: cld-x") {
			t.Errorf("new-session of cld-x as the kill removed its run mark: %v, want duplicate session", err)
		}
		rm.release(t)
		if result := kill(); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
			t.Errorf("kill: exit %d, stdout %q, stderr %q, want exit 0 and nothing", result.Code, result.Stdout, result.Stderr)
		}
		sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
		if sessions := s.Sessions(); len(sessions) != 0 {
			t.Errorf("sessions %q after the kill, want none", sessions)
		}
		checkMarks(t, s, "after the kill", map[string]bool{"x": false})
	})
	t.Run("idle", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		rm := holdRm(t, s, "the sweep's rm of x's run mark", companionFile(s, "x", ".run"))
		term := startCld(t, s, "tmux", rm.env, "new", "-s", "x")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
		// CLD_IDLE_DAYS 0.00001 is 0.864 s.
		time.Sleep(2 * time.Second)
		rm.start(t)
		list := startCldAsync(t, s, s.Work, map[string]string{"CLD_IDLE_DAYS": "0.00001"}, "list")
		rm.held(t)
		joined := startCld(t, s, "tmux", nil, "join", "-s", "x")
		waitClients(t, s, 1)
		rm.release(t)
		if result := list(); result.Code != 0 || result.Stderr != "" {
			t.Errorf("list: exit %d, stderr %q, want exit 0, no stderr", result.Code, result.Stderr)
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
			t.Errorf("sessions %q after the sweep, want [cld-x]", sessions)
		}
		if !joined.Running() {
			t.Errorf("join is not attached:\n%s", joined.Screen())
		}
		checkMarks(t, s, "after the sweep kept x", map[string]bool{"x": false})
	})
}

// A new or a resume that makes no session leaves no run mark, which the tmux that makes the
// session makes: a new refused for want of a terminal writes nothing in cld's record, and one
// whose tmux cannot open the terminal - a TERM that tmux does not know - leaves its entry, ended,
// but no mark; so does a resume there of a session that kill ended, whose mark kill removed.
// restore then brings none of them back.
func TestNoSessionNoMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "new", "-s", "x"); result.Code != 1 {
		t.Errorf("new without a terminal: exit %d, stderr %q, want exit 1", result.Code, result.Stderr)
	}
	for _, ext := range []string{".json", ".env", ".run"} {
		if exists(companionFile(s, "x", ext)) {
			t.Errorf("new without a terminal left x%s", ext)
		}
	}
	unknown := map[string]string{"TERM": "cld-no-such-terminal"}
	if result := s.RunCldOnTerminal(unknown, "new", "-s", "y"); result.Code == 0 {
		t.Errorf("new with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail", result.Stderr)
	}
	if got, want := readEntry(s, "y"), entry("y", s.Work, ""); got != want {
		t.Errorf("y's entry %q, want %q", got, want)
	}
	startCld(t, s, "tmux", nil, "new", "-s", "z")
	z := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after new", map[string]bool{"z": true})
	if result := s.RunCld(nil, "kill", "-s", "z"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !z.Alive() })
	if result := s.RunCldOnTerminal(unknown, "resume", "-s", "z"); result.Code == 0 {
		t.Errorf("resume with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail", result.Stderr)
	}
	checkMarks(t, s, "after tmux failed", map[string]bool{"x": false, "y": false, "z": false})
	if result := s.RunCld(nil, "restore"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("restore: exit %d, stdout %q, stderr %q, want exit 0 and nothing", result.Code, result.Stdout, result.Stderr)
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want z's alone", len(probes))
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// A resume that cannot write the environment beside the session's entry - a directory in its
// place here, where a full disk would do the same - makes no run mark of its own, but claude's exit
// with status 0 still removes the one the session has: here the mark of a session a reboot ended,
// which restore would otherwise bring back after the /exit that ended it on purpose.
func TestRunMarkWithoutEnvironment(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeRestorable(t, s, "a", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"), s.Environ(nil))
	env := companionFile(s, "a", ".env")
	if err := os.Remove(env); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(env, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, time.Hour, companionFile(s, "a", ".run"))
	term := startCld(t, s, "tmux", nil, "resume", "-s", "a")
	claude := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if info, err := os.Stat(env); err != nil || !info.IsDir() {
		t.Fatalf("resume wrote the environment in place of the directory: %v", err)
	}
	info, err := os.Stat(companionFile(s, "a", ".run"))
	if err != nil {
		t.Fatal(err)
	}
	if since := time.Since(info.ModTime()); since < 30*time.Minute {
		t.Errorf("run mark made %v ago after resume, want it kept from an hour ago", since.Round(time.Second))
	}
	claude.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkMarks(t, s, "after claude exited with status 0", map[string]bool{"a": false})
	if result := s.RunCld(nil, "restore"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("restore: exit %d, stdout %q, stderr %q, want exit 0 and nothing", result.Code, result.Stdout, result.Stderr)
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want the resumed one alone", len(probes))
	}
}

// restore leaves ended a session idle for longer than CLD_IDLE_DAYS by its run mark - neither
// started nor given a prompt since - saying so, and removes the mark, where the sweep of list and
// new, which goes by tmux's times, would count it as used from the restore on; it brings back one
// used within the limit, and leaves its mark's time as it was, so that a session that only restore
// starts ends in time all the same. claude's UserPromptSubmit hook touches the mark. A
// CLD_IDLE_DAYS that is no number of days is refused before anything comes back.
func TestRestoreIdle(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	writeRestorable(t, s, "fresh", s.Work, firstID, probe, s.Environ(nil))
	writeRestorable(t, s, "stale", s.Work, secondID, probe, s.Environ(nil))
	age(t, time.Hour, companionFile(s, "fresh", ".run"))
	age(t, 3*24*time.Hour+time.Minute, companionFile(s, "stale", ".run"))

	result := s.RunCld(map[string]string{"CLD_IDLE_DAYS": "2d"}, "restore")
	if want := "cld: CLD_IDLE_DAYS is not a number of days: '2d'\n"; result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("restore, CLD_IDLE_DAYS=2d: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	checkMarks(t, s, "after the refusal", map[string]bool{"fresh": true, "stale": true})

	result = s.RunCld(map[string]string{"CLD_IDLE_DAYS": "2"}, "restore")
	stdout, stderr := "Restored session 'fresh' in "+s.Work+"\n", "cld: left session 'stale' ended, idle for 3 days\n"
	if result.Code != 0 || result.Stdout != stdout || result.Stderr != stderr {
		t.Errorf("restore: exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr, stdout, stderr)
	}
	claude := s.WaitProbes(1)[0]
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-fresh"}) {
		t.Errorf("sessions %q, want [cld-fresh]", sessions)
	}
	checkMarks(t, s, "after restore", map[string]bool{"fresh": true, "stale": false})
	if readEntry(s, "stale") == "" {
		t.Error("restore forgot stale's entry")
	}
	markTime := func() time.Duration {
		t.Helper()
		info, err := os.Stat(companionFile(s, "fresh", ".run"))
		if err != nil {
			t.Fatal(err)
		}
		return time.Since(info.ModTime())
	}
	if since := markTime(); since < 30*time.Minute {
		t.Errorf("fresh's run mark made %v ago after restore, want it kept from an hour ago", since.Round(time.Second))
	}
	claude.Hook("UserPromptSubmit", `{"prompt":"go"}`)
	if since := markTime(); since > time.Minute {
		t.Errorf("fresh's run mark made %v ago after a prompt, want now", since.Round(time.Second))
	}
}

// A session restore cannot bring back is a warning, and the others come back, with status 1: one
// whose directory has gone, pointing at the resume that brings its conversation back from where
// it runs; one without the environment its server started with; one whose claude is older than
// cld runs, which restore checks where it starts it, as new and resume check theirs. A session
// without a run mark stays ended.
func TestRestoreFailures(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	env := append(s.Environ(nil), "CLD_TEST_ORIGIN=recorded")
	gone := filepath.Join(s.Root, "gone")
	old := filepath.Join(s.Root, "old claude")
	s.WriteProgram(old, "#!/bin/sh\necho '2.1.231 (Claude Code)'\n", 0o755)
	writeRestorable(t, s, "gone", gone, firstID, probe, env)
	writeRestorable(t, s, "noenv", s.Work, "", probe, env)
	if err := os.Remove(companionFile(s, "noenv", ".env")); err != nil {
		t.Fatal(err)
	}
	writeRestorable(t, s, "ok", s.Work, secondID, probe, env)
	writeRestorable(t, s, "old", s.Work, "", old, env)
	writeEntry(t, s, "unmarked", s.Work, "")

	result := s.RunCld(nil, "restore")
	wantStderr := "cld: warning: cannot restore session 'gone': session 'gone' ran in " + gone + ", which no longer exists; " +
		"resume it from here with cld resume -s gone " + firstID + "\n" +
		"cld: warning: cannot restore session 'noenv': cld keeps no environment of it: " + companionFile(s, "noenv", ".env") + ": no such file or directory\n" +
		"cld: warning: cannot restore session 'old': claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n"
	if want := "Restored session 'ok' in " + s.Work + "\n"; result.Code != 1 || result.Stdout != want || result.Stderr != wantStderr {
		t.Errorf("restore: exit %d, stdout %q, stderr\n%s\nwant exit 1, stdout %q, stderr\n%s", result.Code, result.Stdout, result.Stderr, want, wantStderr)
	}
	claude := s.WaitProbes(1)[0]
	if want := []string{"--name", "cld-ok", "--settings", sessionSettings(s, "cld-ok", s.Work), "--resume", secondID}; !slices.Equal(claude.Argv, want) {
		t.Errorf("claude arguments %q, want %q", claude.Argv, want)
	}
	if claude.Env["CLD_TEST_ORIGIN"] != "recorded" {
		t.Errorf("claude got CLD_TEST_ORIGIN=%q, want recorded", claude.Env["CLD_TEST_ORIGIN"])
	}
	time.Sleep(time.Second)
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want ok's alone", len(probes))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-ok"}) {
		t.Errorf("sessions %q, want [cld-ok]", sessions)
	}
}

// restore leaves alone, silently, a session whose server runs without it, as one that runs: a
// server of cld's that the session has outlived, or one cld did not start - the user's own tmux
// -L cld-NAME, whatever its sessions are called. No claude starts there, the session keeps its run
// and busy marks, and the server its sessions. A server that starts as restore's tmux is about to
// make a session - the user's own here, whose session takes the name - fails tmux's new-session:
// a warning, with status 1, and the session keeps both marks, as tmux cuts its command short
// before the run-shell that sets them, so that the next restore still has claude continue the
// turn.
func TestRestoreBesideServers(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	for _, name := range []string{"foreign", "lingering", "x"} {
		writeRestorable(t, s, name, s.Work, firstID, probe, s.Environ(nil))
		s.WriteFile(companionFile(s, name, ".busy"), "")
	}
	s.MustTmux("cld-foreign", "-f", "/dev/null", "new-session", "-d", "-s", "cld-foreign")
	s.MustTmux("cld-lingering", "-f", "/dev/null", "new-session", "-d", "-s", "other", ";", "set", "-s", "@cld", "1")
	checkLeft := func(when string) {
		t.Helper()
		for server, sessions := range map[string]string{"cld-foreign": "cld-foreign", "cld-lingering": "other"} {
			if got, err := s.Tmux(server, "list-sessions", "-F", "#{session_name}"); err != nil || got != sessions {
				t.Errorf("%s: sessions of server %s %q, %v, want %q", when, server, got, err, sessions)
			}
		}
		for _, name := range []string{"foreign", "lingering"} {
			if !exists(companionFile(s, name, ".run")) || !exists(companionFile(s, name, ".busy")) {
				t.Errorf("%s: %s lost its run mark or its busy mark", when, name)
			}
		}
	}

	tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
	tmux.start(t)
	first := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	s.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "cld-x")
	tmux.release(t)
	if result, want := first(), "cld: warning: cannot restore session 'x': duplicate session: cld-x\n"; result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("restore: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if !exists(companionFile(s, "x", ".run")) || !exists(companionFile(s, "x", ".busy")) {
		t.Error("x lost its run mark or its busy mark as restore's tmux failed")
	}
	checkLeft("after restore")
	if probes := s.Probes(); len(probes) != 0 {
		t.Errorf("%d claudes started, want none", len(probes))
	}

	s.MustTmux("cld-x", "kill-server")
	sandbox.WaitFor(t, 10*time.Second, "server cld-x to exit", func() bool {
		_, err := s.Tmux("cld-x", "list-sessions")
		return err != nil
	})
	if result, want := s.RunCld(nil, "restore"), "Restored session 'x' in "+s.Work+", continuing its turn\n"; result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("restore again: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
	}
	claude := s.WaitProbes(1)[0]
	if want := []string{"--name", "cld-x", "--settings", sessionSettings(s, "cld-x", s.Work), "--resume", firstID, continuePrompt}; !slices.Equal(claude.Argv, want) {
		t.Errorf("claude arguments %q, want %q", claude.Argv, want)
	}
	if exists(companionFile(s, "x", ".busy")) {
		t.Error("x's busy mark is left as restore brought it back")
	}
	checkLeft("after restore again")
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want x's alone", len(probes))
	}
}

// restore holds the record's lock for each session it brings back, from the lookup to tmux: a
// second restore at once, or a resume of the session, waits, and then finds the session running.
// The first restore's tmux is held as it is about to make the session, until the second cld waits
// for the lock (see waitForLock); without the lock, the second restore would make it too, and one
// of the two tmux commands fail with "duplicate session", and the resume would find no session and
// go on, failing without a terminal.
func TestRestoreAtOnce(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		// want is what the second cld prints, and its exit status
		code           int
		stdout, stderr string
	}{
		{"restore", []string{"restore"}, 0, "", ""},
		{"resume", []string{"resume", "-s", "x"}, 1, "", "cld: session 'x' exists in %s; attach to it with cld join -s x\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"), s.Environ(nil))
			tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
			tmux.start(t)
			first := startCldAsync(t, s, s.Work, tmux.env, "restore")
			tmux.held(t)
			second := startCldAsync(t, s, s.Work, tmux.env, test.args...)
			waitForLock(t, s, "the second "+test.name+" to wait for the record's lock")
			tmux.release(t)
			if result, want := first(), "Restored session 'x' in "+s.Work+"\n"; result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Errorf("first restore: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
			}
			stderr := test.stderr
			if strings.Contains(stderr, "%s") {
				stderr = strings.ReplaceAll(stderr, "%s", s.Work)
			}
			if result := second(); result.Code != test.code || result.Stdout != test.stdout || result.Stderr != stderr {
				t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q", test.name, result.Code, result.Stdout, result.Stderr, test.code, test.stdout, stderr)
			}
			if begun := tmux.begun(t); begun != 1 {
				t.Errorf("%d tmux commands made cld-x, want 1", begun)
			}
			s.WaitProbes(1)
			time.Sleep(500 * time.Millisecond)
			if probes := s.Probes(); len(probes) != 1 {
				t.Errorf("%d claudes started, want 1", len(probes))
			}
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
				t.Errorf("sessions %q, want [cld-x]", sessions)
			}
		})
	}
}

// The interactive list's forget of a session that restore is bringing back waits for the record's
// lock, which restore holds as its tmux, held here until the forget waits (see waitForLock), is
// about to make the session, then finds the session running and forgets nothing: its entry, its
// environment and its run mark stay. Without the lock, the forget would find no session and remove
// them as tmux made it, leaving a session that runs with no environment kept, which every restore
// after a reboot would warn of.
func TestForgetRacingRestore(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"), s.Environ(nil))
	tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
	tmux.start(t)
	restore := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, endedHints)
	armThen(t, term, func() { waitScreen(t, term, forgetArmed) }, "C-x")
	waitForLock(t, s, "the list's forget to wait for the record's lock")
	tmux.release(t)
	if result, want := restore(), "Restored session 'x' in "+s.Work+"\n"; result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("restore: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
	}
	waitScreen(t, term, "session 'x' runs again")
	if readEntry(s, "x") == "" {
		t.Error("the forget took the entry of the session restore brought back")
	}
	if !exists(companionFile(s, "x", ".env")) {
		t.Error("the forget took the environment of the session restore brought back")
	}
	checkMarks(t, s, "after the forget", map[string]bool{"x": true})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q, want [cld-x]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}

// systemdQuoted is value as setup restore writes it in the unit, in double quotes: "\" and "\""
// escaped, "%" doubled, and in ExecStart "$" doubled.
func systemdQuoted(value string, command bool) string {
	value = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(value)
	if command {
		value = strings.ReplaceAll(value, "$", "$$")
	}
	return `"` + value + `"`
}

// restoreUnit is the unit setup restore writes, where cld runs with the environment env: it runs
// cld, by the file it runs from, as cld restore, with the PATH, and the TMUX_TMPDIR,
// XDG_STATE_HOME and CLD_IDLE_DAYS that env sets.
func restoreUnit(t *testing.T, env map[string]string) string {
	t.Helper()
	cld, err := filepath.EvalSymlinks(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	environment := ""
	for _, name := range []string{"PATH", "TMUX_TMPDIR", "XDG_STATE_HOME", "CLD_IDLE_DAYS"} {
		if value, set := env[name]; set {
			environment += "Environment=" + systemdQuoted(name+"="+value, false) + "\n"
		}
	}
	return "# Written by cld setup restore: at login, or at boot with lingering on, cld restore\n" +
		"# brings back the sessions of cld that ran when the machine stopped.\n" +
		"[Unit]\nDescription=Restore cld's sessions\n\n" +
		"[Service]\nType=oneshot\nRemainAfterExit=yes\nKillMode=process\n" + environment +
		"ExecStart=" + systemdQuoted(cld, true) + " restore\n\n" +
		"[Install]\nWantedBy=default.target\n"
}

// setup restore writes the unit that runs cld restore, with the variables cld restore needs,
// quoted as systemd reads them, has the user's systemd read it and enables it, and says whether
// lingering is on, naming loginctl enable-linger where it is off. Run again it leaves the unit as
// it is, but for a variable that changed, and enables it again. Without systemd - no systemctl, or
// a systemctl --user that fails - it writes nothing, nor with a CLD_IDLE_DAYS that cld restore
// would refuse; a failure after the unit is written says so. The fake systemctl and loginctl
// record the calls (see probe).
func TestSetupRestore(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("setup restore works on Linux only; TestSetupRestoreLinuxOnly checks the refusal")
	}
	uid := strconv.Itoa(os.Getuid())
	show := []string{"systemctl", "--user", "show-environment"}
	reload := []string{"systemctl", "--user", "daemon-reload"}
	enable := []string{"systemctl", "--user", "enable", "cld-restore.service"}
	linger := []string{"loginctl", "show-user", uid, "--property=Linger", "--value"}
	const (
		enabled = "Enabled cld-restore.service: your systemd runs cld restore as it starts\n"
		off     = "Lingering is off: your systemd starts, and cld restore with it, at your first login, and at your last logout " +
			"ends the sessions cld restore brought back; loginctl enable-linger has it start at boot, and keeps them\n"
		on = "Lingering is on: your systemd starts at boot, and cld restore with it\n"
	)
	s := sandbox.New(t)
	unit := filepath.Join(s.Home, ".config", "systemd", "user", "cld-restore.service")
	state := filepath.Join(s.Root, `state 100% $HOME "q" \x`)
	env := map[string]string{"XDG_STATE_HOME": state, "CLD_IDLE_DAYS": "7.5"}
	check := func(when string, extra map[string]string, code int, stdout, stderr string, calls ...[]string) {
		t.Helper()
		before := len(s.SystemdCalls())
		result := s.RunCld(extra, "setup", "restore")
		if result.Code != code || result.Stdout != stdout || result.Stderr != stderr {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q", when, result.Code, result.Stdout, result.Stderr, code, stdout, stderr)
		}
		if got := s.SystemdCalls()[before:]; !slices.EqualFunc(got, calls, slices.Equal) {
			t.Errorf("%s: calls %q, want %q", when, got, calls)
		}
	}

	check("without systemctl", map[string]string{"PATH": s.Tools()}, 1, "", "cld: setup restore needs systemd, and systemctl is not installed\n")
	check("without a user manager", map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "show-environment"}, 1, "",
		"cld: setup restore needs your user's systemd: systemctl --user show-environment: fake systemctl --user show-environment failed\n", show)
	check("with CLD_IDLE_DAYS no number of days", map[string]string{"CLD_IDLE_DAYS": "7d"}, 1, "", "cld: CLD_IDLE_DAYS is not a number of days: '7d'\n")
	if exists(unit) {
		t.Fatal("setup restore wrote the unit without systemd")
	}

	check("the first time", env, 0, "Created "+unit+"\n"+enabled+off, "", show, reload, enable, linger)
	all := map[string]string{"PATH": s.Env["PATH"], "TMUX_TMPDIR": s.Env["TMUX_TMPDIR"], "XDG_STATE_HOME": state, "CLD_IDLE_DAYS": "7.5"}
	if data, err := os.ReadFile(unit); err != nil || string(data) != restoreUnit(t, all) {
		t.Errorf("the unit: %v\n%s\nwant\n%s", err, data, restoreUnit(t, all))
	}
	env["CLD_FAKE_LINGER"] = "yes"
	check("again", env, 0, "Left "+unit+" as it was\n"+enabled+on, "", show, enable, linger)
	delete(env, "XDG_STATE_HOME")
	delete(env, "CLD_IDLE_DAYS")
	check("without XDG_STATE_HOME and CLD_IDLE_DAYS", env, 0, "Updated "+unit+"\n"+enabled+on, "", show, reload, enable, linger)
	delete(all, "XDG_STATE_HOME")
	delete(all, "CLD_IDLE_DAYS")
	if data, err := os.ReadFile(unit); err != nil || string(data) != restoreUnit(t, all) {
		t.Errorf("the unit: %v\n%s\nwant\n%s", err, data, restoreUnit(t, all))
	}
	check("where loginctl fails", map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "show-user"}, 0,
		"Left "+unit+" as it was\n"+enabled+"Cannot tell whether lingering is on (loginctl show-user "+uid+" --property=Linger --value: "+
			"fake loginctl show-user "+uid+" --property=Linger --value failed): without it, "+strings.TrimPrefix(off, "Lingering is off: "), "",
		show, enable, linger)
	check("where enable fails", map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "enable"}, 1, "",
		"cld: systemctl --user enable cld-restore.service: fake systemctl --user enable cld-restore.service failed\n",
		show, enable)
	check("where enable fails after a change", map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "enable", "XDG_STATE_HOME": state}, 1, "",
		"cld: systemctl --user enable cld-restore.service: fake systemctl --user enable cld-restore.service failed ("+unit+" written before it)\n",
		show, reload, enable)
	result := s.RunCld(nil, "setup", "restore", "x")
	if want := "cld: setup restore: unexpected argument 'x' (see cld help)\n"; result.Code != 2 || result.Stderr != want {
		t.Errorf("an argument: exit %d, stderr %q, want exit 2, stderr %q", result.Code, result.Stderr, want)
	}
}

// On any system but Linux setup restore refuses to run, whatever it is given, before it looks
// for systemd; -h still shows its help.
func TestSetupRestoreLinuxOnly(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("the refusal is for systems other than Linux")
	}
	s := sandbox.New(t)
	for _, args := range [][]string{{}, {"x"}, {"--bogus"}} {
		result := s.RunCld(nil, append([]string{"setup", "restore"}, args...)...)
		if want := "cld: setup restore works on Linux only\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("%q: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", args, result.Code, result.Stdout, result.Stderr, want)
		}
	}
	if calls := s.SystemdCalls(); len(calls) != 0 {
		t.Errorf("systemd ran: %q", calls)
	}
	if result := s.RunCld(nil, "setup", "restore", "-h"); result.Code != 0 || !strings.HasPrefix(result.Stdout, "have your user's systemd") {
		t.Errorf("-h: exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr)
	}
}

// Completion runs none of the checks of setup restore, nor restore itself: whatever it completes
// after either, it runs no systemctl or loginctl - the fake ones, first on the PATH - and no tmux,
// offering nothing.
func TestCompletionRunsNoSystemd(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"__complete", "setup", "restore", ""},
		{"__complete", "setup", "restore", "-"},
		{"__complete", "restore", ""},
		{"__completeNoDesc", "restore", "-"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools("systemctl", "loginctl")}, args...)
			directive := result.Stdout == ":4\n" || strings.HasSuffix(result.Stdout, "\n:4\n")
			if stderr := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"; result.Code != 0 || !directive || result.Stderr != stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout ending in :4, stderr %q", result.Code, result.Stdout, result.Stderr, stderr)
			}
			if calls := s.SystemdCalls(); len(calls) != 0 {
				t.Errorf("systemd ran: %q", calls)
			}
		})
	}
}
