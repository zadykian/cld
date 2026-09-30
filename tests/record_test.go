package tests

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// cld's record of its sessions, in the sandbox's home directory: the entry join writes, the hooks
// through which claude gives it the conversation's ID and its time, the sessions that have ended
// in list, kill's refusal, join bringing them back by the recorded ID in the recorded directory,
// the interactive list's Enter and Ctrl+X on a session that has ended, the indexes join gives,
// what expires, the lock that two joins at once take, and a record cld cannot write.
// Where a test needs no session, cld runs the fake tmux (see fakeTmux).

// Conversations' IDs, as claude gives them.
const (
	firstID  = "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"
	secondID = "7e6d5c4b-3a29-4817-a6f5-e4d3c2b1a098"
)

// fakeTmux is the environment in which cld finds the fake tmux first on the PATH, which passes the
// version check, says that no server runs, and records the command join would run rather than
// run it (see probe): cld then makes no session, and prints the title of the one it
// would have made.
func fakeTmux(s *sandbox.Sandbox) map[string]string {
	return map[string]string{
		"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
	}
}

// readEntry is session name's entry in cld's record in s, "" where there is none.
func readEntry(s *sandbox.Sandbox, name string) string {
	data, _ := os.ReadFile(entryFile(s, name))
	return string(data)
}

// touched reports an entry of session name in s that was not written or touched within a minute.
func touched(t *testing.T, s *sandbox.Sandbox, name, after string) {
	t.Helper()
	info, err := os.Stat(entryFile(s, name))
	if err != nil {
		t.Fatal(err)
	}
	if since := time.Since(info.ModTime()); since > time.Minute {
		t.Errorf("entry written %v ago after %s, want now", since.Round(time.Second), after)
	}
}

// age makes the files paths as old as ago, as their time goes.
func age(t *testing.T, ago time.Duration, paths ...string) {
	t.Helper()
	then := time.Now().Add(-ago)
	for _, path := range paths {
		if err := os.Chtimes(path, then, then); err != nil {
			t.Fatal(err)
		}
	}
}

// join writes the entry of the session it makes - its name and directory, and no conversation yet
// - before tmux starts it. claude's SessionStart hook writes it again with the conversation's
// ID from its input, as claude starts and after /clear, and leaves it as it is for an ID that is
// no conversation's; the Stop and SessionEnd hooks touch it, as claude answers and as a
// conversation ends. The hooks run in claude's directory, wherever that goes, and keep a
// directory that holds quotes as it is.
func TestRecordHooks(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, `it's "x"`)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	probe := s.WaitProbes(1)[0]
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Errorf("entry as claude starts %q, want %q", got, want)
	}
	probe.Send("cd " + s.Root)
	for _, step := range []struct{ event, input, conversation string }{
		{"SessionStart", `{"session_id":"` + firstID + `","transcript_path":"/t/` + firstID + `.jsonl","cwd":"/t","hook_event_name":"SessionStart","source":"startup"}`, firstID},
		{"SessionStart", `{"session_id":"` + secondID + `","source":"clear"}`, secondID},
		{"SessionStart", `{"session_id":"served:unknown","source":"startup"}`, secondID},
		{"SessionStart", `{"source":"compact"}`, secondID},
	} {
		probe.Hook(step.event, step.input)
		if got, want := readEntry(s, "api-1"), entry("api-1", dir, step.conversation); got != want {
			t.Errorf("entry after %s %s: %q, want %q", step.event, step.input, got, want)
		}
	}
	for _, event := range []string{"Stop", "SessionEnd"} {
		age(t, time.Hour, entryFile(s, "api-1"))
		probe.Hook(event, `{"session_id":"`+secondID+`","reason":"other"}`)
		touched(t, s, "api-1", event)
		if got, want := readEntry(s, "api-1"), entry("api-1", dir, secondID); got != want {
			t.Errorf("entry after %s %q, want %q", event, got, want)
		}
	}
	// A forgotten entry stays forgotten.
	forget(t, s, "api-1")
	probe.Hook("SessionEnd", `{"session_id":"`+secondID+`","reason":"other"}`)
	if _, err := os.Stat(entryFile(s, "api-1")); !os.IsNotExist(err) {
		t.Errorf("SessionEnd made an entry that was forgotten: %v", err)
	}
}

