package tests

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The pane cld restore tabs runs in (decision 59.9): never typed into, it joins its last again
// by cld's exec of ssh, once the report is out.

// ownEnv is the environment of a restore whose exec of the fake ssh only logs, under Ghostty's
// TERM.
var ownEnv = map[string]string{"TZ": "UTC", "CLD_FAKE_SSH_EXIT": "0", "TERM": "xterm-ghostty"}

// onPty puts pane id of m on a pty of the test's, which cld then has as its stdin: the pane cld
// runs in. cld runs in front of the pane's shell.
func onPty(t *testing.T, m *mac.Model, id string) *sandbox.Pty {
	t.Helper()
	p := ptyPane(t, m, id)
	front(m, id, []string{"cld", "restore", "tabs"})
	return p
}

// ownJoin is the remote command of the ssh that joins session app on ws again.
const ownJoin = `exec $SHELL -lc "cd / && cld join --existing -s app; cd; exec $SHELL -l"`

// checkOwnSSH reports the fake ssh's last call in s, unless it ran the ssh that joins session app
// on ws again, under TERM term.
func checkOwnSSH(t *testing.T, s *sandbox.Sandbox, term string) {
	t.Helper()
	calls := s.SSHCalls()
	want := []string{"-t", "-o", "ConnectTimeout=10", "ws", ownJoin}
	if len(calls) == 0 || !slices.Equal(calls[len(calls)-1].Argv, want) ||
		calls[len(calls)-1].Term != term {
		t.Errorf("ssh.jsonl holds %+v, want ssh %q last, under TERM %s", calls, want, term)
	}
}

// cld types into each other pane at its prompt, never its own. There it says which session it
// joins, then execs ssh with the typed line's words, its TERM xterm-256color where Ghostty lacks
// the host's terminfo. The pane keeps its last, with the time of the exec, through the ticks that
// see that ssh in front within two minutes.
func TestTabsOwn(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, cache, term, notes, prefix string
	}{
		{"cached", "me@ws\n", "xterm-ghostty", "", ""},
		{"not cached", "", "xterm-256color", uncachedNote("ws"), uncached},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			s.WriteProbeFile(mac.SSHCache, test.cache)
			m := ghosttyApp(t, s)
			a, b := addPane(m, "~"), addPane(m, "~")
			p := onPty(t, m, a)
			m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b))}
			saveModel(t, s, m)
			now := time.Now().UTC().Truncate(time.Second)
			last := func(session string) *tabs.Last {
				return &tabs.Last{Host: "ws", Session: session, Cut: "quit", Run: "1@x", At: now}
			}
			writeRecord(t, s, recordOf(m, map[string]*tabs.Pane{
				a: {Kind: "local", Since: now, Last: last("app")},
				b: {Kind: "local", Since: now, Last: last("x")}}))
			cut := "as Ghostty quit at " + clockAt(now) + ": " + test.prefix
			checkOwnDryRun(t, s, p, "Would type into 2 panes:\n  in /Users/me, "+cut+wholeLine+
				"\n  here, in /Users/me, "+cut+strings.Replace(wholeLine, "-s x", "-s app", 1)+"\n")
			checkExit(t, "restore tabs", s.RunCldWithInput(p.Terminal, ownEnv, "restore", "tabs"),
				0, "Typed into the pane in /Users/me: session 'x' on ws\n"+
					"Joining session 'app' on ws here\n", test.notes+noWatcher)
			checkOwnSSH(t, s, test.term)
			checkTyped(t, loadModel(t, s), map[int][]string{1: nil})
			front(m, a, append([]string{"ssh"}, s.SSHCalls()[len(s.SSHCalls())-1].Argv...))
			for range 2 {
				if l := tick(t, s, m).Panes[a].Last; l == nil || l.Typed == nil || l.Typed.Before(now) {
					t.Errorf("a's last %+v, want it, typed at the restore", l)
				}
			}
		})
	}
}

// checkOwnDryRun reports a dry-run in s from the pane on p, unless it prints want and execs
// nothing.
func checkOwnDryRun(t *testing.T, s *sandbox.Sandbox, p *sandbox.Pty, want string) {
	t.Helper()
	result := s.RunCldWithInput(p.Terminal, ownEnv, "restore", "tabs", "--dry-run")
	if result.Code != 0 || !strings.Contains(result.Stdout, want) || len(s.SSHCalls()) != 1 {
		t.Errorf("restore tabs --dry-run: exit %d, stdout\n%s\nwant within it\n%s\nand ssh -G "+
			"alone, not %+v", result.Code, result.Stdout, want, s.SSHCalls())
	}
}

// After Ghostty's relaunch, a restore runs from a pane that showed session app before the quit.
// Its merge is the run's first: the pane gets app as its last, and cld joins it again there.
func TestTabsOwnRelaunch(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a := addPane(m, "✳ cld-app", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	tick(t, s, m)
	quitGhostty(m)
	n := relaunch(m, 4242, relaunched, a)
	p := onPty(t, n, a)
	saveModel(t, s, n)
	checkExit(t, "restore tabs", s.RunCldWithInput(p.Terminal, ownEnv, "restore", "tabs"), 0,
		"Joining session 'app' on ws here\n", noWatcher)
	checkOwnSSH(t, s, "xterm-ghostty")
	if l := readRecord(t, s).Panes[a].Last; l == nil || l.Session != "app" || l.Typed == nil {
		t.Errorf("a's last %+v, want session app, typed", l)
	}
}

// A pane whose last is plain ssh to a host joins it again by ssh HOST.
func TestTabsOwnHost(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	p := onPty(t, m, a)
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	now := time.Now().UTC().Truncate(time.Second)
	writeRecord(t, s, recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now,
		Last: &tabs.Last{Host: "ws", Cut: "quit", Run: "1@x", At: now}}}))
	checkExit(t, "restore tabs", s.RunCldWithInput(p.Terminal, ownEnv, "restore", "tabs"), 0,
		"Joining ws here\n", noWatcher)
	if calls := s.SSHCalls(); len(calls) == 0 ||
		!slices.Equal(calls[len(calls)-1].Argv, []string{"ws"}) {
		t.Errorf("ssh.jsonl holds %+v, want ssh ws last", calls)
	}
}
