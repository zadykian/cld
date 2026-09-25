// Package session is cld's tmux side: it runs Claude Code in named sessions, each on a private
// tmux server of its own.
//
// `cld new -n NAME` creates the tmux session "cld-NAME" (NAME defaults to "main") on the server
// cld-NAME (tmux -L cld-NAME), running `claude --name cld-NAME` in the current directory, with
// Remote Control on from the start, and attaches to it; with -w claude also gets --worktree NAME,
// makes git worktree NAME from HEAD or reopens it, and works there.
// `cld join -n NAME` attaches to the session again, `cld kill -n NAME` ends it with its server
// (see Tmux.Kill), and `cld list` shows the sessions, asking each server for its own (see
// Tmux.Sessions) - on a terminal as a list to pick one from with the arrow keys, to join with
// Enter, as join does, or to kill with Ctrl+X pressed twice, as kill does (see internal/picker).
//
// cld looks for session cld-NAME on server cld-NAME only, and for no other session there.
// Whatever claude runs inherits TMUX, which takes a bare tmux to claude's own server: a session
// made that way has another name - cld-NAME is taken - so it is no session of cld's, and kill
// ends it with the server. The server of a session also gives its claude the environment of the
// shell that ran cld new: tmux starts a pane with the environment of the client that started the
// server, but for PATH and the update-environment variables, so on one server for every session
// each claude had the first one's. new refuses a NAME whose server outlives its session - claude
// exited, and the tmux sessions it made keep the server running - rather than start claude there
// with that server's environment, and join and kill refuse it the same way (see lingering). A
// private server (-f /dev/null: no ~/.tmux.conf) keeps these options away from any other tmux use:
//
//   - extended-keys on: tmux answers no kitty keyboard query, so claude falls back to
//     modifyOtherKeys, which tmux forwards only when this is on (Shift+Enter and friends)
//   - the extkeys terminal feature for xterm*: tmux asks the terminal for modified keys only when
//     it knows the terminal supports them, and it does not recognise every terminal that does;
//     Claude Code's docs recommend this for tmux. It goes to a fixed index past tmux's defaults:
//     set -a would add another copy every time cld sets it on a server that has it, as two
//     cld new -n NAME at once do
//   - mouse on, focus-events on: claude probes both and hints when they are off. With the mouse
//     on, the wheel over a program that draws in the main screen without the mouse - claude
//     outside fullscreen, a shell - scrolls the pane's history; claude's fullscreen transcript
//     gets the wheel either way, as tmux passes claude's own mouse reporting on to the terminal
//   - allow-passthrough on: claude wraps its notifications and OSC 52 copies in tmux passthrough
//   - status off: claude keeps the whole tab
//   - prefix C-q: claude binds C-b (background a task) and nearly every other Ctrl key, but not
//     C-q; detach is C-q d, and C-q C-q sends a C-q through
//   - remain-on-exit failed: a claude that fails - at startup, say, for a worktree in a directory
//     it does not trust - leaves its pane on screen with its message, instead of taking both
//     away; /exit and claude's other ways out exit with status 0. An empty remain-on-exit-format
//     keeps tmux from scrolling the pane for its own line, which would push a short error at the
//     top out of sight; the pane-died hook shows how to end the session on the message line
//     instead, until a key is pressed. It names the session through its one window, named NAME:
//     the hook's formats know the pane and its window, not the session. The hook shows it only to
//     a terminal on that window: tmux would show it on the terminal of another session on the
//     server - one claude made - or with none attached keep it and show it in view-mode over the
//     session a terminal attaches to next, which then takes no keys until q; join shows it
//     instead. These go to claude's window only, not the server, so that the sessions claude
//     makes there close as tmux would close them (see Tmux.New)
//
// join attaches with -d, detaching other clients. tmux keeps claude's title changes to the pane
// (set-titles is off), so the tab keeps the session name. claude trusts TERMINAL_EMULATOR over
// TERM_PROGRAM=tmux, and its environment comes from the client that started its server: a
// session created in the JetBrains terminal would keep its claude, and whatever claude starts
// through tmux, acting as if in JediTerm (extended keys off, so Shift+Enter submits) even when
// joined from iTerm2 - hence new leaves TERMINAL_EMULATOR out of the environment it runs tmux
// with.
package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// validName matches a session NAME, in ASCII whatever the locale. tmux turns "." and ":" into
// "_" in session names and claude's --name would keep them, so names are validated instead of
// silently diverging.
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// MaxName is the length of the longest NAME. The path of server cld-NAME's socket has to fit in
// sun_path, 104 bytes on macOS and 108 on Linux with the NUL that ends it: in tmux's default
// directories, /private/tmp/tmux-UID and /tmp/tmux-UID, that leaves room for about 77 characters
// of NAME on macOS and 88 on Linux. Under a longer TMUX_TMPDIR a shorter NAME can overflow it too;
// tmux then fails with "File name too long", and cld with tmux's message (see noServer).
const MaxName = 64