// A session whose server no longer runs - killed, here - has ended: list shows it with the
// directory it ran in, and kill and detach refuse it, pointing at join. join then brings its
// conversation back by the ID the hook recorded - whatever the conversation's name has become - in
// the directory the session ran in, from wherever it runs, with the words after -- after the ID,
// and writes the entry again with that ID. Once that directory has gone, join attaches to the
// session while it runs - here refusing for want of a terminal - and once it has ended refuses it,
// with the join that resumes its conversation from where join runs, which does.
func TestJoinRecorded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir, elsewhere := filepath.Join(s.Root, "project"), filepath.Join(s.Root, "elsewhere")
	for _, d := range []string{dir, elsewhere} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	first := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	if result := s.RunCldIn(elsewhere, nil, "kill", "-n", "api", "-s", "1"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !first.Alive() })

	want := "NAME   STATE     LAST ACTIVE  DIRECTORY\n" + "api-1  ended     -            " + dir + "\n"
	if result := s.RunCldIn(elsewhere, nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	for _, command := range []string{"kill", "detach"} {
		want := "cld: session 'api-1' has ended; resume it with cld join -n api -s 1\n"
		if result := s.RunCldIn(elsewhere, nil, command, "-n", "api", "-s", "1"); result.Code != 1 || result.Stderr != want {
			t.Errorf("%s: exit %d, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stderr, want)
		}
	}

	startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "api", "-s", "1", "--", "--model", "opus", "--add-dir", "../y")
	resumed := s.WaitProbes(2)[1]
	if want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", dir), "--resume", firstID, "--model", "opus", "--add-dir", "../y"}; !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
	if resumed.Cwd != dir || resumed.Env["PWD"] != dir {
		t.Errorf("claude runs in %s, PWD %q, want %s", resumed.Cwd, resumed.Env["PWD"], dir)
	}
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, firstID); got != want {
		t.Errorf("entry %q, want %q", got, want)
	}

	// Its directory gone, a session that runs is joined as one that runs.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	running := "cld: join needs a terminal, and its input is not one\n"
	if result := s.RunCldIn(elsewhere, nil, "join", "-n", "api", "-s", "1"); result.Code != 1 || result.Stdout != "" || result.Stderr != running {
		t.Errorf("join of a session that runs: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, running)
	}
	if result := s.RunCldIn(elsewhere, nil, "kill", "-n", "api", "-s", "1"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !resumed.Alive() })
	gone := "cld: session 'api-1' ran in " + dir + ", which no longer exists; resume it from here with cld join -n api -s 1 --resume " + firstID + "\n"
	if result := s.RunCldIn(elsewhere, nil, "join", "-n", "api", "-s", "1"); result.Code != 1 || result.Stdout != "" || result.Stderr != gone {
		t.Errorf("join without its directory: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, gone)
	}
	startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "api", "-s", "1", "--resume", firstID)
	var here *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude to resume here", func() bool {
		for _, probe := range s.Probes() {
			if probe.PID != first.PID && probe.PID != resumed.PID {
				here = probe
			}
		}
		return here != nil
	})
	if want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", elsewhere), "--resume", firstID}; !slices.Equal(here.Argv, want) || here.Cwd != elsewhere {
		t.Errorf("claude arguments %q in %s, want %q in %s", here.Argv, here.Cwd, want, elsewhere)
	}
}

// join --resume SESSION --fork writes its session's entry with no conversation, as join
// --resume does, not with SESSION's ID: claude resumes a copy under a new ID, which its
// SessionStart hook writes into the entry. join, once the session has ended, then brings the copy
// back by that ID, without --fork-session.
func TestForkRecorded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, "project")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1", "--fork", "--resume", firstID)
	copied := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Errorf("entry as claude starts %q, want %q", got, want)
	}
	copied.Hook("SessionStart", `{"session_id":"`+secondID+`","source":"resume"}`)
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, secondID); got != want {
		t.Errorf("entry after SessionStart %q, want %q", got, want)
	}
	if result := s.RunCldIn(dir, nil, "kill", "-n", "api", "-s", "1"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !copied.Alive() })
	startCldIn(t, s, "tmux", dir, nil, "join", "-n", "api", "-s", "1")
	resumed := s.WaitProbes(2)[1]
	if want := []string{"--name", "cld-api-1", "--settings", sessionSettings(s, "cld-api-1", dir), "--resume", secondID}; !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
}

