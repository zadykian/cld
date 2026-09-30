// Package session is cld's tmux side: it runs Claude Code in named sessions, each on a private
// tmux server of its own.
//
// A session's name is NAME-SUFFIX, from -n NAME and -s SUFFIX: NAME by default the name of the
// git repository the current directory is in or, outside one, of the directory itself (see
// DefaultName) - where that leaves nothing, the name is SUFFIX alone - and SUFFIX, for new, by
// default the index above the highest of the sessions NAME-INDEX running or recorded, or 0 (see
// Tmux.Next).
// Below and in the code, NAME is a session's whole name, as that is all tmux sees, and so is the
// parameter suffix, which follows "cld-": cld's own tmux session for session NAME.
//
// Repositories and directories of one name - two clones of a project, a fork beside it - share
// that NAME and its indexes. new and resume record on the session the option @cld-home, where
// they made it: the repository or, outside one, the directory (see Home). join and kill, where NAME
// is the current directory's by default, refuse a session made elsewhere (see foreign); with -n
// they take it, as they take a session of a cld that recorded none.
//
// `cld new` creates the tmux session "cld-NAME" on the server cld-NAME (tmux -L cld-NAME), running
// `claude --name cld-NAME` in the current directory, and attaches to it; with -w claude also gets
// --worktree cld-NAME, makes git worktree cld-NAME from HEAD or reopens it, and works there.
// Remote Control is left to claude's own setting (see Tmux.create). `cld resume [SESSION]` creates
// the session the same way, without -w, and claude resumes a conversation instead of starting
// one: without SESSION the one the session had last - by the ID of its entry in cld's record (see
// below), or else by the name cld-NAME - in the directory the session ran in, or the current one
// where cld keeps no record of the session, and with SESSION that, in the current directory. It
// also gets --resume ID, --resume cld-NAME or --resume SESSION (see Tmux.Resume). The words given
// to either after "--" go to claude after these, as they are.
// `cld join` attaches to the session again, `cld kill` ends it with its server (see Tmux.Kill),
// and `cld list` shows the sessions, asking each server for its own (see Tmux.Sessions) - on a
// terminal as a list to pick one from with the arrow keys, to join with Enter, as join does, or to
// kill with Ctrl+X pressed twice, as kill does (see internal/picker). The shell completion that
// `cld completion SHELL` prints reads the same sessions, where `cld join -n` completes the NAME of
// NAME-SUFFIX for the names `cld list` shows that run, and `cld join -s` their SUFFIX.
//
// cld keeps a record of its sessions beside tmux, which forgets a session with its server (see
// entry): new and resume write each session's entry - the directory claude starts in and,
// through claude's hooks, the ID of its conversation - so that list shows a session whose server
// no longer runs as ended, for 30 days, resume brings its conversation back by that ID in that
// directory, and new gives no index that names a conversation of that time. `cld list` resumes a
// session that has ended with Enter, as resume does, and forgets it with Ctrl+X twice; join and
// kill refuse it, pointing at resume, and `cld resume -n` and `-s` complete its name.
//
// cld looks for session cld-NAME on server cld-NAME only, and for no other session there.
// Whatever claude runs inherits TMUX, which takes a bare tmux to claude's own server: a session
// made that way has another name - cld-NAME is taken - so it is no session of cld's, and kill
// ends it with the server. The server of a session also gives its claude the environment of the
// shell that ran cld new or cld resume: tmux starts a pane with the environment of the client
// that started the server, but for PATH and the update-environment variables, so on one server
// for every session each claude had the first one's. new and resume refuse a NAME whose server
// outlives its session - claude exited, and the tmux sessions it made keep the server running -
// rather than start claude there with that server's environment, and join refuses it the same
// way, each pointing at kill, which ends such a server (see lingering). A private server
// (-f /dev/null: no ~/.tmux.conf) keeps these options away from any other tmux use:
//
//   - @cld 1: marks the server as cld's: a server named cld-NAME without it - the user's own
//     tmux -L cld-NAME, say - is none of cld's, whatever its sessions are called (see mark)
//   - extended-keys on: tmux answers no kitty keyboard query, so claude falls back to
//     modifyOtherKeys, which tmux forwards only when this is on (Shift+Enter and friends)
//   - the extkeys terminal feature for xterm*: tmux asks the terminal for modified keys only when
//     it knows the terminal supports them, and it does not recognise every terminal that does;
//     Claude Code's docs recommend this for tmux. It goes to a fixed index past tmux's defaults:
//     set -a would add another copy every time cld sets it on a server that has it, as two
//     cld new for one NAME at once do
//   - the hyperlinks terminal feature for xterm*, wezterm and alacritty, at fixed indexes too:
//     claude marks file paths and URLs as OSC 8 links under tmux, and tmux writes them to a
//     terminal only with this feature, which it gives by XTVERSION to iTerm2, foot and tmux
//     alone. wezterm is WezTerm's TERM where set, alacritty Alacritty's where its terminfo is
//     installed, and both take links; a terminal that takes none ignores them
//   - mouse on, focus-events on: claude probes both and hints when they are off. With the mouse
//     on, the wheel over a program that draws in the main screen without the mouse - claude
//     outside fullscreen, a shell - scrolls the pane's history; claude's fullscreen transcript
//     gets the wheel either way, as tmux passes claude's own mouse reporting on to the terminal
//   - C-MouseDown1Pane and M-MouseDown3Pane unbound: of tmux's mouse bindings on a pane, only
//     these two - swap-pane, the pane menu - take the press without asking whether the program
//     there takes the mouse, so claude got a Ctrl+click, with which it opens a link, as a release
//     alone. tmux hands a mouse key it has no binding for to the pane. The session has one pane
//     unless claude splits it, as its agent teams do for teammates in tmux panes, and key tables
//     are the server's, so the panes and sessions claude makes there lose the two as well: C-q {
//     and C-q } still swap panes, C-q > opens the pane menu, and so does a right-click over a
//     program that does not take the mouse
//   - history-limit 50000: while a terminal is attached it shows tmux in its alternate screen,
//     and its own scrollback gets nothing; what claude's classic renderer would leave there, the
//     whole conversation, goes to the pane's history, which tmux cuts at 2000 lines by default.
//     It goes before new-session: tmux 3.7 gives existing panes a new limit too, but earlier
//     releases only the panes made after it
//   - allow-passthrough on: claude wraps its notifications (but the bell, which tmux's default
//     bell-action passes on) and OSC 52 copies in tmux passthrough. It sends notifications only
//     on the channel its setting preferredNotifChannel names: its default, auto, goes by
//     TERM_PROGRAM, which tmux sets to tmux, and sends none. cld leaves the setting to the user:
//     in --settings it would override theirs, for whichever terminal joins later
//   - status off: claude keeps the whole tab
//   - prefix C-q: claude binds C-b (background a task) and nearly every other Ctrl key, but not
//     C-q; detach is C-q d, and C-q C-q sends a C-q through
//   - remain-on-exit failed: a claude that fails - at startup, say, for a worktree in a directory
//     it does not trust - leaves its pane on screen with its message, instead of taking both
//     away; /exit and claude's other ways out exit with status 0. An empty remain-on-exit-format
//     keeps tmux from scrolling the pane for its own line, which would push a short error at the
//     top out of sight; the pane-died hook shows how to end the session on the message line
//     instead, until a key is pressed. It names the session as cld kill takes it, -n and -s,
//     written into the hook as the session is made: the hook's formats know the pane and its
//     window, not the session. The hook shows it only to
//     a terminal on that window - of several, the one used last: tmux would show it on the
//     terminal of another session on the server - one claude made - or with none attached keep it
//     and show it in view-mode over the session a terminal attaches to next, which then takes no
//     keys until q; join shows it instead. These go to claude's pane only, not its window or the
//     server, so that the other panes of its window - a teammate's that claude splits off, one
//     split by hand - and the sessions claude makes there close as tmux would close them, rather
//     than stay on screen as a claude that exited (see Tmux.create)
//
// The tab's title is the session's name after claude's marker, as claude's own title has it
// outside tmux: ◐ and ◑ in turn while claude is busy, ✳ otherwise. Under tmux - TMUX set, which
// claude needs for its passthrough - claude keeps its marker at ✳ (claude 2.1.283), so it tells
// tmux instead: the hooks new and resume give it with --settings keep its status in the option
// @cld-status of its session (see statusHooks), and tmux, with set-titles on for that session
// only, sets the title of every terminal on it from that (see titles). The same hooks keep
// @cld-worktree, and the title ends in " [w]" while claude works in a linked git worktree. claude's
// own title stays in its pane. A terminal that detaches keeps the title tmux set last.
//
// The clients of new, resume and join - and of the list's Enter, which joins - take the terminal
// for UTF-8 (tmux -u), as list's reads of the sessions do (see Tmux.Sessions): tmux by itself does
// so only where TMUX is set or LC_ALL, LC_CTYPE or LANG names UTF-8, and otherwise draws each
// character that is not ASCII as "_" - most of claude's UI, over ssh to a host whose sshd takes no
// LANG, in a container, from cron. claude draws it in UTF-8 whatever the locale, as cld prints the
// title.
//
// join attaches beside any other terminal on the session, which stays attached: the window takes
// the size of the terminal used last (window-size latest), and a larger one shows the rest of its
// screen dotted. With --detach-others it attaches with -d, detaching the others.
// new, resume and join hand tmux the terminal of cld's stdin: without one, or with TERM unset,
// empty or dumb, they refuse once their other checks pass, before tmux starts a server that would
// fail on it (see checkTerminal), and they print the title only where stdout is a terminal.
// claude trusts TERMINAL_EMULATOR, and the other variables that name a terminal to it, over
// TERM_PROGRAM=tmux (see terminalVariables), and its environment comes from the client that
// started its server: a session created in the JetBrains terminal, or Cursor's, would keep its
// claude, and whatever claude starts through tmux, acting as if in that terminal (extended keys
// off, so Shift+Enter submits) even when joined from iTerm2 - hence new and resume leave those
// variables out of the environment they run tmux with (see withoutTerminal). The rest stays as
// the shell that ran new or resume had it for claude's life: join gives tmux the
// update-environment variables of its terminal, SSH_AUTH_SOCK and DISPLAY among them, for what
// starts on the session later, not claude.
package session

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/tool"
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

