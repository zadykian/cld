package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld setup completion in the sandbox's home directory, with nothing on the PATH: the script, byte
// for byte what cld completion SHELL prints, where each shell reads it - where the variables that
// move it say - zsh's lines in .zshrc, and what cld says it did; running it again, files that
// exist, refusals. Where a shell is installed, it then loads what cld wrote, started as a user
// starts it: bash with bash-completion 2, zsh and fish. cld update's refresh of the scripts is in
// update_test.go.

// zshLines are the lines setup completion zsh adds to .zshrc.
const zshLines = `# cld's completion, from cld setup completion zsh
if [[ -r ${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh/_cld ]]; then
  fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh" $fpath)
  (( $+functions[compdef] )) || { autoload -U compinit && compinit -i; }
  autoload -Uz _cld && compdef _cld cld
fi
`

// scripts are where setup completion writes each shell's script, under the home directory, without
// the variables that move it.
var scripts = map[string]string{
	"bash": ".local/share/bash-completion/completions/cld",
	"zsh":  ".local/share/cld/zsh/_cld",
	"fish": ".config/fish/completions/cld.fish",
}

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

// checkContent reports a file at path that does not hold want.
func checkContent(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Errorf("%s holds (%v)\n%s\nwant\n%s", path, err, data, want)
	}
}

// completionCases are where setup completion writes, under the sandbox's root, with the variables
// that move the files, as a shell reads them: in the first directory of BASH_COMPLETION_USER_DIR,
// which comes before XDG_DATA_HOME, as for bash-completion.
var completionCases = []struct {
	name  string
	shell string
	env   map[string]string
	// script is where the script goes, rc where .zshrc is, "" for another shell.
	script string
	rc     string
}{
	{"bash", "bash", nil, "home/" + scripts["bash"], ""},
	{"bash XDG_DATA_HOME", "bash", map[string]string{"XDG_DATA_HOME": "{root}/data"}, "data/bash-completion/completions/cld", ""},
	{"bash BASH_COMPLETION_USER_DIR", "bash", map[string]string{"BASH_COMPLETION_USER_DIR": ":{root}/a:{root}/b", "XDG_DATA_HOME": "{root}/data"},
		"a/completions/cld", ""},
	{"zsh", "zsh", nil, "home/" + scripts["zsh"], "home/.zshrc"},
	{"zsh XDG_DATA_HOME ZDOTDIR", "zsh", map[string]string{"XDG_DATA_HOME": "{root}/data", "ZDOTDIR": "{root}/zdot"}, "data/cld/zsh/_cld", "zdot/.zshrc"},
	{"fish", "fish", nil, "home/" + scripts["fish"], ""},
	{"fish XDG_CONFIG_HOME", "fish", map[string]string{"XDG_CONFIG_HOME": "{root}/config"}, "config/fish/completions/cld.fish", ""},
}

