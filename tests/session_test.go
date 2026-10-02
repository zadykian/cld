package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// How cld uses its tmux server, independent of the outer terminal: cld runs in the baseline
// terminal (a pane of an outer tmux server).

// sessionSettings is the --settings the claude of session name, started in dir, gets in s from a
// cld that finds the sandbox's tmux and git (see settings).
func sessionSettings(s *sandbox.Sandbox, name, dir string) string {
	return settings(s, sandbox.RealTmux, sandbox.RealGit, name, dir, false)
}

// settings is the --settings the claude of session name, cld-NAME, started in dir, gets in s from
// a cld that found tmux and git at those paths: agent view off, in every session, where /bg and ←
// would move the conversation out of it; with fromHead, join -w's worktree branched from HEAD;
// then the hooks that keep claude's status and whether it is in a linked worktree on its
// session, for the tab's title (see TestStatusHooks, TestWorktreeHooks and
// TestHooksOutsideThePane), and the session's entry in cld's record and its busy mark (see
// TestRecordHooks and TestBusyMark). Each of the first runs that tmux on the session's server, by
// the socket in the sandbox's directory, and names the session; the others write or touch the
// entry, in the sandbox's home directory, with the session's name and dir, or make or remove the
// busy mark beside it, and touch the run mark as claude takes a prompt (see TestRestoreIdle).
// That of CwdChanged runs in the background, the record's SessionEnd one
// within claude's own bound, and the others with a timeout of 5 s. Nothing else: no
// remoteControlAtStartup, which would override the user's own setting.
func settings(s *sandbox.Sandbox, tmux, git, name, dir string, fromHead bool) string {
	socket := filepath.Join(s.SocketDir(), name)
	set := func(option, value string) string {
		return `'` + tmux + `' -S '` + socket + `' if -F -t '=` + name + `:' \"#{!=:#{` + option + `},` + value + `}\" \"set -t =` + name + `: ` + option + ` ` + value + `\"`
	}
	status := func(value string) string { return set("@cld-status", value) }
	gitDir := func(which string) string {
		return `\"$('` + git + `' rev-parse --path-format=absolute ` + which + ` 2>/dev/null)\"`
	}
	worktree := `w=0; [ ` + gitDir("--git-dir") + ` = ` + gitDir("--git-common-dir") + ` ] || w=1; ` + set("@cld-worktree", "$w")
	// group is a group of an event's hooks: command, where the event matches matcher, run as how
	// says - with claude's default timeout where how is "" - and hook the event's groups.
	group := func(matcher, command, how string) string {
		if matcher != "" {
			matcher = `"matcher":"` + matcher + `",`
		}
		if how != "" {
			how = `,` + how
		}
		return `{` + matcher + `"hooks":[{"type":"command","command":"` + command + `"` + how + `}]}`
	}
	hook := func(event string, groups ...string) string {
		return `"` + event + `":[` + strings.Join(groups, ",") + `]`
	}
	// on runs each of commands, in a group of its own, and claude waits for it 5 s at most;
	// background runs command, and claude goes on.
	on := func(event, matcher string, commands ...string) string {
		var groups []string
		for _, command := range commands {
			groups = append(groups, group(matcher, command, `"timeout":5`))
		}
		return hook(event, groups...)
	}
	background := func(event, command string) string { return hook(event, group("", command, `"async":true`)) }
	file := entryFile(s, strings.TrimPrefix(name, "cld-"))
	head := `{"name":` + jsonText(strings.TrimPrefix(name, "cld-")) + `,"directory":` + jsonText(dir) + `,"conversation":"`
	temp := `'` + file + `'.$$`
	// The hooks' commands, as the settings' JSON has them.
	escaped := func(command string) string { text := jsonText(command); return text[1 : len(text)-1] }
	record := escaped(`id=$(sed -n 's/.*"session_id" *: *"\([0-9A-Za-z-]*\)".*/\1/p' | head -n 1); ` +
		`if [ -n "$id" ]; then printf '%s%s"}\n' '` + head + `' "$id" >` + temp + ` && mv -f ` + temp + ` '` + file + `'; fi`)
	touch := escaped(`touch -c '` + file + `'`)
	busy := `'` + strings.TrimSuffix(file, ".json") + `.busy'`
	idle := escaped(`rm -f ` + busy)
	busyMark := escaped(`[ ! -e '` + file + `' ] || : >` + busy + `; touch -c '` + strings.TrimSuffix(file, ".json") + `.run'`)
	interrupted := escaped(`if grep -Eq '"is_interrupt": *true'; then rm -f ` + busy + `; fi`)
	stop := escaped(`rm -f ` + busy + `; touch -c '` + file + `'`)
	base := ""
	if fromHead {
		base = `"worktree":{"baseRef":"head"},`
	}
	return `{"disableAgentView":true,` + base + `"hooks":{` + strings.Join([]string{
		background("CwdChanged", worktree),
		on("Elicitation", "", status("waiting")),
		on("ElicitationResult", "", status("busy")),
		on("Notification", "idle_prompt", status("idle"), idle),
		on("PermissionRequest", "", status("waiting")),
		on("PostToolUse", "", status("busy")),
		on("PostToolUseFailure", "", `if grep -Eq '\"is_interrupt\": *true'; then `+status("idle")+`; else `+status("busy")+`; fi`, interrupted),
		hook("SessionEnd", group("", touch, "")),
		on("SessionStart", "", worktree, record),
		on("Stop", "", status("idle"), stop),
		on("StopFailure", "", status("idle"), idle),
		on("UserPromptSubmit", "", status("busy"), busyMark),
	}, ",") + `}}`
}

// jsonText is text as a JSON string, quotes included, without HTML's escapes, as cld writes the
// settings and the entries of its record.
func jsonText(text string) string {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(encoded.String(), "\n")
}

// entryFile is the file of session name's entry in cld's record, in s's home directory.
func entryFile(s *sandbox.Sandbox, name string) string {
	return filepath.Join(s.Home, ".local", "state", "cld", "sessions", name+".json")
}

// forget removes the entries of the sessions names from cld's record in s, as the list's forget
// does: once their sessions end, they are gone rather than ended.
func forget(t *testing.T, s *sandbox.Sandbox, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Remove(entryFile(s, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// entry is session name's entry as cld writes it, and its SessionStart hook: the session started
// in dir, with the conversation of that ID, "" for none yet.
func entry(name, dir, conversation string) string {
	return `{"name":` + jsonText(name) + `,"directory":` + jsonText(dir) + `,"conversation":"` + conversation + "\"}\n"
}

// writeEntry writes session name's entry in cld's record in s, as join would have written it for a
// session in dir, with the conversation of that ID, "" for none yet.
func writeEntry(t *testing.T, s *sandbox.Sandbox, name, dir, conversation string) {
	t.Helper()
	file := entryFile(s, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(file, entry(name, dir, conversation))
}

// Session NAME-SUFFIX is the tmux session cld-NAME-SUFFIX, and claude's --name: NAME is -n's, or
// else the name of the git repository - outside one, as here, of the directory, work - and SUFFIX
// -s's, or else the next index, 0 where no session runs. -n and -s go together, in either order.
// The words after "--" go to claude after cld's own arguments, each as it is: an empty one, a "--",
// and the words cld refuses (see TestRefusesClaudeOptions) as values, after "=" or, for -d, in its
// word.
func TestSessionNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		session string
		// claude is what claude gets after cld's arguments
		claude []string
	}{
		{[]string{"join"}, "cld-work-0", nil},
		{[]string{"join", "-n", "review"}, "cld-review-0", nil},
		{[]string{"join", "--name", "Fix_42-b"}, "cld-Fix_42-b-0", nil},
		{[]string{"join", "-s", "fix"}, "cld-work-fix", nil},
		{[]string{"join", "--suffix", "Fix_42-b"}, "cld-work-Fix_42-b", nil},
		{[]string{"join", "-n", "api", "-s", "fix"}, "cld-api-fix", nil},
		{[]string{"join", "-s", "fix", "--name", "api"}, "cld-api-fix", nil},
		// The spellings of pflag, which reads the options.
		{[]string{"join", "--name=x"}, "cld-x-0", nil},
		{[]string{"join", "-ny"}, "cld-y-0", nil},
		{[]string{"join", "-n=z"}, "cld-z-0", nil},
		{[]string{"join", "-sq"}, "cld-work-q", nil},
		{[]string{"join", "--suffix=7"}, "cld-work-7", nil},
		{[]string{"join", "-nx", "-s=y"}, "cld-x-y", nil},
		// claude's words.
		{[]string{"join", "-s", "a", "--"}, "cld-work-a", nil},
		{[]string{"join", "-n", "m", "--", "--model", "opus", "fix the -p bug"}, "cld-m-0", []string{"--model", "opus", "fix the -p bug"}},
		{[]string{"join", "-s", "p", "--", "--append-system-prompt=-p", "", "-dapi,hooks", "--", "fix it"}, "cld-work-p",
			[]string{"--append-system-prompt=-p", "", "-dapi,hooks", "--", "fix it"}},
	} {
		t.Run(test.session, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := filepath.Join(s.Root, "work")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", dir, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{test.session}) {
				t.Errorf("sessions %q, want [%s]", sessions, test.session)
			}
			if want := append([]string{"--name", test.session, "--settings", sessionSettings(s, test.session, dir)}, test.claude...); !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != dir {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, dir)
			}
		})
	}
}

// tmux takes a command of 16364 bytes at most, each word after its options followed by a NUL (see
// commandLimit in internal/session), and fails on a longer one once it has started its server,
// which leaves the socket. join refuses words for claude that make its command longer - --resume's
// SESSION too - naming its size, before it writes the session's entry in cld's record, checks the
// terminal, which it runs without here, prints the title or starts tmux's server; at the limit,
// counted with the record's hooks, tmux starts claude with them.
func TestCommandLimit(t *testing.T) {
	t.Parallel()
	const limit = 16364
	s := sandbox.New(t)
	refused := func(args ...string) int {
		t.Helper()
		result := s.RunCld(nil, args...)
		var size int
		fmt.Sscanf(result.Stderr, "cld: claude's arguments make tmux's command %d bytes", &size)
		want := fmt.Sprintf("cld: claude's arguments make tmux's command %d bytes, and tmux takes %d at most: "+
			"give claude long text in a file, as with --append-system-prompt-file\n", size, limit)
		if result.Code != 2 || result.Stdout != "" || size <= limit || result.Stderr != want {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q, want exit 2, stderr %q", args[0], result.Code, result.Stdout, result.Stderr, want)
		}
		return size
	}
	long := strings.Repeat("a", 20000)
	fits := long[:len(long)-(refused("join", "-s", "x", "--", "go", long)-limit)]
	if size := refused("join", "-s", "x", "--", "go", fits+"a"); size != limit+1 {
		t.Errorf("a word one byte longer makes the command %d bytes, want %d", size, limit+1)
	}
	refused("join", "-s", "x", "--resume", long)
	refused("join", "-s", "x", "--resume", "a", "--", long)
	if _, err := os.Lstat(filepath.Join(s.SocketDir(), "cld-x")); err == nil {
		t.Errorf("a socket cld-x is left")
	}
	if entry := readEntry(s, "x"); entry != "" {
		t.Errorf("an entry of x is written: %q", entry)
	}
	startCld(t, s, "tmux", nil, "join", "-s", "x", "--", "go", fits)
	probe := s.WaitProbes(1)[0]
	if want := []string{"--name", "cld-x", "--settings", sessionSettings(s, "cld-x", s.Work), "go", fits}; !slices.Equal(probe.Argv, want) {
		t.Errorf("claude arguments %d words, want %d: the last %.20q, want %.20q", len(probe.Argv), len(want), probe.Argv[len(probe.Argv)-1], fits)
	}
}

// tmux starts the claude that join checked. A relative PATH entry before the probe's, "." or an
// empty one, holds a claude of its own, which records that it ran and fails: cld skips it, and
// tmux, handed the probe by its path, does not take it either, as it would the bare word. A
// claude that is a script without #!, which starts the probe, passes the check through /bin/sh
// and starts as tmux's execvp runs it.
func TestStartsTheClaudeItChecks(t *testing.T) {
	t.Parallel()
	const relative = "#!/bin/sh\necho \"$*\" >\"$CLD_PROBE_DIR/relative claude ran\"\nexit 99\n"
	for _, test := range []struct {
		// entry is the PATH entry before the sandbox's; claude there is script, in the working
		// directory for a relative entry.
		name, entry, script string
	}{
		{"after '.'", ".", relative},
		{"after ''", "", relative},
		{"without #!", "bin", "exec '" + filepath.Join(sandbox.ProbeBin, "claude") + "' \"$@\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir, entry := s.Work, test.entry
			if entry == "bin" {
				dir = filepath.Join(s.Root, "bin")
				entry = dir
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			s.WriteProgram(filepath.Join(dir, "claude"), test.script, 0o755)
			startCld(t, s, "tmux", map[string]string{"PATH": entry + string(os.PathListSeparator) + s.Env["PATH"]}, "join")
			probe := s.WaitProbes(1)[0]
			if want := []string{"--name", "cld-0", "--settings", sessionSettings(s, "cld-0", s.Work)}; !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if args, err := os.ReadFile(filepath.Join(s.ProbeDir, "relative claude ran")); err == nil {
				t.Errorf("the claude of the relative entry ran with %q", args)
			}
		})
	}
}

// join -w hands the worktree to claude: claude gets --worktree cld-NAME-SUFFIX, named as the
// session is, with settings that also make it branch a new worktree from HEAD, and starts where cld
// runs; it then makes or reopens the worktree itself and moves into it. The words after "--" come
// after --worktree's value. The repository is named work.
func TestJoinWorktree(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// session is claude's --name and --worktree
		session string
		// after is what claude gets after --worktree's value
		after []string
	}{
		{[]string{"join", "-w"}, "cld-work-0", nil},
		{[]string{"join", "-n", "feat", "--worktree"}, "cld-feat-0", nil},
		{[]string{"join", "-w", "--name=feat"}, "cld-feat-0", nil},
		{[]string{"join", "-wn", "feat"}, "cld-feat-0", nil},
		{[]string{"join", "-s", "feat", "-w"}, "cld-work-feat", nil},
		{[]string{"join", "-ws", "x", "-n", "feat"}, "cld-feat-x", nil},
		{[]string{"join", "-s", "feat", "-w", "--new"}, "cld-work-feat", nil},
		{[]string{"join", "-w", "--", "--effort", "high", "start"}, "cld-work-0", []string{"--effort", "high", "start"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			sub := filepath.Join(repository(t, s, "work"), "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", sub, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			fromHead := settings(s, sandbox.RealTmux, sandbox.RealGit, test.session, sub, true)
			if want := append([]string{"--name", test.session, "--settings", fromHead, "--worktree", test.session}, test.after...); !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != sub {
				t.Errorf("claude starts in %s, want %s", probe.Cwd, sub)
			}
		})
	}
}

// join --resume SESSION makes its session as join makes any, in the current directory, and claude
// gets the same arguments - never -w's - then --resume with SESSION, as one word, whatever it
// holds, and with --fork --fork-session after it; then the words after "--", as join gives them to
// any claude. tmux would end its command at a word ending in ";" (see literal in
// internal/session), and a git repository makes no worktree session. Its name, "_", leaves
// nothing, so -s SUFFIX names the session SUFFIX; without -s the session gets the next index.
func TestJoinResume(t *testing.T) {
	t.Parallel()
	const id = "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"
	for _, test := range []struct {
		args         []string
		name, resume string
		// after is what claude gets after --resume: with --fork, --fork-session, then the words
		// after "--"
		after []string
	}{
		{[]string{"join", "-s", "x", "--resume", "cld-x"}, "x", "cld-x", nil},
		{[]string{"join", "--suffix=x", "--resume=cld-x"}, "x", "cld-x", nil},
		{[]string{"join", "-n", "a", "-s", "x", "--resume", "cld-a-x"}, "a-x", "cld-a-x", nil},
		{[]string{"join", "--resume", id}, "0", id, nil},
		{[]string{"join", "-n", "a", "--resume", id}, "a-0", id, nil},
		{[]string{"join", "-s", "x", "--resume", id}, "x", id, nil},
		{[]string{"join", "-s", "x", "--resume", "a b"}, "x", "a b", nil},
		{[]string{"join", "-s", "x", "--resume", "fix;"}, "x", "fix;", nil},
		{[]string{"join", "-s", "x", "--resume", `fix\;`}, "x", `fix\;`, nil},
		{[]string{"join", "-s", "x", "--resume", "#{session_name}"}, "x", "#{session_name}", nil},
		{[]string{"join", "-s", "x", "--resume", "cld-x", "--", "--mcp-config", "m.json", "--add-dir", "../y"}, "x", "cld-x", []string{"--mcp-config", "m.json", "--add-dir", "../y"}},
		{[]string{"join", "-s", "x", "--resume", "cld-x", "--"}, "x", "cld-x", nil},
		{[]string{"join", "--resume", "a", "--", "--fork-session", "go on;"}, "0", "a", []string{"--fork-session", "go on;"}},
		{[]string{"join", "-s", "x", "--resume", "a", "--", "--"}, "x", "a", []string{"--"}},
		{[]string{"join", "--fork", "--resume", "cld-a-0"}, "0", "cld-a-0", []string{"--fork-session"}},
		{[]string{"join", "-s", "b", "--fork", "--resume", "cld-a-0"}, "b", "cld-a-0", []string{"--fork-session"}},
		{[]string{"join", "--fork", "-n", "a", "--resume", id}, "a-0", id, []string{"--fork-session"}},
		{[]string{"join", "-s", "x", "--fork=true", "--resume", "fix;"}, "x", "fix;", []string{"--fork-session"}},
		{[]string{"join", "-s", "x", "--fork", "--resume", "cld-x-0"}, "x", "cld-x-0", []string{"--fork-session"}},
		{[]string{"join", "-s", "x", "--fork=false", "--resume", "cld-x"}, "x", "cld-x", nil},
		{[]string{"join", "-s", "x", "--fork", "--resume", "a", "--", "--add-dir", "../y"}, "x", "a", []string{"--fork-session", "--add-dir", "../y"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			startCld(t, s, "tmux", nil, test.args...)
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-" + test.name}) {
				t.Errorf("sessions %q, want [cld-%s]", sessions, test.name)
			}
			if want := append([]string{"--name", "cld-" + test.name, "--settings", sessionSettings(s, "cld-"+test.name, s.Work), "--resume", test.resume}, test.after...); !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != s.Work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, s.Work)
			}
			width := max(4, len(test.name))
			list := fmt.Sprintf("%-*s  STATE     LAST ACTIVE  DIRECTORY\n%-*s  attached  now          %s\n", width, "NAME", width, test.name, s.Work)
			if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
			}
		})
	}
}

// join --fork refuses a --resume SESSION that is the name its copy would take, cld-NAME, compared
// as claude compares names - in any case, the spaces around it trimmed - since claude would resume
// a copy of the conversation of that name and give the copy that name too. Where the directory's
// or the repository's name, or the index, makes that name, the refusal comes with exit status 1
// (TestNameOptions has -n and -s alone, with status 2): in repository api with no session of that
// name running or recorded, a copy of cld-api-0 would be session api-0 again. No session is made,
// and claude is run for its version only.
func TestForkOwnName(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// repository is the git repository cld runs in, or where it is empty the work directory,
		// whose name leaves nothing; name is the session's
		repository, name string
	}{
		{[]string{"join", "--fork", "--resume", "cld-0"}, "", "0"},
		{[]string{"join", "-s", "x", "--fork", "--resume", "cld-x"}, "", "x"},
		{[]string{"join", "-s", "x", "--fork", "--resume", " CLD-X\t"}, "", "x"},
		{[]string{"join", "-n", "a", "--fork", "--resume", "cld-a-0"}, "", "a-0"},
		{[]string{"join", "--fork", "--resume", "cld-api-0"}, "api", "api-0"},
		{[]string{"join", "-s", "Fix", "--fork", "--resume", "cld-API-fix"}, "api", "api-Fix"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := s.Work
			if test.repository != "" {
				dir = repository(t, s, test.repository)
			}
			want := "cld: join: --fork would give the copy SESSION's own name, cld-" + test.name + "; give another -s SUFFIX (see cld help)\n"
			if result := s.RunCldIn(dir, nil, test.args...); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
			if probes := s.Probes(); len(probes) != 0 {
				t.Errorf("%d claude processes, want none", len(probes))
			}
		})
	}
}

// tmux would change the directory cld runs in, which cld gives it with -c: tmux ends a command at a
// word ending in ";" (see literal in internal/session), and expands -c as a format (see
// unexpanded), where "#S" is the session's name and "#(touch ran)" runs touch. join still makes its
// session there, with claude in that directory, with --resume and without, and runs nothing. The
// session's path is -c as tmux expanded it: for a directory that does not exist, tmux starts claude
// in the home directory.
func TestDirectoryTmuxWouldChange(t *testing.T) {
	t.Parallel()
	for _, command := range []struct {
		// after is what claude gets after its settings
		args, after []string
	}{
		{[]string{"join", "-n", "a", "-s", "x"}, nil},
		{[]string{"join", "-n", "a", "-s", "x", "--resume", "cld-a-x"}, []string{"--resume", "cld-a-x"}},
	} {
		for _, name := range []string{"w;", "C#S", "x#(touch ran)", "#{session_name};"} {
			args, after := command.args, command.after
			t.Run(strings.Join(args, " ")+" in "+name, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				dir := filepath.Join(s.Work, name)
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				argv := append([]string{"--name", "cld-a-x", "--settings", sessionSettings(s, "cld-a-x", dir)}, after...)
				startCldIn(t, s, "tmux", dir, nil, args...)
				probe := s.WaitProbes(1)[0]
				if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a-x"}) {
					t.Errorf("sessions %q, want [cld-a-x]", sessions)
				}
				if !slices.Equal(probe.Argv, argv) {
					t.Errorf("claude arguments %q, want %q", probe.Argv, argv)
				}
				if probe.Cwd != dir {
					t.Errorf("claude runs in %s, want %s", probe.Cwd, dir)
				}
				if path := s.Format("cld-a-x", "#{session_path}"); path != dir {
					t.Errorf("session path %s, want %s", path, dir)
				}
				if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
					t.Errorf("tmux ran touch from the directory's name")
				}
			})
		}
	}
}

// Outside a git work tree join -w fails before starting anything: claude would say so in a
// session left to kill.
func TestJoinWorktreeRequiresRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: --worktree needs a git repository, and " + s.Work + " is not in one\n"
	if result := s.RunCld(nil, "join", "-s", "feat", "-w"); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// Without -s, join gives a session the SUFFIX above the highest index of the sessions NAME-INDEX
// that run, NAME being the git repository's name, work here, or -n's: a gap stays, a server that
// has outlived its session counts, and so does a server that cld did not start (see
// TestLeavesAForeignServerAlone), as join would refuse either name, and a session whose name
// has NAME in other letters, whose socket would be the same where the socket directory ignores
// case. A stale socket does not count, nor does a session of another NAME, one without NAME, or
// one with no index after it. join --resume SESSION names its session the same way. Outside a
// repository the directory's name takes the repository's place - in the root directory, which
// leaves none, the index alone is the session's name.
func TestDefaultNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// dir is where cld runs: the repository work where it is empty, and otherwise a
		// directory, in the sandbox's root unless it is a whole path
		dir string
		// options go to join, and to join --resume SESSION
		options []string
		// running are the sessions that run before, each on its server, marked as cld marks one;
		// lingering such servers that run without their session, foreign servers that cld did not
		// start, unmarked, each holding a session other, and stale the sockets no server answers on
		running, lingering, foreign, stale []string
		// want are the names that join, then join --resume SESSION, give
		want []string
	}{
		{"in a repository", "", nil, nil, nil, nil, nil, []string{"cld-work-0", "cld-work-1"}},
		{"in a repository, with sessions", "", nil,
			[]string{"cld-work-0", "cld-work-3", "cld-WORK-4", "cld-work-x", "cld-work-6a", "cld-other-9", "cld-9", "cld-work"},
			[]string{"cld-work-5"}, nil, []string{"cld-work-9"}, []string{"cld-work-6", "cld-work-7"}},
		{"outside a repository", "plain", nil, nil, nil, nil, nil, []string{"cld-plain-0", "cld-plain-1"}},
		{"outside a repository, with sessions", "plain", nil,
			[]string{"cld-plain-2", "cld-plain-x-7", "cld-plain-x", "cld-5", "cld-work-9"}, nil, nil, []string{"cld-plain-8"},
			[]string{"cld-plain-3", "cld-plain-4"}},
		{"in the root directory", "/", nil, []string{"cld-2", "cld-plain-7"}, nil, nil, nil, []string{"cld-3", "cld-4"}},
		{"with -n", "", []string{"-n", "api"}, []string{"cld-api-0", "cld-api-x", "cld-work-3"}, nil,
			[]string{"cld-api-4"}, nil, []string{"cld-api-5", "cld-api-6"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			var dir string
			switch {
			case test.dir == "":
				dir = repository(t, s, "work")
			case filepath.IsAbs(test.dir):
				dir = test.dir
			default:
				dir = filepath.Join(s.Root, test.dir)
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range test.running {
				s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", name, "sleep", "600")
			}
			for _, name := range test.lingering {
				s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", "other", "sleep", "600")
			}
			for _, name := range test.foreign {
				s.MustTmux(name, "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
			}
			for _, name := range test.stale {
				staleSocket(t, s, name)
			}
			startCldIn(t, s, "tmux", dir, nil, append([]string{"join"}, test.options...)...)
			s.WaitProbes(1)
			startCldIn(t, s, "tmux", dir, nil, append(append([]string{"join"}, test.options...), "--resume", "SESSION")...)
			var names []string
			for _, probe := range s.WaitProbes(2) {
				names = append(names, probe.Argv[1])
			}
			if slices.Sort(names); !slices.Equal(names, test.want) {
				t.Errorf("claude named %q, want %q", names, test.want)
			}
			want := append(slices.Clone(test.running), test.want...)
			for _, name := range append(slices.Clone(test.lingering), test.foreign...) {
				want = append(want, name+"/other")
			}
			if slices.Sort(want); !slices.Equal(s.Sessions(), want) {
				t.Errorf("sessions %q, want %q", s.Sessions(), want)
			}
		})
	}
}

// What -s and join's default put before SUFFIX or the index is the name of the git repository: that
// of the directory holding the .git that its work trees share, from a subdirectory or a linked
// worktree - one of claude's - too, or else that of the repository's git directory, without
// ".git": a bare repository's, for its worktree, or a submodule's. Outside a work tree - in a
// directory of no repository, or in .git - it is the name of the directory cld runs in, as PWD
// names it where PWD is that directory: a symbolic link's own, not that of the directory it leads
// to. Each run of the characters a NAME cannot have becomes "-", and "-" and "_" go from either
// end; where nothing is left, as for the root directory, the NAME is the index alone, or SUFFIX.
// The fake tmux finds no server: the second join -s fix brings back the session the first made,
// by its entry in cld's record.
func TestNamePrefixes(t *testing.T) {
	t.Parallel()
	// committed makes the repository work with a commit, from which a worktree can branch, and
	// returns its directory.
	committed := func(t *testing.T, s *sandbox.Sandbox) string {
		dir := repository(t, s, "work")
		runGit(t, s, dir, "commit", "-q", "--allow-empty", "-m", "first")
		return dir
	}
	// directory makes the directory name in the sandbox's root, in no repository.
	directory := func(t *testing.T, s *sandbox.Sandbox, name string) string {
		dir := filepath.Join(s.Root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	// link is a symbolic link, link in the sandbox's root, to the directory target.
	link := func(t *testing.T, s *sandbox.Sandbox) string {
		path := filepath.Join(s.Root, "link")
		if err := os.Symlink(directory(t, s, "target"), path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, test := range []struct {
		name string
		// setup makes what cld runs in and returns the directory: by default git init makes the
		// repository name, and cld runs there
		setup func(t *testing.T, s *sandbox.Sandbox) string
		// pwd is whether cld gets PWD, naming that directory
		pwd bool
		// prefix is what the NAME starts with, before the index or SUFFIX
		prefix string
	}{
		{name: "work", prefix: "work-"},
		{name: "work/sub/dir", prefix: "work-"},
		{name: "work/.git", prefix: "git-"},
		{name: "my.site", prefix: "my-site-"},
		{name: ".dotfiles", prefix: "dotfiles-"},
		{name: "a  b", prefix: "a-b-"},
		{name: "café", prefix: "caf-"},
		{name: "__x-", prefix: "x-"},
		{name: "Repo_1", prefix: "Repo_1-"},
		{name: "日本", prefix: ""},
		{name: "a linked worktree", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			work := committed(t, s)
			runGit(t, s, work, "worktree", "add", "-q", "-b", "worktree-cld-x", ".claude/worktrees/cld-x")
			return filepath.Join(work, ".claude", "worktrees", "cld-x", "sub")
		}, prefix: "work-"},
		{name: "a bare repository's worktree", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			runGit(t, s, s.Root, "clone", "-q", "--bare", committed(t, s), "bare.git")
			runGit(t, s, filepath.Join(s.Root, "bare.git"), "worktree", "add", "-q", filepath.Join(s.Root, "tree"))
			return filepath.Join(s.Root, "tree")
		}, prefix: "bare-"},
		{name: "a submodule", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			work := committed(t, s)
			runGit(t, s, s.Root, "init", "-q", "super")
			runGit(t, s, filepath.Join(s.Root, "super"), "-c", "protocol.file.allow=always", "submodule", "add", "-q", work, "module.x")
			return filepath.Join(s.Root, "super", "module.x")
		}, prefix: "module-x-"},
		{name: "a directory", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			return directory(t, s, "plain")
		}, prefix: "plain-"},
		{name: "a directory in a directory", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			return directory(t, s, "plain/.my site")
		}, prefix: "my-site-"},
		{name: "a directory named in another script", setup: func(t *testing.T, s *sandbox.Sandbox) string {
			return directory(t, s, "日本")
		}, prefix: ""},
		{name: "the root directory", setup: func(*testing.T, *sandbox.Sandbox) string { return "/" }, prefix: ""},
		{name: "a symbolic link", setup: link, pwd: true, prefix: "link-"},
		{name: "a symbolic link, without PWD", setup: link, prefix: "target-"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			var dir string
			if test.setup != nil {
				dir = test.setup(t, s)
			} else {
				repository, _, _ := strings.Cut(test.name, "/")
				runGit(t, s, s.Root, "init", "-q", repository)
				dir = filepath.Join(s.Root, test.name)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			fake := map[string]string{
				"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}
			if test.pwd {
				fake["PWD"] = dir
			}
			for _, command := range []struct {
				args []string
				want string
			}{
				{[]string{"join"}, "cld-" + test.prefix + "0"},
				{[]string{"join", "-s", "fix"}, "cld-" + test.prefix + "fix"},
				{[]string{"join", "-s", "fix"}, "cld-" + test.prefix + "fix"},
			} {
				result := s.RunCldOnTerminalIn(dir, fake, command.args...)
				if title := "\x1b]0;\u2733 " + command.want + "\x07"; result.Code != 0 || result.Stdout != title || result.Stderr != "" {
					t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", strings.Join(command.args, " "), result.Code, result.Stdout, result.Stderr, title)
					continue
				}
				if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:3], []string{"-u", "-L", command.want}) {
					t.Errorf("%s: tmux arguments start %q, want -u -L %s", strings.Join(command.args, " "), argv[:3], command.want)
				}
			}
		})
	}
}