// ValidName reports whether name is a valid session NAME: its characters, and at most MaxName of
// them.
func ValidName(name string) bool { return len(name) <= MaxName && validName.MatchString(name) }

// Tmux is the tmux cld runs, found and checked by Check.
type Tmux struct {
	path string
}

// The oldest tmux and claude cld runs. tmux's is the one the tests run on; claude's is the first
// release that takes everything new passes it and does what cld relies on. Both are raised by
// hand (see docs/design.md, decision 6).
var (
	minTmux   = version{3, 7}
	minClaude = version{2, 1, 222}
)

// tmuxVersion and claudeVersion match the start of a version that tmux -V and claude --version
// report: "3.7c", "2.1.282 (Claude Code)".
var (
	tmuxVersion   = regexp.MustCompile(`^([0-9]+)\.([0-9]+)`)
	claudeVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
)

// version is the numbers of a version, the most significant first.
type version []int

// parseVersion reads a version from the start of text, as pattern matches it; false when text
// does not start with one.
func parseVersion(pattern *regexp.Regexp, text string) (version, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return nil, false
	}
	var v version
	for _, digits := range match[1:] {
		v = append(v, number(digits))
	}
	return v, true
}

// before reports whether v is older than minimum, comparing them number by number.
func (v version) before(minimum version) bool { return slices.Compare(v, minimum) < 0 }

func (v version) String() string {
	numbers := make([]string, len(v))
	for i, n := range v {
		numbers[i] = strconv.Itoa(n)
	}
	return strings.Join(numbers, ".")
}

// Check makes the checks every command makes before it runs tmux, in this order: tmux, then
// each of tools, on the PATH (see lookPath), and tmux's version. A tmux -V that fails ends cld
// with its status, after its own message (see exitStatus).
func Check(tools ...string) (*Tmux, error) {
	path, err := lookPath("tmux")
	if err != nil {
		return nil, fail.Runtime("tmux is not installed")
	}
	for _, name := range tools {
		if _, err := lookPath(name); err != nil {
			return nil, fail.Runtime(name + " is not installed")
		}
	}
	t := &Tmux{path: path}
	// Only the major and minor version count: a letter marks a bug-fix release, so 3.7 and 3.7c
	// alike are 3.7. Development builds pass: "tmux next-3.9" reads as 3.9, "tmux 3.8-rc2" as
	// 3.8, and "tmux master" has no version to compare.
	out, err := t.command("-V").Output()
	if err != nil {
		return nil, t.exitStatus(err)
	}
	found := strings.TrimRight(string(out), "\n")
	reported := strings.TrimPrefix(found[strings.LastIndex(found, " ")+1:], "next-")
	if v, ok := parseVersion(tmuxVersion, reported); ok && v.before(minTmux) {
		return nil, fail.Runtime(fmt.Sprintf("tmux %s or newer is required, found '%s'", minTmux, found))
	}
	return t, nil
}

// Claude is the claude new starts: the one CheckClaude found and checked.
type Claude struct {
	path string
}

