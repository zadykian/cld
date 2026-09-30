package session

// The record is what cld keeps of its sessions beside tmux, which forgets a session with its
// server: after a kill, a crash or a reboot. It is a directory, cld in $XDG_STATE_HOME or else in
// ~/.local/state (see stateDir), holding
//
//   - sessions/NAME.json for each session cld-NAME join or restore made, its entry (see entry):
//     the name, the directory claude started in and the ID of claude's conversation, as one line
//     of JSON; the file's time is the entry's. join writes it, with no ID but the one it resumes,
//     as it makes the session (see Tmux.create); then claude's SessionStart hook writes it again
//     with the ID claude gives the conversation - as it starts, and anew after /clear or /resume -
//     and its Stop and SessionEnd hooks touch it as claude answers and as the conversation ends
//     (see recordHooks);
//   - beside the entry, and gone with it (see write and Tmux.Forget): NAME.env, the claude the
//     session started and the environment its server started with, which restore starts it with
//     again (see started); NAME.run, the run mark, there while the session runs or ran when the
//     machine stopped - the tmux of join makes it once it has made the session (see setMarks),
//     claude's UserPromptSubmit hook touches it, so that its time is when the session was last
//     started or given a prompt, which restore goes by (see Tmux.Restore), and kill, the
//     interactive list's Ctrl+X, the sweep of the idle sessions and claude's own exit with status
//     0 remove it (see unmark and died); NAME.busy, the busy mark, there while claude is in a
//     turn - its UserPromptSubmit hook makes it, Stop, StopFailure and an interrupt remove it (see
//     recordHooks), and so does the tmux of join and restore once it has made the session; and
//     NAME.start, the start mark, there while a join hands tmux the session to make: the ID of the
//     process that becomes tmux, which another join waits for, and restore leaves the session to
//     (see leaveStartMark and starting), and the tmux removes once it has made the session (see
//     setMarks);
//   - indexes.json, the highest index given each NAME of NAME-INDEX, and when (see given), so that
//     an index is not given again once its entry is forgotten while claude keeps the conversation
//     of that name (see Tmux.Next);
//   - lock, which join holds from its lookup of a session, or from naming it, until it makes it,
//     and restore while it brings a session back (see Lock).
//
// An entry of a session whose server does not run is one that has ended: list shows it, and join
// brings its conversation back in the directory it ran in, by its ID (see Tmux.Join); restore does
// so for each one with a run mark, detached, as the user's systemd starts after a reboot (see
// Tmux.Restore). cld forgets an entry once it is older than expiry - but not while its session's
// server runs, whose hooks would make no entry again (see write) - and when the interactive list's
// Ctrl+X twice forgets its session (see Tmux.Forget); kill leaves it. cld reads none of claude's
// transcripts, whose format claude keeps to itself: the ID comes from the hook.
//
// The record serves the sessions, never the other way: where cld cannot write it, join warns and
// makes its session all the same, and an entry or an index cld cannot read is none.

import (
	"bytes"
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

// lockWait is how long join waits for the lock that another cld holds (see Lock), and for a
// session that another cld is starting (see starting).
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
	// Directory is the directory claude started in: where join ran, or where join resumed the
	// session's conversation, the entry's.
	Directory string `json:"directory"`
	// Conversation is the ID of the conversation claude had last in the session, as its
	// SessionStart hook wrote it, or "" before then.
	Conversation string `json:"conversation"`
}

// conversationID matches the ID of a conversation as the SessionStart hook writes it, a UUID
// (see recordHooks): an entry with another in its place has none, since join hands the ID to
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

// The files beside a session's entry, named as the entry is but for their extensions: its
// environment, its run mark, its busy mark and its start mark (see the top of this file).
const (
	environment = ".env"
	runMark     = ".run"
	busyMark    = ".busy"
	startMark   = ".start"
)

// companion is the file of session cld-SUFFIX beside its entry, with the extension ext, in the
// record's directory dir.
func companion(dir, suffix, ext string) string {
	return filepath.Join(dir, "sessions", suffix+ext)
}

// sessionFile splits the name of a file in the record's sessions directory into the NAME of the
// session it belongs to and its extension: the entry's, .json, or a companion's; false for any
// other, as a temporary file that a write cut short leaves.
func sessionFile(name string) (suffix, ext string, ok bool) {
	for _, ext := range []string{".json", environment, runMark, busyMark, startMark} {
		if suffix, found := strings.CutSuffix(name, ext); found && ValidName(suffix) {
			return suffix, ext, true
		}
	}
	return "", "", false
}

// started is what the server of a session started with, as NAME.env holds it: the claude that join
// checked, by its path, and the environment tmux started the server with, without the variables
// that name the terminal (see withoutTerminal), so that restore starts the session again as it was
// started (see Tmux.Restore). It is JSON, as a variable can hold any byte but NUL, and readable by
// the user alone, as the environment can hold secrets; the words given to claude after "--" are not
// kept.
type started struct {
	Claude      string   `json:"claude"`
	Environment []string `json:"environment"`
}

