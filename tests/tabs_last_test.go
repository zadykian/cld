package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A pane's last: the binding a quit cut, or a restore typed, which the next restore joins again
// while the pane is at its prompt. A tick clears it where the pane is in use again.

// lastCases are TestTabsLast's panes, each local with a last on ws in the record. Each gives what
// a tick shows there, typed's age where a restore typed the line, and whether last stays.
var lastCases = []struct {
	name, title string
	jobs        [][]string
	typed       time.Duration // 0: nothing typed
	kept        bool
}{
	{"cld on its host", "✳ cld-app", [][]string{ssh("ws")}, 0, false},
	{"cld on another host", "✳ cld-x", [][]string{ssh("ws2")}, 0, false},
	{"ssh to its host", "~", [][]string{ssh("ws")}, 0, false},
	{"ssh to its host a minute after a typed line", "~", [][]string{ssh("ws")}, time.Minute, true},
	{"ssh to its host 3 minutes after a typed line", "~", [][]string{ssh("ws")}, 3 * time.Minute,
		false},
	{"ssh to another host", "~", [][]string{ssh("ws2")}, 0, true},
	{"another program", "vim", [][]string{{"vim"}}, 0, false},
	{"a failed attempt", "~", nil, 10 * time.Minute, true},
	{"at its prompt", "~", nil, 0, true},
	{"cld in front", "~", [][]string{{"cld", "restore", "tabs"}}, 0, true},
}

// A tick clears last where the pane shows cld, on any host, or another program. ssh to last's
// host clears it at once where no restore typed it. Else it waits two minutes from the typed
// line, as a refused join leaves the user in the host's shell. A shell at its prompt keeps it, as
// after a failed attempt, and so do a dead pane and one in front of which cld runs.
func TestTabsLast(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	now := time.Now().UTC().Truncate(time.Second)
	panes := map[string]*tabs.Pane{}
	var ids []string
	var all []mac.Tab
	for _, test := range lastCases {
		id := addPane(m, test.title, test.jobs...)
		last := &tabs.Last{Host: "ws", Session: "app", Cut: "quit", Run: "1@x", At: now}
		if test.typed != 0 {
			typed := now.Add(-test.typed)
			last.Typed = &typed
		}
		panes[id] = &tabs.Pane{Kind: "local", Since: now, Last: last}
		ids, all = append(ids, id), append(all, tabOf(id))
	}
	dead := addPane(m, "~")
	m.Terminals[dead].PID = 0
	panes[dead] = &tabs.Pane{Kind: "local", Since: now, Last: &tabs.Last{Host: "ws", Cut: "quit",
		Run: "1@x", At: now}}
	m.Windows = []mac.Window{windowOf(append(all, tabOf(dead))...)}
	writeRecord(t, s, recordOf(m, panes))
	r := tick(t, s, m)
	for i, test := range lastCases {
		if kept := r.Panes[ids[i]].Last != nil; kept != test.kept {
			t.Errorf("%s: last kept %t, want %t", test.name, kept, test.kept)
		}
	}
	if r.Panes[dead].Last == nil {
		t.Error("dead: last cleared")
	}
}

// Within a run of Ghostty nothing sets last (decision 59.10). A pane that left cld has nothing to
// join again, nor one that left cld for the host's shell, then the host.
func TestTabsLastNotWithinARun(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b := addPane(m, "✳ cld-app", ssh("ws")), addPane(m, "✳ cld-x", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b))}
	tick(t, s, m)
	front(m, a)
	front(m, b, ssh("ws"))
	m.Terminals[b].Name = "me@ws: ~"
	r := tick(t, s, m)
	checkPane(t, "cld, then local", r.Panes[a], shown{Kind: "local", Title: "✳ cld-app"},
		loginOf(m, a))
	checkPane(t, "cld, then ssh", r.Panes[b], shown{Kind: "ssh", Host: "ws", Title: "me@ws: ~"},
		loginOf(m, b))
	front(m, b)
	r = tick(t, s, m)
	for _, id := range []string{a, b} {
		if p := r.Panes[id]; p == nil || p.Kind != "local" || p.Last != nil {
			t.Errorf("pane %s is %+v, want local with no last", id, p)
		}
	}
	if result := dryRun(t, s); strings.Contains(result.Stdout, "Would type") {
		t.Errorf("restore tabs --dry-run printed\n%s\nwant no pane to type into", result.Stdout)
	}
}
