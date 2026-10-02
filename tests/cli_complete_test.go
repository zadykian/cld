package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What __complete offers for the commands, help and setup project's options.

// noFileDirective ends what __complete offers: ShellCompDirectiveNoFileComp, no file names, the
// root's default for an argument with nothing to complete. noFileReport is cobra's line for it on
// stderr.
const (
	noFileDirective = ":4\n"
	noFileReport    = "Completion ended with directive: ShellCompDirectiveNoFileComp\n"
)

// The lines __complete offers, each a word and its description.
const (
	offeredJoin = "join\tattach to session NAME-SUFFIX, creating or resuming it first\n"
	//nolint:dupword // the word, then its description
	offeredDetach     = "detach\tdetach the terminals attached to session NAME-SUFFIX\n"
	offeredCompletion = "completion\tprint the completion script for a shell\n"
	offeredCommands   = offeredJoin + offeredDetach +
		"kill\tend session NAME-SUFFIX and its tmux server\n" +
		"list\tlist cld's sessions; on a terminal, join or kill one\n" + //nolint:dupword // as above
		"restore\tbring back the sessions that ran when the machine stopped\n" +
		"setup\tset up claude in a project, telemetry, shell completion or restore\n" +
		"update\tupdate cld to the latest release\n" + //nolint:dupword // as above
		"version\tshow the version\n" +
		offeredCompletion +
		"help\tshow this help, or the help of COMMAND\n"
)

// What __complete offers for setup and its commands.
const (
	offeredProject   = "project\tset claude up in the project in the current directory\n"
	offeredTelemetry = "telemetry\tsend claude's telemetry through a local OpenTelemetry " +
		"collector\n"
	offeredSetupCompletion = "completion\tset up cld's completion in bash, zsh or fish\n"
	offeredSetupRestore    = "restore\thave your systemd run cld restore at login, or at boot\n"
	offeredSetups          = offeredProject + offeredTelemetry + offeredSetupCompletion +
		offeredSetupRestore

	offeredSetupZsh    = "zsh\tset up cld's completion in zsh\n"
	offeredSetupFish   = "fish\tset up cld's completion in fish\n"
	offeredShellSetups = "bash\tset up cld's completion in bash\n" + offeredSetupZsh +
		offeredSetupFish

	offeredGoland    = "goland\tGoLand's MCP server, port $GOLAND_MCP_PORT or 64422\n"
	offeredJbcontext = "jbcontext\tJetBrains Context's semantic code search, jbcontext mcp\n"
	offeredRider     = "rider\tRider's MCP server, port $RIDER_MCP_PORT or 64482\n"
	offeredReadOnly  = "read-only\tread files and run commands that only read, the default\n"
	offeredCld       = "cld\tcld's own: edit files, run git, go, make, docker and more\n"
	offeredNone      = "none\tnothing more than claude allows by itself\n"
)

// What __complete offers for completion and for the options of join and detach.
const (
	offeredShells = "bash\tprint the completion script for bash\n" +
		"zsh\tprint the completion script for zsh\n" +
		"fish\tprint the completion script for fish\n" +
		"powershell\tprint the completion script for powershell\n"
	// cobra describes an option by the first line of its usage, backquotes included.
	offeredJoinOptions = "--detach-others\tdetach any other terminal attached to the session\n" +
		"--fork\twith --resume, resume a copy of SESSION under a new\n" +
		"--help\thelp for join\n-h\thelp for join\n" +
		"--name\tthe session's `NAME`, before -SUFFIX: by default the\n" +
		"-n\tthe session's `NAME`, before -SUFFIX: by default the\n" +
		"--new\tcreate a session that has ended anew, with a new\n" +
		"--resume\tcreate the session with claude resuming `SESSION`,\n" +
		"--suffix\tthe session's `SUFFIX`, after NAME-: by default the index\n" +
		"-s\tthe session's `SUFFIX`, after NAME-: by default the index\n" +
		"--worktree\tcreate the session with claude in git worktree\n" +
		"-w\tcreate the session with claude in git worktree\n"
	offeredDetachOptions = "--help\thelp for detach\n-h\thelp for detach\n" +
		"--name\tthe session's `NAME`, before -SUFFIX: by default the\n" +
		"-n\tthe session's `NAME`, before -SUFFIX: by default the\n" +
		"--suffix\tthe session's `SUFFIX`, after NAME-\n" +
		"-s\tthe session's `SUFFIX`, after NAME-\n"
)

