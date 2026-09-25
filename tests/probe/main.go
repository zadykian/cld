// Command probe stands in for the programs cld starts, in cld's tests.
//
// Invoked as "claude", it treats the terminal the way claude does - alternate screen, SGR
// all-motion mouse, bracketed paste, focus reports, modifyOtherKeys, its own window title - and
// records what cld and the terminal hand it. Its files in $CLD_PROBE_DIR are named after its pid:
//
//	PID.json  argv, working directory and environment, written once at start
//	PID.in    every input byte, appended as it arrives
//	PID.ctl   FIFO of commands, one per line:
//	            title TEXT        set the window title
//	            osc52 TEXT        copy TEXT to the clipboard (OSC 52 in tmux passthrough)
//	            loadbuffer TEXT   copy TEXT the way claude does inside tmux: tmux load-buffer -w
//	            rekey             leave and re-enter the alternate screen, push the keyboard
//	                              modes again and repaint, as claude does after an external
//	                              editor; the repaint ends in a line "repainted"
//	            inline            leave the alternate screen and turn mouse reporting off, as
//	                              claude outside fullscreen draws
//	            cd DIR            change to DIR, as claude does entering a worktree
//	            tmux ARGS         run tmux with ARGS, split at spaces, as anything claude runs
//	                              may: TMUX takes it to the server of claude's pane
//	            exit [N]          exit at once with status N (default 0), leaving the terminal
//	                              modes on
//
// With $CLD_PROBE_FAIL set, it prints that and exits with status 1 at once, as claude does when
// it cannot start.
//
// "claude --version" answers first, before $CLD_PROBE_FAIL and without writing any file, so that
// the version check cld makes before starting claude neither fails nor counts as a claude: it
// prints $CLD_FAKE_CLAUDE_VERSION, or "99.0.0 (Claude Code)" where that is unset, and exits 0.
// In a directory that has been removed it fails, as claude 2.1.282 does.
//
// Invoked as "tmux", it fakes tmux for the checks cld makes before starting it: "tmux -V" prints
// $CLD_FAKE_TMUX_VERSION, list-sessions prints $CLD_FAKE_TMUX_SESSIONS, whatever server it is
// asked - where a test sets none it fails as tmux does with no server running, and on a server
// named in $CLD_FAKE_TMUX_EXITED (-L NAME, separated by spaces) as tmux does when the server exits
// while it asks - and any other invocation is recorded in $CLD_PROBE_DIR/tmux.json. With
// $CLD_FAKE_TMUX_REAL, the path of a real tmux, it fakes list-sessions only, and runs that tmux
// for the rest.
//
// Invoked as "docker", it fakes the docker calls of cld setup telemetry and appends each, with
// its environment, to $CLD_PROBE_DIR/docker.jsonl, a line of JSON per call. It keeps the one
// container, cld-telemetry, in $CLD_PROBE_DIR/docker.container ("STATUS PORT [RESTARTS]", empty
// for none), which starts as $CLD_FAKE_DOCKER_CONTAINER - none unless a test sets it:
//
//	container inspect  prints "STATUS RESTARTS PORT", RESTARTS 0 unless given, or that there is
//	                   no such container, exit 1
//	rm -f              removes it, or says there is none, exit 0 (as Docker 29.6.0)
//	run -d             starts it, with the port of its label cld.port, as
//	                   $CLD_FAKE_DOCKER_STARTED, "STATUS [RESTARTS]" (default "running"); records
//	                   whether that port is free on 127.0.0.1, as the collector needs it. A
//	                   container started running takes connections on that port, as the
//	                   collector's receiver, until rm -f or the end of the test (see receiver) -
//	                   unless $CLD_FAKE_DOCKER_LISTEN is "no", or something else has the port. With
//	                   $CLD_FAKE_DOCKER_WRITE, "FILE" and a newline, then the rest, it writes the
//	                   rest to FILE, as another program may while the collector starts
//	logs               prints $CLD_FAKE_DOCKER_LOGS on stderr, by default the collector's line
//	                   saying it is ready
//	update             takes the container's restart policy: prints its name, as Docker does
//	run --rm           the collector's validate: succeeds, printing nothing
//
// $CLD_FAKE_DOCKER_FAIL=WORD[=STATUS] makes any call with the argument WORD fail instead, with
// status STATUS (default 1), saying so.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Record is what the probe writes to PID.json and tmux.json.
type Record struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

