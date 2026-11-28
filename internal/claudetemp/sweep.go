package claudetemp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Age is how long ago a conversation's transcript, and its temp directory, were last written
// before the sweep removes the directory that no process holds (decision 67.3).
const Age = 24 * time.Hour

// conversationID matches the name of a conversation's directory in a project of the temp root: a
// UUID, as claude names it.
var conversationID = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Sweep removes each conversation's directory in claude's temp root that no process holds, once
// it and the conversation's transcript are older than Age (decision 67.3). It returns an error for
// each directory it could not remove. Where it cannot tell which directories processes hold, as
// on a Mac, it removes none (decision 67.4).
func Sweep() []error {
	root, ok := ownRoot()
	if !ok {
		return nil
	}
	old := stale(root, time.Now().Add(-Age))
	if len(old) == 0 {
		return nil
	}
	open, known := holds(root)
	if !known {
		return nil
	}
	var failed []error
	for _, dir := range old {
		if held(dir, open) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			failed = append(failed, fmt.Errorf("cannot remove claude's temp directory %s: %w",
				dir, err))
		}
	}
	return failed
}

// ownRoot is the temp root with its parent's symlinks resolved, and whether the user owns it as a
// directory, not a symlink. Another user can plant a symlink at the root in a sticky /tmp before
// claude creates it. claude refuses such a root, and so does the sweep.
func ownRoot() (string, bool) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(Root()))
	if err != nil {
		return "", false
	}
	root := filepath.Join(parent, filepath.Base(Root()))
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || !ownedByUser(info) {
		return "", false
	}
	return root, true
}

// stale is the conversations' directories in root, PROJECT/ID, last written before cutoff, as is
// the conversation's transcript, where it has one.
func stale(root string, cutoff time.Time) []string {
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	transcripts := transcriptsDir()
	var old []string
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, project.Name()))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			dir := filepath.Join(root, project.Name(), entry.Name())
			if entry.IsDir() && conversationID.MatchString(entry.Name()) &&
				before(dir, cutoff) && transcriptBefore(transcripts, entry.Name(), cutoff) {
				old = append(old, dir)
			}
		}
	}
	return old
}

// before reports whether the file at path was last written before cutoff.
func before(path string, cutoff time.Time) bool {
	info, err := os.Lstat(path)
	return err == nil && info.ModTime().Before(cutoff)
}

// transcriptsDir is where claude keeps the transcripts, in a directory for each project:
// projects in CLAUDE_CONFIG_DIR, by default ~/.claude, or "" where cld has no home.
func transcriptsDir() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "projects")
}

// transcriptBefore reports whether conversation id's transcript, in any project of dir, was last
// written before cutoff, or there is none. Where dir is "", cld cannot tell, and reports false.
func transcriptBefore(dir, id string, cutoff time.Time) bool {
	if dir == "" {
		return false
	}
	found, err := filepath.Glob(filepath.Join(dir, "*", id+".jsonl"))
	if err != nil {
		return false
	}
	for _, transcript := range found {
		info, err := os.Stat(transcript)
		if err == nil && !info.ModTime().Before(cutoff) {
			return false
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

// held reports whether a path that a process holds, of open, is dir or under it.
func held(dir string, open []string) bool {
	for _, path := range open {
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
