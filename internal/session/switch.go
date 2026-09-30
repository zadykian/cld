package session

// A terminal on one of cld's sessions moves to another without leaving: C-q s shows cld's list over
// the session, and Enter there moves the terminal to the session picked; C-q ( and C-q ) move it to
// the previous and the next session that runs, in the list's order, and C-q L back to the session
// it came from; and cld join, run in a pane of the session - ! cld join in claude - moves it to the
// session it names (decision 51 in docs/design.md). tmux moves a client between the sessions of one
// server (switch-client), and each session of cld's has a server of its own: what moves the
// terminal is detach-client -E, which has the terminal's tmux client run a shell command in its own
// place, as if the terminal had run it after a detach - with the terminal's environment and
// directory, not the pane's (see Findings in docs/design.md). The command runs cld join, which
// attaches to the session, brings it back or creates it, as it would in the terminal.
//
// tmux runs the command with the session's default-shell, the user's shell - SHELL, or else the
// user's login shell, as the server started - which it also puts in the command's SHELL. So the
// command is words that sh, bash, zsh, fish, ksh and csh each read alike (see moving), and cld
// moves no terminal where default-shell is another shell (see checkShell): exec, cld by the file it
// runs from (see self), quoted where it needs to be as each of them reads it (see shellArgument),
// then join with options whose values are plain. The words join was given in a pane, and the
// directory it ran in, go encoded in one of them (see encodeMove): a word of claude's prompt, as
// the user's shell read it, would run as a command of its own, and tmux's parser, which reads the
// command within an if in a pane (see switchTerminal), drops what follows a newline in it. The
// session goes by -n and -s, or where its NAME cannot be split so, by -s alone in /, where the
// name of the directory leaves nothing (see DefaultName), encoded too.
//
// The keys are run-shell commands, bound on the server as create makes it (see switchKeys): a
// popup's command and its -e expand no format, and run-shell's do, so run-shell hands cld the
// terminal that pressed the key, #{client_name}, and cld list --switch CLIENT names it to
// detach-client -t. They run in the background (-b), so that the key returns at once, and print
// nothing, since tmux would show their output, or a status other than 0, over claude's pane: cld
// list --switch says what goes wrong on the terminal's message line instead (see Tmux.Tell). A
// server that an older cld started has tmux's own keys, choose-tree among them, until its session
// ends. The keys, and the command a terminal runs as it moves, name cld by its file, which cld
// update replaces in place: list --switch and --to, and join --switched-from and --moved, are
// options that every later release keeps, as it keeps completion SHELL, lest the keys of a session
// that an older cld started do nothing.
//
// C-q L goes back to the session that the terminal came from, as tmux's L does on one server: tmux
// knows the last session of each client, and a client that moves to another server is a new one
// there. So the session a switch moves a terminal to records the one it came from, as @cld-last on
// claude's session, which cld join sets as it attaches or makes the session (--switched-from); C-q
// L reads it from the session the terminal is on. It is the session's, not the terminal's: of two
// terminals on a session, C-q L takes each to where the one that came last came from.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
)

// Switch is a terminal on a session of one of cld's servers, which cld moves to another session:
// the server's socket, as TMUX names it, and the NAME of its session, cld-NAME; and client, the
// terminal's tmux client (#{client_name}) where a key named it (see SwitchClient). Where client is
// "", tmux finds the terminal as a bare cld detach does (see DetachTerminal): the one used last on
// the session of the pane cld runs in.
type Switch struct {
	socket, suffix, client string
}

// Switching is the terminal that join and the interactive list move where cld runs in a pane of
// one of cld's servers, as TMUX says, and nil elsewhere: where cld's stdin is a terminal, the pane
// has to be one of that server's live panes, and the server one that cld started (see OwnPane) - a
// shell in a window of the session, claude's external editor; where it is none, as for claude's !
// and Bash tool, which run a command with the pane's TMUX and TMUX_PANE and no terminal (see
// Findings in docs/design.md), the server has to have cld's mark, which tmux says (see mark). In a
// pane of any other tmux, and in a terminal that is no pane of the server TMUX names, cld attaches
// the terminal there, as in any other (see keptKeys).
func (t *Tmux) Switching() *Switch {
	socket, suffix, found := ownServer()
	if !found {
		return nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if _, own := t.OwnPane(); !own {
			return nil
		}
		return &Switch{socket: socket, suffix: suffix}
	}
	ask := t.command("-S", socket, "display-message", "-p", mark)
	ask.Stderr = nil
	marked, err := ask.Output()
	if err != nil || strings.TrimSuffix(string(marked), "\n") != "1" {
		return nil
	}
	return &Switch{socket: socket, suffix: suffix}
}