const modes = "\x1b[?1049h" + // alternate screen
	"\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h" + // SGR all-motion mouse
	"\x1b[?2004h" + // bracketed paste
	"\x1b[?1004h" + // focus reports
	"\x1b[>4;1m" // modifyOtherKeys mode 1

func main() {
	var err error
	switch filepath.Base(os.Args[0]) {
	case "tmux":
		err = fakeTmux()
	case "docker":
		err = fakeDocker()
	case "receiver":
		err = receiver()
	default:
		err = claude()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

func fakeTmux() error {
	real := os.Getenv("CLD_FAKE_TMUX_REAL")
	if len(os.Args) == 2 && os.Args[1] == "-V" && real == "" {
		fmt.Println(os.Getenv("CLD_FAKE_TMUX_VERSION"))
		return nil
	}
	if slices.Contains(os.Args[1:], "list-sessions") {
		if i := slices.Index(os.Args, "-L"); i > 0 && i+1 < len(os.Args) &&
			slices.Contains(strings.Fields(os.Getenv("CLD_FAKE_TMUX_EXITED")), os.Args[i+1]) {
			fmt.Fprintln(os.Stderr, "server exited unexpectedly")
			os.Exit(1)
		}
		sessions := os.Getenv("CLD_FAKE_TMUX_SESSIONS")
		if sessions == "" {
			fmt.Fprintln(os.Stderr, "no server running on /fake/tmux")
			os.Exit(1)
		}
		fmt.Println(sessions)
		return nil
	}
	if real != "" {
		return syscall.Exec(real, append([]string{"tmux"}, os.Args[1:]...), os.Environ())
	}
	return writeRecord(filepath.Join(os.Getenv("CLD_PROBE_DIR"), "tmux.json"))
}

// DockerCall is a line of docker.jsonl: a call of the fake docker.
type DockerCall struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
	// PortFree is, for run -d, whether the port of its label was free.
	PortFree *bool `json:"portFree,omitempty"`
}

// readyLog is what the collector 0.161.0 logs once it runs, less the fields after it.
const readyLog = "2026-09-25T14:52:07.911Z\tinfo\tservice@v0.161.0/service.go:256\tEverything is ready. Begin running and processing data.\n"

func fakeDocker() error {
	dir := os.Getenv("CLD_PROBE_DIR")
	args := os.Args[1:]
	call := DockerCall{Argv: args, Env: map[string]string{}}
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		call.Env[name] = value
	}
	statePath := filepath.Join(dir, "docker.container")
	state, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		state, err = []byte(os.Getenv("CLD_FAKE_DOCKER_CONTAINER")), nil
	}
	if err != nil {
		return err
	}
	status, port, restarts := "", "", "0"
	if fields := strings.Fields(string(state)); len(fields) > 0 {
		status = fields[0]
		if len(fields) > 1 {
			port = fields[1]
		}
		if len(fields) > 2 {
			restarts = fields[2]
		}
	}
	name := ""
	if len(args) > 0 {
		name = args[len(args)-1]
	}
	var out func() error
	fail, failStatus, _ := strings.Cut(os.Getenv("CLD_FAKE_DOCKER_FAIL"), "=")
	switch {
	case len(args) == 0:
		out = func() error { return nil }
	case fail != "" && slices.Contains(args, fail):
		out = func() error {
			fmt.Fprintf(os.Stderr, "fake docker: %s failed\n", strings.Join(args, " "))
			code, err := strconv.Atoi(failStatus)
			if err != nil {
				code = 1
			}
			os.Exit(code)
			return nil
		}
	case len(args) > 1 && args[0] == "container" && args[1] == "inspect":
		out = func() error {
			if status == "" {
				fmt.Fprintf(os.Stderr, "Error response from daemon: No such container: %s\n", name)
				os.Exit(1)
			}
			fmt.Printf("%s %s %s\n", status, restarts, port)
			return nil
		}
	case args[0] == "rm":
		out = func() error {
			if status == "" {
				fmt.Fprintf(os.Stderr, "Error response from daemon: No such container: %s\n", name)
				return nil
			}
			fmt.Println(name)
			if err := stopReceiver(dir); err != nil {
				return err
			}
			return os.WriteFile(statePath, nil, 0o644)
		}
	case args[0] == "run" && slices.Contains(args, "-d"):
		for i, arg := range args {
			if arg == "--label" && i+1 < len(args) {
				port, _ = strings.CutPrefix(args[i+1], "cld.port=")
			}
		}
		listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
		free := err == nil
		if free {
			_ = listener.Close()
		}
		call.PortFree = &free
		out = func() error {
			fmt.Println("0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0")
			if write, set := os.LookupEnv("CLD_FAKE_DOCKER_WRITE"); set {
				file, content, _ := strings.Cut(write, "\n")
				if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
					return err
				}
			}
			started := strings.Fields(os.Getenv("CLD_FAKE_DOCKER_STARTED"))
			if len(started) == 0 {
				started = []string{"running"}
			}
			if err := os.WriteFile(statePath, []byte(strings.Join(slices.Insert(started, 1, port), " ")), 0o644); err != nil {
				return err
			}
			if started[0] != "running" || !free || os.Getenv("CLD_FAKE_DOCKER_LISTEN") == "no" {
				return nil
			}
			return startReceiver(dir, port)
		}
	case args[0] == "update":
		out = func() error {
			if status == "" {
				fmt.Fprintf(os.Stderr, "Error response from daemon: No such container: %s\n", name)
				os.Exit(1)
			}
			fmt.Println(name)
			return nil
		}
	case args[0] == "logs":
		out = func() error {
			logs, set := os.LookupEnv("CLD_FAKE_DOCKER_LOGS")
			if !set {
				logs = readyLog
			}
			fmt.Fprint(os.Stderr, logs)
			return nil
		}
	default:
		out = func() error { return nil }
	}
	data, err := json.Marshal(call)
	if err != nil {
		return err
	}
	calls, err := os.OpenFile(filepath.Join(dir, "docker.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := calls.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := calls.Close(); err != nil {
		return err
	}
	return out()
}

