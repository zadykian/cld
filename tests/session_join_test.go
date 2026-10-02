package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join refuses what would be lost on a session that runs, before touching it, and the attached
// terminals carry on (decision 50.2). The message names the session as join takes it, -n and -s
// split at its last "-". --detach-others is never lost: join refuses only the terminal it lacks.
func TestJoinRefusesWhatWouldBeLost(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "dup")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	second := startCld(t, s, "tmux", nil, "join", "-n", "my-api", "-s", "fix")
	s.WaitProbes(2)
	waitClients(t, s, 2)

	for _, test := range joinLostCases {
		result := s.RunCld(nil, test.args...)
		if result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%q: exit %d, stderr %q, want exit 1, stderr %q",
				test.args, result.Code, result.Stderr, test.want)
		}
		if result.Stdout != "" {
			t.Errorf("%q printed %q before failing", test.args, result.Stdout)
		}
	}
	if probes := s.Probes(); len(probes) != 2 || !first.Running() || !second.Running() {
		t.Errorf("%d claude processes, clients running: %v, %v; want the original two, attached",
			len(probes), first.Running(), second.Running())
	}
	waitClients(t, s, 2)
}

// joinLostCases are the joins TestJoinRefusesWhatWouldBeLost refuses, and their messages.
var joinLostCases = []struct {
	args []string
	want string
}{
	{[]string{"join", "-s", "dup", "-w"}, lostRefusal("dup", "-w", "-s dup")},
	{[]string{"join", "-s", "dup", "--new"}, lostRefusal("dup", "--new", "-s dup")},
	{[]string{"join", "-s", "dup", "--resume", "other"}, lostRefusal("dup", "--resume", "-s dup")},
	{[]string{"join", "-s", "dup", "--fork", "--resume", "other"},
		lostRefusal("dup", "--resume", "-s dup")},
	{[]string{"join", "-s", "dup", "--", "--model", "opus"},
		lostRefusal("dup", "the words after --", "-s dup")},
	{[]string{"join", "-s", "dup", "--new", "-w", "--", "go"}, lostRefusal("dup", "-w", "-s dup")},
	{[]string{"join", "-n", "my-api", "-s", "fix", "-w"},
		lostRefusal("my-api-fix", "-w", "-n my-api -s fix")},
	{[]string{"join", "-n", "my", "-s", "api-fix", "--new"},
		lostRefusal("my-api-fix", "--new", "-n my-api -s fix")},
	{[]string{"join", "-s", "my-api-fix", "--", "go"},
		lostRefusal("my-api-fix", "the words after --", "-n my-api -s fix")},
	{[]string{"join", "-s", "dup", "--detach-others"},
		"cld: join needs a terminal, and its input is not one\n"},
}

// lostRefusal is join's refusal of option, which would be lost on session name, that options name.
func lostRefusal(name, option, options string) string {
	return "cld: session '" + name + "' exists, and " + option + " would be lost: " +
		"its claude has started; attach to it with cld join " + options +
		", or give another -s SUFFIX\n"
}

// join finds only the session it names: session review, on a server of its own, is not session
// rev. So join -s rev creates rev beside it. TestSeesOnlyItsOwnSessions checks a session like that
// on the named session's own server.
func TestJoinFindsOnlyItsSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "review")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	startCld(t, s, "tmux", nil, "join", "-s", "rev")
	probes := s.WaitProbes(2)
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-rev", "cld-review"}) {
		t.Errorf("sessions %q, want [cld-rev cld-review]", sessions)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-rev", "cld-review"}) {
		t.Errorf("clients on %q, want one on each", clients)
	}
	names := []string{probes[0].Argv[1], probes[1].Argv[1]}
	if !slices.Contains(names, "cld-rev") {
		t.Errorf("claudes named %q, want one cld-rev", names)
	}
}

// join acts by the session's state (decision 50.1). It attaches to one that runs, detached (a),
// attached (b) or with claude exited (c), starting no claude. It resumes an ended one (d) by its
// entry's ID, where it ran, and creates one where there is none, or without -s.
func TestJoinStates(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	probes := detachedSessions(t, s, "a", "c")
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(3)
	waitClients(t, s, 1)
	probes["c"].Send("exit 3")
	sandbox.WaitFor(t, 10*time.Second, "claude of c to exit", func() bool {
		return s.Format("cld-c", "#{pane_dead}") == "1"
	})
	writeEntry(t, s, "d", elsewhere, firstID)

	for _, test := range []joinStateCase{
		{"detached", "a", []string{"join", "-s", "a"}, 1, nil, ""},
		{"attached", "b", []string{"join", "-s", "b"}, 2, nil, ""},
		{"exited", "c", []string{"join", "-s", "c"}, 1, nil, ""},
		{"ended", "d", []string{"join", "-s", "d"}, 1, []string{"--resume", firstID}, elsewhere},
		{"unknown", "e", []string{"join", "-s", "e"}, 1, []string{}, s.Work},
		{"no -s", "0", []string{"join"}, 1, []string{}, s.Work},
	} {
		test.join(t, s)
	}
	want := []string{"cld-0", "cld-a", "cld-b", "cld-c", "cld-d", "cld-e"}
	if sessions := s.Sessions(); !slices.Equal(sessions, want) {
		t.Errorf("sessions %q, want cld-0 and cld-a to cld-e", sessions)
	}
}

