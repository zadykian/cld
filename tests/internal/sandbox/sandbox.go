// Package sandbox gives each test an isolated world: its own tmux socket directory, HOME and
// probe records, so tests run in parallel and never touch the user's own cld sessions. cld runs
// each session on a server of its own, named like it: session cld-NAME on server cld-NAME.
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
	// Cld is the cld binary under test; built by TestMain.
	Cld string
	// ProbeBin is the directory holding the probe installed as "claude", and as "docker", the
	// fake docker (see DockerCalls); set by TestMain.
	ProbeBin string
	// FakeTmux is the probe installed as "tmux"; set by TestMain.
	FakeTmux string
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
		sockets, _ := os.ReadDir(s.SocketDir())
		for _, socket := range sockets {
			cmd := exec.Command("tmux", "-S", filepath.Join(s.SocketDir(), socket.Name()), "kill-server")
			cmd.Env = s.Environ(nil)
			_ = cmd.Run()
		}
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
	return s.RunCldIn(s.Work, extra, args...)
}

// RunCldIn runs cld as RunCld does, in the directory dir.
func (s *Sandbox) RunCldIn(dir string, extra map[string]string, args ...string) Result {
	s.t.Helper()
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = s.Environ(extra)
	cmd.Dir = dir
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
// "tmux" is the fake tmux, "claude" the probe and "docker" the fake docker; any other name links
// the real tool.
func (s *Sandbox) Tools(names ...string) string {
	s.t.Helper()
	dir, err := os.MkdirTemp(s.Root, "tools.")
	if err != nil {
		s.t.Fatal(err)
	}
	for _, name := range names {
		target := FakeTmux
		switch name {
		case "claude", "docker":
			target = filepath.Join(ProbeBin, name)
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

// SocketDir is the directory tmux keeps the sandbox's sockets in.
func (s *Sandbox) SocketDir() string {
	return filepath.Join(s.Root, "tmux-"+strconv.Itoa(os.Getuid()))
}

// Tmux runs a command against the sandbox's tmux server named server (tmux -L server): cld's
// session cld-NAME is on server cld-NAME.
func (s *Sandbox) Tmux(server string, args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-L", server}, args...)...)
	cmd.Env = s.Environ(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("tmux -L " + server + " " + strings.Join(args, " ") + ": " + strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// MustTmux is Tmux that fails the test on error.
func (s *Sandbox) MustTmux(server string, args ...string) string {
	s.t.Helper()
	out, err := s.Tmux(server, args...)
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

// Format expands a tmux format for a session's pane (cld sessions have one), on the server named
// like the session. "display -p -t =SESSION" would be shorter, but tmux 3.3 expands it to nothing
// without a client.
func (s *Sandbox) Format(session, format string) string {
	s.t.Helper()
	return s.MustTmux(session, "list-panes", "-s", "-t", "="+session, "-F", format)
}

// Servers lists the names of the sandbox's sockets that cld's servers have - cld-*, whether a
// server still runs on them or not - in order.
func (s *Sandbox) Servers() []string {
	paths, _ := filepath.Glob(filepath.Join(s.SocketDir(), "cld-*"))
	servers := make([]string, 0, len(paths))
	for _, path := range paths {
		servers = append(servers, filepath.Base(path))
	}
	return servers
}

// Sessions lists the sessions on the servers of Servers, in order: a session on the server named
// like it as its name, cld-NAME, and any other as SERVER/SESSION - one that claude made on its
// server, say. Nothing where no server runs.
func (s *Sandbox) Sessions() []string {
	var sessions []string
	for _, server := range s.Servers() {
		out, err := s.Tmux(server, "list-sessions", "-F", "#{session_name}")
		if err != nil {
			continue
		}
		for _, session := range strings.Fields(out) {
			if session != server {
				session = server + "/" + session
			}
			sessions = append(sessions, session)
		}
	}
	slices.Sort(sessions)
	return sessions
}

// Clients lists the clients attached to the servers of Servers, as the sessions they are
// attached to, in order.
func (s *Sandbox) Clients() []string {
	var clients []string
	for _, server := range s.Servers() {
		if out, err := s.Tmux(server, "list-clients", "-F", "#{session_name}"); err == nil {
			clients = append(clients, strings.Fields(out)...)
		}
	}
	slices.Sort(clients)
	return clients
}

// WriteFile writes a file or fails the test.
func (s *Sandbox) WriteFile(path, content string) {
	s.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// WriteProgram writes a program for cld to run, with the permissions mode, or fails the test. It
// holds syscall.ForkLock for reading as it does, as creating a file descriptor should: a process
// that another test forks meanwhile would inherit the file open for writing until it runs its own
// program, and running this one would fail then with ETXTBSY, "text file busy" (golang/go#22315).
func (s *Sandbox) WriteProgram(path, content string, mode os.FileMode) {
	s.t.Helper()
	syscall.ForkLock.RLock()
	err := os.WriteFile(path, []byte(content), mode)
	syscall.ForkLock.RUnlock()
	if err != nil {
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

// DockerCall is a call of the fake docker, as it records it.
type DockerCall struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
	// PortFree is, for run -d, whether the port of its label cld.port was free on 127.0.0.1.
	PortFree *bool `json:"portFree,omitempty"`
}

// DockerCalls are the calls of the fake docker so far, oldest first.
func (s *Sandbox) DockerCalls() []DockerCall {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.ProbeDir, "docker.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	var calls []DockerCall
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var call DockerCall
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			s.t.Fatalf("docker.jsonl: %v", err)
		}
		calls = append(calls, call)
	}
	return calls
}

// DockerContainer is the fake docker's container, as a call left it: "STATUS PORT [RESTARTS]",
// or "" for none. It is "untouched" while no call has changed the one the test gave the fake.
func (s *Sandbox) DockerContainer() string {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.ProbeDir, "docker.container"))
	if errors.Is(err, os.ErrNotExist) {
		return "untouched"
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return string(data)
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