// A session whose entry has no conversation - its claude, or its tmux, failed before claude
// started one - is resumed by its name, cld-NAME, but in the directory it ran in, from wherever
// join runs, as one with a conversation is. The fake tmux says that no server runs, and records
// the command.
func TestJoinWithoutConversation(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir, elsewhere := filepath.Join(s.Root, "project"), filepath.Join(s.Root, "elsewhere")
	for _, d := range []string{dir, elsewhere} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if result := s.RunCldOnTerminalIn(dir, fakeTmux(s), "join", "-n", "api", "-s", "1"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join creating api-1: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if got, want := readEntry(s, "api-1"), entry("api-1", dir, ""); got != want {
		t.Fatalf("entry %q, want %q", got, want)
	}
	if result := s.RunCldOnTerminalIn(elsewhere, fakeTmux(s), "join", "-n", "api", "-s", "1"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join resuming api-1: exit %d, stderr %q", result.Code, result.Stderr)
	}
	record := s.FakeTmuxRecord()
	if i := slices.Index(record.Argv, "--resume"); i < 0 || i+1 == len(record.Argv) || record.Argv[i+1] != "cld-api-1" {
		t.Errorf("tmux arguments %q, want --resume cld-api-1", record.Argv)
	}
	if i := slices.Index(record.Argv, "-c"); i < 0 || i+1 == len(record.Argv) || record.Argv[i+1] != dir || record.Cwd != dir {
		t.Errorf("tmux runs in %s, arguments %q, want -c %s and to run there", record.Cwd, record.Argv, dir)
	}
}

// The entry of a session whose server runs stays, however old, with the files beside it - its
// environment and its marks: the next join, which removes the entries older than 30 days, leaves
// them, so that claude's hooks touch the entry again once the session is used - here Stop, as
// claude answers - and it shows as ended once the session has. An entry of that age without its
// server goes, with the files beside it, and so do the files of an entry that has gone - a busy
// mark a hook made as the entry was forgotten - however new.
func TestRecordWhileRunning(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	a := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join", map[string]bool{"a": true})
	writeRestorable(t, s, "c", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"), nil)
	s.WriteFile(companionFile(s, "c", ".busy"), "")
	s.WriteFile(companionFile(s, "o", ".busy"), "")
	month := 31 * 24 * time.Hour
	var old []string
	for _, name := range []string{"a", "c"} {
		for _, ext := range []string{".json", ".env", ".run", ".busy"} {
			if exists(companionFile(s, name, ext)) {
				old = append(old, companionFile(s, name, ext))
			}
		}
	}
	age(t, month, old...)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	if readEntry(s, "a") != entry("a", s.Work, "") {
		t.Errorf("a's entry %q after another join, want it kept", readEntry(s, "a"))
	}
	for _, ext := range []string{".env", ".run"} {
		if !exists(companionFile(s, "a", ext)) {
			t.Errorf("a%s removed with a's server running, want it kept", ext)
		}
	}
	for _, file := range []string{entryFile(s, "c"), companionFile(s, "c", ".env"), companionFile(s, "c", ".run"), companionFile(s, "c", ".busy"), companionFile(s, "o", ".busy")} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("%s: %v, want it removed", filepath.Base(file), err)
		}
	}
	a.Hook("Stop", `{"session_id":"`+firstID+`"}`)
	touched(t, s, "a", "Stop")
	if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !a.Alive() })
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     ended     -            " + s.Work + "\n" + "b     attached  now          " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
}