// Tmux is the tmux cld runs: found on the PATH by Find, which is all completion needs to read the
// sessions, and checked by Check, as every command needs before it runs tmux.
type Tmux struct {
	path string
}

// The oldest tmux and claude cld runs. tmux's is the oldest release the tests run on, 3.5a, its
// letter a third number (see tmuxVersion): 3.5 wrote keys with Shift wrongly with extended keys,
// and ran #() jobs, the title's among them, with the user's shell instead of /bin/sh. claude's is
// the first release that takes everything new and resume pass it and does what cld relies on.
// Both are raised by hand (see docs/design.md, decision 6).
var (
	minTmux   = version{3, 5, 1}
	minClaude = version{2, 1, 232}
)

// tmuxVersion and claudeVersion match the start of a version that tmux -V and claude --version
// report: "3.7c", "2.1.282 (Claude Code)". tmux's letter, which marks a bug-fix release, is its
// third number (see parseVersion): 3.5a is 3.5.1, and 3.5 is 3.5.0.
var (
	tmuxVersion   = regexp.MustCompile(`^([0-9]+)\.([0-9]+)([a-z]?)`)
	claudeVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
)

// version is the numbers of a version, the most significant first.
type version []int

// parseVersion reads a version from the start of text, as pattern matches it; false when text
// does not start with one. A letter counts as its place in the alphabet, and a group that matched
// nothing as 0.
func parseVersion(pattern *regexp.Regexp, text string) (version, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return nil, false
	}
	var v version
	for _, part := range match[1:] {
		switch {
		case part == "":
			v = append(v, 0)
		case part[0] >= 'a':
			v = append(v, int(part[0]-'a')+1)
		default:
			v = append(v, number(part))
		}
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

// tmuxRelease names v as tmux names its releases, the third number as a letter: 3.5.1 is 3.5a.
func tmuxRelease(v version) string {
	name := fmt.Sprintf("%d.%d", v[0], v[1])
	if v[2] > 0 {
		name += string(rune('a' + v[2] - 1))
	}
	return name
}

// Find finds tmux on the PATH (see tool.LookPath) and checks nothing else: completion reads the
// sessions with it on every TAB, where Check would cost a tmux -V each time.
func Find() (*Tmux, error) {
	path, err := tool.LookPath("tmux")
	if err != nil {
		return nil, fail.Runtime("tmux is not installed")
	}
	return &Tmux{path: path}, nil
}

// Check makes the checks every command makes before it runs tmux, in this order: tmux, then
// each of tools, on the PATH (see tool.LookPath), and tmux's version. A tmux -V that fails ends
// cld with its status, after its own message (see exitStatus). Completion makes none of them.
func Check(tools ...string) (*Tmux, error) {
	t, err := Find()
	if err != nil {
		return nil, err
	}
	for _, name := range tools {
		if _, err := tool.LookPath(name); err != nil {
			return nil, fail.Runtime(name + " is not installed")
		}
	}
	// The major and minor version count, and the letter of a bug-fix release after them: 3.5a
	// passes, and 3.5 does not. Development builds pass: "tmux next-3.9" reads as 3.9,
	// "tmux 3.8-rc2" as 3.8, and "tmux master" has no version to compare.
	out, err := t.command("-V").Output()
	if err != nil {
		return nil, t.exitStatus(err)
	}
	found := strings.TrimRight(string(out), "\n")
	reported := strings.TrimPrefix(found[strings.LastIndex(found, " ")+1:], "next-")
	if v, ok := parseVersion(tmuxVersion, reported); ok && v.before(minTmux) {
		return nil, fail.Runtime(fmt.Sprintf("tmux %s or newer is required, found '%s'", tmuxRelease(minTmux), found))
	}
	return t, nil
}

// Claude is the claude new and resume start: the one CheckClaude found and checked.
type Claude struct {
	path string
}

// CheckClaude checks the version of the claude on the PATH, for the commands that start it: an
// older claude lacks a flag or a setting cld passes it, or behaves otherwise than cld relies on.
// It reads the X.Y.Z that claude --version starts with. Output that starts otherwise passes, as a
// tmux development build does, so that a new format locks no one out; there is no upper bound. A
// claude --version that fails is refused with status 1 and what it printed: a claude that cannot
// report its version is unlikely to start. One that cannot run at all ends cld as a tmux that
// cannot run does (see tool.CannotRun): its version is not what is wrong.
func CheckClaude() (*Claude, error) {
	path, err := tool.LookPath("claude")
	if err != nil {
		return nil, fail.Runtime("claude is not installed")
	}
	// claude --version runs as tmux starts claude (see Tmux.create): the file tool.LookPath found,
	// by its path, in the current directory, with no input - so a version manager's shim, mise's
	// say, runs the claude that directory pins. A directory that has been removed, or that cannot
	// be entered, is refused first, as new and resume refuse it: claude --version fails in the
	// one (claude 2.1.282) and does not start in the other.
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
	// refuses to read one as commands: such a claude cannot run (see tool.CannotRun).
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
		return nil, tool.CannotRun(path, err)
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

// workingDirectory is the current directory, where new and resume start claude. One that has
// been removed is refused, and so is one that cannot be entered - its search permission, or that
// of a directory above it, taken away since: given either with -c, tmux starts claude in the home
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

// number is a run of digits as a number, one too large for an int as the largest.
func number(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return math.MaxInt
	}
	return n
}

// how says how claude exited.
const how = "#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}"

// hint shows how to end session cld-SUFFIX, whose claude failed, on the message line until a key
// is pressed (see the package comment): the pane-died hook shows it to a terminal attached then,
// join to one attaching later. It names the session as cld kill takes it (see Options), in
// characters that tmux's quotes and formats keep as they are.
func hint(suffix string) string {
	return "display-message -d 0 'claude exited with " + how +
		": C-q d detaches, cld kill " + Options(suffix) + " ends the session'"
}

// settings are what new and resume pass claude with --settings, as JSON in this field order.
type settings struct {
	Worktree worktreeSettings  `json:"worktree,omitzero"`
	Hooks    map[string][]hook `json:"hooks"`
}

type worktreeSettings struct {
	BaseRef string `json:"baseRef"`
}

// hook is a command claude runs on an event, where the event's matcher field - the notification's
// type for Notification - matches Matcher, or on every one of the event without it.
type hook struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// hookCommand is a command claude runs through sh: in the background with Async, and otherwise
// waiting for it, for Timeout seconds at most where that is not 0.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookTimeout is how many seconds claude waits for a hook of statusHooks that it does not run in
// the background, and for those of recordHooks but SessionEnd's, where its own default is 600 (30
// for UserPromptSubmit).
const hookTimeout = 5

// statusHooks are the hooks that keep claude's status on its session, for the tab's title (see
// titles). @cld-status is busy from a prompt on, waiting while claude asks - a permission, an MCP
// server's question - and idle once the turn is done, as claude tells its own status apart for
// its title outside tmux. PostToolUse goes back to busy after a question answered; an interrupt
// ends a tool's run with PostToolUseFailure, which says so, while one that comes as claude writes
// leaves busy until claude, idle a minute, notifies idle_prompt, or the next prompt.
//
// @cld-worktree is 1 while claude's directory is in a linked git worktree - its git directory is
// not the repository's common one - and 0 elsewhere: in the main worktree, outside a repository.
// claude runs a hook in its directory, which it sets as it starts, entering the worktree of
// --worktree or of a conversation it resumes, and as it enters or leaves one (EnterWorktree,
// ExitWorktree), each time with CwdChanged. CwdChanged's new_cwd is not taken: it names where the
// shell went, which claude takes back, without another event, when the shell leaves the worktree
// a session works in. git goes by the path cld found in the PATH's absolute entries (see
// tool.LookPath): the hook runs in claude's directory, where a relative entry would find a git of
// the project's own. Where cld finds no git, the hooks leave @cld-worktree alone.
//
// Each hook runs tmux, by the path cld checked, on the server of session cld-SUFFIX by its
// socket, and sets the option on that session by name, both written in as the session is made;
// tmux sets an option only where it changes, since setting any option redraws every terminal on
// the server. The hooks do not go by claude's TMUX and TMUX_PANE: claude does not always run them
// in its pane. It can run a conversation in a background worker of its daemon, which the claude in
// the pane shows (claude 2.1.284), and the worker runs the hooks without either - a bare tmux there
// goes to the default server, where set fails with "no current session", or sets the option on a
// session of that server. The set names the session too: without -t it would take the session of
// the pane in the client's TMUX_PANE or, without one, the session used last, which may be one
// claude made. The socket goes as an absolute path, since the hooks run in claude's directory.
// Nothing is printed for claude to take up: what a UserPromptSubmit or a SessionStart hook prints
// goes to the model.
//
// Each hook starts a tmux client, some 6 ms built from source, or a tenth of a second and more
// where tmux is slow to start, as Ubuntu's snap is (see serverless), and claude waits for a hook
// before it goes on - for PostToolUse's after every tool. The hooks of @cld-status keep it waiting
// all the same, since the order they land in is the status's: claude runs the tools of one answer
// with no call to the model between them, so a busy of PostToolUse run in the background could land
// after the waiting of the next tool's PermissionRequest and leave the title turning while claude
// asks, as could ElicitationResult's before an MCP server's next question, and UserPromptSubmit's
// after a StopFailure that comes at once. Each has a timeout of hookTimeout seconds instead, so
// that a tmux that hangs holds claude up for seconds, not minutes. CwdChanged runs in the
// background (async): its hook asks git about the directory claude started it in, however late it
// lands, and only two changes of claude's own directory in one answer - EnterWorktree, then
// ExitWorktree - could land out of order. claude ends no hook in the background at a timeout, and
// shows one that failed only in its verbose mode or transcript view (claude 2.1.284).
func statusHooks(tmux, git, socket, suffix string) map[string][]hook {
	session := "=cld-" + suffix + ":"
	// set sets option to value, a word of sh that the shell expands.
	set := func(option, value string) string {
		return shellWord(tmux) + ` -S ` + shellWord(socket) + ` if -F -t ` + shellWord(session) +
			` "#{!=:#{` + option + `},` + value + `}" "set -t ` + session + ` ` + option + ` ` + value + `"`
	}
	// on runs command on the event, where it matches matcher, and claude waits for it; background
	// runs command on every one of the event, and claude goes on.
	on := func(matcher, command string) []hook {
		return []hook{{Matcher: matcher, Hooks: []hookCommand{{Type: "command", Command: command, Timeout: hookTimeout}}}}
	}
	background := func(command string) []hook {
		return []hook{{Hooks: []hookCommand{{Type: "command", Command: command, Async: true}}}}
	}
	hooks := map[string][]hook{
		"UserPromptSubmit":   on("", set("@cld-status", "busy")),
		"PostToolUse":        on("", set("@cld-status", "busy")),
		"PostToolUseFailure": on("", `if grep -Eq '"is_interrupt": *true'; then `+set("@cld-status", "idle")+`; else `+set("@cld-status", "busy")+`; fi`),
		"PermissionRequest":  on("", set("@cld-status", "waiting")),
		"Elicitation":        on("", set("@cld-status", "waiting")),
		"ElicitationResult":  on("", set("@cld-status", "busy")),
		"Notification":       on("idle_prompt", set("@cld-status", "idle")),
		"Stop":               on("", set("@cld-status", "idle")),
		"StopFailure":        on("", set("@cld-status", "idle")),
	}
	if git != "" {
		dir := func(which string) string {
			return `"$(` + shellWord(git) + ` rev-parse --path-format=absolute ` + which + ` 2>/dev/null)"`
		}
		worktree := `w=0; [ ` + dir("--git-dir") + ` = ` + dir("--git-common-dir") + ` ] || w=1; ` + set("@cld-worktree", "$w")
		hooks["SessionStart"] = on("", worktree)
		hooks["CwdChanged"] = background(worktree)
	}
	return hooks
}

// shellWord is text as one word of sh, quoted.
func shellWord(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// New creates session cld-SUFFIX on a server of its own, running claude in the current directory,
// and becomes a tmux client attached to it; with worktree claude works in git worktree cld-SUFFIX,
// named as the session is, which claude makes on the branch worktree-cld-SUFFIX or reopens.
// claude gets args, the words given after "--", after cld's own arguments. It returns only when it
// does not get as far.
func (t *Tmux) New(c *Claude, suffix string, worktree bool, args []string) error {
	return t.create(c, suffix, worktree, "", "", args)
}

// Resume creates session cld-SUFFIX as New does, without a worktree, with claude resuming a
// conversation instead of starting one: conversation where that is not empty, and otherwise the
// one the session's entry in cld's record names by its ID (see recorded) - the one the session
// had last, whatever its name now - or else the one named cld-SUFFIX; and args after cld's own
// arguments. claude finds the conversation, and says so when it cannot: cld does not read
// claude's transcripts, whose format claude keeps to itself. The caller has made the directory the
// entry names the current one (see EnterRecorded). It returns only when it does not get as far.
func (t *Tmux) Resume(c *Claude, suffix, conversation string, args []string) error {
	id := ""
	if conversation == "" {
		conversation = "cld-" + suffix
		if r, ok := recorded(suffix); ok && r.Conversation != "" {
			conversation, id = r.Conversation, r.Conversation
		}
	}
	return t.create(c, suffix, false, conversation, id, args)
}

// create makes session cld-SUFFIX for New and Resume, which differ only in claude's arguments:
// with worktree claude works in git worktree cld-SUFFIX, and with a conversation it resumes that,
// whose ID id is where Resume took it from the session's entry. args come after cld's own
// arguments, so that no word of cld's is taken for the value of an option among them, such as
// --add-dir, which takes the words that follow it; it refuses those that would make tmux's
// command longer than tmux takes (see commandLimit). It writes the session's entry as it goes
// (see remember), under the record's lock, which the caller holds (see Lock).
func (t *Tmux) create(c *Claude, suffix string, worktree bool, conversation, id string, args []string) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	name := "cld-" + suffix
	if err := t.occupied(context.Background(), suffix); err != nil {
		return err
	}
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	_, home := DefaultName()
	// cld reports a missing repository in the terminal; claude would report it in a session left
	// to kill.
	if worktree && !inWorkTree() {
		return fail.Runtime("--worktree needs a git repository, and " + dir + " is not in one")
	}
	socket, err := filepath.Abs(filepath.Join(socketDir(), name))
	if err != nil {
		return fail.Runtime(err.Error())
	}
	command := "new"
	if conversation != "" {
		command = "resume"
	}
	// build is tmux's command, whose claude gets the hooks that keep the session's entry in file
	// where file is not empty, and its size as tmux counts it (see commandLimit).
	build := func(file string) (argv []string, size int, err error) {
		// Settings given on claude's command line override the user's and the project's, so they
		// carry only what cld needs: the title's hooks, the record's, and with -w the worktree's
		// base. Remote Control is the user's to choose, with /config or remoteControlAtStartup in
		// their settings, as for a claude started without cld: a true here would override their
		// false, and claude stores the transcript of a session connected to claude.ai on
		// Anthropic's servers (decision 42 in docs/design.md). A resumed conversation does not
		// keep the settings it was started with: they go again.
		git, _ := tool.LookPath("git")
		given := settings{Hooks: statusHooks(t.path, git, socket, suffix)}
		if file != "" {
			start, touch := recordHooks(file, suffix, dir)
			given.Hooks["SessionStart"] = append(given.Hooks["SessionStart"], hook{Hooks: []hookCommand{{Type: "command", Command: start, Timeout: hookTimeout}}})
			given.Hooks["Stop"] = append(given.Hooks["Stop"], hook{Hooks: []hookCommand{{Type: "command", Command: touch, Timeout: hookTimeout}}})
			given.Hooks["SessionEnd"] = []hook{{Hooks: []hookCommand{{Type: "command", Command: touch}}}}
		}
		if worktree {
			// claude branches a new worktree from the remote's default branch unless
			// worktree.baseRef is "head".
			given.Worktree.BaseRef = "head"
		}
		// Without HTML's escapes, so that a hook's 2>/dev/null reads as such, not as
		// 2\u003e/dev/null.
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(given); err != nil {
			return nil, 0, fail.Runtime(err.Error())
		}
		// claude gets the session's name when it resumes too: a conversation resumed by another
		// name is to take the session's, so that the next resume finds it (decision 16 in
		// docs/design.md).
		claude := []string{c.path, "--name", name, "--settings", strings.TrimSuffix(encoded.String(), "\n")}
		if worktree {
			claude = append(claude, "--worktree", name)
		}
		if conversation != "" {
			claude = append(claude, "--resume", conversation)
		}
		claude = append(claude, args...)
		// claude and its arguments go to tmux as separate words: tmux then executes them directly
		// instead of through sh -c, and each reaches claude as given, an empty one too, as the
		// directory reaches tmux (see literal and unexpanded). claude goes by the path
		// CheckClaude checked: tmux would look the bare word up in the PATH, relative entries
		// included, and could start another claude.
		// What follows new-session in the same tmux command - remain-on-exit and the pane-died
		// hook, which go to claude's pane only, so that another pane of its window, split by
		// claude for a teammate or by hand, and a session claude makes on its server close as
		// tmux would close them - takes effect before tmux sees claude exit, however soon; tmux
		// cuts the command short when new-session fails, as when another cld new or cld resume -n
		// NAME got there first. The title's options go to claude's session only, for the same
		// reason (see titles), and so does its home, which join and kill check (see foreign). The
		// targets end in ":" because set takes a pane, which "=NAME" does not find: "=NAME:" is
		// the active pane of the session's window, claude's, its only one yet. -u takes the
		// terminal for UTF-8 whatever the locale (see the package comment).
		target := "=" + name + ":"
		argv = []string{"tmux", "-u", "-L", name, "-f", "/dev/null"}
		options := len(argv)
		argv = append(argv, "set", "-s", "@cld", "1", ";",
			"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys:hyperlinks", ";",
			"set", "-s", "terminal-features[101]", "wezterm:hyperlinks", ";",
			"set", "-s", "terminal-features[102]", "alacritty:hyperlinks", ";",
			"set", "-s", "focus-events", "on", ";",
			"set", "-g", "mouse", "on", ";",
			"unbind", "-n", "C-MouseDown1Pane", ";", "unbind", "-n", "M-MouseDown3Pane", ";",
			"set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
			"set", "-g", "history-limit", "50000", ";",
			"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
			"new-session", "-s", name, "-n", suffix, "-c", literal(unexpanded(dir)))
		for _, word := range claude {
			argv = append(argv, literal(word))
		}
		argv = append(argv, ";",
			"set", "-p", "-t", target, "remain-on-exit", "failed", ";",
			"set", "-p", "-t", target, "remain-on-exit-format", "", ";",
			"set-hook", "-p", "-t", target, "pane-died", "if -F '#{window_active_clients}' \""+hint(suffix)+"\"", ";",
			"set", "-t", target, "@cld-tmux", literal(t.path), ";",
			"set", "-t", target, "@cld-home", literal(home.Dir), ";",
			"set", "-t", target, "@cld-busy", busyMarker, ";",
			"set", "-t", target, "set-titles-string", titles(suffix), ";",
			"set", "-t", target, "set-titles", "on")
		return argv, commandSize(argv[options:]), nil
	}
	// The entry goes once nothing is left to refuse the session, with the ID resume resumes, and
	// claude gets the hooks that keep it where it could be written; the entries that have expired
	// go with it, but for those of the sessions whose servers run. Before it, tmux's command is
	// counted with the hooks for the file the entry goes to - where cld then cannot write it, the
	// command goes without them, and is only shorter - so that a command too long leaves the
	// record as it was: tmux would start its server before it failed on a longer one, saying no
	// more than "command too long" or "failed to send command", and leave the server's socket once
	// the server has ended.
	planned := ""
	if state, err := stateDir(); err == nil {
		planned = entryFile(state, suffix)
	}
	argv, size, err := build(planned)
	if err != nil {
		return err
	}
	if size > commandLimit {
		return fail.Usage(fmt.Sprintf("claude's arguments make tmux's command %d bytes, and tmux takes %d at most: give claude long text in a file, as with --append-system-prompt-file", size, commandLimit))
	}
	file := remember(entry{Name: suffix, Directory: dir, Conversation: id}, func(other string) bool {
		server, _, _, _, err := t.lookup(context.Background(), other)
		return server || err != nil
	})
	if file != planned {
		if argv, _, err = build(file); err != nil {
			return err
		}
	}
	if err := checkTerminal(command); err != nil {
		return err
	}
	if err := printTitle(suffix); err != nil {
		return err
	}
	// The server keeps the environment of the client that starts it, cld's (see the package
	// comment).
	return t.become(argv, withoutTerminal(os.Environ()))
}

// terminalVariables are the variables claude reads before TERM_PROGRAM, which tmux sets to "tmux"
// in a pane as it sets TERM, to tell the terminal it runs in (claude 2.1.282 to 2.1.284):
// Cursor's CURSOR_TRACE_ID; VS Code's VSCODE_GIT_ASKPASS_MAIN, where its path names Cursor,
// Windsurf or Antigravity; macOS's __CFBundleIdentifier, the app the shell runs in, where that is
// a JetBrains IDE, VSCodium, Windsurf or Android Studio; Visual Studio's VisualStudioVersion; and
// JetBrains' TERMINAL_EMULATOR. claude knows none of those terminals for extended keys: it asks
// tmux, which does not answer, and turns them off, so Shift+Enter submits. Each goes whatever its
// value: it names the terminal the session started in, not the one on it. VSCODE_GIT_ASKPASS_MAIN
// goes with VS Code's askpass (see withoutTerminal).
var terminalVariables = []string{"CURSOR_TRACE_ID", "__CFBundleIdentifier", "VisualStudioVersion", "TERMINAL_EMULATOR"}

// vsCodeGit are the helpers a terminal of VS Code, or of one of its forks, gives git, each a
// script that git runs, named by the variable git, beside the file that the variable prefix+MAIN
// names: the askpass and, with git.terminalGitEditor, the editor, whose script VS Code names
// quoted. The script runs node, prefix+NODE, on prefix+MAIN with prefix+EXTRA_ARGS, which asks
// the window through VSCODE_GIT_IPC_HANDLE, and fails at once without it.
var vsCodeGit = []struct{ git, prefix string }{
	{"GIT_ASKPASS", "VSCODE_GIT_ASKPASS_"},
	{"GIT_EDITOR", "VSCODE_GIT_EDITOR_"},
}

// withoutTerminal is environ without terminalVariables and VS Code's helpers for git (see
// vsCodeGit), each as a unit, since a part left without the rest fails: VSCODE_GIT_IPC_HANDLE,
// the socket of the window that asks, every variable of a helper's prefix, and GIT_ASKPASS and
// GIT_EDITOR where they name a script beside their helper's MAIN. A GIT_ASKPASS or GIT_EDITOR
// elsewhere is the user's own, and stays.
func withoutTerminal(environ []string) []string {
	scripts := map[string]string{}
	for _, helper := range vsCodeGit {
		for _, variable := range environ {
			if main, found := strings.CutPrefix(variable, helper.prefix+"MAIN="); found && main != "" {
				scripts[helper.git] = filepath.Dir(main)
			}
		}
	}
	return slices.DeleteFunc(environ, func(variable string) bool {
		name, value, _ := strings.Cut(variable, "=")
		if slices.Contains(terminalVariables, name) || name == "VSCODE_GIT_IPC_HANDLE" {
			return true
		}
		for _, helper := range vsCodeGit {
			if strings.HasPrefix(name, helper.prefix) {
				return true
			}
			if name == helper.git {
				script := value
				if len(script) > 1 && script[0] == '"' && script[len(script)-1] == '"' {
					script = script[1 : len(script)-1]
				}
				dir, found := scripts[name]
				return found && filepath.Dir(script) == dir
			}
		}
		return false
	})
}

// commandLimit is the size of the longest command a tmux client hands its server: the client
// sends the words after its options, each followed by a NUL, behind their count, an int, as one
// message of at most 16384 bytes, 16 of them the message's header (MAX_IMSGSIZE and
// IMSG_HEADER_SIZE in tmux's compat/imsg.h), and fails otherwise with "command too long" or
// "failed to send command". Of new's command, cld's own words take some 5 to 6 KB, most of it
// claude's settings, whose hooks name tmux and the server's socket by their paths, and the file
// of the session's entry in cld's record; the rest is for the words given to claude, resume's
// SESSION among them.
const commandLimit = 16384 - 16 - 4

// commandSize is the size of command as a tmux client hands it to its server, without the count:
// each word followed by a NUL.
func commandSize(command []string) int {
	size := 0
	for _, word := range command {
		size += len(word) + 1
	}
	return size
}

// literal is word as tmux takes it back from its command line: tmux ends a command at a word
// that ends in ";", keeping what comes before the ";" as an argument, and turns a "\;" at the
// end of a word into ";". Of cld's words, the directory, the session's home, the conversation
// given to resume and the words given after "--" can end in one: "a;" goes as "a\;", and "a\;"
// as "a\\;".
func literal(word string) string {
	if before, found := strings.CutSuffix(word, ";"); found {
		return before + `\;`
	}
	return word
}

// unexpanded is text as a tmux format that expands to text itself: every "#" doubled, since
// "##" expands to "#". tmux expands new-session's -c as a format, in which a "#" starts
// something to replace - #S, #{...}, or #(command), which runs command through the shell - and
// where the result is no directory it starts claude in the home directory. claude's words are
// not expanded.
func unexpanded(text string) string {
	return strings.ReplaceAll(text, "#", "##")
}

// occupied is why new and resume refuse the name of session cld-SUFFIX, which is on its server
// (see taken) or whose server runs without it (see lingering), or what went wrong looking; nil
// where its server does not run. Once ctx is done, its tmux is killed.
func (t *Tmux) occupied(ctx context.Context, suffix string) error {
	server, exists, _, made, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if exists {
		return t.taken(suffix, made)
	}
	if server {
		_, refused := t.lingering(ctx, suffix)
		return refused
	}
	return nil
}

// taken is how new and resume refuse session cld-SUFFIX, which is on its server, made in the home
// made: join attaches to it, and one whose claude exited kill ends.
func (t *Tmux) taken(suffix, made string) error {
	// The session's home, where it has one, tells a session of another repository of this one's
	// name from this repository's own.
	if made != "" {
		made = " in " + made
	}
	dead, _ := t.server(suffix, "list-panes", "-t", "=cld-"+suffix, "-F", "#{pane_dead}").Output()
	if strings.TrimRight(string(dead), "\n") == "1" {
		return fail.Runtime(fmt.Sprintf("session '%s' exists%s, but its claude exited; end it with cld kill %s", suffix, made, Options(suffix)))
	}
	return fail.Runtime(fmt.Sprintf("session '%s' exists%s; attach to it with cld join %s", suffix, made, Options(suffix)))
}

// ended is how join and kill refuse session cld-SUFFIX, which has ended, where cld's record has
// its entry (see recorded): resume brings its conversation back. The advice for the command line
// is kept apart (fail.Error's Advice). nil where there is no entry.
func ended(suffix string) error {
	if _, ok := recorded(suffix); !ok {
		return nil
	}
	return &fail.Error{Status: 1, Message: fmt.Sprintf("session '%s' has ended", suffix), Advice: "; resume it with cld resume " + Options(suffix)}
}

// Join becomes a tmux client attached to session cld-SUFFIX, beside any other or, with
// detachOthers, detaching them; with a home, only to a session made there (see foreign). It
// returns only when it does not get as far. Joinable and Attach are its steps after the check for
// cld's own pane, for a caller that has to look the session up before it hands the terminal over.
func (t *Tmux) Join(suffix string, home Home, detachOthers bool) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	if err := t.Joinable(context.Background(), suffix, home); err != nil {
		return err
	}
	return t.Attach(suffix, detachOthers)
}

