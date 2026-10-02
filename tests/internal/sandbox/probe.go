package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Record is what the probe writes about a process it stands in for.
type Record struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

// Probe is one run of the probe as claude, seen through the files it writes.
type Probe struct {
	Record
	t    testing.TB
	PID  int
	base string
}

// Probes lists the probes started so far, oldest first.
func (s *Sandbox) Probes() []*Probe {
	s.t.Helper()
	var probes []*Probe
	for _, path := range s.glob(filepath.Join(s.ProbeDir, "*.json")) {
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

// Input is everything the probe has read from its terminal so far.
func (p *Probe) Input() []byte {
	p.t.Helper()
	data, err := os.ReadFile(p.base + ".in")
	if err != nil {
		p.t.Fatal(err)
	}
	return data
}

// Mark is the position the next input will be read at, for WaitInput.
func (p *Probe) Mark() int {
	p.t.Helper()
	return len(p.Input())
}

// WaitInput waits until the probe has read want at or after mark.
func (p *Probe) WaitInput(mark int, want string) {
	p.t.Helper()
	WaitFor(p.t, 10*time.Second, strconv.Quote(want)+" in the probe input", func() bool {
		return bytes.Contains(p.Input()[mark:], []byte(want))
	})
}

// Send hands the probe a command (see controls in tests/probe/control.go).
func (p *Probe) Send(command string) {
	p.t.Helper()
	fifo, err := os.OpenFile(p.base+".ctl", os.O_WRONLY, 0)
	if err != nil {
		p.t.Fatal(err)
	}
	_, err = fifo.WriteString(command + "\n")
	if err := errors.Join(err, fifo.Close()); err != nil {
		p.t.Fatal(err)
	}
}

// Hook runs the hooks claude's settings give event, as claude would, with input - JSON, {} where
// empty - as their input (see tests/probe/hooks.go), and waits until they have run. A hook that
// fails, or prints anything, fails the test.
func (p *Probe) Hook(event, input string) {
	p.t.Helper()
	before := len(p.hooksRun())
	p.Send(strings.TrimSuffix("hook "+event+" "+input, " "))
	var lines []string
	WaitFor(p.t, 10*time.Second, "the "+event+" hooks to run", func() bool {
		lines = p.hooksRun()
		return len(lines) > before
	})
	if last := lines[len(lines)-1]; last != event {
		p.t.Errorf("hook %s: %s", event, last)
	}
}

// hooksRun is the lines the probe has ended in PID.hooks, which it creates at the first hook:
// what follows the last newline is being written.
func (p *Probe) hooksRun() []string {
	p.t.Helper()
	data, err := os.ReadFile(p.base + ".hooks")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		p.t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	return lines[:len(lines)-1]
}

// Alive reports whether the probe process is still running.
func (p *Probe) Alive() bool {
	return syscall.Kill(p.PID, 0) == nil
}

// readJSON decodes the JSON file at path into value, or fails the test.
func readJSON(tb testing.TB, path string, value any) {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		tb.Fatalf("%s: %v", path, err)
	}
}
