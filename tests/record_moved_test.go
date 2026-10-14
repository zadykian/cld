package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The directory claude was last in, beside the record's entry (decision 56): the hooks and the
// kill that write it, as claude's /cd moves claude.

// runSettingsHooks runs each command that claude's settings give event, as claude would once the
// probe is gone: with sh -c in dir, the probe's environment and {} as input. It reports one that
// fails or prints anything.
func runSettingsHooks(t *testing.T, claude *sandbox.Probe, event, dir string) {
	t.Helper()
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	i := slices.Index(claude.Argv, "--settings")
	if i < 0 || i+1 == len(claude.Argv) || json.Unmarshal([]byte(claude.Argv[i+1]), &settings) != nil {
		t.Fatalf("no settings in claude's arguments %q", claude.Argv)
	}
	var env []string
	for name, value := range claude.Env {
		env = append(env, name+"="+value)
	}
	for _, group := range settings.Hooks[event] {
		for _, hook := range group.Hooks {
			cmd := exec.Command("/bin/sh", "-c", hook.Command)
			cmd.Dir, cmd.Env, cmd.Stdin = dir, env, strings.NewReader("{}")
			if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
				t.Errorf("%s's %s: %v, printed %q", event, hook.Command, err, out)
			}
		}
	}
}

// join writes the directory claude starts in beside the entry. claude's Stop and SessionEnd hooks
// write the one claude is in, read in its pane, %0. So does a teammate split off with claude's
// settings, its pane active. A shell's cd, which fires CwdChanged, writes nothing, nor does a hook
// once the entry is forgotten.
func TestLastDirectoryHooks(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	moved, other := filepath.Join(s.Root, `it's "moved"`), filepath.Join(s.Root, "other")
	teammate := filepath.Join(s.Root, "teammate")
	makeDirs(t, moved, other, teammate)
	startCld(t, s, "tmux", nil, "join", "-s", "x")
	claude := s.WaitProbes(1)[0]
	checkLastDirectory(t, s, "x", "join", s.Work)
	moveClaude(t, s, "x", claude, moved)
	claude.Hook("CwdChanged", `{"new_cwd":`+jsonText(moved)+`}`)
	checkLastDirectory(t, s, "x", "CwdChanged", s.Work)
	claude.Hook("Stop", `{}`)
	checkLastDirectory(t, s, "x", "Stop", moved)
	// As claude starts a teammate of its agent teams in tmux (docs/design/findings/claude.md).
	s.MustTmux("cld-x", append([]string{"split-window", "-t", "=cld-x:", "-c", teammate,
		filepath.Join(sandbox.ProbeBin, "claude")}, claude.Argv...)...)
	s.WaitProbes(2)[1].Hook("Stop", `{}`)
	checkLastDirectory(t, s, "x", "a teammate's Stop", moved)
	moveClaude(t, s, "x", claude, other)
	claude.Send("unsetenv TMUX TMUX_PANE")
	claude.Hook("SessionEnd", `{"reason":"other"}`)
	checkLastDirectory(t, s, "x", "SessionEnd without TMUX_PANE", other)
	forget(t, s, "x")
	if err := os.Remove(companionFile(s, "x", ".dir")); err != nil {
		t.Fatal(err)
	}
	claude.Hook("Stop", `{}`)
	if exists(companionFile(s, "x", ".dir")) {
		t.Error("Stop wrote claude's directory once the entry was forgotten")
	}
}

// tmux names a directory removed under claude "DIR (deleted)", which the hooks pass over. A claude
// that exited shows the directory written in the list. Once kill-server has ended the session, as
// a reboot does, SessionEnd's hooks find no server: they print nothing, exit 0 and leave it.
func TestLastDirectoryKept(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	moved, gone := filepath.Join(s.Root, "moved"), filepath.Join(s.Root, "gone")
	makeDirs(t, moved, gone)
	startCld(t, s, "tmux", nil, "join", "-s", "x")
	claude := s.WaitProbes(1)[0]
	moveClaude(t, s, "x", claude, moved)
	claude.Hook("Stop", `{}`)
	moveClaude(t, s, "x", claude, gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	runSettingsHooks(t, claude, "Stop", s.Work)
	checkLastDirectory(t, s, "x", "Stop in a directory removed", moved)
	claude.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool {
		return s.Format("cld-x", "#{pane_dead}") == "1"
	})
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\nx     exited    now          " + moved + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	s.MustTmux("cld-x", "kill-server")
	waitServerExit(t, s, "x")
	runSettingsHooks(t, claude, "SessionEnd", s.Work)
	checkLastDirectory(t, s, "x", "SessionEnd with no server", moved)
}

// A session whose claude moved, as /cd moves it, then was killed with no turn since, ended in the
// directory claude was in: list and completion show it there. join brings the conversation back
// there from anywhere, PWD naming it, but the entry keeps the directory join made the session in,
// and the session its home there. The kill keeps every character of a directory's name.
func TestLastDirectoryKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// What sh, tmux's formats and JSON would take for their own. elsewhere's name, as the work
	// directory's, leaves nothing of a session's name.
	moved := filepath.Join(s.Root, `it's "#{moved}" $x`)
	elsewhere, lines := filepath.Join(s.Root, "elsewhere", "_"), filepath.Join(s.Root, "a\nb'")
	makeDirs(t, moved, filepath.Dir(elsewhere), elsewhere, lines)
	startCld(t, s, "tmux", nil, "join", "-s", "x")
	first := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	moveClaude(t, s, "x", first, moved)
	killAndWait(t, s, s.Work, first, "-s", "x")
	checkLastDirectory(t, s, "x", "kill", moved)
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\nx     ended     -            " + moved + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	completed := s.RunCld(nil, "__complete", "join", "-s", "")
	if want := offered("x\tended in " + moved); completed.Stdout != want {
		t.Errorf("join -s completes\n%s\nwant\n%s", completed.Stdout, want)
	}
	startCldIn(t, s, "tmux", elsewhere, nil, "join", "-s", "x")
	resumed := s.WaitProbes(2)[1]
	args := []string{"--name", "cld-x", "--settings", sessionSettings(s, "cld-x", s.Work),
		"--resume", firstID}
	if !slices.Equal(resumed.Argv, args) || resumed.Cwd != moved || resumed.Env["PWD"] != moved {
		t.Errorf("claude %q in %s, PWD %q, want %q in %s",
			resumed.Argv, resumed.Cwd, resumed.Env["PWD"], args, moved)
	}
	if got, want := readEntry(s, "x"), entry("x", s.Work, firstID); got != want {
		t.Errorf("entry %q, want %q", got, want)
	}
	if home := s.MustTmux("cld-x", "show", "-v", "-t", "=cld-x:", "@cld-home"); home != s.Work {
		t.Errorf("@cld-home %q, want %q", home, s.Work)
	}
	checkLastDirectory(t, s, "x", "the join", moved)
	// The probe reads a line for each command: it goes to lines by a link.
	link := filepath.Join(s.Root, "lines")
	if err := os.Symlink(lines, link); err != nil {
		t.Fatal(err)
	}
	moveClaude(t, s, "x", resumed, link)
	killAndWait(t, s, s.Work, resumed, "-s", "x")
	checkLastDirectory(t, s, "x", "kill in a directory of two lines", lines)
}