// Joinable is join's lookup: nil when session cld-SUFFIX is on its server - with a home, made
// there, or where it has none (see foreign) - and otherwise why join refuses it - for one that
// has ended, pointing at resume (see ended) - with the advice for the command line kept apart
// (fail.Error's Advice). Once ctx is done, its tmux is killed.
func (t *Tmux) Joinable(ctx context.Context, suffix string, home Home) error {
	server, exists, _, made, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if !exists && server {
		_, refused := t.lingering(ctx, suffix)
		return refused
	}
	if !exists {
		if err := ended(suffix); err != nil {
			return err
		}
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: "; create it with cld new " + Options(suffix)}
	}
	return foreign("join", suffix, made, home)
}

// Attach is the rest of join, for a session Joinable found, from a terminal that is not a live
// pane of one of cld's servers (see OwnPane): it refuses a terminal tmux could not attach from
// (see checkTerminal), and becomes a tmux client attached to session cld-SUFFIX, beside any other
// or, with detachOthers, detaching them. It returns only when it does not get as far.
func (t *Tmux) Attach(suffix string, detachOthers bool) error {
	if err := emptyTMUX(); err != nil {
		return err
	}
	if err := checkTerminal("join"); err != nil {
		return err
	}
	name := "cld-" + suffix
	if err := printTitle(suffix); err != nil {
		return err
	}
	// -u takes the terminal for UTF-8 whatever the locale, as new's client does.
	attach := []string{"tmux", "-u", "-L", name, "attach-session"}
	if detachOthers {
		attach = append(attach, "-d")
	}
	// After attach-session in one command list, the hint goes to this terminal, attached by then.
	return t.become(append(attach, "-t", "="+name, ";", "if", "-F", "#{pane_dead}", hint(suffix)), os.Environ())
}