// join refuses, where the session runs, what would be lost on it - -w, --new, --resume (with
// --fork too) and the words for claude after "--" - before touching it: the attached client and
// its claude carry on. The message names the option, and the session as join takes it: -n what
// comes before the last "-" of its name and -s what follows - here, where the directory's name
// leaves nothing, -s alone for a name without one. --detach-others is never refused: join goes on
// to attach, and refuses here only the terminal it does not have.
func TestJoinRefusesWhatWouldBeLost(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "dup")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	second := startCld(t, s, "tmux", nil, "join", "-n", "my-api", "-s", "fix")
	s.WaitProbes(2)
	waitClients(t, s, 2)

	lost := func(name, option, options string) string {
		return "cld: session '" + name + "' exists, and " + option + " would be lost: its claude has started; attach to it with cld join " + options + ", or give another -s SUFFIX\n"
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-s", "dup", "-w"}, lost("dup", "-w", "-s dup")},
		{[]string{"join", "-s", "dup", "--new"}, lost("dup", "--new", "-s dup")},
		{[]string{"join", "-s", "dup", "--resume", "other"}, lost("dup", "--resume", "-s dup")},
		{[]string{"join", "-s", "dup", "--fork", "--resume", "other"}, lost("dup", "--resume", "-s dup")},
		{[]string{"join", "-s", "dup", "--", "--model", "opus"}, lost("dup", "the words after --", "-s dup")},
		{[]string{"join", "-s", "dup", "--new", "-w", "--", "go"}, lost("dup", "-w", "-s dup")},
		{[]string{"join", "-n", "my-api", "-s", "fix", "-w"}, lost("my-api-fix", "-w", "-n my-api -s fix")},
		{[]string{"join", "-n", "my", "-s", "api-fix", "--new"}, lost("my-api-fix", "--new", "-n my-api -s fix")},
		{[]string{"join", "-s", "my-api-fix", "--", "go"}, lost("my-api-fix", "the words after --", "-n my-api -s fix")},
		{[]string{"join", "-s", "dup", "--detach-others"}, "cld: join needs a terminal, and its input is not one\n"},
	} {
		result := s.RunCld(nil, test.args...)
		if result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%q: exit %d, stderr %q, want exit 1, stderr %q", test.args, result.Code, result.Stderr, test.want)
		}
		if result.Stdout != "" {
			t.Errorf("%q printed %q before failing", test.args, result.Stdout)
		}
	}
	if probes := s.Probes(); len(probes) != 2 || !first.Running() || !second.Running() {
		t.Errorf("%d claude processes, clients running: %v, %v; want the original two, attached", len(probes), first.Running(), second.Running())
	}
	waitClients(t, s, 2)
}

// join finds only the session it names: session review, on a server of its own, is not session
// rev, although its name starts with rev, so join -s rev creates rev beside it. A session like
// that on the named session's own server is TestSeesOnlyItsOwnSessions' to check.
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
	if names := []string{probes[0].Argv[1], probes[1].Argv[1]}; !slices.Contains(names, "cld-rev") {
		t.Errorf("claudes named %q, want one cld-rev", names)
	}
}

// join is the one command for a session, in each of its states: where the session runs - a
// terminal attached, none, or claude exited - it attaches beside the others, starting no claude;
// where it has ended, cld's record keeping its entry, claude resumes its conversation by the
// entry's ID, in the directory it ran in, wherever join runs; where there is no such session, and
// without -s, it creates it in the current directory. The session a runs detached, b has a
// terminal, c's claude has exited, and d has ended in another directory, where its conversation
// has an ID.
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
	sandbox.WaitFor(t, 10*time.Second, "claude of c to exit", func() bool { return s.Format("cld-c", "#{pane_dead}") == "1" })
	writeEntry(t, s, "d", elsewhere, "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f")

	for _, test := range []struct {
		name, session string
		args          []string
		// clients is how many terminals are then on the session, with this one
		clients int
		// resume is what claude gets after its settings where join starts one, and dir where
		// it runs; nil where join starts none
		resume []string
		dir    string
	}{
		{"detached", "a", []string{"join", "-s", "a"}, 1, nil, ""},
		{"attached", "b", []string{"join", "-s", "b"}, 2, nil, ""},
		{"exited", "c", []string{"join", "-s", "c"}, 1, nil, ""},
		{"ended", "d", []string{"join", "-s", "d"}, 1, []string{"--resume", "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"}, elsewhere},
		{"unknown", "e", []string{"join", "-s", "e"}, 1, []string{}, s.Work},
		{"no -s", "0", []string{"join"}, 1, []string{}, s.Work},
	} {
		name := "cld-" + test.session
		before := len(s.Probes())
		term := startCld(t, s, "tmux", nil, test.args...)
		sandbox.WaitFor(t, 10*time.Second, test.name+": the terminals on "+name, func() bool {
			count := 0
			for _, client := range s.Clients() {
				if client == name {
					count++
				}
			}
			return count == test.clients
		})
		if !term.Running() {
			t.Errorf("%s: cld is not attached", test.name)
		}
		if test.resume == nil {
			time.Sleep(300 * time.Millisecond)
			if after := len(s.Probes()); after != before {
				t.Errorf("%s: %d claudes started, want none", test.name, after-before)
			}
			continue
		}
		var probe *sandbox.Probe
		sandbox.WaitFor(t, 10*time.Second, test.name+": the claude of "+name, func() bool {
			for _, p := range s.Probes() {
				if p.Argv[1] == name {
					probe = p
				}
			}
			return probe != nil
		})
		want := append([]string{"--name", name, "--settings", sessionSettings(s, name, test.dir)}, test.resume...)
		if !slices.Equal(probe.Argv, want) {
			t.Errorf("%s: claude arguments %q, want %q", test.name, probe.Argv, want)
		}
		if probe.Cwd != test.dir {
			t.Errorf("%s: claude runs in %s, want %s", test.name, probe.Cwd, test.dir)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0", "cld-a", "cld-b", "cld-c", "cld-d", "cld-e"}) {
		t.Errorf("sessions %q, want cld-0 and cld-a to cld-e", sessions)
	}
}

// join refuses, where the session has ended and it resumes its conversation, -w, which would be
// lost: claude takes a conversation back to its worktree itself. With --new it makes the session
// anew, a new conversation, in the worktree with -w; with --resume SESSION it resumes that - here
// in the current directory, not the one the session ran in - and the entry then names the new
// conversation's directory, with no ID until claude's hook gives one. The repository is work, and
// the session work-x, whose entry names another directory.
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
		{[]string{"join", "-s", "x", "--resume", "other", "--fork"}, []string{"--resume", "other", "--fork-session"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			work := repository(t, s, "work")
			runGit(t, s, work, "commit", "-q", "--allow-empty", "-m", "first")
			elsewhere := filepath.Join(s.Root, "elsewhere")
			if err := os.Mkdir(elsewhere, 0o755); err != nil {
				t.Fatal(err)
			}
			writeEntry(t, s, "work-x", elsewhere, "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f")
			if test.claude == nil {
				want := "cld: session 'work-x' has ended, and -w would be lost: claude takes its conversation back to its worktree itself; " +
					"resume it with cld join -n work -s x, or give --new for a new conversation\n"
				if result := s.RunCldIn(work, nil, test.args...); result.Code != 1 || result.Stdout != "" || result.Stderr != want {
					t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
				}
				if probes := s.Probes(); len(probes) != 0 {
					t.Errorf("%d claudes started, want none", len(probes))
				}
				return
			}
			startCldIn(t, s, "tmux", work, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			settings := settings(s, sandbox.RealTmux, sandbox.RealGit, "cld-work-x", work, slices.Contains(test.args, "-w"))
			if want := append([]string{"--name", "cld-work-x", "--settings", settings}, test.claude...); !slices.Equal(probe.Argv, want) {
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

// list shows cld's sessions: the name, whether a terminal is attached, and the directory claude
// is in now, also under a locale that is not UTF-8. It asks each server for the session named
// like it, and shows no other: none that claude made on its server, none renamed by hand - which
// it shows as the session that has ended (see record_test.go). Without a server, or with none of
// cld's sessions on the servers, and no record of any, there is nothing to show, and it shows
// nothing, not even the header.
func TestList(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("without a server: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	// A server named like a session of cld's, holding a session of another name; a socket named
	// like no session of cld's can be; and the server cld 0.3.0 and earlier shared (see
	// TestLeavesTheSharedServerAlone).
	others := sandbox.New(t)
	others.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	others.MustTmux("cld-x.y", "-f", "/dev/null", "new-session", "-d", "-s", "cld-x.y", "sleep", "600")
	others.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	if result := others.RunCld(nil, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("with none of cld's sessions: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}

	elsewhere, moved := filepath.Join(s.Root, "elsewhere"), filepath.Join(s.Work, "café")
	for _, dir := range []string{elsewhere, moved} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	b := s.WaitProbes(1)[0]
	detached := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "long_name", "-s", "1")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	detached.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detached.Running() })
	// A session that claude makes on its server is not cld's to show, even one listed before cld's:
	// tmux names a session it is given no name for with a number, as a bare tmux in claude's pane
	// would.
	s.MustTmux("cld-b", "new-session", "-d", "sleep", "60")
	b.Send("cd " + moved)
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		return s.Format("cld-b", "#{pane_current_path}") == moved
	})

	want := "NAME         STATE     LAST ACTIVE  DIRECTORY\n" +
		"b            attached  now          " + moved + "\n" +
		"long_name-1  detached  now          " + elsewhere + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	// tmux writes to a client whose locale is not UTF-8 with "_" for what it cannot print: the
	// tabs, and the "é".
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	if result := s.RunCld(notUTF8, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("LANG=C: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	// A session renamed by hand is no longer the one its server is named after, nor on the server
	// its new name would have: it has ended, as cld's record has it, in the directory it started
	// in.
	s.MustTmux("cld-long_name-1", "rename-session", "-t", "=cld-long_name-1", "cld-renamed")
	want = "NAME         STATE     LAST ACTIVE  DIRECTORY\n" + "b            attached  now          " + moved + "\n" + "long_name-1  ended     -            " + elsewhere + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("renamed: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
}

// list shows claude's status after each session's state, as the title's hooks keep it on the
// session (see TestStatusHooks): idle once a turn is done, busy, waiting while claude asks, and
// nothing where no hook has set it, where the option holds anything else - here a tab, which
// would split the line's fields if tmux wrote it - or once claude has exited - here in a turn,
// which leaves the option busy. A pane split off in claude's window shows the status claude left,
// as the active pane: the one selected beside claude's once claude has failed - f, waiting, not
// exited - and the one that keeps the session once claude's /exit has closed its pane - g, idle.
// The rows keep the order of the names, the waiting sessions among them, and STATE is as wide as
// the longest, in the table and in the interactive list, which draws waiting in bold - on the
// selected row too, in inverse video - and nothing else. join -s and detach -s describe each
// session so too.
func TestListShowsClaudesStatus(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b", "c", "d", "e", "f", "g")
	probes["a"].Hook("Stop", `{}`)
	probes["b"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	probes["c"].Hook("PermissionRequest", `{"tool_name":"Bash"}`)
	s.MustTmux("cld-d", "set", "-t", "=cld-d:", "@cld-status", "bu\tsy")
	probes["e"].Hook("UserPromptSubmit", `{"prompt":"go"}`)
	probes["e"].Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude e to exit", func() bool { return s.Format("cld-e", "#{pane_dead}") == "1" })
	if status := s.Format("cld-e", "#{@cld-status}"); status != "busy" {
		t.Errorf("@cld-status of the session whose claude exited is %q, want busy", status)
	}
	probes["f"].Hook("PermissionRequest", `{"tool_name":"Bash"}`)
	s.MustTmux("cld-f", "split-window", "-d", "-t", "=cld-f:", "-c", s.Work, "sleep", "600")
	s.MustTmux("cld-f", "select-pane", "-t", "=cld-f:.1")
	probes["f"].Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude f to exit", func() bool { return s.Format("cld-f", "#{pane_dead}") == "1\n0" })
	probes["g"].Hook("Stop", `{}`)
	s.MustTmux("cld-g", "split-window", "-d", "-t", "=cld-g:", "-c", s.Work, "sleep", "600")
	probes["g"].Send("exit 0")
	sandbox.WaitFor(t, 10*time.Second, "claude g's pane to close", func() bool { return s.Format("cld-g", "#{pane_dead}") == "0" })
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitClients(t, s, 1)

	states := [][2]string{{"a", "detached, idle"}, {"b", "attached, busy"}, {"c", "detached, waiting"}, {"d", "detached"}, {"e", "exited"}, {"f", "detached, waiting"}, {"g", "detached, idle"}}
	table := "NAME  STATE              LAST ACTIVE  DIRECTORY\n"
	for _, row := range states {
		table += fmt.Sprintf("%-4s  %-17s  now          %s\n", row[0], row[1], s.Work)
	}
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != table || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, table)
	}
	completions := "a\tdetached, idle\nb\tattached, busy\nc\tdetached, waiting\nd\tdetached\ne\texited\nf\tdetached, waiting\ng\tdetached, idle\n:4\n"
	for _, command := range []string{"join", "detach"} {
		if result := s.RunCld(nil, "__complete", command, "-s", ""); result.Code != 0 || result.Stdout != completions {
			t.Errorf("__complete %s -s: exit %d, stdout %q, want %q", command, result.Code, result.Stdout, completions)
		}
	}

	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	// shown is the list with the session selected, in a terminal columns wide, as text and styled,
	// as cells(term.Styled()) has it: waiting in bold as far as the line holds it.
	shown := func(selected string, columns int) (text, styled []string) {
		header := strings.TrimRight(cutTo("  NAME  STATE              LAST ACTIVE  DIRECTORY", columns), " ")
		text, styled = []string{header}, []string{"[]" + header}
		for _, row := range states {
			marker, attributes := " ", ""
			if row[0] == selected {
				marker, attributes = ">", "inverse=7"
			}
			full := fmt.Sprintf("%s %-4s  %-17s  now          %s", marker, row[0], row[1], s.Work)
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
		footer := footerIn(listHints, columns)
		return append(text, "", footer), append(styled, "", "[intensity=2]"+footer)
	}
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
		text, styled := shown(step.selected, step.columns)
		waitLines(t, term, text...)
		if got := cells(term.Styled()); !slices.Equal(got, styled) {
			t.Errorf("session %s selected, %d columns, the list is\n%s\nwant\n%s", step.selected, step.columns, strings.Join(got, "\n"), strings.Join(styled, "\n"))
		}
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, table)
}

// list first ends each session idle for longer than CLD_IDLE_DAYS days - no terminal attached, and
// neither a key typed into one nor an attach since - as kill ends it, claude and the server with
// it, and says so on stderr; the table shows it as ended, from its entry in cld's record, as after
// a kill. CLD_IDLE_DAYS takes a fraction of a day: 0.0001 is 8.64 s. A session with a terminal
// attached is never idle, and a key typed into a terminal on one makes it active again: session c,
// whose terminal attached before the limit and is detached by tmux's detach-client, which moves
// neither time, stays for the key alone. The table shows each that runs as active now. With
// CLD_IDLE_DAYS 0 list ends none, and neither completion nor join with -s ends one; a value that
// is no number of days is refused before anything runs, by list and by join without -s.
func TestEndsIdleSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "c")
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(3)
	joined := startCld(t, s, "tmux", nil, "join", "-s", "c")
	waitClients(t, s, 2)
	idle := map[string]string{"CLD_IDLE_DAYS": "0.0001"}
	time.Sleep(10 * time.Second)

	for _, value := range []string{"x", "-1", "1e3", "30d", " 30", "0x1p3", "inf"} {
		want := "cld: CLD_IDLE_DAYS is not a number of days: '" + value + "'\n"
		for _, command := range []string{"list", "join"} {
			if result := s.RunCld(map[string]string{"CLD_IDLE_DAYS": value}, command); result.Code != 1 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("%s, CLD_IDLE_DAYS=%q: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", command, value, result.Code, result.Stdout, result.Stderr, want)
			}
		}
	}
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     detached  now          " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n" +
		"c     attached  now          " + s.Work + "\n"
	if result := s.RunCld(map[string]string{"CLD_IDLE_DAYS": "0"}, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("CLD_IDLE_DAYS=0: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	completions := "a\tdetached\nb\tattached\nc\tattached\n:4\n"
	if result := s.RunCld(idle, "__complete", "join", "-s", ""); result.Code != 0 || result.Stdout != completions {
		t.Errorf("__complete join -s: exit %d, stdout %q, want %q", result.Code, result.Stdout, completions)
	}
	startCld(t, s, "tmux", idle, "join", "-s", "d")
	s.WaitProbes(4)
	waitClients(t, s, 3)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c", "cld-d"}) {
		t.Fatalf("sessions %q, want [cld-a cld-b cld-c cld-d]", sessions)
	}

	mark := probes["c"].Mark()
	joined.Keys("z")
	probes["c"].WaitInput(mark, "z")
	s.MustTmux("cld-c", "detach-client", "-s", "=cld-c")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !joined.Running() })
	want = "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     ended     -            " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n" +
		"c     detached  now          " + s.Work + "\n" +
		"d     attached  now          " + s.Work + "\n"
	result := s.RunCld(idle, "list")
	ended := regexp.MustCompile(`^cld: ended session 'a', idle for ([0-9]+) seconds\n$`).FindStringSubmatch(result.Stderr)
	if result.Code != 0 || result.Stdout != want || ended == nil {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stderr \"cld: ended session 'a', idle for N seconds\", stdout\n%s", result.Code, result.Stderr, result.Stdout, want)
	} else if seconds, _ := strconv.Atoi(ended[1]); seconds < 9 {
		t.Errorf("session a idle for %d seconds, want 9 or more", seconds)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
	if _, err := s.Tmux("cld-a", "list-sessions"); err == nil {
		t.Error("session a's server survived")
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b", "cld-c", "cld-d"}) {
		t.Errorf("sessions %q, want [cld-b cld-c cld-d]", sessions)
	}
	// The sweep ends a as kill would: restore leaves it ended, and brings back the others.
	checkMarks(t, s, "after the sweep", map[string]bool{"a": false, "b": true, "c": true, "d": true})
}

// join without -s ends the idle sessions as list does, and says so on stderr, once it has taken
// the index: the session it makes does not take the name of one it has just ended, whose
// conversation cld join finds by that name. Here, where the directory's name leaves nothing, the
// session is the index alone: 0, which ends, and join makes session 1, with a claude of its own,
// as cld join --resume SESSION would. join runs with the TMUX of session x's claude, as in a pane
// of x's server, and keeps x, as idle as 0: ending it would end the claude that ran cld.
func TestJoinEndsIdleSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0", "x")
	time.Sleep(10 * time.Second)
	term := startCld(t, s, "tmux", map[string]string{"CLD_IDLE_DAYS": "0.0001", "TMUX": probes["x"].Env["TMUX"]}, "join")
	sandbox.WaitFor(t, 10*time.Second, "claude 0 to exit", func() bool { return !probes["0"].Alive() })
	var fresh *sandbox.Probe
	for _, probe := range s.WaitProbes(3) {
		if probe.PID != probes["0"].PID && probe.PID != probes["x"].PID {
			fresh = probe
		}
	}
	if !slices.Equal(fresh.Argv[:2], []string{"--name", "cld-1"}) {
		t.Errorf("join started claude with %q, want --name cld-1", fresh.Argv)
	}
	waitClients(t, s, 1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-1", "cld-x"}) || !probes["x"].Alive() {
		t.Errorf("sessions %q, claude x alive: %v; want [cld-1 cld-x], x alive", sessions, probes["x"].Alive())
	}
	if note := regexp.MustCompile(`cld: ended session '0', idle for [0-9]+ seconds\r?\n`); !note.Match(term.Output()) {
		t.Errorf("the terminal got no note of the session ended: %q", term.Output())
	}
}

// The kill of an idle session checks again, in the same tmux command, that the session is idle: a
// terminal that attaches between list's read and the kill keeps the session, and list says
// nothing of it. Its table shows the session as it read it.
func TestKeepsAnIdleSessionJoinedMeanwhile(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := detachedSessions(t, s, "a")["a"]
	time.Sleep(10 * time.Second)
	kill := holdTmux(t, s, "the kill of idle session a", "*' if -F -t =cld-a: '*")
	kill.start(t)
	env := maps.Clone(kill.env)
	env["CLD_IDLE_DAYS"] = "0.0001"
	list := exec.Command(sandbox.Cld, "list")
	list.Env, list.Dir = s.Environ(env), s.Work
	var stdout, stderr bytes.Buffer
	list.Stdout, list.Stderr = &stdout, &stderr
	if err := list.Start(); err != nil {
		t.Fatal(err)
	}
	kill.held(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	waitClients(t, s, 1)
	kill.release(t)
	if err := list.Wait(); err != nil {
		t.Errorf("list: %v, stderr %q", err, stderr.String())
	}
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  now          " + s.Work + "\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stderr %q, stdout\n%s\nwant no stderr, stdout\n%s", stderr.String(), stdout.String(), want)
	}
	if !probe.Alive() || !term.Running() || !slices.Equal(s.Sessions(), []string{"cld-a"}) {
		t.Errorf("claude alive: %v, terminal attached: %v, sessions %q; want session a as it was", probe.Alive(), term.Running(), s.Sessions())
	}
}

// kill ends the session it names, the claude in it and its server; the terminal attached to it
// is left clean, and its cld exits with status 0, as when claude exits: the session goes before
// the server, which would tell the terminal that the server exited, with status 1. The other
// sessions, on their own servers, carry on.
func TestKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// The terminal runs cld a in a shell that writes down its exit status.
	status := filepath.Join(s.Root, "a.status")
	a := terminal.New(t, "tmux", s)
	a.Start(append([]string{"sh", "-c", `"$@"; echo $? >"$0"`, status}, s.CldArgv("join", "-s", "a")...), s.Env, s.Work)
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)

	if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["cld-a"].Alive() })
	sandbox.WaitFor(t, 10*time.Second, "cld a to return", func() bool { return !a.Running() })
	if code, _ := os.ReadFile(status); string(code) != "0\n" {
		t.Errorf("cld a exited with status %q, want 0:\n%s", strings.TrimSpace(string(code)), strings.TrimSpace(a.Screen()))
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

// kill ends only the session it names: without its server there is none, and session review,
// on a server of its own, is not session rev, although its name starts with rev. A session like
// that on the named session's own server is TestSeesOnlyItsOwnSessions' to check.
// detach, too, acts only on the session it names.
func TestKillRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev' (see cld list)\n"
	for _, command := range []string{"kill", "detach"} {
		if result := s.RunCld(nil, command, "-s", "rev"); result.Code != 1 || result.Stderr != want {
			t.Errorf("%s without a server: exit %d, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stderr, want)
		}
	}

	startCld(t, s, "tmux", nil, "join", "-s", "review")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for _, command := range []string{"kill", "detach"} {
		if result := s.RunCld(nil, command, "-s", "rev"); result.Code != 1 || result.Stderr != want {
			t.Errorf("%s beside cld-review: exit %d, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stderr, want)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-review"}) || !probe.Alive() {
		t.Errorf("sessions %q, claude alive: %v; want cld-review running", sessions, probe.Alive())
	}
}

// Repositories of one name share NAME and its indexes: work/api and scratch/api here, and the
// directory api outside a repository, whose path holds a tab. join records where it made a
// session, the repository's directory - by a symbolic link to work here, as PWD names it - or
// outside one the directory. Without -n, join and kill refuse a session of NAME made elsewhere,
// naming where, before join looks at what it would lose there; list shows each session's
// directory whatever its home. A subdirectory and a linked worktree of the repository, where git
// names it by its real path, take its sessions, and join -s offers only the sessions join takes,
// the refusals and completion in a locale without UTF-8 too. With -n, in the root directory,
// where -s names a session whole, and for a session that records no home, as an older cld's, the
// session is taken from anywhere. detach takes a session, and its -s offers one, as join does.
func TestSessionOfAnotherRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	work, other, plain := filepath.Join(s.Root, "work", "api"), filepath.Join(s.Root, "scratch", "api"), filepath.Join(s.Root, "pla\tin", "api")
	for _, dir := range []string{work, other} {
		runGit(t, s, s.Root, "init", "-q", dir)
	}
	runGit(t, s, work, "commit", "-q", "--allow-empty", "-m", "first")
	runGit(t, s, work, "worktree", "add", "-q", "-b", "worktree-x", filepath.Join(".claude", "worktrees", "x"))
	worktree, sub := filepath.Join(work, ".claude", "worktrees", "x"), filepath.Join(work, "sub")
	for _, dir := range []string{plain, sub} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	linked := filepath.Join(s.Root, "link", "api")
	if err := os.Symlink(filepath.Join(s.Root, "work"), filepath.Dir(linked)); err != nil {
		t.Fatal(err)
	}
	startCldIn(t, s, "tmux", linked, map[string]string{"PWD": linked}, "join")
	s.WaitProbes(1)
	startCldIn(t, s, "tmux", other, nil, "join")
	s.WaitProbes(2)
	// An older cld's server: no @cld-home, and no @cld, but its prefix, C-q, marks it as cld's (see
	// TestLeavesAForeignServerAlone).
	s.MustTmux("cld-api-2", "-f", "/dev/null", "set", "-g", "prefix", "C-q", ";",
		"new-session", "-d", "-s", "cld-api-2", "-c", s.Work, "sleep", "600")
	startCldIn(t, s, "tmux", plain, nil, "join")
	claudes := s.WaitProbes(3)
	waitClients(t, s, 3)
	for session, home := range map[string]string{"cld-api-0": linked, "cld-api-1": other, "cld-api-3": plain} {
		if made := s.MustTmux(session, "show", "-v", "-t", "="+session+":", "@cld-home"); made != home {
			t.Errorf("%s's @cld-home is %q, want %q", session, made, home)
		}
	}

	list := "NAME   STATE     LAST ACTIVE  DIRECTORY\n" +
		"api-0  attached  now          " + work + "\n" +
		"api-1  attached  now          " + other + "\n" +
		"api-2  detached  now          " + s.Work + "\n" +
		"api-3  attached  now          " + plain + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
	}
	// tmux writes to a client whose locale is not UTF-8 with "_" for what it cannot print, the tab
	// before the home among them, which would leave the home unread and the session taken: cld's
	// -u has it write them as they are.
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	for _, env := range []map[string]string{nil, notUTF8} {
		for _, test := range []struct {
			dir  string
			args []string
			want string
		}{
			{other, []string{"join", "-s", "0"}, "cld: session 'api-0' belongs to " + linked + ", not to this repository; name it with cld join -n api -s 0\n"},
			{other, []string{"kill", "-s", "0"}, "cld: session 'api-0' belongs to " + linked + ", not to this repository; name it with cld kill -n api -s 0\n"},
			{other, []string{"detach", "-s", "0"}, "cld: session 'api-0' belongs to " + linked + ", not to this repository; name it with cld detach -n api -s 0\n"},
			{plain, []string{"kill", "-s", "0"}, "cld: session 'api-0' belongs to " + linked + ", not to this directory; name it with cld kill -n api -s 0\n"},
			{work, []string{"join", "-s", "1"}, "cld: session 'api-1' belongs to " + other + ", not to this repository; name it with cld join -n api -s 1\n"},
			{sub, []string{"kill", "-s", "3"}, "cld: session 'api-3' belongs to " + plain + ", not to this repository; name it with cld kill -n api -s 3\n"},
			{other, []string{"join", "-s", "0", "--new"}, "cld: session 'api-0' belongs to " + linked + ", not to this repository; name it with cld join -n api -s 0\n"},
			{other, []string{"join", "-s", "0", "--resume", "x"}, "cld: session 'api-0' belongs to " + linked + ", not to this repository; name it with cld join -n api -s 0\n"},
			{other, []string{"join", "-s", "2", "--new"}, "cld: session 'api-2' exists, and --new would be lost: its claude has started; attach to it with cld join -n api -s 2, or give another -s SUFFIX\n"},
		} {
			if result := s.RunCldIn(test.dir, env, test.args...); result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
				t.Errorf("%s in %s, environment %q: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", strings.Join(test.args, " "), test.dir, env, result.Code, result.Stdout, result.Stderr, test.want)
			}
		}
	}
	for _, claude := range claudes {
		if !claude.Alive() {
			t.Errorf("claude %s has exited", claude.Argv[1])
		}
	}

	for _, test := range []struct {
		dir  string
		args []string
		want string
	}{
		{work, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{sub, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{worktree, []string{"-s", ""}, "0\tattached\n2\tdetached\n:4\n"},
		{other, []string{"-s", ""}, "1\tattached\n2\tdetached\n:4\n"},
		{plain, []string{"-s", ""}, "2\tdetached\n3\tattached\n:4\n"},
		{other, []string{"-n", "api", "-s", ""}, "0\tattached\n1\tattached\n2\tdetached\n3\tattached\n:4\n"},
		{"/", []string{"-s", "api"}, "api-0\tattached\napi-1\tattached\napi-2\tdetached\napi-3\tattached\n:4\n"},
	} {
		for _, command := range []string{"join", "detach"} {
			args := append([]string{"__complete", command}, test.args...)
			for _, env := range []map[string]string{nil, notUTF8} {
				if result := s.RunCldIn(test.dir, env, args...); result.Code != 0 || result.Stdout != test.want {
					t.Errorf("%s %q in %s, environment %q: exit %d, stdout\n%s\nwant\n%s", command, test.args, test.dir, env, result.Code, result.Stdout, test.want)
				}
			}
		}
	}

	startCldIn(t, s, "tmux", worktree, nil, "join", "-s", "0")
	startCldIn(t, s, "tmux", other, nil, "join", "-n", "api", "-s", "3")
	sandbox.WaitFor(t, 10*time.Second, "the joins to attach", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-api-0", "cld-api-0", "cld-api-1", "cld-api-3", "cld-api-3"})
	})
	for _, test := range []struct {
		dir  string
		args []string
	}{
		{worktree, []string{"detach", "-s", "0"}},
		{other, []string{"detach", "-n", "api", "-s", "3"}},
		{other, []string{"kill", "-s", "2"}},
		{sub, []string{"kill", "-s", "0"}},
		{other, []string{"kill", "-n", "api", "-s", "3"}},
		{"/", []string{"kill", "-s", "api-1"}},
	} {
		if result := s.RunCldIn(test.dir, nil, test.args...); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
			t.Errorf("%s in %s: exit %d, stdout %q, stderr %q, want exit 0 and no output", strings.Join(test.args, " "), test.dir, result.Code, result.Stdout, result.Stderr)
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "the sessions to end", func() bool { return len(s.Sessions()) == 0 })
}

