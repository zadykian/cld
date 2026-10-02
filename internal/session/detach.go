package session

import (
	"context"
	"strings"
)

// Detach detaches every terminal attached to session cld-SUFFIX, for cld detach -s, as C-q d in
// each would, for a terminal that keeps C-q from tmux (decision 44.1). It refuses what kill
// refuses. With no terminal on the session, the if keeps detach-client from failing. A detach that
// fails is its exit status, after tmux's message.
func (t *Tmux) Detach(suffix string, home Home) error {
	server, exists, _, made, err := t.lookup(context.Background(), suffix)
	if err != nil {
		return err
	}
	if !exists && server {
		_, refused := t.lingering(context.Background(), suffix)
		return refused
	}
	if !exists {
		if err := ended(suffix); err != nil {
			return err
		}
		return noSession(suffix)
	}
	if err := foreign("detach", suffix, made, home); err != nil {
		return err
	}
	name := "=cld-" + suffix
	detach := t.server(suffix, "if", "-F", "-t", name+":", "#{session_attached}",
		"detach-client -s "+name)
	if err := detach.Run(); err != nil {
		return t.exitStatus(err)
	}
	return nil
}

// DetachTerminal detaches the terminal cld is in, for cld detach without -n and -s where Inside: in
// claude, ! cld detach. tmux detaches the terminal used last on the pane's session, only where one
// is attached and under cld's mark, which the same command prints first (decision 44.2). A
// detach that fails, on a server that has exited say, is its exit status, after tmux's message.
func (t *Tmux) DetachTerminal() error {
	socket, suffix, _ := ownServer()
	detach := t.command("-S", socket, "display-message", "-p", mark, ";",
		"if", "-F", "#{&&:"+mark+",#{session_attached}}", "detach-client")
	marked, err := detach.Output()
	if err != nil {
		return t.exitStatus(err)
	}
	if strings.TrimSuffix(string(marked), "\n") == "0" {
		return notClds(suffix, "; name the session with -s SUFFIX (see cld list)")
	}
	return nil
}

// Inside reports whether cld runs inside one of cld's servers, as TMUX names the socket of server
// cld-NAME: there cld detach without -n and -s detaches the terminal (see DetachTerminal). Unlike
// OwnPane it needs no terminal, as claude runs ! cld detach without one (decision 44.2).
func Inside() bool {
	_, _, found := ownServer()
	return found
}
