package tests

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The fake ps, the sh fakes of open and ghostty, and a real pty's size and cld on it as input.

// The fake ps prints the model's processes in the columns cld asks for, its start in UTC and ??
// for no tty, as macOS's ps prints them with TZ=UTC and LC_ALL=C. It prints nothing with no model,
// refuses other arguments, and logs each call with TZ and LC_ALL.
func TestFakePs(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	extra := map[string]string{"TZ": "UTC", "LC_ALL": "C"}
	checkExit(t, "ps with no model", runFake(t, s, extra, "ps", mac.PsArgs...), 0, "", "")
	started := time.Date(2026, 10, 5, 9, 7, 3, 0, time.FixedZone("CEST", 2*60*60))
	saveModel(t, s, &mac.Model{Processes: []mac.Process{
		{PID: ghosttyPID, PPID: 1, PGID: ghosttyPID, Start: started,
			Args: []string{ghosttyExecutable}},
		{PID: 4250, PPID: ghosttyPID, PGID: 4250, TTY: "ttys016",
			Start: started.Add(11 * 24 * time.Hour),
			Args:  []string{"/usr/bin/login", "-flp", "me", "/bin/bash", "-c", "exec -l zsh"}},
		{PID: 123456, PPID: 4250, PGID: 123456, TTY: "pts/3", Start: started, Args: []string{"-zsh"}},
	}})
	checkExit(t, "ps", runFake(t, s, extra, "ps", mac.PsArgs...), 0,
		"  812     1   812 ??       Mon Oct  5 07:07:03 2026 "+ghosttyExecutable+"\n"+
			" 4250   812  4250 ttys016  Fri Oct 16 07:07:03 2026 "+
			"/usr/bin/login -flp me /bin/bash -c exec -l zsh\n"+
			"123456  4250 123456 pts/3    Mon Oct  5 07:07:03 2026 -zsh\n", "")
	checkExit(t, "ps -o pid= -p 812", runFake(t, s, nil, "ps", "-o", "pid=", "-p", "812"),
		1, "", "fake ps: unexpected arguments\n")
	calls := s.PsCalls()
	if len(calls) != 3 || !slices.Equal(calls[1].Argv, mac.PsArgs) ||
		!maps.Equal(calls[1].Env, extra) || len(calls[2].Env) != 0 {
		t.Errorf("ps.jsonl holds %+v, want three calls, with TZ and LC_ALL in the first two", calls)
	}
}

// The fake open logs its arguments. Where ghostty.next.json is not empty, open puts it in place as
// the model Ghostty, byte for byte, and empties it: the Ghostty that starts. It runs under a PATH
// of itself alone.
func TestFakeOpen(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	tools := s.Tools("open")
	model, next := filepath.Join(s.ProbeDir, mac.ModelFile), filepath.Join(s.ProbeDir, mac.NextFile)
	open := func() sandbox.Result {
		return runCommand(t, fakeCommand(t.Context(), s, map[string]string{"PATH": tools},
			filepath.Join(tools, "open"), "-b", "com.mitchellh.ghostty"))
	}
	checkExit(t, "open with no next model", open(), 0, "", "")
	if _, err := os.Stat(model); err == nil {
		t.Error("open with no next model made one")
	}
	// Leading spaces, a backslash and no newline at the end, which a read loop must keep.
	s.WriteFile(next, "{\n  \"running\": true,\n  \"version\": \"1.4\\\\0\"\n}")
	checkExit(t, "open", open(), 0, "", "")
	checkContent(t, model, "{\n  \"running\": true,\n  \"version\": \"1.4\\\\0\"\n}\n")
	checkContent(t, next, "")
	if m := loadModel(t, s); !m.Running || m.Version != `1.4\0` {
		t.Errorf("the model Ghostty is %+v, want running 1.4\\0", m)
	}
	s.WriteFile(model, "{}\n")
	checkExit(t, "open with an empty next model", open(), 0, "", "")
	checkContent(t, model, "{}\n")
	checkContent(t, filepath.Join(s.ProbeDir, mac.OpenLog),
		strings.Repeat("-b com.mitchellh.ghostty\n", 3))
}

