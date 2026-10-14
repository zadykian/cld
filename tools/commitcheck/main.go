// Command commitcheck checks the messages of the commits a branch adds to main against the style
// of CLAUDE.md's Commits.
//
// Run from the repository root:
//
//	go run ./tools/commitcheck [RANGE]
//
// RANGE is a git revision range, origin/main..HEAD by default. Its merge commits are passed over:
// main's ruleset refuses them, and CI's run of a pull request checks out GitHub's merge of it. A
// header is type(scope)!: description, the scope and the ! optional, its type one CLAUDE.md lists
// and its scope of a-z, 0-9, _ and -. A blank line parts it from the body, and no line passes 72
// columns. Findings go to stdout as COMMIT:LINE: message, the header being line 1. The exit
// status is 1 for a finding and 2 for a usage or setup error, which go run reports as 1.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// defaultRange is the commits checked where no RANGE is given: those of HEAD that main lacks.
const defaultRange = "origin/main..HEAD"

func main() {
	var stdout, stderr strings.Builder
	status := run(".", os.Args[1:], &stdout, &stderr)
	if _, err := os.Stdout.WriteString(stdout.String()); err != nil {
		fmt.Fprintln(os.Stderr, "commitcheck:", err)
		status = 2
	}
	fmt.Fprint(os.Stderr, stderr.String())
	os.Exit(status)
}

// run checks the commits of the repository at dir that the command line args name, and returns
// the exit status.
func run(dir string, args []string, stdout, stderr *strings.Builder) int {
	flags := flag.NewFlagSet("commitcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: go run ./tools/commitcheck [RANGE]")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return 2
	}
	revisions := defaultRange
	if flags.NArg() == 1 {
		revisions = flags.Arg(0)
	}
	commits, err := list(dir, revisions)
	if err != nil {
		fmt.Fprintln(stderr, "commitcheck:", err)
		return 2
	}
	status := 0
	for _, c := range commits {
		for _, f := range check(c.msg) {
			fmt.Fprintf(stdout, "%s:%d: %s\n", c.id, f.line, f.msg)
			status = 1
		}
	}
	return status
}

// A commit is a commit's abbreviated name and its message.
type commit struct {
	id, msg string
}

// list returns the commits but merges in the range revisions of the repository at dir, oldest
// first. A signature git would show for log.showSignature stays out of the messages.
func list(dir, revisions string) ([]commit, error) {
	cmd := exec.Command("git", "-C", dir, "log", "--no-merges", "--reverse", "--no-show-signature",
		"-z", "--format=%h%n%B", "--end-of-options", revisions, "--")
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("git log: %s", bytes.TrimSpace(exit.Stderr))
		}
		return nil, fmt.Errorf("git log: %w", err)
	}
	var commits []commit
	for record := range strings.SplitSeq(string(out), "\x00") {
		if id, msg, ok := strings.Cut(record, "\n"); ok {
			commits = append(commits, commit{id, msg})
		}
	}
	return commits, nil
}
