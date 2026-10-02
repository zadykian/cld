package tests

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// yourTmuxLines are the lines the user guide gives for ~/.tmux.conf, for the user's own tmux that
// cld runs in. extended-keys is always where cld's tmux is older than 3.7 (before37), which then
// asks no tmux for modified keys. With on, the other tmux passes them only to a program that asks.
func yourTmuxLines(before37 bool) string {
	extendedKeys := "on"
	if before37 {
		extendedKeys = "always"
	}
	return "set -s extended-keys " + extendedKeys + "\n" +
		"set -as terminal-features 'xterm*:extkeys:hyperlinks'\n" +
		"set -s set-clipboard on\n" +
		"set -s focus-events on\n"
}

// tmuxOlder reports whether the tmux the tests run - cld's, and the tmux cld runs in - is older
// than major.minor. It reads tmux -V, where one without a version, "tmux master", is not older.
// Of the others, "tmux 3.5a" is 3.5, and a development build's "tmux next-3.8" 3.8.
func tmuxOlder(t *testing.T, major, minor int) bool {
	t.Helper()
	out, err := exec.Command(sandbox.RealTmux, "-V").Output()
	if err != nil {
		t.Fatalf("tmux -V: %v", err)
	}
	match := regexp.MustCompile(`([0-9]+)\.([0-9]+)`).FindStringSubmatch(string(out))
	if match == nil {
		return false
	}
	var version []int
	for _, digits := range match[1:] {
		n, err := strconv.Atoi(digits)
		if err != nil {
			t.Fatalf("tmux -V: %v", err)
		}
		version = append(version, n)
	}
	return slices.Compare(version, []int{major, minor}) < 0
}

// shownMessages is the messages that server's tmux has shown, from its log, even those claude drew
// over at once.
func shownMessages(s *sandbox.Sandbox, server string) []string {
	var messages []string
	for line := range strings.SplitSeq(s.MustTmux(server, "show-messages"), "\n") {
		if _, message, found := strings.Cut(line, " message: "); found {
			messages = append(messages, message)
		}
	}
	return messages
}

// Inside the user's own tmux, yours, join and the list's Enter name on the message line the keys
// yours keeps from claude, until a key (decision 43). The keys are its prefixes, and Shift+Enter
// where extended-keys does not pass it. With the guide's lines Shift+Enter and clipboard copies
// come through. tmux 3.5 shows no message, and cld names nothing there.
func TestKeysYourTmuxKeeps(t *testing.T) {
	t.Parallel()
	before36 := tmuxOlder(t, 3, 6)
	for _, yours := range yourTmuxCases(tmuxOlder(t, 3, 7)) {
		t.Run(yours.name, func(t *testing.T) {
			t.Parallel()
			message := yours.message
			if before36 {
				message = ""
			}
			checkYourTmux(t, yours, message)
		})
	}
}

// yourTmux is a configuration of the user's own tmux, and what cld says of it.
type yourTmux struct {
	name string
	// conf is the configuration of yours, or none for tmux's defaults
	conf string
	// message is the message line from tmux 3.6, or none for no message
	message    string
	shiftEnter []string
}

// yourTmuxCases are TestKeysYourTmuxKeeps' configurations of yours, under a tmux older than 3.7
// where before37. That tmux asks yours for no modified keys, so under extended-keys on Shift+Enter
// arrives as Enter, and under always alone cld names it all the same.
func yourTmuxCases(before37 bool) []yourTmux {
	const guide = `: see "Inside your own tmux" in cld's guide`
	lines := yourTmuxLines(before37)
	twoPrefixes := "your tmux keeps C-a and C-b" + guide
	extendedKeysOn, shiftEnterOn := "your tmux keeps C-b"+guide, expectations["tmux"].shiftEnter
	if before37 {
		twoPrefixes = "your tmux keeps C-a, C-b and Shift+Enter: see cld's guide"
		extendedKeysOn, shiftEnterOn = "your tmux keeps C-b and Shift+Enter"+guide, []string{"\r"}
	}
	return []yourTmux{
		{"default", "", "your tmux keeps C-b and Shift+Enter" + guide, []string{"\r"}},
		{"the guide's lines", lines, "your tmux keeps C-b" + guide,
			expectations["tmux"].shiftEnter},
		{"extended-keys on", yourTmuxLines(false), extendedKeysOn, shiftEnterOn},
		{"two prefixes", "set -g prefix C-a\nset -g prefix2 C-b\nset -s extended-keys always\n",
			twoPrefixes, expectations["tmux"].shiftEnter},
		{"no prefix", lines + "set -g prefix None\n", "", expectations["tmux"].shiftEnter},
	}
}

