package tests

import (
	"path/filepath"
	"strings"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The --settings that join gives claude, as the tests expect it word for word. It turns agent view
// off (decision 47). Its hooks keep claude's status and worktree, its record entry and the run and
// busy marks (decisions 25, 26, 40 and 48). Decision 39 sets their timeouts. See TestStatusHooks,
// TestWorktreeHooks, TestHooksOutsideThePane, TestRecordHooks and TestBusyMark.

// sessionSettings is the --settings the claude of session name, started in dir, gets in s from a
// cld that finds the sandbox's tmux and git (see settings).
func sessionSettings(s *sandbox.Sandbox, name, dir string) string {
	return settings(s, sandbox.RealTmux, sandbox.RealGit, name, dir, false)
}

// settings is the --settings the claude of session name, cld-NAME, started in dir, gets in s from
// a cld that found tmux and git at those paths. With fromHead, join -w's worktree branches from
// HEAD. Nothing else: no remoteControlAtStartup, which is the user's own (decision 42).
func settings(s *sandbox.Sandbox, tmux, git, name, dir string, fromHead bool) string {
	set := tmuxSetter(tmux, filepath.Join(s.SocketDir(), name), name)
	status := func(value string) string { return set("@cld-status", value) }
	worktree := worktreeCommand(git, set)
	r := newRecordCommands(entryFile(s, strings.TrimPrefix(name, "cld-")),
		strings.TrimPrefix(name, "cld-"), dir)
	interrupt := `if grep -Eq '\"is_interrupt\": *true'; then ` + status("idle") + `; else ` +
		status("busy") + `; fi`
	base := ""
	if fromHead {
		base = `"worktree":{"baseRef":"head"},`
	}
	return `{"disableAgentView":true,` + base + `"hooks":{` + strings.Join([]string{
		inBackground("CwdChanged", worktree),
		onEvent("Elicitation", "", status("waiting")),
		onEvent("ElicitationResult", "", status("busy")),
		onEvent("Notification", "idle_prompt", status("idle"), r.idle),
		onEvent("PermissionRequest", "", status("waiting")),
		onEvent("PostToolUse", "", status("busy")),
		onEvent("PostToolUseFailure", "", interrupt, r.interrupted),
		hookEvent("SessionEnd", hookGroup("", r.touch, "")),
		onEvent("SessionStart", "", worktree, r.record),
		onEvent("Stop", "", status("idle"), r.stop),
		onEvent("StopFailure", "", status("idle"), r.idle),
		onEvent("UserPromptSubmit", "", status("busy"), r.busyMark),
	}, ",") + `}}`
}

// tmuxSetter gives the hooks' command that sets option to value on session name, as the settings'
// JSON has it. It goes by the server's socket, only where the option holds another value.
func tmuxSetter(tmux, socket, name string) func(option, value string) string {
	return func(option, value string) string {
		return `'` + tmux + `' -S '` + socket + `' if -F -t '=` + name + `:' \"#{!=:#{` + option +
			`},` + value + `}\" \"set -t =` + name + `: ` + option + ` ` + value + `\"`
	}
}

// worktreeCommand is the hooks' command that sets @cld-worktree to 1 where claude's directory is
// in a linked git worktree, and to 0 elsewhere, by set (see tmuxSetter).
func worktreeCommand(git string, set func(option, value string) string) string {
	gitDir := func(which string) string {
		return `\"$('` + git + `' rev-parse --path-format=absolute ` + which + ` 2>/dev/null)\"`
	}
	return `w=0; [ ` + gitDir("--git-dir") + ` = ` + gitDir("--git-common-dir") + ` ] || w=1; ` +
		set("@cld-worktree", "$w")
}

// recordCommands are the hooks' commands that keep session's entry in cld's record, and its busy
// and run marks beside it, as the settings' JSON has them.
type recordCommands struct {
	record, touch, idle, busyMark, interrupted, stop string
}

// newRecordCommands is the recordCommands of session, started in dir, whose entry is file.
func newRecordCommands(file, session, dir string) recordCommands {
	head := `{"name":` + jsonText(session) + `,"directory":` + jsonText(dir) + `,"conversation":"`
	temp := `'` + file + `'.$$`
	busy := `'` + strings.TrimSuffix(file, ".json") + `.busy'`
	return recordCommands{
		record: jsonEscaped(`id=$(sed -n 's/.*"session_id" *: *"\([0-9A-Za-z-]*\)".*/\1/p' | ` +
			`head -n 1); if [ -n "$id" ]; then printf '%s%s"}\n' '` + head + `' "$id" >` + temp +
			` && mv -f ` + temp + ` '` + file + `'; fi`),
		touch: jsonEscaped(`touch -c '` + file + `'`),
		idle:  jsonEscaped(`rm -f ` + busy),
		busyMark: jsonEscaped(`[ ! -e '` + file + `' ] || : >` + busy + `; touch -c '` +
			strings.TrimSuffix(file, ".json") + `.run'`),
		interrupted: jsonEscaped(`if grep -Eq '"is_interrupt": *true'; then rm -f ` + busy + `; fi`),
		stop:        jsonEscaped(`rm -f ` + busy + `; touch -c '` + file + `'`),
	}
}

// jsonEscaped is command as the settings' JSON has it in a string, without the quotes.
func jsonEscaped(command string) string {
	text := jsonText(command)
	return text[1 : len(text)-1]
}

// hookGroup is a group of an event's hooks: command, where the event matches matcher, run as how
// says, with claude's default timeout where how is "".
func hookGroup(matcher, command, how string) string {
	if matcher != "" {
		matcher = `"matcher":"` + matcher + `",`
	}
	if how != "" {
		how = `,` + how
	}
	return `{` + matcher + `"hooks":[{"type":"command","command":"` + command + `"` + how + `}]}`
}

// hookEvent is the event's entry in the settings' hooks, with its groups.
func hookEvent(event string, groups ...string) string {
	return `"` + event + `":[` + strings.Join(groups, ",") + `]`
}

// onEvent runs each of commands, in a group of its own, and claude waits for it 5 s at most.
func onEvent(event, matcher string, commands ...string) string {
	var groups []string
	for _, command := range commands {
		groups = append(groups, hookGroup(matcher, command, `"timeout":5`))
	}
	return hookEvent(event, groups...)
}

// inBackground runs command, and claude goes on.
func inBackground(event, command string) string {
	return hookEvent(event, hookGroup("", command, `"async":true`))
}