// setup completion SHELL writes the script where the shell reads it, making its directories, and
// for zsh .zshrc with the lines that load it, then says what it wrote and that a new shell takes
// it; nothing else changes. Run again, it changes nothing, and says so.
func TestSetupCompletion(t *testing.T) {
	t.Parallel()
	for _, test := range completionCases {
		t.Run(test.name, func(t *testing.T) {
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
			added := sandboxFiles(t, s)
			for dir := filepath.Dir(test.script); dir != "." && dir != "home"; dir = filepath.Dir(dir) {
				delete(added, dir)
			}
			for dir := filepath.Dir(test.rc); dir != "." && dir != "home"; dir = filepath.Dir(dir) {
				delete(added, dir)
			}
			delete(added, test.script)
			delete(added, test.rc)
			if changed := changedFiles(added, before); len(changed) > 0 {
				t.Errorf("beside the files, cld changed %q", changed)
			}

			after := sandboxFiles(t, s)
			want = "Left " + script + " as it was\n"
			if test.rc != "" {
				want += "Left " + rc + " as it was\n"
			}
			if result := setupCompletion(s, test.shell, test.env); result != (sandbox.Result{Stdout: want}) {
				t.Errorf("run again: got %+v, want stdout\n%s", result, want)
			}
			if changed := changedFiles(sandboxFiles(t, s), after); len(changed) > 0 {
				t.Errorf("run again, cld changed %q", changed)
			}
		})
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

// Files that exist: a script is replaced whole, keeping its mode, and through a symbolic link the
// file it leads to; .zshrc keeps what it holds, gets cld's lines after a blank line, and is left
// as it is where it has their first line, wherever and however it goes on.
func TestSetupCompletionEditsFiles(t *testing.T) {
	t.Parallel()
	const rcStart = "export EDITOR=vi\n"
	for _, test := range []struct {
		name  string
		shell string
		// script is what the script holds before, with mode; link, where set, is where the
		// script is a symbolic link to, under the root, holding script. rc is .zshrc before, and
		// wantRC after; "" for none.
		script string
		mode   os.FileMode
		link   string
		rc     string
		wantRC string
		// stdout is what cld says, with {script} and {rc} for the files.
		stdout string
	}{
		{name: "a script", shell: "bash", script: "complete -F _mine cld\n", mode: 0o600,
			stdout: "Updated {script}\nStart a new bash for it to take effect\n"},
		{name: "a script through a link", shell: "fish", script: "# mine\n", mode: 0o644, link: "dotfiles/cld.fish",
			stdout: "Updated {script}\nStart a new fish for it to take effect\n"},
		{name: "a .zshrc", shell: "zsh", rc: rcStart, wantRC: rcStart + "\n" + zshLines,
			stdout: "Created {script}\nUpdated {rc}: the lines that load the script\nStart a new zsh for it to take effect\n"},
		{name: "a .zshrc without a last newline", shell: "zsh", rc: "export EDITOR=vi", wantRC: rcStart + "\n" + zshLines,
			stdout: "Created {script}\nUpdated {rc}: the lines that load the script\nStart a new zsh for it to take effect\n"},
		{name: "a .zshrc ending in a blank line", shell: "zsh", rc: rcStart + "\n", wantRC: rcStart + "\n" + zshLines,
			stdout: "Created {script}\nUpdated {rc}: the lines that load the script\nStart a new zsh for it to take effect\n"},
		{name: "an empty .zshrc", shell: "zsh", rc: "\n", wantRC: "\n\n" + zshLines,
			stdout: "Created {script}\nUpdated {rc}: the lines that load the script\nStart a new zsh for it to take effect\n"},
		{name: "a .zshrc with the lines moved and changed", shell: "zsh",
			rc:     "# cld's completion, from cld setup completion zsh  \r\nfpath=(~/mine $fpath)\n" + rcStart,
			wantRC: "# cld's completion, from cld setup completion zsh  \r\nfpath=(~/mine $fpath)\n" + rcStart,
			stdout: "Created {script}\nLeft {rc} as it was\nStart a new zsh for it to take effect\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			script, rc := filepath.Join(s.Home, scripts[test.shell]), filepath.Join(s.Home, ".zshrc")
			target := script
			if test.script != "" {
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
			}
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
		})
	}
}

// setup completion reads every file it writes before it writes any: a .zshrc it cannot read, or a
// symbolic link that leads nowhere, which writing would replace with a file, leaves the script
// unwritten too. Without a home directory, it cannot tell where the files go; where a directory
// cannot be made, it says why. Each is a failure, with status 1.
func TestSetupCompletionRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		shell string
		env   map[string]string
		// prepare makes what cld refuses, in the home directory home; stderr is cld's message,
		// with {home} for it.
		prepare func(t *testing.T, home string)
		stderr  string
	}{
		{"no home directory", "bash", map[string]string{"HOME": ""}, nil,
			"cld: cannot find where the script goes: $HOME is not defined\n"},
		{"no home directory for .zshrc", "zsh", map[string]string{"HOME": "", "XDG_DATA_HOME": "{root}/data"}, nil,
			"cld: cannot find .zshrc: $HOME is not defined\n"},
		{".zshrc a directory", "zsh", nil, func(t *testing.T, home string) {
			if err := os.Mkdir(filepath.Join(home, ".zshrc"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "cld: cannot read {home}/.zshrc: is a directory\n"},
		{".zshrc a link that leads nowhere", "zsh", nil, func(t *testing.T, home string) {
			if err := os.Symlink(filepath.Join(home, "dotfiles", "zshrc"), filepath.Join(home, ".zshrc")); err != nil {
				t.Fatal(err)
			}
		}, "cld: {home}/.zshrc is a symbolic link to a file that does not exist\n"},
		{"a file where a directory goes", "fish", nil, func(t *testing.T, home string) {
			if err := os.WriteFile(filepath.Join(home, ".config"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "cld: cannot read {home}/.config/fish/completions/cld.fish: not a directory\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.prepare != nil {
				test.prepare(t, s.Home)
			}
			before := sandboxFiles(t, s)
			want := strings.ReplaceAll(test.stderr, "{home}", s.Home)
			if result := setupCompletion(s, test.shell, test.env); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if changed := changedFiles(sandboxFiles(t, s), before); len(changed) > 0 {
				t.Errorf("cld changed %q", changed)
			}
		})
	}
}

// shellEnv is the environment a shell runs in to load what setup completion wrote: the sandbox's,
// with env, and the cld under test first on the PATH, for the scripts that run it on every TAB.
func shellEnv(s *sandbox.Sandbox, env map[string]string) []string {
	extra := map[string]string{"PATH": filepath.Dir(sandbox.Cld) + string(os.PathListSeparator) + os.Getenv("PATH")}
	for name, value := range env {
		extra[name] = strings.ReplaceAll(value, "{root}", s.Root)
	}
	return s.Environ(extra)
}

// runShell runs the shell at path with args, in env, and returns what it prints on stdout;
// stderr, where an interactive shell without a terminal complains of job control, is dropped.
func runShell(t *testing.T, s *sandbox.Sandbox, env []string, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command(path, args...)
	cmd.Env, cmd.Dir = env, s.Work
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", path, args, err, out)
	}
	return string(out)
}

// lookShell is the shell name on the PATH; a test without it skips.
func lookShell(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	return path
}

// bash-completion 2, loaded as ~/.bashrc loads it, finds the script where setup completion bash
// wrote it, as it looks for a command's completion at its first TAB, and registers cld's
// function: _comp_load since bash-completion 2.12, __load_completion before.
func TestSetupCompletionBashLoadsIt(t *testing.T) {
	t.Parallel()
	bash := lookShell(t, "bash")
	var main string
	for _, candidate := range []string{"/usr/share/bash-completion/bash_completion", "/opt/homebrew/share/bash-completion/bash_completion",
		"/usr/local/share/bash-completion/bash_completion"} {
		if _, err := os.Stat(candidate); err == nil {
			main = candidate
			break
		}
	}
	if main == "" {
		t.Skip("bash-completion 2 is not installed")
	}
	load := ". " + main + "; if declare -F _comp_load >/dev/null; then _comp_load cld; else __load_completion cld; fi; complete -p cld"
	for _, test := range completionCases {
		if test.shell != "bash" {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := setupCompletion(s, "bash", test.env); result.Code != 0 {
				t.Fatalf("setup completion bash: %+v", result)
			}
			out := runShell(t, s, shellEnv(s, test.env), bash, "--norc", "--noprofile", "-i", "-c", load)
			if !strings.HasSuffix(out, " -F __start_cld cld\n") {
				t.Errorf("complete -p cld prints %q, want cld's function, __start_cld", out)
			}
		})
	}
}

// zsh, started interactively, loads the script where setup completion zsh wrote it, through the
// lines in .zshrc: _cld completes cld, from cld's file, whether compinit runs in them or ran
// before them - where a second compinit would drop foo's completion - or runs after them.
func TestSetupCompletionZshLoadsIt(t *testing.T) {
	t.Parallel()
	zsh := lookShell(t, "zsh")
	const (
		compinit = "autoload -U compinit && compinit -i\ncompdef _gnu_generic foo\n"
		check    = `autoload +X _cld; print -r -- "cld=$_comps[cld] foo=$_comps[foo] from=$functions_source[_cld]"`
	)
	for _, test := range completionCases {
		if test.shell != "zsh" {
			continue
		}
		for _, order := range []struct {
			name          string
			before, after string
			foo           string
		}{
			{"alone", "", "", ""},
			{"after compinit", compinit, "", "_gnu_generic"},
			{"before compinit", "", compinit, "_gnu_generic"},
		} {
			t.Run(test.name+" "+order.name, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				rc := filepath.Join(s.Root, test.rc)
				if order.before != "" {
					if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
						t.Fatal(err)
					}
					s.WriteFile(rc, order.before)
				}
				if result := setupCompletion(s, "zsh", test.env); result.Code != 0 {
					t.Fatalf("setup completion zsh: %+v", result)
				}
				if order.after != "" {
					data, err := os.ReadFile(rc)
					if err != nil {
						t.Fatal(err)
					}
					s.WriteFile(rc, string(data)+order.after)
				}
				out := runShell(t, s, shellEnv(s, test.env), zsh, "-i", "-c", check)
				if want := "cld=_cld foo=" + order.foo + " from=" + filepath.Join(s.Root, test.script) + "\n"; out != want {
					t.Errorf("zsh prints %q, want %q", out, want)
				}
			})
		}
	}
}

// fish finds the script where setup completion fish wrote it, in a home directory where fish has
// never run, and completes cld's commands through it, with their descriptions.
func TestSetupCompletionFishLoadsIt(t *testing.T) {
	t.Parallel()
	fish := lookShell(t, "fish")
	for _, test := range completionCases {
		if test.shell != "fish" {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := setupCompletion(s, "fish", test.env); result.Code != 0 {
				t.Fatalf("setup completion fish: %+v", result)
			}
			out := runShell(t, s, shellEnv(s, test.env), fish, "-c", `complete -C "cld setup completion "`)
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			slices.Sort(lines)
			want := []string{"bash\tset up cld's completion in bash", "fish\tset up cld's completion in fish", "zsh\tset up cld's completion in zsh"}
			if !slices.Equal(lines, want) {
				t.Errorf("fish completes %q, want %q", lines, want)
			}
		})
	}
}
