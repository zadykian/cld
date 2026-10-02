package session

import (
	"bytes"
	"strings"
)

// recordHooks are the hooks that keep session cld-SUFFIX's entry, in file, for claude started in
// dir, and the busy mark beside it (decisions 40.2 and 48.3). UserPromptSubmit touches the run
// mark too, for restore to tell an idle session (decision 48.8). None prints anything, and
// SessionEnd's alone has no timeout, as claude gives those hooks 1.5 s together (decision 40.2).
func recordHooks(file, suffix, dir string) map[string][]hook {
	base := strings.TrimSuffix(file, ".json")
	busy, run := shellWord(base+busyMark), shellWord(base+runMark)
	touch := `touch -c ` + shellWord(file)
	idle := `rm -f ` + busy
	prompted := `[ ! -e ` + shellWord(file) + ` ] || : >` + busy + `; touch -c ` + run
	interrupted := `if grep -Eq '"is_interrupt": *true'; then ` + idle + `; fi`
	return map[string][]hook{
		"SessionStart":       waitedHook("", entryWriter(file, suffix, dir)),
		"UserPromptSubmit":   waitedHook("", prompted),
		"Stop":               waitedHook("", idle+`; `+touch),
		"StopFailure":        waitedHook("", idle),
		"PostToolUseFailure": waitedHook("", interrupted),
		"Notification":       waitedHook("idle_prompt", idle),
		"SessionEnd":         {{Hooks: []hookCommand{{Type: "command", Command: touch}}}},
	}
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
