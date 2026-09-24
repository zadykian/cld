// Command probe stands in for the programs cld starts, in cld's tests.
//
// Invoked as "claude", it treats the terminal the way claude does - alternate screen, SGR
// all-motion mouse, bracketed paste, focus reports, modifyOtherKeys, its own window title - and
// records what cld and the terminal hand it. Its files in $CLD_PROBE_DIR are named after its pid:
//
//	PID.json  argv, working directory and environment, written once at start
//	PID.in    every input byte, appended as it arrives
//	PID.ctl   FIFO of commands, one per line:
//	            title TEXT        set the window title
//	            osc52 TEXT        copy TEXT to the clipboard (OSC 52 in tmux passthrough)
//	            loadbuffer TEXT   copy TEXT the way claude does inside tmux: tmux load-buffer -w
//	            rekey             leave and re-enter the alternate screen, push the keyboard
//	                              modes again and repaint, as claude does after an external
//	                              editor; the repaint ends in a line "repainted"
//	            inline            leave the alternate screen and turn mouse reporting off, as
//	                              claude outside fullscreen draws
//	            exit              exit at once, leaving the terminal modes on
//
// Invoked as "tmux", it fakes tmux for the checks cld makes before starting it: "tmux -V" prints
// $CLD_FAKE_TMUX_VERSION, has-session finds no session, and any other invocation is recorded in
// $CLD_PROBE_DIR/tmux.json.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

// Record is what the probe writes to PID.json and tmux.json.
type Record struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

const modes = "\x1b[?1049h" + // alternate screen
	"\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h" + // SGR all-motion mouse
	"\x1b[?2004h" + // bracketed paste
	"\x1b[?1004h" + // focus reports
	"\x1b[>4;1m" // modifyOtherKeys mode 1

func main() {
	var err error
	if filepath.Base(os.Args[0]) == "tmux" {
		err = fakeTmux()
	} else {
		err = claude()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

func fakeTmux() error {
	if len(os.Args) == 2 && os.Args[1] == "-V" {
		fmt.Println(os.Getenv("CLD_FAKE_TMUX_VERSION"))
		return nil
	}
	if slices.Contains(os.Args[1:], "has-session") {
		fmt.Fprintln(os.Stderr, "can't find session: fake")
		os.Exit(1)
	}
	return writeRecord(filepath.Join(os.Getenv("CLD_PROBE_DIR"), "tmux.json"))
}

func claude() error {
	base := filepath.Join(os.Getenv("CLD_PROBE_DIR"), strconv.Itoa(os.Getpid()))
	if err := syscall.Mkfifo(base+".ctl", 0o600); err != nil {
		return err
	}
	// Opened read-write so that the FIFO never reports end of file between two writers.
	control, err := os.OpenFile(base+".ctl", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	input, err := os.OpenFile(base+".in", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	// Written last: once PID.json exists, the other files do too.
	if err := writeRecord(base + ".json"); err != nil {
		return err
	}
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		return err
	}

	var mu sync.Mutex
	write := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = os.Stdout.WriteString(s)
	}
	write(modes + "\x1b]0;probe\x07" + screen())
	go obey(control, write)

	buffer := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buffer)
		if n > 0 {
			if _, err := input.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			return nil // the terminal went away
		}
	}
}

func obey(control *os.File, write func(string)) {
	lines := bufio.NewScanner(control)
	for lines.Scan() {
		command, argument, _ := strings.Cut(lines.Text(), " ")
		switch command {
		case "title":
			write("\x1b]0;" + argument + "\x07")
		case "osc52":
			write(passthrough("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(argument)) + "\x07"))
		case "loadbuffer":
			// tmux keeps the text in a buffer and hands it to the terminal as OSC 52.
			load := exec.Command("tmux", "load-buffer", "-w", "-")
			load.Stdin = strings.NewReader(argument)
			if err := load.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "probe: tmux load-buffer:", err)
			}
		case "rekey":
			// A screen model that took CSI > 4 ; 2 m for SGR 4;2 would draw the repaint underlined
			// and faint, and replay it that way on every reattach.
			write("\x1b[?1049l\x1b[?1049h\x1b[<u\x1b[>1u\x1b[>4;2m\x1b[H\x1b[2J" + screen() + "repainted\r\n")
		case "inline":
			write("\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?1006l\x1b[?1049l")
		case "exit":
			os.Exit(0)
		}
	}
}

// screen is what the probe draws: its arguments, then a line in the attributes claude uses.
func screen() string {
	return "probe " + strings.Join(os.Args[1:], " ") + "\r\n" +
		"\x1b[1mbold\x1b[22m \x1b[2mdim\x1b[22m \x1b[3mitalic\x1b[23m \x1b[7minverse\x1b[27m " +
		"\x1b[38;5;208mcolour\x1b[39m\r\n"
}

// passthrough wraps a sequence so that tmux hands it to the outer terminal unchanged.
func passthrough(sequence string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b") + "\x1b\\"
}

func writeRecord(path string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	record := Record{Argv: os.Args[1:], Cwd: cwd, Env: map[string]string{}}
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		record.Env[name] = value
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	// Renamed into place, so a reader never sees a partial file.
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