// SwitchClient is the terminal of tmux client client on the server TMUX names, which the session's
// keys name to cld list --switch (see switchKeys): the popup of C-q s, and the run-shell of C-q (,
// C-q ) and C-q L, have TMUX, and no TMUX_PANE (see Findings in docs/design.md).
func SwitchClient(client string) (*Switch, error) {
	socket, suffix, found := ownServer()
	if !found {
		return nil, fail.Runtime("list --switch moves a terminal on one of cld's sessions, and TMUX names none")
	}
	return &Switch{socket: socket, suffix: suffix, client: client}, nil
}

// SwitchJoin is join without -s in a pane of one of cld's servers (see Switching): it moves the
// terminal there to a new session, which the terminal's cld join makes under the next index - in
// the directory join runs in, where -w needs a git work tree, as create checks it (see move).
func (t *Tmux) SwitchJoin(sw *Switch, j Joining) error {
	if j.Worktree {
		if err := workTree(); err != nil {
			return err
		}
	}
	return t.move(sw, j)
}

// switchJoin is join -s in a pane of one of cld's servers (see Switching): it moves the terminal
// there to session cld-SUFFIX (see move), once it has refused what join would refuse before it
// starts claude, so that the terminal stays where it is and claude shows why: for a session that
// runs, one made in another repository or directory without -n (see foreign) and what would be
// lost (see Joining's lost); a server that runs without its session (see lingering); for a session
// that has ended, -w, which would be lost, and a directory it cannot enter (see enterable); and -w
// in a directory that is no git work tree where the session is made. Whether claude is too old the
// terminal's cld join says, as the claude it starts is the one on the terminal's PATH. The
// terminal's cld join looks the session up again, under the record's lock, and acts on what it
// finds then: this lookup takes no lock, and a session that another cld is starting (see starting)
// it leaves to that one, which the terminal's cld join waits for (see settled), rather than wait
// for it here, while claude waits for join.
func (t *Tmux) switchJoin(sw *Switch, suffix string, j Joining) error {
	if starting(suffix) {
		return t.move(sw, j)
	}
	server, exists, _, made, err := t.lookup(context.Background(), suffix)
	switch {
	case err != nil:
		return err
	case exists:
		if err := foreign("join", suffix, made, j.Home); err != nil {
			return err
		}
		if err := j.lost(suffix, false); err != nil {
			return err
		}
	case server:
		_, refused := t.lingering(context.Background(), suffix)
		return refused
	default:
		if r, ended := recorded(suffix); ended && !j.New && j.Conversation == "" {
			if err := j.lost(suffix, true); err != nil {
				return err
			}
			if err := enterable(r); err != nil {
				return err
			}
		} else if j.Worktree {
			if err := workTree(); err != nil {
				return err
			}
		}
	}
	return t.move(sw, j)
}

// move moves the terminal of sw as join, run in a pane of sw's server with the words Typed after
// it, asks: the terminal runs cld join --moved, which carries those words and the directory join
// runs in (see encodeMove), and runs join with them there (see Moved), where the NAME of the
// session comes from as it does for join here (see DefaultName) and where claude starts, with
// --switched-from before them, so that the session records where the terminal came from (see the
// top of this file). The directory has to be there, as join refuses one that has gone (see
// workingDirectory).
func (t *Tmux) move(sw *Switch, j Joining) error {
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	return t.switchTerminal(sw, "--moved="+encodeMove(dir, j.Typed))
}

// SwitchTo moves the terminal of sw to session cld-SUFFIX, which runs or has ended, as cld join
// -n NAME -s SUFFIX joins it - brought back where it has ended - for the interactive list's Enter
// in the popup of C-q s or in a pane of the server, and for C-q (, C-q ) and C-q L; a NAME with no
// such split, as cld join -s NAME joins it in /. The terminal's own session it leaves as it is: the
// terminal is there already.
func (t *Tmux) SwitchTo(sw *Switch, suffix string) error {
	if suffix == sw.suffix {
		return nil
	}
	if name, index, ok := split(suffix); ok {
		return t.switchTerminal(sw, "-n", name, "-s", index)
	}
	return t.switchTerminal(sw, "--moved="+encodeMove("/", []string{"-s", suffix}))
}

// encodeMove is the value of join's --moved that carries directory dir and words to the terminal
// that a join in a pane moves (see the top of this file): dir and each word, each after the one
// before and a NUL, which no argument has, in URL-safe base64 without padding - letters, digits,
// "-" and "_", which every shell reads as they are, and tmux's parser too.
func encodeMove(dir string, words []string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(append([]string{dir}, words...), "\x00")))
}

