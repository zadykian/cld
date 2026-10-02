package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zadykian/cld/internal/tool"
)

// validName matches a session NAME, in ASCII whatever the locale. cld refuses any other name
// rather than change it, as tmux would change a "." or ":" and claude would not (decision 1).
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// MaxName is the length of the longest NAME, so that the path of server cld-NAME's socket fits in
// sun_path in tmux's default directories (decision 13.2). Under a longer TMUX_TMPDIR, tmux fails
// with "File name too long", and cld with tmux's message (see noServer).
const MaxName = 64

// ValidName reports whether name is a valid session NAME: its characters, and at most MaxName of
// them.
func ValidName(name string) bool { return len(name) <= MaxName && validName.MatchString(name) }

// Home is where the sessions made in the current directory belong, and where their NAME comes from
// by default: the git repository the directory is in or, outside one, the directory itself. join
// records Dir as the session's @cld-home (decision 37).
type Home struct {
	// Dir is the directory whose name DefaultName takes - the repository's (see repository), or
	// the current directory - by the path that gives the name; "" where there is none.
	Dir string
	// Repository is whether Dir is a git repository's.
	Repository bool
}

// Takes reports whether join, detach and kill, run in home h, take a session whose @cld-home is
// made: where either is "", or both are one directory. Paths that differ are compared as files,
// since git and a file system that ignores case can name one directory by two (decision 37.2).
func (h Home) Takes(made string) bool {
	if made == "" || h.Dir == "" || made == h.Dir {
		return true
	}
	here, err := os.Stat(h.Dir)
	if err != nil {
		return false
	}
	there, err := os.Stat(made)
	return err == nil && os.SameFile(here, there)
}

// DefaultName is the NAME of NAME-SUFFIX where -n gives none, and the home it comes from: the
// name of the repository (see repository) or of the current directory, made a NAME (see asName).
// Where nothing is left of the name, as for "/", the NAME is "", and SUFFIX is the whole name.
func DefaultName() (string, Home) {
	name, dir, found := repository()
	if !found {
		// Getwd gives the directory as PWD names it: the name pwd shows, not that of the
		// directory a symbolic link leads to.
		var err error
		if dir, err = os.Getwd(); err != nil {
			return "", Home{}
		}
		name = filepath.Base(dir)
	}
	return asName(name), Home{Dir: dir, Repository: found}
}

// Options is how -n and -s name session NAME on cld's command line: split at its last "-", where
// both parts are NAMEs, and otherwise -s NAME alone (decision 24.6).
func Options(name string) string {
	if prefix, suffix, ok := split(name); ok {
		return "-n " + prefix + " -s " + suffix
	}
	return "-s " + name
}

// notInName is a run of the characters a NAME cannot have.
var notInName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// asName is name as the start of a NAME: each run of the characters a NAME cannot have becomes
// "-", and "-" and "_" go from either end. my.site is my-site, and "/" nothing at all.
func asName(name string) string {
	return strings.Trim(notInName.ReplaceAllString(name, "-"), "-_")
}

// repository is the name and the directory of the git repository the current directory is in,
// and whether there is one: the directory holding the common .git, or else the git directory of a
// bare repository's worktree or a submodule, named without ".git" (decision 24.3). Outside a work
// tree, without git, or where git fails, there is none.
func repository() (name, dir string, found bool) {
	git, err := tool.Command("git", "rev-parse", "--is-inside-work-tree", "--git-common-dir")
	if err != nil {
		return "", "", false
	}
	git.Stderr = nil
	out, err := git.Output()
	if err != nil {
		return "", "", false
	}
	inside, common, _ := strings.Cut(strings.TrimRight(string(out), "\n"), "\n")
	if inside != "true" || common == "" {
		return "", "", false
	}
	if !filepath.IsAbs(common) {
		current, err := os.Getwd()
		if err != nil {
			return "", "", false
		}
		common = filepath.Join(current, common)
	}
	common = filepath.Clean(common)
	if filepath.Base(common) == ".git" {
		dir = filepath.Dir(common)
		return filepath.Base(dir), dir, true
	}
	return strings.TrimSuffix(filepath.Base(common), ".git"), common, true
}

// inWorkTree reports whether the current directory is in a git work tree.
func inWorkTree() bool {
	git, err := tool.Command("git", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	git.Stderr = nil
	out, _ := git.Output() //nolint:errcheck // git fails outside a work tree
	return strings.TrimRight(string(out), "\n") == "true"
}
