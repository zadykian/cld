package tests

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld's scripts themselves: the terms of Ghostty's dictionary they use, their syntax, their logic
// against a mock Ghostty, where macOS's osascript runs them, and the real ps.

// scriptsDir holds the scripts cld embeds.
var scriptsDir = filepath.Join("..", "internal", "ghostty", "scripts")

// sdefTerms are the terms of Ghostty's dictionary, testdata/ghostty/Ghostty.sdef, as JXA spells
// them: its classes and their plurals, properties, commands, parameters and enumerators in
// camelCase. JXA's own byId, length and running join them.
func sdefTerms(t *testing.T) map[string]bool {
	t.Helper()
	kinds := map[string]bool{"class": true, "property": true, "command": true, "parameter": true,
		"enumerator": true}
	terms := map[string]bool{"byId": true, "length": true, "running": true}
	decoder := xml.NewDecoder(strings.NewReader(readFile(t, filepath.Join("testdata", "ghostty",
		"Ghostty.sdef"))))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return terms
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && kinds[start.Name.Local] {
			for _, attr := range start.Attr {
				if attr.Name.Local == "name" || attr.Name.Local == "plural" {
					terms[camelCase(attr.Value)] = true
				}
			}
		}
	}
}

// camelCase is a dictionary's term as JXA spells it: "focused terminal" as focusedTerminal.
func camelCase(term string) string {
	words := strings.Fields(term)
	for i := 1; i < len(words); i++ {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, "")
}

// Where a script's text names Ghostty's terms: a chain on G, the application, and the keys of a
// record a command of G's takes. A surface configuration's keys, set on c, name them too.
var (
	chainOnG      = regexp.MustCompile(`\bG((?:\.[A-Za-z]+|\[[^\]]*\])+)`)
	chainName     = regexp.MustCompile(`\.([A-Za-z]+)`)
	commandRecord = regexp.MustCompile(`G\.[A-Za-z]+\((?:[^{};]*,\s*)?\{([^{}]*)\}\)`)
	recordKey     = regexp.MustCompile(`([A-Za-z]+):`)
	configKey     = regexp.MustCompile(`c\.([A-Za-z]+) =`)
)

// scriptTerms are the terms of Ghostty's that script names, in order.
func scriptTerms(script string) []string {
	var terms []string
	for _, chain := range chainOnG.FindAllStringSubmatch(script, -1) {
		for _, name := range chainName.FindAllStringSubmatch(chain[1], -1) {
			terms = append(terms, name[1])
		}
	}
	for _, record := range commandRecord.FindAllStringSubmatch(script, -1) {
		for _, key := range recordKey.FindAllStringSubmatch(record[1], -1) {
			terms = append(terms, key[1])
		}
	}
	for _, key := range configKey.FindAllStringSubmatch(script, -1) {
		terms = append(terms, key[1])
	}
	return terms
}

// Every term of Ghostty's that a script names is in Ghostty's dictionary, as the release the
// findings checked has it. Each script starts with its marker, which the fake osascript reads.
func TestTabsScriptTerms(t *testing.T) {
	t.Parallel()
	terms := sdefTerms(t)
	bad := "G.windows.fooBar(); G.split(t, {direction: 'right', bogus: 1}); c.nope = 1;"
	known := func(term string) bool { return terms[term] }
	if got := slices.DeleteFunc(scriptTerms(bad), known); !slices.Equal(got,
		[]string{"fooBar", "bogus", "nope"}) {
		t.Errorf("unknown terms of %q: %q, want fooBar, bogus and nope", bad, got)
	}
	scripts, err := filepath.Glob(filepath.Join(scriptsDir, "*.js"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts in %s: %v", scriptsDir, err)
	}
	for _, path := range scripts {
		script := readFile(t, path)
		name := strings.TrimSuffix(filepath.Base(path), ".js")
		if marker, _, _ := strings.Cut(script, "\n"); marker != "// cld: "+name {
			t.Errorf("%s starts %q, want its marker // cld: %s", path, marker, name)
		}
		used := scriptTerms(script)
		if len(used) == 0 {
			t.Errorf("%s names none of Ghostty's terms", path)
		}
		for _, term := range used {
			if !terms[term] {
				t.Errorf("%s: %s is not in Ghostty.sdef", path, term)
			}
		}
	}
}

// Each script compiles as JavaScript for Open Scripting Architecture, where macOS has
// osacompile. JXA finds Ghostty's terms only as a script runs.
func TestTabsScriptSyntax(t *testing.T) {
	t.Parallel()
	osacompile, err := exec.LookPath("osacompile")
	if err != nil {
		t.Skip("no osacompile: the macos job runs it")
	}
	scripts, err := filepath.Glob(filepath.Join(scriptsDir, "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range scripts {
		compiled := filepath.Join(t.TempDir(), filepath.Base(path)+".scpt")
		out, err := exec.Command(osacompile, "-l", "JavaScript", "-o", compiled, path).
			CombinedOutput()
		if err != nil {
			t.Errorf("osacompile %s: %v\n%s", path, err, out)
		}
	}
}

// The real ps lists a pane's shell on a real pty, its login there being Ghostty's child, here the
// tests' own process. The dry-run reads it as a shell at its prompt, with the pty's size.
func TestTabsRealPs(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	pty := sandbox.OpenPty(t)
	pty.SetSize(40, 120, 1680, 1120)
	shell := startOnPty(t, pty)
	m := ghosttyAt(os.Getpid(), ghosttyStart)
	m.Processes = nil
	m.Terminals["A"] = &mac.Terminal{Name: "~", Cwd: s.Home, PID: shell, TTY: pty.Path}
	m.Windows = []mac.Window{windowOf(tabOf("A"))}
	saveModel(t, s, m)
	s.WriteProbeFile(mac.ShowConfig, quietConfig)
	path := s.RealTools("ps") + string(os.PathListSeparator) + s.Tools("osascript", "ghostty")
	result := s.RunCld(map[string]string{"TZ": "UTC", "PATH": path}, "restore", "tabs",
		"--dry-run")
	if result.Code != 0 || result.Stderr != "" {
		t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
	}
	r := readRecord(t, s)
	p := r.Panes["A"]
	if p == nil || p.Kind != "local" || p.Login != shell || p.TTY != pty.Path || p.Size == nil ||
		*p.Size != (tabs.Size{Cols: 120, Rows: 40, Width: 1680, Height: 1120}) {
		t.Errorf("the pane is %+v, size %+v; want local, login %d on %s, 120x40 in 1680x1120",
			p, p.Size, shell, pty.Path)
	}
	if !strings.HasPrefix(r.Run.ID, strconv.Itoa(os.Getpid())+"@") {
		t.Errorf("run %s, want the tests' own process's", r.Run.ID)
	}
}

// startOnPty starts sh on pty as its controlling terminal, in a session of its own as a login's
// shell is, and returns its pid. It ends with the test.
func startOnPty(t *testing.T, pty *sandbox.Pty) int {
	t.Helper()
	sh := exec.Command("sh")
	sh.Stdin, sh.Stdout, sh.Stderr = pty.Terminal, pty.Terminal, pty.Terminal
	sh.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := sh.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sh.Process.Kill() //nolint:errcheck // it may have ended
		_ = sh.Wait()         //nolint:errcheck // a killed shell's status is no failure
	})
	return sh.Process.Pid
}
