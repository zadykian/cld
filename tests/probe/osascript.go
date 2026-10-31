package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
)

// scriptError is the error document of cld's scripts (#176).
type scriptError struct {
	Step    string `json:"step"`
	Number  int    `json:"number"`
	Message string `json:"message"`
	Version string `json:"version"`
}

// snapshotFailures are the failure modes a snapshot answers with an error document: those that
// snapshot.js prints, as Ghostty's failures give them.
var snapshotFailures = map[string]scriptError{
	"denied": {"count", -1743, "Not authorized to send Apple events to Ghostty.", ""},
	"off": {"probe", -1743,
		"AppleScript is disabled by the macos-applescript configuration.", ""},
	"old":     {"tty", 0, "TypeError: G.terminals.tty is not a function", "1.3.1"},
	"changed": {"changed", 0, "the windows changed while cld read them", "1.4.0"},
	"gone":    {"read", -600, "Application isn't running.", "1.4.0"},
}

// failureModes are the failure modes, "" answering as Ghostty would.
var failureModes = []string{
	"", "denied", "off", "old", "changed", "gone", "garbage", "partial", "short", "timeout",
}

// fakeOsascript fakes osascript running cld's scripts against the model Ghostty (internal/mac).
// The script's first line, its marker, picks the answer: snapshot or apply. It logs each call to
// osascript.jsonl, then fails as failMode says. But for garbage, a mode fails only a Ghostty that
// runs: where none does, both scripts answer before any Apple Event.
func fakeOsascript() error {
	script, input, p := readCall(os.Args[1:])
	dir := os.Getenv("CLD_PROBE_DIR")
	call := mac.OsascriptCall{Script: script, Input: json.RawMessage(input), Env: environment()}
	if err := appendJSON(filepath.Join(dir, mac.OsascriptLog), call); err != nil {
		return err
	}
	mode, err := failMode(dir)
	if err != nil {
		return err
	}
	m, err := mac.Load(dir)
	if err != nil {
		return err
	}
	switch {
	case mode == "garbage":
		garbage("")
	case !m.Running && script == "apply":
		return printJSON(failed(scriptError{"running", 0, "Ghostty is not running", ""}))
	case !m.Running:
		_, err := fmt.Println(`{"running":false,"apps":[]}`)
		return err
	case mode == "timeout":
		stall()
	case script == "apply":
		return apply(dir, m, p, mode)
	}
	return snapshot(dir, m, mode)
}

// readCall reads the words cld runs osascript with, a script and its JSON input, and the plan an
// apply's input holds. It refuses anything else, logging no call.
func readCall(args []string) (script, input string, p *plan) {
	if len(args) != 5 || args[0] != "-l" || args[1] != "JavaScript" || args[2] != "-e" {
		refuse("unexpected arguments")
	}
	marker, _, _ := strings.Cut(args[3], "\n")
	script, input = strings.TrimPrefix(marker, "// cld: "), args[4]
	if script != "snapshot" && script != "apply" {
		refuse("unexpected script " + strconv.Quote(marker))
	}
	if !json.Valid([]byte(input)) {
		refuse("the input is no JSON")
	}
	p = &plan{}
	if script == "apply" && json.Unmarshal([]byte(input), p) != nil {
		refuse("the input is no plan")
	}
	return script, input, p
}

