package tests

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// list shows claude's status after each session's state, from the title's hooks (decision 49).
// That is idle (a), busy (b), waiting (c), and none for another value (d) or once claude has exited
// (e). A pane split off shows the status claude left (f, g). The interactive list draws waiting in
// bold, and join -s and detach -s describe each session so too.
func TestListShowsClaudesStatus(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startStatuses(t, s)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitClients(t, s, 1)

	table := "NAME  STATE              LAST ACTIVE  DIRECTORY\n"
	for _, row := range statusRows {
		table += fmt.Sprintf("%-4s  %-17s  now          %s\n", row[0], row[1], s.Work)
	}
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != table || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, table)
	}
	completions := "a\tdetached, idle\nb\tattached, busy\nc\tdetached, waiting\nd\tdetached\n" +
		"e\texited\nf\tdetached, waiting\ng\tdetached, idle\n:4\n"
	for _, command := range []string{"join", "detach"} {
		result := s.RunCld(nil, "__complete", command, "-s", "")
		if result.Code != 0 || result.Stdout != completions {
			t.Errorf("__complete %s -s: exit %d, stdout %q, want %q",
				command, result.Code, result.Stdout, completions)
		}
	}
	checkStatusList(t, s, table)
}

// statusRows are the sessions TestListShowsClaudesStatus starts, and their states.
var statusRows = [][2]string{
	{"a", "detached, idle"}, {"b", "attached, busy"}, {"c", "detached, waiting"}, {"d", "detached"},
	{"e", "exited"}, {"f", "detached, waiting"}, {"g", "detached, idle"},
}

// startStatuses starts sessions a to g, detached, and leaves each in its status. e exits in a
// turn, which leaves the option busy. f's split pane is active once claude has failed, and g's
// keeps the session once claude's /exit has closed its pane.
func startStatuses(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	probes := detachedSessions(t, s, "a", "b", "c", "d", "e", "f", "g")
	probes["a"].Hook("Stop", `{}`)
	probes["b"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	probes["c"].Hook("PermissionRequest", `{"tool_name":"Bash"}`)
	// A tab, which would split the line's fields if tmux wrote it.
	s.MustTmux("cld-d", "set", "-t", "=cld-d:", "@cld-status", "bu\tsy")
	probes["e"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	probes["e"].Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude e to exit", func() bool {
		return s.Format("cld-e", "#{pane_dead}") == "1"
	})
	if status := s.Format("cld-e", "#{@cld-status}"); status != "busy" {
		t.Errorf("@cld-status of the session whose claude exited is %q, want busy", status)
	}
	probes["f"].Hook("PermissionRequest", `{"tool_name":"Bash"}`)
	s.MustTmux("cld-f", "split-window", "-d", "-t", "=cld-f:", "-c", s.Work, "sleep", "600")
	s.MustTmux("cld-f", "select-pane", "-t", "=cld-f:.1")
	probes["f"].Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude f to exit", func() bool {
		return s.Format("cld-f", "#{pane_dead}") == "1\n0"
	})
	probes["g"].Hook("Stop", `{}`)
	s.MustTmux("cld-g", "split-window", "-d", "-t", "=cld-g:", "-c", s.Work, "sleep", "600")
	probes["g"].Send("exit 0")
	sandbox.WaitFor(t, 10*time.Second, "claude g's pane to close", func() bool {
		return s.Format("cld-g", "#{pane_dead}") == "0"
	})
}

// checkStatusList opens the interactive list on the sessions, moves its selection and narrows it,
// checking what it draws, and leaves it, checking that it prints table.
func checkStatusList(t *testing.T, s *sandbox.Sandbox, table string) {
	t.Helper()
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	for _, step := range []struct {
		keys     []string
		columns  int
		selected string
	}{
		{nil, 120, "a"},
		{[]string{"Down", "Down"}, 120, "c"},
		{nil, 21, "c"}, // > c     detached, wai
		{[]string{"Up"}, 21, "b"},
	} {
		if step.keys != nil {
			term.Keys(step.keys...)
		}
		if step.columns != 120 {
			term.Resize(step.columns, 10)
		}
		text, styled := statusShown(s.Work, step.selected, step.columns)
		waitLines(t, term, text...)
		if got := cells(term.Styled()); !slices.Equal(got, styled) {
			t.Errorf("session %s selected, %d columns, the list is\n%s\nwant\n%s",
				step.selected, step.columns, strings.Join(got, "\n"), strings.Join(styled, "\n"))
		}
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, table)
}

// statusShown is the list of statusRows in work with the session selected, in a terminal columns
// wide, as text and styled as cells has it: waiting in bold as far as the line holds it.
func statusShown(work, selected string, columns int) (text, styled []string) {
	header := strings.TrimRight(
		cutTo("  NAME  STATE              LAST ACTIVE  DIRECTORY", columns), " ")
	text, styled = []string{header}, []string{"[]" + header}
	for _, row := range statusRows {
		marker, attributes := " ", ""
		if row[0] == selected {
			marker, attributes = ">", "inverse=7"
		}
		full := fmt.Sprintf("%s %-4s  %-17s  now          %s", marker, row[0], row[1], work)
		line := cutTo(full, columns)
		text = append(text, strings.TrimRight(line, " "))
		if attributes == "" {
			line = strings.TrimRight(line, " ")
		}
		if i := strings.Index(full, "waiting"); i >= 0 && i < len(line) {
			end := min(i+len("waiting"), len(line))
			bold := "[" + strings.TrimSpace("intensity=1 "+attributes) + "]" + line[i:end]
			if end < len(line) {
				bold += "[" + attributes + "]"
			}
			line = line[:i] + bold + line[end:]
		}
		styled = append(styled, "["+attributes+"]"+line)
	}
	footer := footerIn(columns)
	return append(text, "", footer), append(styled, "", "[intensity=2]"+footer)
}