// receiverFile is the file in $CLD_PROBE_DIR that holds the port of a receiver the fake docker
// started, while it runs (see receiver).
const receiverFile = "docker.receiver"

// startReceiver starts the probe as the receiver of the collector on 127.0.0.1:port (see
// receiver), and returns once the receiver listens there, or has found the port taken.
func startReceiver(dir, port string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	started, done, err := os.Pipe()
	if err != nil {
		return err
	}
	defer started.Close()
	cmd := exec.Command(executable, port, filepath.Join(dir, receiverFile))
	cmd.Args[0] = "receiver"
	cmd.ExtraFiles = []*os.File{done}
	// A session of its own, with none of the pipes cld reads docker's output from: cld, and the
	// test, go on without it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = cmd.Start()
	_ = done.Close()
	if err != nil {
		return err
	}
	_ = cmd.Process.Release()
	_, err = io.ReadAll(started) // Up to the receiver's closing its end.
	return err
}

// receiver stands in for the receiver of a collector that the fake docker started: it listens on
// 127.0.0.1:PORT and takes connections, closing each, while the file FILE, which it writes with
// the port, exists - the fake's rm -f removes it, and the end of the test the whole sandbox. It
// closes descriptor 3 once it listens, or has found the port taken; a collector that cannot have
// the port does not get ready, and nor does this one.
func receiver() error {
	port, file := os.Args[1], os.Args[2]
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err == nil {
		err = os.WriteFile(file, []byte(port), 0o644)
	}
	_ = os.NewFile(3, "started").Close()
	if err != nil {
		return err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	// No longer than go test's own limit, should a test end without removing its sandbox.
	for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(file); err != nil {
			break
		}
	}
	return listener.Close()
}

