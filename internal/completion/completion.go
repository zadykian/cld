// Package completion is cld setup completion: it sets up cld's completion in bash, zsh or fish,
// writing the script that cld completion SHELL prints where the shell reads it:
//
//   - bash: completions/cld in the first directory of $BASH_COMPLETION_USER_DIR, or else
//     $XDG_DATA_HOME/bash-completion/completions/cld, by default under ~/.local/share - the
//     directory that bash-completion 2 looks in first for a command's completion, at its first
//     TAB. That needs nothing more where ~/.bashrc loads bash-completion, as Debian's and
//     Ubuntu's do; bash-completion 1, which Homebrew installs for macOS's bash 3.2, reads no such
//     directory;
//   - zsh: $XDG_DATA_HOME/cld/zsh/_cld, by default under ~/.local/share, and the lines of
//     zshLines at the end of $ZDOTDIR/.zshrc, by default ~/.zshrc, which put that directory first
//     on $fpath and register _cld for cld with compdef. zsh has no directory it reads for every
//     user as bash-completion and fish do: the one it has, the first of $fpath, belongs to root
//     (see docs/design/findings/environment.md). The lines run compinit, which defines compdef,
//     only where nothing before them has, since a second compinit drops what compdef registered
//     after the first; with -i, which leaves out insecure directories rather than ask at each
//     start, as compinit does where Homebrew's are group-writable. A compinit after them still
//     finds _cld on $fpath, by the #compdef line the script starts with. They do nothing where the
//     script is missing, as on a machine where cld set nothing up that shares the .zshrc;
//   - fish: $XDG_CONFIG_HOME/fish/completions/cld.fish, by default under ~/.config: fish's own
//     directory of completions, the first it looks in, and where cobra's help and cld's docs had
//     users write the script by hand, so that cld replaces such a script rather than hide behind
//     it.
//
// The script is written whole, as internal/configfile writes a file: made, directories included,
// or replaced where it differs. .zshrc gets zshLines where it lacks their first line, and keeps
// everything else. Setup reads both files before it writes either.
//
// cld update runs Refresh once it has replaced cld: where a script is at the place setup writes it
// and starts as cobra's does, the new cld prints the script anew, with cld completion SHELL, which
// every release has, and Refresh writes it where it differs. A script without descriptions, as
// cld completion SHELL --no-descriptions prints it, stays one. .zshrc is left alone. cld is
// updated by then, so a script Refresh cannot write is no failure of the update's: it warns,
// naming the command that writes the script, and goes on with the next shell.
//
// The package prints nothing: Setup and Refresh return what they did, for cld to report.
package completion

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
)

// Shells are the shells setup completion sets up, in the order its help lists them.
var Shells = []string{"bash", "zsh", "fish"}

// headers are how cobra's script for each shell starts, as cld completion SHELL prints it.
var headers = map[string]string{
	"bash": "# bash completion V2 for cld ",
	"zsh":  "#compdef cld\n",
	"fish": "# fish completion for cld ",
}

// noDescriptions is in a script that cld completion SHELL --no-descriptions prints, and only there.
const noDescriptions = " __completeNoDesc "

// zshLines are the lines setup completion zsh adds to .zshrc (see the package comment). zsh finds
// the script where cld writes it, since XDG_DATA_HOME and HOME are the variables cld reads.
const zshLines = `# cld's completion, from cld setup completion zsh
if [[ -r ${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh/_cld ]]; then
  fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh" $fpath)
  (( $+functions[compdef] )) || { autoload -U compinit && compinit -i; }
  autoload -Uz _cld && compdef _cld cld
fi
`

// Script is where setup completion writes the script for shell, one of Shells.
func Script(shell string) (string, error) {
	switch shell {
	case "bash":
		for _, dir := range filepath.SplitList(os.Getenv("BASH_COMPLETION_USER_DIR")) {
			if dir != "" {
				return filepath.Join(dir, "completions", "cld"), nil
			}
		}
		dir, err := xdg("XDG_DATA_HOME", ".local/share")
		return filepath.Join(dir, "bash-completion", "completions", "cld"), err
	case "zsh":
		dir, err := xdg("XDG_DATA_HOME", ".local/share")
		return filepath.Join(dir, "cld", "zsh", "_cld"), err
	}
	dir, err := xdg("XDG_CONFIG_HOME", ".config")
	return filepath.Join(dir, "fish", "completions", "cld.fish"), err
}

// xdg is the directory the variable name holds, or else fallback in the home directory.
func xdg(name, fallback string) (string, error) {
	if dir := os.Getenv(name); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fail.Runtime("cannot find where the script goes: " + err.Error())
	}
	return filepath.Join(home, fallback), nil
}

// zshrc is the .zshrc of zsh: in $ZDOTDIR, where zsh reads its files, or else the home directory.
func zshrc() (string, error) {
	dir := os.Getenv("ZDOTDIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fail.Runtime("cannot find .zshrc: " + err.Error())
		}
		dir = home
	}
	return filepath.Join(dir, ".zshrc"), nil
}