// Without -s, join gives the index above the highest of the sessions that run and of the entries
// of cld's record - those that have ended, too - and of the highest index the record says it gave
// NAME, so that a forgotten entry's index is not given again; -s's index counts as given too. An
// entry, and an index given, that is older than 30 days counts no more, and join removes it. The
// fake tmux says that no server runs.
func TestRecordNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	named := func(want string, args ...string) {
		t.Helper()
		result := s.RunCldOnTerminal(fakeTmux(s), append([]string{"join"}, args...)...)
		if title := "\x1b]0;✳ cld-" + want + "\a"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
			t.Errorf("join %q: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", args, result.Code, result.Stdout, result.Stderr, title)
		}
	}
	named("0")
	named("1")
	forget(t, s, "1")
	named("2")
	named("7", "-s", "7")
	named("8")
	named("api-0", "-n", "api")
	named("API-1", "-n", "API")
	indexes := filepath.Join(filepath.Dir(filepath.Dir(entryFile(s, "0"))), "indexes.json")
	var given map[string]struct {
		Index int       `json:"index"`
		Time  time.Time `json:"time"`
	}
	if data, err := os.ReadFile(indexes); err != nil || json.Unmarshal(data, &given) != nil {
		t.Fatalf("indexes.json: %v\n%s", err, data)
	}
	if given[""].Index != 8 || given["api-"].Index != 0 || given["API-"].Index != 1 {
		t.Errorf("indexes given %+v, want 8 for none, 0 for api- and 1 for API-", given)
	}

	// 30 days on, what the record holds counts no more.
	var old []string
	for _, name := range []string{"0", "2", "7", "8"} {
		old = append(old, entryFile(s, name))
	}
	age(t, 31*24*time.Hour, old...)
	data, err := os.ReadFile(indexes)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteFile(indexes, strings.ReplaceAll(string(data), time.Now().UTC().Format("2006-01-02"), time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")))
	if result := s.RunCld(nil, "list"); result.Stdout != "NAME   STATE     LAST ACTIVE  DIRECTORY\n"+"API-1  ended     -            "+s.Work+"\n"+"api-0  ended     -            "+s.Work+"\n" {
		t.Errorf("list with the entries expired:\n%s", result.Stdout)
	}
	named("0")
	for _, name := range []string{"2", "7", "8"} {
		if _, err := os.Stat(entryFile(s, name)); !os.IsNotExist(err) {
			t.Errorf("entry %s: %v, want it removed", name, err)
		}
	}
}

// Two joins without -s at once take two indexes, where both took the same before, and the second
// failed as tmux made its session: each holds the lock of cld's record from the name to tmux. The
// first is held, with the lock, in its lookup of cld-5 while the second starts; without the lock
// the second would be held there too, and take the same name. cld runs no tmux for a socket that
// refuses the connection (see TestStaleSocketsRunNoTmux): cld-5's takes connections until the
// lookup goes on, and is gone then, as a server that exits leaves it, so that tmux finds no server
// there.
func TestJoinNextAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(s.SocketDir(), "cld-5"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	lookup := holdLookup(t, s, "5")
	startCld(t, s, "tmux", lookup.env, "join")
	lookup.held(t)
	startCld(t, s, "tmux", lookup.env, "join")
	time.Sleep(time.Second)
	// Closing the listener removes its socket.
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	lookup.release(t)
	s.WaitProbes(2)
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0", "cld-1"}) {
		t.Errorf("sessions %q, want [cld-0 cld-1]", sessions)
	}
}

// Two joins of one session at once, on two terminals, make it once, and both end attached to it:
// the first, which finds no session, makes it, and the lock of cld's record goes with it as it
// becomes tmux, before tmux has made the session - held here, as it is about to. The second then
// takes the lock, and finds no session either, but the start mark the first left, naming the
// process that became tmux, which runs: it waits, and attaches once tmux has made the session.
// Without the start mark, the second would make the session too, and its tmux fail with
// "duplicate session". The same holds for a session that has ended, which the first brings back.
func TestJoinOneSessionAtOnce(t *testing.T) {
	t.Parallel()
	for _, ended := range []bool{false, true} {
		name := "unknown"
		if ended {
			name = "ended"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if ended {
				writeEntry(t, s, "x", s.Work, firstID)
			}
			tmux := holdTmux(t, s, "the first join's tmux making cld-x", "*' new-session -s cld-x '*")
			tmux.start(t)
			first := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
			tmux.held(t)
			second := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
			time.Sleep(time.Second)
			if begun := tmux.begun(t); begun != 1 {
				t.Errorf("%d tmux commands began to make cld-x while the first was held, want 1", begun)
			}
			tmux.release(t)
			waitClients(t, s, 2)
			if clients := s.Clients(); !slices.Equal(clients, []string{"cld-x", "cld-x"}) {
				t.Errorf("clients attached to %q, want both to cld-x", clients)
			}
			if !first.Running() || !second.Running() {
				t.Errorf("first attached: %v, second attached: %v; want both", first.Running(), second.Running())
			}
			probe := s.WaitProbes(1)[0]
			time.Sleep(500 * time.Millisecond)
			if probes := s.Probes(); len(probes) != 1 {
				t.Errorf("%d claudes started, want 1", len(probes))
			}
			var resume []string
			if ended {
				resume = []string{"--resume", firstID}
			}
			// The hooks in the settings name the tmux that holds the test's commands.
			held := filepath.Join(strings.Split(tmux.env["PATH"], string(os.PathListSeparator))[0], "tmux")
			if want := append([]string{"--name", "cld-x", "--settings", settings(s, held, sandbox.RealGit, "cld-x", s.Work, false)}, resume...); !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
				t.Errorf("sessions %q, want [cld-x]", sessions)
			}
			waitScreen(t, second, "probe --name cld-x")
		})
	}
}