// Kill ends session cld-SUFFIX with its server, for cld kill: End, whatever its panes' pids, with
// tmux's messages on cld's stdout and stderr.
func (t *Tmux) Kill(suffix string, home Home) error {
	return t.End(context.Background(), suffix, home, nil, os.Stdout, os.Stderr)
}

// End is kill's steps after the name's check, which the interactive list's Ctrl+X takes too (see
// internal/picker): the lookup of session cld-SUFFIX, then the kill of the session with its
// server, and so of whatever claude started there through tmux. claude gets SIGHUP, as when its
// terminal closes. The session goes first, in the same tmux command: a terminal attached to it is
// told that the session exited, and the cld there - tmux by then - exits with status 0, where a
// kill-server alone would tell it that the server exited, with status 1. With a home, End ends
// only a session made there, or one that has none (see foreign). With pids, End ends the session
// only if one of its panes' pids (#{pane_pid}) is among them - claude's, read with the session
// (see Session) - so that a session made again under the name since they were read counts as no
// session. No session is an error, with the advice for the command line kept apart (fail.Error's
// Advice) - for one that has ended, of cld's record, pointing at resume (see ended). A server
// that has outlived the session - claude exited, and the tmux sessions it made keep the server
// running - End ends with kill-server alone, whatever pids and home: no pane of the session is
// left to check, nor its home, and whichever claude of that name left the server, the kill of its
// session would have ended it. The kill-server runs under lingering's check, made again in the
// same tmux command (see outlives): where it no longer holds - a session cld-SUFFIX made since,
// on a fresh server that a cld new started on the socket, say - the kill ends nothing, and End
// says nothing, as where the kill had come first. A server that runs without the session
// otherwise is an error too (see lingering). A kill that fails is its exit status (fail.Status),
// after what tmux wrote to stdout and stderr. Once ctx is done, its tmux is killed.
//
// claude shuts down on the SIGHUP: it runs its SessionEnd hooks with the reason "other", and
// exits. End does not wait for it: claude, orphaned, may still run its hooks when End returns,
// and what it prints then is lost (see docs/design.md, decision 32).
func (t *Tmux) End(ctx context.Context, suffix string, home Home, pids []string, stdout, stderr io.Writer) error {
	server, exists, found, made, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	command := []string{"kill-session", "-t", "=cld-" + suffix, ";", "kill-server"}
	switch {
	case !exists && server:
		if outlived, refused := t.lingering(ctx, suffix); !outlived {
			return refused
		}
		command = []string{"if", "-F", outlives(suffix), "kill-server"}
	case !exists || len(pids) > 0 && !slices.ContainsFunc(found, func(pid string) bool { return slices.Contains(pids, pid) }):
		if !exists {
			if err := ended(suffix); err != nil {
				return err
			}
		}
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: " (see cld list)"}
	}
	if err := foreign("kill", suffix, made, home); err != nil {
		return err
	}
	kill := t.serverContext(ctx, suffix, command...)
	kill.Stdout, kill.Stderr = stdout, stderr
	if err := kill.Run(); err != nil {
		return t.exitStatus(err)
	}
	return nil
}