// Lock takes the record's lock, which join holds from its lookup of a session, or from naming it,
// until tmux has taken over, restore while it brings a session back (see Tmux.Restore), and the
// interactive list's forget (see Tmux.Forget), and returns what lets it go: the lock goes with cld
// as it becomes tmux, too, before tmux has made the session, which the start mark covers (see
// leaveStartMark). Next reads the record under it and create writes the entry under it, so two
// joins at once give two indexes, where both would take the same and the second fail as tmux made
// its session. A lock that another cld holds is waited for, up to lockWait; one cld cannot take -
// past that wait, or where it cannot make the record's directory - is gone without, silently: the
// entry's write says what is wrong.
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

// remember writes r, the entry of the session create is about to make, with what its server starts
// with, s; then the index its name ends in, if one does - under the lock, which the caller holds
// (see Lock) - forgets the entries and indexes that have expired - but not the entries of the
// sessions whose servers run, as runs says - and returns the file that holds r and the session's
// run mark, which the tmux that makes the session makes (see setMarks). What it cannot write is a
// warning, since the session goes on without it: where that is r, it returns "" for both, and
// where it is the environment, "" for the mark, as restore could not start the session again -
// tmux then makes none, but the pane-died hook still removes one the session had (see died).
func remember(r entry, s started, runs func(suffix string) bool) (file, run string) {
	file, run, err := write(r, s, runs)
	if err != nil {
		output.Warn("cannot record session '" + r.Name + "': " + err.Error())
	}
	return file, run
}