// cld sees only the sessions it started, each cld-NAME on its server cld-NAME. A bare tmux that
// claude runs reaches claude's own server through TMUX, and a session made that way has another
// name there, even named like a session of cld's: list leaves it out, detach and kill act as for
// no session, and join makes a session of that name on a server of its own, --resume too. Beside
// one whose name starts with claude's session's, as cld-a-x does with cld-a, cld still finds
// cld-a, by its whole name: join refuses --resume there, as it would be lost, attaches to it,
// beside the terminal there, and kill ends it.
// kill ends the others with the server. What cld sets for a failed claude stays on claude's
// window: a session claude makes whose program fails closes, as tmux would close it.
func TestSeesOnlyItsOwnSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for _, session := range []string{"cld-inside", "cld-a-x"} {
		probe.Send("tmux new-session -d -s " + session + " sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make "+session, func() bool {
			return slices.Contains(s.Sessions(), "cld-a/"+session)
		})
	}
	// As claude's tmux would make it, but made once this returns.
	s.MustTmux("cld-a", "new-session", "-d", "-s", "cld-failing", "false")
	sandbox.WaitFor(t, 10*time.Second, "the failing session to close", func() bool {
		return !slices.Contains(s.Sessions(), "cld-a/cld-failing")
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-a/cld-a-x", "cld-a/cld-inside"}) {
		t.Fatalf("sessions %q, want [cld-a cld-a/cld-a-x cld-a/cld-inside]", sessions)
	}

	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     attached  now          " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	lost := "cld: session 'a' exists, and --resume would be lost: its claude has started; attach to it with cld join -s a, or give another -s SUFFIX\n"
	if result := s.RunCld(nil, "join", "-s", "a", "--resume", "cld-a"); result.Code != 1 || result.Stdout != "" || result.Stderr != lost {
		t.Errorf("join -s a --resume cld-a: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, lost)
	}
	second := startCld(t, s, "tmux", nil, "join", "-s", "a")
	waitClients(t, s, 2)
	waitScreen(t, second, "probe --name cld-a")
	for _, test := range []struct{ command, want string }{
		{"kill", "cld: no session 'inside' (see cld list)\n"},
		{"detach", "cld: no session 'inside' (see cld list)\n"},
	} {
		if result := s.RunCld(nil, test.command, "-s", "inside"); result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n inside: exit %d, stderr %q, want exit 1, stderr %q", test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "inside")
	if probes := s.WaitProbes(2); !slices.ContainsFunc(probes, func(p *sandbox.Probe) bool { return p.Argv[1] == "cld-inside" }) {
		t.Errorf("join -s inside started no claude named cld-inside")
	}
	startCld(t, s, "tmux", nil, "join", "-s", "a-x", "--resume", "cld-a-x")
	resumed := []string{"--name", "cld-a-x", "--settings", sessionSettings(s, "cld-a-x", s.Work), "--resume", "cld-a-x"}
	if probes := s.WaitProbes(3); !slices.ContainsFunc(probes, func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }) {
		t.Errorf("join -s a-x --resume cld-a-x started no claude with %q", resumed)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-a-x", "cld-a/cld-a-x", "cld-a/cld-inside", "cld-inside"}) {
		t.Errorf("sessions %q, want [cld-a cld-a-x cld-a/cld-a-x cld-a/cld-inside cld-inside]", sessions)
	}

	if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("kill -n a: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "the clds attached to a to return", func() bool { return !first.Running() && !second.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a-x", "cld-inside"}) {
		t.Errorf("sessions %q after kill -n a, want [cld-a-x cld-inside]", sessions)
	}
}

// A server outlives its session when claude exits while the tmux sessions it made keep the
// server running. list shows the session as one that has ended, from cld's record, and join
// refuses the name, pointing at kill, whatever its options: it would start claude there with the
// environment of the cld that started the server, and finds no session to attach to. kill ends
// the server, and what claude made with it, as it prints nothing, and join --new then starts a
// fresh one. A server that runs without any session - one a cld join is starting, or one exiting -
// kill leaves as it is, and refuses the name; so it does a server where session cld-NAME has been
// made since its lookup, and where it has been made since kill read the server, the kill's own
// tmux command ends nothing. detach refuses the name as join does, finding no session to detach
// from.
func TestLingeringServer(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
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
	lingering := s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}")

	const refused = "cld: session 'a' has ended, but its tmux server still runs (see tmux -L cld-a ls); end it with cld kill -s a\n"
	for _, test := range []struct {
		args         []string
		code         int
		stdout, want string
	}{
		{[]string{"list"}, 0, "NAME  STATE     LAST ACTIVE  DIRECTORY\na     ended     -            " + s.Work + "\n", ""},
		{[]string{"join", "-s", "a"}, 1, "", refused},
		{[]string{"detach", "-s", "a"}, 1, "", refused},
		{[]string{"join", "-s", "a", "--new"}, 1, "", refused},
		{[]string{"join", "-s", "a", "--resume", "SESSION"}, 1, "", refused},
		{[]string{"join", "-s", "a", "-w"}, 1, "", refused},
		{[]string{"kill", "-s", "a"}, 0, "", ""},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != test.code || result.Stdout != test.stdout || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.code, test.stdout, test.want)
		}
	}
	if sessions := s.Sessions(); len(sessions) != 0 || len(s.Probes()) != 1 {
		t.Errorf("sessions %q, %d claude processes; want none and the first one", sessions, len(s.Probes()))
	}

	startCld(t, s, "tmux", nil, "join", "-s", "a", "--new")
	s.WaitProbes(2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
	if server := s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}"); server == lingering {
		t.Errorf("join reached the server %s that outlived a, want a fresh one", server)
	}

	// Marked, as the server a cld join is starting is (see TestLeavesAForeignServerAlone).
	s.MustTmux("cld-b", "start-server", ";", "set", "-s", "exit-empty", "off", ";", "set", "-s", "@cld", "1")
	const empty = "cld: session 'b' has ended, but its tmux server still runs (see tmux -L cld-b ls)\n"
	for _, command := range []string{"kill", "join", "detach"} {
		if result := s.RunCld(nil, command, "-s", "b"); result.Code != 1 || result.Stdout != "" || result.Stderr != empty {
			t.Errorf("%s -s b: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", command, result.Code, result.Stdout, result.Stderr, empty)
		}
	}
	if _, err := s.Tmux("cld-b", "list-sessions"); err != nil {
		t.Errorf("the server without a session: %v, want it running", err)
	}

	// Session cld-NAME made on a server that looks like one that outlived it - a fresh server of
	// a cld join's, say - after the lookup, by the time kill reads the server (display-message), or
	// after that read, by the time of the kill (kill-server). A tmux first on the PATH makes it.
	for _, test := range []struct {
		name, at string
		code     int
		want     string
	}{
		{"c", "display-message", 1, "cld: session 'c' has ended, but its tmux server still runs (see tmux -L cld-c ls)\n"},
		{"d", "kill-server", 0, ""},
	} {
		s.MustTmux("cld-"+test.name, "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", "side", "sleep", "600")
		env := wrapTmux(t, s, "case \"$*\" in *"+test.at+"*)\n"+
			"\ttmux -L cld-"+test.name+" new-session -d -s cld-"+test.name+" sleep 600 ;;\n"+
			"esac\n")
		if result := s.RunCld(env, "kill", "-s", test.name); result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("kill -s %s, the session made before %s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				test.name, test.at, result.Code, result.Stdout, result.Stderr, test.code, test.want)
		}
		if sessions := s.MustTmux("cld-"+test.name, "list-sessions", "-F", "#{session_name}"); sessions != "cld-"+test.name+"\nside" {
			t.Errorf("sessions on server cld-%s %q, want cld-%[1]s and side", test.name, sessions)
		}
	}
}

// Where tmux's socket directory ignores case, as macOS's does by default, names that differ only
// in case share one socket: tmux -L cld-A reaches the server of session a, which has no session
// cld-A. join, whatever its options, and kill refuse A and name session a, rather than take its
// server for one that outlived session A and end it, or point at a kill that would - also once a's
// claude has exited and a session it made keeps the server running. list shows a once. Where the
// sandbox's socket directory ignores case, as on macOS, the name cld-A finds a's socket already
// and the test runs against the real thing; elsewhere a symlink cld-A to a's socket plays such a
// directory, as a casefold tmpfs does on Linux (see docs/design/findings/tmux-sessions.md). detach
// refuses A as join does.
func TestNamesDifferingInCase(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	socket := filepath.Join(s.SocketDir(), "cld-A")
	if _, err := os.Lstat(socket); errors.Is(err, os.ErrNotExist) {
		if err := os.Symlink("cld-a", socket); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	const clash = "cld: session name 'A' clashes with session 'a': tmux's socket directory ignores case here, so both names reach server cld-a (see tmux -L cld-a ls)\n"
	refused := func(when string) {
		t.Helper()
		for _, args := range [][]string{{"join", "-s", "A"}, {"join", "-s", "A", "--new"}, {"join", "-s", "A", "--resume", "x"},
			{"detach", "-s", "A"}, {"kill", "-s", "A"}} {
			if result := s.RunCld(nil, args...); result.Code != 1 || result.Stdout != "" || result.Stderr != clash {
				t.Errorf("%s: %s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", when, strings.Join(args, " "), result.Code, result.Stdout, result.Stderr, clash)
			}
		}
	}
	refused("a running")
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     attached  now          " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	if !probe.Alive() || len(s.Probes()) != 1 {
		t.Fatalf("claude a alive: %v, %d claude processes; want it alive and alone", probe.Alive(), len(s.Probes()))
	}

	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	refused("a lingering")
	if sessions := s.MustTmux("cld-a", "list-sessions", "-F", "#{session_name}"); sessions != "side" {
		t.Errorf("sessions on a's server %q, want side", sessions)
	}
}

// A server that dies - SIGKILL - leaves its socket behind, as tmux leaves every socket: list
// passes over it, showing its session as one that has ended, from cld's record, beside the other
// sessions; detach and kill find no session, and point at join; and join brings the session back,
// by its name where its entry has no ID yet, starting a fresh server on the socket.
func TestStaleSocket(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	pid, err := strconv.Atoi(s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld a to lose its server", func() bool { return !a.Running() })
	if _, err := os.Stat(filepath.Join(s.SocketDir(), "cld-a")); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}

	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     ended     -            " + s.Work + "\n" + "b     attached  now          " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	for _, test := range []struct{ command, want string }{
		{"kill", "cld: session 'a' has ended; resume it with cld join -s a\n"},
		{"detach", "cld: session 'a' has ended; resume it with cld join -s a\n"},
	} {
		if result := s.RunCld(nil, test.command, "-s", "a"); result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n a: exit %d, stderr %q, want exit 1, stderr %q", test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	resumed := []string{"--name", "cld-a", "--settings", sessionSettings(s, "cld-a", s.Work), "--resume", "cld-a"}
	if probes := s.WaitProbes(3); !slices.ContainsFunc(probes, func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }) {
		t.Errorf("join -s a started no claude with %q", resumed)
	}
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// A stale socket costs no tmux, however many tmux-UID keeps: cld connects to a socket before it
// runs tmux there, and passes over one that refuses the connection, and a name with no socket, as
// tmux would say no server is running. list asks only the servers that take the connection -
// more of them here than it asks at once - and shows their sessions in the order of their names;
// join, detach and kill of a stale socket's name run no tmux but tmux -V - join, which would create
// the session there, no lookup of its own, and has no terminal here; join without -s looks
// from the highest index down, only until a server runs, here one that outlives its session, and
// then reads the servers that run, and no stale socket, for its sweep of the idle sessions. The
// sockets stay. A tmux first on the PATH writes down what it runs.
func TestStaleSocketsRunNoTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	logged, asked := loggedTmux(t, s)
	var running, stale []string
	// Each on a server marked as cld marks its own (see TestLeavesAForeignServerAlone).
	for i := range 11 {
		name := "cld-work-" + strconv.Itoa(i)
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", name, "-c", s.Work, "sleep", "600")
		running = append(running, name)
	}
	s.MustTmux("cld-work-11", "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", "other", "sleep", "600")
	servers := append(slices.Clone(running), "cld-work-11")
	for i := 12; i < 30; i++ {
		stale = append(stale, "cld-work-"+strconv.Itoa(i))
	}
	stale = append(stale, "cld-work-99")
	for _, name := range stale {
		staleSocket(t, s, name)
	}

	slices.Sort(running)
	want := "NAME     STATE     LAST ACTIVE  DIRECTORY\n"
	for _, name := range running {
		want += fmt.Sprintf("%-9sdetached  now          %s\n", strings.TrimPrefix(name, "cld-"), s.Work)
	}
	if result := s.RunCld(logged, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	if got := asked(); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(servers))) {
		t.Errorf("list asked %q, want %q", got, servers)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-s", "work-20"}, "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "work-99"}, "cld: no session 'work-99' (see cld list)\n"},
		{[]string{"detach", "-s", "work-25"}, "cld: no session 'work-25' (see cld list)\n"},
		{[]string{"join", "-s", "work-50"}, "cld: join needs a terminal, and its input is not one\n"},
	} {
		if result := s.RunCld(logged, test.args...); result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.want)
		}
		if got := asked(); len(got) != 0 {
			t.Errorf("%s asked %q, want none", strings.Join(test.args, " "), got)
		}
	}

	startCld(t, s, "tmux", logged, "join", "-n", "work")
	if name := s.WaitProbes(1)[0].Argv[1]; name != "cld-work-12" {
		t.Errorf("claude named %s, want cld-work-12", name)
	}
	// The index's lookup comes first, then the sweep's read of the sessions, at once.
	if got := asked(); len(got) == 0 || got[0] != "cld-work-11" || !slices.Equal(slices.Sorted(slices.Values(got[1:])), slices.Sorted(slices.Values(servers))) {
		t.Errorf("join asked %q, want cld-work-11, then %q", got, servers)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(s.SocketDir(), name)); err != nil {
			t.Errorf("stale socket %s: %v", name, err)
		}
	}
}

// list asks the servers at once, eight at a time: a tmux first on the PATH holds every
// list-sessions until the test lets them go, and eight of the 12 begin, and no more while they are
// held; the rest begin once they are let go, and list shows the sessions in the order of their
// names, whichever server answered first.
func TestListAsksServersAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	var names []string
	// Each on a server marked as cld marks its own (see TestLeavesAForeignServerAlone).
	for i := range 12 {
		name := "cld-a" + strconv.Itoa(i)
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", name, "-c", s.Work, "sleep", "600")
		names = append(names, strings.TrimPrefix(name, "cld-"))
	}
	asks := holdTmux(t, s, "list's asks", "*list-sessions*")
	asks.start(t)
	var stdout, stderr bytes.Buffer
	argv := s.CldArgv("list")
	list := exec.Command(argv[0], argv[1:]...)
	list.Env, list.Dir, list.Stdout, list.Stderr = s.Environ(asks.env), s.Work, &stdout, &stderr
	if err := list.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = list.Process.Kill()
		_ = list.Wait()
	})
	sandbox.WaitFor(t, 10*time.Second, "eight asks to begin", func() bool { return asks.begun(t) >= 8 })
	// The rest would begin within this, were they not held back.
	time.Sleep(time.Second)
	if begun := asks.begun(t); begun != 8 {
		t.Errorf("%d asks began at once, want 8", begun)
	}
	asks.release(t)
	err := list.Wait()
	slices.Sort(names)
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n"
	for _, name := range names {
		want += fmt.Sprintf("%-6sdetached  now          %s\n", name, s.Work)
	}
	if err != nil || stdout.String() != want || stderr.String() != "" {
		t.Errorf("list: %v, stderr %q, stdout\n%s\nwant\n%s", err, stderr.String(), stdout.String(), want)
	}
	if begun := asks.begun(t); begun != 12 {
		t.Errorf("%d asks began, want 12", begun)
	}
}

// cld 0.3.0 and earlier ran every session on one server, -L cld. cld leaves it alone: list does
// not show its sessions, detach and kill find none there, and join makes a session of that name on
// a server of its own - here without a terminal first, which it refuses.
func TestLeavesTheSharedServerAlone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"list"}, 0, ""},
		{[]string{"join", "-s", "old"}, 1, "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "old"}, 1, "cld: no session 'old' (see cld list)\n"},
		{[]string{"detach", "-s", "old"}, 1, "cld: no session 'old' (see cld list)\n"},
	} {
		if result := s.RunCld(nil, test.args...); result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.code, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "old")
	s.WaitProbes(1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-old"}) {
		t.Errorf("sessions on cld's servers %q, want [cld-old]", sessions)
	}
	if old := s.MustTmux("cld", "list-sessions", "-F", "#{session_name}"); old != "cld-old" {
		t.Errorf("sessions on the shared server %q, want the one there", old)
	}
}

// A tmux server named like one of cld's that cld did not start - the user's own tmux -L cld-x,
// say - is none of cld's, whatever its sessions are called: cld marks the servers it starts with
// @cld. list shows nothing there, and join, whatever its options, and kill refuse the name,
// pointing at no kill, which would end the sessions there; they stay, and kill ends none of them,
// not even on a server that loses the mark only after kill read it. A server that cld 0.8.2 and
// earlier started has no mark, but its prefix is C-q: it is cld's, list shows its session, join
// finds it, refusing --new there, and kill ends it. detach -s refuses the name as join does, and
// finds the old session; without -s, in a pane of such a server, it detaches no terminal there,
// and refuses as it is none of cld's.
func TestLeavesAForeignServerAlone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	s.MustTmux("cld-y", "-f", "/dev/null", "new-session", "-d", "-s", "cld-y", "sleep", "600")
	s.MustTmux("cld-old", "-f", "/dev/null", "set", "-g", "prefix", "C-q", ";",
		"new-session", "-d", "-s", "cld-old", "-c", s.Work, "sleep", "600")
	type run struct {
		args           []string
		code           int
		stdout, stderr string
	}
	runs := []run{{[]string{"list"}, 0, "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "old   detached  now          " + s.Work + "\n", ""}}
	for _, name := range []string{"x", "y"} {
		for _, args := range [][]string{{"join"}, {"join", "--new"}, {"join", "--resume", "x"}, {"detach"}, {"kill"}} {
			runs = append(runs, run{append(args, "-s", name), 1, "", "cld: tmux server cld-" + name + " is not one of cld's; use another name\n"})
		}
	}
	runs = append(runs,
		run{[]string{"join", "-s", "old", "--new"}, 1, "",
			"cld: session 'old' exists, and --new would be lost: its claude has started; attach to it with cld join -s old, or give another -s SUFFIX\n"},
		run{[]string{"detach", "-s", "old"}, 0, "", ""},
		run{[]string{"kill", "-s", "old"}, 0, "", ""})
	for _, test := range runs {
		if result := s.RunCld(nil, test.args...); result.Code != test.code || result.Stdout != test.stdout || result.Stderr != test.stderr {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q", strings.Join(test.args, " "),
				result.Code, result.Stdout, result.Stderr, test.code, test.stdout, test.stderr)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x/other", "cld-y"}) || len(s.Probes()) != 0 {
		t.Errorf("sessions %q, %d claude processes; want [cld-x/other cld-y] and none", sessions, len(s.Probes()))
	}

	// detach without -s, as a program in a pane of cld-x runs it, with a terminal on other.
	term := terminal.New(t, "tmux", s)
	term.Start([]string{sandbox.RealTmux, "-L", "cld-x", "attach-session", "-t", "=other"}, s.Env, s.Work)
	sandbox.WaitFor(t, 10*time.Second, "a terminal on other", func() bool { return slices.Contains(s.Clients(), "other") })
	pane := map[string]string{"TMUX": filepath.Join(s.SocketDir(), "cld-x") + ",1,0",
		"TMUX_PANE": s.MustTmux("cld-x", "display-message", "-p", "-t", "=other:", "#{pane_id}")}
	const foreign = "cld: tmux server cld-x is not one of cld's; name the session with -s SUFFIX (see cld list)\n"
	if result := s.RunCld(pane, "detach"); result.Code != 1 || result.Stdout != "" || result.Stderr != foreign {
		t.Errorf("detach in a pane of cld-x: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, foreign)
	}
	if clients := s.MustTmux("cld-x", "list-clients", "-F", "#{session_name}"); clients != "other" || !term.Running() {
		t.Errorf("clients on cld-x %q, the terminal running: %v; want the one on other", clients, term.Running())
	}

	// A server of cld's that has outlived its session, and loses the mark after kill read it -
	// one of the user's own started on the socket since, say: the kill's own tmux command, which
	// checks the mark again (see TestLingeringServer), ends nothing. A tmux first on the PATH
	// takes the mark away.
	s.MustTmux("cld-z", "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", "side", "sleep", "600")
	env := wrapTmux(t, s, "case \"$*\" in *kill-server*)\n\ttmux -L cld-z set -su @cld ;;\nesac\n")
	if result := s.RunCld(env, "kill", "-s", "z"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("kill -s z, the mark gone before kill-server: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			result.Code, result.Stdout, result.Stderr)
	}
	if sessions := s.MustTmux("cld-z", "list-sessions", "-F", "#{session_name}"); sessions != "side" {
		t.Errorf("sessions on server cld-z %q, want side", sessions)
	}
}

// join -s completes the names of the sessions list shows - here, where the directory's name
// leaves nothing, a session's name is its SUFFIX - attached, detached, with claude exited, or
// ended, that start with what was typed, in list's order, each with its state - a session that
// has ended with the directory it ran in - and then offers no file names (":4",
// ShellCompDirectiveNoFileComp). That is cobra's __complete, which the completion scripts run on
// every TAB; it starts no server and no claude. As list does, it asks the server of each socket
// for its own session, and leaves out the sessions cld did not start - one that claude makes, on
// its own session's server under another name, one made there by hand, one on a server named like
// cld's that cld did not start, and one on the server that cld 0.3.0 and earlier shared - and
// offers a session renamed by hand, whose server then runs without it, and a stale socket's, whose
// server has died, as those that have ended, from cld's record. detach -s offers the same of the
// sessions that run. join -n and detach -n offer what comes before a "-" of a name, and none has
// one here (see TestCompleteSuffixes). --resume's SESSION offers nothing, and neither do the
// other arguments, file names included, nor claude's words after "--", cld's options included.
func TestCompleteNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "__complete", "join", "-s", ""); result.Code != 0 || result.Stdout != ":4\n" {
		t.Errorf("without a server: exit %d, stdout %q, want exit 0, stdout %q", result.Code, result.Stdout, ":4\n")
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after completing without a server, want none", sessions)
	}

	startCld(t, s, "tmux", nil, "join", "-s", "rev")
	rev := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for i, name := range []string{"review", "cafe", "bad", "gone"} {
		term := startCld(t, s, "tmux", nil, "join", "-s", name)
		s.WaitProbes(2 + i)
		waitClients(t, s, 2)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld "+name+" to detach", func() bool { return !term.Running() })
	}
	for _, probe := range s.Probes() {
		if probe.Argv[1] == "cld-bad" {
			probe.Send("exit 1")
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "claude bad to exit", func() bool { return s.Format("cld-bad", "#{pane_dead}") == "1" })
	// Renamed by hand, session cafe is no longer cld-cafe, which its server runs without.
	s.MustTmux("cld-cafe", "rename-session", "-t", "=cld-cafe", "cld-café")
	// A server that dies leaves its socket behind.
	pid, err := strconv.Atoi(s.MustTmux("cld-gone", "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "server cld-gone to die", func() bool {
		_, err := s.Tmux("cld-gone", "list-sessions")
		return err != nil
	})
	if _, err := os.Stat(filepath.Join(s.SocketDir(), "cld-gone")); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}
	rev.Send("tmux new-session -d -s cld-inside sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-rev/cld-inside")
	})
	s.MustTmux("cld-review", "new-session", "-d", "-s", "cld-by-hand", "sleep", "600")
	s.MustTmux("cld-own", "-f", "/dev/null", "new-session", "-d", "-s", "cld-own", "sleep", "600")
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	want := []string{"cld-bad", "cld-cafe/cld-café", "cld-own", "cld-rev", "cld-rev/cld-inside", "cld-review", "cld-review/cld-by-hand"}
	if sessions := s.Sessions(); !slices.Equal(sessions, want) {
		t.Fatalf("sessions %q, want %q", sessions, want)
	}
	probes := len(s.Probes())

	listed := []string{"bad", "cafe", "gone", "rev", "review"}
	cafe, gone := "cafe\tended in "+s.Work, "gone\tended in "+s.Work
	names := []string{"bad\texited", cafe, gone, "rev\tattached", "review\tdetached"}
	running := []string{"bad\texited", "rev\tattached", "review\tdetached"}
	result := s.RunCld(nil, "list")
	var shown []string
	for _, line := range strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")[1:] {
		shown = append(shown, strings.Fields(line)[0])
	}
	if result.Code != 0 || !slices.Equal(shown, listed) {
		t.Errorf("list: exit %d, names %q, want %q:\n%s", result.Code, shown, listed, result.Stdout)
	}

	// offered is what __complete prints for names, each one line, and then the directive.
	offered := func(names ...string) string {
		return strings.Join(append(names, ":4"), "\n") + "\n"
	}
	all := offered(names...)
	bare := func(names []string) []string {
		var bare []string
		for _, name := range names {
			bare = append(bare, strings.Split(name, "\t")[0])
		}
		return bare
	}
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	for _, test := range []struct {
		args []string
		env  map[string]string
		want string
	}{
		{[]string{"__complete", "join", "-s", ""}, nil, all},
		{[]string{"__complete", "join", "--suffix", ""}, nil, all},
		{[]string{"__complete", "join", "--suffix="}, nil, all},
		{[]string{"__complete", "join", "-s", "re"}, nil, offered("rev\tattached", "review\tdetached")},
		{[]string{"__complete", "join", "-s", "revi"}, nil, offered("review\tdetached")},
		{[]string{"__complete", "join", "-s", "x"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "caf"}, nil, offered(cafe)},
		{[]string{"__complete", "join", "-s", "g"}, nil, offered(gone)},
		{[]string{"__complete", "join", "-s", "in"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "b"}, nil, offered("bad\texited")},
		{[]string{"__complete", "join", "-s", "o"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "ow"}, nil, offered()},
		// pflag's -s=SUFFIX; in -sSUFFIX cobra takes the word for options, and finds none.
		{[]string{"__complete", "join", "-s=re"}, nil, offered("rev\tattached", "review\tdetached")},
		{[]string{"__complete", "join", "-sre"}, nil, offered()},
		{[]string{"__completeNoDesc", "join", "-s", ""}, nil, offered(bare(names)...)},
		{[]string{"__complete", "join", "-s", ""}, map[string]string{"CLD_COMPLETION_DESCRIPTIONS": "0"}, offered(bare(names)...)},
		{[]string{"__complete", "join", "-s", ""}, notUTF8, all},
		{[]string{"__complete", "join", "--resume", "x", "-s", "g"}, nil, offered(gone)},
		{[]string{"__complete", "detach", "-s", ""}, nil, offered(running...)},
		{[]string{"__complete", "detach", "-s", "g"}, nil, offered()},
		{[]string{"__complete", "detach", "--suffix=re"}, nil, offered("rev\tattached", "review\tdetached")},
		{[]string{"__completeNoDesc", "detach", "-s", ""}, nil, offered(bare(running)...)},
		// No name here has a NAME of NAME-SUFFIX for -n.
		{[]string{"__complete", "join", "-n", ""}, nil, offered()},
		{[]string{"__complete", "detach", "-n", ""}, nil, offered()},
		{[]string{"__complete", "join", "--resume", ""}, nil, offered()},
		{[]string{"__complete", "join", "--resume", "g"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "rev", ""}, nil, offered()},
		{[]string{"__complete", "join", "--", ""}, nil, offered()},
		{[]string{"__complete", "join", "--", "--"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "rev", "--", "-"}, nil, offered()},
		{[]string{"__complete", "join", "--", "-s", ""}, nil, offered()},
		{[]string{"__complete", "kill", "-s", ""}, nil, offered()},
		{[]string{"__complete", "join", ""}, nil, offered()},
		{[]string{"__complete", "detach", ""}, nil, offered()},
		{[]string{"__complete", "list", ""}, nil, offered()},
		// cobra answers these with ShellCompDirectiveDefault, and the shell would offer files.
		{[]string{"__complete", "joni", "-n", ""}, nil, offered()},
		{[]string{"__complete", "join", "-x", "-n", ""}, nil, offered()},
		{[]string{"__complete", "list", "-n", ""}, nil, offered()},
	} {
		if result := s.RunCld(test.env, test.args...); result.Code != 0 || result.Stdout != test.want {
			t.Errorf("cld %q, %v: exit %d, stdout\n%s\nwant\n%s", test.args, test.env, result.Code, result.Stdout, test.want)
		}
	}
	if count := len(s.Probes()); count != probes {
		t.Errorf("%d claude processes after completing, want %d", count, probes)
	}
}