// A join that looks the session up as another cld's tmux has started the session's server, but
// not yet made the session there, attaches to it once it is made, also where its lookup answers
// only after that tmux has made the session and removed the start mark: join reads the mark
// before the lookup, and the server without its session, with the mark there, sends it to look
// again. Read after the lookup, the mark gone would have it take the server for one that outlived
// its session, and refuse the name. The test plays the other cld: its entry, the start mark naming
// the test's own process, an empty server of cld's and, while the join's lookup is held, the
// session made there and the mark removed.
func TestJoinAsTmuxMakesTheSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "x", s.Work, "")
	s.MustTmux("cld-x", "start-server", ";", "set", "-s", "exit-empty", "off", ";", "set", "-s", "@cld", "1")
	lookup := holdTmuxOutput(t, s, "the join's lookup of cld-x", lookupPattern("x"))
	lookup.start(t)
	mark := companionFile(s, "x", ".start")
	s.WriteFile(mark, strconv.Itoa(os.Getpid())+"\n")
	term := startCld(t, s, "tmux", lookup.env, "join", "-s", "x")
	lookup.held(t)
	s.MustTmux("cld-x", "new-session", "-d", "-s", "cld-x", "sleep", "600")
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	lookup.release(t)
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-x"}) {
		t.Errorf("clients attached to %q, want [cld-x]", clients)
	}
	if !term.Running() {
		t.Errorf("join ended, screen:\n%s", term.Screen())
	}
	if probes := s.Probes(); len(probes) != 0 {
		t.Errorf("%d claudes started, want none", len(probes))
	}
}

// A start mark that names no cld starting the session is none, and join does not wait for it: one
// that a join whose tmux failed to make the session left - a TERM that tmux does not know - naming
// the client that has exited; and one older than 10 s, whose process ID may name another process
// by then, here the test's own. join brings the one session back, and makes the other, at once.
func TestJoinPassesOverAStaleStartMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCldOnTerminal(map[string]string{"TERM": "cld-no-such-terminal"}, "join", "-s", "y"); result.Code == 0 {
		t.Errorf("join with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail", result.Stderr)
	}
	if !exists(companionFile(s, "y", ".start")) {
		t.Error("no start mark left by the join whose tmux failed")
	}
	old := companionFile(s, "w", ".start")
	s.WriteFile(old, strconv.Itoa(os.Getpid())+"\n")
	age(t, 11*time.Second, old)
	for i, name := range []string{"y", "w"} {
		began := time.Now()
		startCld(t, s, "tmux", nil, "join", "-s", name)
		waitClients(t, s, i+1)
		if took := time.Since(began); took > 7*time.Second {
			t.Errorf("join -s %s attached %v after it started, want it not to wait for the start mark", name, took.Round(time.Millisecond))
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-w", "cld-y"}) {
		t.Errorf("sessions %q, want [cld-w cld-y]", sessions)
	}
	if probes := s.Probes(); len(probes) != 2 {
		t.Errorf("%d claudes started, want 2", len(probes))
	}
}