// Resumable is resume's checks for the interactive list's Enter on a session that has ended: nil
// when session cld-SUFFIX has ended - its server does not run - and its entry in cld's record
// names a directory that can be entered, and otherwise why resume refuses it, with the advice for
// the command line kept apart (fail.Error's Advice). Once ctx is done, its tmux is killed.
func (t *Tmux) Resumable(ctx context.Context, suffix string) error {
	if err := t.occupied(ctx, suffix); err != nil {
		return err
	}
	r, ok := recorded(suffix)
	if !ok {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: " (see cld list)"}
	}
	return enterable(r)
}

// Forget forgets session cld-SUFFIX, which has ended, for the interactive list's Ctrl+X on its
// row: its entry in cld's record goes, and the list no longer shows it, but the index its name
// may end in stays given (see Next), as claude keeps its conversation. A session that runs again
// under the name is not forgotten, and neither is a name without an entry: each is an error, with
// the advice for the command line kept apart (fail.Error's Advice). Once ctx is done, its tmux is
// killed.
func (t *Tmux) Forget(ctx context.Context, suffix string) error {
	_, exists, _, _, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if exists {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("session '%s' runs again", suffix), Advice: " (see cld list)"}
	}
	if _, ok := recorded(suffix); !ok {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix), Advice: " (see cld list)"}
	}
	dir, _ := stateDir() // recorded found the entry there
	if err := os.Remove(entryFile(dir, suffix)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail.Runtime("cannot forget session '" + suffix + "': " + reason(err).Error())
	}
	return nil
}