// join -n offers the NAME of NAME-SUFFIX for the sessions cld list shows, those that run and those
// that have ended: what comes before the last "-" of their names, where that and what follows are
// both NAMEs, once each, described by the number of its sessions. join -s offers the SUFFIX of the
// sessions named after NAME - -n's, or else the git repository's, work here, from a subdirectory
// too - where join takes it, and not the sessions of another NAME, or of none, each described by
// its state, and a session that has ended by the directory it ran in: entries of cld's record
// without their sessions. Outside a repository NAME is the directory's name - in the root
// directory, whose name leaves nothing, every name is a SUFFIX. detach -n and -s offer the same of
// the sessions that run.
func TestCompleteSuffixes(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	work := repository(t, s, "work")
	sub := filepath.Join(work, "sub")
	other := filepath.Join(s.Root, "other")
	for _, dir := range []string{sub, other} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Each on a server marked as cld marks its own (see TestLeavesAForeignServerAlone).
	for _, name := range []string{"cld-work-0", "cld-work-fix", "cld-work--x", "cld-work", "cld-other-1", "cld-3", "cld-a-b-c"} {
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", name, "sleep", "600")
	}
	for _, name := range []string{"work-0", "work-7", "other-2", "4"} {
		writeEntry(t, s, name, other, "")
	}
	ended := "\tended in " + other + "\n"
	for _, test := range []struct {
		dir  string
		args []string
		// join and detach are what join and detach offer
		join, detach string
	}{
		{work, []string{"-s", ""}, "0\tdetached\n7" + ended + "fix\tdetached\n:4\n", "0\tdetached\nfix\tdetached\n:4\n"},
		{sub, []string{"-s", ""}, "0\tdetached\n7" + ended + "fix\tdetached\n:4\n", "0\tdetached\nfix\tdetached\n:4\n"},
		{work, []string{"-s", "f"}, "fix\tdetached\n:4\n", "fix\tdetached\n:4\n"},
		{work, []string{"-s", "7"}, "7" + ended + ":4\n", ":4\n"},
		{work, []string{"-s", "-"}, ":4\n", ":4\n"},
		{work, []string{"-s", "work"}, ":4\n", ":4\n"},
		{work, []string{"-n", "other", "-s", ""}, "1\tdetached\n2" + ended + ":4\n", "1\tdetached\n:4\n"},
		{work, []string{"--name=a", "-s", ""}, "b-c\tdetached\n:4\n", "b-c\tdetached\n:4\n"},
		{work, []string{"-n", "a-b", "--suffix", ""}, "c\tdetached\n:4\n", "c\tdetached\n:4\n"},
		{other, []string{"-s", ""}, "1\tdetached\n2" + ended + ":4\n", "1\tdetached\n:4\n"},
		{"/", []string{"-s", ""},
			"3\tdetached\n4" + ended + "a-b-c\tdetached\nother-1\tdetached\nother-2" + ended + "work\tdetached\nwork--x\tdetached\nwork-0\tdetached\nwork-7" + ended + "work-fix\tdetached\n:4\n",
			"3\tdetached\na-b-c\tdetached\nother-1\tdetached\nwork\tdetached\nwork--x\tdetached\nwork-0\tdetached\nwork-fix\tdetached\n:4\n"},
		{work, []string{"-n", ""}, "a-b\t1 session\nother\t2 sessions\nwork-\t1 session\nwork\t3 sessions\n:4\n",
			"a-b\t1 session\nother\t1 session\nwork-\t1 session\nwork\t2 sessions\n:4\n"},
		{work, []string{"-n", "w"}, "work-\t1 session\nwork\t3 sessions\n:4\n", "work-\t1 session\nwork\t2 sessions\n:4\n"},
		{work, []string{"-s", "0", "-n", ""}, "a-b\t1 session\nother\t2 sessions\nwork-\t1 session\nwork\t3 sessions\n:4\n",
			"a-b\t1 session\nother\t1 session\nwork-\t1 session\nwork\t2 sessions\n:4\n"},
	} {
		for command, want := range map[string]string{"join": test.join, "detach": test.detach} {
			args := append([]string{"__complete", command}, test.args...)
			if result := s.RunCldIn(test.dir, nil, args...); result.Code != 0 || result.Stdout != want {
				t.Errorf("%s %q in %s: exit %d, stdout\n%s\nwant\n%s", command, test.args, test.dir, result.Code, result.Stdout, want)
			}
		}
	}
}

// join attaches beside the terminal on the session, which stays attached to the same claude, and
// C-q d there detaches that terminal alone; join --detach-others detaches every other terminal.
func TestJoinBesideOtherClients(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)

	second := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	waitClients(t, s, 2)
	waitScreen(t, second, "probe --name cld-shared")
	if !first.Running() {
		t.Error("the first client was detached")
	}
	mark := probe.Mark()
	first.Keys("x")
	probe.WaitInput(mark, "x")
	third := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	waitClients(t, s, 3)
	waitScreen(t, third, "probe --name cld-shared")
	second.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "the second client to be detached", func() bool { return !second.Running() })
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-shared", "cld-shared"}) || !first.Running() || !third.Running() {
		t.Errorf("clients attached to %q after C-q d, want the first and third, to cld-shared", clients)
	}

	fourth := startCld(t, s, "tmux", nil, "join", "-s", "shared", "--detach-others")
	sandbox.WaitFor(t, 10*time.Second, "the other clients to be detached", func() bool { return !first.Running() && !third.Running() })
	waitScreen(t, fourth, "probe --name cld-shared")
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-shared"}) || !fourth.Running() {
		t.Errorf("clients attached to %q, want one, to cld-shared", clients)
	}
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() {
		t.Errorf("%d claude processes, want the original one", len(probes))
	}
}

// detach detaches terminals without C-q d, for a terminal that keeps C-q from tmux. Run as claude
// runs ! cld detach - without a terminal, with the TMUX and TMUX_PANE of claude's pane - it
// detaches the terminal on claude's session that a key was typed in last, whether it attached
// first or last, and leaves the others attached; each cld there returns with status 0. With -s,
// there or from anywhere, it detaches every terminal on the session named, and leaves the other
// sessions and their terminals alone. Where no terminal is attached, either does nothing and says
// nothing: -s where none is attached to the server at all, when tmux's detach-client would fail
// with "no current client", and the bare one also where a session claude made on the server has a
// terminal, which a bare detach-client would detach. In a shell on the server, whose terminal
// tmux knows, it goes by that without TMUX_PANE. A TMUX whose server is gone ends it with tmux's
// message and status. claude keeps running throughout.
func TestDetach(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// Each terminal runs cld in a shell that writes down its exit status.
	statuses := 0
	attach := func(args ...string) (terminal.Terminal, string) {
		t.Helper()
		statuses++
		status := filepath.Join(s.Root, strconv.Itoa(statuses)+".status")
		term := terminal.New(t, "tmux", s)
		term.Start(append([]string{"sh", "-c", `"$@"; echo $? >"$0"`, status}, s.CldArgv(args...)...), s.Env, s.Work)
		return term, status
	}
	detached := func(what string, term terminal.Terminal, status string) {
		t.Helper()
		sandbox.WaitFor(t, 10*time.Second, what+" to return", func() bool { return !term.Running() })
		if code, _ := os.ReadFile(status); string(code) != "0\n" {
			t.Errorf("%s exited with status %q, want 0:\n%s", what, strings.TrimSpace(string(code)), strings.TrimSpace(term.Screen()))
		}
	}
	quiet := func(what string, extra map[string]string, args ...string) {
		t.Helper()
		if result := s.RunCld(extra, args...); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0 and no output", what, result.Code, result.Stdout, result.Stderr)
		}
	}
	clients := func(what string, want ...string) {
		t.Helper()
		if got := s.Clients(); !slices.Equal(got, want) {
			t.Errorf("%s: clients attached to %q, want %q", what, got, want)
		}
	}

	first, firstStatus := attach("join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	second, secondStatus := attach("join", "-s", "a")
	waitClients(t, s, 2)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 3)
	pane := map[string]string{"TMUX": probe.Env["TMUX"], "TMUX_PANE": probe.Env["TMUX_PANE"]}
	if !strings.HasPrefix(pane["TMUX"], filepath.Join(s.SocketDir(), "cld-a")+",") || pane["TMUX_PANE"] == "" {
		t.Fatalf("claude a has TMUX %q and TMUX_PANE %q", pane["TMUX"], pane["TMUX_PANE"])
	}

	// A key in the first terminal, which attached before the second: the first detaches.
	mark := probe.Mark()
	first.Keys("x")
	probe.WaitInput(mark, "x")
	quiet("detach in claude's pane", pane, "detach")
	detached("the first terminal", first, firstStatus)
	clients("after a key in the first terminal", "cld-a", "cld-b")
	if !second.Running() {
		t.Error("the second terminal was detached")
	}
	// A key in the second terminal, which attached before the third: the second detaches.
	third, thirdStatus := attach("join", "-s", "a")
	waitClients(t, s, 3)
	mark = probe.Mark()
	second.Keys("y")
	probe.WaitInput(mark, "y")
	quiet("detach in claude's pane", pane, "detach")
	detached("the second terminal", second, secondStatus)
	clients("after a key in the second terminal", "cld-a", "cld-b")

	// -s detaches every terminal on the session: from claude's pane, the terminal on b.
	quiet("detach -s b in claude's pane", pane, "detach", "-s", "b")
	waitClients(t, s, 1)
	clients("after detach -s b", "cld-a")
	fourth, fourthStatus := attach("join", "-s", "a")
	waitClients(t, s, 2)
	quiet("detach -s a", nil, "detach", "-s", "a")
	detached("the third terminal", third, thirdStatus)
	detached("the fourth terminal", fourth, fourthStatus)
	clients("after detach -s a")

	// No terminal on the server, then one on a session claude made there only.
	quiet("detach -s a with no terminal", nil, "detach", "-s", "a")
	quiet("detach in claude's pane with no terminal", pane, "detach")
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	onSide := func() {
		t.Helper()
		side := terminal.New(t, "tmux", s)
		side.Start([]string{sandbox.RealTmux, "-L", "cld-a", "attach-session", "-t", "=side"}, s.Env, s.Work)
		sandbox.WaitFor(t, 10*time.Second, "a terminal on side", func() bool { return slices.Contains(s.Clients(), "side") })
	}
	onSide()
	quiet("detach in claude's pane with a terminal on side", pane, "detach")
	quiet("detach -s a with a terminal on side", nil, "detach", "-s", "a")
	clients("with a terminal on side", "side")

	// A shell on a's server, in a window of its own: tmux finds its pane by its terminal, and not
	// the session used last, side, which a terminal attaches to after one attaches to a.
	s.MustTmux("cld-a", "detach-client", "-s", "=side")
	waitClients(t, s, 0)
	fifth, fifthStatus := attach("join", "-s", "a")
	waitClients(t, s, 1)
	onSide()
	shell := filepath.Join(s.Root, "shell")
	s.MustTmux("cld-a", append([]string{"new-window", "-d", "-t", "=cld-a:", "sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, shell,
		"env", "-u", "TMUX_PANE"}, s.CldArgv("detach")...)...)
	var code []byte
	sandbox.WaitFor(t, 10*time.Second, "cld detach in a shell to return", func() bool {
		code, _ = os.ReadFile(shell + ".code")
		return strings.HasSuffix(string(code), "\n")
	})
	if stderr, _ := os.ReadFile(shell + ".err"); string(code) != "0\n" || len(stderr) != 0 {
		t.Errorf("cld detach in a shell: exit %s, stderr %q, want exit 0 and no output", strings.TrimSpace(string(code)), stderr)
	}
	detached("the fifth terminal", fifth, fifthStatus)
	clients("after detach in a shell", "side")

	gone := filepath.Join(s.SocketDir(), "cld-gone")
	want := "error connecting to " + gone + " (No such file or directory)\n"
	if result := s.RunCld(map[string]string{"TMUX": gone + ",1,0", "TMUX_PANE": "%0"}, "detach"); result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("detach with no server: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if probes := s.Probes(); len(probes) != 2 || !probe.Alive() {
		t.Errorf("%d claude processes, claude a alive: %v; want the two, alive", len(probes), probe.Alive())
	}
}

// claudeOf is the claude of session name, among the probes started so far, once it has started.
func claudeOf(t *testing.T, s *sandbox.Sandbox, name string) *sandbox.Probe {
	t.Helper()
	var found *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude of cld-"+name+" to start", func() bool {
		for _, probe := range s.Probes() {
			if len(probe.Argv) > 1 && probe.Argv[1] == "cld-"+name {
				found = probe
			}
		}
		return found != nil
	})
	return found
}

// paneOf is the TMUX and TMUX_PANE of claude's pane in session name, with which claude runs a
// command: ! cld join among them.
func paneOf(t *testing.T, s *sandbox.Sandbox, name string) map[string]string {
	t.Helper()
	probe := claudeOf(t, s, name)
	pane := map[string]string{"TMUX": probe.Env["TMUX"], "TMUX_PANE": probe.Env["TMUX_PANE"]}
	if !strings.HasPrefix(pane["TMUX"], filepath.Join(s.SocketDir(), "cld-"+name)+",") || pane["TMUX_PANE"] == "" {
		t.Fatalf("claude %s has TMUX %q and TMUX_PANE %q", name, pane["TMUX"], pane["TMUX_PANE"])
	}
	return pane
}

// lastOf is what session name records as the session a switch moved its terminal from.
func lastOf(s *sandbox.Sandbox, name string) string {
	out, _ := s.Tmux("cld-"+name, "show", "-v", "-t", "=cld-"+name+":", "@cld-last")
	return out
}

// join, run in a pane of one of cld's servers, moves the terminal on that session to the session
// it names, rather than attach a session inside the session: run as claude runs ! cld join -
// without a terminal, with its pane's TMUX and TMUX_PANE and variables of claude's own - it moves
// the terminal a key was typed in last, of two on the session, and leaves the other there, the
// session running on; and says nothing, with status 0. The terminal runs cld join in its client's
// place, with its own environment and in the directory join ran in: a session it creates has its
// claude there, with the terminal's variables, not claude's, and the words after "--"; without -s,
// under the next index. The session joined records where the terminal came from, for C-q L. What
// join refuses before it would start claude it refuses in the pane, where claude shows it, and the
// terminal stays: an option that would be lost. With no terminal on the session there is none to
// move, and join refuses. In a shell in a window of the session, join goes by that terminal.
func TestJoinMovesTheTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	first := startCld(t, s, "tmux", map[string]string{"CLD_TERMINAL": "first"}, "join", "-s", "a")
	waitClients(t, s, 1)
	second := startCld(t, s, "tmux", map[string]string{"CLD_TERMINAL": "second"}, "join", "-s", "a")
	waitClients(t, s, 2)
	waitScreen(t, second, "probe --name cld-a")
	claude := map[string]string{"CLAUDE_CODE_SESSION_ID": "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", "CLAUDE_CODE_CHILD_SESSION": "1"}
	pane := paneOf(t, s, "a")
	maps.Copy(claude, pane)
	moved := func(what string, result sandbox.Result) {
		t.Helper()
		if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0 and no output", what, result.Code, result.Stdout, result.Stderr)
		}
	}

	// A key in the first terminal: the first moves to b, and the second stays on a.
	probe := claudeOf(t, s, "a")
	mark := probe.Mark()
	first.Keys("x")
	probe.WaitInput(mark, "x")
	moved("join -s b in claude's pane", s.RunCld(claude, "join", "-s", "b"))
	waitScreen(t, first, "probe --name cld-b")
	waitClients(t, s, 2)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a", "cld-b"}) || !first.Running() || !second.Running() {
		t.Errorf("clients attached to %q, want the second on cld-a and the first on cld-b", clients)
	}
	if last := lastOf(s, "b"); last != "a" {
		t.Errorf("b records %q as the last session, want a", last)
	}

	// A key in the second, and a join that creates c, from a directory of its own, whose name
	// leaves nothing of a NAME and goes encoded to the terminal: claude c starts there, with
	// the second terminal's variables and not claude a's.
	dir := filepath.Join(s.Work, `' #;$`)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mark = probe.Mark()
	second.Keys("y")
	probe.WaitInput(mark, "y")
	moved("join -s c in claude's pane", s.RunCldIn(dir, claude, "join", "-s", "c", "--", "hello"))
	waitScreen(t, second, "probe --name cld-c")
	c := claudeOf(t, s, "c")
	if c.Cwd != dir {
		t.Errorf("claude c runs in %s, want %s", c.Cwd, dir)
	}
	if c.Env["CLD_TERMINAL"] != "second" || c.Env["CLAUDE_CODE_SESSION_ID"] != "" || c.Env["CLAUDE_CODE_CHILD_SESSION"] != "" {
		t.Errorf("claude c has CLD_TERMINAL %q, CLAUDE_CODE_SESSION_ID %q and CLAUDE_CODE_CHILD_SESSION %q, want the second terminal's, and none of claude a's",
			c.Env["CLD_TERMINAL"], c.Env["CLAUDE_CODE_SESSION_ID"], c.Env["CLAUDE_CODE_CHILD_SESSION"])
	}
	if !slices.Contains(c.Argv, "hello") {
		t.Errorf("claude c has arguments %q, want hello among them", c.Argv)
	}
	if last := lastOf(s, "c"); last != "a" {
		t.Errorf("c records %q as the last session, want a", last)
	}
	waitClients(t, s, 2)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b", "cld-c"}) {
		t.Errorf("clients attached to %q, want [cld-b cld-c]", clients)
	}

	// Refused in claude's pane, where claude shows why, the terminal staying on b.
	inB := paneOf(t, s, "b")
	lost := "cld: session 'c' exists, and --new would be lost: its claude has started; attach to it with cld join -s c, or give another -s SUFFIX\n"
	if result := s.RunCld(inB, "join", "-s", "c", "--new"); result.Code != 1 || result.Stderr != lost {
		t.Errorf("join -s c --new in b's pane: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, lost)
	}
	if !first.Running() || !slices.Contains(s.Clients(), "cld-b") {
		t.Errorf("the first terminal left b, clients attached to %q", s.Clients())
	}

	// Without -s, the terminal's cld join makes a session under the next index, 0 here.
	moved("join in b's pane", s.RunCld(inB, "join"))
	waitScreen(t, first, "probe --name cld-0")
	if last := lastOf(s, "0"); last != "b" {
		t.Errorf("0 records %q as the last session, want b", last)
	}

	// No terminal on a: none to move.
	none := "cld: no terminal is attached to this session for join to move; run cld join in a terminal (see cld help join)\n"
	if result := s.RunCld(claude, "join", "-s", "b"); result.Code != 1 || result.Stdout != "" || result.Stderr != none {
		t.Errorf("join -s b with no terminal on a: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, none)
	}

	// A shell in a window of c: join goes by its terminal, the second's.
	shell := filepath.Join(s.Root, "shell")
	s.MustTmux("cld-c", append([]string{"new-window", "-t", "=cld-c:", "-c", s.Work, "sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, shell,
		"env", "-u", "TMUX_PANE"}, s.CldArgv("join", "-s", "b")...)...)
	waitScreen(t, second, "probe --name cld-b")
	var code []byte
	sandbox.WaitFor(t, 10*time.Second, "cld join in a shell to return", func() bool {
		code, _ = os.ReadFile(shell + ".code")
		return strings.HasSuffix(string(code), "\n")
	})
	if stderr, _ := os.ReadFile(shell + ".err"); string(code) != "0\n" || len(stderr) != 0 {
		t.Errorf("cld join in a shell: exit %s, stderr %q, want exit 0 and no output", strings.TrimSpace(string(code)), stderr)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-0", "cld-b"}) {
		t.Errorf("clients attached to %q, want [cld-0 cld-b]", clients)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0", "cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want 0, a, b and c, each running", sessions)
	}
}

// Two joins at once, from claude's panes in two sessions, each with a terminal, into one session
// that does not run: both terminals move there, and the session is made once. Each terminal's cld
// join makes it or, finding the start mark of the other, waits and attaches (see
// TestJoinOneSessionAtOnce): the first is held as its tmux is about to make the session.
func TestJoinMovesToOneSessionAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	tmux := holdTmux(t, s, "the first terminal's tmux making cld-x", "*' new-session -s cld-x '*")
	first := startCld(t, s, "tmux", tmux.env, "join", "-s", "a")
	waitClients(t, s, 1)
	second := startCld(t, s, "tmux", tmux.env, "join", "-s", "b")
	waitClients(t, s, 2)
	inA, inB := paneOf(t, s, "a"), paneOf(t, s, "b")
	tmux.start(t)
	if result := s.RunCld(inA, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join -s x in a's pane: exit %d, stderr %q", result.Code, result.Stderr)
	}
	tmux.held(t)
	if result := s.RunCld(inB, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join -s x in b's pane: exit %d, stderr %q", result.Code, result.Stderr)
	}
	time.Sleep(time.Second)
	if begun := tmux.begun(t); begun != 1 {
		t.Errorf("%d tmux commands began to make cld-x while the first was held, want 1", begun)
	}
	tmux.release(t)
	sandbox.WaitFor(t, 10*time.Second, "both terminals on cld-x", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-x", "cld-x"})
	})
	waitScreen(t, first, "probe --name cld-x")
	waitScreen(t, second, "probe --name cld-x")
	claudeOf(t, s, "x")
	time.Sleep(500 * time.Millisecond)
	if probes := s.Probes(); len(probes) != 3 {
		t.Errorf("%d claudes started, want 3: a's, b's and one of x", len(probes))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-x"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-x]", sessions)
	}
}

// C-q ( and C-q ) move the terminal to the previous and the next of the sessions that run, in the
// order of their names, going round, and C-q L back to the session it came from, which the session
// it moved to records; the session it leaves runs on, detached. A session that has ended is passed
// over, and where C-q L has no session to go back to, or it has ended, or gone - its entry
// forgotten - or no other session runs, the message line says so and the terminal stays. The
// message goes after three seconds, or a key, and claude's pane is drawn meanwhile: tmux 3.5,
// which cannot, draws it once the message has gone.
func TestSwitchKeys(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "c")
	term := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitScreen(t, term, "probe --name cld-b")
	told := func(what, want string) {
		t.Helper()
		sandbox.WaitFor(t, 10*time.Second, what, func() bool {
			return slices.Contains(shownMessages(s, "cld-b"), want)
		})
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("%s: clients attached to %q, want [cld-b]", what, clients)
		}
	}
	term.Keys("C-q", "L")
	told("C-q L with no session to go back to", "cld: no session to go back to")
	claudeOf(t, s, "b").Send("link https://example.com/cld drawn meanwhile")
	waitScreen(t, term, "drawn meanwhile")
	on := func(key, name string) {
		t.Helper()
		term.Keys("C-q", key)
		waitScreen(t, term, "probe --name cld-"+name)
		sandbox.WaitFor(t, 10*time.Second, "the terminal on cld-"+name+" alone", func() bool {
			return slices.Equal(s.Clients(), []string{"cld-" + name})
		})
	}
	on(")", "c")
	on(")", "a")
	on("(", "c")
	on("L", "a")
	if last := lastOf(s, "a"); last != "c" {
		t.Errorf("a records %q as the last session, want c", last)
	}
	on("L", "c")
	on("(", "b")
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want a, b and c, each running", sessions)
	}
	s.RunCld(nil, "kill", "-s", "c")
	term.Keys("C-q", "L")
	told("C-q L to c, which has ended", "cld: session 'c' has ended")
	forget(t, s, "c")
	term.Keys("C-q", "L")
	told("C-q L to c, whose entry is forgotten", "cld: no session 'c'")
	on(")", "a")
	on(")", "b")
	s.RunCld(nil, "kill", "-s", "a")
	term.Keys("C-q", ")")
	told("C-q ) with no other session", "cld: no other session runs")
	outside := "cld: list --switch moves a terminal on one of cld's sessions, and TMUX names none\n"
	if result := s.RunCld(nil, "list", "--switch", "/dev/pts/0", "--to", "next"); result.Code != 1 || result.Stderr != outside {
		t.Errorf("list --switch outside cld's servers: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, outside)
	}
}

// The keys, and the command a terminal runs as it moves, name cld by the file it runs from, which
// goes quoted for run-shell's format and sh, for tmux's parser, and for the shell tmux runs the
// command with, the session's default-shell: here a copy of cld named "cld;" - a ";" at the end of
// a word of the popup's command would end it - in a directory whose name has a "'", a " ", a "#",
// a "\" and a ";", and of which nothing is left in a session's name. C-q s, Down and Enter, C-q )
// and C-q L move the terminal alike under sh, bash, zsh and fish, each where it is installed. So
// does cld join, run as claude runs !, in that directory, with words after "--" that fish, or
// tmux's parser within an if, would read otherwise quoted as sh quotes them: one with a "\" before
// a "'", one ending in "\" before one with a ";" and a "#" in it, and one of several lines, some
// indented, one starting with "#". claude gets them as given, in that directory.
func TestSwitchQuoting(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sh", "bash", "zsh", "fish"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skipf("%s is not installed", name)
			}
			s := sandbox.New(t)
			dir := filepath.Join(s.Root, `' #\;`)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(sandbox.Cld)
			if err != nil {
				t.Fatal(err)
			}
			cld := filepath.Join(dir, "cld;")
			if err := os.WriteFile(cld, binary, 0o755); err != nil {
				t.Fatal(err)
			}
			// Each session's server, by that cld, runs the command with shell.
			start := func(suffix string) terminal.Terminal {
				t.Helper()
				term := terminal.New(t, "tmux", s)
				term.Start([]string{cld, "join", "-s", suffix}, s.Env, s.Work)
				waitScreen(t, term, "probe --name cld-"+suffix)
				s.MustTmux("cld-"+suffix, "set", "-g", "default-shell", shell)
				return term
			}
			detach := start("b")
			detach.Keys("C-q", "d")
			sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detach.Running() })
			term := start("a")
			on := func(name, last string) {
				t.Helper()
				waitScreen(t, term, "probe --name cld-"+name)
				sandbox.WaitFor(t, 10*time.Second, "the terminal on cld-"+name+" alone", func() bool {
					return slices.Equal(s.Clients(), []string{"cld-" + name})
				})
				if got := lastOf(s, name); got != last {
					t.Errorf("%s records %q as the last session, want %s", name, got, last)
				}
			}
			term.Keys("C-q", "s")
			waitScreen(t, term, listHints)
			term.Keys("Down")
			sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
			term.Keys("Enter")
			on("b", "a")
			term.Keys("C-q", ")")
			on("a", "b")
			term.Keys("C-q", "L")
			on("b", "a")

			words := []string{`x\'`, `C:\`, `; echo moved #`, "fix:\n    indented\n# heading\nend"}
			join := exec.Command(cld, append([]string{"join", "-s", "c", "--"}, words...)...)
			join.Env, join.Dir = s.Environ(paneOf(t, s, "b")), dir
			if out, err := join.CombinedOutput(); err != nil || len(out) > 0 {
				t.Fatalf("join -s c in b's pane: %v, output %q, want exit 0 and no output", err, out)
			}
			on("c", "b")
			c := claudeOf(t, s, "c")
			if c.Cwd != dir {
				t.Errorf("claude c runs in %s, want %s", c.Cwd, dir)
			}
			if len(c.Argv) < len(words) || !slices.Equal(c.Argv[len(c.Argv)-len(words):], words) {
				t.Errorf("claude c has arguments %q, want %q last", c.Argv, words)
			}
		})
	}
}

// tmux runs the command a terminal runs as it moves with the session's default-shell, and cld
// writes it for sh, bash, zsh, fish, ksh and csh: with another - nu, say, where the words would
// not be read as cld means them - the terminal stays on its session, and cld says why, on the
// message line for a key, and to claude for ! cld join. The message line shows the message as it
// is: here the shell is in a directory named "50%done #x", whose "%d" tmux would replace with the
// day of the month, and whose "#" it would read as a format's.
func TestMoveNeedsAKnownShell(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	term := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitScreen(t, term, "probe --name cld-b")
	nu := filepath.Join(s.Root, "50%done #x", "nu")
	if err := os.Mkdir(filepath.Dir(nu), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", nu); err != nil {
		t.Fatal(err)
	}
	s.MustTmux("cld-b", "set", "-g", "default-shell", nu)
	refused := "cld: cannot move the terminal with tmux's default-shell '" + nu + "': cld writes the move for sh, bash, zsh, fish, ksh and csh alone"
	term.Keys("C-q", ")")
	sandbox.WaitFor(t, 10*time.Second, "C-q ) to be refused", func() bool {
		return slices.Contains(shownMessages(s, "cld-b"), refused)
	})
	want := refused + "; detach with C-q d and run cld join (see cld help join)\n"
	if result := s.RunCld(paneOf(t, s, "b"), "join", "-s", "a"); result.Code != 1 || result.Stdout != "" || result.Stderr != want {
		t.Errorf("join -s a in b's pane: exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want [cld-b]", clients)
	}
}