// Moved is join --moved, the command that a terminal runs as a join in a pane of one of cld's
// servers moves it (see Tmux.move): it goes to the directory value carries, the one that join ran
// in, where PWD names it as that join's did, and returns the words that join was given, which the
// terminal's join runs (see encodeMove). A value that carries no absolute directory is refused,
// status 2, and a directory it cannot enter, status 1, the terminal having left its session.
func Moved(value string) ([]string, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	fields := strings.Split(string(data), "\x00")
	if err != nil || !filepath.IsAbs(fields[0]) {
		return nil, fail.Usage(fmt.Sprintf("invalid value '%s' for --moved (see cld help)", value))
	}
	dir := fields[0]
	if err := os.Chdir(dir); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return nil, fail.Runtime("cannot enter " + dir + ", where cld join ran: " + err.Error())
	}
	if err := os.Setenv("PWD", dir); err != nil {
		return nil, fail.Runtime(err.Error())
	}
	return fields[1:], nil
}

// Step is C-q (, C-q ) and C-q L on the terminal of sw, as to says: previous and next move it to
// the session before and after its own among those that run, in the list's order - the order of
// their names - the last after the first and the first after the last, as tmux's own keys go
// round the sessions of one server; last moves it back to the session its own recorded as the one
// the terminal came from (see the top of this file). Where there is none to move to - no other
// session runs, none was recorded, or the one recorded does not run - it says so on the terminal's
// message line (see Tell). Once ctx is done, its tmux is killed.
func (t *Tmux) Step(ctx context.Context, sw *Switch, to string) error {
	sessions, err := t.Sessions(ctx)
	if err != nil {
		return err
	}
	var running []string
	for _, s := range sessions {
		if s.State != Ended {
			running = append(running, s.Name)
		}
	}
	var target string
	switch to {
	case "previous":
		for _, name := range running {
			if name < sw.suffix {
				target = name
			}
		}
		if target == "" && len(running) > 0 {
			target = running[len(running)-1]
		}
	case "next":
		if i := slices.IndexFunc(running, func(name string) bool { return name > sw.suffix }); i >= 0 {
			target = running[i]
		} else if len(running) > 0 {
			target = running[0]
		}
	default:
		last, err := t.last(ctx, sw.suffix)
		if err != nil {
			return err
		}
		switch {
		case last == "":
			t.Tell(sw, "no session to go back to")
			return nil
		case !slices.Contains(running, last) && slices.ContainsFunc(sessions, func(s Session) bool { return s.Name == last }):
			t.Tell(sw, "session '"+last+"' has ended")
			return nil
		case !slices.Contains(running, last):
			t.Tell(sw, "no session '"+last+"'")
			return nil
		}
		target = last
	}
	if target == "" || target == sw.suffix {
		t.Tell(sw, "no other session runs")
		return nil
	}
	return t.SwitchTo(sw, target)
}

// Tell shows text on the message line of the terminal of sw, after "cld: ", for three seconds or
// until a key, which then reaches claude, as tmux shows its own messages: for C-q (, C-q ) and C-q
// L where there is no session to move to, and for what goes wrong in cld list --switch, whose
// output the popup takes away as it closes, and tmux would show over claude's pane from the keys'
// run-shell (see switchKeys). -C keeps claude's pane drawn meanwhile, where tmux would draw nothing
// more of it until the message goes (see showKept): tmux 3.5, which has no -C, holds it back for
// those three seconds at most. A message tmux cannot show is lost.
func (t *Tmux) Tell(sw *Switch, text string) {
	if sw.client == "" {
		return
	}
	args := []string{"-S", sw.socket, "display-message", "-c", sw.client, "-d", "3000"}
	if !t.older(version{3, 6, 0}) {
		args = append(args, "-C")
	}
	tell := t.command(append(args, unexpanded("cld: "+text))...)
	tell.Stdout, tell.Stderr = nil, nil
	_ = tell.Run()
}

// last is the session that session cld-SUFFIX recorded as the one the terminal on it came from
// (see the top of this file), or "" where it recorded none. Once ctx is done, its tmux is killed.
func (t *Tmux) last(ctx context.Context, suffix string) (string, error) {
	out, err := combinedOutput(t.serverContext(ctx, suffix, "list-sessions", "-f", only(suffix), "-F", "#{@cld-last}"))
	if err != nil {
		return "", fail.Runtime(out)
	}
	last, _, _ := strings.Cut(out, "\n")
	if !ValidName(last) {
		return "", nil
	}
	return last, nil
}