// Session is one of the sessions cld started.
type Session struct {
	// Name is the session's NAME, without "cld-".
	Name string
	// State is "attached" or "detached", whether a terminal is attached, "exited" once claude
	// has, or Ended once its server no longer runs, for a session of cld's record.
	State string
	// Attached is whether a terminal is attached, claude exited or not.
	Attached bool
	// PIDs are the process ids of the programs in the session's panes, tmux's #{pane_pid}: claude's,
	// and those of panes made by hand in its session - a window split, say. A pane keeps its pid
	// once its program has exited, and a session made again under the name has others (see End).
	PIDs []string
	// Directory is the directory claude is in now, or once it has exited the one its session
	// started in, and for a session that has ended the one its entry names.
	Directory string
	// Home is where new or resume made the session, as @cld-home has it (see Home); "" for a
	// session of a cld that recorded none, 0.8.2 or earlier, and for one that has ended, whose
	// entry records none.
	Home string
}

// Sessions reads the sessions cld started, in the order of their names: those whose servers run,
// and those that have ended, as Ended - the entries of cld's record without their session (see
// entries). Each has a server of its own: Sessions asks every server with a socket cld-NAME in
// tmux's directory (see socketDir) for its session cld-NAME, one tmux command a server that takes
// the connection (see serverless), up to asks servers at once, in the order of the names. It
// passes over a socket whose NAME no session can have, a server that cld did not start (see mark),
// and the socket cld, of the one server earlier versions of cld shared. tmux never removes a
// socket - not when its server exits, is killed or dies - and on a stale one says that no server
// is running: Sessions passes over it without running tmux, as it passes over a server that exits
// while it asks, when a claude exits or a cld kill runs. cld removes none either: tmux replaces a
// stale socket under a lock, which cld would not hold, so cld could remove the socket of a server
// that a cld new had just started there. Once a server fails otherwise, Sessions asks no more -
// where tmux refuses its directory, every server would fail alike - and of those that failed, the
// first in the order of the names gives the error. Sessions starts no server, and writes nothing:
// it is list's read of the sessions, and completion's, on every TAB. Once ctx is done, its tmux is
// killed.
func (t *Tmux) Sessions(ctx context.Context) ([]Session, error) {
	dir := socketDir()
	sockets, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return nil, fail.Runtime(fmt.Sprintf("cannot read %s: %v", dir, err))
	}
	connect := tmuxDir()
	var suffixes []string
	for _, socket := range sockets {
		suffix, found := strings.CutPrefix(socket.Name(), "cld-")
		if found && ValidName(suffix) && !serverless(ctx, connect, suffix) {
			suffixes = append(suffixes, suffix)
		}
	}
	// Each ask takes a place in asking before it starts, in the order of the names, and gives it
	// back once it has its answer, in a slot of its own; one that fails first says so in failing,
	// and no ask starts after that. Those under way finish: each comes before the ones not
	// started, and may fail too.
	found := make([]*Session, len(suffixes))
	failed := make([]error, len(suffixes))
	asking := make(chan struct{}, asks)
	var failing atomic.Bool
	var wait sync.WaitGroup
	for i, suffix := range suffixes {
		asking <- struct{}{}
		if failing.Load() {
			break
		}
		wait.Go(func() {
			defer func() { <-asking }()
			if found[i], failed[i] = t.session(ctx, suffix); failed[i] != nil {
				failing.Store(true)
			}
		})
	}
	wait.Wait()
	var sessions []Session
	for i := range suffixes {
		if failed[i] != nil {
			return nil, failed[i]
		}
		if found[i] != nil {
			sessions = append(sessions, *found[i])
		}
	}
	// The sessions that have ended: those of the record with no session on their server - none, or
	// one that outlives its session (see lingering), which resume refuses.
	for _, r := range entries() {
		if !slices.ContainsFunc(sessions, func(s Session) bool { return s.Name == r.Name }) {
			sessions = append(sessions, Session{Name: r.Name, State: Ended, Directory: r.Directory})
		}
	}
	slices.SortFunc(sessions, func(a, b Session) int { return strings.Compare(a.Name, b.Name) })
	return sessions, nil
}

// asks is how many servers Sessions asks at once. Through Ubuntu's snap, where a tmux took
// 100-180 ms to start on 8 CPUs at a load of about 4, 10 servers took 1.2-1.5 s one after another,
// 0.24-0.26 s eight at a time and 0.21-0.24 s all at once (see docs/design.md, Findings).
const asks = 8