// CheckClaude checks the version of the claude on the PATH, for the commands that start it: an
// older claude lacks a flag or a setting cld passes it, or behaves otherwise than cld relies on.
// It reads the X.Y.Z that claude --version starts with. Output that starts otherwise passes, as a
// tmux development build does, so that a new format locks no one out; there is no upper bound. A
// claude --version that fails is refused with status 1 and what it printed: a claude that cannot
// report its version is unlikely to start. One that cannot run at all ends cld as a tmux that
// cannot run does (see cannotRun): its version is not what is wrong.
func CheckClaude() (*Claude, error) {
	path, err := lookPath("claude")
	if err != nil {
		return nil, fail.Runtime("claude is not installed")
	}
	// claude --version runs as tmux starts claude (see Tmux.New): the file lookPath found, by its
	// path, in the current directory, with no input - so a version manager's shim, mise's say,
	// runs the claude that directory pins. A directory that has been removed, or that cannot be
	// entered, is refused first, as new refuses it: claude --version fails in the one (claude
	// 2.1.282) and does not start in the other.
	dir, err := workingDirectory()
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	run := func(name string, args ...string) error {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		// What claude printed is in once it has exited. A process it leaves in the background - a
		// wrapper's update check, say - can hold its stdout and stderr open for as long as it
		// runs; cld waits for that no longer than this, and ends the pipes.
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); !errors.Is(err, exec.ErrWaitDelay) {
			return err
		}
		return nil
	}
	err = run(path, "--version")
	// tmux starts claude with execvp, which runs a file the system will not execute with /bin/sh,
	// as a shell does (glibc's and macOS's); os/exec does not. That is how a script without #!
	// runs. A binary - for another machine, or cut short - is no script, and a shell such as bash
	// refuses to read one as commands: such a claude cannot run (see cannotRun).
	if errors.Is(err, syscall.ENOEXEC) && !binary(path) {
		err = run("/bin/sh", path, "--version")
	}
	required := "claude " + minClaude.String() + " or newer is required"
	output := strings.TrimRight(stdout.String(), "\n")
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		how := "status " + strconv.Itoa(exit.ExitCode())
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			how = "signal " + strconv.Itoa(int(status.Signal()))
		}
		message := required + ", but claude --version exited with " + how
		if printed := strings.Trim(output+"\n"+strings.TrimRight(stderr.String(), "\n"), "\n"); printed != "" {
			message += ": " + printed
		}
		return nil, fail.Runtime(message)
	case err != nil:
		return nil, cannotRun(path, err)
	}
	if v, ok := parseVersion(claudeVersion, output); ok && v.before(minClaude) {
		return nil, fail.Runtime(fmt.Sprintf("%s, found '%s'", required, output))
	}
	return &Claude{path: path}, nil
}

// binary reports whether the file at path is a binary rather than a script, telling them apart as
// bash does before it runs a file the system will not execute (check_binary_file): it starts with
// ELF's magic number, or has a NUL in its first line - its first two, after #! - within its first
// 80 bytes. A file cld cannot read passes for a script: /bin/sh then says why.
func binary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sample := make([]byte, 80)
	n, _ := io.ReadFull(f, sample)
	sample = sample[:n]
	if bytes.HasPrefix(sample, []byte("\x7fELF")) {
		return true
	}
	lines := 1
	if bytes.HasPrefix(sample, []byte("#!")) {
		lines = 2
	}
	for _, c := range sample {
		if c == 0 {
			return true
		}
		if c == '\n' {
			if lines--; lines == 0 {
				break
			}
		}
	}
	return false
}

// workingDirectory is the current directory, where new starts claude. One that has been removed
// is refused, and so is one that cannot be entered - its search permission, or that of a
// directory above it, taken away since: given either with -c, tmux starts claude in the home
// directory instead (tmux 3.7c), as it did for the script, which went on with the PWD it got
// where the directory had been removed.
func workingDirectory() (string, error) {
	dir, err := os.Getwd()
	if errors.Is(err, fs.ErrNotExist) {
		return "", fail.Runtime("the current directory no longer exists")
	}
	// With PWD set, Getwd first looks at ".", which takes the search permission as well.
	if errors.Is(err, fs.ErrPermission) {
		return "", cannotEnter(err)
	}
	if err != nil {
		return "", fail.Runtime("cannot get the current directory: " + err.Error())
	}
	// tmux, and claude --version, enter the directory by its path.
	if err := unix.Access(dir, unix.X_OK); err != nil {
		return "", cannotEnter(err)
	}
	return dir, nil
}

// cannotEnter refuses a current directory that cannot be entered, with the system's reason.
func cannotEnter(err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		err = errno
	}
	return fail.Runtime("cannot enter the current directory: " + err.Error())
}

