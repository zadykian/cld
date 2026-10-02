package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// withTmuxTmpdir is env with cld's own TMUX_TMPDIR, or none where cld has none, in place of its
// own: the directory of the servers' sockets, which cld reads the sessions from.
func withTmuxTmpdir(env []string) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(variable string) bool {
		return strings.HasPrefix(variable, "TMUX_TMPDIR=")
	})
	if dir, set := os.LookupEnv("TMUX_TMPDIR"); set {
		env = append(env, "TMUX_TMPDIR="+dir)
	}
	return env
}

// terminalVariables are the variables claude reads before TERM_PROGRAM to tell its terminal, each
// naming the terminal the session started in, not the one on it (decision 33.1).
// VSCODE_GIT_ASKPASS_MAIN goes with VS Code's askpass (see vsCodeGit).
var terminalVariables = []string{
	"CURSOR_TRACE_ID", "__CFBundleIdentifier", "VisualStudioVersion", "TERMINAL_EMULATOR",
}

// vsCodeGit are the helpers a terminal of VS Code, or of a fork, gives git: the askpass and the
// editor, scripts named by the variable git beside the file prefix+MAIN names. They ask the window
// through VSCODE_GIT_IPC_HANDLE, and fail at once without it (decision 33.2).
var vsCodeGit = []struct{ git, prefix string }{
	{"GIT_ASKPASS", "VSCODE_GIT_ASKPASS_"},
	{"GIT_EDITOR", "VSCODE_GIT_EDITOR_"},
}

// withoutTerminal is environ without terminalVariables and VS Code's helpers for git, each helper
// as a unit: its prefix's variables, VSCODE_GIT_IPC_HANDLE, and its git variable where that names a
// script beside its MAIN (decision 33.2). A GIT_ASKPASS or GIT_EDITOR elsewhere stays.
func withoutTerminal(environ []string) []string {
	scripts := helperScripts(environ)
	return slices.DeleteFunc(environ, func(variable string) bool {
		name, value, _ := strings.Cut(variable, "=")
		return slices.Contains(terminalVariables, name) || name == "VSCODE_GIT_IPC_HANDLE" ||
			vsCodeHelper(name, value, scripts)
	})
}

// helperScripts maps the git variable of each helper of vsCodeGit to the directory of its MAIN,
// where environ names one.
func helperScripts(environ []string) map[string]string {
	scripts := map[string]string{}
	for _, helper := range vsCodeGit {
		for _, variable := range environ {
			main, found := strings.CutPrefix(variable, helper.prefix+"MAIN=")
			if found && main != "" {
				scripts[helper.git] = filepath.Dir(main)
			}
		}
	}
	return scripts
}

// vsCodeHelper reports whether the variable name, of value, belongs to a helper of vsCodeGit: one
// of its prefix, or its git variable naming a script in the directory scripts has for it.
func vsCodeHelper(name, value string, scripts map[string]string) bool {
	for _, helper := range vsCodeGit {
		if strings.HasPrefix(name, helper.prefix) {
			return true
		}
		if name == helper.git {
			dir, found := scripts[name]
			return found && filepath.Dir(unquoted(value)) == dir
		}
	}
	return false
}

// unquoted is script without the double quotes around it, in which VS Code names its editor's.
func unquoted(script string) string {
	if len(script) > 1 && script[0] == '"' && script[len(script)-1] == '"' {
		return script[1 : len(script)-1]
	}
	return script
}