// The words given to a join in a pane go to the terminal's cld join encoded, a third longer (see
// TestJoinMovesTheTerminal), in the tmux command that moves the terminal: where they make it longer
// than the 16364 bytes tmux takes (see TestCommandLimit), join refuses them in the pane, as claude
// runs !, with status 2, naming the command's size, and the terminal stays on its session, no
// server started for the session named.
func TestJoinMoveCommandLimit(t *testing.T) {
	t.Parallel()
	const limit = 16364
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitScreen(t, term, "probe --name cld-b")
	result := s.RunCld(paneOf(t, s, "b"), "join", "-s", "x", "--", strings.Repeat("a", 13000))
	var size int
	fmt.Sscanf(result.Stderr, "cld: join's words make tmux's command %d bytes", &size)
	want := fmt.Sprintf("cld: join's words make tmux's command %d bytes, and tmux takes %d at most: "+
		"give claude long text in a file, as with --append-system-prompt-file\n", size, limit)
	if result.Code != 2 || result.Stdout != "" || size <= limit || result.Stderr != want {
		t.Errorf("join -s x in b's pane: exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) || !term.Running() {
		t.Errorf("clients attached to %q, want the terminal on cld-b", clients)
	}
	if _, err := os.Lstat(filepath.Join(s.SocketDir(), "cld-x")); err == nil {
		t.Errorf("a socket cld-x is left")
	}
}

// A join that a move runs, without -s, ends the idle sessions as join does, once it has taken its
// index, but for the session the terminal has just left, which claude may have moved it from with
// its Bash tool, no key typed there for longer than CLD_IDLE_DAYS: that one, detached by the move,
// runs on (TestJoinEndsIdleSessions has the terminal's own session, which TMUX names). The
// terminal's variables, CLD_IDLE_DAYS among them, are the join's.
func TestJoinMoveKeepsTheSessionItLeft(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0")
	term := startCld(t, s, "tmux", map[string]string{"CLD_IDLE_DAYS": "0.0001"}, "join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	time.Sleep(10 * time.Second)
	if result := s.RunCld(paneOf(t, s, "a"), "join"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("join in a's pane: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	waitScreen(t, term, "probe --name cld-1")
	sandbox.WaitFor(t, 10*time.Second, "claude 0 to exit", func() bool { return !probes["0"].Alive() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-1", "cld-a"}) || !claudeOf(t, s, "a").Alive() {
		t.Errorf("sessions %q, want [cld-1 cld-a], claude a running", sessions)
	}
	if note := regexp.MustCompile(`cld: ended session '0', idle for [0-9]+ seconds\r?\n`); !note.Match(term.Output()) {
		t.Errorf("the terminal got no note of the session ended: %q", term.Output())
	}
}

// The list C-q s shows over a session ends no idle session, as its keys' --to do not: tmux runs it
// with the environment of the session's server, not the terminal's, here a CLD_IDLE_DAYS of some
// 9 s that the terminal's cld join started the server with, past which session 0 has been idle.
// cld list, run where that value is set, would end it.
func TestPopupEndsNoSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0")
	term := startCld(t, s, "tmux", map[string]string{"CLD_IDLE_DAYS": "0.0001"}, "join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	time.Sleep(10 * time.Second)
	term.Keys("C-q", "s")
	waitScreen(t, term, listHints)
	time.Sleep(500 * time.Millisecond)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0", "cld-a"}) || !probes["0"].Alive() {
		t.Errorf("sessions %q with the list over a, want [cld-0 cld-a], claude 0 running", sessions)
	}
	if strings.Contains(term.Screen(), "ended") {
		t.Errorf("the list over a shows a session ended:\n%s", term.Screen())
	}
	term.Keys("Escape")
	sandbox.WaitFor(t, 10*time.Second, "the list to close", func() bool { return !strings.Contains(term.Screen(), listHints) })
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a"}) {
		t.Errorf("clients attached to %q, want [cld-a]", clients)
	}
}

// Joining from another directory leaves claude, and the session, where they are.
func TestJoinFromElsewhereKeepsClaude(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-n", "app", "-s", "task")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	second := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "app", "-s", "task")
	waitScreen(t, second, "probe --name cld-app-task")
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() || probe.Cwd != s.Work {
		t.Errorf("%d claude processes after joining, want the original one in %s", len(probes), s.Work)
	}
	if path := s.Format("cld-app-task", "#{session_path}"); path != s.Work {
		t.Errorf("session directory %s after joining, want %s", path, s.Work)
	}
}

// A reattach repaints claude's screen from tmux's own copy of it: the same text in the same
// attributes - also after claude pushed its keyboard modes again, as it does after an external
// editor, and in a terminal of another size - and claude has the alternate screen, mouse
// reporting and the wheel back.
func TestReattachRepaintsTheSameScreen(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "paint")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("rekey")
	waitScreen(t, first, "repainted")
	painted := cells(first.Styled())
	if underlined(painted) {
		t.Errorf("claude's screen is underlined:\n%s", strings.Join(painted, "\n"))
	}
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	for _, size := range [][2]int{{120, 40}, {100, 30}} {
		term := terminal.New(t, "tmux", s)
		term.Resize(size[0], size[1])
		term.Start(s.CldArgv("join", "-s", "paint"), s.Env, s.Work)
		waitScreen(t, term, "repainted")
		if repainted := cells(term.Styled()); !slices.Equal(repainted, painted) {
			t.Errorf("reattached at %dx%d:\n%s\nwant\n%s", size[0], size[1], strings.Join(repainted, "\n"), strings.Join(painted, "\n"))
		}
		if modes := term.Modes(); !modes.AltScreen || !modes.Mouse {
			t.Errorf("modes after reattaching at %dx%d %+v, want the alternate screen and mouse reporting on", size[0], size[1], modes)
		}
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
}

// claude exiting closes its own session only, and its server: the other session, its client and
// its claude carry on.
func TestClaudeExitClosesOnlyItsSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	b := startCld(t, s, "tmux", nil, "join", "-s", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)
	probes["cld-a"].Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld a to exit", func() bool { return !a.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if !b.Running() || !probes["cld-b"].Alive() {
		t.Error("session b did not survive session a's claude exiting")
	}
	sandbox.WaitFor(t, 10*time.Second, "session a's server to exit", func() bool {
		_, err := s.Tmux("cld-a", "list-sessions")
		return err != nil
	})
}

// A claude that fails keeps its session: the terminal stays attached and shows claude's last words
// and how to end the session, on the message line and, once a key has cleared it, on a line of the
// pane's border below claude's words, which stay at the top; list says claude exited, and join,
// which attaches to the session (see TestClaudeFailingDetached), refuses to make it anew until kill
// ends it. A join --resume whose claude finds no conversation fails that way. A pane split off in
// claude's window that fails while claude runs closes, with no hint.
func TestFailedClaudeKeepsSession(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		// startup is what claude prints as it fails at startup; fail makes it fail later instead
		startup string
		// how the hint says claude exited: tmux names a signal where the C library has
		// sys_signame (macOS), and numbers it elsewhere
		how  []string
		fail func(t *testing.T, s *sandbox.Sandbox)
	}{
		{"start", []string{"join", "-s", "bad"}, "Error: cannot start", []string{"status 1"}, nil},
		{"resume", []string{"join", "-s", "bad", "--resume", "x"}, "No conversation found with session ID: x", []string{"status 1"}, nil},
		{"status", []string{"join", "-s", "bad"}, "", []string{"status 3"}, func(t *testing.T, s *sandbox.Sandbox) {
			s.WaitProbes(1)[0].Send("exit 3")
		}},
		{"signal", []string{"join", "-s", "bad"}, "", []string{"signal 15", "signal term"}, func(t *testing.T, s *sandbox.Sandbox) {
			if err := syscall.Kill(s.WaitProbes(1)[0].PID, syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			extra := map[string]string{}
			if test.startup != "" {
				extra["CLD_PROBE_FAIL"] = test.startup
			}
			term := startCld(t, s, "tmux", extra, test.args...)
			if test.fail != nil {
				waitClients(t, s, 1)
				// Another pane of claude's window that fails - one claude splits off for a
				// teammate, say - closes as tmux closes it, with no hint: what cld sets for a
				// failed claude goes to claude's pane only.
				s.MustTmux("cld-bad", "split-window", "-d", "-t", "=cld-bad:", "exit 5")
				sandbox.WaitFor(t, 10*time.Second, "the split pane to close", func() bool {
					return s.Format("cld-bad", "#{pane_dead}") == "0"
				})
				if strings.Contains(term.Screen(), "claude exited") {
					t.Errorf("a split pane that failed shows the hint:\n%s", term.Screen())
				}
				test.fail(t, s)
			}
			if test.startup != "" {
				waitScreen(t, term, test.startup)
			}
			hint := ": cld kill -s bad ends the session, C-q d or cld detach -s bad detaches"
			waitScreen(t, term, hint)
			how := slices.IndexFunc(test.how, func(how string) bool {
				return strings.Contains(term.Screen(), "claude exited with "+how+hint)
			})
			if how < 0 {
				t.Fatalf("the hint does not say claude exited with %s:\n%s", strings.Join(test.how, " or "), term.Screen())
			}
			term.Keys("x")
			waitBorderLine(t, term, "claude exited with "+test.how[how]+hint)
			if screen := term.Screen(); !strings.HasPrefix(screen, test.startup) {
				t.Errorf("claude's words are not at the top:\n%s", screen)
			}
			if !term.Running() {
				t.Error("the terminal was detached")
			}

			list := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
				"bad   exited    now          " + s.Work + "\n"
			if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
			}
			for _, option := range []string{"--new", "--resume"} {
				want := "cld: session 'bad' exists, and " + option + " would be lost: its claude has started; attach to it with cld join -s bad, or give another -s SUFFIX\n"
				args := []string{"join", "-s", "bad", option}
				if option == "--resume" {
					args = append(args, "x")
				}
				if result := s.RunCld(nil, args...); result.Code != 1 || result.Stderr != want {
					t.Errorf("%s: exit %d, stderr %q, want exit 1, stderr %q", strings.Join(args, " "), result.Code, result.Stderr, want)
				}
			}
			if result := s.RunCld(nil, "kill", "-s", "bad"); result.Code != 0 {
				t.Errorf("kill: exit %d, stderr %q", result.Code, result.Stderr)
			}
			sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q after kill, want none", sessions)
			}
		})
	}
}

// A claude that fails with no terminal attached leaves the hint to join, which shows it on the
// message line: from the hook, tmux would keep it and show it in view-mode over the next session
// any terminal attaches to. The line of the pane's border is there all the same, and stays once a
// key has cleared the message line; it goes to claude's window, not to a session claude made on
// its server. Joining a live session shows no hint.
func TestClaudeFailingDetached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "bad")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-bad/side")
	})
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })
	probe.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-bad", "#{pane_dead}") == "1" })
	for session, want := range map[string]string{"cld-bad": "bottom", "side": ""} {
		if status := s.MustTmux("cld-bad", "show", "-wv", "-t", "="+session+":", "pane-border-status"); status != want {
			t.Errorf("pane-border-status of %s's window %q, want %q", session, status, want)
		}
	}

	other := startCld(t, s, "tmux", nil, "join", "-s", "other")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	if mode := s.Format("cld-other", "#{pane_mode}"); mode != "" {
		t.Errorf("a new session opens in %s:\n%s", mode, other.Screen())
	}

	hint := "claude exited with status 1: cld kill -s bad ends the session, C-q d or cld detach -s bad detaches"
	joined := startCld(t, s, "tmux", nil, "join", "-s", "bad")
	waitScreen(t, joined, hint)
	if screen := joined.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
		t.Errorf("the hint is not on the message line:\n%s", screen)
	}
	if mode := s.Format("cld-bad", "#{pane_mode}"); mode != "" {
		t.Errorf("the joined session is in %s", mode)
	}
	joined.Keys("x")
	waitBorderLine(t, joined, hint)

	live := startCld(t, s, "tmux", nil, "join", "-s", "other")
	waitScreen(t, live, "probe --name cld-other")
	if screen := live.Screen(); strings.Contains(screen, "claude exited") {
		t.Errorf("joining a live session shows the hint:\n%s", screen)
	}
}

// The lines that say how claude exited and how to end the session say what fits the terminal's
// width whole, rather than be cut at it: a command cut short could name another session, -s 1 of
// -s 12, and the border line stays on screen. As the terminal narrows, the line leaves out C-q d
// and cld detach, and then the kill.
func TestFailedClaudeLinesFit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// Wide enough for all of it from the start: a resize racing claude's exit would have the
	// message line, which covers the border line until a key is pressed, say less.
	term := terminal.New(t, "tmux", s)
	term.Resize(150, 40)
	term.Start(s.CldArgv("join", "-n", "my-awesome-proj", "-s", "12"), s.Env, s.Work)
	waitClients(t, s, 1)
	s.WaitProbes(1)[0].Send("exit 1")
	exited := "claude exited with status 1"
	kill := exited + ": cld kill -n my-awesome-proj -s 12 ends the session"
	all := kill + ", C-q d or cld detach -n my-awesome-proj -s 12 detaches"
	waitScreen(t, term, all)
	term.Keys("x")
	for _, width := range []struct {
		columns int
		text    string
	}{{150, all}, {120, kill}, {90, kill}, {80, exited}} {
		term.Resize(width.columns, 40)
		waitBorderLine(t, term, width.text)
	}
}

// Each session runs on a server of its own, named like it, and each claude gets the environment of
// the shell that ran the cld join that made it - where a resuming claude looks for the
// conversation, in CLAUDE_CONFIG_DIR. On one server shared by every session, tmux started each pane
// with the environment of the client that had started the server - the first session's - but for
// PATH and the update-environment variables.
func TestServerPerSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	shells := map[string]map[string]string{
		"a": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-a")},
		"b": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-b"), "VIRTUAL_ENV": "/venv/b"},
		"c": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-c")},
	}
	startCld(t, s, "tmux", shells["a"], "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", shells["b"], "join", "-s", "b")
	s.WaitProbes(2)
	startCld(t, s, "tmux", shells["c"], "join", "-s", "c", "--resume", "cld-c")
	for _, probe := range s.WaitProbes(3) {
		shell := shells[strings.TrimPrefix(probe.Argv[1], "cld-")]
		for _, name := range []string{"CLAUDE_CONFIG_DIR", "VIRTUAL_ENV"} {
			if value, want := probe.Env[name], shell[name]; value != want {
				t.Errorf("claude %s sees %s=%q, want %q", probe.Argv[1], name, value, want)
			}
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-c], each on its own server", sessions)
	}
}

func TestIgnoresUserTmuxConfig(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join")
	s.WaitProbes(1)
	if left := s.MustTmux("cld-0", "show", "-gv", "status-left"); left == "POISONED" {
		t.Error("~/.tmux.conf was loaded")
	}
	socket := filepath.Join(s.Root, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	if _, err := os.Stat(socket); err == nil {
		t.Error("cld started the default tmux server")
	}
}

// The hooks join gives claude keep its status in @cld-status on its session, which the
// tab's title reads (see TestContractTitle), telling apart what claude's own title does outside
// tmux: busy from a prompt on, and after a question answered; waiting while claude asks - a
// permission, an MCP server's question; idle once the turn is done, or failed, or a tool of it was
// interrupted, or claude says it has been idle a while. claude runs each hook in its own
// environment, here with the TMUX and TMUX_PANE of its pane, which the hooks do not need (see
// TestHooksOutsideThePane).
func TestStatusHooks(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "x", "--resume", "cld-x")
	probe := s.WaitProbes(1)[0]
	status := func() string { return s.Format("cld-x", "#{@cld-status}") }
	if got := status(); got != "" {
		t.Fatalf("@cld-status is %q before any hook, want none", got)
	}
	for _, step := range []struct{ event, input, want string }{
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "busy"},
		{"PermissionRequest", `{"tool_name":"Bash"}`, "waiting"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "busy"},
		{"Elicitation", `{"mcp_server_name":"m"}`, "waiting"},
		{"ElicitationResult", `{"mcp_server_name":"m"}`, "busy"},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":false}`, "busy"},
		{"Notification", `{"notification_type":"permission_prompt"}`, "busy"},
		{"Stop", `{}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":true}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"StopFailure", `{"error":"rate_limit"}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"Notification", `{"notification_type":"idle_prompt"}`, "idle"},
		{"SessionStart", `{"source":"clear"}`, "idle"},
	} {
		probe.Hook(step.event, step.input)
		if got := status(); got != step.want {
			t.Errorf("@cld-status is %q after %s %s, want %q", got, step.event, step.input, step.want)
		}
	}
	if global := s.MustTmux("cld-x", "show", "-gqv", "@cld-status"); global != "" {
		t.Errorf("global @cld-status %q, want none: it goes to claude's session", global)
	}
}

// The same hooks keep @cld-worktree on claude's session, which the tab's title reads (see
// TestContractTitle): 1 while claude's directory is in a linked git worktree - one claude's
// --worktree makes, or another - and 0 in the main worktree or outside a repository, as claude
// starts in it, and each time claude's directory changes. claude runs the hooks in its directory.
func TestWorktreeHooks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// start is where cld runs, in the work directory's repository, which has the linked
		// worktree .claude/worktrees/x
		start string
		want  string
	}{
		{"in the main worktree", ".", "0"},
		{"in a linked worktree", ".claude/worktrees/x", "1"},
		{"in a linked worktree's subdirectory", ".claude/worktrees/x/sub", "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			worktree := gitWorktree(t, s, "x")
			if err := os.Mkdir(filepath.Join(worktree, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", filepath.Join(s.Work, test.start), nil, "join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			status := func() string { return s.Format("cld-x", "#{@cld-worktree}") }
			probe.Hook("SessionStart", `{"source":"startup"}`)
			if got := status(); got != test.want {
				t.Errorf("@cld-worktree is %q as claude starts, want %q", got, test.want)
			}
			for _, step := range []struct{ dir, want string }{
				{worktree, "1"},
				{s.Work, "0"},
				{filepath.Join(worktree, "sub"), "1"},
				{s.Root, "0"},
				{filepath.Join(s.Work, ".git"), "0"},
				{worktree, "1"},
			} {
				probe.Send("cd " + step.dir)
				probe.Hook("CwdChanged", `{"new_cwd":"`+step.dir+`"}`)
				if got := status(); got != step.want {
					t.Errorf("@cld-worktree is %q once claude is in %s, want %q", got, step.dir, step.want)
				}
			}
		})
	}
}

// Where cld finds no git, claude gets no hooks that run it, and the title never says [w]; the
// hooks that keep the session's entry in cld's record run no git, and agent view is off all the
// same.
func TestWorktreeHooksWithoutGit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{"PATH": s.Tools("tmux", "claude"), "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}, "join", "-s", "x")
	if result.Code != 0 {
		t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	var given struct {
		DisableAgentView bool           `json:"disableAgentView"`
		Hooks            map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(argv[i+1]), &given); err != nil {
		t.Fatal(err)
	}
	if !given.DisableAgentView {
		t.Errorf("settings without agent view off: %s", argv[i+1])
	}
	events := slices.Sorted(maps.Keys(given.Hooks))
	if want := []string{"Elicitation", "ElicitationResult", "Notification", "PermissionRequest", "PostToolUse",
		"PostToolUseFailure", "SessionEnd", "SessionStart", "Stop", "StopFailure", "UserPromptSubmit"}; !slices.Equal(events, want) {
		t.Errorf("hooks for %q, want %q", events, want)
	}
	if start, _ := json.Marshal(given.Hooks["SessionStart"]); strings.Contains(string(start), "rev-parse") {
		t.Errorf("SessionStart hooks run git: %s", start)
	}
}

// claude does not always run the hooks in its pane: it can run a conversation in a background
// worker of its daemon, which the claude in the pane shows, and the worker runs the hooks without
// TMUX and TMUX_PANE (claude 2.1.284). The hooks name the server and the session themselves, so
// they keep the status and the worktree on claude's session all the same - not on a session
// claude made on its server, nor on one of the default server, where a bare tmux goes without
// TMUX and where it failed with "no current session" while that server had none.
func TestHooksOutsideThePane(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "x")
	probe := s.WaitProbes(1)[0]
	s.MustTmux("cld-x", "new-session", "-d", "-s", "made")
	s.MustTmux("default", "new-session", "-d", "-s", "other")
	probe.Send("unsetenv TMUX TMUX_PANE")
	for _, step := range []struct{ event, input, option, want string }{
		{"SessionStart", `{"source":"resume"}`, "@cld-worktree", "0"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "@cld-status", "busy"},
		{"PermissionRequest", `{"tool_name":"Bash"}`, "@cld-status", "waiting"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "@cld-status", "busy"},
		{"Stop", `{}`, "@cld-status", "idle"},
	} {
		probe.Hook(step.event, step.input)
		if got := s.Format("cld-x", "#{"+step.option+"}"); got != step.want {
			t.Errorf("%s is %q after %s %s, want %q", step.option, got, step.event, step.input, step.want)
		}
	}
	for _, other := range []struct{ server, session string }{{"cld-x", "made"}, {"default", "other"}} {
		if got := s.MustTmux(other.server, "list-panes", "-s", "-t", "="+other.session, "-F", "#{@cld-status}#{@cld-worktree}"); got != "" {
			t.Errorf("session %s on server %s has %q of the hooks' options, want none", other.session, other.server, got)
		}
	}
}

// The hooks name the server's socket by an absolute path also where TMUX_TMPDIR is relative,
// which tmux takes from the directory cld runs in: they run in claude's, which claude changes.
func TestHooksUnderRelativeTmuxTmpdir(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{
		"PATH":                  filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
		"TMUX_TMPDIR":           "..",
	}, "join", "-s", "x")
	if result.Code != 0 {
		t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	// The work directory's parent is the sandbox's TMUX_TMPDIR.
	if want := settings(s, sandbox.FakeTmux, sandbox.RealGit, "cld-x", s.Work, false); argv[i+1] != want {
		t.Errorf("settings\n%s\nwant\n%s", argv[i+1], want)
	}
}

func TestServerOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join")
	s.WaitProbes(1)
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
	// What cld sets for a failed claude goes to claude's pane, not its window, and the tab's title -
	// the tmux the title's job runs is the one cld checked - and the session's home, the work
	// directory, to claude's session; the other panes of claude's window, and the sessions claude
	// makes on its server, keep tmux's (see TestFailedClaudeKeepsSession). show without -v prints
	// an option only where it is set.
	for _, option := range []struct {
		args  []string
		value string
	}{
		{[]string{"-pv", "-t", "=cld-0:", "remain-on-exit"}, "on"},
		{[]string{"-p", "-t", "=cld-0:", "remain-on-exit-format"}, "remain-on-exit-format ''"},
		{[]string{"-w", "-t", "=cld-0:", "remain-on-exit"}, ""},
		{[]string{"-w", "-t", "=cld-0:", "remain-on-exit-format"}, ""},
		{[]string{"-gwv", "remain-on-exit"}, "off"},
		{[]string{"-v", "-t", "=cld-0:", "set-titles"}, "on"},
		{[]string{"-v", "-t", "=cld-0:", "set-titles-string"}, "#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-0#{?@cld-worktree, [w],}"},
		{[]string{"-v", "-t", "=cld-0:", "@cld-busy"}, busyMarker},
		{[]string{"-v", "-t", "=cld-0:", "@cld-tmux"}, sandbox.RealTmux},
		{[]string{"-v", "-t", "=cld-0:", "@cld-home"}, s.Work},
		{[]string{"-gv", "set-titles"}, "off"},
	} {
		if value := s.MustTmux("cld-0", append([]string{"show"}, option.args...)...); value != option.value {
			t.Errorf("show %s is %q, want %q", strings.Join(option.args, " "), value, option.value)
		}
	}
	if hooks := s.MustTmux("cld-0", "show-hooks", "-g", "pane-died"); strings.Contains(hooks, "[") {
		t.Errorf("global pane-died hooks, want none: they go to claude's pane\n%s", hooks)
	}
	if hooks := s.MustTmux("cld-0", "show-hooks", "-w", "-t", "=cld-0:", "pane-died"); hooks != "" {
		t.Errorf("pane-died hooks of claude's window, want none: they go to claude's pane\n%s", hooks)
	}
	if hooks := strings.Split(s.MustTmux("cld-0", "show-hooks", "-p", "-t", "=cld-0:", "pane-died"), "\n"); len(hooks) != 1 || !strings.Contains(hooks[0], "window_active_clients") {
		t.Errorf("pane-died hooks of claude's pane, want one:\n%s", strings.Join(hooks, "\n"))
	}
	// Two cld join -s 0 at once could both set the options on one server - the second one going on
	// without the record's lock, past its wait (see TestJoinAtOnce): the lookup of each finds no
	// server, and the tmux command of the second reaches the server the first one started, setting
	// them again before its new-session fails. The terminal features go to fixed indexes, so each
	// entry is there once however often it is set. The fake tmux says no server is running, and
	// runs the real one for the rest. Without -s, the second would take NAME 1, from cld's record.
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	second := s.RunCldOnTerminal(map[string]string{
		"PATH":               filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"],
		"CLD_FAKE_TMUX_REAL": realTmux,
	}, "join", "-s", "0", "--new")
	if want := "duplicate session: cld-0\n"; second.Code != 1 || second.Stderr != want {
		t.Errorf("a second cld join: exit %d, stderr %q, want exit 1, stderr %q", second.Code, second.Stderr, want)
	}
	features := strings.Split(s.MustTmux("cld-0", "show", "-sv", "terminal-features"), "\n")
	for _, entry := range []string{"xterm*:extkeys:hyperlinks", "wezterm:hyperlinks", "alacritty:hyperlinks"} {
		if count := len(slices.DeleteFunc(slices.Clone(features), func(f string) bool { return f != entry })); count != 1 {
			t.Errorf("%d %s entries in terminal-features, want 1", count, entry)
		}
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want the first one", len(probes))
	}
	// "list-keys -T prefix C-q" would be shorter, but tmux 3.7 prints nothing for it.
	bindings := strings.Split(s.MustTmux("cld-0", "list-keys", "-T", "prefix"), "\n")
	if !slices.ContainsFunc(bindings, func(binding string) bool {
		return slices.Equal(strings.Fields(binding), []string{"bind-key", "-T", "prefix", "C-q", "send-prefix"})
	}) {
		t.Errorf("C-q C-q is not bound to send-prefix:\n%s", strings.Join(bindings, "\n"))
	}
	// s, ( , ) and L run cld, in place of tmux's choose-tree and switch-client (see
	// TestSwitchKeys); tmux writes ( and ) with a "\" before them.
	for _, key := range []string{"s", "(", ")", "L"} {
		if !slices.ContainsFunc(bindings, func(binding string) bool {
			fields := strings.Fields(binding)
			return len(fields) > 5 && strings.TrimPrefix(fields[3], `\`) == key && fields[4] == "run-shell" && fields[5] == "-b"
		}) {
			t.Errorf("C-q %s is not bound to run-shell -b:\n%s", key, strings.Join(bindings, "\n"))
		}
	}
	// tmux hands a mouse key it has no binding for to the pane: a Ctrl+click and an Alt+right-click
	// reach claude whole (see TestContractClicks). tmux's other mouse bindings stay.
	var keys []string
	for binding := range strings.SplitSeq(s.MustTmux("cld-0", "list-keys", "-T", "root"), "\n") {
		if fields := strings.Fields(binding); len(fields) > 3 {
			keys = append(keys, fields[3])
		}
	}
	if !slices.Contains(keys, "MouseDown1Pane") || slices.Contains(keys, "C-MouseDown1Pane") || slices.Contains(keys, "M-MouseDown3Pane") {
		t.Errorf("root bindings %q, want tmux's without C-MouseDown1Pane and M-MouseDown3Pane", keys)
	}
}

// claude trusts TERMINAL_EMULATOR, and the other variables that name a terminal to it, over
// TERM_PROGRAM=tmux, and a server keeps the environment of the client that started it: unless
// cld left them out of the environment it runs tmux with, a session created in a JetBrains
// terminal, or in Cursor's, would hand them to its claude, joined from anywhere, and to whatever
// claude starts through tmux on its server. VS Code's askpass goes with VSCODE_GIT_ASKPASS_MAIN,
// and its editor with it (see TestVSCodeGit).
func TestClaudeNeverSeesTheTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	const dist = "/home/u/.cursor-server/bin/1/extensions/git/dist/"
	given := map[string]string{
		"TERMINAL_EMULATOR":             "JetBrains-JediTerm",
		"__CFBundleIdentifier":          "com.jetbrains.goland",
		"CURSOR_TRACE_ID":               "0123456789abcdef",
		"VisualStudioVersion":           "17.0",
		"VSCODE_GIT_ASKPASS_MAIN":       dist + "askpass-main.js",
		"VSCODE_GIT_ASKPASS_NODE":       "/home/u/.cursor-server/bin/1/node",
		"VSCODE_GIT_ASKPASS_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":         "/run/user/1000/vscode-git-1.sock",
		"GIT_ASKPASS":                   dist + "askpass.sh",
		"VSCODE_GIT_EDITOR_MAIN":        dist + "git-editor-main.js",
		"VSCODE_GIT_EDITOR_NODE":        "/home/u/.cursor-server/bin/1/node",
		"VSCODE_GIT_EDITOR_EXTRA_ARGS":  "",
		"GIT_EDITOR":                    `"` + dist + `git-editor.sh"`,
	}
	startCld(t, s, "tmux", given, "join", "-s", "ide")
	probe := s.WaitProbes(1)[0]
	global := strings.Split(s.MustTmux("cld-ide", "show-environment", "-g"), "\n")
	for _, name := range slices.Sorted(maps.Keys(given)) {
		if value, found := probe.Env[name]; found {
			t.Errorf("claude sees %s=%s", name, value)
		}
		if slices.ContainsFunc(global, func(variable string) bool { return strings.HasPrefix(variable, name+"=") }) {
			t.Errorf("the server's environment has %s:\n%s", name, strings.Join(global, "\n"))
		}
	}
}

// tmux takes a terminal for UTF-8 only where TMUX is set or LC_ALL, LC_CTYPE or LANG names UTF-8,
// and otherwise draws each character that is not ASCII as "_" - most of claude's UI, over ssh to
// a host whose sshd takes no LANG. The clients of join - where it creates the session, with
// --resume here, and where it attaches - and of the list's Enter take it for UTF-8 whatever the
// locale, and show what claude draws as it is: here its arguments, where --resume puts SESSION.
func TestClientsTakeUTF8(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	const conversation = "✳ ⏺ café │"
	// attached waits for the terminal to attach to session cld-NAME, and checks that tmux takes it
	// for UTF-8 and, where claude draws some, shows what is not ASCII.
	attached := func(how string, term terminal.Terminal, name string, shows bool) {
		t.Helper()
		waitClients(t, s, 1)
		if flag := s.MustTmux("cld-"+name, "list-clients", "-F", "#{client_utf8}"); flag != "1" {
			t.Errorf("%s: client_utf8 %q under LANG=C, want 1", how, flag)
		}
		if shows {
			waitScreen(t, term, "--resume "+conversation)
		}
	}
	detach := func(term terminal.Terminal) {
		t.Helper()
		term.Keys("C-q", "d")
		waitClients(t, s, 0)
	}

	term := startCld(t, s, "tmux", notUTF8, "join", "-s", "n")
	attached("join creating", term, "n", false)
	detach(term)
	term = startCld(t, s, "tmux", notUTF8, "join", "-s", "r", "--resume", conversation)
	attached("join --resume", term, "r", true)
	detach(term)
	term = startCld(t, s, "tmux", notUTF8, "join", "-s", "r")
	attached("join", term, "r", true)
	detach(term)
	term = terminal.New(t, "tmux", s)
	startList(t, s, term, listScript, notUTF8)
	waitScreen(t, term, listHints)
	term.Keys("Down", "Enter")
	attached("the list's Enter", term, "r", true)
}

// Inside another tmux ($TMUX set) cld nests: its server is another one. Its client gets an empty
// TMUX, as join's has to (see TestNestsOnADeadPanesPty), and takes the terminal, a pane of the
// other tmux, for UTF-8 whatever the locale says, with -u as outside one (see TestClientsTakeUTF8;
// an empty TMUX would do it by itself). cld looks for its own panes on its own servers only,
// cld-NAME: in a live pane of any other server it nests - the default one, the one server cld
// 0.3.0 and earlier shared, one named like no session of cld's can be, or one named like cld's
// that cld did not start, the user's own tmux -L cld-outer (see TestLeavesAForeignServerAlone),
// where list is interactive too.
func TestNestsInsideAnotherTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	elsewhere := filepath.Join(s.Root, "elsewhere", "default") + ",1,0"
	startCld(t, s, "tmux", map[string]string{"TMUX": elsewhere, "LANG": "C"}, "join")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0"}) {
		t.Errorf("sessions %q, want [cld-0]", sessions)
	}
	if utf8 := s.MustTmux("cld-0", "list-clients", "-F", "#{client_utf8}"); utf8 != "1" {
		t.Errorf("client_utf8 %q under LANG=C, want 1", utf8)
	}

	for i, server := range []string{"default", "cld", "cld-x.y", "cld-outer"} {
		// A pane of that server runs cld on its own pty, with the TMUX tmux sets for it.
		name := "in" + strconv.Itoa(i)
		out := filepath.Join(s.Root, name)
		s.MustTmux(server, append([]string{"-f", "/dev/null", "new-session", "-d", "-s", "outer", "-c", s.Work,
			"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, s.CldArgv("join", "-s", name)...)...)
		sandbox.WaitFor(t, 10*time.Second, "cld in a pane of server "+server+" to attach or return", func() bool {
			_, err := os.Stat(out + ".code")
			return err == nil || slices.Contains(s.Clients(), "cld-"+name)
		})
		if !slices.Contains(s.Clients(), "cld-"+name) {
			stderr, _ := os.ReadFile(out + ".err")
			t.Errorf("cld join in a pane of server %s did not attach: %q", server, stderr)
		}
	}
	s.MustTmux("cld-outer", append([]string{"new-session", "-d", "-s", "list", "-c", s.Work}, s.CldArgv("list")...)...)
	sandbox.WaitFor(t, 10*time.Second, "the interactive list in a pane of server cld-outer", func() bool {
		screen, _ := s.Tmux("cld-outer", "capture-pane", "-p", "-t", "=list:")
		return strings.Contains(screen, "enter to join")
	})
}

// yourTmuxLines are the lines the user guide gives for ~/.tmux.conf, for the user's own tmux that
// cld runs in: with extended-keys always where cld's tmux is older than 3.7 - before37 - which
// then asks no tmux for modified keys, and with on the other tmux passes them only to a program
// that asks.
func yourTmuxLines(before37 bool) string {
	extendedKeys := "on"
	if before37 {
		extendedKeys = "always"
	}
	return "set -s extended-keys " + extendedKeys + "\n" +
		"set -as terminal-features 'xterm*:extkeys:hyperlinks'\n" +
		"set -s set-clipboard on\n" +
		"set -s focus-events on\n"
}

// tmuxOlder reports whether the tmux the tests run - cld's, and the tmux cld runs in - is older
// than major.minor, from its tmux -V: "tmux 3.5a" is 3.5, and a development build's
// "tmux next-3.8" 3.8. One without a version, "tmux master", is not older.
func tmuxOlder(t *testing.T, major, minor int) bool {
	t.Helper()
	out, err := exec.Command(sandbox.RealTmux, "-V").Output()
	if err != nil {
		t.Fatalf("tmux -V: %v", err)
	}
	match := regexp.MustCompile(`([0-9]+)\.([0-9]+)`).FindStringSubmatch(string(out))
	if match == nil {
		return false
	}
	var version []int
	for _, digits := range match[1:] {
		n, _ := strconv.Atoi(digits)
		version = append(version, n)
	}
	return slices.Compare(version, []int{major, minor}) < 0
}

// shownMessages is the messages that server's tmux has shown, from its log, however soon claude
// drew over them.
func shownMessages(s *sandbox.Sandbox, server string) []string {
	var messages []string
	for line := range strings.SplitSeq(s.MustTmux(server, "show-messages"), "\n") {
		if _, message, found := strings.Cut(line, " message: "); found {
			messages = append(messages, message)
		}
	}
	return messages
}

// Inside the user's own tmux, which reads the terminal's keys before cld's client in its pane does,
// join and the list's Enter name the keys it keeps from claude on the message line once attached,
// until a key, which reaches claude: its prefix and prefix2, and Shift+Enter where its
// extended-keys is off - it then ignores the request for modified keys that cld's client makes, and
// Shift+Enter reaches claude as Enter. The claude join starts draws over the message as it enters
// the alternate screen, and tmux draws it again a second later. A default tmux keeps C-b, which C-b
// C-b sends through, and Shift+Enter; with the user guide's lines Shift+Enter and clipboard copies
// come through, and with no prefix either nothing is said, which tmux's log of messages would show
// however soon claude drew over it. The baseline terminal runs the user's tmux, server yours, whose
// pane runs cld once yours has taken the terminal for a tmux, which it recognises by its answers:
// from tmux 3.7 for one that sends modified keys (see TestKeysYourTmuxKeepsWithoutItsTerminal), and
// before, not, so that under extended-keys always alone cld names Shift+Enter all the same. Before
// 3.7 cld's tmux does not take yours for one either, and its client asks yours for no modified
// keys: with extended-keys on, which passes them only to a program that asks, Shift+Enter arrives
// as Enter, and the guide's lines have always. tmux 3.5 shows no message but by holding back
// claude's screen until the key, and there cld names nothing, the keys kept as elsewhere. join
// makes a session it resumes as it makes any.
func TestKeysYourTmuxKeeps(t *testing.T) {
	t.Parallel()
	const guide = `: see "Inside your own tmux" in cld's guide`
	before36, before37 := tmuxOlder(t, 3, 6), tmuxOlder(t, 3, 7)
	lines := yourTmuxLines(before37)
	twoPrefixes := "your tmux keeps C-a and C-b" + guide
	extendedKeysOn, shiftEnterOn := "your tmux keeps C-b"+guide, expectations["tmux"].shiftEnter
	if before37 {
		twoPrefixes = "your tmux keeps C-a, C-b and Shift+Enter: see cld's guide"
		extendedKeysOn, shiftEnterOn = "your tmux keeps C-b and Shift+Enter"+guide, []string{"\r"}
	}
	for _, yours := range []struct {
		name string
		// conf is the configuration of yours, or none for tmux's defaults
		conf string
		// message is the message line from tmux 3.6, or none for no message
		message    string
		shiftEnter []string
	}{
		{"default", "", "your tmux keeps C-b and Shift+Enter" + guide, []string{"\r"}},
		{"the guide's lines", lines, "your tmux keeps C-b" + guide, expectations["tmux"].shiftEnter},
		{"extended-keys on", yourTmuxLines(false), extendedKeysOn, shiftEnterOn},
		{"two prefixes", "set -g prefix C-a\nset -g prefix2 C-b\nset -s extended-keys always\n", twoPrefixes, expectations["tmux"].shiftEnter},
		{"no prefix", lines + "set -g prefix None\n", "", expectations["tmux"].shiftEnter},
	} {
		t.Run(yours.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			conf := "/dev/null"
			if yours.conf != "" {
				conf = filepath.Join(s.Root, "yours.conf")
				s.WriteFile(conf, yours.conf)
			}
			message := yours.message
			if before36 {
				message = ""
			}
			term := terminal.New(t, "tmux", s)
			term.Start([]string{"tmux", "-L", "yours", "-f", conf, "new-session", "-s", "yours", "sleep", "3600"}, s.Env, s.Work)
			// yours learns its terminal's features from the terminal's answers, which come as its
			// first pane starts - overline among them, from its entry for a tmux, and extkeys from
			// tmux 3.7: cld runs in the pane once they have come.
			sandbox.WaitFor(t, 10*time.Second, "yours to take its terminal for a tmux", func() bool {
				features, err := s.Tmux("yours", "display", "-p", "-t", "=yours:", "#{client_termfeatures}")
				return err == nil && slices.Contains(strings.Split(features, ","), "overline")
			})
			s.MustTmux("yours", append([]string{"respawn-pane", "-k", "-t", "=yours:", "-c", s.Work}, s.CldArgv("join", "-s", "nested")...)...)
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			waitScreen(t, term, "probe --name cld-nested")
			if message != "" {
				waitScreen(t, term, message)
			}
			shiftEnter := between(t, term, probe, "S-Enter")
			if !slices.Contains(yours.shiftEnter, shiftEnter) {
				t.Errorf("Shift+Enter arrives as %s, want one of %q", strconv.Quote(shiftEnter), yours.shiftEnter)
			}
			sandbox.WaitFor(t, 10*time.Second, "the message to go with the first key", func() bool {
				return !strings.Contains(term.Screen(), "your tmux keeps")
			})
			switch yours.name {
			case "default":
				if got := between(t, term, probe, "C-b", "C-b"); got != "\x02" {
					t.Errorf("C-b C-b arrives as %s, want \"\\x02\"", strconv.Quote(got))
				}
				// join and the list's Enter, each in a window of yours of its own, show it to their
				// own terminals.
				s.MustTmux("yours", append([]string{"new-window", "-c", s.Work}, s.CldArgv("join", "-s", "nested")...)...)
				waitClients(t, s, 2)
				if message != "" {
					waitScreen(t, term, message)
				}
				s.MustTmux("yours", append([]string{"new-window", "-c", s.Work}, s.CldArgv("list")...)...)
				waitScreen(t, term, "> nested")
				term.Keys("Enter")
				waitClients(t, s, 3)
				if message != "" {
					waitScreen(t, term, message)
				}
			case "the guide's lines":
				probe.Send("osc52 copied inside your tmux")
				if clipboard := term.Clipboard(); clipboard != "copied inside your tmux" {
					t.Errorf("clipboard %q, want %q", clipboard, "copied inside your tmux")
				}
			}
			if message == "" {
				if messages := shownMessages(s, "cld-nested"); len(messages) != 0 {
					t.Errorf("messages %q, want none", messages)
				}
			}
		})
	}
}

// With extended-keys on, the user's tmux still asks its terminal for modified keys only where it
// takes the terminal for one that sends them - one it recognises, or one the guide's
// terminal-features line names - and Shift+Enter comes as Enter from any other: cld names
// Shift+Enter where the client that tmux formats for the pane's session lacks the feature extkeys,
// as where no client is attached at all. The tests' terminal, a tmux, is one tmux 3.7 recognises,
// so yours runs detached here, and tmux's log of messages tells what cld's client was shown, and
// that tmux 3.5 was shown none. Three keys leave no room within 80 columns for the guide's
// section, and the line names the guide alone. A server named like cld's that cld did not start,
// the user's own tmux -L cld-yours, is the user's tmux as much as any other (see
// TestLeavesAForeignServerAlone). Where TMUX names a tmux that cld's terminal is no pane of - a
// TMUX, and a TMUX_PANE, that a program inherited - that tmux reports another pane, and cld says
// nothing.
func TestKeysYourTmuxKeepsWithoutItsTerminal(t *testing.T) {
	t.Parallel()
	want := []string{"your tmux keeps C-b, C-a and Shift+Enter: see cld's guide"}
	if tmuxOlder(t, 3, 6) {
		want = nil
	}
	for _, server := range []string{"yours", "cld-yours"} {
		t.Run("a pane of "+server, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			conf := filepath.Join(s.Root, "yours.conf")
			s.WriteFile(conf, "set -s extended-keys on\nset -g prefix2 C-a\n")
			s.MustTmux(server, append([]string{"-f", conf, "new-session", "-d", "-s", "yours", "-c", s.Work}, s.CldArgv("join", "-s", "nested")...)...)
			s.WaitProbes(1)
			waitClients(t, s, 1)
			if messages := shownMessages(s, "cld-nested"); !slices.Equal(messages, want) {
				t.Errorf("messages %q, want %q", messages, want)
			}
		})
	}
	t.Run("no pane of yours", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		s.MustTmux("yours", "-f", "/dev/null", "new-session", "-d", "-s", "yours", "sleep", "3600")
		socket, pane, _ := strings.Cut(s.MustTmux("yours", "list-panes", "-t", "=yours:", "-F", "#{socket_path} #{pane_id}"), " ")
		startCld(t, s, "tmux", map[string]string{"TMUX": socket + ",1,0", "TMUX_PANE": pane}, "join", "-s", "elsewhere")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		if messages := shownMessages(s, "cld-elsewhere"); len(messages) != 0 {
			t.Errorf("messages %q where cld's terminal is no pane of yours, want none", messages)
		}
	})
}

// A dead pane - one cld keeps for a failed claude, say - keeps the name of its closed pty, and the
// system hands the name to the next terminal opened. tmux takes a client with $TMUX set on a pty of
// that name for one inside its own pane, when the pane is on the server it attaches to; cld's
// client, with an empty TMUX, attaches. join to a session that runs, that is: one that makes its
// session starts a server of its own, with no dead pane. Not parallel: a terminal another test
// opens could take the name first.
func TestNestsOnADeadPanesPty(t *testing.T) {
	s := sandbox.New(t)
	// Session main, on a server marked as cld marks its own, and a dead pane on its server:
	// remain-on-exit keeps the pane, dead, once false has exited.
	s.MustTmux("cld-main", "-f", "/dev/null", "set", "-s", "@cld", "1", ";", "new-session", "-d", "-s", "cld-main", "sleep", "600")
	s.MustTmux("cld-main", "set", "-g", "remain-on-exit", "on", ";", "new-session", "-d", "-s", "dead", "false")
	sandbox.WaitFor(t, 10*time.Second, "the pane to die", func() bool {
		return s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_dead}") == "1"
	})
	dead := s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_tty}")

	// A pane of another tmux: the terminal writes down its pty and runs join.
	// printf ends the line: uutils' tty (0.8.0) prints the name without a newline.
	env := map[string]string{"TMUX": filepath.Join(s.Root, "elsewhere", "default") + ",1,0"}
	maps.Copy(env, s.Env)
	ttyFile := filepath.Join(s.Root, "tty")
	term := terminal.New(t, "tmux", s)
	term.Start(append([]string{"sh", "-c", `tty=$(tty) && printf '%s\n' "$tty" >"$0" && exec "$@" join -s main`, ttyFile}, s.CldArgv()...), env, s.Work)
	var tty []byte
	sandbox.WaitFor(t, 10*time.Second, "the terminal's pty", func() bool {
		tty, _ = os.ReadFile(ttyFile)
		return strings.HasSuffix(string(tty), "\n")
	})
	if got := strings.TrimSuffix(string(tty), "\n"); got != dead {
		t.Skipf("the terminal got %s rather than the dead pane's %s", got, dead)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld join to attach", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-main"}) || !term.Running()
	})
	if !term.Running() {
		t.Fatalf("cld join on %s failed: %q", dead, term.Output())
	}
}

// In a live pane of one of cld's servers - claude's external editor, say - a session attached
// would show inside a session of cld's, itself or another, both taking C-q: join moves the
// terminal on the pane's session instead (see TestJoinMovesTheTerminal), and where none is
// attached, as here, refuses, whether it would attach, create the session or bring it back, and
// the terminals attached elsewhere stay. cld finds the server through the socket the pane's TMUX
// names, also where TMUX_TMPDIR has changed since. join makes this check before claude --version,
// which the terminal's cld join runs where it starts claude: a claude too old is not what it
// reports here (TestOnlyJoinRunsClaude has no terminal, so cld makes no such check there).
func TestJoinInItsOwnPaneWithNoTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	nested := "cld: no terminal is attached to this session for join to move; run cld join in a terminal (see cld help join)\n"
	moved := filepath.Join(s.Root, "moved")
	if err := os.Mkdir(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, test := range []struct {
		// env is what the pane runs cld with besides the server's environment.
		env  []string
		args []string
		want string
	}{
		{nil, []string{"join", "-s", "a"}, nested},
		{nil, []string{"join", "-s", "b"}, nested},
		{nil, []string{"join", "-s", "c"}, nested},
		{nil, []string{"join", "-s", "c", "--resume", "x"}, nested},
		{nil, []string{"join"}, nested},
		// tmux -L cld-a would look for the socket in the directory TMUX_TMPDIR names now.
		{[]string{"TMUX_TMPDIR=" + moved}, []string{"join", "-s", "a"}, nested},
		{[]string{"CLD_FAKE_CLAUDE_VERSION=2.1.231 (Claude Code)"}, []string{"join", "-s", "d"}, nested},
		{[]string{"CLD_FAKE_CLAUDE_VERSION=2.1.231 (Claude Code)"}, []string{"join", "-s", "d", "--resume", "x"}, nested},
	} {
		// A pane on session a's server runs cld on its own pty, with the TMUX tmux sets for it.
		out := filepath.Join(s.Root, strconv.Itoa(i))
		argv := append(append([]string{"env"}, test.env...), s.CldArgv(test.args...)...)
		s.MustTmux("cld-a", append([]string{"new-session", "-d", "-s", "in-" + strconv.Itoa(i),
			"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, argv...)...)
		command := strings.Join(append(append(slices.Clone(test.env), "cld"), test.args...), " ")
		var code []byte
		sandbox.WaitFor(t, 10*time.Second, command+" to return", func() bool {
			code, _ = os.ReadFile(out + ".code")
			return strings.HasSuffix(string(code), "\n")
		})
		if stderr, _ := os.ReadFile(out + ".err"); string(code) != "1\n" || string(stderr) != test.want {
			t.Errorf("%s: exit %s, stderr %q, want exit 1, stderr %q", command, strings.TrimSpace(string(code)), stderr, test.want)
		}
	}
	if sessions := s.Sessions(); slices.ContainsFunc(sessions, func(session string) bool {
		return strings.HasSuffix(session, "cld-c") || strings.HasSuffix(session, "cld-d") || strings.HasSuffix(session, "cld-0")
	}) {
		t.Errorf("sessions %q, want no cld-0, cld-c or cld-d", sessions)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a", "cld-b"}) {
		t.Errorf("clients attached to %q, want the first ones, to cld-a and cld-b", clients)
	}
}

// The footer of the interactive list, and once Ctrl+X has armed the kill, on a detached row and on
// an attached one; and on a row that has ended, where Ctrl+X arms the forget.
const (
	listHints         = "↑/↓ to navigate · enter to join · ctrl+x to kill · esc to quit"
	killArmed         = "ctrl+x again to kill · esc to keep"
	killArmedAttached = "ctrl+x again to kill and detach its terminal · esc to keep"
	endedHints        = "↑/↓ to navigate · enter to resume · ctrl+x to forget · esc to quit"
	forgetArmed       = "ctrl+x again to forget · esc to keep"
)

// On a terminal, cld list shows cld's sessions on the alternate screen, the first one selected:
// the arrows move the selection, Enter joins the selected session as cld join does, and Esc or
// Ctrl+C leave, printing the plain table. Elsewhere it prints the table, as it did before.
func TestListJoin(t *testing.T) {
	t.Parallel()
	// Enter joins the selected session, as cld join -s NAME does. tmux throws away what the
	// terminal has not read yet as its client starts, so the list's last output - the main screen
	// back, the cursor shown and the session's title - comes before a question the terminal
	// answers once it has read it (see busy terminal). tmux gives the terminal back, after a
	// detach, as it got it: as it was before the list.
	t.Run("enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			listHints)
		// The selected row is drawn in inverse video, and only that one; the footer is dim.
		styled := cells(term.Styled())
		for i, want := range map[int]string{
			0: "[]  NAME  STATE     LAST ACTIVE  DIRECTORY",
			1: "[inverse=7]> a     detached  now          " + s.Work,
			2: "[]  b     detached  now          " + s.Work,
			5: "[intensity=2]" + listHints,
		} {
			if styled[i] != want {
				t.Errorf("line %d is %q, want %q", i+1, styled[i], want)
			}
		}
		term.Keys("Down", "Enter")
		waitScreen(t, term, "probe --name cld-b")
		if title := term.Title(); title != "✳ cld-b" {
			t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
		}
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
		if probes := s.Probes(); len(probes) != 3 {
			t.Errorf("%d claude processes, want 3", len(probes))
		}
		const handOver = "\x1b[?25h\x1b[?1049l" + "\x1b]0;✳ cld-b\a" + "\x1b[c"
		sandbox.WaitFor(t, 10*time.Second, "the main screen, the cursor, the title and the question, in that order", func() bool {
			return bytes.Contains(term.Output(), []byte(handOver))
		})
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// A terminal too busy to answer at once - over a slow link, say - holds the join back until
	// it answers, and its answer does not reach claude as keys: with the terminal frozen as the
	// lookup ends, and thawed a second and a half later, the list becomes tmux only then.
	t.Run("busy terminal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid := list.read(t, "pid")
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Enter")
		lookup.held(t)
		thaw := term.Freeze()
		lookup.release(t)
		time.Sleep(1500 * time.Millisecond)
		if strings.HasPrefix(program(pid), "tmux") {
			t.Error("cld became tmux before the terminal answered")
		}
		thaw()
		waitScreen(t, term, "probe --name cld-b")
		// The terminal answered before it drew claude's screen: a key typed now comes after it.
		term.Keys("z")
		probes["b"].WaitInput(0, "z")
		if answer := attributesAnswer.Find(probes["b"].Input()); answer != nil {
			t.Errorf("claude read the terminal's answer %q as keys", answer)
		}
	})

	// A terminal that does not answer holds the join back five seconds, and no longer.
	t.Run("terminal that does not answer", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		lookup := holdLookup(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid := list.read(t, "pid")
		term.Keys("Enter")
		lookup.held(t)
		thaw := term.Freeze()
		released := time.Now()
		lookup.release(t)
		sandbox.WaitFor(t, 20*time.Second, "cld to become tmux", func() bool { return strings.HasPrefix(program(pid), "tmux") })
		if waited := time.Since(released); waited < 5*time.Second {
			t.Errorf("cld became tmux %v after its lookup, with no answer from the terminal; want five seconds", waited)
		}
		thaw()
		waitScreen(t, term, "probe --name cld-a")
	})

	// An attached row joins too, beside the other terminal, as cld join does, and the footer is
	// the same.
	t.Run("attached", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		other := startCld(t, s, "tmux", nil, "join", "-s", "b")
		s.WaitProbes(2)
		waitClients(t, s, 1)
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     attached  now          "+s.Work,
			"",
			listHints)
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     attached  now          "+s.Work,
			"",
			listHints)
		term.Keys("Enter")
		waitScreen(t, term, "probe --name cld-b")
		waitClients(t, s, 2)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b", "cld-b"}) || !other.Running() {
			t.Errorf("clients attached to %q, want two, to cld-b", clients)
		}
	})

	// An exited row joins, and the terminal shows claude's last words and the hint, as cld join
	// does.
	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		probes["b"].Send("exit 1")
		sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-b", "#{pane_dead}") == "1" })
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     exited    now          "+s.Work,
			"",
			listHints)
		term.Keys("Down", "Enter")
		hint := "claude exited with status 1: cld kill -s b ends the session, C-q d or cld detach -s b detaches"
		waitScreen(t, term, hint)
		if screen := term.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
			t.Errorf("the hint is not on the message line:\n%s", screen)
		}
	})

	// A row reads exited once claude has, whether a terminal is attached or not; Enter joins
	// beside that terminal all the same.
	t.Run("exited and attached", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		other := startCld(t, s, "tmux", nil, "join", "-s", "b")
		s.WaitProbes(2)
		waitClients(t, s, 1)
		for _, probe := range s.Probes() {
			if probe.Argv[1] == "cld-b" {
				probe.Send("exit 1")
			}
		}
		sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-b", "#{pane_dead}") == "1" })
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     exited    now          "+s.Work,
			"",
			listHints)
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     exited    now          "+s.Work,
			"",
			listHints)
		term.Keys("Enter")
		waitScreen(t, term, "claude exited with status 1")
		waitClients(t, s, 2)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b", "cld-b"}) || !other.Running() {
			t.Errorf("clients attached to %q, want two, to cld-b", clients)
		}
	})

	// Esc and Ctrl+C leave, joining nothing: cld exits 0, and the terminal is as it was, with the
	// plain table printed, from the rows the list read. So does Esc typed twice at once, as Alt+Esc
	// comes too, and Esc twice followed at once by a letter: the first Esc stands alone unless a
	// sequence follows the second (see keys that do nothing).
	for _, quit := range []struct {
		name string
		keys func(terminal.Terminal)
	}{
		{"Escape", func(term terminal.Terminal) { term.Keys("Escape") }},
		{"C-c", func(term terminal.Terminal) { term.Keys("C-c") }},
		{"M-Escape", func(term terminal.Terminal) { term.Keys("M-Escape") }},
		// In one write, so that the letter comes within the wait for a lone Esc.
		{"Escape Escape j", func(term terminal.Terminal) { term.Paste("\x1b\x1bj") }},
	} {
		t.Run("quit with "+quit.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			probes := detachedSessions(t, s, "a", "b")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, nil)
			waitLines(t, term,
				"  NAME  STATE     LAST ACTIVE  DIRECTORY",
				"> a     detached  now          "+s.Work,
				"  b     detached  now          "+s.Work,
				"",
				listHints)
			if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
				t.Errorf("modes while the list is open %+v, want the alternate screen and the cursor hidden", modes)
			}
			quit.keys(term)
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
				"a     detached  now          " + s.Work + "\n" +
				"b     detached  now          " + s.Work + "\n"
			waitLines(t, term, strings.Split(strings.TrimSuffix(table, "\n"), "\n")...)
			afterList(t, term, table)
			list.checkRestored(t, term)
			if clients := s.Clients(); len(clients) != 0 {
				t.Errorf("clients attached to %q, want none", clients)
			}
			for name, probe := range probes {
				if !probe.Alive() {
					t.Errorf("claude %s exited", name)
				}
			}
		})
	}

	// The selection stops at the first and the last row, and other keys do nothing: letters, an
	// arrow with Shift, and keys with Alt, which terminals send as Esc and the key - Alt+j, and
	// Alt+Up as the terminals that send any key with Alt that way send it (ESC ESC [ A). Each
	// step ends on a row that a key taken for another would not have left selected.
	t.Run("keys that do nothing", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		selected := func(name string) {
			t.Helper()
			lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
			for _, row := range []string{"a", "b", "c"} {
				marker := " "
				if row == name {
					marker = ">"
				}
				lines = append(lines, marker+" "+row+"     detached  now          "+s.Work)
			}
			waitLines(t, term, append(lines, "", listHints)...)
		}
		selected("a")
		term.Keys("Up", "Down")
		selected("b")
		term.Keys("M-j", "S-Up", "M-Up", "k", "q", "Down")
		selected("c")
		term.Keys("Down", "Up")
		selected("b")
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// The list draws once for a key, not once for each of its bytes: an arrow comes as three. It
	// draws once for keys that come together too, as two typed one at a time may under load: each
	// arrow is typed once the frame for the one before is in the output log.
	t.Run("a frame a key", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		for arrows, row := range []string{"b", "c"} {
			term.Keys("Down")
			// Each frame starts at the top left; the output log trails the screen.
			var frames int
			sandbox.WaitFor(t, 10*time.Second, "the frame with "+row+" selected in the output log", func() bool {
				output := term.Output()
				frames = bytes.Count(output, []byte("\x1b[1;1H"))
				return bytes.Contains(output, []byte("\x1b[7m> "+row+" "))
			})
			if want := arrows + 2; frames != want {
				t.Errorf("%d frames up to the one with %s selected, want %d: the first and one an arrow", frames, row, want)
			}
		}
	})

	// The list reads only the keys it takes: what comes with Ctrl+C, in the same write, stays
	// with the terminal for the program that reads it next.
	t.Run("keys after leaving", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		// Out of raw mode, the terminal holds the keys for a line; dd takes them as they are.
		list := startList(t, s, term, `"$@"; echo $? >"$0.code"; stty raw; dd bs=64 count=1 of="$0.typed" 2>/dev/null`, nil)
		waitScreen(t, term, listHints)
		term.Paste("\x03typed")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		var typed []byte
		sandbox.WaitFor(t, 10*time.Second, "the keys after Ctrl+C to reach dd", func() bool {
			typed, _ = os.ReadFile(string(list) + ".typed")
			return len(typed) > 0
		})
		if string(typed) != "typed" {
			t.Errorf("dd read %q, want %q", typed, "typed")
		}
	})

	// SIGTERM, SIGHUP, SIGINT and SIGQUIT end the list as they end cld, with 128 and the signal's
	// number, but only once the terminal is as it was.
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT} {
		t.Run("signal "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, pidScript, nil)
			waitScreen(t, term, listHints)
			pid, err := strconv.Atoi(list.read(t, "pid"))
			if err != nil {
				t.Fatal(err)
			}
			if err := syscall.Kill(pid, sig); err != nil {
				t.Fatal(err)
			}
			if code, want := list.code(t), strconv.Itoa(128+int(sig)); code != want {
				t.Errorf("exit %s, want %s", code, want)
			}
			list.checkRestored(t, term)
		})
	}

	// A signal cld was started with ignored stays ignored, as under nohup: SIGHUP leaves the list
	// open.
	t.Run("ignored signal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, "trap '' HUP; "+pidScript, nil)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// SIGTSTP - from outside: Ctrl+Z is a key in raw mode - stops cld with the terminal as it was,
	// and once the shell has cld go on (fg), the list takes the terminal again and draws it all.
	// SIGSTOP leaves the list on the screen, where the shell writes over it, and the terminal in
	// raw mode, which the shell may put back to its own (bash does); the list takes the terminal
	// again all the same.
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGSTOP} {
		t.Run("stopped with "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a", "b")
			term := terminal.New(t, "tmux", s)
			list := startJob(t, s, term)
			rows := func(selected string) []string {
				lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
				for _, row := range []string{"a", "b"} {
					marker := " "
					if row == selected {
						marker = ">"
					}
					lines = append(lines, marker+" "+row+"     detached  now          "+s.Work)
				}
				return append(lines, "", listHints)
			}
			waitLines(t, term, rows("a")...)
			pid, err := strconv.Atoi(list.read(t, "pid"))
			if err != nil {
				t.Fatal(err)
			}
			if err := syscall.Kill(pid, sig); err != nil {
				t.Fatal(err)
			}
			// cld stops itself with SIGSTOP for SIGTSTP (see pause in internal/picker).
			if status, want := list.read(t, "stopped"), strconv.Itoa(128+int(syscall.SIGSTOP)); status != want {
				t.Errorf("the shell reports cld stopped with status %s, want %s", status, want)
			}
			if sig == syscall.SIGTSTP {
				if before, during := list.read(t, "before"), list.read(t, "during"); before != during {
					t.Errorf("stty -g while cld is stopped\n%s\nwant as before\n%s", during, before)
				}
				waitScreen(t, term, "the shell's line")
				sandbox.WaitFor(t, 10*time.Second, "the main screen and the cursor while cld is stopped", func() bool {
					modes := term.Modes()
					return !modes.AltScreen && modes.Cursor
				})
			}
			s.WriteFile(string(list)+".go", "")
			waitLines(t, term, rows("a")...)
			if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
				t.Errorf("modes once cld goes on %+v, want the alternate screen and the cursor hidden", modes)
			}
			list.checkRaw(t)
			term.Keys("Down")
			waitLines(t, term, rows("b")...)
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  now          "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
			list.checkRestored(t, term)
		})
	}

	// Where no shell with job control would have cld go on - its process group is its session
	// leader's, an orphaned one - SIGTSTP stops nothing, as without the list: the list puts the
	// terminal back, takes it again and goes on.
	t.Run("SIGTSTP without job control", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, nil)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, syscall.SIGTSTP); err != nil {
			t.Fatal(err)
		}
		const backAndAgain = "\x1b[?25h\x1b[?1049l" + "\x1b[?1049h\x1b[?25l"
		sandbox.WaitFor(t, 10*time.Second, "the terminal put back and taken again", func() bool {
			return bytes.Contains(term.Output(), []byte(backAndAgain))
		})
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		list.checkRestored(t, term)
	})

	// A signal that comes while Enter looks the session up ends cld at once, however long the
	// lookup takes - on a server that hangs, say - joining nothing: the lookup's tmux is killed,
	// and the tab keeps its title. A tmux first on the PATH holds the lookup.
	t.Run("signal at enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		lookup := holdLookup(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, lookup.env)
		waitScreen(t, term, listHints)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		term.Keys("Enter")
		held := lookup.held(t)
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code := list.code(t); code != "143" {
			t.Errorf("exit %s, want 143", code)
		}
		if !gone(held) {
			t.Error("the lookup's tmux outlived cld")
		}
		list.checkRestored(t, term)
		if clients := s.Clients(); len(clients) != 0 {
			t.Errorf("clients attached to %q, want none", clients)
		}
		if title := term.Title(); title == "✳ cld-a" {
			t.Errorf("terminal title %q, for a session not joined", title)
		}
	})

	// Esc and Ctrl+C leave while Enter looks the session up too, printing the table: the lookup's
	// tmux is killed.
	for _, key := range []string{"Escape", "C-c"} {
		t.Run("quit at enter with "+key, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			lookup := holdLookup(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, lookup.env)
			waitScreen(t, term, listHints)
			term.Keys("Enter")
			held := lookup.held(t)
			term.Keys(key)
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			if !gone(held) {
				t.Error("the lookup's tmux outlived cld")
			}
			afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  now          "+s.Work+"\n")
			list.checkRestored(t, term)
			if clients := s.Clients(); len(clients) != 0 {
				t.Errorf("clients attached to %q, want none", clients)
			}
		})
	}

	// While Enter looks the session up, other keys do nothing: Down leaves the selection where it
	// is, and a second Enter looks nothing up, so the first lookup joins its session once it ends.
	// Each key, typed once the frame before it is in the output log, draws a frame, which tells the
	// test that the list has taken it.
	t.Run("keys during the lookup", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "a")
		term := startCld(t, s, "tmux", lookup.env, "list")
		waitScreen(t, term, listHints)
		term.Keys("Enter")
		lookup.held(t)
		waitFrames(t, term, 2)
		term.Keys("Down")
		waitFrames(t, term, 3)
		if row := selectedRow(term); row != "a" {
			t.Errorf("row %q selected during the lookup, want a", row)
		}
		term.Keys("Enter")
		waitFrames(t, term, 4)
		if count := lookup.begun(t); count != 1 {
			t.Errorf("cld-a looked up %d times during the lookup, want once: the second Enter looked it up again", count)
		}
		lookup.release(t)
		waitScreen(t, term, "probe --name cld-a")
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a"}) {
			t.Errorf("clients attached to %q, want one, to cld-a", clients)
		}
		// join looks the session up again, under the record's lock, once the list has handed over.
		if count := lookup.begun(t); count != 2 {
			t.Errorf("cld-a looked up %d times, want twice: Enter's, then join's", count)
		}
	})

	// A session ended when Enter is pressed - killed elsewhere - is brought back, as cld join
	// brings it back: the lookup finds its entry in cld's record, and once the list has handed the
	// terminal over, claude resumes the session's conversation - here by the session's name, as the
	// entry has no ID.
	t.Run("ended", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		term.Keys("Down", "Enter")
		waitScreen(t, term, "probe --name cld-b")
		waitClients(t, s, 1)
		resumed := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", s.Work), "--resume", "cld-b"}
		if !slices.ContainsFunc(s.Probes(), func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }) {
			t.Errorf("no claude started with %q", resumed)
		}
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
	})

	// A session gone when Enter is pressed - killed elsewhere, and forgotten - stays unjoined: the
	// footer says so, and the list, still in raw mode, reads the sessions again and selects the row
	// that took its place.
	t.Run("gone", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		forget(t, s, "b")
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> c     detached  now          "+s.Work,
			"",
			"no session 'b'")
		list.checkRaw(t)
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// The selection follows its session's place rather than its row's number: with the rows above
	// it gone too, it goes to the next row the list showed, which is now the first...
	t.Run("gone with the rows above", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c", "d")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		for _, name := range []string{"a", "b"} {
			if result := s.RunCld(nil, "kill", "-s", name); result.Code != 0 {
				t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
			}
			forget(t, s, name)
		}
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> c     detached  now          "+s.Work,
			"  d     detached  now          "+s.Work,
			"",
			"no session 'b'")
	})

	// ... and with no row after it left, to the one above, although a session made meanwhile now
	// has its row's number.
	t.Run("gone from the last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-s", "c"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		forget(t, s, "c")
		detachedSessions(t, s, "z")
		term.Keys("Down", "Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     detached  now          "+s.Work,
			"  z     detached  now          "+s.Work,
			"",
			"no session 'c'")
	})

	// A session whose server runs on without it - claude exited, and a tmux session it made keeps
	// the server running - stays unjoined too, as cld join refuses it, and shows as ended.
	t.Run("lingering server", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		probes["b"].Send("tmux new-session -d -s side sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
			return slices.Contains(s.Sessions(), "cld-b/side")
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		probes["b"].Send("exit")
		sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
			return !slices.Contains(s.Sessions(), "cld-b")
		})
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     ended     -            "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			"session 'b' has ended, but its tmux server still runs")
		if clients := s.Clients(); len(clients) != 0 {
			t.Errorf("clients attached to %q, want none", clients)
		}
	})

	// When the sessions cannot be read again after a failed Enter, the list says why as well, and
	// keeps its rows. A tmux first on the PATH fails to read them once the test says so.
	t.Run("read again fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		broken := filepath.Join(s.Root, "broken")
		env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
			"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
			"esac\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		forget(t, s, "b")
		s.WriteFile(broken, "")
		term.Keys("Down", "Enter")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			"no session 'b' · lost the server")
	})

	// With its last row gone, the list shows that there are no sessions, under its header, and
	// leaving prints nothing.
	t.Run("last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		if result := s.RunCld(nil, "kill", "-s", "a"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		forget(t, s, "a")
		// cld kill ends the server, which takes a moment to exit: waiting for it keeps Enter's lookup
		// from reaching it as it goes (see docs/design/findings/tmux-sessions.md).
		sandbox.WaitFor(t, 10*time.Second, "a's server to exit", func() bool {
			_, err := s.Tmux("cld-a", "list-sessions")
			return err != nil && strings.Contains(err.Error(), "no server running")
		})
		term.Keys("Enter")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "no sessions", "", "no session 'a'")
		term.Keys("Down")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "no sessions", "", "esc to quit")
		// Enter has nothing to join.
		term.Keys("Enter")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "no sessions", "", "esc to quit")
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "")
		waitLines(t, term)
	})

	// With no sessions, there is nothing to pick: cld list prints nothing and exits 0 at once.
	t.Run("no sessions", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
			t.Errorf("cld opened the alternate screen: %q", output)
		}
		waitLines(t, term)
	})

	// Every line is cut at the terminal's width, counted in cells, so that none wraps: a wide
	// character (日) where a row reaches the edge is left out whole, an é takes one cell, and so
	// do the arrows and dots of the footer. Leaving prints the whole directory.
	t.Run("narrow", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		// The row shows 29 cells of the directory, so it lives outside the sandbox, where its é
		// shows: /tmp/éN, or /private/tmp/éN on macOS. Its first 日 takes the row's cells 60 and 61.
		const prefix = "> a     attached  now          "
		base := shortDirectory(t, "/tmp/é")
		shown := base + "/" + strings.Repeat("_", 59-len(prefix)-utf8.RuneCountInString(base+"/"))
		dir := shown + "日本日本"
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		startCldIn(t, s, "tmux", dir, nil, "join", "-s", "a")
		s.WaitProbes(1)
		waitClients(t, s, 1)
		detachedSessions(t, s, "b")
		term := terminal.New(t, "tmux", s)
		term.Resize(60, 40)
		list := startList(t, s, term, listScript, nil)
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			prefix+shown,
			cutTo("  b     detached  now          "+s.Work, 60),
			"",
			footerIn(listHints, 60))
		term.Keys("Down")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     attached  now          "+shown,
			cutTo("> b     detached  now          "+s.Work, 60),
			"",
			footerIn(listHints, 60))
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
			"a     attached  now          " + dir + "\n" +
			"b     detached  now          " + s.Work + "\n"
		afterList(t, term, table)
	})

	// A terminal too short for every row keeps the selected one in view, with the header and the
	// footer.
	t.Run("short", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c", "d")
		term := terminal.New(t, "tmux", s)
		term.Resize(120, 6)
		term.Start(s.CldArgv("list"), s.Env, s.Work)
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			listHints)
		term.Keys("Down", "Down", "Down")
		scrolled := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  b     detached  now          " + s.Work,
			"  c     detached  now          " + s.Work,
			"> d     detached  now          " + s.Work,
			"",
			listHints,
		}
		waitLines(t, term, scrolled...)
		// Back up, the rows scroll back with the selection.
		term.Keys("Up", "Up", "Up")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+s.Work,
			"  b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			listHints)
		// A taller terminal shows the rows scrolled out above, now that they fit.
		term.Keys("Down", "Down", "Down")
		waitLines(t, term, scrolled...)
		term.Resize(120, 10)
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"  b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"> d     detached  now          "+s.Work,
			"",
			listHints)
	})

	// A resize redraws the list at the new size. It also takes lines away, so that a list still
	// drawing for the old size would push its footer off the screen: a line wider than the
	// terminal does not show as such, since the next line drawn clears what it wrapped onto.
	t.Run("resize", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		long := filepath.Join(s.Work, "a-directory-longer-than-thirty-cells")
		if err := os.Mkdir(long, 0o755); err != nil {
			t.Fatal(err)
		}
		probes["a"].Send("cd " + long)
		sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
			return s.Format("cld-a", "#{pane_current_path}") == long
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+long,
			"  b     detached  now          "+s.Work,
			"",
			listHints)
		// Cut at 30 cells, the screen shows no spaces at the end of a line.
		cutTo30 := func(line string) string { return strings.TrimRight(cutTo(line, 30), " ") }
		term.Resize(30, 5)
		waitLines(t, term,
			cutTo30("  NAME  STATE     LAST ACTIVE  DIRECTORY"),
			cutTo30("> a     detached  now          "+long),
			cutTo30("  b     detached  now          "+s.Work),
			"",
			footerIn(listHints, 30))
		term.Keys("Down")
		waitLines(t, term,
			cutTo30("  NAME  STATE     LAST ACTIVE  DIRECTORY"),
			cutTo30("  a     detached  now          "+long),
			cutTo30("> b     detached  now          "+s.Work),
			"",
			footerIn(listHints, 30))
	})

	// A control character in a directory shows as "?", so that none moves the cursor or changes
	// the terminal: here an ESC would clear the screen. tmux 3.7c passes them on as they are to
	// a UTF-8 client - cld's -u, or the sandbox's LANG for s.Format.
	t.Run("control characters", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a")
		dir := filepath.Join(s.Work, "e\x01f\x1b[2Jg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		probes["a"].Send("cd " + dir)
		var reported string
		sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
			reported = s.Format("cld-a", "#{pane_current_path}")
			return reported != s.Work
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          "+strings.NewReplacer("\x01", "?", "\x1b", "?").Replace(reported),
			"",
			listHints)
	})

	// The list keeps arrows working after a program left application cursor keys on (CSI ?1h),
	// when the terminal sends ESC O A and ESC O B for them.
	t.Run("application cursor keys", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		term.Start(append([]string{"sh", "-c", `printf '\033[?1h' && exec "$@"`, "sh"}, s.CldArgv("list")...), s.Env, s.Work)
		waitScreen(t, term, listHints)
		term.Keys("Down", "Enter")
		waitScreen(t, term, "probe --name cld-b")
	})

	// In a live pane of one of cld's servers - a shell in a window of the session, say - a session
	// attached would show inside a session of cld's: Enter moves the terminal on the pane's session
	// to the session picked, as join does there, and the list exits 0. The session left runs on.
	t.Run("own pane", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "b")
		term := startCld(t, s, "tmux", nil, "join", "-s", "a")
		waitScreen(t, term, "probe --name cld-a")
		// A window of session a, which the terminal shows; it runs cld list with the TMUX tmux
		// sets for it.
		out := filepath.Join(s.Root, "own")
		s.MustTmux("cld-a", append([]string{"new-window", "-t", "=cld-a:", "sh", "-c", `"$@"; echo $? >"$0.code"`, out}, s.CldArgv("list")...)...)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		term.Keys("Enter")
		waitScreen(t, term, "probe --name cld-b")
		var code []byte
		sandbox.WaitFor(t, 10*time.Second, "cld list to return", func() bool {
			code, _ = os.ReadFile(out + ".code")
			return strings.HasSuffix(string(code), "\n")
		})
		if string(code) != "0\n" {
			t.Errorf("exit %s, want 0", strings.TrimSpace(string(code)))
		}
		if clients, sessions := s.Clients(), s.Sessions(); !slices.Equal(clients, []string{"cld-b"}) || !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
			t.Errorf("clients attached to %q, sessions %q; want the terminal on cld-b, and both sessions", clients, sessions)
		}
	})

	// Output that goes to a pipe, input that is not the terminal, a terminal that cannot move the
	// cursor, and a job in the background get the plain table, with no key typed.
	t.Run("not a terminal", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		table := []string{
			"NAME  STATE     LAST ACTIVE  DIRECTORY",
			"a     detached  now          " + s.Work,
			"b     detached  now          " + s.Work,
		}
		for _, test := range []struct {
			name, script string
			extra        map[string]string
		}{
			{"a pipe", `{ "$@"; echo $? >"$0.code"; } | cat`, nil},
			{"stdin from /dev/null", `"$@" </dev/null; echo $? >"$0.code"`, nil},
			{"TERM=dumb", `"$@"; echo $? >"$0.code"`, map[string]string{"TERM": "dumb"}},
			{"TERM unset", `env -u TERM "$@"; echo $? >"$0.code"`, nil},
			// It would stop (SIGTTOU) as it set the terminal up. Job control puts it in a process
			// group of its own, and goes off again before it ends: bash 3.2, macOS's sh, reports a
			// job's end on the terminal while job control is on, in a script too.
			{"a background job", `set -m; "$@" & set +m; wait $!; echo $? >"$0.code"`, nil},
		} {
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, test.script, test.extra)
			if code := list.code(t); code != "0" {
				t.Errorf("%s: exit %s, want 0", test.name, code)
			}
			waitLines(t, term, table...)
			if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
				t.Errorf("%s: cld opened the alternate screen: %q", test.name, output)
			}
		}
	})
}

