// Package session is cld's tmux side: it runs Claude Code in named sessions on a private tmux
// server.
//
// `cld new -n NAME` creates the tmux session "cld-NAME" (NAME defaults to "main") running
// `claude --name cld-NAME` in the current directory, with Remote Control on from the start, and
// attaches to it; with -w claude also gets --worktree NAME, makes git worktree NAME from HEAD or
// reopens it, and works there.
// `cld join -n NAME` attaches to the session again, `cld kill -n NAME` ends it, and `cld list`
// shows the sessions; all three see only the sessions cld started (see the constant mine). A
// private server (-L cld, no ~/.tmux.conf) keeps these options away from any other tmux use:
//
//   - extended-keys on: tmux answers no kitty keyboard query, so claude falls back to
//     modifyOtherKeys, which tmux forwards only when this is on (Shift+Enter and friends)
//   - the extkeys terminal feature for xterm*: tmux asks the terminal for modified keys only when
//     it knows the terminal supports them, and it does not recognise every terminal that does;
//     Claude Code's docs recommend this for tmux. It goes to a fixed index past tmux's defaults:
//     set -a would add another copy every time cld creates a session
//   - mouse on, focus-events on: claude probes both and hints when they are off. With the mouse
//     on, the wheel over a program that draws in the main screen without the mouse - claude
//     outside fullscreen, a shell - scrolls the pane's history; claude's fullscreen transcript
//     gets the wheel either way, as tmux passes claude's own mouse reporting on to the terminal
//   - allow-passthrough on: claude wraps its notifications and OSC 52 copies in tmux passthrough
//   - status off: claude keeps the whole tab
//   - prefix C-q: claude binds C-b (background a task) and nearly every other Ctrl key, but not
//     C-q; detach is C-q d, and C-q C-q sends a C-q through
//   - remain-on-exit failed, from tmux 3.5: a claude that fails - at startup, say, for a worktree
//     in a directory it does not trust - leaves its pane on screen with its message, instead of
//     taking both away; /exit and claude's other ways out exit with status 0. An empty
//     remain-on-exit-format keeps tmux from scrolling the pane for its own line, which would push
//     a short error at the top out of sight; the pane-died hook shows how to end the session on
//     the message line instead, until a key is pressed. It names the session through its one
//     window, named NAME: the hook's formats know the pane and its window, not the session. The
//     hook shows it only to a terminal on that window: tmux would show it on another session's
//     terminal, or with none attached keep it and show it in view-mode over the next session a
//     terminal attaches to, which then takes no keys until q; join shows it instead. tmux 3.3 and
//     3.4 keep writing focus reports to a dead pane that had them on, as claude's has, and crash -
//     with every session on the server - when the terminal's focus changes or it detaches, so
//     there remain-on-exit stays off. These go to claude's window only, not the server (see
//     Tmux.New)
//
// join attaches with -d, detaching other clients. tmux keeps claude's title changes to the pane
// (set-titles is off), so the tab keeps the session name. The server keeps the environment of
// the client that started it, and claude trusts TERMINAL_EMULATOR over TERM_PROGRAM=tmux: a
// server started from the JetBrains terminal would make every claude on it act as if in
// JediTerm (extended keys off, so Shift+Enter submits) even when attached from iTerm2 - hence
// new leaves TERMINAL_EMULATOR out of the environment it runs tmux with.
package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/zadykian/cld/internal/fail"
)

// validName matches a session NAME, in ASCII whatever the locale. tmux turns "." and ":" into
// "_" in session names and claude's --name would keep them, so names are validated instead of
// silently diverging.
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidName reports whether name is a valid session NAME.
func ValidName(name string) bool { return validName.MatchString(name) }

// Tmux is the tmux cld runs, found and checked by Check.
type Tmux struct {
	path string
	// remain is remain-on-exit for claude's window: "failed" from tmux 3.5, "off" before (see
	// the package comment).
	remain string
}

