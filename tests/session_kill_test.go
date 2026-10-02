package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// kill ends the session it names, its claude and its server, and the other sessions carry on. The
// terminal attached is left clean, and its cld exits 0: the session goes before the server, whose
// exit alone would give status 1 (docs/design/findings/tmux-sessions.md).
func TestKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// The terminal runs cld a in a shell that writes down its exit status.
	status := filepath.Join(s.Root, "a.status")
	a := terminal.New(t, "tmux", s)
	a.Start(append([]string{"sh", "-c", `"$@"; echo $? >"$0"`, status},
		s.CldArgv("join", "-s", "a")...), s.Env, s.Work)
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)

	result := s.RunCld(nil, "kill", "-s", "a")
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output",
			result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["cld-a"].Alive()
	})
	sandbox.WaitFor(t, 10*time.Second, "cld a to return", func() bool { return !a.Running() })
	if code, err := os.ReadFile(status); err != nil || string(code) != "0\n" {
		t.Errorf("cld a exited with status %q, want 0:\n%s",
			strings.TrimSpace(string(code)), strings.TrimSpace(a.Screen()))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if modes := a.Modes(); modes.AltScreen || modes.Mouse {
		t.Errorf("terminal modes after the kill %+v, want none", modes)
	}
	if !probes["cld-b"].Alive() {
		t.Error("claude b did not survive session a's kill")
	}
	if _, err := s.Tmux("cld-a", "list-sessions"); err == nil {
		t.Error("session a's server survived its kill")
	}

	if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
		t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "session b's server to exit", func() bool {
		_, err := s.Tmux("cld-b", "list-sessions")
		return err != nil
	})
}

// kill and detach act only on the session they name: without its server there is none, and
// session review, on a server of its own, is not session rev. TestSeesOnlyItsOwnSessions checks a
// session like that on the named session's own server.
func TestKillRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev' (see cld list)\n"
	for _, command := range []string{"kill", "detach"} {
		result := s.RunCld(nil, command, "-s", "rev")
		if result.Code != 1 || result.Stderr != want {
			t.Errorf("%s without a server: exit %d, stderr %q, want exit 1, stderr %q",
				command, result.Code, result.Stderr, want)
		}
	}

	startCld(t, s, "tmux", nil, "join", "-s", "review")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for _, command := range []string{"kill", "detach"} {
		result := s.RunCld(nil, command, "-s", "rev")
		if result.Code != 1 || result.Stderr != want {
			t.Errorf("%s beside cld-review: exit %d, stderr %q, want exit 1, stderr %q",
				command, result.Code, result.Stderr, want)
		}
	}
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-review"}) || !probe.Alive() {
		t.Errorf("sessions %q, claude alive: %v; want cld-review running", sessions, probe.Alive())
	}
}

// A server outlives its session where the tmux sessions claude made keep it running (decision 13).
// list shows the session ended, and join and detach refuse the name, pointing at kill, which ends
// the server; join --new then starts a fresh one. A server without any session, or where session
// cld-NAME has been made meanwhile, kill leaves and refuses.
func TestLingeringServer(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	lingering := outliveSessionA(t, s)
	checkOutlivedRefused(t, s)

	startCld(t, s, "tmux", nil, "join", "-s", "a", "--new")
	s.WaitProbes(2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
	if server := s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}"); server == lingering {
		t.Errorf("join reached the server %s that outlived a, want a fresh one", server)
	}
	checkSessionlessServerKept(t, s)
	checkSessionMadeMeanwhile(t, s)
}

// outliveSessionA starts session a, whose claude makes a tmux session on its server and exits, and
// returns the pid of the server left running.
func outliveSessionA(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a/side"}) {
		t.Fatalf("sessions %q, want [cld-a/side]", sessions)
	}
	return s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}")
}

// checkOutlivedRefused checks what list, join and detach make of session a, whose server outlived
// it, and that kill then ends the server.
func checkOutlivedRefused(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	const refused = "cld: session 'a' has ended, but its tmux server still runs " +
		"(see tmux -L cld-a ls); end it with cld kill -s a\n"
	for _, test := range []struct {
		args         []string
		code         int
		stdout, want string
	}{
		{[]string{"list"}, 0, "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
			"a     ended     -            " + s.Work + "\n", ""},
		{[]string{"join", "-s", "a"}, 1, "", refused},
		{[]string{"detach", "-s", "a"}, 1, "", refused},
		{[]string{"join", "-s", "a", "--new"}, 1, "", refused},
		{[]string{"join", "-s", "a", "--resume", "SESSION"}, 1, "", refused},
		{[]string{"join", "-s", "a", "-w"}, 1, "", refused},
		{[]string{"kill", "-s", "a"}, 0, "", ""},
	} {
		result := s.RunCld(nil, test.args...)
		if result.Code != test.code || result.Stdout != test.stdout || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr,
				test.code, test.stdout, test.want)
		}
	}
	if sessions := s.Sessions(); len(sessions) != 0 || len(s.Probes()) != 1 {
		t.Errorf("sessions %q, %d claude processes; want none and the first one",
			sessions, len(s.Probes()))
	}
}

// checkSessionlessServerKept checks that kill, join and detach refuse a marked server without any
// session, as the server a cld join is starting is (see TestLeavesAForeignServerAlone), and that
// kill leaves it.
func checkSessionlessServerKept(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	s.MustTmux("cld-b", "start-server", ";", "set", "-s", "exit-empty", "off", ";",
		"set", "-s", "@cld", "1")
	const empty = "cld: session 'b' has ended, but its tmux server still runs " +
		"(see tmux -L cld-b ls)\n"
	for _, command := range []string{"kill", "join", "detach"} {
		result := s.RunCld(nil, command, "-s", "b")
		if result.Code != 1 || result.Stdout != "" || result.Stderr != empty {
			t.Errorf("%s -s b: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				command, result.Code, result.Stdout, result.Stderr, empty)
		}
	}
	if _, err := s.Tmux("cld-b", "list-sessions"); err != nil {
		t.Errorf("the server without a session: %v, want it running", err)
	}
}

// checkSessionMadeMeanwhile checks kill on a server that looks as if it outlived its session, when
// a tmux first on the PATH makes session cld-NAME as kill reads the server (display-message), or
// after that read (kill-server). kill refuses the first, and its own tmux command ends neither.
func checkSessionMadeMeanwhile(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, test := range []struct {
		name, at string
		code     int
		want     string
	}{
		{"c", "display-message", 1,
			"cld: session 'c' has ended, but its tmux server still runs (see tmux -L cld-c ls)\n"},
		{"d", "kill-server", 0, ""},
	} {
		s.MustTmux("cld-"+test.name, "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", "side", "sleep", "600")
		env := wrapTmux(t, s, "case \"$*\" in *"+test.at+"*)\n"+
			"\ttmux -L cld-"+test.name+" new-session -d -s cld-"+test.name+" sleep 600 ;;\n"+
			"esac\n")
		result := s.RunCld(env, "kill", "-s", test.name)
		if result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("kill -s %s, the session made before %s: exit %d, stdout %q, stderr %q, "+
				"want exit %d, stderr %q", test.name, test.at, result.Code, result.Stdout,
				result.Stderr, test.code, test.want)
		}
		sessions := s.MustTmux("cld-"+test.name, "list-sessions", "-F", "#{session_name}")
		if sessions != "cld-"+test.name+"\nside" {
			t.Errorf("sessions on server cld-%s %q, want cld-%[1]s and side", test.name, sessions)
		}
	}
}