// completeCases are the words __complete gets, and what it offers.
var completeCases = []struct {
	args []string
	want string
}{
	{[]string{"__complete", ""}, offeredCommands + noFileDirective},
	{[]string{"__complete", "c"}, offeredCompletion + noFileDirective},
	{[]string{"__complete", "help", ""}, offeredCommands + noFileDirective},
	{[]string{"__complete", "help", "j"}, offeredJoin + noFileDirective},
	{[]string{"__complete", "help", "d"}, offeredDetach + noFileDirective},
	{[]string{"__complete", "help", "x"}, noFileDirective},
	{[]string{"__complete", "help", "join", ""}, noFileDirective},
	{[]string{"__complete", "help", "new"}, noFileDirective},
	// After a command with commands of its own, help takes one of those, and nothing after it.
	{[]string{"__complete", "help", "setup", ""}, offeredSetups + noFileDirective},
	{[]string{"__complete", "help", "setup", "t"}, offeredTelemetry + noFileDirective},
	{[]string{"__complete", "help", "setup", "project", ""}, noFileDirective},
	{[]string{"__complete", "help", "setup", "x"}, noFileDirective},
	{[]string{"__complete", "help", "setup", "telemetry", ""}, noFileDirective},
	{[]string{"__complete", "help", "setup", "completion", ""}, offeredShellSetups + noFileDirective},
	{[]string{"__complete", "help", "setup", "completion", "z"}, offeredSetupZsh + noFileDirective},
	{[]string{"__complete", "help", "setup", "completion", "zsh", ""}, noFileDirective},
	{[]string{"__complete", "help", "completion", ""}, offeredShells + noFileDirective},
	{[]string{"__complete", "help", "nope", ""}, noFileDirective},
	{[]string{"__complete", "completion", ""}, offeredShells + noFileDirective},
	{[]string{"__complete", "setup", ""}, offeredSetups + noFileDirective},
	{[]string{"__complete", "setup", "p"}, offeredProject + noFileDirective},
	{[]string{"__complete", "setup", "c"}, offeredSetupCompletion + noFileDirective},
	{[]string{"__complete", "setup", "r"}, offeredSetupRestore + noFileDirective},
	{[]string{"__complete", "setup", "restore", ""}, noFileDirective},
	{[]string{"__complete", "restore", ""}, noFileDirective},
	{[]string{"__complete", "setup", "completion", ""}, offeredShellSetups + noFileDirective},
	{[]string{"__complete", "setup", "completion", "f"}, offeredSetupFish + noFileDirective},
	{[]string{"__complete", "setup", "completion", "bash", ""}, noFileDirective},
	// --mcp offers its servers; after a comma, those the list does not have, after it.
	{[]string{"__complete", "setup", "project", "--m"},
		"--mcp\tan MCP `SERVER` for claude in the project: goland or\n" + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", ""},
		offeredGoland + offeredJbcontext + offeredRider + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "j"}, offeredJbcontext + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp=r"}, offeredRider + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "rider,"},
		"rider," + offeredGoland + "rider," + offeredJbcontext + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "goland,rider,j"},
		"goland,rider," + offeredJbcontext + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "goland,jbcontext,rider,"}, noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "x"}, noFileDirective},
	{[]string{"__completeNoDesc", "setup", "project", "--mcp", "goland,"},
		"goland,jbcontext\ngoland,rider\n" + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "goland", ""}, noFileDirective},
	// --permissions offers its sets, the default first.
	{[]string{"__complete", "setup", "project", "--p"},
		"--permissions\twhat claude may do in the project without asking:\n" + noFileDirective},
	{[]string{"__complete", "setup", "project", "--permissions", ""},
		offeredReadOnly + offeredCld + offeredNone + noFileDirective},
	{[]string{"__complete", "setup", "project", "--permissions", "n"}, offeredNone + noFileDirective},
	{[]string{"__complete", "setup", "project", "--mcp", "goland", "--permissions=c"},
		offeredCld + noFileDirective},
	{[]string{"__complete", "setup", "project", "--permissions", "x"}, noFileDirective},
	{[]string{"__completeNoDesc", "setup", "project", "--permissions", ""},
		"read-only\ncld\nnone\n" + noFileDirective},
	{[]string{"__complete", "setup", "project", "--permissions", "cld", ""}, noFileDirective},
	// No URL, port or file name is offered, --collector-config's FILE included.
	{[]string{"__complete", "setup", "telemetry", "--l"},
		"--local\twhere traces, metrics and logs go, such as\n" + noFileDirective},
	{[]string{"__complete", "setup", "telemetry", "--local", ""}, noFileDirective},
	{[]string{"__complete", "setup", "telemetry", "--port", ""}, noFileDirective},
	{[]string{"__complete", "setup", "telemetry", "--collector-config", ""}, noFileDirective},
	{[]string{"__complete", "setup", "telemetry", "--remote", "https://otel.example.com:4317", ""},
		noFileDirective},
	{[]string{"__complete", "completion", "bash", ""}, noFileDirective},
	{[]string{"__complete", "join", "-"}, offeredJoinOptions + noFileDirective},
	{[]string{"__complete", "detach", "-"}, offeredDetachOptions + noFileDirective},
	{[]string{"__complete", "join", "--f"},
		"--fork\twith --resume, resume a copy of SESSION under a new\n" + noFileDirective},
	{[]string{"__complete", "join", "--resume", ""}, noFileDirective},
	{[]string{"__complete", "list", ""}, noFileDirective},
}

// __complete offers the commands, and those help takes, each with its description; the options;
// setup project's MCP servers and permission sets; and never file names (decision 17.3). Without
// the word to complete it fails, as cld's other command-line mistakes do.
func TestCompleteCommands(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	for _, test := range completeCases {
		result := s.RunCld(none, test.args...)
		if result.Code != 0 || result.Stdout != test.want || result.Stderr != noFileReport {
			t.Errorf("cld %q: exit %d, stderr %q, stdout\n%s\nwant exit 0, stderr %q, stdout\n%s",
				test.args, result.Code, result.Stderr, result.Stdout, noFileReport, test.want)
		}
	}
	for _, command := range []string{"__complete", "__completeNoDesc"} {
		result := s.RunCld(none, command)
		want := "cld: " + command + ": missing the word to complete (see cld help)\n"
		if result.Code != 2 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("cld %s: exit %d, stdout %q, stderr %q, want exit 2, stderr %q",
				command, result.Code, result.Stdout, result.Stderr, want)
		}
	}
}