// write is remember's work: the file it wrote r to, or "", the run mark, or "", and what it could
// not write first. The environment goes before the session is made, and so before its run mark,
// so that restore finds the environment of any session it finds marked; what goes wrong with it
// leaves the sweep and the index to come all the same.
func write(r entry, s started, runs func(suffix string) bool) (file, run string, err error) {
	dir, err := stateDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		return "", "", reason(err)
	}
	file = entryFile(dir, r.Name)
	if err := replace(file, entryLine(r)); err != nil {
		return "", "", err
	}
	environ, err := json.Marshal(s)
	if err == nil {
		err = replace(companion(dir, r.Name, environment), append(environ, '\n'))
	}
	if err == nil {
		run = companion(dir, r.Name, runMark)
	}
	// The expired entries go, with the files beside them, and so do those files where their entry
	// has gone - forgotten while a hook made a mark - and what a write cut short left, once as old;
	// one cld cannot remove stays, read as none. So does the entry of a session whose server runs,
	// however old - one left alone for longer than expiry, say: its claude's hooks touch the entry
	// as the session is used again, and as it ends, but would make none that was gone.
	files, _ := os.ReadDir(filepath.Join(dir, "sessions"))
	stays := map[string]bool{}
	for _, other := range files {
		suffix, _, ok := sessionFile(other.Name())
		if !ok {
			if info, err := other.Info(); err != nil || time.Since(info.ModTime()) <= expiry {
				continue
			}
		} else {
			kept, known := stays[suffix]
			if !known {
				info, err := os.Stat(entryFile(dir, suffix))
				kept = err == nil && (time.Since(info.ModTime()) <= expiry || runs(suffix))
				stays[suffix] = kept
			}
			if kept {
				continue
			}
		}
		_ = os.Remove(filepath.Join(dir, "sessions", other.Name()))
	}
	highest := indexes(dir)
	if prefix, index, ok := indexOf(r.Name); ok {
		if g, found := highest[prefix]; !found || index >= g.Index {
			highest[prefix] = given{Index: index, Time: time.Now().UTC()}
		}
	}
	data, indexErr := json.Marshal(highest)
	if indexErr == nil {
		indexErr = replace(filepath.Join(dir, "indexes.json"), append(data, '\n'))
	}
	if err == nil {
		err = indexErr
	}
	return file, run, err
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
// started in dir, and its busy mark beside it. SessionStart writes the entry anew with the
// conversation's ID - session_id in the hook's input, which claude gives as JSON on a line - and
// Stop and SessionEnd touch it, so that its time is when claude last wrote the conversation in the
// session, as near as a hook tells: after a crash or a reboot, where no SessionEnd runs, the entry
// expires about when claude removes the conversation (see expiry). claude runs SessionStart as it
// starts, after /clear and /resume, with the ID of the conversation it goes on with, and after a
// compaction with the same one; Stop as it has answered a prompt; SessionEnd as a conversation
// ends: at /exit, /clear, and the SIGHUP of kill (claude 2.1.284). The ID is taken where it has
// only the characters of one, and the entry is written whole - the name and the directory as
// create wrote them in - to a temporary file renamed over it; a touch -c makes no entry that was
// forgotten.
//
// The busy mark says that claude was in a turn, for restore to have it continue the turn after a
// reboot (see Tmux.Restore): UserPromptSubmit makes it, where the entry is there, as claude takes
// a prompt, and Stop, StopFailure and an interrupt remove it, as the turn ends - an interrupt in a
// tool as PostToolUseFailure says, and one as claude writes, which no event tells, once claude,
// idle a minute, notifies idle_prompt, as for the title (see statusHooks). UserPromptSubmit also
// touches the run mark, where there is one (touch -c), so that its time is when the session was
// last given a prompt, for restore to tell a session that was idle (see Tmux.Restore). None prints
// anything, which claude would hand the model from SessionStart and UserPromptSubmit.
//
// claude waits for each, as for the title's hooks (see statusHooks), and none starts tmux: a sh
// with sed, head, printf and mv, touch, rm or grep, some milliseconds. SessionStart's stays in
// order: in the background, the write of the conversation claude goes on with after /clear could
// land after that of a /resume right after it - and claude awaits SessionStart's hooks all the
// same; so do the marks, whose order is the turn's. Stop's runs beside Stop's idle, which claude
// waits for, and each has a timeout of hookTimeout seconds. SessionEnd's has none of its own:
// claude gives its SessionEnd hooks 1.5 s together, or the longest timeout among them up to 60 s,
// as it exits, at /clear and at /resume, so that a timeout of 5 would hold claude up longer there
// (claude 2.1.284).
func recordHooks(file, suffix, dir string) map[string][]hook {
	// The line with no conversation ends in "", "}" and a newline: the hook writes what comes
	// before the second quote, then the ID, then the rest.
	head := bytes.TrimSuffix(entryLine(entry{Name: suffix, Directory: dir}), []byte("\"}\n"))
	temp := shellWord(file) + `.$$`
	start := `id=$(sed -n 's/.*"session_id" *: *"\([0-9A-Za-z-]*\)".*/\1/p' | head -n 1); ` +
		`if [ -n "$id" ]; then printf '%s%s"}\n' ` + shellWord(string(head)) + ` "$id" >` + temp +
		` && mv -f ` + temp + ` ` + shellWord(file) + `; fi`
	touch := `touch -c ` + shellWord(file)
	busy := shellWord(strings.TrimSuffix(file, ".json") + busyMark)
	run := shellWord(strings.TrimSuffix(file, ".json") + runMark)
	idle := `rm -f ` + busy
	wait := func(matcher, command string) []hook {
		return []hook{{Matcher: matcher, Hooks: []hookCommand{{Type: "command", Command: command, Timeout: hookTimeout}}}}
	}
	return map[string][]hook{
		"SessionStart":       wait("", start),
		"UserPromptSubmit":   wait("", `[ ! -e `+shellWord(file)+` ] || : >`+busy+`; touch -c `+run),
		"Stop":               wait("", idle+`; `+touch),
		"StopFailure":        wait("", idle),
		"PostToolUseFailure": wait("", `if grep -Eq '"is_interrupt": *true'; then `+idle+`; fi`),
		"Notification":       wait("idle_prompt", idle),
		"SessionEnd":         {{Hooks: []hookCommand{{Type: "command", Command: touch}}}},
	}
}

// marked reports whether session cld-SUFFIX has its run mark in the record's directory dir.
func marked(dir, suffix string) bool {
	_, err := os.Stat(companion(dir, suffix, runMark))
	return err == nil
}

// leaveStartMark writes the start mark beside the entry in file of the session that cld, attached,
// is about to have tmux make: cld's process ID, which the tmux client it becomes keeps. The lock of
// the record goes with cld as it becomes tmux, before tmux has made the session, so another join
// of the session, or restore, would find none there and make it too; the mark tells another join
// to wait for it, and restore to leave the session to it (see starting). tmux removes it once it
// has made the session (see setMarks). One cld cannot write is none: the record serves the
// sessions.
func leaveStartMark(file string) {
	_ = replace(strings.TrimSuffix(file, ".json")+startMark, []byte(strconv.Itoa(os.Getpid())+"\n"))
}

