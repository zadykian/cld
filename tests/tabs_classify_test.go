package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What each pane shows, as the record keeps it: the leader of its foreground group, through ps.

// shown is what the record keeps of a pane that classification sets.
type shown struct {
	Kind, Host, Title string
}

// classifyCases are TestTabsClassify's panes: a title, the jobs in front of the pane's shell,
// and what the record shows. A want whose Title is "" has the pane's title.
var classifyCases = []struct {
	name, title string
	jobs        [][]string
	want        shown
}{
	{"ssh", "me@ws: ~", [][]string{ssh("ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"ssh by its path", "", [][]string{{"/usr/bin/ssh", "ws"}}, shown{Kind: "ssh", Host: "ws"}},
	{"a login's ssh", "", [][]string{{"-ssh", "ws"}}, shown{Kind: "ssh", Host: "ws"}},
	{"-zsh", "~", nil, shown{Kind: "local"}},
	{"a shell by its path", "~", [][]string{{"/bin/zsh"}}, shown{Kind: "local"}},
	{"a stale title at a prompt", "✳ cld-app-3", nil, shown{Kind: "local"}},
	{"ghostty +ssh", "~", [][]string{{ghosttyExecutable, "+ssh", "ws"}, ssh("ws")},
		shown{Kind: "ssh", Host: "ws"}},
	{"ghostty +ssh before its ssh", "~", [][]string{{ghosttyExecutable, "+ssh", "ws"}},
		shown{Kind: "other"}},
	{"ws-join", "~", [][]string{{"/bin/sh", "/Users/me/bin/ws-join", "-n", "app", "-s", "3"},
		ssh("-t", "ws", "cld", "join", "-n", "app", "-s", "3")}, shown{Kind: "ssh", Host: "ws"}},
	{"ws-join by its name", "~", [][]string{{"ws-join", "-n", "app"}, ssh("ws")},
		shown{Kind: "ssh", Host: "ws"}},
	{"git push", "~", [][]string{{"git", "push"},
		ssh("git@github.com", "git-receive-pack", "me/app.git")}, shown{Kind: "other"}},
	{"a host like an option", "~", [][]string{ssh("--", "-x")}, shown{Kind: "ssh"}},
	{"a host with a ;", "~", [][]string{ssh("ws;x")}, shown{Kind: "ssh"}},
	{"a host of 256", "~", [][]string{ssh(strings.Repeat("h", 256))}, shown{Kind: "ssh"}},
	{"no host", "~", [][]string{ssh("-v")}, shown{Kind: "ssh"}},
	{"-p 22 ws", "", [][]string{ssh("-p", "22", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-p22 ws", "", [][]string{ssh("-p22", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-lroot ws", "", [][]string{ssh("-lroot", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-l root ws", "", [][]string{ssh("-l", "root", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-o X=Y ws", "", [][]string{ssh("-o", "X=Y", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-oX=Y ws", "", [][]string{ssh("-oX=Y", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-tt ws", "", [][]string{ssh("-tt", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-- ws", "", [][]string{ssh("--", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"-tl root ws", "", [][]string{ssh("-tl", "root", "ws")}, shown{Kind: "ssh", Host: "ws"}},
	{"a remote command", "", [][]string{ssh("-i", "key", "-J", "jump", "ws", "ls", "-l")},
		shown{Kind: "ssh", Host: "ws"}},
	{"flags", "", [][]string{ssh("-46AaCfGgKkMNnqsTtVvXxYy", "ws")},
		shown{Kind: "ssh", Host: "ws"}},
	{"vim", "vim", [][]string{{"vim", "notes"}}, shown{Kind: "other"}},
	{"a local tmux", "~", [][]string{{"tmux", "attach"}}, shown{Kind: "other"}},
	{"mosh", "~", [][]string{{"mosh", "ws"}}, shown{Kind: "other"}},
}

// Each pane is what the leader of its foreground group shows. ssh names its host, past its
// options' values, directly or as the child of ghostty +ssh or ws-join. A shell is local,
// whatever its title, and any other program other.
func TestTabsClassify(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyAt(812, ghosttyStart)
	var shells []string
	for _, shell := range []string{"sh", "bash", "-bash", "zsh", "fish", "ksh", "dash", "tcsh",
		"csh", "nu"} {
		shells = append(shells, addPane(m, "~", []string{shell}))
	}
	ids := make([]string, len(classifyCases))
	var all []mac.Tab
	for i, test := range classifyCases {
		ids[i] = addPane(m, test.title, test.jobs...)
		all = append(all, tabOf(ids[i]))
	}
	dead, notReady := addPane(m, "✳ cld-app", ssh("ws")), addPane(m, "~")
	m.Terminals[dead].PID, m.Terminals[notReady].TTY = 0, ""
	for _, id := range append(shells, dead, notReady) {
		all = append(all, tabOf(id))
	}
	m.Windows = []mac.Window{windowOf(all...)}
	r := tick(t, s, m)
	for i, test := range classifyCases {
		if test.want.Title == "" {
			test.want.Title = test.title
		}
		checkPane(t, test.name, r.Panes[ids[i]], test.want, loginOf(m, ids[i]))
	}
	for _, id := range shells {
		checkPane(t, "shell", r.Panes[id], shown{Kind: "local", Title: "~"}, loginOf(m, id))
	}
	checkPane(t, "dead", r.Panes[dead], shown{Kind: "dead"}, 0)
	checkPane(t, "not ready", r.Panes[notReady], shown{Kind: "dead"}, 0)
}

// checkPane reports a pane p, named what, that does not show want, or whose login is not login.
func checkPane(t *testing.T, what string, p *tabs.Pane, want shown, login int) {
	t.Helper()
	if p == nil {
		t.Errorf("%s: no pane in the record", what)
		return
	}
	got := shown{Kind: p.Kind, Host: p.Host, Title: p.Title}
	if got != want || p.Login != login {
		t.Errorf("%s: recorded %+v, login %d, want %+v, login %d", what, got, p.Login, want, login)
	}
}
