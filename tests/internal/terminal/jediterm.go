package terminal

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// JediTermClasspath holds the compiled tests/jediterm driver and its libraries; set by TestMain.
var JediTermClasspath string

// jediTerm is JediTerm's emulator (jediterm-core) behind a pty, headless: the Java driver in
// tests/jediterm does the emulation and answers one command per line.
type jediTerm struct {
	unsupported
	sandbox *sandbox.Sandbox
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
}

func newJediTerm(t testing.TB, s *sandbox.Sandbox) *jediTerm {
	return &jediTerm{unsupported: unsupported{t: t, name: "jediterm"}, sandbox: s}
}

type jediTermReply struct {
	OK          bool            `json:"ok"`
	Value       json.RawMessage `json:"value"`
	Unsupported bool            `json:"unsupported"`
	Error       string          `json:"error"`
}

func (j *jediTerm) call(fields ...string) json.RawMessage {
	j.t.Helper()
	if _, err := io.WriteString(j.stdin, strings.Join(fields, "\t")+"\n"); err != nil {
		j.t.Fatalf("jediterm driver: %v (log: %s)", err, j.log())
	}
	line, err := j.stdout.ReadBytes('\n')
	if err != nil {
		j.t.Fatalf("jediterm driver: %v (log: %s)", err, j.log())
	}
	var reply jediTermReply
	if err := json.Unmarshal(line, &reply); err != nil {
		j.t.Fatalf("jediterm driver: %v: %q", err, line)
	}
	if reply.Unsupported {
		j.t.Skip("jediterm: " + reply.Error)
	}
	if !reply.OK {
		j.t.Fatalf("jediterm driver %s: %s", fields[0], reply.Error)
	}
	return reply.Value
}

func (j *jediTerm) log() string {
	data, _ := os.ReadFile(filepath.Join(j.sandbox.Root, "jediterm.log"))
	return strings.TrimSpace(string(data))
}

func (j *jediTerm) Start(argv []string, env map[string]string, dir string) {
	j.t.Helper()
	if JediTermClasspath == "" {
		j.t.Fatal("jediterm: the driver is not compiled (run with CLD_TERMINALS including jediterm)")
	}
	// What the JetBrains IDEs add to the environment of their terminals.
	extra := map[string]string{"TERMINAL_EMULATOR": "JetBrains-JediTerm"}
	for name, value := range env {
		extra[name] = value
	}
	logFile, err := os.Create(filepath.Join(j.sandbox.Root, "jediterm.log"))
	if err != nil {
		j.t.Fatal(err)
	}
	defer logFile.Close()
	j.cmd = exec.Command("java", "-cp", JediTermClasspath, "JediTermDriver")
	j.cmd.Env = j.sandbox.Environ(extra)
	j.cmd.Stderr = logFile
	if j.stdin, err = j.cmd.StdinPipe(); err != nil {
		j.t.Fatal(err)
	}
	stdout, err := j.cmd.StdoutPipe()
	if err != nil {
		j.t.Fatal(err)
	}
	j.stdout = bufio.NewReader(stdout)
	if err := j.cmd.Start(); err != nil {
		j.t.Fatal(err)
	}
	// Shift+Enter sends ESC CR, as with the IDE setting that makes Shift+Enter a newline.
	j.call(append([]string{"start", dir, "1"}, argv...)...)
}

func (j *jediTerm) Keys(keys ...string) {
	j.t.Helper()
	for _, key := range keys {
		j.call("keys", key)
		time.Sleep(50 * time.Millisecond)
	}
}

func (j *jediTerm) Paste(text string) {
	j.t.Helper()
	j.call("paste", base64.StdEncoding.EncodeToString([]byte(text)))
}

func (j *jediTerm) WheelUp() {
	j.t.Helper()
	j.call("wheel-up")
}

func (j *jediTerm) Focus(focused bool) {
	j.t.Helper()
	j.call("focus")
}

func (j *jediTerm) Title() string {
	j.t.Helper()
	return j.text(j.call("title"))
}

func (j *jediTerm) Screen() string {
	j.t.Helper()
	return j.text(j.call("screen"))
}

func (j *jediTerm) Output() []byte {
	j.t.Helper()
	return []byte(j.text(j.call("output")))
}

func (j *jediTerm) Modes() Modes {
	j.t.Helper()
	var modes Modes
	if err := json.Unmarshal(j.call("modes"), &modes); err != nil {
		j.t.Fatal(err)
	}
	return modes
}

func (j *jediTerm) Clipboard() string {
	j.t.Helper()
	return j.text(j.call("clipboard"))
}

func (j *jediTerm) Running() bool {
	j.t.Helper()
	return string(j.call("running")) == "true"
}

func (j *jediTerm) text(value json.RawMessage) string {
	j.t.Helper()
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		j.t.Fatal(err)
	}
	return text
}

func (j *jediTerm) Close() {
	if j.cmd == nil || j.cmd.Process == nil {
		return
	}
	_ = j.stdin.Close() // the driver kills its pty and exits at end of input
	done := make(chan struct{})
	go func() {
		_ = j.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = j.cmd.Process.Kill()
		<-done
	}
}