// starting reports whether another cld is starting session cld-SUFFIX: its start mark is there
// (see leaveStartMark), younger than lockWait, and names a process that runs - the tmux client
// that cld became, until tmux has made the session, or has failed to. A mark whose process has
// ended, or that is older - one tmux did not remove, as new-session failed and cut its command
// short - is none: its process ID may name another process by now.
func starting(suffix string) bool {
	dir, err := stateDir()
	if err != nil {
		return false
	}
	file := companion(dir, suffix, startMark)
	info, err := os.Stat(file)
	if err != nil || time.Since(info.ModTime()) > lockWait {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	err = unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}

// setMarks is the command, as tmux's words, that sets the marks beside the entry in file of a
// session that tmux has just made, where it follows new-session in the same tmux command: it
// removes the busy mark, as the claude it starts is in no turn yet, and the start mark, as the
// session is made (see starting), and makes the run mark, run, anew, where run is not "" - restore
// passes none, keeping the mark's time (see Tmux.Restore). tmux cuts its command short where
// new-session fails - a name taken, a terminal it cannot open (see Findings in docs/design.md) - so
// a session that was never made gets no mark, and one that restore could not bring back keeps both
// of its own; the start mark stays, naming the tmux client that has exited, which counts as none
// (see starting). It is a run-shell, which tmux waits for, some milliseconds, before what follows
// it, and before the command's client returns; claude, which takes a good part of a second to
// start, has not exited by then, so the pane-died hook removes the mark after it (see died). It
// prints nothing and exits 0 however rm and touch fare: tmux would show what it printed, and
// "returned" with a status other than 0, on claude's pane, or on the output of restore's tmux,
// which would then fail.
func setMarks(file, run string) []string {
	sh := `rm -f ` + shellWord(strings.TrimSuffix(file, ".json")+busyMark) + ` ` + shellWord(strings.TrimSuffix(file, ".json")+startMark)
	if run != "" {
		sh += `; touch ` + shellWord(run)
	}
	return []string{"run-shell", unexpanded(`{ ` + sh + `; } 2>/dev/null || true`)}
}

// unmark is the sh command that removes the run mark of session cld-SUFFIX, which kill, the list's
// Ctrl+X and the sweep of the idle sessions run with tmux's run-shell in the tmux command that ends
// the session, before its kill-session: restore then leaves the session ended. tmux serves other
// clients while sh runs, and the session holds its name meanwhile, so that none of the name is
// made on the server that the kill-server then ends, where its own mark would outlive it; and a
// session made again under the name - on a server that starts only once this one has gone - keeps
// the mark its own tmux makes, where a removal after the kill could take it. It prints nothing and
// exits 0 however rm fares: a mark cld cannot remove stays, silently, as the session ends all the
// same. "" where cld has no record's directory. It goes as run-shell's argument, a format, with
// every "#" doubled (see unexpanded).
func unmark(suffix string) string {
	dir, err := stateDir()
	if err != nil {
		return ""
	}
	return unexpanded(`rm -f ` + shellWord(companion(dir, suffix, runMark)) + ` 2>/dev/null || true`)
}

// readStarted reads what the server of session cld-SUFFIX started with, from the record's
// directory dir (see started).
func readStarted(dir, suffix string) (started, error) {
	data, err := os.ReadFile(companion(dir, suffix, environment))
	if err != nil {
		return started{}, reason(err)
	}
	var s started
	if err := json.Unmarshal(data, &s); err != nil || !filepath.IsAbs(s.Claude) {
		return started{}, errors.New(companion(dir, suffix, environment) + ": not what cld writes")
	}
	return s, nil
}

// enter makes the directory the session of entry r ran in the current one, where join starts
// claude to resume the session's conversation, as tmux will: PWD names it too, as a shell's cd
// would. A directory that no longer exists, or cannot be entered, is refused, with the advice to
// resume the conversation from the current directory by its ID, or else by its name (see
// enterError).
func enter(r entry) error {
	if err := enterable(r); err != nil {
		return err
	}
	if err := os.Chdir(r.Directory); err != nil {
		return enterError(r, err)
	}
	if err := os.Setenv("PWD", r.Directory); err != nil {
		return fail.Runtime(err.Error())
	}
	return nil
}

// enterable is nil where the directory of entry r can be entered, and otherwise why join refuses
// to resume the session's conversation there.
func enterable(r entry) error {
	if _, err := os.Stat(r.Directory); err != nil {
		return enterError(r, err)
	}
	if err := unix.Access(r.Directory, unix.X_OK); err != nil {
		return enterError(r, err)
	}
	return nil
}

// enterError refuses the directory of entry r, which err says cannot be entered, with the join
// that resumes the conversation in the current directory instead: by the entry's ID, or else by
// the session's name.
func enterError(r entry, err error) error {
	conversation := r.Conversation
	if conversation == "" {
		conversation = "cld-" + r.Name
	}
	advice := "; resume it from here with cld join " + Options(r.Name) + " --resume " + conversation
	if errors.Is(err, fs.ErrNotExist) {
		return &fail.Error{Status: 1, Message: "session '" + r.Name + "' ran in " + r.Directory + ", which no longer exists", Advice: advice}
	}
	var errno unix.Errno
	if errors.As(err, &errno) {
		err = errno
	}
	return &fail.Error{Status: 1, Message: "cannot enter " + r.Directory + ", where session '" + r.Name + "' ran: " + err.Error(), Advice: advice}
}
