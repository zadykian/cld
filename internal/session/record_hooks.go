package session

import (
	"bytes"
	"strings"
)

// recordHooks are the hooks that keep session cld-SUFFIX's entry, in file, naming dir, and the
// files beside it (decisions 40.2 and 48.3). UserPromptSubmit touches the run mark too, for restore
// (decision 48.8). Stop and SessionEnd write claude's directory with the tmux at tmux, on the
// server of socket (decision 56.2). None prints anything; SessionEnd's have no timeout.
func recordHooks(file, suffix, dir, tmux, socket string) map[string][]hook {
	base := strings.TrimSuffix(file, ".json")
	busy, run := shellWord(base+busyMark), shellWord(base+runMark)
	touch := `touch -c ` + shellWord(file)
	idle := `rm -f ` + busy
	prompted := `[ ! -e ` + shellWord(file) + ` ] || : >` + busy + `; touch -c ` + run
	interrupted := `if grep -Eq '"is_interrupt": *true'; then ` + idle + `; fi`
	moved := dirHook(file, tmux, socket)
	return map[string][]hook{
		"SessionStart":       waitedHook("", entryWriter(file, suffix, dir)),
		"UserPromptSubmit":   waitedHook("", prompted),
		"Stop":               append(waitedHook("", idle+`; `+touch), backgroundHook(moved)...),
		"StopFailure":        waitedHook("", idle),
		"PostToolUseFailure": waitedHook("", interrupted),
		"Notification":       waitedHook("idle_prompt", idle),
		// claude gives these 1.5 s together, which a timeout of 5 s would stretch (decision 40.2).
		"SessionEnd": {{Hooks: []hookCommand{{Type: "command", Command: touch}}},
			{Hooks: []hookCommand{{Type: "command", Command: moved}}}},
	}
}

// dirHook is the sh command that writes claude's directory beside the entry in file (see
// dirWriter). The tmux at tmux reads it on the server of socket, in claudePane, whichever pane
// runs the hook (decision 56.2).
func dirHook(file, tmux, socket string) string {
	read := `$(` + shellWord(tmux) + ` -u -S ` + shellWord(socket) + ` display -p -t ` +
		claudePane + ` '#{pane_current_path}')`
	return dirWriter(file, read, func(text string) string { return text }) + ` || true`
}

// claudePane is claude's pane on its own server, whatever pane is active (decision 56.2).
const claudePane = "%0"

// dirWriter is the sh command that writes the directory value gives, a word of sh, in NAME.dir
// beside the entry in file. It writes only a directory, while the entry exists, by a temporary file
// renamed over NAME.dir (decision 56.1), its errors to /dev/null. The text around value goes
// through fixed, as run-shell would expand a "#" in it.
func dirWriter(file, value string, fixed func(string) string) string {
	dir := shellWord(strings.TrimSuffix(file, ".json") + lastDir)
	return fixed(`{ [ -e `+shellWord(file)+` ] && d=`) + value + fixed(` && [ -d "$d" ] && `+
		`printf '%s\n' "$d" >`+dir+`.$$ && mv -f `+dir+`.$$ `+dir+`; } 2>/dev/null`)
}

// entryWriter is the sh command that writes session cld-SUFFIX's entry in file anew. It takes the
// session_id of the hook's input where that has only the characters of an ID (decision 40.2). It
// writes the whole line to a temporary file, renamed over the entry.
func entryWriter(file, suffix, dir string) string {
	// The line with no conversation ends in "", "}" and a newline: the hook writes what comes
	// before the second quote, then the ID, then the rest.
	head := bytes.TrimSuffix(entryLine(entry{Name: suffix, Directory: dir}), []byte("\"}\n"))
	temp := shellWord(file) + `.$$`
	return `id=$(sed -n 's/.*"session_id" *: *"\([0-9A-Za-z-]*\)".*/\1/p' | head -n 1); ` +
		`if [ -n "$id" ]; then printf '%s%s"}\n' ` + shellWord(string(head)) + ` "$id" >` + temp +
		` && mv -f ` + temp + ` ` + shellWord(file) + `; fi`
}