// joinStateCase is a join of TestJoinStates.
type joinStateCase struct {
	name, session string
	args          []string
	// clients is how many terminals are then on the session, with this one
	clients int
	// resume is what claude gets after its settings where join starts one, and dir where it
	// runs; nil where join starts none
	resume []string
	dir    string
}

// join runs the case's join, and checks the terminals on its session and the claude it starts.
func (c joinStateCase) join(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	name := "cld-" + c.session
	before := len(s.Probes())
	term := startCld(t, s, "tmux", nil, c.args...)
	sandbox.WaitFor(t, 10*time.Second, c.name+": the terminals on "+name, func() bool {
		count := 0
		for _, client := range s.Clients() {
			if client == name {
				count++
			}
		}
		return count == c.clients
	})
	if !term.Running() {
		t.Errorf("%s: cld is not attached", c.name)
	}
	if c.resume == nil {
		time.Sleep(300 * time.Millisecond)
		if after := len(s.Probes()); after != before {
			t.Errorf("%s: %d claudes started, want none", c.name, after-before)
		}
		return
	}
	var probe *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, c.name+": the claude of "+name, func() bool {
		for _, p := range s.Probes() {
			if p.Argv[1] == name {
				probe = p
			}
		}
		return probe != nil
	})
	want := append([]string{"--name", name, "--settings", sessionSettings(s, name, c.dir)},
		c.resume...)
	if !slices.Equal(probe.Argv, want) {
		t.Errorf("%s: claude arguments %q, want %q", c.name, probe.Argv, want)
	}
	if probe.Cwd != c.dir {
		t.Errorf("%s: claude runs in %s, want %s", c.name, probe.Cwd, c.dir)
	}
}

// On an ended session, join refuses -w, which would be lost (decision 50.2). --new makes the
// session anew, in the worktree with -w, and --resume SESSION resumes that, both here, not where
// the session ran. The entry then names this directory, with no ID until claude's hook gives one.
func TestJoinOverAnEndedSession(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// claude is what claude gets after its settings; nil where join refuses
		claude []string
	}{
		{[]string{"join", "-s", "x", "-w"}, nil},
		{[]string{"join", "-s", "x", "--new"}, []string{}},
		{[]string{"join", "-s", "x", "--new", "-w"}, []string{"--worktree", "cld-work-x"}},
		{[]string{"join", "-s", "x", "--resume", "other"}, []string{"--resume", "other"}},
		{[]string{"join", "-s", "x", "--resume", "other", "--fork"},
			[]string{"--resume", "other", "--fork-session"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			work := committedWork(t, s)
			elsewhere := filepath.Join(s.Root, "elsewhere")
			if err := os.Mkdir(elsewhere, 0o755); err != nil {
				t.Fatal(err)
			}
			writeEntry(t, s, "work-x", elsewhere, firstID)
			if test.claude == nil {
				checkWorktreeLost(t, s, work, test.args)
				return
			}
			startCldIn(t, s, "tmux", work, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			flags := settings(s, sandbox.RealTmux, sandbox.RealGit, "cld-work-x", work,
				slices.Contains(test.args, "-w"))
			want := append([]string{"--name", "cld-work-x", "--settings", flags}, test.claude...)
			if !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, work)
			}
			if got, want := readEntry(s, "work-x"), entry("work-x", work, ""); got != want {
				t.Errorf("entry %q, want %q", got, want)
			}
		})
	}
}

// checkWorktreeLost runs cld with args in work, and checks that it refuses -w on the ended session
// work-x, starting no claude.
func checkWorktreeLost(t *testing.T, s *sandbox.Sandbox, work string, args []string) {
	t.Helper()
	want := "cld: session 'work-x' has ended, and -w would be lost: " +
		"claude takes its conversation back to its worktree itself; " +
		"resume it with cld join -n work -s x, or give --new for a new conversation\n"
	result := s.RunCldIn(work, nil, args...)
	if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if probes := s.Probes(); len(probes) != 0 {
		t.Errorf("%d claudes started, want none", len(probes))
	}
}
