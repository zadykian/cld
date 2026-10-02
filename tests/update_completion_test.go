package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The completion scripts that cld update writes anew once cld is replaced (decision 22.5).

// staleLine is a line of an older cld's script, which the new cld does not print.
const staleLine = "# old\n"

// updateRefresh is a case of TestUpdateRefreshesCompletion. files are what the files under the
// sandbox's root hold before update, and want what they hold after, where it differs. updated are
// the scripts update names, shell and path.
type updateRefresh struct {
	name string
	// env are variables for update, with {root} for the sandbox's root.
	env     map[string]string
	files   map[string]string
	want    map[string]string
	updated [][2]string
}

// updateRefreshes are TestUpdateRefreshesCompletion's cases.
var updateRefreshes = []updateRefresh{
	{
		name: "default places",
		files: map[string]string{
			"home/" + scripts["bash"]: "# bash completion V2 for cld   -*- shell-script -*-\n" +
				staleLine,
			"home/" + scripts["zsh"]: "#compdef cld\n" +
				"# requestComp=\"${words[1]} __completeNoDesc ${words[2,-1]}\"\n",
			"home/" + scripts["fish"]: "complete -c cld -a mine\n",
			"home/.zshrc":             zshLines,
		},
		want: map[string]string{
			"home/" + scripts["bash"]: fakeScript("0.5.0", "completion", "bash"),
			"home/" + scripts["zsh"]:  fakeScript("0.5.0", "completion", "zsh", "--no-descriptions"),
		},
		updated: [][2]string{{"bash", "home/" + scripts["bash"]}, {"zsh", "home/" + scripts["zsh"]}},
	},
	{
		name: "moved by variables",
		env:  map[string]string{"XDG_DATA_HOME": "{root}/data", "XDG_CONFIG_HOME": "{root}/config"},
		files: map[string]string{
			"data/bash-completion/completions/cld": fakeScript("0.5.0", "completion", "bash"),
			"config/fish/completions/cld.fish":     "# fish completion for cld \n" + staleLine,
			"home/" + scripts["fish"]:              "# fish completion for cld \n" + staleLine,
		},
		want: map[string]string{
			"config/fish/completions/cld.fish": fakeScript("0.5.0", "completion", "fish"),
		},
		updated: [][2]string{{"fish", "config/fish/completions/cld.fish"}},
	},
}

// updateWrite writes content to path, making its directory.
func updateWrite(t *testing.T, s *sandbox.Sandbox, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(path, content)
}

// update has the new cld print each script where setup completion writes one, and writes and
// names those that differ. A script without descriptions stays one. A script printed the same, a
// file unlike cobra's script, one elsewhere and .zshrc stay untouched. TestUpdate has no script,
// and update says no more there.
func TestUpdateRefreshesCompletion(t *testing.T) {
	t.Parallel()
	for _, test := range updateRefreshes {
		t.Run(test.name, test.run)
	}
}

// run updates cld over the case's files.
func (test updateRefresh) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := latestRelease(t, s, "0.5.0")
	path := filepath.Join(s.Root, "bin", "cld")
	placeCld(t, s, path, oldCld(t), 0o755)
	for name, content := range test.files {
		updateWrite(t, s, filepath.Join(s.Root, name), content)
	}
	cmd := updateCommand(s, path, r.url)
	for name, value := range test.env {
		cmd.Env = append(cmd.Env, name+"="+strings.ReplaceAll(value, "{root}", s.Root))
	}
	want := "Updated cld 0.4.0 to 0.5.0: " + path + "\n"
	for _, updated := range test.updated {
		want += "Updated the completion script for " + updated[0] + ": " +
			filepath.Join(s.Root, updated[1]) + "\n"
	}
	if result := runCommand(t, cmd); result != (sandbox.Result{Stdout: want}) {
		t.Errorf("got %+v, want stdout\n%s", result, want)
	}
	for name, content := range test.files {
		if updated, ok := test.want[name]; ok {
			content = updated
		}
		checkContent(t, filepath.Join(s.Root, name), content)
	}
}

// Where the new cld cannot print a script, cld is updated all the same. update warns, naming the
// script, why and the command to run by hand, then goes on to the next shell and exits with 0.
func TestUpdateCompletionFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	const files = "download/v0.5.0"
	binary := "#!/bin/sh\n" +
		"case \"$1 $2\" in\n" +
		"'completion bash') echo 'no completion here' >&2; exit 3 ;;\n" +
		"'completion fish') printf '# fish completion for cld %s, 0.5.0\\n' \"$*\" ;;\n" +
		"*) echo 'cld 0.5.0' ;;\n" +
		"esac\n"
	r.write(t, files, hostBinary, binary)
	r.sum(t, files)
	r.setLatest("0.5.0")
	dir := filepath.Join(s.Root, "bin")
	path := filepath.Join(dir, "cld")
	placeCld(t, s, path, oldCld(t), 0o755)
	bash, fish := filepath.Join(s.Home, scripts["bash"]), filepath.Join(s.Home, scripts["fish"])
	updateWrite(t, s, bash, "# bash completion V2 for cld \n")
	updateWrite(t, s, fish, "# fish completion for cld \n")
	result := runCommand(t, updateCommand(s, path, r.url))
	want := sandbox.Result{
		Stdout: "Updated cld 0.4.0 to 0.5.0: " + path + "\n" +
			"Updated the completion script for fish: " + fish + "\n",
		Stderr: "cld: warning: cannot update the completion script for bash, " + bash + ": " + path +
			" completion bash: exit status 3: no completion here. Run cld setup completion bash manually\n",
	}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, binary, 0o755)
	checkContent(t, bash, "# bash completion V2 for cld \n")
	checkContent(t, fish, fakeScript("0.5.0", "completion", "fish"))
}