// lookPath finds name in the absolute entries of the PATH: the first executable file of that
// name or, where none is executable, the first file of that name, as bash's search had it - one
// that then cannot run (see cannotRun), rather than one not installed. cld never runs a program
// from a relative entry - ".", or an empty one - where the shell would; exec.LookPath refuses one
// found there, but stops at it, even when a later, absolute entry has the program too.
func lookPath(name string) (string, error) {
	lastResort := ""
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		path := filepath.Join(dir, name)
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
		if info, err := os.Stat(path); lastResort == "" && err == nil && !info.IsDir() {
			lastResort = path
		}
	}
	if lastResort != "" {
		return lastResort, nil
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// tool is the program name, found by lookPath, to run with args, cld's stdin and stderr.
func tool(name string, args ...string) (*exec.Cmd, error) {
	path, err := lookPath(name)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Args[0] = name
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	return cmd, nil
}

// number is a run of digits as a number, one too large for an int as the largest.
func number(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return math.MaxInt
	}
	return n
}

// how says how claude exited; hint shows how to end a session whose claude failed, on the message
// line until a key is pressed (see the package comment): the pane-died hook shows it to a
// terminal attached then, join to one attaching later.
const (
	how  = "#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}"
	hint = "display-message -d 0 'claude exited with " + how +
		": C-q d detaches, cld kill -n #{window_name} ends the session'"
)

// settings are what new passes claude with --settings, as JSON in this field order.
type settings struct {
	RemoteControlAtStartup bool             `json:"remoteControlAtStartup"`
	Worktree               worktreeSettings `json:"worktree,omitzero"`
}

type worktreeSettings struct {
	BaseRef string `json:"baseRef"`
}

// New creates session cld-SUFFIX on a server of its own, running claude in the current directory,
// and becomes a tmux client attached to it; with worktree claude works in git worktree SUFFIX. It
// returns only when it does not get as far.
func (t *Tmux) New(c *Claude, suffix string, worktree bool) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	name := "cld-" + suffix
	server, exists, _, err := t.lookup(context.Background(), suffix)
	if err != nil {
		return err
	}
	if exists {
		dead, _ := t.server(suffix, "list-panes", "-t", "="+name, "-F", "#{pane_dead}").Output()
		if strings.TrimRight(string(dead), "\n") == "1" {
			return fail.Runtime(fmt.Sprintf("session '%s' exists, but its claude exited; end it with cld kill -n %[1]s", suffix))
		}
		return fail.Runtime(fmt.Sprintf("session '%s' exists; attach to it with cld join -n %[1]s", suffix))
	}
	if server {
		return t.lingering(context.Background(), suffix)
	}
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	// Settings given on claude's command line override the user's and the project's. Remote
	// Control starts with the session, so it can be reached from claude.ai and the mobile app;
	// claude still keeps it off where org policy or the project's own settings turn it off.
	given := settings{RemoteControlAtStartup: true}
	if worktree {
		// cld reports a missing repository in the terminal; claude would report it in a session
		// left to kill.
		if !inWorkTree() {
			return fail.Runtime("--worktree needs a git repository, and " + dir + " is not in one")
		}
		// claude branches a new worktree from the remote's default branch unless
		// worktree.baseRef is "head".
		given.Worktree.BaseRef = "head"
	}
	encoded, err := json.Marshal(given)
	if err != nil {
		return fail.Runtime(err.Error())
	}
	claude := []string{c.path, "--name", name, "--settings", string(encoded)}
	if worktree {
		claude = append(claude, "--worktree", suffix)
	}
	if err := fail.Print(Title(suffix)); err != nil {
		return err
	}
	// claude and its arguments go to tmux as separate words: tmux then executes them directly
	// instead of through sh -c. claude goes by the path CheckClaude checked: tmux would look the
	// bare word up in the PATH, relative entries included, and could start another claude. What
	// follows new-session in the same tmux command - remain-on-exit and the pane-died hook, which
	// go to claude's window only, so that a session claude makes on its server closes as tmux
	// would close it - takes effect before tmux sees claude exit, however soon; tmux cuts the
	// command short when new-session fails, as when another cld new -n NAME got there first. The
	// targets end in ":" because set takes a pane, which "=NAME" does not find.
	window := "=" + name + ":"
	argv := []string{"tmux", "-L", name, "-f", "/dev/null",
		"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys", ";",
		"set", "-s", "focus-events", "on", ";",
		"set", "-g", "mouse", "on", ";", "set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
		"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
		"new-session", "-s", name, "-n", suffix, "-c", dir}
	argv = append(argv, claude...)
	argv = append(argv, ";",
		"set", "-w", "-t", window, "remain-on-exit", "failed", ";",
		"set", "-w", "-t", window, "remain-on-exit-format", "", ";",
		"set-hook", "-w", "-t", window, "pane-died", "if -F '#{window_active_clients}' \""+hint+"\"")
	// The server keeps the environment of the client that starts it, cld's (see the package
	// comment).
	return t.become(argv, slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, "TERMINAL_EMULATOR=")
	}))
}

