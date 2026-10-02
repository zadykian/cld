package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// controller carries out the commands of PID.ctl: write draws on claude's terminal, and base is
// the path of the probe's files, less the extension.
type controller struct {
	write func(string)
	base  string
}

// controls are the commands PID.ctl takes, by the first word of a line; each gets the rest of
// the line, and a line naming none is passed over.
var controls = map[string]func(c *controller, argument string){
	// title TEXT sets the window title.
	"title": func(c *controller, text string) { c.write("\x1b]0;" + text + "\x07") },
	// osc52 TEXT copies TEXT to the clipboard, in tmux passthrough.
	"osc52": func(c *controller, text string) {
		c.write(passthrough("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"))
	},
	"loadbuffer": (*controller).loadBuffer,
	// notify CHANNEL TEXT notifies TEXT as claude does on CHANNEL (see notification).
	"notify": func(c *controller, argument string) {
		channel, text, _ := strings.Cut(argument, " ")
		c.write(notification(channel, text))
	},
	// link URI TEXT writes TEXT as a link to URI (OSC 8), as claude marks a file path or a URL
	// under tmux, and a line break.
	"link": func(c *controller, argument string) {
		uri, text, _ := strings.Cut(argument, " ")
		c.write("\x1b]8;;" + uri + "\x07" + text + "\x1b]8;;\x07\r\n")
	},
	"rekey": (*controller).rekey,
	// inline leaves the alternate screen and turns mouse reporting off, as claude outside
	// fullscreen draws.
	"inline": func(c *controller, _ string) {
		c.write("\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?1006l\x1b[?1049l")
	},
	// cd DIR changes to DIR, as claude does entering a worktree.
	"cd": func(_ *controller, dir string) {
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintln(os.Stderr, "probe:", err)
		}
	},
	"tmux": (*controller).tmux,
	"hook": (*controller).hook,
	// unsetenv NAMES unsets the variables NAMES, split at spaces, for the hooks and tmux the probe
	// runs from then on. claude runs the hooks of a background conversation without TMUX and
	// TMUX_PANE.
	"unsetenv": func(_ *controller, names string) {
		for name := range strings.FieldsSeq(names) {
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintln(os.Stderr, "probe:", err)
			}
		}
	},
	// exit [N] exits at once with status N, leaving the terminal modes on.
	"exit": func(_ *controller, argument string) {
		status, _ := strconv.Atoi(argument) //nolint:errcheck // no N gives 0, the default status
		os.Exit(status)
	},
}

// obey carries out each command of the FIFO control, a line each.
func obey(control *os.File, c *controller) {
	lines := bufio.NewScanner(control)
	for lines.Scan() {
		command, argument, _ := strings.Cut(lines.Text(), " ")
		if do, ok := controls[command]; ok {
			do(c, argument)
		}
	}
}

// loadBuffer is "loadbuffer TEXT": it copies TEXT as claude does inside tmux, with tmux
// load-buffer -w. tmux keeps it in a buffer and hands it to the terminal as OSC 52.
func (*controller) loadBuffer(text string) {
	load := exec.Command("tmux", "load-buffer", "-w", "-")
	load.Stdin = strings.NewReader(text)
	if err := load.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe: tmux load-buffer:", err)
	}
}

// rekey leaves and re-enters the alternate screen, pushes the keyboard modes again and repaints,
// as claude does after an external editor. The repaint ends in a line "repainted".
func (c *controller) rekey(string) {
	// A screen model that took CSI > 4 ; 2 m for SGR 4;2 would draw the repaint underlined and
	// faint, and replay it that way on every reattach.
	c.write("\x1b[?1049l\x1b[?1049h\x1b[<u\x1b[>1u\x1b[>4;2m\x1b[H\x1b[2J" +
		screen() + "repainted\r\n")
}

// tmux is "tmux ARGS": it runs tmux with ARGS, split at spaces, as anything claude runs may. TMUX
// takes it to the server of claude's pane.
func (*controller) tmux(args string) {
	if out, err := exec.Command("tmux", strings.Fields(args)...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "probe: tmux: %v: %s", err, out)
	}
}

// hook is "hook EVENT [JSON]": it runs EVENT's hooks (see runHooks), then appends a line to
// PID.hooks. The line is EVENT, and after ": " why where a hook failed or printed anything, which
// claude would show or hand the model, for the test to report.
func (c *controller) hook(argument string) {
	event, input, _ := strings.Cut(argument, " ")
	line := event
	if err := runHooks(event, input); err != nil {
		line += ": " + strconv.Quote(err.Error())
	}
	if err := appendLine(c.base+".hooks", []byte(line)); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
	}
}

// notification is what claude 2.1.284 writes, with TMUX set, to notify text on the channel its
// setting preferredNotifChannel names, the title left to claude's default (decision 29).
func notification(channel, text string) string {
	switch channel {
	case "iterm2":
		return passthrough("\x1b]9;" + text + "\x07")
	case "kitty":
		return passthrough("\x1b]99;i=1:d=0:p=title;Claude Code\x07") +
			passthrough("\x1b]99;i=1:p=body;"+text+"\x07") +
			passthrough("\x1b]99;i=1:d=1:a=focus;\x07")
	case "ghostty":
		return passthrough("\x1b]777;notify;Claude Code;" + text + "\x07")
	case "terminal_bell":
		return "\x07"
	}
	fmt.Fprintln(os.Stderr, "probe: no notification channel", channel)
	return ""
}

// passthrough wraps a sequence so that tmux hands it to the outer terminal unchanged.
func passthrough(sequence string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b") + "\x1b\\"
}
