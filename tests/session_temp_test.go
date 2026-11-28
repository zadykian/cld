package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// claude's temp directories of its conversations, in the sandbox's temp root, and their removal
// as a session ends (decision 67.2).

// claudeTemp is claude's temp directory of the conversation id in project, in s's temp root,
// made with a file in it.
func claudeTemp(t *testing.T, s *sandbox.Sandbox, project, id string) string {
	t.Helper()
	dir := filepath.Join(s.ClaudeTemp(), "claude-"+strconv.Itoa(os.Getuid()), project, id)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(filepath.Join(dir, "tasks", "a.output"), "output\n")
	return dir
}

// Each end of a session removes claude's temp directory of its conversation, in each project, and
// no other. The ends are cld's kill, claude's exit with status 0, and the sweep of idle sessions.
func TestEndRemovesClaudeTemp(t *testing.T) {
	t.Parallel()
	for name, end := range map[string]func(*testing.T, *sandbox.Sandbox, terminal.Terminal,
		*sandbox.Probe){
		"kill": func(t *testing.T, s *sandbox.Sandbox, _ terminal.Terminal, _ *sandbox.Probe) {
			t.Helper()
			checkExit(t, "kill", s.RunCld(nil, "kill", "-s", "x"), 0, "", "")
		},
		"exit": func(_ *testing.T, _ *sandbox.Sandbox, _ terminal.Terminal, probe *sandbox.Probe) {
			probe.Send("exit")
		},
		"idle": func(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, _ *sandbox.Probe) {
			t.Helper()
			term.Keys("C-q", "d")
			sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool {
				return !term.Running()
			})
			// CLD_IDLE_DAYS 0.00001 is 0.864 s.
			time.Sleep(2 * time.Second)
			s.RunCld(map[string]string{"CLD_IDLE_DAYS": "0.00001"}, "list")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			term := startCld(t, s, "tmux", nil, "join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
			mine := []string{claudeTemp(t, s, "-a", firstID), claudeTemp(t, s, "-b", firstID)}
			other := claudeTemp(t, s, "-a", secondID)
			end(t, s, term, probe)
			sandbox.WaitFor(t, 10*time.Second, "session x to end", func() bool {
				return len(s.Sessions()) == 0
			})
			for _, dir := range mine {
				if exists(dir) {
					t.Errorf("%s: claude's temp directory %s is left", name, dir)
				}
			}
			if !exists(filepath.Join(other, "tasks", "a.output")) {
				t.Errorf("%s: another conversation's temp directory %s is gone", name, other)
			}
		})
	}
}

// kill ends a session whose server outlived it, and removes claude's temp directory of its
// conversation, which claude may have made since its pane went (decision 67.2). claude's pane goes
// by kill-pane, as the hook for exit status 0 ends it, so the end's own removal is the one to run.
func TestOutlivedKillRemovesClaudeTemp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "x")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-x/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	// The directory is made once the pane's own removal has run.
	dir := claudeTemp(t, s, "-a", firstID)
	checkExit(t, "kill", s.RunCld(nil, "kill", "-s", "x"), 0, "", "")
	if exists(dir) {
		t.Errorf("claude's temp directory %s is left by the kill of the server that outlived x", dir)
	}
}

// The temp root is CLAUDE_CODE_TMPDIR, else TMPDIR (decision 67.1), where an empty variable counts
// as unset. A session that has no conversation yet, with no SessionStart, removes nothing at its
// end, whatever directories are there (decision 67.2).
func TestEndTempRoot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		env   map[string]string
		start bool
		gone  bool
	}{
		{"empty CLAUDE_CODE_TMPDIR", map[string]string{"CLAUDE_CODE_TMPDIR": ""}, true, true},
		{"no conversation", nil, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			startCld(t, s, "tmux", test.env, "join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			if test.start {
				probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
			}
			dirs := []string{claudeTemp(t, s, "-a", firstID), claudeTemp(t, s, "-a", secondID)}
			checkExit(t, "kill", s.RunCld(test.env, "kill", "-s", "x"), 0, "", "")
			sandbox.WaitFor(t, 10*time.Second, "session x to end", func() bool {
				return len(s.Sessions()) == 0
			})
			if exists(dirs[0]) == test.gone {
				t.Errorf("claude's temp directory %s removed: %v, want %v", dirs[0], !exists(dirs[0]),
					test.gone)
			}
			if !exists(dirs[1]) {
				t.Errorf("another conversation's temp directory %s is gone", dirs[1])
			}
		})
	}
}