// Join becomes a tmux client attached to session cld-SUFFIX, detaching any other. It returns
// only when it does not get as far. Joinable and Attach are its steps after the terminal's check,
// for a caller that has to look the session up before it hands the terminal over.
func (t *Tmux) Join(suffix string) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	if err := t.Joinable(context.Background(), suffix); err != nil {
		return err
	}
	return t.Attach(suffix)
}

// Joinable is join's lookup: nil when session cld-SUFFIX is on its server, and otherwise why join
// refuses it, with the advice for the command line kept apart (fail.Error's Advice). Once ctx is
// done, its tmux is killed.
func (t *Tmux) Joinable(ctx context.Context, suffix string) error {
	server, exists, _, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if !exists && server {
		return t.lingering(ctx, suffix)
	}
	if !exists {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: "; create it with cld new -n " + suffix}
	}
	return nil
}

// Attach is the rest of join, for a session Joinable found, from a terminal that is not a live
// pane of one of cld's servers (see OwnPane): it becomes a tmux client attached to session
// cld-SUFFIX, detaching any other. It returns only when it does not get as far.
func (t *Tmux) Attach(suffix string) error {
	if err := emptyTMUX(); err != nil {
		return err
	}
	name := "cld-" + suffix
	if err := fail.Print(Title(suffix)); err != nil {
		return err
	}
	// After attach-session in one command list, the hint goes to this terminal, attached by then.
	return t.become([]string{"tmux", "-L", name, "attach-session", "-d", "-t", "=" + name, ";",
		"if", "-F", "#{pane_dead}", hint}, os.Environ())
}

// Kill ends session cld-SUFFIX with its server, for cld kill: End, whatever its panes' pids, with
// tmux's messages on cld's stdout and stderr.
func (t *Tmux) Kill(suffix string) error {
	return t.End(context.Background(), suffix, nil, os.Stdout, os.Stderr)
}

// End is kill's steps after the name's check, which the interactive list's Ctrl+X takes too (see
// internal/picker): the lookup of session cld-SUFFIX, then the kill of the session with its
// server, and so of whatever claude started there through tmux. claude gets SIGHUP, as when its
// terminal closes. The session goes first, in the same tmux command: a terminal attached to it is
// told that the session exited, and the cld there - tmux by then - exits with status 0, where a
// kill-server alone would tell it that the server exited, with status 1. With pids, End ends the
// session only if one of its panes' pids (#{pane_pid}) is among them - claude's, read with the
// session (see Session) - so that a session made again under the name since they were read
// counts as no session. No session is an error, with the advice for the command line kept apart
// (fail.Error's Advice), and so is a server that runs without it (see lingering). A kill that
// fails is its exit status (fail.Status), after what tmux wrote to stdout and stderr. Once ctx is
// done, its tmux is killed.
func (t *Tmux) End(ctx context.Context, suffix string, pids []string, stdout, stderr io.Writer) error {
	server, exists, found, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if !exists && server {
		return t.lingering(ctx, suffix)
	}
	if !exists || len(pids) > 0 && !slices.ContainsFunc(found, func(pid string) bool { return slices.Contains(pids, pid) }) {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: " (see cld list)"}
	}
	kill := t.serverContext(ctx, suffix, "kill-session", "-t", "=cld-"+suffix, ";", "kill-server")
	kill.Stdout, kill.Stderr = stdout, stderr
	if err := kill.Run(); err != nil {
		return t.exitStatus(err)
	}
	return nil
}