// The fake ghostty prints show-config for +show-config, and finds a key in ssh-cache for
// +ssh-cache --host=KEY. Anything else exits 2. It logs each call, and runs under a PATH of
// itself alone.
func TestFakeGhostty(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args   []string
		files  map[string]string
		code   int
		stdout string
	}{
		{[]string{"+show-config"}, nil, 0, ""},
		{[]string{"+show-config"}, map[string]string{mac.ShowConfig: "font-size = 13\n" +
			"  keybind = ctrl+\\=new_tab\nmacos-titlebar-style = hidden"}, 0,
			"font-size = 13\n  keybind = ctrl+\\=new_tab\nmacos-titlebar-style = hidden\n"},
		{[]string{"+ssh-cache", "--host=me@ws"}, map[string]string{mac.SSHCache: "me@mac\nme@ws"},
			0, ""},
		{[]string{"+ssh-cache", "--host=me@w"}, map[string]string{mac.SSHCache: "me@ws\n"},
			1, ""},
		{[]string{"+ssh-cache", "--host=me@ws"}, nil, 1, ""},
		{[]string{"+ssh-cache", "--host="}, nil, 2, ""},
		{[]string{"+ssh-cache"}, nil, 2, ""},
		{[]string{"+show-config", "--default"}, nil, 2, ""},
		{[]string{"+version"}, nil, 2, ""},
		{nil, nil, 2, ""},
	} {
		s := sandbox.New(t)
		for name, content := range test.files {
			s.WriteFile(filepath.Join(s.ProbeDir, name), content)
		}
		tools := s.Tools("ghostty")
		args := strings.Join(test.args, " ")
		checkExit(t, "ghostty "+args, runCommand(t, fakeCommand(t.Context(), s,
			map[string]string{"PATH": tools}, filepath.Join(tools, "ghostty"), test.args...)),
			test.code, test.stdout, "")
		checkContent(t, filepath.Join(s.ProbeDir, mac.GhosttyLog), args+"\n")
	}
}

// Pty.SetSize sets the size, in cells and in pixels, that TIOCGWINSZ reads by the terminal's path,
// as cld reads a pane's by its tty. stty reads it too.
func TestPtySize(t *testing.T) {
	t.Parallel()
	p := sandbox.OpenPty(t)
	for _, want := range []unix.Winsize{
		{Row: 24, Col: 80, Xpixel: 1280, Ypixel: 768},
		{Row: 61, Col: 237, Xpixel: 3792, Ypixel: 1952},
	} {
		p.SetSize(int(want.Row), int(want.Col), int(want.Xpixel), int(want.Ypixel))
		fd, err := unix.Open(p.Path, unix.O_RDONLY|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
		if closeErr := unix.Close(fd); err == nil {
			err = closeErr
		}
		if err != nil || *got != want {
			t.Errorf("%s's size is %+v (%v), want %+v", p.Path, got, err, want)
		}
		stty := exec.Command("stty", "size")
		stty.Stdin = p.Terminal
		size := fmt.Sprintf("%d %d\n", want.Row, want.Col)
		if out, err := stty.Output(); err != nil || string(out) != size {
			t.Errorf("stty size printed %q (%v), want %q", out, err, size)
		}
	}
}

// RunCldWithInput gives cld the pty as its stdin, and so the terminal join's check passes, which
// leaves TERM's. With no stdin, the check stops join first.
func TestRunCldWithInput(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	p := sandbox.OpenPty(t)
	dumb := map[string]string{"TERM": "dumb"}
	checkExit(t, "join on a pty", s.RunCldWithInput(p.Terminal, dumb, "join", "-s", "x"), 1, "",
		needsTerminal+"TERM is dumb\n")
	checkExit(t, "join with no stdin", s.RunCldWithInput(nil, dumb, "join", "-s", "x"), 1, "",
		needsTerminal+"its input is not one\n")
}
