package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Session NAME-SUFFIX is tmux session cld-NAME-SUFFIX and claude's --name, from -n and -s in either
// order, or their defaults (decision 24). The words after "--" reach claude as they are, those
// TestRefusesClaudeOptions refuses too, given as values (decision 41.1).
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
		{[]string{"join", "-n", "m", "--", "--model", "opus", "fix the -p bug"}, "cld-m-0",
			[]string{"--model", "opus", "fix the -p bug"}},
		{[]string{"join", "-s", "p", "--", "--append-system-prompt=-p", "", "-dapi,hooks", "--",
			"fix it"}, "cld-work-p",
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
			want := append([]string{"--name", test.session,
				"--settings", sessionSettings(s, test.session, dir)}, test.claude...)
			if !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != dir {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, dir)
			}
		})
	}
}

// Without -s, join takes the index above the highest of NAME's sessions, and a gap stays (decision
// 24.1). Servers that outlived theirs, foreign ones (decision 34.2) and NAME in other letters
// count; stale sockets, other NAMEs and names without an index do not. join --resume SESSION names
// its session alike, and outside a repository the directory's name stands for NAME (decision 24.3).
func TestDefaultNames(t *testing.T) {
	t.Parallel()
	for _, test := range defaultNamesCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := test.directory(t, s)
			test.startServers(t, s)
			startCldIn(t, s, "tmux", dir, nil, append([]string{"join"}, test.options...)...)
			s.WaitProbes(1)
			resume := append(append([]string{"join"}, test.options...), "--resume", "SESSION")
			startCldIn(t, s, "tmux", dir, nil, resume...)
			var names []string
			for _, probe := range s.WaitProbes(2) {
				names = append(names, probe.Argv[1])
			}
			if slices.Sort(names); !slices.Equal(names, test.want) {
				t.Errorf("claude named %q, want %q", names, test.want)
			}
			if want := test.sessions(); !slices.Equal(s.Sessions(), want) {
				t.Errorf("sessions %q, want %q", s.Sessions(), want)
			}
		})
	}
}

// defaultNamesCase is a case of TestDefaultNames.
type defaultNamesCase struct {
	name string
	// dir is where cld runs: the repository work where empty, and otherwise a directory, in the
	// sandbox's root unless a whole path
	dir string
	// options go to join, and to join --resume SESSION
	options []string
	// running are sessions, each on its server, marked as cld marks one. lingering are such servers
	// without their session, foreign are unmarked servers, both holding a session other, and stale
	// are sockets no server answers on.
	running, lingering, foreign, stale []string
	// want are the names that join, then join --resume SESSION, give
	want []string
}

var defaultNamesCases = []defaultNamesCase{
	{"in a repository", "", nil, nil, nil, nil, nil, []string{"cld-work-0", "cld-work-1"}},
	{"in a repository, with sessions", "", nil,
		[]string{"cld-work-0", "cld-work-3", "cld-WORK-4", "cld-work-x", "cld-work-6a", "cld-other-9",
			"cld-9", "cld-work"},
		[]string{"cld-work-5"}, nil, []string{"cld-work-9"}, []string{"cld-work-6", "cld-work-7"}},
	{"outside a repository", "plain", nil, nil, nil, nil, nil,
		[]string{"cld-plain-0", "cld-plain-1"}},
	{"outside a repository, with sessions", "plain", nil,
		[]string{"cld-plain-2", "cld-plain-x-7", "cld-plain-x", "cld-5", "cld-work-9"}, nil, nil,
		[]string{"cld-plain-8"}, []string{"cld-plain-3", "cld-plain-4"}},
	{"in the root directory", "/", nil, []string{"cld-2", "cld-plain-7"}, nil, nil, nil,
		[]string{"cld-3", "cld-4"}},
	{"with -n", "", []string{"-n", "api"}, []string{"cld-api-0", "cld-api-x", "cld-work-3"}, nil,
		[]string{"cld-api-4"}, nil, []string{"cld-api-5", "cld-api-6"}},
}

// directory makes the directory cld runs in, and returns it.
func (c defaultNamesCase) directory(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	switch {
	case c.dir == "":
		return repository(t, s, "work")
	case filepath.IsAbs(c.dir):
		return c.dir
	}
	dir := filepath.Join(s.Root, c.dir)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// startServers starts the servers join then finds, and leaves the stale sockets.
func (c defaultNamesCase) startServers(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, name := range c.running {
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", name, "sleep", "600")
	}
	for _, name := range c.lingering {
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", "other", "sleep", "600")
	}
	for _, name := range c.foreign {
		s.MustTmux(name, "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	}
	for _, name := range c.stale {
		staleSocket(t, s, name)
	}
}

// sessions are the sessions that run once both joins have started theirs, sorted.
func (c defaultNamesCase) sessions() []string {
	want := append(slices.Clone(c.running), c.want...)
	for _, name := range append(slices.Clone(c.lingering), c.foreign...) {
		want = append(want, name+"/other")
	}
	slices.Sort(want)
	return want
}

// repository makes the git repository name in the sandbox's root, whose name a session's takes,
// and returns its directory.
func repository(t *testing.T, s *sandbox.Sandbox, name string) string {
	t.Helper()
	dir := filepath.Join(s.Root, name)
	runGit(t, s, s.Root, "init", "-q", dir)
	return dir
}