// Session is one of the sessions cld started.
type Session struct {
	// Name is the session's NAME, without "cld-".
	Name string
	// State is "attached" or "detached", whether a terminal is attached, or "exited" once claude
	// has.
	State string
	// Attached is whether a terminal is attached, claude exited or not.
	Attached bool
	// PIDs are the process ids of the programs in the session's panes, tmux's #{pane_pid}: claude's,
	// and those of panes made by hand in its session - a window split, say. A pane keeps its pid
	// once its program has exited, and a session made again under the name has others (see End).
	PIDs []string
	// Directory is the directory claude is in now, or once it has exited the one its session
	// started in.
	Directory string
}

// Sessions reads the sessions cld started, in the order of their names; none where no server
// runs. Each has a server of its own: Sessions asks every server with a socket cld-NAME in tmux's
// directory (see socketDir) for its session cld-NAME, one tmux command a socket. It passes over a
// socket whose NAME no session can have, and the socket cld, of the one server earlier versions
// of cld shared. tmux never removes a socket - not when its server exits, is killed or dies - and
// on a stale one says that no server is running, which Sessions passes over too, as it passes over
// a server that exits while it asks, when a claude exits or a cld kill runs. cld removes none
// either: tmux replaces a stale socket under a lock, which cld would not hold, so cld could
// remove the socket of a server that a cld new had just started there. Once ctx is done, its tmux
// is killed.
func (t *Tmux) Sessions(ctx context.Context) ([]Session, error) {
	dir := socketDir()
	sockets, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return nil, fail.Runtime(fmt.Sprintf("cannot read %s: %v", dir, err))
	}
	// pane_current_path is the directory claude is in now, not the one its session started in;
	// once claude has exited there is none, and list shows where the session started.
	state := "#{?pane_dead,exited,#{?session_attached,attached,detached}}"
	path := "#{?pane_dead,#{session_path},#{pane_current_path}}"
	var sessions []Session
	for _, socket := range sockets {
		suffix, found := strings.CutPrefix(socket.Name(), "cld-")
		if !found || !ValidName(suffix) {
			continue
		}
		// tmux writes to a client whose LC_ALL, LC_CTYPE or LANG does not name UTF-8 - unset or C,
		// as over ssh, in containers and cron - with "_" for each character it cannot print: the
		// tabs, and any non-ASCII letter in a directory. -u marks the client UTF-8, so the output
		// arrives as it is.
		out, err := combinedOutput(t.commandContext(ctx, "-u", "-L", "cld-"+suffix, "list-sessions",
			"-f", only(suffix), "-F", "#{session_name}\t"+state+"\t#{session_attached}\t"+panePIDs+"\t"+path))
		if err != nil {
			if noServer(out) {
				continue
			}
			return nil, fail.Runtime(out)
		}
		line, _, _ := strings.Cut(out, "\n")
		if field := fields(line, 5); field[0] == "cld-"+suffix {
			clients, _ := strconv.Atoi(field[2])
			sessions = append(sessions, Session{Name: suffix, State: field[1], Attached: clients > 0, PIDs: strings.Fields(field[3]), Directory: field[4]})
		}
	}
	return sessions, nil
}

// socketDir is the directory tmux keeps the sockets of -L in: tmux-UID in TMUX_TMPDIR, or in /tmp
// where TMUX_TMPDIR is unset or empty, as tmux(1) has it - or names nothing, where tmux falls back
// to /tmp as well.
func socketDir() string {
	base := os.Getenv("TMUX_TMPDIR")
	if _, err := os.Stat(base); err != nil {
		base = "/tmp"
	}
	return filepath.Join(base, "tmux-"+strconv.Itoa(os.Getuid()))
}

// fields splits a line of Sessions' output into count fields: runs of tabs separate them, tabs
// around the line are dropped, and the last, the directory, takes the rest of it.
func fields(line string, count int) []string {
	rest := strings.Trim(line, "\t")
	var split []string
	for len(split) < count-1 {
		field, after, _ := strings.Cut(rest, "\t")
		split, rest = append(split, field), strings.TrimLeft(after, "\t")
	}
	return append(split, rest)
}

// only is a filter for session cld-SUFFIX alone on its server, beside the sessions claude made
// there. It compares whole names, where a target "cld-rev" would find cld-review.
func only(suffix string) string { return "#{==:#{session_name},cld-" + suffix + "}" }

// panePIDs are the pids of the programs in a session's panes, each followed by a space:
// #{pane_pid} alone, in a list-sessions format, is the pid of the active pane of the session's
// current window only.
const panePIDs = "#{W:#{P:#{pane_pid} }}"