// The record is in $XDG_STATE_HOME/cld where that is a whole path, and otherwise in the home
// directory's .local/state/cld. Where cld cannot write it, join warns, and makes the session all
// the same, without the hooks that would write it, and with agent view off; list reads no entries
// there.
func TestRecordPlace(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	state := filepath.Join(s.Root, "state")
	env := fakeTmux(s)
	env["XDG_STATE_HOME"] = state
	if result := s.RunCldOnTerminal(env, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Errorf("join: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if data, err := os.ReadFile(filepath.Join(state, "cld", "sessions", "x.json")); err != nil || string(data) != entry("x", s.Work, "") {
		t.Errorf("entry in $XDG_STATE_HOME: %q, %v", data, err)
	}
	if _, err := os.Stat(entryFile(s, "x")); !os.IsNotExist(err) {
		t.Errorf("entry in the home directory: %v, want none", err)
	}
	env["XDG_STATE_HOME"] = "state"
	if result := s.RunCldOnTerminal(env, "join", "-s", "z"); result.Code != 0 || result.Stderr != "" {
		t.Errorf("join: exit %d, stderr %q", result.Code, result.Stderr)
	}
	if entry := readEntry(s, "z"); entry == "" {
		t.Error("no entry in the home directory with a relative $XDG_STATE_HOME")
	}

	blocked := filepath.Join(s.Root, "blocked")
	s.WriteFile(blocked, "")
	env["XDG_STATE_HOME"] = blocked
	result := s.RunCldOnTerminal(env, "join", "-s", "y")
	if want := "cld: warning: cannot record session 'y': " + blocked + ": not a directory\n"; result.Code != 0 || result.Stderr != want {
		t.Errorf("join: exit %d, stderr %q, want exit 0, stderr %q", result.Code, result.Stderr, want)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	if settings := argv[i+1]; strings.Contains(settings, "SessionEnd") || strings.Contains(settings, "session_id") {
		t.Errorf("settings with the record's hooks: %s", settings)
	} else if !strings.HasPrefix(settings, `{"disableAgentView":true,"hooks":{`) {
		t.Errorf("settings without agent view off: %s", settings)
	}
	if result := s.RunCld(env, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("list: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
}

// In the interactive list, a session that has ended has hints of its own: Enter resumes it as
// cld join does - by its conversation's ID, in the directory it ran in - and Ctrl+X twice
// forgets it, taking its row and its entry away. A session whose directory has gone is refused,
// and the list stays open.
func TestListEnded(t *testing.T) {
	t.Parallel()
	// ended makes sessions a, running, and b, in another directory whose name leaves nothing of a
	// session's name, with the conversation firstID, which has ended; and returns b's directory.
	ended := func(t *testing.T, s *sandbox.Sandbox) string {
		t.Helper()
		detachedSessions(t, s, "a")
		dir := filepath.Join(s.Root, "other", "_")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		term := startCldIn(t, s, "tmux", dir, nil, "join", "-s", "b")
		b := s.WaitProbes(2)[1]
		b.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		sandbox.WaitFor(t, 10*time.Second, "cld b to return", func() bool { return !term.Running() })
		return dir
	}
	rows := func(s *sandbox.Sandbox, dir, selected string) []string {
		lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
		for _, row := range []string{"a     detached  now          " + s.Work, "b     ended     -            " + dir} {
			marker := " "
			if row[:1] == selected {
				marker = ">"
			}
			lines = append(lines, marker+" "+row)
		}
		return append(lines, "")
	}

	t.Run("resume", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		dir := ended(t, s)
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term, append(rows(s, dir, "a"), listHints)...)
		term.Keys("Down")
		waitLines(t, term, append(rows(s, dir, "b"), endedHints)...)
		term.Keys("Enter")
		waitScreen(t, term, "probe --name cld-b")
		resumed := s.WaitProbes(3)[2]
		if want := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", dir), "--resume", firstID}; !slices.Equal(resumed.Argv, want) {
			t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
		}
		if resumed.Cwd != dir {
			t.Errorf("claude runs in %s, want %s", resumed.Cwd, dir)
		}
		if title := term.Title(); title != "✳ cld-b" {
			t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
		}
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	t.Run("forget", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		dir := ended(t, s)
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term, append(rows(s, dir, "a"), listHints)...)
		term.Keys("Down")
		armThen(t, term, func() {
			waitLines(t, term, append(rows(s, dir, "b"), forgetArmed)...)
			if readEntry(s, "b") == "" {
				t.Error("the first Ctrl+X forgot b")
			}
		}, "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "> a     detached  now          "+s.Work, "", listHints)
		if entry := readEntry(s, "b"); entry != "" {
			t.Errorf("b's entry %q after the forget, want none", entry)
		}
		if exists(companionFile(s, "b", ".env")) {
			t.Error("b's environment stays after the forget")
		}
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  now          "+s.Work+"\n")
	})

	t.Run("directory gone", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		dir := ended(t, s)
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term, append(rows(s, dir, "a"), listHints)...)
		term.Keys("Down", "Enter")
		waitLines(t, term, append(rows(s, dir, "b"), "session 'b' ran in "+dir+", which no longer exists")...)
		if list.exited() || len(s.Probes()) != 2 {
			t.Errorf("list exited: %v, %d claude processes; want the list open and the first two", list.exited(), len(s.Probes()))
		}
	})
}