// session asks the server of socket cld-SUFFIX for its session cld-SUFFIX, for Sessions: nil
// where no server runs there, or runs without it.
func (t *Tmux) session(ctx context.Context, suffix string) (*Session, error) {
	// pane_current_path is the directory claude is in now, not the one its session started in;
	// once claude has exited there is none, and list shows where the session started. The
	// session's home comes right before it, both paths, which can hold a tab: the home's length in
	// bytes (n:) goes before them, and the directory takes the rest of the line.
	state := "#{?pane_dead,exited,#{?session_attached,attached,detached}}"
	path := "#{n:@cld-home}\t#{@cld-home}#{?pane_dead,#{session_path},#{pane_current_path}}"
	// tmux writes to a client whose LC_ALL, LC_CTYPE or LANG does not name UTF-8 - unset or C, as
	// over ssh, in containers and cron - with "_" for each character it cannot print: the tabs,
	// and any non-ASCII letter in a directory. -u marks the client UTF-8, so the output arrives as
	// it is.
	out, err := combinedOutput(t.commandContext(ctx, "-u", "-L", "cld-"+suffix, "list-sessions",
		"-f", only(suffix), "-F", "#{session_name}\t"+state+"\t#{session_attached}\t"+panePIDs+"\t"+path))
	if err != nil {
		if noServer(out) {
			return nil, nil
		}
		return nil, fail.Runtime(out)
	}
	line, _, _ := strings.Cut(out, "\n")
	field := fields(line, 6)
	if field[0] != "cld-"+suffix {
		return nil, nil
	}
	clients, _ := strconv.Atoi(field[2])
	home, directory := cutHome(field[4], field[5])
	return &Session{Name: suffix, State: field[1], Attached: clients > 0, PIDs: strings.Fields(field[3]),
		Directory: directory, Home: home}, nil
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

// tmuxDir is the socket directory (see socketDir) as tmux's socket paths name it, for serverless:
// with its symbolic links resolved, as tmux resolves them, and absolute. It is "" where tmux would
// not connect to a socket there: a directory tmux refuses - one that is a symbolic link or no
// directory, one of another user's, or one that others can use ("directory ... has unsafe
// permissions") - or none, which tmux makes first.
func tmuxDir() string {
	dir := socketDir()
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o007 != 0 {
		return ""
	}
	if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Getuid() {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	if resolved, err = filepath.Abs(resolved); err != nil {
		return ""
	}
	return resolved
}

// serverless reports whether no server runs on socket cld-SUFFIX in dir, tmuxDir's, found as
// tmux finds it, without running tmux: a tmux takes 6-20 ms to start, and one from Ubuntu's snap
// 100-200 ms, where a connection takes microseconds (see docs/design.md, Findings). tmux's client
// first connects to the socket, and takes a refused connection (ECONNREFUSED: a socket whose
// server has gone) or no socket (ENOENT) for no server - "no server running on" and "error
// connecting to ... (No such file or directory)", which noServer takes. serverless makes that
// connection, and closes it at once: a server that takes it loses a client, as it does after each
// tmux command. Anything else leaves it to tmux, which says what is wrong: a server takes the
// connection, dir is "", the path is too long for sun_path and its NUL - which tmux checks before
// it connects - or the connection fails otherwise.
func serverless(ctx context.Context, dir, suffix string) bool {
	if dir == "" {
		return false
	}
	path := filepath.Join(dir, "cld-"+suffix)
	if len(path) >= len(unix.RawSockaddrUnix{}.Path) {
		return false
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err == nil {
		_ = conn.Close()
		return false
	}
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.ENOENT)
}

// fields splits a line of Sessions' output into count fields: runs of tabs separate them, tabs
// around the line are dropped, and the last, the home and the directory (see cutHome), takes the
// rest of it.
func fields(line string, count int) []string {
	rest := strings.Trim(line, "\t")
	var split []string
	for len(split) < count-1 {
		field, after, _ := strings.Cut(rest, "\t")
		split, rest = append(split, field), strings.TrimLeft(after, "\t")
	}
	return append(split, rest)
}

// cutHome splits the end of a line of Sessions' output, rest, into the session's home and the
// directory after it, at the home's length. A home cut short with the line, by a newline in it,
// is the whole of rest.
func cutHome(length, rest string) (home, directory string) {
	n, err := strconv.Atoi(length)
	if err != nil || n < 0 {
		return "", rest
	}
	n = min(n, len(rest))
	return rest[:n], rest[n:]
}

// only is a filter for session cld-SUFFIX alone on its server, beside the sessions claude made
// there, and on a server that cld started only (see mark). It compares whole names, where a
// target "cld-rev" would find cld-review.
func only(suffix string) string {
	return "#{&&:#{==:#{session_name},cld-" + suffix + "}," + mark + "}"
}

// mark is, as a format, 1 on a server that cld started and 0 on any other: a tmux server named
// cld-NAME that cld did not start - the user's own tmux -L cld-NAME, say - is none of cld's
// business, whatever its sessions are called. create sets the server option @cld, which a format
// finds before any other option of that name (see docs/design.md, Findings). The servers that cld
// 0.8.2 and earlier started have none: their prefix, C-q, marks them instead, as tmux's default,
// C-b, marks no other.
const mark = "#{||:#{@cld},#{==:#{prefix},C-q}}"

// panePIDs are the pids of the programs in a session's panes, each followed by a space:
// #{pane_pid} alone, in a list-sessions format, is the pid of the active pane of the session's
// current window only.
const panePIDs = "#{W:#{P:#{pane_pid} }}"

// lookup reports whether the server of session cld-SUFFIX is running, whether the session is on
// it and, if it is, the pids of its panes (see Session) and where it was made (Session's Home). No
// server, no session, and none on a server that cld did not start (see only); where the socket
// says there is no server, no tmux runs (see serverless). The home, a path, comes last, after a
// tab, and takes the rest; -u, as for Sessions, has tmux write the tab, and any other character,
// as it is. Once ctx is done, its tmux is killed.
func (t *Tmux) lookup(ctx context.Context, suffix string) (server, session bool, pids []string, home string, err error) {
	if serverless(ctx, tmuxDir(), suffix) {
		return false, false, nil, "", nil
	}
	found, err := combinedOutput(t.commandContext(ctx, "-u", "-L", "cld-"+suffix, "list-sessions", "-f", only(suffix),
		"-F", "#{session_name} "+panePIDs+"\t#{@cld-home}"))
	if err != nil {
		if noServer(found) {
			return false, false, nil, "", nil
		}
		return false, false, nil, "", fail.Runtime(found)
	}
	name, rest, _ := strings.Cut(found, " ")
	if name != "cld-"+suffix {
		return true, false, nil, "", nil
	}
	rest, home, _ = strings.Cut(rest, "\t")
	return true, true, strings.Fields(rest), home, nil
}

// foreign is how join and kill, named in the advice as command, refuse session cld-SUFFIX, made in
// the home made, where home, the current directory's, does not take it (see Home.Takes): a
// repository or directory of the same name made it, and -n NAME was not given. The advice names
// the session with -n (see Options), which join and kill then take from anywhere; it is kept apart
// (fail.Error's Advice).
func foreign(command, suffix, made string, home Home) error {
	if home.Takes(made) {
		return nil
	}
	here := "this directory"
	if home.Repository {
		here = "this repository"
	}
	return &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' belongs to %s, not to %s", suffix, made, here),
		Advice:  fmt.Sprintf("; name it with cld %s %s", command, Options(suffix))}
}

// lingering is how new, resume, join and kill refuse session cld-SUFFIX when its server runs
// without it, and whether the server has outlived the session, which kill ends instead (see End).
// A server that cld did not start (see mark) is not cld's to use or to end: cld refuses the name,
// pointing at no kill, which would end the sessions there. Mostly the server has outlived the
// session: claude exited, and the tmux sessions it made keep the server running. new and
// resume would start claude there with the environment of the cld that started the server, not
// their own, and join finds no session there, so they refuse the name, pointing at kill. The
// server has outlived the session where it has sessions, none of them cld-SUFFIX, and its socket
// is that of server cld-SUFFIX (see outlives). A server with none is one a cld new is starting,
// before new-session makes its session, or one exiting, and one with session cld-SUFFIX by now
// has had it made since the lookup, by the cld new starting the server, say: kill refuses the
// name there as well, pointing at no kill. A session of claude's that was
// renamed - by hand, or by a tmux rename-session that claude runs, which renames the session of
// its pane - is not the one its server is named after, and counts as ended: kill ends its server,
// claude with it. And where tmux's socket directory ignores case, as macOS's does by default, the
// socket cld-SUFFIX can be that of another session's server, whose NAME differs only in case:
// tmux -L cld-A reaches the server of session a, which the socket path it was started with
// (#{socket_path}) names. That session may well be running, so cld refuses the name and names the
// session, rather than end the server or point at a kill that would. The advice for the command
// line is kept apart (fail.Error's Advice). Once ctx is done, its tmux is killed.
func (t *Tmux) lingering(ctx context.Context, suffix string) (outlived bool, refused error) {
	// Spaces, not tabs, which tmux writes as "_" to a client whose locale is not UTF-8 (see
	// Sessions); the mark and the check are 1 or 0, and the path follows them.
	read := t.serverContext(ctx, suffix, "display-message", "-p", mark+" "+outlives(suffix)+" #{socket_path}")
	read.Stderr = nil
	out, _ := read.Output()
	marked, rest, _ := strings.Cut(strings.TrimSuffix(string(out), "\n"), " ")
	check, path, _ := strings.Cut(rest, " ")
	if marked == "0" {
		return false, &fail.Error{Status: 1, Message: fmt.Sprintf("tmux server cld-%s is not one of cld's", suffix), Advice: "; use another name"}
	}
	if other, found := strings.CutPrefix(filepath.Base(path), "cld-"); found && other != suffix && strings.EqualFold(other, suffix) {
		return false, &fail.Error{Status: 1,
			Message: fmt.Sprintf("session name '%s' clashes with session '%s': tmux's socket directory ignores case here, so both names reach server cld-%[2]s", suffix, other),
			Advice:  fmt.Sprintf(" (see tmux -L cld-%s ls)", other)}
	}
	refusal := &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' has ended, but its tmux server still runs", suffix),
		Advice:  fmt.Sprintf(" (see tmux -L cld-%s ls)", suffix)}
	if check != "1" {
		return false, refusal
	}
	refusal.Advice += "; end it with cld kill " + Options(suffix)
	return true, refusal
}

