package session

// The record is what cld keeps of its sessions beside tmux, which forgets a session with its
// server (decision 40). A session's entry, sessions/NAME.json, has its environment and its run,
// busy and start marks beside it (decisions 48 and 50.4). docs/design/overview.md follows the
// record through a session's life.

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// expiry is how long cld keeps an entry, and an index given: claude's default cleanupPeriodDays,
// after which claude removes a conversation not written since (decision 40.6).
const expiry = 30 * 24 * time.Hour

// lockWait is how long join waits for the lock that another cld holds, and for a session that
// another cld is starting (decisions 40.6 and 50.4).
const lockWait = 10 * time.Second

// Ended is the state of a session that has ended: one of cld's record whose server does not run
// (see Sessions).
const Ended = "ended"

// entry is the entry of a session in cld's record, as sessions/NAME.json holds it. The file's time
// is the entry's, that of its last write or touch (decision 40.1).
type entry struct {
	// Name is the session's NAME, without "cld-".
	Name string `json:"name"`
	// Directory is the directory claude started in: where join ran, or the entry's where join
	// resumed the session's conversation.
	Directory string `json:"directory"`
	// Conversation is the ID of the conversation claude had last in the session, as its
	// SessionStart hook wrote it, or "" before then.
	Conversation string `json:"conversation"`
}

// conversationID matches the ID of a conversation as the SessionStart hook writes it, a UUID. An
// entry with another in its place has none, since join hands the ID to claude as an argument.
var conversationID = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]*$`)

// given is the highest index given a NAME, and when, in indexes.json.
type given struct {
	Index int       `json:"index"`
	Time  time.Time `json:"time"`
}

// started is what the server of a session started with, as NAME.env holds it (decision 48.4). It
// holds the claude that join checked, by its path, and tmux's environment without the terminal's
// variables.
type started struct {
	Claude      string   `json:"claude"`
	Environment []string `json:"environment"`
}

// The files beside a session's entry, named as the entry is but for their extensions.
const (
	environment = ".env"
	runMark     = ".run"
	busyMark    = ".busy"
	startMark   = ".start"
)

// stateDir is the directory of cld's record: cld in $XDG_STATE_HOME or, where that is unset or
// relative, in ~/.local/state (decision 40.1). A relative path would name another directory in
// claude's hooks, which run in claude's directory.
func stateDir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Abs(filepath.Join(base, "cld"))
}

// entryFile is the file of session cld-SUFFIX's entry in the record's directory dir.
func entryFile(dir, suffix string) string {
	return filepath.Join(dir, "sessions", suffix+".json")
}

// companion is the file of session cld-SUFFIX beside its entry, with the extension ext, in the
// record's directory dir.
func companion(dir, suffix, ext string) string {
	return filepath.Join(dir, "sessions", suffix+ext)
}

// sessionFile splits the name of a file in the record's sessions directory into the NAME of its
// session and its extension, the entry's or a companion's. It returns false for any other file, as
// a temporary file that a write cut short leaves.
func sessionFile(name string) (suffix, ext string, ok bool) {
	for _, ext := range []string{".json", environment, runMark, busyMark, startMark} {
		if suffix, found := strings.CutSuffix(name, ext); found && ValidName(suffix) {
			return suffix, ext, true
		}
	}
	return "", "", false
}

// Lock takes the record's lock and returns what lets it go (decision 40.6). The lock goes with
// cld's exec of tmux too, before tmux has made the session, which the start mark covers (decision
// 50.4). A lock held elsewhere is waited for up to lockWait. One cld cannot take is gone without,
// silently, as the entry's write says what is wrong.
func Lock() (unlock func()) {
	dir, err := stateDir()
	if err != nil {
		return func() {}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return func() {}
	}
	file, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return func() {}
	}
	deadline := time.Now().Add(lockWait)
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = file.Close() } //nolint:errcheck // the close lets the lock go regardless
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = file.Close() //nolint:errcheck // a file only opened, to lock
			return func() {}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