// In the interactive list, Ctrl+X arms the kill of the selected session and a second Ctrl+X
// within two seconds kills it, as cld kill -s NAME does; Esc keeps it. The list then reads the
// sessions again and stays open, the selection on the row that took the killed row's place.
func TestListKill(t *testing.T) {
	t.Parallel()
	// A terminal attached to the session is detached and left clean, as by cld kill, and the
	// armed kill's footer says so first; the other sessions carry on. The killed session stays in
	// its place, selected, as one that has ended.
	t.Run("kill", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "c")
		other := startCld(t, s, "tmux", nil, "join", "-s", "b")
		waitClients(t, s, 1)
		for _, probe := range s.WaitProbes(3) {
			if probe.Argv[1] == "cld-b" {
				probes["b"] = probe
			}
		}
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		rows := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          " + s.Work,
			"> b     attached  now          " + s.Work,
			"  c     detached  now          " + s.Work,
			"",
		}
		waitScreen(t, term, listHints)
		term.Keys("Down")
		waitLines(t, term, append(rows, listHints)...)
		armThen(t, term, func() {
			waitLines(t, term, append(rows, killArmedAttached)...)
			if footer := cells(term.Styled())[5]; footer != "[intensity=2]"+killArmedAttached {
				t.Errorf("the footer is %q, want it dim", footer)
			}
			if !probes["b"].Alive() || !other.Running() {
				t.Error("the first Ctrl+X killed b")
			}
		}, "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     ended     -            "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			endedHints)
		sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool { return !probes["b"].Alive() })
		sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool { return !other.Running() })
		if modes := other.Modes(); modes.AltScreen || modes.Mouse {
			t.Errorf("modes of b's terminal after the kill %+v, want none", modes)
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
		}
		for _, name := range []string{"a", "c"} {
			if !probes[name].Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
		list.checkRaw(t)
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  now          "+s.Work+"\n"+"b     ended     -            "+s.Work+"\n"+"c     detached  now          "+s.Work+"\n")
		list.checkRestored(t, term)
	})

	// Esc disarms the kill and does nothing more: the list stays open, and the next Ctrl+X arms
	// the kill again.
	t.Run("esc", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		rows := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          " + s.Work,
			"  b     detached  now          " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		armThen(t, term, func() { waitLines(t, term, append(rows, killArmed)...) }, "Escape")
		waitLines(t, term, append(rows, listHints)...)
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		if list.exited() {
			t.Error("the list closed")
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
	})

	// Two seconds after the first Ctrl+X, the kill disarms; a Ctrl+X after that arms it again.
	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := startCld(t, s, "tmux", nil, "list")
		rows := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          " + s.Work,
			"  b     detached  now          " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		pressed := time.Now()
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		waitLines(t, term, append(rows, listHints)...)
		// Seeing the footer change takes a while: two seconds more is slack for load.
		if waited := time.Since(pressed); waited < 2*time.Second || waited > 4*time.Second {
			t.Errorf("the kill disarmed %v after Ctrl+X, want two seconds", waited)
		}
		term.Keys("C-x")
		waitLines(t, term, append(rows, killArmed)...)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
	})

	// A key that has come by the end of the two seconds counts as typed within them, although cld
	// has not read it yet - held up, stopped here, over their end: Esc keeps the session and the
	// list open, and a second Ctrl+X kills it. Once cld goes on, the key and the end of the wait
	// are there to take together, and cld may take either first: Esc is typed three times.
	t.Run("read late", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, pidScript, nil)
		pid, err := strconv.Atoi(list.read(t, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		rows := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     detached  now          " + s.Work,
			"  b     detached  now          " + s.Work,
			"",
		}
		waitLines(t, term, append(rows, listHints)...)
		// readLate arms the kill, stops cld, types key and has cld go on once the two seconds are
		// over and key is there to read. When cld stopped too late to be sure that the two seconds
		// were not over by then, it types nothing, and reports false once the kill has disarmed;
		// the third time, the test is skipped.
		late := 0
		readLate := func(key string) bool {
			t.Helper()
			pressed := time.Now()
			term.Keys("C-x")
			waitLines(t, term, append(rows, killArmed)...)
			armed := time.Now()
			if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
				t.Fatal(err)
			}
			sandbox.WaitFor(t, 10*time.Second, "cld to stop", func() bool { return isStopped(pid) })
			inTime := time.Since(pressed) < 2*time.Second
			if inTime {
				term.Keys(key)
				sandbox.WaitFor(t, 10*time.Second, "the key to reach the terminal", func() bool { return list.unread(t) })
				time.Sleep(time.Until(armed.Add(2200 * time.Millisecond)))
			}
			if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
				t.Fatal(err)
			}
			if !inTime {
				waitLines(t, term, append(rows, listHints)...)
				if late++; late == 3 {
					t.Skip("cld stopped two seconds after Ctrl+X or later three times: too loaded to tell")
				}
			}
			return inTime
		}
		for kept := 0; kept < 3; {
			if !readLate("Escape") {
				continue
			}
			sandbox.WaitFor(t, 10*time.Second, "the list to take Esc", func() bool {
				return list.exited() || strings.Contains(term.Screen(), listHints)
			})
			if list.exited() {
				t.Fatalf("Esc %d of 3, read late, closed the list", kept+1)
			}
			waitLines(t, term, append(rows, listHints)...)
			kept++
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
			t.Errorf("sessions %q, claude a alive: %v; want both, a alive", sessions, probes["a"].Alive())
		}
		for !readLate("C-x") {
		}
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "> a     ended     -            "+s.Work, "  b     detached  now          "+s.Work, "", endedHints)
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     ended     -            "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
		if !probes["b"].Alive() {
			t.Error("claude b exited")
		}
	})

	// Any other key disarms the kill, and then does what it does: an arrow moves the selection, a
	// letter does nothing more, and Ctrl+C leaves. A Ctrl+X right after the key, within the two
	// seconds, then arms the kill again rather than kill.
	t.Run("other keys", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		lines := func(selected, footer string) []string {
			lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
			for _, row := range []string{"a", "b"} {
				marker := " "
				if row == selected {
					marker = ">"
				}
				lines = append(lines, marker+" "+row+"     detached  now          "+s.Work)
			}
			return append(lines, "", footer)
		}
		waitLines(t, term, lines("a", listHints)...)
		term.Keys("C-x")
		waitLines(t, term, lines("a", killArmed)...)
		term.Keys("Down", "C-x")
		waitLines(t, term, lines("b", killArmed)...)
		term.Keys("k", "C-x")
		waitLines(t, term, lines("b", killArmed)...)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
			t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
		}
		term.Keys("C-c")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"a     detached  now          "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
		for name, probe := range probes {
			if !probe.Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// Enter, once Ctrl+X has armed the kill, disarms it and joins the selected session, which the
	// kill leaves alone.
	t.Run("enter", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "Enter")
		waitScreen(t, term, "probe --name cld-b")
		waitClients(t, s, 1)
		if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
			t.Errorf("clients attached to %q, want one, to cld-b", clients)
		}
		term.Keys("C-q", "d")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
			t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
		}
		for name, probe := range probes {
			if !probe.Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// A key held down repeats: the terminal types it again after a delay, and then many times a
	// second. After the Ctrl+X that killed, Ctrl+X does nothing until none has come for a second,
	// so that the repeats of that Ctrl+X, held a little too long, do not arm and forget the session
	// it killed, which stays on its row as one that has ended; another key ends the wait at once.
	t.Run("held down", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		term := startCld(t, s, "tmux", nil, "list")
		header := "  NAME  STATE     LAST ACTIVE  DIRECTORY"
		row := func(marker, name string) string { return marker + " " + name + "     detached  now          " + s.Work }
		ended := func(marker, name string) string { return marker + " " + name + "     ended     -            " + s.Work }
		waitScreen(t, term, listHints)
		term.Keys("C-x")
		waitScreen(t, term, killArmed)
		// The second Ctrl+X, held down: it repeats after half a second, a common delay, then 20
		// times a second for a second, past the second after the kill. Two that reach cld a second
		// apart are two presses rather than a key held down, and end the wait. The terminal times
		// the keys, but the time it took beyond their waits may have come between any two: where
		// that may have made a second, the case cannot tell, and is skipped.
		holding := time.Now()
		term.Hold("C-x", 500*time.Millisecond, 50*time.Millisecond, 20)
		if over := time.Since(holding) - 500*time.Millisecond - 20*50*time.Millisecond; over >= 400*time.Millisecond {
			t.Skipf("typing Ctrl+X held down took %v beyond its waits: two may have reached cld a second apart", over.Round(time.Millisecond))
		}
		// The repeats have neither armed nor forgotten a, which has ended, once the kill has. Down,
		// after them, moves the selection from a to b, and ends the wait: a Ctrl+X right after it
		// arms the kill, and a second one kills b.
		waitLines(t, term, header, ended(">", "a"), row(" ", "b"), row(" ", "c"), "", endedHints)
		term.Keys("Down")
		armThen(t, term, func() {
			waitLines(t, term, header, ended(" ", "a"), row(">", "b"), row(" ", "c"), "", killArmed)
			sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b", "cld-c"}) {
				t.Errorf("sessions %q, want [cld-b cld-c]", sessions)
			}
			for _, name := range []string{"b", "c"} {
				if !probes[name].Alive() {
					t.Errorf("claude %s exited", name)
				}
			}
		}, "C-x")
		waitLines(t, term, header, ended(" ", "a"), ended(">", "b"), row(" ", "c"), "", endedHints)
		// A second without Ctrl+X ends the wait too; the test leaves it half a second more.
		time.Sleep(1500 * time.Millisecond)
		term.Keys("C-x")
		waitLines(t, term, header, ended(" ", "a"), ended(">", "b"), row(" ", "c"), "", forgetArmed)
		if !probes["c"].Alive() {
			t.Error("claude c exited")
		}
	})

	// Killing the last session leaves it on its row, as one that has ended; forgetting it then
	// leaves the list with no sessions, as when its last row has gone elsewhere: the header over
	// "no sessions", and leaving prints nothing.
	t.Run("last row", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "> a     ended     -            "+s.Work, "", endedHints)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		// A letter first ends the wait after a kill (see held down).
		term.Keys("k", "C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "no sessions", "", "esc to quit")
		// Ctrl+X has nothing to arm, or to forget.
		term.Keys("k", "C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "no sessions", "", "esc to quit")
		if list.exited() {
			t.Error("the list closed")
		}
		term.Keys("Escape")
		if code := list.code(t); code != "0" {
			t.Errorf("exit %s, want 0", code)
		}
		afterList(t, term, "")
	})

	// An exited session is killed the same way, and the terminal still attached to it is
	// detached.
	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		failed := startCld(t, s, "tmux", nil, "join", "-s", "b")
		waitClients(t, s, 1)
		for _, probe := range s.WaitProbes(2) {
			if probe.Argv[1] == "cld-b" {
				probe.Send("exit 1")
			}
		}
		waitScreen(t, failed, "claude exited with status 1: cld kill -s b ends the session, C-q d or cld detach -s b detaches")
		term := startCld(t, s, "tmux", nil, "list")
		rows := []string{
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          " + s.Work,
			"> b     exited    now          " + s.Work,
			"",
		}
		waitScreen(t, term, listHints)
		term.Keys("Down")
		waitLines(t, term, append(rows, listHints)...)
		armThen(t, term, func() { waitLines(t, term, append(rows, killArmedAttached)...) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     ended     -            "+s.Work,
			"",
			endedHints)
		sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool { return !failed.Running() })
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
			t.Errorf("sessions %q, want [cld-a]", sessions)
		}
	})

	// A session ended by the second Ctrl+X - killed elsewhere here - is reported, and the list
	// reads the sessions again, where it has ended, and stays open. The steps outside come before
	// the first Ctrl+X, so that they need not fit in the two seconds.
	t.Run("gone", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     ended     -            "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			"session 'b' has ended")
		list.checkRaw(t)
		if list.exited() {
			t.Error("the list closed")
		}
		for _, name := range []string{"a", "c"} {
			if !probes[name].Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	})

	// A session ended and made again under the same name is another session, with another
	// claude: the kill ends nothing, as for a session gone, and the list shows the new one.
	t.Run("replaced", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		old := detachedSessions(t, s, "a", "b", "c")["b"]
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, nil)
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
			t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
		}
		sandbox.WaitFor(t, 10*time.Second, "the first claude b to exit", func() bool { return !old.Alive() })
		detachedSessions(t, s, "b")
		var replaced *sandbox.Probe
		for _, probe := range s.Probes() {
			if probe.Argv[1] == "cld-b" && probe.PID != old.PID {
				replaced = probe
			}
		}
		if replaced == nil {
			t.Fatal("no second claude b")
		}
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     detached  now          "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			"no session 'b'")
		if !replaced.Alive() {
			t.Error("the second claude b exited")
		}
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-b cld-c]", sessions)
		}
		if list.exited() {
			t.Error("the list closed")
		}
	})

	// A session whose server runs on without it - claude exited, and the tmux session it made keeps
	// the server running - is ended with the server, as cld kill ends it, although no pane is left
	// to check against the pids the list read. Its row stays, selected, as one that has ended.
	t.Run("lingering server", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b", "c")
		probes["b"].Send("tmux new-session -d -s side sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
			return slices.Contains(s.Sessions(), "cld-b/side")
		})
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		probes["b"].Send("exit")
		sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
			return !slices.Contains(s.Sessions(), "cld-b")
		})
		armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> b     ended     -            "+s.Work,
			"  c     detached  now          "+s.Work,
			"",
			endedHints)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
		}
	})

	// A kill-session that fails leaves the session listed, with what tmux said in the footer, or
	// its exit status when it said nothing. A tmux first on the PATH fails it.
	t.Run("kill-session fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		silent := filepath.Join(s.Root, "silent")
		env := wrapTmux(t, s, "case \"$*\" in *kill-session*)\n"+
			"\tif [ -e '"+silent+"' ]; then exit 5; fi\n"+
			"\techo 'tmux: cannot kill' >&2; exit 5 ;;\n"+
			"esac\n")
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, listScript, env)
		waitScreen(t, term, listHints)
		row := "> a     detached  now          " + s.Work
		term.Keys("C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", row, "", "tmux: cannot kill")
		s.WriteFile(silent, "")
		// A letter first ends the wait after a kill (see held down).
		term.Keys("k", "C-x", "C-x")
		waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", row, "", "tmux kill-session: exit status 5")
		list.checkRaw(t)
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
			t.Errorf("sessions %q, want [cld-a]", sessions)
		}
	})

	// When the sessions cannot be read again after a kill, the list says why and keeps its rows,
	// but for the one it killed. A tmux first on the PATH fails to read them once the test says so.
	t.Run("read again fails", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b", "c")
		broken := filepath.Join(s.Root, "broken")
		env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
			"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
			"esac\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		term.Keys("Down")
		sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
		s.WriteFile(broken, "")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"  a     detached  now          "+s.Work,
			"> c     detached  now          "+s.Work,
			"",
			"lost the server")
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
		}
	})

	// The killed session's server exits after the kill, and a read of the sessions that meets it
	// exiting is told now and then that the server exited unexpectedly (see
	// docs/design/findings/tmux-sessions.md): the list passes over that server, as cld list does. A
	// tmux first on the PATH leaves the killed session's server running, taking connections as an
	// exiting one still does for a moment - where it has gone, the list runs no tmux there (see
	// TestStaleSocketsRunNoTmux) - and fails the first read of it after the kill as tmux does then.
	t.Run("server exiting", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a", "b")
		exiting := filepath.Join(s.Root, "exiting")
		env := wrapTmux(t, s, "if [ -e '"+exiting+"' ]; then case \"$*\" in\n"+
			"\t*'-L cld-a '*' kill-session -t =cld-a '*) exit 0 ;;\n"+
			"\t*'-L cld-a list-sessions '*'#{?pane_dead,exited'*) rm '"+exiting+"'; echo 'server exited unexpectedly' >&2; exit 1 ;;\n"+
			"esac; fi\n")
		term := startCld(t, s, "tmux", env, "list")
		waitScreen(t, term, listHints)
		s.WriteFile(exiting, "")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     ended     -            "+s.Work,
			"  b     detached  now          "+s.Work,
			"",
			endedHints)
		if _, err := os.Stat(exiting); err == nil {
			t.Error("the list did not read the killed session's server after the kill")
		}
	})

	// While the kill runs, keys other than those that leave do nothing: Down leaves the selection
	// where it is, and Ctrl+X arms nothing - Down has ended the wait after the kill (see held
	// down), which would keep Ctrl+X from doing anything too. The list draws a frame once it has
	// taken the keys that have come, which tells the test that it has taken them: the two Ctrl+X
	// that start the kill are pasted, in one write, and draw one frame, and each key after them is
	// typed once the frame before it is in the output log. Typed one at a time, the two Ctrl+X may
	// reach the list together under load, or over two seconds apart. A tmux first on the PATH
	// holds the kill's lookup.
	t.Run("keys during the kill", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		lookup := holdLookup(t, s, "a")
		term := startCld(t, s, "tmux", lookup.env, "list")
		waitScreen(t, term, listHints)
		term.Paste("\x18\x18")
		lookup.held(t)
		waitFrames(t, term, 2)
		term.Keys("Down")
		waitFrames(t, term, 3)
		term.Keys("C-x")
		waitFrames(t, term, 4)
		if row := selectedRow(term); row != "a" {
			t.Errorf("row %q selected during the kill, want a", row)
		}
		if strings.Contains(term.Screen(), killArmed) {
			t.Error("Ctrl+X armed the kill while the kill ran")
		}
		lookup.release(t)
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     ended     -            "+s.Work,
			"  b     detached  now          "+s.Work,
			"",
			endedHints)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		if !probes["b"].Alive() {
			t.Error("claude b exited")
		}
	})

	// Esc leaves while the kill runs, printing the table: the kill's tmux is killed, and the table
	// shows what it did by then. Held in its lookup or in kill-session, the kill ends nothing;
	// held as it reads the sessions again, it has ended the session, which the table leaves out.
	// A tmux first on the PATH holds the step, once the list has read the sessions it opens with.
	for _, step := range []struct {
		name, pattern string
		ends          bool
	}{
		{"lookup", "*'#{==:#{session_name},cld-a},'*' -F #{session_name} #{W:#{P:#{pane_pid} }}\t#{@cld-home}'", false},
		{"kill-session", "*kill-session*", false},
		{"read", "*'#{?pane_dead,exited'*", true},
	} {
		t.Run("quit during the kill's "+step.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			probes := detachedSessions(t, s, "a", "b")
			hold := holdTmux(t, s, "the kill's "+step.name, step.pattern)
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, hold.env)
			waitScreen(t, term, listHints)
			hold.start(t)
			term.Keys("C-x", "C-x")
			held := hold.held(t)
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			if !gone(held) {
				t.Error("the kill's tmux outlived cld")
			}
			table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  now          " + s.Work + "\n" + "b     detached  now          " + s.Work + "\n"
			sessions := []string{"cld-a", "cld-b"}
			if step.ends {
				table, sessions = "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+"b     detached  now          "+s.Work+"\n", []string{"cld-b"}
			}
			afterList(t, term, table)
			list.checkRestored(t, term)
			if got := s.Sessions(); !slices.Equal(got, sessions) {
				t.Errorf("sessions %q, want %q", got, sessions)
			}
			if probes["a"].Alive() == step.ends {
				t.Errorf("claude a alive: %v, want %v", !step.ends, step.ends)
			}
		})
	}

	// The kill goes by the pids of all the session's panes, as tmux reports only the active one's
	// for a session: claude's window split by hand, its other pane selected since the list read
	// the sessions, is still the session on the row.
	t.Run("split window", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		probes := detachedSessions(t, s, "a", "b")
		term := startCld(t, s, "tmux", nil, "list")
		waitScreen(t, term, listHints)
		s.MustTmux("cld-a", "split-window", "-d", "-t", "=cld-a:", "sleep", "600")
		s.MustTmux("cld-a", "select-pane", "-t", "=cld-a:.1")
		term.Keys("C-x", "C-x")
		waitLines(t, term,
			"  NAME  STATE     LAST ACTIVE  DIRECTORY",
			"> a     ended     -            "+s.Work,
			"  b     detached  now          "+s.Work,
			"",
			endedHints)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
			t.Errorf("sessions %q, want [cld-b]", sessions)
		}
	})

	// The hints take 62 cells, on a row with a terminal attached as on another: in 61 columns the
	// arrows' hint goes, so that the others, esc to quit last, show whole. The armed kill's on
	// such a row, 58 cells, shows whole too.
	t.Run("61 columns", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		detachedSessions(t, s, "a")
		startCld(t, s, "tmux", nil, "join", "-s", "b")
		waitClients(t, s, 1)
		s.WaitProbes(2)
		term := terminal.New(t, "tmux", s)
		term.Resize(61, 24)
		term.Start(s.CldArgv("list"), s.Env, s.Work)
		rows := func(selected string) []string {
			lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
			for _, row := range []string{"a     detached  now          ", "b     attached  now          "} {
				marker := " "
				if row[:1] == selected {
					marker = ">"
				}
				lines = append(lines, cutTo(marker+" "+row+s.Work, 61))
			}
			return append(lines, "")
		}
		hints := "enter to join · ctrl+x to kill · esc to quit"
		waitLines(t, term, append(rows("a"), hints)...)
		term.Keys("Down")
		waitLines(t, term, append(rows("b"), hints)...)
		term.Keys("C-x")
		waitLines(t, term, append(rows("b"), killArmedAttached)...)
	})
}

