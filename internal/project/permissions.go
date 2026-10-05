package project

// Permissions is a set of permission rules, which --permissions picks: setup config project and
// user add its entries to permissions.allow and permissions.deny.
type Permissions struct {
	// Name is the set's name for --permissions, and Description what completion shows for it.
	Name, Description string
	// allow and deny are the set's own entries, and server what it allows of a server after them.
	allow, deny []string
	server      func(Server) []string
}

// PermissionSets are the sets --permissions takes, the default first: read-only lets claude read
// and nothing more, cld is this repository's own, and none adds nothing. The settings are shared:
// whoever accepts the folder's workspace trust gives claude what they allow.
var PermissionSets = []Permissions{
	{"read-only", "read files and run commands that only read, the default", readOnlyAllow, nil,
		func(s Server) []string { return s.read }},
	{"cld", "cld's own: edit files, run git, go, make, docker and more", cldAllow, cldDeny,
		func(s Server) []string { return s.all }},
	{"none", "nothing more than claude allows by itself", nil, nil,
		func(Server) []string { return nil }},
}

// readOnlyAllow lets claude read files and run commands that only read. A Bash rule admits every
// option, so none of these commands takes one that runs a command or writes a file. git diff, log
// and show are left out: their --output writes any file, .git/config too (decision 28.4).
var readOnlyAllow = []string{
	"Read",
	"Bash(ls:*)",
	"Bash(pwd:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(grep:*)",
	"Bash(stat:*)",
	"Bash(du:*)",
	"Bash(which:*)",
	"Bash(git status:*)",
}

// cldAllow is what --permissions cld lets claude do without a prompt, as this repository has it:
// one developer's tools, and nearly anything, such as Edit, Write and Bash(git:*).
var cldAllow = []string{
	"Read",
	"Edit",
	"Write",
	"WebSearch",
	"WebFetch",
	"Bash(ls:*)",
	"Bash(dir:*)",
	"Bash(pwd:*)",
	"Bash(find:*)",
	"Bash(grep:*)",
	"Bash(grep -E:*)",
	"Bash(rg:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(sort:*)",
	"Bash(uniq:*)",
	"Bash(printf:*)",
	"Bash(echo:*)",
	"Bash(tree:*)",
	"Bash(stat:*)",
	"Bash(du:*)",
	"Bash(which:*)",
	"Bash(where:*)",
	"Bash(mkdir:*)",
	"Bash(touch:*)",
	"Bash(chmod:*)",
	"Bash(sed:*)",
	"Bash(nl -ba)",
	"Bash(git:*)",
	"Bash(jq:*)",
	"Bash(go:*)",
	"Bash(gofmt:*)",
	"Bash(make:*)",
	"Bash(shellcheck:*)",
	"Bash(shfmt:*)",
	"Bash(docker build:*)",
	"Bash(docker run:*)",
	"Bash(docker images:*)",
	"Bash(gh issue view:*)",
	"Bash(gh issue list:*)",
	"Bash(gh pr create:*)",
	"Bash(gh pr view:*)",
	"Bash(gh pr list:*)",
	"Bash(gh pr checks:*)",
	"Bash(gh pr diff:*)",
	"Bash(gh run list:*)",
	"Bash(gh run view:*)",
	"Workflow(code-review)",
}

// cldDeny is what --permissions cld refuses claude: merging a pull request, which lands on main by
// fast-forward alone, and pushing to main, which the maintainer does. A rule matches the command
// as written, so these stop the usual forms of both, not every one.
var cldDeny = []string{
	"Bash(gh pr merge *)",
	"Bash(git push * main)",
	"Bash(git push * main *)",
	"Bash(git push * +main)",
	"Bash(git push *:main)",
	"Bash(git push *:main *)",
	"Bash(git push *:refs/heads/main)",
	"Bash(git push *:refs/heads/main *)",
}
