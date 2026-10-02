package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// SocketDir is the directory tmux keeps the sandbox's sockets in.
func (s *Sandbox) SocketDir() string {
	return filepath.Join(s.Root, "tmux-"+strconv.Itoa(os.Getuid()))
}

// Tmux runs a command against the sandbox's tmux server named server (tmux -L server): cld's
// session cld-NAME is on server cld-NAME.
func (s *Sandbox) Tmux(server string, args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-L", server}, args...)...)
	cmd.Env = s.Environ(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux -L %s %s: %s",
			server, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// MustTmux is Tmux that fails the test on error.
func (s *Sandbox) MustTmux(server string, args ...string) string {
	s.t.Helper()
	out, err := s.Tmux(server, args...)
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

// Format expands a tmux format for a session's pane (cld sessions have one), on the server named
// like the session. "display -p -t =SESSION" would be shorter, but tmux 3.3 expands it to nothing
// without a client.
func (s *Sandbox) Format(session, format string) string {
	s.t.Helper()
	return s.MustTmux(session, "list-panes", "-s", "-t", "="+session, "-F", format)
}

// Servers lists the names of the sandbox's sockets that cld's servers have - cld-*, whether a
// server still runs on them or not - in order.
func (s *Sandbox) Servers() []string {
	paths := s.glob(filepath.Join(s.SocketDir(), "cld-*"))
	servers := make([]string, 0, len(paths))
	for _, path := range paths {
		servers = append(servers, filepath.Base(path))
	}
	return servers
}

// Sessions lists the sessions on the servers of Servers, in order: a session on the server named
// like it as its name, cld-NAME, and any other as SERVER/SESSION - one that claude made on its
// server, say. Nothing where no server runs.
func (s *Sandbox) Sessions() []string {
	var sessions []string
	for _, server := range s.Servers() {
		out, err := s.Tmux(server, "list-sessions", "-F", "#{session_name}")
		if err != nil {
			continue
		}
		for session := range strings.FieldsSeq(out) {
			if session != server {
				session = server + "/" + session
			}
			sessions = append(sessions, session)
		}
	}
	slices.Sort(sessions)
	return sessions
}

// Clients lists the clients attached to the servers of Servers, as the sessions they are
// attached to, in order.
func (s *Sandbox) Clients() []string {
	var clients []string
	for _, server := range s.Servers() {
		if out, err := s.Tmux(server, "list-clients", "-F", "#{session_name}"); err == nil {
			clients = append(clients, strings.Fields(out)...)
		}
	}
	slices.Sort(clients)
	return clients
}

// killServers kills the server of each of the sandbox's sockets, so that none outlives its test.
func (s *Sandbox) killServers() {
	sockets, err := os.ReadDir(s.SocketDir())
	if err != nil {
		return // no socket directory: no server ran
	}
	for _, socket := range sockets {
		cmd := exec.Command("tmux", "-S", filepath.Join(s.SocketDir(), socket.Name()), "kill-server")
		cmd.Env = s.Environ(nil)
		_ = cmd.Run() //nolint:errcheck // a stale socket, or a fake tmux's, has no server to kill
	}
}