// outlives is a format that tmux makes 1 on a server that has outlived session cld-SUFFIX, and 0
// on any other: the server is one that cld started (see mark), it has sessions, none of them
// cld-SUFFIX - N/s compares whole names, as only does - and the base of its socket path, the one
// it was started on, is cld-SUFFIX. lingering reads it, and End's kill-server runs under it, in
// the same tmux command, so that the check and the kill see the same server. && takes two
// operands, and there is no !, before tmux 3.6.
func outlives(suffix string) string {
	return "#{&&:" + mark + ",#{&&:#{S:1},#{&&:#{==:#{N/s:cld-" + suffix + "},0},#{==:#{b:socket_path},cld-" + suffix + "}}}}"
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
// $TMUX is set, and only on the server TMUX names, if that is one of cld's - named cld-NAME, and
// started by cld (see mark): the terminal of any other tmux nests. tty names the terminal on its
// stdin, cld's.
func (t *Tmux) OwnPane() (string, bool) {
	socket, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	suffix, found := strings.CutPrefix(filepath.Base(socket), "cld-")
	if !found || !ValidName(suffix) {
		return "", false
	}
	tty, err := tool.Command("tty")
	if err != nil {
		return "", false
	}
	terminal, err := tty.Output()
	if err != nil {
		return "", false
	}
	panes := t.command("-S", socket, "list-panes", "-a", "-F", "#{?"+mark+",#{?pane_dead,,#{pane_tty}},}")
	panes.Stderr = nil
	live, err := panes.Output()
	if err != nil {
		return "", false
	}
	return suffix, slices.Contains(strings.Split(strings.TrimRight(string(live), "\n"), "\n"), strings.TrimRight(string(terminal), "\n"))
}

// Home is where the sessions made in the current directory belong, and where the NAME they take
// by default comes from (see DefaultName): the git repository the directory is in or, outside
// one, the directory itself. new and resume record Dir on the session as @cld-home, and join and
// kill refuse a session of the directory's NAME made in another (see foreign).
type Home struct {
	// Dir is the directory whose name DefaultName takes - the repository's (see repository), or
	// the current directory - by the path that gives the name; "" where there is none.
	Dir string
	// Repository is whether Dir is a git repository's.
	Repository bool
}

// Takes reports whether join and kill, run in home h, take a session whose @cld-home is made:
// where either is "" - a session of a cld that recorded no home, or no home here, as with -n - and
// where both are one directory. Paths that differ are compared as files: git names the common
// .git by its real path from a linked worktree, and from the main work tree by a path from the
// current directory, which the shell's PWD can name through a symbolic link; and a file system
// that ignores case, as macOS's does by default, finds a directory by paths in other cases.
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
// name of the git repository the current directory is in (see repository) or, outside one, of the
// current directory itself - /root gives root - made a NAME (see asName). Where nothing is left of
// the name, as for the root directory, it is "", and the session's name is SUFFIX alone.
func DefaultName() (string, Home) {
	name, dir, found := repository()
	if !found {
		// Getwd gives the directory as the shell's PWD names it, where that is the directory: the
		// name pwd shows, not that of the directory a symbolic link leads to.
		var err error
		if dir, err = os.Getwd(); err != nil {
			return "", Home{}
		}
		name = filepath.Base(dir)
	}
	return asName(name), Home{Dir: dir, Repository: found}
}

// Options is how -n and -s name session NAME on cld's command line: split at its last "-", -n
// what comes before it and -s what follows, where both are NAMEs; otherwise -s NAME alone, which
// names it where DefaultName is "" - in the root directory, say, where such a NAME is made.
func Options(name string) string {
	if i := strings.LastIndexByte(name, '-'); i > 0 && ValidName(name[:i]) && ValidName(name[i+1:]) {
		return "-n " + name[:i] + " -s " + name[i+1:]
	}
	return "-s " + name
}

// notInName is a run of the characters a NAME cannot have.
var notInName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// asName is name as the start of a NAME: each run of the characters a NAME cannot have becomes
// "-", and "-" and "_" go from either end - my.site is my-site, .dotfiles dotfiles, and "/"
// nothing at all.
func asName(name string) string {
	return strings.Trim(notInName.ReplaceAllString(name, "-"), "-_")
}

// repository is the name and the directory of the git repository the current directory is in,
// and whether it is in one: the directory that holds the .git every work tree of the repository
// shares, and its name, so that a linked worktree - one of claude's, under .claude/worktrees - and
// a subdirectory have the repository's name too; for a bare repository's worktree, and a
// submodule, whose git directory has a name of its own, that directory, and its name without
// ".git". A directory is in no repository outside a git work tree - in the .git directory, say -
// where git is missing, and where it fails. git gives the directory as a path from the current
// one, or else a whole path.
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

// Next is the NAME new gives a session without -s: prefix, then the index above the highest
// among the sessions named prefix and an index whose servers run, the entries of cld's record so
// named, and the highest index the record says was given after prefix (see indexes) - 0 where
// there is none, and gaps left as they are. A name comes back only once its entry has expired,
// with claude's conversation of that name, or has been forgotten, and the index given since has
// expired. A server that has outlived its session counts, and so does one that cld did not start,
// since new would refuse either name (see lingering). prefix is compared ignoring case, as a
// socket directory that ignores case would: its socket for a prefix in other letters would reach
// that server. Next reads the socket directory as Sessions does, and looks up only the servers
// whose sockets have such a name and an index at or above the one the record alone would give,
// from the highest index down, until one runs: that one decides, and in the common case it is the
// only server asked, as lookup runs no tmux for a stale socket. new and resume call it under the
// record's lock (see Lock), which they hold until tmux makes the session, so that two at once take
// two NAMEs. Once ctx is done, its tmux is killed.
func (t *Tmux) Next(ctx context.Context, prefix string) (string, error) {
	dir := socketDir()
	sockets, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return "", fail.Runtime(fmt.Sprintf("cannot read %s: %v", dir, err))
	}
	next := 0
	for _, r := range entries() {
		if start, index, ok := indexOf(r.Name); ok && strings.EqualFold(start, prefix) {
			next = max(next, index+1)
		}
	}
	if state, err := stateDir(); err == nil {
		for start, g := range indexes(state) {
			if strings.EqualFold(start, prefix) && g.Index < math.MaxInt {
				next = max(next, g.Index+1)
			}
		}
	}
	type indexed struct {
		suffix string
		index  int
	}
	var named []indexed
	for _, socket := range sockets {
		suffix, found := strings.CutPrefix(socket.Name(), "cld-")
		if !found || !ValidName(suffix) || len(suffix) <= len(prefix) || !strings.EqualFold(suffix[:len(prefix)], prefix) {
			continue
		}
		digits := suffix[len(prefix):]
		if strings.Trim(digits, "0123456789") != "" {
			continue
		}
		// An index too large for an int, or the largest, which no index is above, is none.
		index, err := strconv.Atoi(digits)
		if err != nil || index == math.MaxInt || index < next {
			continue
		}
		named = append(named, indexed{suffix, index})
	}
	slices.SortStableFunc(named, func(a, b indexed) int { return cmp.Compare(b.index, a.index) })
	for _, socket := range named {
		server, _, _, _, err := t.lookup(ctx, socket.suffix)
		if err != nil {
			return "", err
		}
		if server {
			return prefix + strconv.Itoa(socket.index+1), nil
		}
	}
	return prefix + strconv.Itoa(next), nil
}

// inWorkTree reports whether the current directory is in a git work tree.
func inWorkTree() bool {
	git, err := tool.Command("git", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	git.Stderr = nil
	out, _ := git.Output()
	return strings.TrimRight(string(out), "\n") == "true"
}

// checkTerminal refuses a terminal tmux could not attach from, for command - new, resume or join -
// once its other checks have passed: cld's stdin, which tmux takes for the client's terminal, has
// to be a terminal, and TERM set, not empty and not dumb. tmux would fail on either - "open
// terminal failed: not a terminal", "terminal does not support clear" (tmux 3.7c) - with no word
// of cld's, after cld had printed the title to a pipe or a file, and for new and resume only once
// it had started the server, whose socket stays behind for list and every TAB to ask (see
// Sessions). A TERM that names no terminal tmux knows is left to tmux, which says so.
func checkTerminal(command string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fail.Runtime(command + " needs a terminal, and its input is not one")
	}
	name, set := os.LookupEnv("TERM")
	switch {
	case !set:
		return fail.Runtime(command + " needs a terminal, and TERM is not set")
	case name == "":
		return fail.Runtime(command + " needs a terminal, and TERM is empty")
	case name == "dumb":
		return fail.Runtime(command + " needs a terminal, and TERM is dumb")
	}
	return nil
}

// printTitle prints Title for new, resume and join, where stdout is a terminal: tmux draws on the
// terminal of cld's stdin, and a stdout that is no terminal - cld new | tee, say - would only take
// the escape in as text.
func printTitle(suffix string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	return output.Print(Title(suffix))
}

// Title is what sets the terminal's title to the name of session cld-SUFFIX after the marker ✳,
// as new, resume and join print it before tmux starts, where stdout is a terminal (see
// printTitle), and the session list before it hands the terminal over: tmux, once attached, keeps
// the title itself (see titles).
func Title(suffix string) string {
	return "\033]0;✳ cld-" + suffix + "\007"
}

// titles is the title tmux gives the terminals on session cld-SUFFIX (its set-titles-string, with
// set-titles on for that session): the session's name after claude's marker - busyMarker while
// @cld-status is busy (see statusHooks), ✳ otherwise, and for a claude that exited, which a turn
// it failed in leaves busy - and " [w]" after it while @cld-worktree says claude is in a linked
// git worktree. claude's own title, #T, stays out: under tmux claude keeps a ✳ that never turns,
// and a program it runs could set another.
func titles(suffix string) string {
	return "#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-" + suffix + "#{?@cld-worktree, [w],}"
}

// busyMarker is claude's marker while it is busy, expanded with strftime (T:): ◐ in even seconds
// and ◑ in odd ones, as claude outside tmux turns them every 960 ms. With status off tmux has no
// timer to expand the title again, so the marker comes with a job that, a second later and in the
// background, refreshes the status of the terminal the title was expanded for - that is, its
// title - which expands the title, and so runs the job, again: tmux runs it at most once a second
// for each terminal, and not at all for a title that is not busy or a session no terminal is on.
// Each run is a sh, a sleep and a tmux client: some 0.5% of a core for each terminal while the
// title is busy, and 14% where tmux is slow to start, as Ubuntu's snap is.
// It names tmux by @cld-tmux, the path cld checked, quoted for the shell there: in the format, a
// "#" or a "%" would be taken for a format or a conversion of strftime's, and a ")" for the end of
// the job.
const busyMarker = "#{?#{m:*[02468],%S},◐,◑}" +
	"#((sleep 1; #{q:@cld-tmux} -S #{q:socket_path} refresh-client -S -t #{q:client_name}) >/dev/null 2>&1 &)"

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
// the end; when cmd cannot run at all, tool.CannotRun's message instead. A session lookup that
// fails ends cld with status 1 and that output, as the script's did with bash's message: a tmux
// that cannot run gets as far as a lookup only when it stops being runnable after answering
// tmux -V.
func combinedOutput(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) && out.Len() == 0 {
		out.WriteString(tool.CannotRun(cmd.Path, err).Error())
	}
	return strings.TrimRight(out.String(), "\n"), err
}

// become replaces cld with tmux, run with argv and env: the terminal's process is tmux from
// then on, and tmux's exit status is cld's. It returns only when that fails.
func (t *Tmux) become(argv, env []string) error {
	return tool.CannotRun(t.path, syscall.Exec(t.path, argv, env))
}

// exitStatus is the exit status of a tmux command that failed, which has said why, or how cld
// ends when it could not run tmux at all.
func (t *Tmux) exitStatus(err error) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return tool.CannotRun(t.path, err)
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return fail.Status(128 + int(status.Signal()))
	}
	return fail.Status(exit.ExitCode())
}