// tmuxVersion matches the start of a version tmux -V reports.
var tmuxVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)`)

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
	t := &Tmux{path: path, remain: "failed"}
	// allow-passthrough needs tmux 3.3, keeping failed sessions 3.5 (see the package comment);
	// "tmux next-3.6" and "tmux master" are development builds.
	out, err := t.command("-V").Output()
	if err != nil {
		return nil, t.exitStatus(err)
	}
	found := strings.TrimRight(string(out), "\n")
	version := strings.TrimPrefix(found[strings.LastIndex(found, " ")+1:], "next-")
	if match := tmuxVersion.FindStringSubmatch(version); match != nil {
		major, minor := number(match[1]), number(match[2])
		if major < 3 || major == 3 && minor < 3 {
			return nil, fail.Runtime(fmt.Sprintf("tmux 3.3 or newer is required, found '%s'", found))
		}
		if major == 3 && minor < 5 {
			t.remain = "off"
		}
	}
	return t, nil
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

// New creates session cld-SUFFIX, running claude in the current directory, and becomes a tmux
// client attached to it; with worktree claude works in git worktree SUFFIX. It returns only when
// it does not get as far.
func (t *Tmux) New(suffix string, worktree bool) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	name := "cld-" + suffix
	exists, err := t.hasSession(suffix)
	if err != nil {
		return err
	}
	if exists {
		dead, _ := t.command("-L", "cld", "list-panes", "-t", "="+name, "-F", "#{pane_dead}").Output()
		if strings.TrimRight(string(dead), "\n") == "1" {
			return fail.Runtime(fmt.Sprintf("session '%s' exists, but its claude exited; end it with cld kill -n %[1]s", suffix))
		}
		return fail.Runtime(fmt.Sprintf("session '%s' exists; attach to it with cld join -n %[1]s", suffix))
	}
	// The script went on with the PWD it got where that directory had been removed, and tmux
	// started claude in the home directory instead.
	dir, err := os.Getwd()
	if errors.Is(err, fs.ErrNotExist) {
		return fail.Runtime("the current directory no longer exists")
	}
	if err != nil {
		return fail.Runtime("cannot get the current directory: " + err.Error())
	}
	// Settings given on claude's command line override the user's and the project's. Remote
	// Control starts with the session, so it can be reached from claude.ai and the mobile app;
	// claude still keeps it off where org policy or the project's own settings turn it off.
	given := settings{RemoteControlAtStartup: true}
	if worktree {
		// cld reports a missing repository in the terminal; claude would report it in a session
		// left to kill or, with tmux 3.3 and 3.4, in a pane that closes as claude exits.
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
	claude := []string{"claude", "--name", name, "--settings", string(encoded)}
	if worktree {
		claude = append(claude, "--worktree", suffix)
	}
	if err := setTitle(name); err != nil {
		return err
	}
	// claude and its arguments go to tmux as separate words: tmux then executes them directly
	// instead of through sh -c. What follows new-session in the same tmux command - the mark,
	// remain-on-exit and the pane-died hook, which go to claude's window only, so that a session
	// made on the server without cld closes as tmux would - takes effect before tmux sees claude
	// exit, however soon; tmux cuts the command short when new-session fails, so a session of
	// that name made meanwhile stays unmarked. The targets end in ":" because set takes a pane,
	// which "=NAME" does not find.
	window := "=" + name + ":"
	argv := []string{"tmux", "-L", "cld", "-f", "/dev/null",
		"set", "-s", "extended-keys", "on", ";", "set", "-s", "terminal-features[100]", "xterm*:extkeys", ";",
		"set", "-s", "focus-events", "on", ";",
		"set", "-g", "mouse", "on", ";", "set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
		"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
		"new-session", "-s", name, "-n", suffix, "-c", dir}
	argv = append(argv, claude...)
	argv = append(argv, ";",
		"set", "-F", "-t", window, "@cld", "#{session_id}", ";",
		"set", "-w", "-t", window, "remain-on-exit", t.remain, ";",
		"set", "-w", "-t", window, "remain-on-exit-format", "", ";",
		"set-hook", "-w", "-t", window, "pane-died", "if -F '#{window_active_clients}' \""+hint+"\"")
	// The server keeps the environment of the client that starts it (see the package comment).
	return t.become(argv, slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, "TERMINAL_EMULATOR=")
	}))
}

// Join becomes a tmux client attached to session cld-SUFFIX, detaching any other. It returns
// only when it does not get as far.
func (t *Tmux) Join(suffix string) error {
	if err := t.readyClient(); err != nil {
		return err
	}
	exists, err := t.hasSession(suffix)
	if err != nil {
		return err
	}
	if !exists {
		return fail.Runtime(fmt.Sprintf("no session '%s'; create it with cld new -n %[1]s", suffix))
	}
	name := "cld-" + suffix
	if err := setTitle(name); err != nil {
		return err
	}
	// After attach-session in one command list, the hint goes to this terminal, attached by then.
	return t.become([]string{"tmux", "-L", "cld", "attach-session", "-d", "-t", "=" + name, ";",
		"if", "-F", "#{pane_dead}", hint}, os.Environ())
}

// Kill ends session cld-SUFFIX. claude gets SIGHUP, as when its terminal closes; an attached
// terminal is detached. A kill-session that fails ends cld with its status, after its message.
func (t *Tmux) Kill(suffix string) error {
	exists, err := t.hasSession(suffix)
	if err != nil {
		return err
	}
	if !exists {
		return fail.Runtime(fmt.Sprintf("no session '%s' (see cld list)", suffix))
	}
	kill := t.command("-L", "cld", "kill-session", "-t", "=cld-"+suffix)
	kill.Stdout = os.Stdout
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
	// Directory is the directory claude is in now, or once it has exited the one its session
	// started in.
	Directory string
}

// Sessions reads the sessions cld started, in tmux's order; none when its server is not running.
func (t *Tmux) Sessions() ([]Session, error) {
	// pane_current_path is the directory claude is in now, not the one its session started in;
	// once claude has exited there is none, and list shows where the session started. Only the
	// sessions cld started hold their id in @cld (see the constant mine).
	state := "#{?pane_dead,exited,#{?session_attached,attached,detached}}"
	path := "#{?pane_dead,#{session_path},#{pane_current_path}}"
	// tmux writes to a client whose LC_ALL, LC_CTYPE or LANG does not name UTF-8 - unset or C, as
	// over ssh, in containers and cron - with "_" for each character it cannot print: the tabs,
	// and any non-ASCII letter in a directory. -u marks the client UTF-8, so the output arrives
	// as it is.
	out, err := combinedOutput(t.command("-u", "-L", "cld", "list-sessions", "-f", mine, "-F",
		"#{session_name}\t"+state+"\t"+path))
	if err != nil {
		if noServer(out) {
			return nil, nil
		}
		return nil, fail.Runtime(out)
	}
	var sessions []Session
	for _, line := range strings.Split(out, "\n") {
		name, state, directory := fields(line)
		if name, found := strings.CutPrefix(name, "cld-"); found {
			sessions = append(sessions, Session{Name: name, State: state, Directory: directory})
		}
	}
	return sessions, nil
}

// fields splits a line of Sessions' output: runs of tabs separate the fields, tabs around the
// line are dropped, and the directory takes the rest of it.
func fields(line string) (name, state, directory string) {
	rest := strings.Trim(line, "\t")
	name, rest, _ = strings.Cut(rest, "\t")
	state, rest, _ = strings.Cut(strings.TrimLeft(rest, "\t"), "\t")
	return name, state, strings.TrimLeft(rest, "\t")
}

// mine holds for the sessions cld started. new marks them: the user option @cld holds the
// session's id. Whatever claude runs inherits TMUX, which takes a bare tmux to cld's server, and
// a session made there that way - or by hand with tmux -L cld - is not cld's. The mark is the id
// rather than a flag because a format looks @cld up in the server's, the pane's and the window's
// options before the session's, and a flag set there would mark any session.
const mine = "#{==:#{@cld},#{session_id}}"

// hasSession reports whether session cld-SUFFIX exists; a session of that name that cld did not
// start is an error. The filter compares whole names, where a target "cld-rev" would find
// cld-review. No server, no session.
func (t *Tmux) hasSession(suffix string) (bool, error) {
	filter := "#{==:#{session_name},cld-" + suffix + "}"
	found, err := combinedOutput(t.command("-L", "cld", "list-sessions", "-f", filter, "-F", "#{?"+mine+",cld,other}"))
	if err != nil {
		if noServer(found) {
			return false, nil
		}
		return false, fail.Runtime(found)
	}
	if found == "other" {
		return false, fail.Runtime(fmt.Sprintf("session '%s' exists, but cld did not start it (see tmux -L cld ls)", suffix))
	}
	return found == "cld", nil
}

// noServer reports whether tmux failed, saying message, because cld's server is not running.
func noServer(message string) bool {
	return strings.HasPrefix(message, "no server running on ") || strings.HasPrefix(message, "error connecting to ")
}

// tmux refuses to attach a client with $TMUX set whose tty has the name of any pane on the server.
// A dead pane - a failed claude's - keeps the name of its closed pty, and the system hands the name
// to the next terminal opened: inside another tmux, tmux would refuse that terminal as nested. So
// cld makes the check itself, on the live panes, and gives its client an empty TMUX, which tmux's
// check skips; set, even empty, TMUX still tells the client that the terminal takes UTF-8.

// readyClient readies cld to become a tmux client: it refuses a terminal that is a live pane of
// cld's server, and empties a TMUX that is set, for the client and every tmux command before it.
func (t *Tmux) readyClient() error {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	if t.ownPane() {
		return fail.Runtime("this terminal is a pane of cld's tmux server; detach with C-q d first")
	}
	return os.Setenv("TMUX", "")
}

// ownPane reports whether this terminal is a live pane of cld's server - claude's external
// editor, say - where a session attached would show inside itself. tmux refuses that too,
// advising to unset $TMUX, but goes by name, dead panes included (see above); cld says how to get
// out instead, and like tmux looks only when $TMUX is set. tty names the terminal on its stdin,
// cld's.
func (t *Tmux) ownPane() bool {
	tty, err := tool("tty")
	if err != nil {
		return false
	}
	terminal, err := tty.Output()
	if err != nil {
		return false
	}
	panes := t.command("-L", "cld", "list-panes", "-a", "-F", "#{?pane_dead,,#{pane_tty}}")
	panes.Stderr = nil
	live, err := panes.Output()
	if err != nil {
		return false
	}
	return slices.Contains(strings.Split(strings.TrimRight(string(live), "\n"), "\n"), strings.TrimRight(string(terminal), "\n"))
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

// setTitle sets the terminal's title to the session's name, which the tab keeps: tmux keeps
// claude's own title changes to its pane.
func setTitle(name string) error {
	return fail.Print("\033]0;✳ " + name + "\007")
}

// command is a tmux command run with cld's stdin and stderr, as tmux.
func (t *Tmux) command(args ...string) *exec.Cmd {
	cmd := exec.Command(t.path, args...)
	cmd.Args[0] = "tmux"
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	return cmd
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