// A session cld join --resume made is a session like any other in the interactive list: Enter
// joins it, and Ctrl+X twice kills it with its server, which leaves it as one that has ended. After
// such a kill - by mistake, say - cld join -n NAME -s SUFFIX brings its conversation back in a new
// session.
func TestListResumedSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	resumed := startCld(t, s, "tmux", nil, "join", "-s", "b", "--resume", "cld-b")
	sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-b", func() bool {
		return slices.Contains(s.Clients(), "cld-b")
	})
	resumed.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !resumed.Running() })
	argv := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", s.Work), "--resume", "cld-b"}
	claude := func(other *sandbox.Probe) *sandbox.Probe {
		for _, probe := range s.Probes() {
			if slices.Equal(probe.Argv, argv) && (other == nil || probe.PID != other.PID) {
				return probe
			}
		}
		return nil
	}
	var b *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude b to start", func() bool { b = claude(nil); return b != nil })
	rows := func(selected string) []string {
		lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
		for _, name := range []string{"a", "b"} {
			mark := " "
			if name == selected {
				mark = ">"
			}
			lines = append(lines, mark+" "+name+"     detached  now          "+s.Work)
		}
		return append(lines, "")
	}

	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(rows("a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(rows("b"), listHints)...)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want one, to cld-b", clients)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("join: exit %s, want 0", code)
	}

	term = terminal.New(t, "tmux", s)
	list = startList(t, s, term, listScript, nil)
	waitLines(t, term, append(rows("a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(rows("b"), listHints)...)
	armThen(t, term, func() { waitLines(t, term, append(rows("b"), killArmed)...) }, "C-x")
	waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY", "  a     detached  now          "+s.Work, "> b     ended     -            "+s.Work, "", endedHints)
	sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool { return !b.Alive() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q after the kill, want [cld-a]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("kill: exit %s, want 0", code)
	}

	startCld(t, s, "tmux", nil, "join", "-s", "b")
	sandbox.WaitFor(t, 10*time.Second, "claude b to resume again", func() bool { return claude(b) != nil })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// program is the name of the program process pid runs: from /proc where there is one, which is
// quicker than ps. A tmux client may name itself "tmux: client" there (prctl in tmux's
// setproctitle, where the system has no setproctitle of its own).
func program(pid string) string {
	if name, err := os.ReadFile("/proc/" + pid + "/comm"); err == nil {
		return strings.TrimSpace(string(name))
	}
	name, _ := exec.Command("ps", "-o", "comm=", "-p", pid).Output()
	return filepath.Base(strings.TrimSpace(string(name)))
}

// isStopped reports whether process pid is stopped, every thread of it: from /proc where there is
// one, or else from ps.
func isStopped(pid int) bool {
	if tasks, _ := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/task/*/stat"); len(tasks) > 0 {
		for _, task := range tasks {
			stat, err := os.ReadFile(task)
			if err != nil {
				return false
			}
			// The state follows the program's name, which is in parentheses and may hold any.
			if fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:])); len(fields) == 0 || fields[0] != "T" {
				return false
			}
		}
		return true
	}
	state, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return strings.HasPrefix(strings.TrimSpace(string(state)), "T")
}

// gitInit makes the sandbox's work directory a git repository, whose name, "_", leaves nothing
// of a session's name. git runs in the sandbox's environment, as cld does, so no GIT_DIR, say,
// sends it to another repository (see TestMain).
func gitInit(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	runGit(t, s, s.Root, "init", "-q", s.Work)
}

// repository makes the git repository name in the sandbox's root, whose name a session's takes,
// and returns its directory.
func repository(t *testing.T, s *sandbox.Sandbox, name string) string {
	t.Helper()
	dir := filepath.Join(s.Root, name)
	runGit(t, s, s.Root, "init", "-q", dir)
	return dir
}

// runGit runs git with args in dir, in the sandbox's environment (see gitInit), as an author and
// committer of its own, where the sandbox's home has no git configuration.
func runGit(t *testing.T, s *sandbox.Sandbox, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=cld", "-c", "user.email=cld@example.com"}, args...)...)
	cmd.Env = s.Environ(nil)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// gitWorktree makes the linked worktree .claude/worktrees/NAME of the work directory's
// repository, which gitInit made, on a commit of its own, as claude's --worktree NAME makes one,
// and returns its path.
func gitWorktree(t *testing.T, s *sandbox.Sandbox, name string) string {
	t.Helper()
	path := filepath.Join(s.Work, ".claude", "worktrees", name)
	for _, args := range [][]string{
		{"-c", "user.name=cld", "-c", "user.email=cld@example.com", "commit", "-q", "--allow-empty", "-m", name},
		{"worktree", "add", "-q", "-b", "worktree-" + name, path},
	} {
		cmd := exec.Command("git", append([]string{"-C", s.Work}, args...)...)
		cmd.Env = s.Environ(nil)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return path
}

// waitClients waits until count clients are attached to cld's servers, all told.
func waitClients(t *testing.T, s *sandbox.Sandbox, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d attached client(s)", count), func() bool {
		return len(s.Clients()) == count
	})
}

func waitScreen(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%q on the screen", text), func() bool {
		return strings.Contains(term.Screen(), text)
	})
}

// waitBorderLine waits until the screen's last line is a line of the pane's border with text in
// it, as the pane-died hook leaves it below a claude that failed: text between the border's ─, not
// the message line, which starts with the text.
func waitBorderLine(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := term.Screen()
		last := screen[strings.LastIndexByte(screen, '\n')+1:]
		if strings.HasPrefix(last, "─") && strings.Trim(last, "─ ") == text {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 10s waiting for the last line to be the pane's border with %q; the screen shows\n%s", text, screen)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitModes waits until the terminal's modes are as ok wants them, and reports them otherwise: the
// terminal may not have taken in yet what sets them, which can come after the screen it shows -
// tmux turns every mouse mode off and on again once it has drawn.
func waitModes(t *testing.T, term terminal.Terminal, when, want string, ok func(terminal.Modes) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for modes := term.Modes(); !ok(modes); modes = term.Modes() {
		if time.Now().After(deadline) {
			t.Errorf("modes %s %+v, want %s", when, modes, want)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitLines waits until the screen shows exactly lines, from the top, and nothing below them;
// spaces at the end of a line do not count.
func waitLines(t *testing.T, term terminal.Terminal, lines ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := strings.Split(term.Screen(), "\n")
		for i := range got {
			got[i] = strings.TrimRight(got[i], " ")
		}
		for len(got) > 0 && got[len(got)-1] == "" {
			got = got[:len(got)-1]
		}
		if slices.Equal(got, lines) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 10s waiting for the screen to show\n%s\nit shows\n%s", strings.Join(lines, "\n"), strings.Join(got, "\n"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// shortDirectory creates a directory named prefix and a number, with the symbolic links in its
// path resolved, and removes it when the test ends.
func shortDirectory(t *testing.T, prefix string) string {
	t.Helper()
	for {
		dir := prefix + strconv.Itoa(rand.IntN(1000))
		err := os.Mkdir(dir, 0o755)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
}

// cutCells is text cut to columns cells, for text whose characters take one cell each but for
// Han ones, which take two.
func cutCells(text string, columns int) string {
	used := 0
	for i, r := range text {
		size := 1
		if unicode.Is(unicode.Han, r) {
			size = 2
		}
		if used+size > columns {
			return text[:i]
		}
		used += size
	}
	return text
}

// cutTo is text cut to columns characters, for text whose characters take one cell each.
func cutTo(text string, columns int) string {
	runes := []rune(text)
	return string(runes[:min(columns, len(runes))])
}

// footerIn is the list's hints as they show in columns cells: without the arrows' hint when the
// hints do not all fit, then cut, without the spaces the screen does not show at the end.
func footerIn(hints string, columns int) string {
	if utf8.RuneCountInString(hints) > columns {
		hints = strings.TrimPrefix(hints, "↑/↓ to navigate · ")
	}
	return strings.TrimRight(cutTo(hints, columns), " ")
}

// detachedSessions creates cld's sessions of the given names, each from a terminal of its own
// that then detaches with C-q d, and returns their claudes by name.
func detachedSessions(t *testing.T, s *sandbox.Sandbox, names ...string) map[string]*sandbox.Probe {
	t.Helper()
	for _, name := range names {
		term := startCld(t, s, "tmux", nil, "join", "-s", name)
		sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-"+name, func() bool {
			return slices.Contains(s.Clients(), "cld-"+name)
		})
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
	probes := map[string]*sandbox.Probe{}
	sandbox.WaitFor(t, 10*time.Second, "the claudes to start", func() bool {
		for _, probe := range s.Probes() {
			probes[strings.TrimPrefix(probe.Argv[1], "cld-")] = probe
		}
		return !slices.ContainsFunc(names, func(name string) bool { return probes[name] == nil })
	})
	return probes
}

// wrapTmux puts a tmux first on the PATH of the environment it returns: an sh script that runs
// script with tmux's arguments as "$@", and then the tmux the tests run.
func wrapTmux(t *testing.T, s *sandbox.Sandbox, script string) map[string]string {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.MkdirTemp(s.Root, "bin.")
	if err != nil {
		t.Fatal(err)
	}
	s.WriteProgram(filepath.Join(bin, "tmux"), "#!/bin/sh\n"+script+"exec '"+tmux+"' \"$@\"\n", 0o755)
	return map[string]string{"PATH": bin + string(os.PathListSeparator) + s.Env["PATH"]}
}

// loggedTmux puts a tmux first on the PATH of the environment it returns (see wrapTmux) that
// writes down what it runs, and returns with it asked, which gives the servers it ran
// list-sessions on since asked was last called, as they ran.
func loggedTmux(t *testing.T, s *sandbox.Sandbox) (env map[string]string, asked func() []string) {
	t.Helper()
	ran := filepath.Join(s.Root, "tmux ran")
	env = wrapTmux(t, s, "echo \"$*\" >>'"+ran+"'\n")
	return env, func() []string {
		t.Helper()
		out, err := os.ReadFile(ran)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if err := os.Remove(ran); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		var servers []string
		for line := range strings.SplitSeq(string(out), "\n") {
			if words := strings.Fields(line); slices.Contains(words, "list-sessions") {
				servers = append(servers, words[slices.Index(words, "-L")+1])
			}
		}
		return servers
	}
}

// heldTmux holds the tmux commands of a kind that cld run with env runs, from when the test holds
// them until it releases them or ends: a tmux first on the PATH (see wrapTmux) waits as long as
// the file hold is there. It also counts the commands that begin, held or not, a line each in
// hold.begun.
type heldTmux struct {
	hold, what string
	env        map[string]string
}

// holdLookup holds the lookup of session cld-NAME that Enter, the kill and join make, from the
// start. It tells the lookup from the read of the sessions, which asks server cld-NAME with the
// same filter, by the one format that follows.
func holdLookup(t *testing.T, s *sandbox.Sandbox, name string) heldTmux {
	t.Helper()
	lookup := holdTmux(t, s, "the lookup of cld-"+name, lookupPattern(name))
	lookup.start(t)
	return lookup
}

// lookupPattern is the pattern of sh's case that the arguments of the lookup of session cld-NAME
// match (see holdLookup).
func lookupPattern(name string) string {
	return "*'#{==:#{session_name},cld-" + name + "},'*' -F #{session_name} #{W:#{P:#{pane_pid} }}\t#{@cld-home}'"
}

// holdTmux is ready to hold the tmux commands, described by what, whose arguments match pattern,
// a pattern of sh's case, once the test calls start.
func holdTmux(t *testing.T, s *sandbox.Sandbox, what, pattern string) heldTmux {
	t.Helper()
	return holding(t, s, what, pattern, "", "")
}

// holdTmuxOutput is holdTmux, but for commands that run first: what they write, on stdout and
// without its last newlines, and their exit status are held back until the test releases them, as
// if tmux took that long to answer.
func holdTmuxOutput(t *testing.T, s *sandbox.Sandbox, what, pattern string) heldTmux {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	return holding(t, s, what, pattern, "\tout=$('"+tmux+"' \"$@\" 2>&1); status=$?\n",
		"\t[ -z \"$out\" ] || printf '%s\\n' \"$out\"\n\texit $status\n")
}

// holding is holdTmux, and holdTmuxOutput, with the lines of sh's run before the commands are held
// and then after.
func holding(t *testing.T, s *sandbox.Sandbox, what, pattern, run, then string) heldTmux {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "hold.")
	if err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(dir, "hold")
	t.Cleanup(func() { _ = os.Remove(hold) })
	return heldTmux{hold: hold, what: what, env: wrapTmux(t, s, "case \"$*\" in "+pattern+")\n"+run+
		"\techo $$ >>'"+hold+".begun'\n"+
		"\tif [ -e '"+hold+"' ]; then echo $$ >'"+hold+".held'; fi\n"+
		"\twhile [ -e '"+hold+"' ]; do sleep 0.05; done\n"+then+
		"\t;;\n"+
		"esac\n")}
}

// start holds the commands from now on.
func (h heldTmux) start(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(h.hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// held waits for a command to be held, and returns the pid of the tmux holding it.
func (h heldTmux) held(t *testing.T) int {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, h.what, func() bool {
		data, _ = os.ReadFile(h.hold + ".held")
		return len(data) > 0 && data[len(data)-1] == '\n'
	})
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// release lets the commands go on.
func (h heldTmux) release(t *testing.T) {
	t.Helper()
	if err := os.Remove(h.hold); err != nil {
		t.Fatal(err)
	}
}

// begun counts the commands that have begun, held or not.
func (h heldTmux) begun(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(h.hold + ".begun")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return bytes.Count(data, []byte("\n"))
}

// gone reports whether process pid has ended and been waited for.
func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// listScript runs cld list, as "$@", recording what listRun reads.
const listScript = `tty >"$0.tty"; stty -g >"$0.before"; "$@"; echo $? >"$0.code"; stty -g >"$0.after"`

// pidScript is listScript that also records cld's pid: the inner sh writes its own, which exec
// hands on to cld.
var pidScript = strings.Replace(listScript, `"$@";`, `sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@";`, 1)

// jobScript is pidScript run as a job of a shell with job control (see startJob), which goes on
// with the script when cld stops: it records cld's status then (stopped) and the terminal's mode
// (during), and writes a line of its own. Once the test says so, with the file go, it puts its
// own mode back - as bash does when it takes the terminal back, and dash does not - and cld back
// in the foreground (fg).
const jobScript = `tty >"$0.tty"; stty -g >"$0.before"; ` +
	`sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@"; ` +
	`echo $? >"$0.stopped"; stty -g >"$0.during"; echo "the shell's line"; ` +
	`until [ -e "$0.go" ]; do sleep 0.05; done; stty "$(cat "$0.before")"; ` +
	`fg >/dev/null; echo $? >"$0.code"; stty -g >"$0.after"`

// attributesAnswer matches a terminal's answer to a question for its primary device attributes.
var attributesAnswer = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

// waitFrames waits until the list has drawn count frames: each starts at the top left. The
// output log trails the screen. Keys that reach the list together draw one frame, as keys typed
// one at a time may under load: a test that counts frames types a key once the frame before it
// is in the log, or pastes keys meant to come together, in one write.
func waitFrames(t *testing.T, term terminal.Terminal, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d frames in the output log", count), func() bool {
		return bytes.Count(term.Output(), []byte("\x1b[1;1H")) >= count
	})
}

// armThen presses Ctrl+X, runs armed - which waits for the list to arm the kill, and checks what
// it will meanwhile - and presses then, such as C-x to kill or Escape to keep, which has to reach
// the list within the two seconds of the arm. Under load the checks can take them all: when a
// second has gone by since the first Ctrl+X, a letter disarms the kill, if it is still armed, and
// Ctrl+X arms it again, then follows at once.
func armThen(t *testing.T, term terminal.Terminal, armed func(), then string) {
	t.Helper()
	pressed := time.Now()
	term.Keys("C-x")
	armed()
	if time.Since(pressed) < time.Second {
		term.Keys(then)
		return
	}
	term.Keys("k", "C-x", then)
}

// listRun is cld list run under sh, with the files sh writes: the terminal's name (tty), its
// mode before and after cld (stty -g), and cld's exit status. The script gets the files' common
// path as $0 and cld list as "$@".
type listRun string

// startList runs script in term, in the sandbox's environment with extra variables added. sh
// then sleeps, so that the terminal shows what cld left behind rather than tmux's "Pane is dead".
func startList(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, script string, extra map[string]string) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh"}, script, extra)
}

// startJob runs jobScript in term under an interactive sh (-i), which has job control. macOS's
// sh, bash 3.2 as Apple builds it, hears of a job that stops (waitpid's WUNTRACED) only when it
// is interactive: under set -m in a script, it goes on waiting for a stopped job to end. There,
// bash puts its own mode back as the job stops, before the script reads it (during).
func startJob(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh", "-i"}, jobScript, nil)
}

// startListIn is startList with shell, a command line, in place of sh.
func startListIn(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, shell []string, script string, extra map[string]string) listRun {
	t.Helper()
	dir, err := os.MkdirTemp(s.Root, "list.")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	maps.Copy(env, s.Env)
	maps.Copy(env, extra)
	run := listRun(filepath.Join(dir, "cld"))
	argv := append(slices.Clone(shell), "-c", script+"; exec sleep 600", string(run))
	term.Start(append(argv, s.CldArgv("list")...), env, s.Work)
	return run
}

// exited reports whether cld has exited.
func (r listRun) exited() bool {
	_, err := os.Stat(string(r) + ".code")
	return err == nil
}

// read waits for the named file to hold a line and returns it.
func (r listRun) read(t *testing.T, name string) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write "+name, func() bool {
		data, _ = os.ReadFile(string(r) + "." + name)
		return len(data) > 0 && data[len(data)-1] == '\n'
	})
	return strings.TrimSuffix(string(data), "\n")
}

// code is cld's exit status, once it has exited.
func (r listRun) code(t *testing.T) string {
	t.Helper()
	return r.read(t, "code")
}

// checkRaw checks that the terminal is in raw mode: no line editing, no echo, and Ctrl+C a key.
func (r listRun) checkRaw(t *testing.T) {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	stty := exec.Command("stty", "-a")
	stty.Stdin = tty
	out, err := stty.Output()
	if err != nil {
		t.Fatalf("stty -a: %v", err)
	}
	for _, flag := range []string{"-icanon", "-echo", "-isig"} {
		if !slices.Contains(strings.Fields(strings.ReplaceAll(string(out), ";", " ")), flag) {
			t.Errorf("the terminal is not in raw mode: no %s in\n%s", flag, out)
		}
	}
}

// unread reports whether the terminal has input that cld has not read, as cld itself tells: in
// raw mode, the terminal is readable once it has a byte.
func (r listRun) unread(t *testing.T) bool {
	t.Helper()
	tty, err := os.Open(strings.TrimSpace(r.readTTY(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	fd := int(tty.Fd())
	var set unix.FdSet
	set.Set(fd)
	n, err := unix.Select(fd+1, &set, nil, nil, &unix.Timeval{})
	return err == nil && n > 0
}

// readTTY is the terminal's name; uutils' tty (0.8.0) prints it without a newline.
func (r listRun) readTTY(t *testing.T) string {
	t.Helper()
	var data []byte
	sandbox.WaitFor(t, 10*time.Second, "sh to write the terminal's name", func() bool {
		data, _ = os.ReadFile(string(r) + ".tty")
		return len(data) > 0
	})
	return string(data)
}

// checkRestored checks that cld left the terminal as it found it: the same mode (stty -g), the
// main screen, no mouse reporting and the cursor visible. The terminal may still be reading
// what cld wrote last when sh has written its exit status.
func (r listRun) checkRestored(t *testing.T, term terminal.Terminal) {
	t.Helper()
	if before, after := r.read(t, "before"), r.read(t, "after"); before != after {
		t.Errorf("stty -g after cld\n%s\nwant as before\n%s", after, before)
	}
	waitModes(t, term, "after cld", "the main screen, no mouse and the cursor visible", func(modes terminal.Modes) bool {
		return !modes.AltScreen && !modes.Mouse && modes.Cursor
	})
}

// afterList waits for cld to leave the alternate screen and print want after it, with the
// terminal's CR LF line ends as LF. The output log trails the screen and cld's exit status: the
// outer terminal writes it as it goes, through a pipe.
func afterList(t *testing.T, term terminal.Terminal, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		output := string(term.Output())
		end := strings.LastIndex(output, "\x1b[?1049l")
		printed := ""
		if end >= 0 {
			if printed = strings.ReplaceAll(output[end+len("\x1b[?1049l"):], "\r\n", "\n"); printed == want {
				return
			}
		}
		if time.Now().After(deadline) {
			if end < 0 {
				t.Fatalf("timed out after 10s: cld never left the alternate screen: %q", output)
			}
			t.Fatalf("timed out after 10s: printed on leaving\n%q\nwant\n%q", printed, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// attribute names the attribute each SGR code sets or clears.
var attribute = map[string]string{
	"1": "intensity", "2": "intensity", "22": "intensity",
	"3": "italic", "23": "italic",
	"4": "underline", "21": "underline", "24": "underline",
	"5": "blink", "6": "blink", "25": "blink",
	"7": "inverse", "27": "inverse",
	"8": "hidden", "28": "hidden",
	"9": "strike", "29": "strike",
	"53": "overline", "55": "overline",
	"38": "fg", "39": "fg", "48": "bg", "49": "bg", "58": "underline-colour", "59": "underline-colour",
}

// clearing lists the SGR codes that turn their attribute off.
var clearing = map[string]bool{
	"22": true, "23": true, "24": true, "4:0": true, "25": true, "27": true, "28": true,
	"29": true, "55": true, "39": true, "49": true, "59": true,
}

// cells turns a Styled screen into lines of runs, "[attribute=code ...]text", whatever the
// SGR sequences that happened to draw them: equal cells give equal lines. Trailing blank cells
// and lines are dropped, so that screens of different sizes compare by what is drawn on them.
func cells(styled string) []string {
	state := map[string]string{} // capture-pane -e carries attributes over line ends
	var lines []string
	for row := range strings.SplitSeq(styled, "\n") {
		var attrs, texts []string // one per run
		matches := sgr.FindAllStringSubmatch(row, -1)
		for i, text := range sgr.Split(row, -1) {
			if i > 0 {
				apply(state, matches[i-1][1])
			}
			if text == "" {
				continue
			}
			if n := len(attrs); n > 0 && attrs[n-1] == describe(state) {
				texts[n-1] += text
			} else {
				attrs, texts = append(attrs, describe(state)), append(texts, text)
			}
		}
		if n := len(attrs); n > 0 && attrs[n-1] == "" {
			if texts[n-1] = strings.TrimRight(texts[n-1], " "); texts[n-1] == "" {
				attrs, texts = attrs[:n-1], texts[:n-1]
			}
		}
		var line strings.Builder
		for i := range attrs {
			line.WriteString("[" + attrs[i] + "]" + texts[i])
		}
		lines = append(lines, line.String())
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// apply updates the attributes of the cells after an SGR sequence with parameters params.
func apply(state map[string]string, params string) {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		if code == "" || code == "0" {
			clear(state)
			continue
		}
		base, _, _ := strings.Cut(code, ":")
		// A colour in the ; form takes the codes after it: 5;N or 2;R;G;B.
		if (base == "38" || base == "48" || base == "58") && base == code {
			n := 4
			if i+1 < len(codes) && codes[i+1] == "5" {
				n = 2
			}
			code = strings.Join(codes[i:min(i+n+1, len(codes))], ";")
			i += n
		}
		name, known := attribute[base]
		switch {
		case !known:
		case clearing[code]:
			delete(state, name)
		default:
			state[name] = code
		}
	}
}

func describe(state map[string]string) string {
	var pairs []string
	for name, code := range state {
		pairs = append(pairs, name+"="+code)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// underlined reports whether any cell of a screen from cells is underlined.
func underlined(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, "underline=")
	})
}
