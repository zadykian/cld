package tests

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What list shows of the sessions the fake tmux lists, LAST ACTIVE and claude's status, and the
// idle sessions it ends.

const (
	idleDay = 24 * time.Hour
	// neverAttached is a last attach for a session no terminal has attached to.
	neverAttached = -1
)

// idleCase is session a, idle for idle, as list finds it under CLD_IDLE_DAYS days.
type idleCase struct {
	idle time.Duration
	// attached, unless 0, is how long ago a terminal last attached, idle then being the time since
	// the activity alone
	attached time.Duration
	days     string
	// active is LAST ACTIVE, where the session stays
	active string
	// limit, where the session ends, is CLD_IDLE_DAYS as a time, and ended how long the note says
	// the session was idle
	limit time.Duration
	ended string
}

// name names the case by its idle times and CLD_IDLE_DAYS.
func (c idleCase) name() string {
	name := fmt.Sprintf("idle %v", c.idle)
	switch c.attached {
	case 0:
	case neverAttached:
		name += ", never attached"
	default:
		name += fmt.Sprintf(", attached %v ago", c.attached)
	}
	return fmt.Sprintf("%s, CLD_IDLE_DAYS %q", name, c.days)
}

// line is the line the fake tmux lists for session a.
func (c idleCase) line() string {
	switch c.attached {
	case 0:
		return fakeSession("a", c.idle)
	case neverAttached:
		return fakeSessionAt("a", fakeTime(c.idle), "")
	}
	return fakeSessionAt("a", fakeTime(c.idle), fakeTime(c.attached))
}

var idleCases = []idleCase{
	{idle: 0, active: "now"},
	{idle: 55 * time.Second, active: "now"},
	{idle: 61 * time.Second, active: "1m"},
	{idle: 5*time.Minute + 30*time.Second, active: "5m"},
	{idle: 2*time.Hour + 30*time.Minute, active: "2h"},
	{idle: 3*idleDay + 23*time.Hour, active: "3d"},
	{idle: 29*idleDay + 23*time.Hour, active: "29d"},
	{idle: 5*time.Minute + 30*time.Second, attached: 40 * idleDay, active: "5m"},
	{idle: 40 * idleDay, attached: 2*time.Hour + 30*time.Minute, active: "2h"},
	{idle: 5*time.Minute + 30*time.Second, attached: neverAttached, active: "5m"},
	{idle: 40*idleDay + time.Hour, days: "0", active: "40d"},
	{idle: 40*idleDay + time.Hour, days: "45", active: "40d"},
	{idle: 40*idleDay + time.Hour, limit: 30 * idleDay, ended: "40 days"},
	{idle: 40 * idleDay, attached: 31*idleDay + time.Hour, limit: 30 * idleDay, ended: "31 days"},
	{idle: 31*idleDay + time.Hour, attached: 40 * idleDay, limit: 30 * idleDay, ended: "31 days"},
	{idle: 40*idleDay + time.Hour, attached: neverAttached, limit: 30 * idleDay, ended: "40 days"},
	{idle: 31*idleDay + time.Hour, days: "30", limit: 30 * idleDay, ended: "31 days"},
	{idle: 26 * time.Hour, days: "0.5", limit: 12 * time.Hour, ended: "1 day"},
	{idle: 90 * time.Minute, days: ".05", limit: 72 * time.Minute, ended: "1 hour"},
	{idle: 150 * time.Second, days: "0.001", limit: 86400 * time.Millisecond,
		ended: "2 minutes"},
}

