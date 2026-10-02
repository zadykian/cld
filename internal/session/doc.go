// Package session is cld's tmux side: it runs Claude Code in named sessions, each on a private
// tmux server of its own (decision 13). docs/design/overview.md follows a session through its life,
// and gives each of the server's options with its reason.
//
// In the code, NAME is a session's whole name, all tmux sees, and the parameter suffix is what
// follows "cld-". cld looks for session cld-NAME on server cld-NAME only: a session claude makes
// there has another name, so cld leaves it alone. A server keeps the environment of the client
// that started it, so join refuses a server that outlives its session (see lingering).
//
// What concerns claude alone goes on its pane or session, never its window or the server. The
// clients that attach and the reads of the sessions run tmux -u, as tmux writes "_" for what is
// not ASCII under a locale without UTF-8. switch.go moves a terminal between sessions, record.go
// and record_*.go keep cld's record of its sessions, and restore.go is cld restore.
package session