// lookup reports whether the server of session cld-SUFFIX is running, whether the session is on
// it and, if it is, the pids of its panes (see Session). No server, no session. Once ctx is done,
// its tmux is killed.
func (t *Tmux) lookup(ctx context.Context, suffix string) (server, session bool, pids []string, err error) {
	found, err := combinedOutput(t.serverContext(ctx, suffix, "list-sessions", "-f", only(suffix), "-F", "#{session_name} "+panePIDs))
	if err != nil {
		if noServer(found) {
			return false, false, nil, nil
		}
		return false, false, nil, fail.Runtime(found)
	}
	name, rest, _ := strings.Cut(found, " ")
	if name != "cld-"+suffix {
		return true, false, nil, nil
	}
	return true, true, strings.Fields(rest), nil
}

// lingering is how new, join and kill refuse session cld-SUFFIX when its server runs without it.
// Mostly the server has outlived it: claude exited, and the tmux sessions it made keep the server
// running. new would start claude there with the environment of the cld that started the server,
// not its own, and cld leaves what claude made to the user. But where tmux's socket directory
// ignores case, as macOS's does by default, the socket cld-SUFFIX can be that of another session's
// server, whose NAME differs only in case: tmux -L cld-A reaches the server of session a, which
// the socket path it was started with names. That session may well be running, so cld refuses the
// name and names the session, rather than point at a kill-server that would end it. The advice
// for the command line is kept apart (fail.Error's Advice). Once ctx is done, its tmux is killed.
func (t *Tmux) lingering(ctx context.Context, suffix string) error {
	socket := t.serverContext(ctx, suffix, "list-sessions", "-F", "#{socket_path}")
	socket.Stderr = nil
	out, _ := socket.Output()
	path, _, _ := strings.Cut(string(out), "\n")
	if other, found := strings.CutPrefix(filepath.Base(path), "cld-"); found && other != suffix && strings.EqualFold(other, suffix) {
		return &fail.Error{Status: 1,
			Message: fmt.Sprintf("session name '%s' clashes with session '%s': tmux's socket directory ignores case here, so both names reach server cld-%[2]s", suffix, other),
			Advice:  fmt.Sprintf(" (see tmux -L cld-%s ls)", other)}
	}
	return &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' has ended, but its tmux server still runs", suffix),
		Advice:  fmt.Sprintf(" (see tmux -L cld-%[1]s ls); end it with tmux -L cld-%[1]s kill-server", suffix)}
}

// noServer reports whether tmux failed, saying message, because the server is not running: its
// socket is stale ("no server running on"), there is none ("error connecting to", with the
// system's message for no such file), or the server exited while tmux was asking it ("server
// exited unexpectedly"), as a server does once its session ends or is killed. Any other error
// connecting is cld's to report: a socket path too long for sun_path, say, which a TMUX_TMPDIR
// longer than tmux's default directories leaves room for (see MaxName).
func noServer(message string) bool {
	return strings.HasPrefix(message, "no server running on ") ||
		strings.HasPrefix(message, "error connecting to ") && strings.HasSuffix(message, " (No such file or directory)") ||
		strings.HasSuffix(message, "server exited unexpectedly")
}

// tmux refuses to attach a client with $TMUX set whose tty has the name of any pane on the server.
// A dead pane - a failed claude's - keeps the name of its closed pty, and the system hands the name
// to the next terminal opened: inside another tmux, tmux would refuse that terminal as nested. So
// cld makes the check itself, on the live panes, and gives its client an empty TMUX, which tmux's
// check skips; set, even empty, TMUX still tells the client that the terminal takes UTF-8.

// readyClient readies cld to become a tmux client: it refuses a terminal that is a live pane of
// one of cld's servers, and empties a TMUX that is set, for the client and every tmux command
// before it.
func (t *Tmux) readyClient() error {
	if suffix, found := t.OwnPane(); found {
		return fail.Runtime(fmt.Sprintf("this terminal is a pane of the tmux server of session '%s'; detach with C-q d first", suffix))
	}
	return emptyTMUX()
}

// emptyTMUX empties TMUX where it is set (see above).
func emptyTMUX() error {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	return os.Setenv("TMUX", "")
}