// switchTerminal has the terminal of sw run cld join with words in its tmux client's place (see
// moving), once it has checked the shell that tmux runs it with (see checkShell). A terminal a key
// named goes by its client, which detach-client -t takes; the popup it runs in closes with the
// detach, and tmux ends the list in it, before or after this tmux command returns (see Findings in
// docs/design.md), so nothing follows. Otherwise tmux finds the terminal as a bare cld detach does
// (see DetachTerminal), by cld's stdin, handed to it, and else by TMUX_PANE, and runs
// detach-client only where a terminal is on the pane's session - a bare one would take a terminal
// of another session of the server, one claude made - and the server has cld's mark: without
// either, cld refuses, since there is no terminal to move. The mark and the count of terminals come
// first, in the same tmux command, and the command goes quoted for tmux's parser, which reads a
// word quoted as sh's is, within the if's command. The words join was given in a pane go encoded,
// a third longer (see encodeMove), and where they make tmux's command longer than tmux takes (see
// commandLimit), join refuses them, as it would where it made the session.
func (t *Tmux) switchTerminal(sw *Switch, words ...string) error {
	cld, err := self()
	if err != nil {
		return fail.Runtime("cannot find the file cld runs from: " + err.Error())
	}
	command := moving(cld, sw.suffix, words)
	if err := t.checkShell(sw); err != nil {
		return err
	}
	if sw.client != "" {
		detach := t.command("-S", sw.socket, "detach-client", "-t", sw.client, "-E", literal(command))
		if err := detach.Run(); err != nil {
			return t.exitStatus(err)
		}
		return nil
	}
	args := []string{"display-message", "-p", mark + " #{session_attached}", ";",
		"if", "-F", "#{&&:" + mark + ",#{session_attached}}", "detach-client -E " + shellWord(command)}
	if size := commandSize(args); size > commandLimit {
		return fail.Usage(fmt.Sprintf("join's words make tmux's command %d bytes, and tmux takes %d at most: give claude long text in a file, as with --append-system-prompt-file", size, commandLimit))
	}
	detach := t.command(append([]string{"-S", sw.socket}, args...)...)
	out, err := detach.Output()
	if err != nil {
		return t.exitStatus(err)
	}
	marked, attached, _ := strings.Cut(strings.TrimSuffix(string(out), "\n"), " ")
	switch {
	case marked != "1":
		return &fail.Error{Status: 1, Message: "tmux server cld-" + sw.suffix + " is not one of cld's", Advice: "; run cld join in a terminal (see cld help)"}
	case attached == "0":
		return &fail.Error{Status: 1, Message: "no terminal is attached to this session for join to move", Advice: "; run cld join in a terminal (see cld help join)"}
	}
	return nil
}

// moving is the command the terminal runs in its tmux client's place: exec of cld, the file cld at
// its path, as join --switched-from FROM, which records on the session joined the session FROM the
// terminal leaves, with words, each as the user's shell takes it (see shellArgument).
func moving(cld, from string, words []string) string {
	command := "exec " + shellArgument(cld) + " join --switched-from " + shellArgument(from)
	for _, word := range words {
		command += " " + shellArgument(word)
	}
	return command
}

// shells are the base names of the default-shells that tmux may run the command a terminal runs in
// its client's place with (see moving): sh and the shells that read it as sh does, bash, zsh, fish,
// ksh, csh and theirs, each of which has exec and reads a word bare or quoted alike (see
// shellArgument).
var shells = []string{"sh", "ash", "dash", "bash", "ksh", "mksh", "oksh", "yash", "zsh", "fish", "csh", "tcsh"}

// checkShell refuses to move the terminal of sw where tmux would run the command with a
// default-shell other than those the command is written for (see shells) - nu or pwsh, say - which
// could leave the terminal out of any session, running nothing: tmux runs it with the
// default-shell of the terminal's session, which a display-message of the terminal's client, or of
// the pane cld runs in, shows (see switchTerminal).
func (t *Tmux) checkShell(sw *Switch) error {
	args := []string{"-S", sw.socket, "display-message", "-p"}
	if sw.client != "" {
		args = append(args, "-c", sw.client)
	}
	out, err := t.command(append(args, "#{default-shell}")...).Output()
	if err != nil {
		return t.exitStatus(err)
	}
	shell := strings.TrimSuffix(string(out), "\n")
	if slices.Contains(shells, filepath.Base(shell)) {
		return nil
	}
	return &fail.Error{Status: 1, Message: "cannot move the terminal with tmux's default-shell '" + shell + "': cld writes the move for sh, bash, zsh, fish, ksh and csh alone",
		Advice: "; detach with C-q d and run cld join (see cld help join)"}
}

