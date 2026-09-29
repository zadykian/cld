package session

// The record is what cld keeps of its sessions beside tmux, which forgets a session with its
// server: after a kill, a crash or a reboot. It is a directory, cld in $XDG_STATE_HOME or else in
// ~/.local/state (see stateDir), holding
//
//   - sessions/NAME.json for each session cld-NAME new or resume made, its entry (see entry): the
//     name, the directory claude started in and the ID of claude's conversation, as one line of
//     JSON; the file's time is the entry's. new and resume write it, with no ID but the one resume
//     resumes, as they make the session (see Tmux.create); then claude's SessionStart hook
//     writes it again with the ID claude gives the conversation - as it starts, and anew after
//     /clear or /resume - and its Stop and SessionEnd hooks touch it as claude answers and as the
//     conversation ends (see recordHooks);
//   - indexes.json, the highest index given each NAME of NAME-INDEX, and when (see given), so that
//     an index is not given again once its entry is forgotten while claude keeps the conversation
//     of that name (see Tmux.Next);
//   - lock, which new and resume hold from naming a session until they make it (see Lock).
//
// An entry of a session whose server does not run is one that has ended: list shows it, and
// resume brings its conversation back in the directory it ran in, by its ID (see Tmux.Resume).
// cld forgets an entry once it is older than expiry - but not while its session's server runs,
// whose hooks would make no entry again (see write) - and when the interactive list's Ctrl+X
// twice forgets its session (see Tmux.Forget); kill leaves it. cld reads none of claude's
// transcripts, whose format claude keeps to itself: the ID comes from the hook.
//
// The record serves the sessions, never the other way: where cld cannot write it, new and resume
// warn and make their session all the same, and an entry or an index cld cannot read is none.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

// expiry is how long cld keeps an entry, and an index given: claude's default cleanupPeriodDays,
// 30, after which claude removes a conversation not written since (claude 2.1.284). A setting of
// cleanupPeriodDays of the user's own is not read. An entry's time follows its conversation's
// writes as the hooks touch it, as claude answers (see recordHooks).
const expiry = 30 * 24 * time.Hour

// lockWait is how long new and resume wait for the lock that another cld holds (see Lock).
const lockWait = 10 * time.Second

// Ended is the state of a session that has ended: one of cld's record whose server does not run
// (see Sessions).
const Ended = "ended"

// entry is the entry of a session in cld's record, as sessions/NAME.json holds it; the file's time
// is the entry's: when it was last written or touched. The record is described at the top of this
// file.
type entry struct {
	// Name is the session's NAME, without "cld-".
	Name string `json:"name"`
	// Directory is the directory claude started in: where new or resume ran, or for a resume of
	// an entry, the entry's.
	Directory string `json:"directory"`
	// Conversation is the ID of the conversation claude had last in the session, as its
	// SessionStart hook wrote it, or "" before then.
	Conversation string `json:"conversation"`
}