// change is one file setup completion writes: created, changed in what ("" for the whole file),
// or neither and left as it was; write writes it.
type change struct {
	file    string
	created bool
	changed bool
	what    string
	write   func() error
}

// Setup sets up completion in shell, one of Shells, with script, what cld completion SHELL prints,
// and returns what it did.
func Setup(shell string, script []byte) (string, error) {
	path, err := Script(shell)
	if err != nil {
		return "", err
	}
	file, err := configfile.Read(path)
	if err != nil {
		return "", err
	}
	changes := []change{{path, !file.Exists, !file.Exists || !bytes.Equal(file.Data, script), "", func() error { return file.Write(script) }}}
	if shell == "zsh" {
		path, err := zshrc()
		if err != nil {
			return "", err
		}
		rc, err := configfile.Read(path)
		if err != nil {
			return "", err
		}
		data, added := addLines(rc.Data)
		changes = append(changes, change{path, !rc.Exists, added, "the lines that load the script", func() error { return rc.Write(data) }})
	}

	var written []string
	for _, c := range changes {
		if !c.changed {
			continue
		}
		if err := c.write(); err != nil {
			if len(written) > 0 {
				err = fail.Runtime(err.Error() + " (" + strings.Join(written, " and ") + " written before it)")
			}
			return "", err
		}
		written = append(written, c.file)
	}
	var report strings.Builder
	for _, c := range changes {
		switch {
		case !c.changed:
			report.WriteString("Left " + c.file + " as it was\n")
		case c.created:
			report.WriteString("Created " + c.file + "\n")
		case c.what != "":
			report.WriteString("Updated " + c.file + ": " + c.what + "\n")
		default:
			report.WriteString("Updated " + c.file + "\n")
		}
	}
	if len(written) > 0 {
		report.WriteString("Start a new " + shell + " for it to take effect\n")
	}
	return report.String(), nil
}

// addLines is .zshrc, holding data, with zshLines at its end where it lacks their first line, and
// whether they were added. A blank line goes before them in a file with anything in it.
func addLines(data []byte) ([]byte, bool) {
	marker, _, _ := strings.Cut(zshLines, "\n")
	for line := range strings.Lines(string(data)) {
		if strings.TrimRight(line, " \t\r\n") == marker {
			return data, false
		}
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	return []byte(text + zshLines), true
}

// Refresh writes anew the scripts setup completion wrote, from cld, the file cld update has just
// replaced (see the package comment), and returns what it did, a line for each script it wrote,
// and a warning for each it could not.
func Refresh(cld string) (report string, warnings []string) {
	var written strings.Builder
	for _, shell := range Shells {
		path, err := Script(shell)
		if err != nil {
			continue // No home directory, so no script where setup writes one.
		}
		wrote, err := refresh(cld, shell, path)
		if err != nil {
			warnings = append(warnings, refreshWarning(shell, path, err))
		} else if wrote {
			written.WriteString("Updated the completion script for " + shell + ": " + path + "\n")
		}
	}
	return written.String(), warnings
}

// refresh writes anew the script for shell at path from cld, where it is cobra's, and returns
// whether it wrote it.
func refresh(cld, shell, path string) (bool, error) {
	file, err := configfile.Read(path)
	if err != nil || !file.Exists || !bytes.HasPrefix(file.Data, []byte(headers[shell])) {
		return false, err
	}
	args := []string{"completion", shell}
	if bytes.Contains(file.Data, []byte(noDescriptions)) {
		args = append(args, "--no-descriptions")
	}
	script, err := exec.Command(cld, args...).Output()
	switch {
	case err != nil:
		return false, fmt.Errorf("%s %s: %s", cld, strings.Join(args, " "), reason(err))
	case len(script) == 0:
		return false, errors.New(cld + " " + strings.Join(args, " ") + " prints nothing")
	case bytes.Equal(script, file.Data):
		return false, nil
	}
	return true, file.Write(script)
}

// refreshWarning is what cld update says of err, which kept Refresh from writing the script for
// shell at path, with the command that writes it. configfile's messages name path, which this one
// names before them.
func refreshWarning(shell, path string, err error) string {
	message := err.Error()
	for _, prefix := range []string{"cannot read " + path + ": ", "cannot write " + path + ": "} {
		message = strings.TrimPrefix(message, prefix)
	}
	return fmt.Sprintf("cannot update the completion script for %s, %s: %s. Run cld setup completion %[1]s manually", shell, path, message)
}

// reason is what err says, without the operation and file a path error names, or with the status
// a command ended with and what it wrote to stderr.
func reason(err error) string {
	var (
		pathError *fs.PathError
		exitError *exec.ExitError
	)
	switch {
	case errors.As(err, &pathError):
		return pathError.Err.Error()
	case errors.As(err, &exitError):
		if stderr := strings.TrimSpace(string(exitError.Stderr)); stderr != "" {
			return exitError.ProcessState.String() + ": " + stderr
		}
		return exitError.ProcessState.String()
	}
	return err.Error()
}
