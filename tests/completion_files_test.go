package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup completion on files that exist, which it edits in place, and what it refuses
// (decision 22.4).

// completionEdit is a file that exists before setup completion runs.
type completionEdit struct {
	name  string
	shell string
	// script is what the script holds before, with mode; link, where set, is where the script is
	// a symbolic link to, under the root, holding script. rc is .zshrc before, and wantRC after;
	// "" for none.
	script string
	mode   os.FileMode
	link   string
	rc     string
	wantRC string
	// stdout is what cld says, with {script} and {rc} for the files.
	stdout string
}

const (
	// zshrcStart is a user's .zshrc.
	zshrcStart = "export EDITOR=vi\n"
	// zshrcMoved is a .zshrc with cld's first line, spaces and a CR added, then a line of the
	// user's in place of cld's others, and zshrcStart.
	zshrcMoved = "# cld's completion, from cld setup completion zsh  \r\n" +
		"fpath=(~/mine $fpath)\n" + zshrcStart
	// zshUpdated is what setup completion zsh says where it adds its lines to a .zshrc.
	zshUpdated = "Created {script}\nUpdated {rc}: the lines that load the script\n" +
		"Start a new zsh for it to take effect\n"
)

// completionEdits are TestSetupCompletionEditsFiles' cases.
var completionEdits = []completionEdit{
	{name: "a script", shell: "bash", script: "complete -F _mine cld\n", mode: 0o600,
		stdout: "Updated {script}\nStart a new bash for it to take effect\n"},
	{name: "a script through a link", shell: "fish", script: "# mine\n", mode: 0o644,
		link:   "dotfiles/cld.fish",
		stdout: "Updated {script}\nStart a new fish for it to take effect\n"},
	{name: "a .zshrc", shell: "zsh", rc: zshrcStart, wantRC: zshrcStart + "\n" + zshLines,
		stdout: zshUpdated},
	{name: "a .zshrc without a last newline", shell: "zsh", rc: "export EDITOR=vi",
		wantRC: zshrcStart + "\n" + zshLines, stdout: zshUpdated},
	{name: "a .zshrc ending in a blank line", shell: "zsh", rc: zshrcStart + "\n",
		wantRC: zshrcStart + "\n" + zshLines, stdout: zshUpdated},
	{name: "an empty .zshrc", shell: "zsh", rc: "\n", wantRC: "\n\n" + zshLines,
		stdout: zshUpdated},
	{name: "a .zshrc with the lines moved and changed", shell: "zsh",
		rc: zshrcMoved, wantRC: zshrcMoved,
		stdout: "Created {script}\nLeft {rc} as it was\nStart a new zsh for it to take effect\n"},
}

// Files that exist: a script is replaced whole, keeping its mode, and through a symbolic link the
// file it leads to. .zshrc keeps what it holds and gets cld's lines after a blank line. A .zshrc
// that holds their first line anywhere, followed by anything, stays unchanged.
func TestSetupCompletionEditsFiles(t *testing.T) {
	t.Parallel()
	for _, test := range completionEdits {
		t.Run(test.name, test.run)
	}
}

// run writes the files that exist, runs setup completion, and checks what it writes and says.
func (test completionEdit) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	script, rc := filepath.Join(s.Home, scripts[test.shell]), filepath.Join(s.Home, ".zshrc")
	target := test.writeScript(t, s, script)
	if test.rc != "" {
		s.WriteFile(rc, test.rc)
	}
	want := strings.NewReplacer("{script}", script, "{rc}", rc).Replace(test.stdout)
	if result := setupCompletion(s, test.shell, nil); result != (sandbox.Result{Stdout: want}) {
		t.Errorf("got %+v, want stdout\n%s", result, want)
	}
	checkContent(t, target, completionScript(t, s, test.shell))
	if test.script != "" {
		if info, err := os.Stat(target); err != nil || info.Mode().Perm() != test.mode {
			t.Errorf("%s: %v (%v), want mode %o", target, info.Mode(), err, test.mode)
		}
	}
	if test.link != "" {
		if link, err := os.Readlink(script); err != nil || link != target {
			t.Errorf("%s leads to %q (%v), want %q", script, link, err, target)
		}
	}
	if test.wantRC != "" {
		checkContent(t, rc, test.wantRC)
	}
}

// writeScript writes the script that exists, through the link where there is one, and returns
// the file that holds it: script itself where there is no link, or no script.
func (test completionEdit) writeScript(t *testing.T, s *sandbox.Sandbox, script string) string {
	t.Helper()
	if test.script == "" {
		return script
	}
	target := script
	if test.link != "" {
		target = filepath.Join(s.Root, test.link)
		if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, script); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(test.script), test.mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, test.mode); err != nil {
		t.Fatal(err)
	}
	return target
}

// completionRefusal is what setup completion refuses: prepare makes it in the home directory
// home, and stderr is cld's message, with {home} for that directory.
type completionRefusal struct {
	name    string
	shell   string
	env     map[string]string
	prepare func(t *testing.T, home string)
	stderr  string
}

// completionRefusals are TestSetupCompletionRefuses' cases.
var completionRefusals = []completionRefusal{
	{"no home directory", "bash", map[string]string{"HOME": ""}, nil,
		"cld: cannot find where the script goes: $HOME is not defined\n"},
	{"no home directory for .zshrc", "zsh",
		map[string]string{"HOME": "", "XDG_DATA_HOME": "{root}/data"}, nil,
		"cld: cannot find .zshrc: $HOME is not defined\n"},
	{".zshrc a directory", "zsh", nil, zshrcDirectory,
		"cld: cannot read {home}/.zshrc: is a directory\n"},
	{".zshrc a link that leads nowhere", "zsh", nil, zshrcLinkToNothing,
		"cld: {home}/.zshrc is a symbolic link to a file that does not exist\n"},
	{"a file where a directory goes", "fish", nil, configAFile,
		"cld: cannot read {home}/.config/fish/completions/cld.fish: not a directory\n"},
}

// setup completion reads every file it writes before it writes any. So a .zshrc it cannot read,
// or a link to nothing, which writing would replace with a file, leaves the script unwritten too.
// Without a home directory, it cannot tell where the files go; where a directory cannot be made,
// it says why. Each is a failure, with status 1.
func TestSetupCompletionRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range completionRefusals {
		t.Run(test.name, test.run)
	}
}

// run makes what cld refuses, and runs setup completion, which fails and changes nothing.
func (test completionRefusal) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if test.prepare != nil {
		test.prepare(t, s.Home)
	}
	before := sandboxFiles(t, s)
	want := strings.ReplaceAll(test.stderr, "{home}", s.Home)
	result := setupCompletion(s, test.shell, test.env)
	if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if changed := changedFiles(sandboxFiles(t, s), before); len(changed) > 0 {
		t.Errorf("cld changed %q", changed)
	}
}

// zshrcDirectory makes .zshrc in home a directory.
func zshrcDirectory(t *testing.T, home string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(home, ".zshrc"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// zshrcLinkToNothing makes .zshrc in home a symbolic link to a file that does not exist.
func zshrcLinkToNothing(t *testing.T, home string) {
	t.Helper()
	target := filepath.Join(home, "dotfiles", "zshrc")
	if err := os.Symlink(target, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
}

// configAFile makes .config in home a file, where fish's directory goes.
func configAFile(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".config"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
