// Package sandbox gives each test an isolated world: its own tmux socket directory, HOME and
// probe records, so tests run in parallel and never touch the user's own cld sessions.
package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
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
)

var (
	// Repo is the root of the repository.
	Repo = func() string {
		_, file, _, _ := runtime.Caller(0)
		return filepath.Join(filepath.Dir(file), "..", "..", "..")
	}()
	// Cld is the script under test.
	Cld = filepath.Join(Repo, "bin", "cld")
	// ProbeBin is the directory holding the probe installed as "claude"; set by TestMain.
	ProbeBin string
	// FakeTmux is the probe installed as "tmux"; set by TestMain.
	FakeTmux string
	// Bash, when set through CLD_BASH, runs cld under that bash instead of the one on PATH,
	// e.g. macOS's /bin/bash 3.2.
	Bash = os.Getenv("CLD_BASH")
)

// Record is what the probe writes about a process it stands in for.
type Record struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

// Sandbox is the isolated world of one test.
type Sandbox struct {
	t        testing.TB
	Root     string
	Home     string
	Work     string
	ProbeDir string
	// Env is the environment cld runs in; terminals add their own variables on top.
	Env map[string]string
}

// New creates a sandbox that is torn down when the test ends.
func New(t testing.TB) *Sandbox {
	t.Helper()
	// tmux puts its sockets in $TMUX_TMPDIR and a socket path must fit in ~104 bytes, so the
	// root stays short instead of living under t.TempDir().
	root, err := os.MkdirTemp("/tmp", "cld.")
	if err != nil {
		t.Fatal(err)
	}
	if root, err = filepath.EvalSymlinks(root); err != nil { // macOS: /tmp -> /private/tmp
		t.Fatal(err)
	}
	s := &Sandbox{
		t:        t,
		Root:     root,
		Home:     filepath.Join(root, "home"),
		Work:     filepath.Join(root, "work"),
		ProbeDir: filepath.Join(root, "probe"),
	}
	for _, dir := range []string{s.Home, s.Work, s.ProbeDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// cld must not read the user's tmux configuration; this one would show if it did.
	s.WriteFile(filepath.Join(s.Home, ".tmux.conf"), "set -g prefix C-a\nset -g status-left POISONED\n")
	locale := "C.UTF-8"
	if runtime.GOOS == "darwin" {
		locale = "en_US.UTF-8"
	}
	s.Env = map[string]string{
		"PATH":          ProbeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME":          s.Home,
		"TMUX_TMPDIR":   root,
		"CLD_PROBE_DIR": s.ProbeDir,
		"TERM":          "xterm-256color",
		"LANG":          locale,
	}
	t.Cleanup(func() {
		_, _ = s.Tmux("kill-server")
		_ = os.RemoveAll(root)
	})
	return s
}

// Environ turns the sandbox environment plus extra variables into exec.Cmd.Env form.
func (s *Sandbox) Environ(extra map[string]string) []string {
	env := make([]string, 0, len(s.Env)+len(extra))
	for name, value := range s.Env {
		if _, overridden := extra[name]; !overridden {
			env = append(env, name+"="+value)
		}
	}
	for name, value := range extra {
		env = append(env, name+"="+value)
	}
	slices.Sort(env)
	return env
}

// CldArgv is the command line that runs cld with args.
func (s *Sandbox) CldArgv(args ...string) []string {
	if Bash != "" {
		return append([]string{Bash, Cld}, args...)
	}
	return append([]string{Cld}, args...)
}

// Result is the outcome of a command run without a terminal.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// RunCld runs cld without a terminal, which is enough for everything cld does before tmux.
// extra variables are added to the sandbox environment.
func (s *Sandbox) RunCld(extra map[string]string, args ...string) Result {
	s.t.Helper()
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = s.Environ(extra)
	cmd.Dir = s.Work
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		s.t.Fatalf("run cld: %v", err)
	}
	return Result{Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
}

// Tools creates a directory holding only the named tools, for a PATH that lacks the others.
// "tmux" is the fake tmux and "claude" the probe; any other name links the real tool.
func (s *Sandbox) Tools(names ...string) string {
	s.t.Helper()
	dir, err := os.MkdirTemp(s.Root, "tools.")
	if err != nil {
		s.t.Fatal(err)
	}
	for _, name := range names {
		target := FakeTmux
		switch name {
		case "claude":
			target = filepath.Join(ProbeBin, "claude")
		case "tmux":
		default:
			if target, err = exec.LookPath(name); err != nil {
				s.t.Fatal(err)
			}
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			s.t.Fatal(err)
		}
	}
	return dir
}

// Tmux runs a command against cld's private tmux server.
func (s *Sandbox) Tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-L", "cld"}, args...)...)
	cmd.Env = s.Environ(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("tmux " + strings.Join(args, " ") + ": " + strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// MustTmux is Tmux that fails the test on error.
func (s *Sandbox) MustTmux(args ...string) string {
	s.t.Helper()
	out, err := s.Tmux(args...)
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

// Format expands a tmux format for a session's pane (cld sessions have one).
// "display -p -t =SESSION" would be shorter, but tmux 3.3 expands it to nothing without a client.
func (s *Sandbox) Format(session, format string) string {
	s.t.Helper()
	return s.MustTmux("list-panes", "-s", "-t", "="+session, "-F", format)
}

// Sessions lists the sessions on cld's server, or nothing if it is not running.
func (s *Sandbox) Sessions() []string {
	out, err := s.Tmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	sessions := strings.Fields(out)
	slices.Sort(sessions)
	return sessions
}

// WriteFile writes a file or fails the test.
func (s *Sandbox) WriteFile(path, content string) {
	s.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// Probes lists the probes started so far, oldest first.
func (s *Sandbox) Probes() []*Probe {
	s.t.Helper()
	paths, _ := filepath.Glob(filepath.Join(s.ProbeDir, "*.json"))
	var probes []*Probe
	for _, path := range paths {
		pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(path), ".json"))
		if err != nil {
			continue // tmux.json
		}
		probe := &Probe{t: s.t, PID: pid, base: strings.TrimSuffix(path, ".json")}
		readJSON(s.t, path, &probe.Record)
		probes = append(probes, probe)
	}
	slices.SortFunc(probes, func(a, b *Probe) int { return a.PID - b.PID })
	return probes
}

// WaitProbes waits until count probes have started and returns them.
func (s *Sandbox) WaitProbes(count int) []*Probe {
	s.t.Helper()
	WaitFor(s.t, 15*time.Second, strconv.Itoa(count)+" probe(s) to start", func() bool {
		return len(s.Probes()) >= count
	})
	return s.Probes()
}

// FakeTmuxRecord is what the fake tmux recorded about its invocation.
func (s *Sandbox) FakeTmuxRecord() Record {
	s.t.Helper()
	var record Record
	readJSON(s.t, filepath.Join(s.ProbeDir, "tmux.json"), &record)
	return record
}

// Probe is one run of the probe as claude, seen through the files it writes.
type Probe struct {
	Record
	t    testing.TB
	PID  int
	base string
}

// Input is everything the probe has read from its terminal so far.
func (p *Probe) Input() []byte {
	data, _ := os.ReadFile(p.base + ".in")
	return data
}

// Mark is the position the next input will be read at, for WaitInput.
func (p *Probe) Mark() int {
	return len(p.Input())
}

// WaitInput waits until the probe has read want at or after mark.
func (p *Probe) WaitInput(mark int, want string) {
	p.t.Helper()
	WaitFor(p.t, 10*time.Second, strconv.Quote(want)+" in the probe input", func() bool {
		return bytes.Contains(p.Input()[mark:], []byte(want))
	})
}

// Send hands the probe a command (see tests/probe).
func (p *Probe) Send(command string) {
	p.t.Helper()
	fifo, err := os.OpenFile(p.base+".ctl", os.O_WRONLY, 0)
	if err != nil {
		p.t.Fatal(err)
	}
	defer fifo.Close()
	if _, err := fifo.WriteString(command + "\n"); err != nil {
		p.t.Fatal(err)
	}
}

// Alive reports whether the probe process is still running.
func (p *Probe) Alive() bool {
	return syscall.Kill(p.PID, 0) == nil
}

// WaitFor polls condition until it holds, failing the test after timeout.
func WaitFor(t testing.TB, timeout time.Duration, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func readJSON(t testing.TB, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
