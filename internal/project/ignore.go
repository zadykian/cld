package project

import (
	"bytes"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/tool"
)

// ignoreLines are the lines .gitignore needs, each with the other form git reads the same: a
// pattern with a slash before its end is anchored to the .gitignore's directory either way.
var ignoreLines = [][]string{
	{"/.claude/settings.local.json", ".claude/settings.local.json"},
	{"/.claude/plans/", ".claude/plans/"},
	{"/.claude/worktrees/", ".claude/worktrees/"},
}

// shared are the paths under .claude that Claude Code's docs have a project share through git,
// .claude/settings.json last. A directory has its slash, so that git takes it for one before it
// exists.
var shared = []string{
	".claude/commands/",
	".claude/agents/",
	".claude/skills/",
	".claude/rules/",
	".claude/hooks/",
	".claude/CLAUDE.md",
	settingsFile,
}

// ignoreChange reads .gitignore and adds the lines of ignoreLines it lacks.
func ignoreChange() (change, error) {
	ignore, err := configfile.Read(ignoreFile)
	if err != nil {
		return change{}, err
	}
	data, added := editIgnore(ignore.Data)
	save := func() error { return ignore.Write(data) }
	return change{ignoreFile, !ignore.Exists, added, save}, nil
}

// editIgnore is data, a .gitignore, with the lines of ignoreLines it lacks added at its end in its
// line endings, and those lines. A line counts with a carriage return or trailing spaces, but not
// with a trailing tab (decision 28.1).
func editIgnore(data []byte) ([]byte, []string) {
	there := make([]bool, len(ignoreLines))
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), " ")
		for at, forms := range ignoreLines {
			there[at] = there[at] || slices.Contains(forms, line)
		}
	}
	var added []string
	for at, forms := range ignoreLines {
		if !there[at] {
			added = append(added, forms[0])
		}
	}
	if len(added) == 0 {
		return data, nil
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	edited := slices.Clone(data)
	if len(edited) > 0 && !bytes.HasSuffix(edited, []byte("\n")) {
		edited = append(edited, newline...)
	}
	for _, line := range added {
		edited = append(edited, line+newline...)
	}
	return edited, added
}

// match is a path of shared that git ignores, as git was asked about it, and the pattern that
// ignores it, as git check-ignore names it: "PATTERN (FILE, line LINE)".
type match struct {
	path, by string
}

// checkIgnored warns of the paths of shared that git ignores, a warning for each pattern, and
// fails where git ignores .claude/settings.json.
func checkIgnored() error {
	var patterns []string
	var paths [][]string
	settings := ""
	for _, m := range ignored() {
		switch at := slices.Index(patterns, m.by); {
		case m.path == settingsFile:
			settings = m.by
		case at >= 0:
			paths[at] = append(paths[at], m.path)
		default:
			patterns, paths = append(patterns, m.by), append(paths, []string{m.path})
		}
	}
	for at, pattern := range patterns {
		output.Warn("git ignores " + and(paths[at]) +
			", which a project shares through git, by the pattern " + pattern)
	}
	if settings != "" {
		return fail.Runtime("git ignores " + settingsFile + " all the same, by the pattern " + settings)
	}
	return nil
}

// ignored is what git ignores of shared once .gitignore is written, in shared's order, and nothing
// where git cannot tell. Directories go with their slash and --no-index, submodules aside, and the
// rest, such as a symbolic link to a directory, as files with the index (decision 28.2).
func ignored() []match {
	git, err := tool.LookPath("git")
	if err != nil {
		return nil
	}
	var asked, dirs, files []string
	for _, path := range shared {
		name, dir := strings.CutSuffix(path, "/")
		if info, err := os.Lstat(name); dir && (err != nil || info.IsDir()) {
			dirs = append(dirs, path)
		} else {
			path = name
			files = append(files, path)
		}
		asked = append(asked, path)
	}
	by := checkIgnore(git, dirs, "--no-index")
	for _, path := range submodules(git, slices.Collect(maps.Keys(by))) {
		delete(by, path)
	}
	maps.Copy(by, checkIgnore(git, files))
	var matches []match
	for _, path := range asked {
		if pattern, ok := by[path]; ok {
			matches = append(matches, match{path, pattern})
		}
	}
	return matches
}

// checkIgnore asks git, the program at the path git, which of paths it ignores, with options such
// as --no-index. It returns each ignored path's pattern, as match has it, and none where git cannot
// tell. git names the last pattern that matches, an exception (!) where it keeps the path.
func checkIgnore(git string, paths []string, options ...string) map[string]string {
	by := map[string]string{}
	if len(paths) == 0 {
		return by
	}
	args := append([]string{"check-ignore"}, options...)
	cmd := exec.Command(git, append(args, "--verbose", "-z", "--stdin")...)
	cmd.Args[0] = "git"
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		return by
	}
	fields := strings.Split(string(out), "\x00")
	for at := 0; at+4 <= len(fields); at += 4 {
		source, line, pattern, path := fields[at], fields[at+1], fields[at+2], fields[at+3]
		if !strings.HasPrefix(pattern, "!") {
			by[path] = pattern + " (" + source + ", line " + line + ")"
		}
	}
	return by
}

// submodules are those of dirs, with their slash as ignored asks about them, that git's index
// holds as submodules: an entry of mode 160000 by the directory's name. git, the program at that
// path, runs only where dirs has any, and so only where a pattern matches.
func submodules(git string, dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	var names []string
	for _, dir := range dirs {
		names = append(names, strings.TrimSuffix(dir, "/"))
	}
	cmd := exec.Command(git, append([]string{"ls-files", "--stage", "-z", "--"}, names...)...)
	cmd.Args[0] = "git"
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var found []string
	for entry := range strings.SplitSeq(string(out), "\x00") {
		// An entry is "MODE OBJECT STAGE\tPATH", PATH from the current directory, as names are.
		info, name, _ := strings.Cut(entry, "\t")
		if strings.HasPrefix(info, "160000 ") && slices.Contains(names, name) {
			found = append(found, name+"/")
		}
	}
	return found
}