// OwnPane reports whether this terminal is a live pane of one of cld's servers - claude's
// external editor, say - and names the server's session: new and join refuse it, and list prints
// its table there. A session attached there would show inside a session of cld's, itself or
// another, both taking C-q. tmux refuses the first too, advising to unset $TMUX, but goes by name,
// dead panes included (see above); cld says how to get out instead. Like tmux it looks only when
// $TMUX is set, and only on the server TMUX names, if that is one of cld's: the terminal of any
// other tmux nests. tty names the terminal on its stdin, cld's.
func (t *Tmux) OwnPane() (string, bool) {
	socket, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	suffix, found := strings.CutPrefix(filepath.Base(socket), "cld-")
	if !found || !ValidName(suffix) {
		return "", false
	}
	tty, err := tool("tty")
	if err != nil {
		return "", false
	}
	terminal, err := tty.Output()
	if err != nil {
		return "", false
	}
	panes := t.command("-S", socket, "list-panes", "-a", "-F", "#{?pane_dead,,#{pane_tty}}")
	panes.Stderr = nil
	live, err := panes.Output()
	if err != nil {
		return "", false
	}
	return suffix, slices.Contains(strings.Split(strings.TrimRight(string(live), "\n"), "\n"), strings.TrimRight(string(terminal), "\n"))
}

// inWorkTree reports whether the current directory is in a git work tree.
func inWorkTree() bool {
	git, err := tool("git", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	git.Stderr = nil
	out, _ := git.Output()
	return strings.TrimRight(string(out), "\n") == "true"
}

// Title is what sets the terminal's title to the name of session cld-SUFFIX, which the tab
// keeps: tmux keeps claude's own title changes to its pane.
func Title(suffix string) string {
	return "\033]0;✳ cld-" + suffix + "\007"
}

// command is a tmux command run with cld's stdin and stderr, as tmux.
func (t *Tmux) command(args ...string) *exec.Cmd {
	return t.commandContext(context.Background(), args...)
}

// commandContext is command, killed once ctx is done: the interactive list abandons a lookup, a
// kill or a read of the sessions that way (see internal/picker). A program the killed tmux left
// behind with its output open then holds cld up for a second at most.
func (t *Tmux) commandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.path, args...)
	cmd.Args[0] = "tmux"
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if ctx.Done() != nil {
		cmd.WaitDelay = time.Second
	}
	return cmd
}

// server is a tmux command run against the server of session cld-SUFFIX, named like it.
func (t *Tmux) server(suffix string, args ...string) *exec.Cmd {
	return t.serverContext(context.Background(), suffix, args...)
}

// serverContext is server, killed once ctx is done (see commandContext).
func (t *Tmux) serverContext(ctx context.Context, suffix string, args ...string) *exec.Cmd {
	return t.commandContext(ctx, append([]string{"-L", "cld-" + suffix}, args...)...)
}

// combinedOutput runs cmd and returns its stdout and stderr together, without the newlines at
// the end; when cmd cannot run at all, cannotRun's message instead. A session lookup that fails
// ends cld with status 1 and that output, as the script's did with bash's message: a tmux that
// cannot run gets as far as a lookup only when it stops being runnable after answering tmux -V.
func combinedOutput(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) && out.Len() == 0 {
		out.WriteString(cannotRun(cmd.Path, err).Error())
	}
	return strings.TrimRight(out.String(), "\n"), err
}

// become replaces cld with tmux, run with argv and env: the terminal's process is tmux from
// then on, and tmux's exit status is cld's. It returns only when that fails.
func (t *Tmux) become(argv, env []string) error {
	return cannotRun(t.path, syscall.Exec(t.path, argv, env))
}

// exitStatus is the exit status of a tmux command that failed, which has said why, or how cld
// ends when it could not run tmux at all.
func (t *Tmux) exitStatus(err error) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return cannotRun(t.path, err)
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return fail.Status(128 + int(status.Signal()))
	}
	return fail.Status(exit.ExitCode())
}

// cannotRun ends cld when the system cannot run the program at path at all - a #! naming no
// interpreter, a binary for another machine, a file without the execute permission - with the
// status a shell gives: 127 when the system reports no such file, 126 otherwise.
func cannotRun(path string, err error) error {
	status := 126
	if errors.Is(err, fs.ErrNotExist) {
		status = 127
	}
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	return &fail.Error{Status: status, Message: fmt.Sprintf("cannot run %s: %v", path, err)}
}