// More conversations' IDs, for the sweep's directories.
const (
	thirdID  = "3c2b1a09-8f7e-4d6c-b5a4-938271605f4e"
	fourthID = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
)

// transcript writes conversation id's transcript in project, in s's claude directory, as old as
// ago.
func transcript(t *testing.T, s *sandbox.Sandbox, project, id string, ago time.Duration) {
	t.Helper()
	file := filepath.Join(s.Home, ".claude", "projects", project, id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(file, "{}\n")
	age(t, ago, file)
}

// list and join without -s remove claude's temp directories, older than a day, that no process
// holds, whose transcript is older too or gone (decision 67.3). On a Mac they remove none
// (decision 67.4).
func TestSweepRemovesClaudeTemp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	old := 48 * time.Hour
	orphan, bare := claudeTemp(t, s, "-a", firstID), claudeTemp(t, s, "-b", fourthID)
	young, held := claudeTemp(t, s, "-a", secondID), claudeTemp(t, s, "-b", thirdID)
	transcript(t, s, "-a", firstID, old)
	transcript(t, s, "-a", secondID, 0)
	age(t, old, orphan, bare, young, held)
	file, err := os.Open(filepath.Join(held, "tasks", "a.output"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close() //nolint:errcheck // a file only read
	checkExit(t, "list", s.RunCld(nil, "list"), 0, "", "")
	swept := runtime.GOOS == "linux"
	checkSwept(t, "list", map[string]bool{orphan: swept, bare: swept, young: false, held: false})

	again := claudeTemp(t, s, "-c", firstID)
	age(t, old, again)
	startCld(t, s, "tmux", nil, "join")
	s.WaitProbes(1)
	checkSwept(t, "join", map[string]bool{again: swept, held: false})
}

// checkSwept reports each directory of dirs that the sweep, what, did not remove as dirs has it:
// true for removed.
func checkSwept(t *testing.T, what string, dirs map[string]bool) {
	t.Helper()
	for dir, removed := range dirs {
		if exists(dir) == removed {
			t.Errorf("%s: claude's temp directory %s removed: %v, want %v", what, dir, !removed,
				removed)
		}
	}
}

// The sweep leaves alone a temp root that is a symbolic link (decision 67.3). Another user can
// plant one in a sticky directory before claude makes the root, and claude refuses it too.
func TestSweepSkipsALinkedRoot(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	real := filepath.Join(s.Root, "real")
	dir := filepath.Join(real, "-a", firstID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, 48*time.Hour, dir)
	link := filepath.Join(s.ClaudeTemp(), "claude-"+strconv.Itoa(os.Getuid()))
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	checkExit(t, "list", s.RunCld(nil, "list"), 0, "", "")
	if !exists(dir) {
		t.Errorf("the sweep followed the link %s and removed %s", link, dir)
	}
}

// A directory that the sweep cannot remove is a warning, and list exits with status 0, as join
// does, which runs the same sweep (decision 67.3). Here nobody owns the root and the directory,
// but not the project's directory above it, so the removal fails.
func TestSweepWarnsOfAFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("needs the sweep, which a Mac has not, and root, to run as nobody")
	}
	s := sandbox.New(t)
	const nobody = 65534
	root := filepath.Join(s.ClaudeTemp(), "claude-"+strconv.Itoa(nobody))
	project := filepath.Join(root, "-a")
	dir := filepath.Join(project, firstID)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, 48*time.Hour, dir)
	err := filepath.WalkDir(s.Root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, nobody, nobody)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(project, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(project, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(s.Root) }) //nolint:errcheck // best effort, as the sandbox's
	cmd := exec.Command(sandbox.Cld, "list")
	cmd.Dir = s.Work
	cmd.Env = s.Environ(fakeTmuxEnv(s, nil))
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: nobody, Gid: nobody}}
	result := runCommand(t, cmd)
	want := "cld: warning: cannot remove claude's temp directory " + dir + ": "
	if result.Code != 0 || !strings.HasPrefix(result.Stderr, want) {
		t.Errorf("list: exit %d, stderr %q, want exit 0 and a warning starting %q", result.Code,
			result.Stderr, want)
	}
}
