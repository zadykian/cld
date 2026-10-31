package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The dry-run's report of a restore's plan: the dedupe (decision 59.6), where panes joined to a
// session since a loss take the places of its panes on that session.

// dedupeCase is a loss of two tabs on session app 5 minutes ago, on plain ssh to ws where host
// holds. Panes on it opened since the given ages. A kept pane at its prompt has app as its last,
// cut 30 minutes ago. reopen is the dry-run's line for the loss's window, and notes its notes.
type dedupeCase struct {
	name   string
	since  []time.Duration
	kept   bool
	host   bool
	reopen string
	notes  []string
}

// Notes of the dedupe, on the lost panes one and two, and on the kept pane.
const (
	openedSince = "session 'app' on ws shows in a pane opened since: "
	dedupedOne  = openedSince + "a shell takes its place in /Users/me/one"
	dedupedTwo  = openedSince + "a shell takes its place in /Users/me/two"
	dedupedKept = openedSince + "left the pane in /Users/me as it is"
)

// A pane joined since a loss, to a session or by plain ssh to a host, takes the place of a lost
// pane there, the earliest lost before it opened. So two tabs on one session come back as two,
// less those joined since. A pane the dedupe takes opens a shell in its directory, with a
// note. A kept pane counts from its last's cut: a restore types nothing there, and drops its last.
func TestTabsPlanDedupe(t *testing.T) {
	t.Parallel()
	both := "2 tabs: session 'app' on ws, session 'app' on ws"
	for _, test := range []dedupeCase{
		{"none since", nil, false, false, both, nil},
		{"one before the loss", []time.Duration{10 * time.Minute}, false, false, both, nil},
		{"one since", []time.Duration{time.Minute}, false, false,
			"2 tabs: a shell in /Users/me/one, session 'app' on ws", []string{dedupedOne}},
		{"two since", []time.Duration{time.Minute, 2 * time.Minute}, false, false,
			"2 tabs: a shell in /Users/me/one, a shell in /Users/me/two",
			[]string{dedupedOne, dedupedTwo}},
		{"one since and a kept pane", []time.Duration{time.Minute}, true, false, both,
			[]string{dedupedKept}},
		{"one on a host since", []time.Duration{time.Minute}, false, true,
			"2 tabs: a shell in /Users/me/one, ssh to ws", []string{"ssh to ws shows in a pane " +
				"opened since: a shell takes its place in /Users/me/one"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			checkDedupe(t, test)
		})
	}
}

// checkDedupe reports test's dry-run, where it does not reopen and note what test says.
func checkDedupe(t *testing.T, test dedupeCase) {
	t.Helper()
	s := sandbox.New(t)
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	now := time.Now().UTC().Truncate(time.Second)
	panes, on, title := map[string]*tabs.Pane{}, tabs.Pane{Kind: "cld", Host: "ws", Session: "app"},
		"✳ cld-app"
	if test.host {
		on, title = tabs.Pane{Kind: "ssh", Host: "ws"}, "~"
	}
	var all []mac.Tab
	for _, age := range test.since {
		id := addPane(m, title, ssh("ws"))
		p := on
		p.Since = now.Add(-age)
		panes[id], all = &p, append(all, tabOf(id))
	}
	kept := ""
	if test.kept {
		kept = addPane(m, "~")
		panes[kept] = &tabs.Pane{Kind: "local", Since: now, Last: &tabs.Last{Host: "ws",
			Session: "app", Cut: "quit", Run: "1@x", At: now.Add(-30 * time.Minute)}}
		all = append(all, tabOf(kept))
	}
	m.Windows = []mac.Window{windowOf(append(all, tabOf(addPane(m, "~")))...)}
	saveModel(t, s, m)
	r := recordOf(m, panes)
	lost := tabs.Window{}
	for _, id := range []string{"one", "two"} {
		c := on
		c.Cwd = "/Users/me/" + id
		lost.Tabs = append(lost.Tabs, tabs.Tab{Shape: "made", Tree: &tabs.Node{Pane: id, Copy: &c}})
	}
	at := now.Add(-5 * time.Minute)
	r.Losses = []*tabs.Loss{{At: at, Run: r.Run.ID, Cause: "closed", Windows: []tabs.Window{lost}}}
	writeRecord(t, s, r)
	result := dryRun(t, s)
	want := "Would reopen 1 window, closed at " + clockAt(at) + ":\n  " + test.reopen + "\n"
	if notes := strings.Join(test.notes, "\n  "); notes != "" {
		want += "Notes:\n  " + notes + "\n"
	}
	if result.Code != 0 || !strings.HasSuffix(result.Stdout, want) ||
		strings.Contains(result.Stdout, "Would type") {
		t.Errorf("restore tabs --dry-run: exit %d, stdout\n%s\nwant it to end with\n%s",
			result.Code, result.Stdout, want)
	}
	if kept != "" {
		checkKeptDeduped(t, s, kept)
	}
}

// checkKeptDeduped restores in s, and reports the kept pane unless the restore typed nothing there
// and dropped its last.
func checkKeptDeduped(t *testing.T, s *sandbox.Sandbox, kept string) {
	t.Helper()
	if result := s.RunCld(tabsEnv, "restore", "tabs"); result.Code != 0 {
		t.Errorf("restore tabs: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if typed := loadModel(t, s).Terminals[kept].Typed; typed != nil {
		t.Errorf("the restore typed %q into the kept pane, want nothing", typed)
	}
	if last := readRecord(t, s).Panes[kept].Last; last != nil {
		t.Errorf("the kept pane's last is %+v, want none", last)
	}
}
