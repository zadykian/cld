package session

import "strings"

// settings are what join passes claude with --settings, as JSON in this field order.
type settings struct {
	DisableAgentView bool              `json:"disableAgentView"`
	Worktree         worktreeSettings  `json:"worktree,omitzero"`
	Hooks            map[string][]hook `json:"hooks"`
}

// worktreeSettings is claude's worktree setting, whose baseRef join gives with -w (decision 4).
type worktreeSettings struct {
	BaseRef string `json:"baseRef"`
}

// hook is a command claude runs on an event, where the event's matcher field - the notification's
// type for Notification - matches Matcher, or on every one of the event without it.
type hook struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// hookCommand is a command claude runs through sh: in the background with Async, and otherwise
// waiting for it, for Timeout seconds at most where that is not 0.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookTimeout is how many seconds claude waits for a hook of statusHooks that it does not run in
// the background, and for those of recordHooks but SessionEnd's, where its own default is 600 (30
// for UserPromptSubmit; decision 39.2).
const hookTimeout = 5

// statusHooks are the hooks that keep claude's status in @cld-status on session cld-SUFFIX, for the
// title and list (decision 25), and with git, found by an absolute path, @cld-worktree (decision
// 26). claude waits for each but CwdChanged's, which it runs in the background (decision 39).
func statusHooks(tmux, git, socket, suffix string) map[string][]hook {
	set := optionSetter(tmux, socket, suffix)
	busy, waiting, idle := set("@cld-status", "busy"), set("@cld-status", "waiting"),
		set("@cld-status", "idle")
	interrupted := `if grep -Eq '"is_interrupt": *true'; then ` + idle + `; else ` + busy + `; fi`
	hooks := map[string][]hook{
		"UserPromptSubmit":   waitedHook("", busy),
		"PostToolUse":        waitedHook("", busy),
		"PostToolUseFailure": waitedHook("", interrupted),
		"PermissionRequest":  waitedHook("", waiting),
		"Elicitation":        waitedHook("", waiting),
		"ElicitationResult":  waitedHook("", busy),
		"Notification":       waitedHook("idle_prompt", idle),
		"Stop":               waitedHook("", idle),
		"StopFailure":        waitedHook("", idle),
	}
	if git != "" {
		worktree := worktreeCommand(git, set)
		hooks["SessionStart"] = waitedHook("", worktree)
		hooks["CwdChanged"] = backgroundHook(worktree)
	}
	return hooks
}

// optionSetter gives the sh command that sets an option of session cld-SUFFIX to a value, which
// the shell expands, where it differs: tmux by its path, on the server by its socket (decision
// 25.3).
func optionSetter(tmux, socket, suffix string) func(option, value string) string {
	session := "=cld-" + suffix + ":"
	return func(option, value string) string {
		return shellWord(tmux) + ` -S ` + shellWord(socket) + ` if -F -t ` + shellWord(session) +
			` "#{!=:#{` + option + `},` + value + `}" "set -t ` + session + ` ` + option + ` ` + value + `"`
	}
}

// worktreeCommand sets @cld-worktree with set: 1 where git's directory is not the common one, in a
// linked worktree, and 0 elsewhere (decision 26.2).
func worktreeCommand(git string, set func(option, value string) string) string {
	dir := func(which string) string {
		return `"$(` + shellWord(git) + ` rev-parse --path-format=absolute ` + which +
			` 2>/dev/null)"`
	}
	return `w=0; [ ` + dir("--git-dir") + ` = ` + dir("--git-common-dir") + ` ] || w=1; ` +
		set("@cld-worktree", "$w")
}

// waitedHook runs command on the event, where it matches matcher, and claude waits for it.
func waitedHook(matcher, command string) []hook {
	return []hook{{Matcher: matcher,
		Hooks: []hookCommand{{Type: "command", Command: command, Timeout: hookTimeout}}}}
}

// backgroundHook runs command on every one of the event, and claude goes on.
func backgroundHook(command string) []hook {
	return []hook{{Hooks: []hookCommand{{Type: "command", Command: command, Async: true}}}}
}

// shellWord is text as one word of sh, quoted.
func shellWord(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
