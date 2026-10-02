package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld setup completion in the sandbox's home, with nothing on the PATH: each script where its
// shell reads it, byte for byte what cld completion SHELL prints (decision 22). The other
// completion_*_test.go files hold files that exist, refusals and the shells loading the scripts.
// TestUpdateRefreshesCompletion checks cld update writing them anew.

// completionScript is what cld completion shell prints.
func completionScript(t *testing.T, s *sandbox.Sandbox, shell string) string {
	t.Helper()
	result := s.RunCld(map[string]string{"PATH": s.Tools()}, "completion", shell)
	if result.Code != 0 || result.Stderr != "" || result.Stdout == "" {
		t.Fatalf("cld completion %s: exit %d, stderr %q", shell, result.Code, result.Stderr)
	}
	return result.Stdout
}

// setupCompletion runs cld setup completion shell with nothing on the PATH, and the variables env,
// in which {root} stands for the sandbox's root.
func setupCompletion(s *sandbox.Sandbox, shell string, env map[string]string) sandbox.Result {
	extra := map[string]string{"PATH": s.Tools()}
	for name, value := range env {
		extra[name] = strings.ReplaceAll(value, "{root}", s.Root)
	}
	return s.RunCld(extra, "setup", "completion", shell)
}

// completionCase is where setup completion writes, under the sandbox's root, with the variables
// that move the files.
type completionCase struct {
	name  string
	shell string
	env   map[string]string
	// script is where the script goes, rc where .zshrc is, "" for another shell.
	script string
	rc     string
}

// completionCases are where each shell reads the script. The first directory of
// BASH_COMPLETION_USER_DIR comes before XDG_DATA_HOME, as for bash-completion.
var completionCases = []completionCase{
	{"bash", "bash", nil, "home/" + scripts["bash"], ""},
	{"bash XDG_DATA_HOME", "bash", map[string]string{"XDG_DATA_HOME": "{root}/data"},
		"data/bash-completion/completions/cld", ""},
	{"bash BASH_COMPLETION_USER_DIR", "bash", map[string]string{
		"BASH_COMPLETION_USER_DIR": ":{root}/a:{root}/b", "XDG_DATA_HOME": "{root}/data",
	}, "a/completions/cld", ""},
	{"zsh", "zsh", nil, "home/" + scripts["zsh"], "home/.zshrc"},
	{"zsh XDG_DATA_HOME ZDOTDIR", "zsh", map[string]string{
		"XDG_DATA_HOME": "{root}/data", "ZDOTDIR": "{root}/zdot",
	}, "data/cld/zsh/_cld", "zdot/.zshrc"},
	{"fish", "fish", nil, "home/" + scripts["fish"], ""},
	{"fish XDG_CONFIG_HOME", "fish", map[string]string{"XDG_CONFIG_HOME": "{root}/config"},
		"config/fish/completions/cld.fish", ""},
}

// setup completion SHELL writes the script where the shell reads it, making its directories, and
// for zsh the lines in .zshrc that load it. It says what it wrote and that a new shell takes it,
// and changes nothing else. Run again, it changes nothing, and says so.
func TestSetupCompletion(t *testing.T) {
	t.Parallel()
	for _, test := range completionCases {
		t.Run(test.name, test.checkWrites)
	}
}

// checkWrites runs setup completion in a new sandbox, and again, checking what each run writes
// and says.
func (test completionCase) checkWrites(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	before := sandboxFiles(t, s)
	script, rc := filepath.Join(s.Root, test.script), filepath.Join(s.Root, test.rc)
	want := "Created " + script + "\n"
	if test.rc != "" {
		want += "Created " + rc + "\n"
	}
	want += "Start a new " + test.shell + " for it to take effect\n"
	if result := setupCompletion(s, test.shell, test.env); result != (sandbox.Result{Stdout: want}) {
		t.Errorf("got %+v, want stdout\n%s", result, want)
	}
	checkContent(t, script, completionScript(t, s, test.shell))
	if info, err := os.Stat(script); err != nil || info.Mode().Perm() != newFileMode(t, s) {
		t.Errorf("%s: %v (%v), want mode %v", script, info.Mode(), err, newFileMode(t, s))
	}
	if test.rc != "" {
		checkContent(t, rc, zshLines)
	}
	if changed := changedFiles(test.others(sandboxFiles(t, s)), before); len(changed) > 0 {
		t.Errorf("beside the files, cld changed %q", changed)
	}
	test.checkRunAgain(t, s, script, rc)
}

// others is files less the script, .zshrc and the directories that hold them, but home.
func (test completionCase) others(files map[string]string) map[string]string {
	for _, name := range []string{test.script, test.rc} {
		for dir := filepath.Dir(name); dir != "." && dir != "home"; dir = filepath.Dir(dir) {
			delete(files, dir)
		}
		delete(files, name)
	}
	return files
}

// checkRunAgain runs setup completion once more, which leaves the files as they are and says so.
func (test completionCase) checkRunAgain(t *testing.T, s *sandbox.Sandbox, script, rc string) {
	t.Helper()
	after := sandboxFiles(t, s)
	want := "Left " + script + " as it was\n"
	if test.rc != "" {
		want += "Left " + rc + " as it was\n"
	}
	if result := setupCompletion(s, test.shell, test.env); result != (sandbox.Result{Stdout: want}) {
		t.Errorf("run again: got %+v, want stdout\n%s", result, want)
	}
	if changed := changedFiles(sandboxFiles(t, s), after); len(changed) > 0 {
		t.Errorf("run again, cld changed %q", changed)
	}
}

// sandboxFiles is what the sandbox holds, as tree has it, but for the directories of tools that
// Tools makes for the PATH.
func sandboxFiles(t *testing.T, s *sandbox.Sandbox) map[string]string {
	t.Helper()
	files := tree(t, s.Root)
	for name := range files {
		if strings.HasPrefix(name, "tools.") {
			delete(files, name)
		}
	}
	return files
}

// changedFiles are the paths that a and b hold differently, sorted.
func changedFiles(a, b map[string]string) []string {
	var names []string
	for name, value := range a {
		if other, ok := b[name]; !ok || other != value {
			names = append(names, name)
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// newFileMode is the mode of a file made with mode 0644, as cld makes one: less the umask, which
// the sandbox's own files show.
func newFileMode(t *testing.T, s *sandbox.Sandbox) os.FileMode {
	t.Helper()
	info, err := os.Stat(filepath.Join(s.Home, ".tmux.conf"))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