// stopReceiver stops the receiver of the container, if one runs (see receiver), and returns once
// its port is free, as docker rm -f returns once the collector is gone.
func stopReceiver(dir string) error {
	file := filepath.Join(dir, receiverFile)
	port, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		err = os.Remove(file)
	}
	if err != nil {
		return err
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		listener, err := net.Listen("tcp4", "127.0.0.1:"+string(port))
		if err == nil {
			return listener.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the receiver on port %s did not stop: %v", port, err)
		}
	}
}

func claude() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		return version()
	}
	if message := os.Getenv("CLD_PROBE_FAIL"); message != "" {
		fmt.Println(message)
		os.Exit(1)
	}
	base := filepath.Join(os.Getenv("CLD_PROBE_DIR"), strconv.Itoa(os.Getpid()))
	if err := syscall.Mkfifo(base+".ctl", 0o600); err != nil {
		return err
	}
	// Opened read-write so that the FIFO never reports end of file between two writers.
	control, err := os.OpenFile(base+".ctl", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	input, err := os.OpenFile(base+".in", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	// Written last: once PID.json exists, the other files do too.
	if err := writeRecord(base + ".json"); err != nil {
		return err
	}
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		return err
	}

	var mu sync.Mutex
	write := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = os.Stdout.WriteString(s)
	}
	write(modes + "\x1b]0;probe\x07" + screen())
	go obey(control, write)

	buffer := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buffer)
		if n > 0 {
			if _, err := input.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			return nil // the terminal went away
		}
	}
}

// version answers claude --version.
func version() error {
	if _, err := os.Getwd(); err != nil {
		fmt.Fprintln(os.Stderr, "error: The current working directory was deleted, so that command didn't work. Please cd into a different directory and try again.")
		os.Exit(1)
	}
	reported, set := os.LookupEnv("CLD_FAKE_CLAUDE_VERSION")
	if !set {
		reported = "99.0.0 (Claude Code)"
	}
	_, err := fmt.Println(reported)
	return err
}

func obey(control *os.File, write func(string)) {
	lines := bufio.NewScanner(control)
	for lines.Scan() {
		command, argument, _ := strings.Cut(lines.Text(), " ")
		switch command {
		case "title":
			write("\x1b]0;" + argument + "\x07")
		case "osc52":
			write(passthrough("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(argument)) + "\x07"))
		case "loadbuffer":
			// tmux keeps the text in a buffer and hands it to the terminal as OSC 52.
			load := exec.Command("tmux", "load-buffer", "-w", "-")
			load.Stdin = strings.NewReader(argument)
			if err := load.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "probe: tmux load-buffer:", err)
			}
		case "rekey":
			// A screen model that took CSI > 4 ; 2 m for SGR 4;2 would draw the repaint underlined
			// and faint, and replay it that way on every reattach.
			write("\x1b[?1049l\x1b[?1049h\x1b[<u\x1b[>1u\x1b[>4;2m\x1b[H\x1b[2J" + screen() + "repainted\r\n")
		case "inline":
			write("\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?1006l\x1b[?1049l")
		case "cd":
			if err := os.Chdir(argument); err != nil {
				fmt.Fprintln(os.Stderr, "probe:", err)
			}
		case "tmux":
			if out, err := exec.Command("tmux", strings.Fields(argument)...).CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "probe: tmux: %v: %s", err, out)
			}
		case "exit":
			status, _ := strconv.Atoi(argument)
			os.Exit(status)
		}
	}
}

// screen is what the probe draws: its arguments, then a line in the attributes claude uses.
func screen() string {
	return "probe " + strings.Join(os.Args[1:], " ") + "\r\n" +
		"\x1b[1mbold\x1b[22m \x1b[2mdim\x1b[22m \x1b[3mitalic\x1b[23m \x1b[7minverse\x1b[27m " +
		"\x1b[38;5;208mcolour\x1b[39m\r\n"
}

// passthrough wraps a sequence so that tmux hands it to the outer terminal unchanged.
func passthrough(sequence string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b") + "\x1b\\"
}

func writeRecord(path string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	record := Record{Argv: os.Args[1:], Cwd: cwd, Env: map[string]string{}}
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		record.Env[name] = value
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	// Renamed into place, so a reader never sees a partial file.
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
