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

// bash with bash-completion 2, zsh and fish load what setup completion wrote, each started as a
// user starts it; a shell that is not installed skips (decision 22.7).

// shellEnv is the environment a shell runs in to load what setup completion wrote: the sandbox's,
// with env, and the cld under test first on the PATH, for the scripts that run it on every TAB.
func shellEnv(s *sandbox.Sandbox, env map[string]string) []string {
	path := filepath.Dir(sandbox.Cld) + string(os.PathListSeparator) + os.Getenv("PATH")
	extra := map[string]string{"PATH": path}
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

// lookFile is the first of paths that exists; a test without any skips, saying what is not
// installed.
func lookFile(t *testing.T, what string, paths ...string) string {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skipf("%s is not installed", what)
	return ""
}

// bashCompletion is bash-completion 2's main script, which ~/.bashrc loads; a test without it
// skips.
func bashCompletion(t *testing.T) string {
	t.Helper()
	return lookFile(t, "bash-completion 2", "/usr/share/bash-completion/bash_completion",
		"/opt/homebrew/share/bash-completion/bash_completion",
		"/usr/local/share/bash-completion/bash_completion")
}

// bash-completion 2, loaded as ~/.bashrc loads it, finds the script where setup completion bash
// wrote it, and registers cld's function. The test loads it as a first TAB does: by _comp_load
// since bash-completion 2.12, __load_completion before.
func TestSetupCompletionBashLoadsIt(t *testing.T) {
	t.Parallel()
	bash := lookShell(t, "bash")
	main := bashCompletion(t)
	load := ". " + main + "; if declare -F _comp_load >/dev/null; then _comp_load cld; " +
		"else __load_completion cld; fi; complete -p cld"
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
			out := runShell(t, s, shellEnv(s, test.env), bash,
				"--norc", "--noprofile", "-i", "-c", load)
			if !strings.HasSuffix(out, " -F __start_cld cld\n") {
				t.Errorf("complete -p cld prints %q, want cld's function, __start_cld", out)
			}
		})
	}
}

// zshOrder is what .zshrc holds before and after cld's lines: compinit on one side, or nothing,
// where the lines run it themselves. foo is the completion foo then has.
type zshOrder struct {
	name          string
	before, after string
	foo           string
}

const (
	// zshCompinit runs compinit, then gives foo a completion, which a second compinit would drop.
	zshCompinit = "autoload -U compinit && compinit -i\ncompdef _gnu_generic foo\n"
	// zshCheck prints cld's and foo's completions, and the file _cld comes from.
	zshCheck = `autoload +X _cld; ` +
		`print -r -- "cld=$_comps[cld] foo=$_comps[foo] from=$functions_source[_cld]"`
)

// zshOrders are TestSetupCompletionZshLoadsIt's places for compinit.
var zshOrders = []zshOrder{
	{"alone", "", "", ""},
	{"after compinit", zshCompinit, "", "_gnu_generic"},
	{"before compinit", "", zshCompinit, "_gnu_generic"},
}

// zsh, started interactively, loads the script where setup completion zsh wrote it, through the
// lines in .zshrc. _cld completes cld, from cld's file, whether compinit runs in the lines,
// before them or after them, and foo keeps its completion (decision 22.3).
func TestSetupCompletionZshLoadsIt(t *testing.T) {
	t.Parallel()
	zsh := lookShell(t, "zsh")
	for _, test := range completionCases {
		if test.shell != "zsh" {
			continue
		}
		for _, order := range zshOrders {
			t.Run(test.name+" "+order.name, func(t *testing.T) {
				t.Parallel()
				checkZshLoads(t, zsh, test, order)
			})
		}
	}
}

// checkZshLoads runs setup completion zsh with compinit in order's place in .zshrc, and checks
// what an interactive zsh then completes.
func checkZshLoads(t *testing.T, zsh string, test completionCase, order zshOrder) {
	t.Helper()
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
	out := runShell(t, s, shellEnv(s, test.env), zsh, "-i", "-c", zshCheck)
	want := "cld=_cld foo=" + order.foo + " from=" + filepath.Join(s.Root, test.script) + "\n"
	if out != want {
		t.Errorf("zsh prints %q, want %q", out, want)
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
			out := runShell(t, s, shellEnv(s, test.env), fish,
				"-c", `complete -C "cld setup completion "`)
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			slices.Sort(lines)
			want := []string{
				"bash\tset up cld's completion in bash",
				"fish\tset up cld's completion in fish",
				"zsh\tset up cld's completion in zsh",
			}
			if !slices.Equal(lines, want) {
				t.Errorf("fish completes %q, want %q", lines, want)
			}
		})
	}
}
