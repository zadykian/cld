package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join sets the server's options and key bindings as docs/design/overview.md lists them. Those
// that concern every session go on the server, and those that concern claude alone on claude's
// pane or session.
func TestServerOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join")
	s.WaitProbes(1)
	checkServerWide(t, s)
	checkClaudesOwn(t, s)
	checkSetTwice(t, s)
	checkKeyBindings(t, s)
}

// checkServerWide checks the options of the server and its sessions.
func checkServerWide(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, option := range []struct{ scope, name, value string }{
		{"-sv", "@cld", "1"},
		{"-sv", "extended-keys", "on"},
		{"-sv", "focus-events", "on"},
		{"-gv", "mouse", "on"},
		{"-gv", "allow-passthrough", "on"},
		{"-gv", "status", "off"},
		{"-gv", "history-limit", "50000"},
		{"-gv", "prefix", "C-q"},
	} {
		if value := s.MustTmux("cld-0", "show", option.scope, option.name); value != option.value {
			t.Errorf("%s is %q, want %q", option.name, value, option.value)
		}
	}
	// claude's pane has the history cld sets, which tmux before 3.7 gives only a pane made after
	// the option.
	if limit := s.Format("cld-0", "#{history_limit}"); limit != "50000" {
		t.Errorf("claude's pane keeps %q lines of history, want 50000", limit)
	}
}

// checkClaudesOwn checks that what cld sets for a failed claude and its passthrough goes to its
// pane. The tab's title and the session's home go to its session. The other panes of claude's
// window, and the sessions claude makes, keep the server's. show without -v prints an option only
// where set.
func checkClaudesOwn(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, option := range []struct {
		args  []string
		value string
	}{
		{[]string{"-pv", "-t", "=cld-0:", "remain-on-exit"}, "on"},
		{[]string{"-p", "-t", "=cld-0:", "remain-on-exit-format"}, "remain-on-exit-format ''"},
		{[]string{"-w", "-t", "=cld-0:", "remain-on-exit"}, ""},
		{[]string{"-w", "-t", "=cld-0:", "remain-on-exit-format"}, ""},
		{[]string{"-gwv", "remain-on-exit"}, "off"},
		{[]string{"-pv", "-t", "=cld-0:", "allow-passthrough"}, "all"},
		{[]string{"-w", "-t", "=cld-0:", "allow-passthrough"}, ""},
		{[]string{"-v", "-t", "=cld-0:", "set-titles"}, "on"},
		{[]string{"-v", "-t", "=cld-0:", "set-titles-string"},
			"#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-0" +
				"#{?@cld-worktree, [w],}"},
		{[]string{"-v", "-t", "=cld-0:", "@cld-busy"}, busyMarker},
		{[]string{"-v", "-t", "=cld-0:", "@cld-tmux"}, sandbox.RealTmux},
		{[]string{"-v", "-t", "=cld-0:", "@cld-home"}, s.Work},
		{[]string{"-gv", "set-titles"}, "off"},
	} {
		value := s.MustTmux("cld-0", append([]string{"show"}, option.args...)...)
		if value != option.value {
			t.Errorf("show %s is %q, want %q", strings.Join(option.args, " "), value, option.value)
		}
	}
	if hooks := s.MustTmux("cld-0", "show-hooks", "-g", "pane-died"); strings.Contains(hooks, "[") {
		t.Errorf("global pane-died hooks, want none: they go to claude's pane\n%s", hooks)
	}
	hooks := s.MustTmux("cld-0", "show-hooks", "-w", "-t", "=cld-0:", "pane-died")
	if hooks != "" {
		t.Errorf("pane-died hooks of claude's window, want none: they go to claude's pane\n%s",
			hooks)
	}
	panes := strings.Split(s.MustTmux("cld-0", "show-hooks", "-p", "-t", "=cld-0:", "pane-died"),
		"\n")
	if len(panes) != 1 || !strings.Contains(panes[0], "window_active_clients") {
		t.Errorf("pane-died hooks of claude's pane, want one:\n%s", strings.Join(panes, "\n"))
	}
}

// checkSetTwice has a second join -s 0 set the options again on the server, before its new-session
// fails (decision 50.4). A join past its wait for the record's lock would do so. The terminal
// features, at fixed indexes, keep one copy of each entry (decision 30). Without -s, the second
// would take index 1. The fake tmux finds no server, and runs the real one after.
func checkSetTwice(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	second := s.RunCldOnTerminal(map[string]string{
		"PATH": filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) +
			s.Env["PATH"],
		"CLD_FAKE_TMUX_REAL": realTmux,
	}, "join", "-s", "0", "--new")
	if want := "duplicate session: cld-0\n"; second.Code != 1 || second.Stderr != want {
		t.Errorf("a second cld join: exit %d, stderr %q, want exit 1, stderr %q",
			second.Code, second.Stderr, want)
	}
	features := strings.Split(s.MustTmux("cld-0", "show", "-sv", "terminal-features"), "\n")
	for _, entry := range []string{
		"xterm*:extkeys:hyperlinks", "wezterm:hyperlinks", "alacritty:hyperlinks",
	} {
		others := func(f string) bool { return f != entry }
		if count := len(slices.DeleteFunc(slices.Clone(features), others)); count != 1 {
			t.Errorf("%d %s entries in terminal-features, want 1", count, entry)
		}
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want the first one", len(probes))
	}
}

// checkKeyBindings checks the keys cld binds and unbinds.
func checkKeyBindings(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	// "list-keys -T prefix C-q" would be shorter, but tmux 3.7 prints nothing for it.
	bindings := strings.Split(s.MustTmux("cld-0", "list-keys", "-T", "prefix"), "\n")
	sendPrefix := []string{"bind-key", "-T", "prefix", "C-q", "send-prefix"}
	if !slices.ContainsFunc(bindings, func(binding string) bool {
		return slices.Equal(strings.Fields(binding), sendPrefix)
	}) {
		t.Errorf("C-q C-q is not bound to send-prefix:\n%s", //nolint:dupword // the key, twice
			strings.Join(bindings, "\n"))
	}
	// s, ( , ) and L run cld, in place of tmux's choose-tree and switch-client (see
	// TestSwitchKeys); tmux writes ( and ) with a "\" before them.
	for _, key := range []string{"s", "(", ")", "L"} {
		if !slices.ContainsFunc(bindings, func(binding string) bool {
			fields := strings.Fields(binding)
			return len(fields) > 5 && strings.TrimPrefix(fields[3], `\`) == key &&
				fields[4] == "run-shell" && fields[5] == "-b"
		}) {
			t.Errorf("C-q %s is not bound to run-shell -b:\n%s", key, strings.Join(bindings, "\n"))
		}
	}
	// tmux hands a mouse key it has no binding for to the pane: a Ctrl+click and an
	// Alt+right-click reach claude whole (see TestContractClicks). tmux's other mouse bindings
	// stay.
	var keys []string
	for binding := range strings.SplitSeq(s.MustTmux("cld-0", "list-keys", "-T", "root"), "\n") {
		if fields := strings.Fields(binding); len(fields) > 3 {
			keys = append(keys, fields[3])
		}
	}
	if !slices.Contains(keys, "MouseDown1Pane") || slices.Contains(keys, "C-MouseDown1Pane") ||
		slices.Contains(keys, "M-MouseDown3Pane") {
		t.Errorf("root bindings %q, want tmux's without C-MouseDown1Pane and M-MouseDown3Pane",
			keys)
	}
}