// checkYourTmux runs cld join in a pane of yours, configured so, and checks the message line, how
// Shift+Enter arrives, and the message going with the first key.
func checkYourTmux(t *testing.T, yours yourTmux, message string) {
	t.Helper()
	s := sandbox.New(t)
	term, probe := startInYours(t, s, yours.conf)
	if message != "" {
		waitScreen(t, term, message)
	}
	shiftEnter := between(t, term, probe, "S-Enter")
	if !slices.Contains(yours.shiftEnter, shiftEnter) {
		t.Errorf("Shift+Enter arrives as %s, want one of %q",
			strconv.Quote(shiftEnter), yours.shiftEnter)
	}
	sandbox.WaitFor(t, 10*time.Second, "the message to go with the first key", func() bool {
		return !strings.Contains(term.Screen(), "your tmux keeps")
	})
	switch yours.name {
	case "default":
		checkDefaultTmux(t, s, term, probe, message)
	case "the guide's lines":
		probe.Send("osc52 copied inside your tmux")
		if clipboard := term.Clipboard(); clipboard != "copied inside your tmux" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied inside your tmux")
		}
	}
	if message == "" {
		if messages := shownMessages(s, "cld-nested"); len(messages) != 0 {
			t.Errorf("messages %q, want none", messages)
		}
	}
}

// startInYours starts yours, with the configuration conf, in the baseline terminal, and cld join
// in its pane once yours takes the terminal for a tmux. It returns the terminal and claude.
func startInYours(
	t *testing.T, s *sandbox.Sandbox, conf string,
) (terminal.Terminal, *sandbox.Probe) {
	t.Helper()
	path := "/dev/null"
	if conf != "" {
		path = filepath.Join(s.Root, "yours.conf")
		s.WriteFile(path, conf)
	}
	term := terminal.New(t, "tmux", s)
	term.Start([]string{"tmux", "-L", "yours", "-f", path, "new-session", "-s", "yours",
		"sleep", "3600"}, s.Env, s.Work)
	// yours learns its terminal's features from the terminal's answers, which come as its first
	// pane starts. They hold overline, from its entry for a tmux, and extkeys from tmux 3.7.
	sandbox.WaitFor(t, 10*time.Second, "yours to take its terminal for a tmux", func() bool {
		features, err := s.Tmux("yours", "display", "-p", "-t", "=yours:",
			"#{client_termfeatures}")
		return err == nil && slices.Contains(strings.Split(features, ","), "overline")
	})
	s.MustTmux("yours", append([]string{"respawn-pane", "-k", "-t", "=yours:", "-c", s.Work},
		s.CldArgv("join", "-s", "nested")...)...)
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	waitScreen(t, term, "probe --name cld-nested")
	return term, probe
}

// checkDefaultTmux checks that the prefix typed twice sends C-b through a default yours. join and
// the list's Enter, each in a window of yours of its own, show the message to their own terminals.
func checkDefaultTmux(
	t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, probe *sandbox.Probe,
	message string,
) {
	t.Helper()
	if got := between(t, term, probe, "C-b", "C-b"); got != "\x02" {
		t.Errorf("C-b C-b arrives as %s, want \"\\x02\"", //nolint:dupword // the key, twice
			strconv.Quote(got))
	}
	s.MustTmux("yours", append([]string{"new-window", "-c", s.Work},
		s.CldArgv("join", "-s", "nested")...)...)
	waitClients(t, s, 2)
	if message != "" {
		waitScreen(t, term, message)
	}
	s.MustTmux("yours", append([]string{"new-window", "-c", s.Work}, s.CldArgv("list")...)...)
	waitScreen(t, term, "> nested")
	term.Keys("Enter")
	waitClients(t, s, 3)
	if message != "" {
		waitScreen(t, term, message)
	}
}

// Under extended-keys on, yours asks a terminal it does not recognise for no modified keys. So cld
// names Shift+Enter where the pane's client lacks extkeys, or none is attached (decision 43.1).
// yours runs detached, and its log tells what cld's client was shown. An unmarked cld-yours counts
// as yours, and cld names nothing where TMUX names a tmux of another pane.
func TestKeysYourTmuxKeepsWithoutItsTerminal(t *testing.T) {
	t.Parallel()
	want := []string{"your tmux keeps C-b, C-a and Shift+Enter: see cld's guide"}
	if tmuxOlder(t, 3, 6) {
		want = nil
	}
	for _, server := range []string{"yours", "cld-yours"} {
		t.Run("a pane of "+server, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			conf := filepath.Join(s.Root, "yours.conf")
			s.WriteFile(conf, "set -s extended-keys on\nset -g prefix2 C-a\n")
			s.MustTmux(server, append([]string{"-f", conf, "new-session", "-d", "-s", "yours",
				"-c", s.Work}, s.CldArgv("join", "-s", "nested")...)...)
			s.WaitProbes(1)
			waitClients(t, s, 1)
			if messages := shownMessages(s, "cld-nested"); !slices.Equal(messages, want) {
				t.Errorf("messages %q, want %q", messages, want)
			}
		})
	}
	t.Run("no pane of yours", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		s.MustTmux("yours", "-f", "/dev/null", "new-session", "-d", "-s", "yours", "sleep", "3600")
		socket, pane, _ := strings.Cut(s.MustTmux("yours", "list-panes", "-t", "=yours:",
			"-F", "#{socket_path} #{pane_id}"), " ")
		startCld(t, s, "tmux", map[string]string{"TMUX": socket + ",1,0", "TMUX_PANE": pane},
			"join", "-s", "elsewhere")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		if messages := shownMessages(s, "cld-elsewhere"); len(messages) != 0 {
			t.Errorf("messages %q where cld's terminal is no pane of yours, want none", messages)
		}
	})
}