// conversationID matches the ID of a conversation as the SessionStart hook writes it, a UUID
// (see recordHooks): an entry with another in its place has none, since resume hands the ID to
// claude as an argument.
var conversationID = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]*$`)

// given is the highest index given a NAME, and when, in indexes.json.
type given struct {
	Index int       `json:"index"`
	Time  time.Time `json:"time"`
}

// stateDir is the directory of cld's record: cld in $XDG_STATE_HOME or, where that is unset or
// not a whole path, in ~/.local/state, as the XDG Base Directory Specification has it. A relative
// path would name another directory in each directory cld runs in, and claude's hooks run in
// claude's.
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

// Lock takes the record's lock, which new and resume hold from naming their session until tmux
// has taken over, and returns what lets it go: the lock goes with cld as it becomes tmux, too.
// Next reads the record under it and create writes the entry under it, so two new at once give
// two indexes, where both would take the same and the second fail as tmux made its session. A
// lock that another cld holds is waited for, up to lockWait; one cld cannot take - past that
// wait, or where it cannot make the record's directory - is gone without, silently: the entry's
// write says what is wrong.
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
			return func() { _ = file.Close() }
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = file.Close()
			return func() {}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// recorded is the entry of session cld-SUFFIX, and whether there is one: none where it has
// expired, or cannot be read.
func recorded(suffix string) (entry, bool) {
	dir, err := stateDir()
	if err != nil {
		return entry{}, false
	}
	return readEntry(entryFile(dir, suffix), suffix)
}

// readEntry reads the entry of session cld-SUFFIX from file. The name in it is the session's: a
// socket directory that ignores case would read another session's file for a name in other
// letters, as it reaches another session's server (see lingering).
func readEntry(file, suffix string) (entry, bool) {
	info, err := os.Stat(file)
	if err != nil || time.Since(info.ModTime()) > expiry {
		return entry{}, false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return entry{}, false
	}
	var r entry
	if err := json.Unmarshal(data, &r); err != nil || r.Name != suffix || !filepath.IsAbs(r.Directory) {
		return entry{}, false
	}
	if !conversationID.MatchString(r.Conversation) {
		r.Conversation = ""
	}
	return r, true
}

// entries are the entries of the record that have not expired, in the order of their names.
func entries() []entry {
	dir, err := stateDir()
	if err != nil {
		return nil
	}
	files, err := os.ReadDir(filepath.Join(dir, "sessions"))
	if err != nil {
		return nil
	}
	var found []entry
	for _, file := range files {
		suffix, ok := strings.CutSuffix(file.Name(), ".json")
		if !ok || !ValidName(suffix) {
			continue
		}
		if r, ok := readEntry(entryFile(dir, suffix), suffix); ok {
			found = append(found, r)
		}
	}
	return found
}

// indexes are the highest indexes given, by the start of the names they follow - "NAME-", or ""
// for the names that are an index alone - less those given longer ago than expiry.
func indexes(dir string) map[string]given {
	data, err := os.ReadFile(filepath.Join(dir, "indexes.json"))
	if err != nil {
		return map[string]given{}
	}
	all := map[string]given{}
	if json.Unmarshal(data, &all) != nil {
		return map[string]given{}
	}
	for prefix, g := range all {
		if time.Since(g.Time) > expiry {
			delete(all, prefix)
		}
	}
	return all
}

// indexOf splits name into the start that Next takes, "" or ending in "-", and the index that
// follows it; false where no index ends the name.
func indexOf(name string) (prefix string, index int, ok bool) {
	prefix = strings.TrimRight(name, "0123456789")
	if prefix == name || prefix != "" && !strings.HasSuffix(prefix, "-") {
		return "", 0, false
	}
	// An index too large for an int, or the largest, which no index is above, is none.
	index, err := strconv.Atoi(name[len(prefix):])
	if err != nil || index == math.MaxInt {
		return "", 0, false
	}
	return prefix, index, true
}

// remember writes r, the entry of the session create is about to make, with the index its name
// ends in, if one does - under the lock, which the caller holds (see Lock) - forgets the entries
// and indexes that have expired - but not the entries of the sessions whose servers run, as runs
// says - and returns the file that holds r. What it cannot write is a warning, since the session
// goes on without it; where that is r, it returns "".
func remember(r entry, runs func(suffix string) bool) string {
	file, err := write(r, runs)
	if err != nil {
		output.Warn("cannot record session '" + r.Name + "': " + err.Error())
	}
	return file
}

// write is remember's work: the file it wrote r to, or "", and what it could not write.
func write(r entry, runs func(suffix string) bool) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		return "", reason(err)
	}
	file := entryFile(dir, r.Name)
	if err := replace(file, entryLine(r)); err != nil {
		return "", err
	}
	// The expired entries go, and what a write cut short left; one cld cannot remove stays, read
	// as none. So does the entry of a session whose server runs, however old - one left alone for
	// longer than expiry, say: its claude's hooks touch the entry as the session is used again,
	// and as it ends, but would make none that was gone.
	files, _ := os.ReadDir(filepath.Join(dir, "sessions"))
	for _, other := range files {
		info, err := other.Info()
		if err != nil || time.Since(info.ModTime()) <= expiry {
			continue
		}
		if suffix, ok := strings.CutSuffix(other.Name(), ".json"); ok && ValidName(suffix) && runs(suffix) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, "sessions", other.Name()))
	}
	highest := indexes(dir)
	if prefix, index, ok := indexOf(r.Name); ok {
		if g, found := highest[prefix]; !found || index >= g.Index {
			highest[prefix] = given{Index: index, Time: time.Now().UTC()}
		}
	}
	data, err := json.Marshal(highest)
	if err != nil {
		return "", err
	}
	return file, replace(filepath.Join(dir, "indexes.json"), append(data, '\n'))
}

// entryLine is r as sessions/NAME.json holds it: a line of JSON, without HTML's escapes, as the
// SessionStart hook writes it too (see recordHooks).
func entryLine(r entry) []byte {
	var line bytes.Buffer
	encoder := json.NewEncoder(&line)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(r)
	return line.Bytes()
}

// replace replaces file with data: written to a temporary file beside it, then renamed over it,
// so that no reader - another cld, the hooks - sees it half written.
func replace(file string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*")
	if err != nil {
		return reason(err)
	}
	_, err = temp.Write(data)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), file)
	}
	if err != nil {
		_ = os.Remove(temp.Name())
		return reason(err)
	}
	return nil
}

// reason is err as the system says it, with the path it names.
func reason(err error) error {
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		return errors.New(pathError.Path + ": " + pathError.Err.Error())
	}
	return err
}

// recordHooks are the hooks that keep session cld-SUFFIX's entry, as file holds it, for claude
// started in dir: SessionStart writes the entry anew with the conversation's ID - session_id in
// the hook's input, which claude gives as JSON on a line - and touch touches it, for Stop and
// SessionEnd, so that its time is when claude last wrote the conversation in the session, as near
// as a hook tells: after a crash or a reboot, where no SessionEnd runs, the entry expires about
// when claude removes the conversation (see expiry). claude runs SessionStart as it starts, after
// /clear and /resume, with the ID of the conversation it goes on with, and after a compaction with
// the same one; Stop as it has answered a prompt; SessionEnd as a conversation ends: at /exit,
// /clear, and the SIGHUP of kill (claude 2.1.284). The ID is taken where it has only the
// characters of one, and the entry is written whole - the name and the directory as create wrote
// them in - to a temporary file renamed over it; a touch -c makes no entry that was forgotten.
// None prints anything, which claude would hand the model from SessionStart.
//
// claude waits for each, as for the title's hooks (see statusHooks), and none starts tmux: a sh
// with sed, head, printf and mv, or touch, some milliseconds. SessionStart's stays in order: in
// the background, the write of the conversation claude goes on with after /clear could land after
// that of a /resume right after it - and claude awaits SessionStart's hooks all the same. Stop's
// touch runs beside Stop's idle, which claude waits for. Both have a timeout of hookTimeout
// seconds. SessionEnd's has none of its own: claude gives its SessionEnd hooks 1.5 s together,
// or the longest timeout among them up to 60 s, as it exits, at /clear and at /resume, so that a
// timeout of 5 would hold claude up longer there (claude 2.1.284).
func recordHooks(file, suffix, dir string) (start, touch string) {
	// The line with no conversation ends in "", "}" and a newline: the hook writes what comes
	// before the second quote, then the ID, then the rest.
	head := bytes.TrimSuffix(entryLine(entry{Name: suffix, Directory: dir}), []byte("\"}\n"))
	temp := shellWord(file) + `.$$`
	start = `id=$(sed -n 's/.*"session_id" *: *"\([0-9A-Za-z-]*\)".*/\1/p' | head -n 1); ` +
		`if [ -n "$id" ]; then printf '%s%s"}\n' ` + shellWord(string(head)) + ` "$id" >` + temp +
		` && mv -f ` + temp + ` ` + shellWord(file) + `; fi`
	touch = `touch -c ` + shellWord(file)
	return start, touch
}

// EnterRecorded makes the directory session cld-SUFFIX ran in, as its entry has it, the current
// one, where resume without SESSION starts claude, as tmux will: PWD names it too, as a shell's
// cd would. Without an entry it changes nothing, and resume goes by the session's name where it
// runs. A directory that no longer exists, or cannot be entered, is refused, with the advice to
// resume the conversation from the current directory by its ID, or else by its name - but a
// session that runs, or whose server does, is refused as create refuses it (see occupied): the
// advice would fail as taken.
func (t *Tmux) EnterRecorded(suffix string) error {
	r, ok := recorded(suffix)
	if !ok {
		return nil
	}
	err := enterable(r)
	if err == nil {
		if err = os.Chdir(r.Directory); err == nil {
			return os.Setenv("PWD", r.Directory)
		}
		err = enterError(r, err)
	}
	if refused := t.occupied(context.Background(), suffix); refused != nil {
		return refused
	}
	return err
}

// enterable is nil where the directory of entry r can be entered, and otherwise why resume refuses
// it.
func enterable(r entry) error {
	if _, err := os.Stat(r.Directory); err != nil {
		return enterError(r, err)
	}
	if err := unix.Access(r.Directory, unix.X_OK); err != nil {
		return enterError(r, err)
	}
	return nil
}

// enterError refuses the directory of entry r, which err says cannot be entered.
func enterError(r entry, err error) error {
	conversation := r.Conversation
	if conversation == "" {
		conversation = "cld-" + r.Name
	}
	advice := "; resume it from here with cld resume " + Options(r.Name) + " " + conversation
	if errors.Is(err, fs.ErrNotExist) {
		return &fail.Error{Status: 1, Message: "session '" + r.Name + "' ran in " + r.Directory + ", which no longer exists", Advice: advice}
	}
	var errno unix.Errno
	if errors.As(err, &errno) {
		err = errno
	}
	return &fail.Error{Status: 1, Message: "cannot enter " + r.Directory + ", where session '" + r.Name + "' ran: " + err.Error(), Advice: advice}
}