// failMode is the first line of osascript-fail where that exists, else $CLD_FAKE_OSASCRIPT_FAIL:
// denied, off, old, changed, gone, garbage, partial, short or timeout; "" answers. changed fails
// only a snapshot, and partial and short only an apply. old refuses an apply, which cld never runs
// on Ghostty 1.3.1.
func failMode(dir string) (string, error) {
	mode := os.Getenv("CLD_FAKE_OSASCRIPT_FAIL")
	data, err := os.ReadFile(filepath.Join(dir, mac.FailFile))
	switch {
	case err == nil:
		line, _, _ := strings.Cut(string(data), "\n")
		mode = strings.TrimSpace(line)
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	if !slices.Contains(failureModes, mode) {
		refuse("unknown failure mode " + strconv.Quote(mode))
	}
	return mode, nil
}

// refuse fails as osascript does where cld runs it wrong, with status 2.
func refuse(message string) {
	fmt.Fprintln(os.Stderr, "fake osascript:", message)
	os.Exit(2)
}

// garbage fails as osascript does where a script throws out of run, or prints no JSON: status 1,
// and why on stderr where the fake says.
func garbage(why string) {
	if why != "" {
		why = ": " + why
	}
	fmt.Fprintln(os.Stderr, "fake osascript: execution error"+why)
	os.Exit(1)
}

// stall plays osascript held up by an Automation prompt, until cld's deadline kills it. It gives
// up after two minutes, or once its parent has gone, so that no copy outlives its test for long.
func stall() {
	parent := os.Getppid()
	for end := time.Now().Add(2 * time.Minute); time.Now().Before(end); {
		if os.Getppid() != parent {
			os.Exit(1)
		}
		time.Sleep(50 * time.Millisecond)
	}
	garbage("no deadline killed it in two minutes")
}

// failed is the document of a script that failed at a step.
func failed(e scriptError) map[string]scriptError {
	return map[string]scriptError{"error": e}
}

// The document snapshot.js prints (#176), its keys in the script's order.
type (
	document struct {
		Running   bool        `json:"running"`
		Apps      []app       `json:"apps"`
		Version   string      `json:"version"`
		Terminals []string    `json:"terminals"`
		Windows   []windowDoc `json:"windows"`
	}
	app struct {
		PID        int     `json:"pid"`
		Launched   float64 `json:"launched"`
		Executable string  `json:"executable"`
	}
	windowDoc struct {
		Tabs []tabDoc `json:"tabs"`
	}
	tabDoc struct {
		Selected  bool          `json:"selected"`
		Focused   string        `json:"focused"`
		Terminals []terminalDoc `json:"terminals"`
	}
	terminalDoc struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Cwd  string `json:"cwd"`
		PID  int    `json:"pid"`
		TTY  string `json:"tty"`
	}
)

// snapshot prints snapshot.js's document of m, a Ghostty that runs, once the delay file's wait is
// over, or the error document of mode.
func snapshot(dir string, m *mac.Model, mode string) error {
	if failure, canned := snapshotFailures[mode]; canned {
		return printJSON(failed(failure))
	}
	doc := document{Running: true, Version: m.Version, Terminals: m.IDs(),
		Windows: []windowDoc{}}
	for range max(m.Apps, 1) {
		doc.Apps = append(doc.Apps, app{m.PID, m.Launched, m.Executable})
	}
	for _, window := range m.Windows {
		read := windowDoc{Tabs: []tabDoc{}}
		for _, tab := range window.Tabs {
			terminals, err := leaves(m, tab.Tree)
			if err != nil {
				return err
			}
			read.Tabs = append(read.Tabs, tabDoc{tab.Selected, tab.Focused, terminals})
		}
		doc.Windows = append(doc.Windows, read)
	}
	if doc.Terminals == nil {
		doc.Terminals = []string{}
	}
	if err := hold(dir); err != nil {
		return err
	}
	return printJSON(doc)
}

// leaves are the terminals of a tab's tree, in its order.
func leaves(m *mac.Model, tree *mac.Node) ([]terminalDoc, error) {
	terminals := []terminalDoc{}
	for _, id := range tree.Leaves() {
		t, ok := m.Terminals[id]
		if !ok {
			return nil, fmt.Errorf("%s: no terminal %s", mac.ModelFile, id)
		}
		terminals = append(terminals, terminalDoc{id, t.Name, t.Cwd, t.PID, t.TTY})
	}
	return terminals, nil
}

// hold waits the seconds the delay file holds, where it exists, and removes it first: only the
// snapshot that removes it waits.
func hold(dir string) error {
	path := filepath.Join(dir, mac.DelayFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return fmt.Errorf("%s: %w", mac.DelayFile, err)
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	time.Sleep(time.Duration(seconds * float64(time.Second)))
	return nil
}

// printJSON prints value as JSON.stringify writes it, and osascript prints a script's result: on
// a line, with <, > and & as they are.
func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