// list shows how long ago each session was last active, and first ends each one idle for longer
// than CLD_IDLE_DAYS days, with a note (decision 46). The later of activity and last attach
// counts. The fake tmux records the kill, which checks again against the cutoff, and prints
// nothing, as tmux does once the kill has ended the server, rather than "kept".
func TestIdleSessionsWithFakeTmux(t *testing.T) {
	t.Parallel()
	for _, test := range idleCases {
		t.Run(test.name(), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			line := test.line()
			started := time.Now()
			result := s.RunCld(fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": line,
				"CLD_IDLE_DAYS":          test.days,
			}), "list")
			if test.ended == "" {
				want := fmt.Sprintf("NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
					"a     detached  %-11s  /w\n", test.active)
				if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
					t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s",
						result.Code, result.Stderr, result.Stdout, want)
				}
				if fakeTmuxRan(s) {
					t.Errorf("tmux ran %q", s.FakeTmuxRecord().Argv)
				}
				return
			}
			want := "cld: ended session 'a', idle for " + test.ended + "\n"
			if result.Code != 0 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stderr %q",
					result.Code, result.Stdout, result.Stderr, want)
			}
			checkIdleKill(t, s, started, test.limit)
		})
	}
}

// idleCutoff finds the cutoff in the kill of an idle session.
var idleCutoff = regexp.MustCompile(`#\{e\|<:#\{session_activity\},([0-9]+)\}`)

// checkIdleKill reports a kill of session a other than an if -F that kills it only while idle
// (decision 46.4). Idle is as of a cutoff of now less limit, now being from started to the check.
func checkIdleKill(t *testing.T, s *sandbox.Sandbox, started time.Time, limit time.Duration) {
	t.Helper()
	argv := s.FakeTmuxRecord().Argv
	cutoff := idleCutoff.FindStringSubmatch(strings.Join(argv, " "))
	if cutoff == nil {
		t.Fatalf("tmux arguments %q compare no activity", argv)
	}
	idle := "#{&&:#{==:#{session_attached},0},#{&&:#{e|<:#{session_activity}," + cutoff[1] +
		"},#{e|<:#{session_last_attached}," + cutoff[1] + "}}}"
	kill := "run-shell '" + strings.ReplaceAll(unmarkCommand(s, "a"), "'", `'\''`) +
		"' ; if -F -t =cld-a: '" + idle + "' 'kill-session -t =cld-a ; kill-server' " +
		"'display-message -p kept'"
	want := []string{"-L", "cld-a", "if", "-F", "-t", "=cld-a:", idle, kill,
		"display-message -p kept"}
	if !slices.Equal(argv, want) {
		t.Errorf("tmux arguments\n%q\nwant\n%q", argv, want)
	}
	seconds, err := strconv.ParseInt(cutoff[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	earliest, latest := started.Add(-limit).Unix(), time.Now().Add(-limit).Unix()+1
	if seconds < earliest || seconds > latest {
		t.Errorf("cutoff %d, want from %d to %d: now less %v", seconds, earliest, latest, limit)
	}
}

// list shows claude's status after the session's state, from the one field tmux writes both in
// (decision 49.1). A claude that exited shows the state alone. STATE is as wide as its longest,
// at least 8, and completion shows both. TestListShowsClaudesStatus has the real tmux write it.
func TestClaudesStatusWithFakeTmux(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ field, state string }{
		{"detached ", "detached"},
		{"detached busy", "detached, busy"},
		{"detached waiting", "detached, waiting"},
		{"detached idle", "detached, idle"},
		{"attached waiting", "attached, waiting"},
		{"exited", "exited"},
	} {
		t.Run(test.field, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			fake := fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": fakeSessionIn("a", test.field, fakeTime(0), fakeTime(0)),
			})
			width := max(len(test.state), 8)
			want := fmt.Sprintf("NAME  %-*s  LAST ACTIVE  DIRECTORY\n"+
				"a     %-*s  now          /w\n", width, "STATE", width, test.state)
			result := s.RunCld(fake, "list")
			if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s",
					result.Code, result.Stderr, result.Stdout, want)
			}
			want = "a\t" + test.state + "\n:4\n"
			result = s.RunCld(fake, "__complete", "join", "-s", "")
			if result.Code != 0 || result.Stdout != want {
				t.Errorf("__complete join -s: exit %d, stdout %q, want %q",
					result.Code, result.Stdout, want)
			}
		})
	}
}
