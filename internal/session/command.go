package session

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// serverOptions are the options create gives a server, before new-session in the same command, as
// docs/design/overview.md#the-servers-options gives them with their reasons.
var serverOptions = []string{
	"set", "-s", "@cld", "1", ";",
	"set", "-s", "extended-keys", "on", ";",
	"set", "-s", "terminal-features[100]", "xterm*:extkeys:hyperlinks", ";",
	"set", "-s", "terminal-features[101]", "wezterm:hyperlinks", ";",
	"set", "-s", "terminal-features[102]", "alacritty:hyperlinks", ";",
	"set", "-s", "focus-events", "on", ";",
	"set", "-g", "mouse", "on", ";",
	"unbind", "-n", "C-MouseDown1Pane", ";", "unbind", "-n", "M-MouseDown3Pane", ";",
	"set", "-g", "allow-passthrough", "on", ";", "set", "-g", "status", "off", ";",
	"set", "-g", "history-limit", "50000", ";",
	"set", "-g", "prefix", "C-q", ";", "bind", "C-q", "send-prefix", ";",
}

// command is tmux's command that makes the session, and its size as tmux counts it (see
// commandLimit). Where file is not "", claude's hooks keep the session's entry there. tmux then
// sets the marks beside it once the session is made, the run mark run where that is not "".
func (cr *creation) command(file, run string) (argv []string, size int, err error) {
	claude, err := cr.claudeWords(file)
	if err != nil {
		return nil, 0, err
	}
	// -u takes the terminal for UTF-8 whatever the locale (see the package comment).
	argv = []string{"tmux", "-u", "-L", "cld-" + cr.suffix, "-f", "/dev/null"}
	options := len(argv)
	argv = append(argv, serverOptions...)
	argv = append(argv, cr.keys...)
	argv = append(argv, cr.newSession(claude)...)
	argv = append(argv, cr.sessionOptions()...)
	argv = append(argv, cr.marks(file, run)...)
	argv = append(argv, showKept(cr.l.kept)...)
	return argv, commandSize(argv[options:]), nil
}

// claudeWords are claude and its arguments: the session's name, the settings, and the launch's
// words after cld's own (decision 41.1). The settings' hooks keep the entry in file, if not "".
func (cr *creation) claudeWords(file string) ([]string, error) {
	// Flag settings outrank the user's, so they carry only what cld needs (decision 42.2), agent
	// view off among it (decision 47). They go again with a resume, which does not keep them.
	git, _ := tool.LookPath("git") //nolint:errcheck // none leaves out the worktree's hooks
	given := settings{DisableAgentView: true,
		Hooks: statusHooks(cr.t.path, git, cr.socket, cr.suffix)}
	if file != "" {
		for event, hooks := range recordHooks(file, cr.suffix, cr.dir) {
			given.Hooks[event] = append(given.Hooks[event], hooks...)
		}
	}
	if cr.l.worktree {
		// claude branches a new worktree from the remote's default branch unless
		// worktree.baseRef is "head".
		given.Worktree.BaseRef = "head"
	}
	encoded, err := settingsJSON(given)
	if err != nil {
		return nil, err
	}
	// The name goes with a resume too, so that the next one finds the conversation (decisions
	// 16.2 and 45).
	name := "cld-" + cr.suffix
	claude := []string{cr.claude, "--name", name, "--settings", encoded}
	if cr.l.worktree {
		claude = append(claude, "--worktree", name)
	}
	claude = append(claude, cr.l.resume...)
	return append(claude, cr.l.args...), nil
}

// settingsJSON is given as JSON without HTML's escapes, so that a hook's 2>/dev/null reads as
// such, not as 2>/dev/null (decision 26.3).
func settingsJSON(given settings) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(given); err != nil {
		return "", fail.Runtime(err.Error())
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

// newSession is new-session with claude's words, each a word of its own, which tmux runs without
// sh -c. The directory goes as a format that expands to itself (decision 16.8).
func (cr *creation) newSession(claude []string) []string {
	words := []string{"new-session"}
	if cr.l.detached {
		words = append(words, "-d")
	}
	words = append(words, "-s", "cld-"+cr.suffix, "-n", cr.suffix, "-c", literal(unexpanded(cr.dir)))
	for _, word := range claude {
		words = append(words, literal(word))
	}
	return words
}

// sessionOptions follow new-session, which tmux skips where it fails. claude's pane gets the
// failure's options and hook (decision 5), and passthrough while a redraw is due (decision 55).
// Its session gets the title's (decision 25.4), its home and its tmux. "=NAME:" is the session's
// active pane, claude's, as set takes a pane.
func (cr *creation) sessionOptions() []string {
	target := "=cld-" + cr.suffix + ":"
	options := []string{";",
		"set", "-p", "-t", target, "remain-on-exit", "on", ";",
		"set", "-p", "-t", target, "remain-on-exit-format", "", ";",
		"set-hook", "-p", "-t", target, "pane-died", died(cr.suffix, cr.diedRun), ";",
		"set", "-p", "-t", target, "allow-passthrough", "all", ";",
		"set", "-t", target, "@cld-tmux", literal(cr.t.path), ";",
		"set", "-t", target, "@cld-home", literal(cr.home), ";",
		"set", "-t", target, "@cld-busy", busyMarker, ";",
		"set", "-t", target, "set-titles-string", titles(cr.suffix), ";",
		"set", "-t", target, "set-titles", "on"}
	if cr.l.from != "" && cr.l.from != cr.suffix {
		options = append(options, ";", "set", "-t", target, "@cld-last", cr.l.from)
	}
	return options
}

// marks are the commands, after a ";", that set the marks beside the entry in file once the session
// is made (see setMarks). run is the run mark, whose time a restored session keeps.
func (cr *creation) marks(file, run string) []string {
	if file == "" {
		return nil
	}
	if cr.l.restored {
		run = ""
	}
	return append([]string{";"}, setMarks(file, run)...)
}

// commandLimit is the size of the longest command a tmux client hands its server: the words after
// its options, each ended by a NUL, behind their count, an int. They go in one message of 16384
// bytes at most, 16 of them its header (decision 41.5).
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

// literal is word as tmux takes it back from its command line (decision 16.8). There a word ending
// in ";" ends the command, and a "\;" is a ";": "a;" goes as "a\;", and "a\;" as "a\;".
func literal(word string) string {
	if before, found := strings.CutSuffix(word, ";"); found {
		return before + `\;`
	}
	return word
}

// unexpanded is text as a tmux format that expands to text itself, every "#" doubled. tmux expands
// new-session's -c as a format, where #(command) runs a command (decision 16.8).
func unexpanded(text string) string {
	return strings.ReplaceAll(text, "#", "##")
}