// plain matches a word that sh, bash, zsh, fish, ksh and csh each read as it is, unquoted:
// letters, digits and "_@+:,./-", and "=" but at its start, where zsh would take =word for the
// path of the program word.
var plain = regexp.MustCompile(`^[A-Za-z0-9_@+:,./-][A-Za-z0-9_@+=:,./-]*$`)

// shellArgument is word as one word of the command a terminal runs in its client's place (see
// moving): as it is where it is plain, and otherwise within single quotes, but for each "'" and
// "\", which go outside them with a "\" before them. sh, bash, zsh, fish, ksh and csh read that
// alike (see Findings in docs/design.md), where fish reads a "\" before a "\" or a "'" within
// single quotes as an escape, and sh as a "\": quoted as sh quotes it (see shellWord), a word would
// end its quotes early in fish, and what follows run as a command of its own.
func shellArgument(word string) string {
	if plain.MatchString(word) {
		return word
	}
	var quoted, run strings.Builder
	quote := func() {
		if run.Len() > 0 {
			quoted.WriteString("'" + run.String() + "'")
			run.Reset()
		}
	}
	for i := 0; i < len(word); i++ {
		if b := word[i]; b == '\'' || b == '\\' {
			quote()
			quoted.WriteByte('\\')
			quoted.WriteByte(b)
		} else {
			run.WriteByte(b)
		}
	}
	quote()
	if quoted.Len() == 0 {
		return "''"
	}
	return quoted.String()
}

// switchKeys are the keys that create binds in the prefix table of a session's server, as tmux's
// words, each binding followed by a ";" (see the top of this file), for cld at the path cld, and
// tmux at the path tmux on the server's socket: s shows cld list --switch in a popup over the
// whole terminal, without a border, as tmux's own s shows choose-tree over the pane; (, ) and L
// run cld list --switch --to previous, next or last. Each is a run-shell -b of sh, whose command
// run-shell expands as a format: #{q:client_name} is the terminal's client, quoted for sh, and
// each "#" of the paths is doubled, as "##" expands to "#". tmux hands the words of the popup's
// command to the program as they are, with no shell (see Findings in docs/design.md), but reads
// cld's path as it reads each word of a command, where one ending in ";" would end it (see
// literal). The keys print nothing and exit 0 whatever happens: cld list --switch says on the
// message line what goes wrong (see Tell). None where cld cannot find the file it runs from, cld
// "": tmux's own keys stay.
func switchKeys(tmux, socket, cld string) []string {
	if cld == "" {
		return nil
	}
	client := "#{q:client_name}"
	quiet := func(sh string) string { return sh + " >/dev/null 2>&1 || true" }
	list := unexpanded(shellWord(cld)) + " list --switch " + client
	popup := unexpanded(shellWord(tmux)) + " -S " + unexpanded(shellWord(socket)) + " display-popup -c " + client +
		" -B -w 100% -h 100% -E " + unexpanded(shellWord(literal(cld))) + " list --switch " + client
	return []string{
		"bind", "s", "run-shell", "-b", quiet(popup), ";",
		"bind", "(", "run-shell", "-b", quiet(list + " --to previous"), ";",
		"bind", ")", "run-shell", "-b", quiet(list + " --to next"), ";",
		"bind", "L", "run-shell", "-b", quiet(list + " --to last"), ";",
	}
}

// self is the file cld runs from, with its symbolic links resolved, which cld update replaces in
// place: the keys and the command a terminal runs in its client's place name cld by it. A path
// with a control character in it, a newline say, is none that tmux's parser and every shell take
// alike (see shellArgument): the keys stay tmux's own, and a move is refused.
func self() (string, error) {
	file, err := os.Executable()
	if err == nil {
		file, err = filepath.EvalSymlinks(file)
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	if err == nil && strings.ContainsFunc(file, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return "", errors.New("its path, " + strconv.Quote(file) + ", has a control character")
	}
	return file, err
}

// workTree refuses, for -w, a current directory that is in no git work tree, as create does: cld
// reports it in the terminal, where claude would report it in a session left to kill.
func workTree() error {
	if inWorkTree() {
		return nil
	}
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	return fail.Runtime("--worktree needs a git repository, and " + dir + " is not in one")
}

// split splits session NAME into -n's NAME and -s's SUFFIX at its last "-", where both are NAMEs:
// the split cld's messages name a session by (see Options); false where it has no such split.
func split(name string) (string, string, bool) {
	if i := strings.LastIndexByte(name, '-'); i > 0 && ValidName(name[:i]) && ValidName(name[i+1:]) {
		return name[:i], name[i+1:], true
	}
	return "", "", false
}
