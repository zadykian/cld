package session

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// Sessions reads the sessions cld started, in the order of their names: those of the servers that
// take the connection, asks at a time (decision 38.2), and those that have ended, as Ended. It
// starts no server and writes nothing, as list, join's sweep and completion read with it. Once ctx
// is done, its tmux is killed.
func (t *Tmux) Sessions(ctx context.Context) ([]Session, error) {
	sockets, err := readSockets()
	if err != nil {
		return nil, err
	}
	sessions, err := t.ask(ctx, connected(ctx, sockets))
	if err != nil {
		return nil, err
	}
	// The sessions that have ended: those of the record with no session on their server - none, or
	// one that outlives its session (see lingering), which join refuses.
	for _, r := range entries() {
		if !slices.ContainsFunc(sessions, func(s Session) bool { return s.Name == r.Name }) {
			sessions = append(sessions, endedSession(r))
		}
	}
	slices.SortFunc(sessions, func(a, b Session) int { return strings.Compare(a.Name, b.Name) })
	return sessions, nil
}

// connected are the NAMEs of the sockets cld-NAME whose servers may run: a socket that refuses the
// connection, or a NAME no session can have, is passed over (decision 38.1).
func connected(ctx context.Context, sockets []os.DirEntry) []string {
	connect := tmuxDir()
	var suffixes []string
	for _, socket := range sockets {
		suffix, found := strings.CutPrefix(socket.Name(), "cld-")
		if found && ValidName(suffix) && !serverless(ctx, connect, suffix) {
			suffixes = append(suffixes, suffix)
		}
	}
	return suffixes
}

// asks is how many servers Sessions asks at once (decision 38.2).
const asks = 8

// ask asks the server of each of suffixes for its session, asks at once, in their order. Once one
// fails no more start, and of those that failed the first in that order gives the error.
func (t *Tmux) ask(ctx context.Context, suffixes []string) ([]Session, error) {
	// Each ask takes a place in asking before it starts, and gives it back once answered, in a slot
	// of its own. Those under way when one fails finish, and may fail too.
	found := make([]*Session, len(suffixes))
	failed := make([]error, len(suffixes))
	asking := make(chan struct{}, asks)
	var failing atomic.Bool
	var wait sync.WaitGroup
	for i, suffix := range suffixes {
		asking <- struct{}{}
		if failing.Load() {
			break
		}
		wait.Go(func() {
			defer func() { <-asking }()
			if found[i], failed[i] = t.session(ctx, suffix); failed[i] != nil {
				failing.Store(true)
			}
		})
	}
	wait.Wait()
	var sessions []Session
	for i := range suffixes {
		if failed[i] != nil {
			return nil, failed[i]
		}
		if found[i] != nil {
			sessions = append(sessions, *found[i])
		}
	}
	return sessions, nil
}

// The fields of the line session reads, apart at tabs. claude's status shares the state's field,
// after a space, where tmux writes only the hooks' words (decision 49.3). The times share a field
// (decision 46.5), and the home's length goes before the home and the directory (decision 37.5).
const (
	statusFormat = "#{?#{==:#{@cld-status},busy},busy," +
		"#{?#{==:#{@cld-status},waiting},waiting,#{?#{==:#{@cld-status},idle},idle,}}}"
	stateFormat = "#{?pane_dead,exited,#{?session_attached,attached,detached} " +
		statusFormat + "}"
	timesFormat = "#{session_activity} #{session_last_attached}"
	// The directory claude is in now, or once it has exited, where its session started.
	pathFormat    = "#{n:@cld-home}\t#{@cld-home}#{?pane_dead,#{session_path},#{pane_current_path}}"
	sessionFormat = "#{session_name}\t" + stateFormat + "\t#{session_attached}\t" + panePIDs +
		"\t" + timesFormat + "\t" + pathFormat
)

// session asks the server of socket cld-SUFFIX for its session cld-SUFFIX, for Sessions: nil
// where no server runs there, or runs without it. -u has tmux write the tabs, and any letter
// beyond ASCII, unchanged.
func (t *Tmux) session(ctx context.Context, suffix string) (*Session, error) {
	out, err := combinedOutput(t.commandContext(ctx, "-u", "-L", "cld-"+suffix, "list-sessions",
		"-f", only(suffix), "-F", sessionFormat))
	if err != nil {
		if noServer(out) {
			return nil, nil
		}
		return nil, fail.Runtime(out)
	}
	read := time.Now()
	line, _, _ := strings.Cut(out, "\n")
	field := fields(line, 7)
	if field[0] != "cld-"+suffix {
		return nil, nil
	}
	clients, _ := strconv.Atoi(field[2]) //nolint:errcheck // no number is no terminal attached
	home, directory := cutHome(field[5], field[6])
	s := &Session{Name: suffix, Attached: clients > 0, PIDs: strings.Fields(field[3]),
		Directory: directory, Home: home}
	s.State, s.Status, _ = strings.Cut(field[1], " ")
	if !s.Attached {
		s.Idle = idleSince(read, strings.Fields(field[4]))
	}
	return s, nil
}

// idleSince is how long a session with no terminal attached has been idle at now, by the later of
// times, its activity and its last attach, the second missing before any attach (decision 46.5):
// 0 where one of them is no whole seconds.
func idleSince(now time.Time, times []string) time.Duration {
	if len(times) == 0 {
		return 0
	}
	var last int64
	for _, value := range times {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0
		}
		last = max(last, seconds)
	}
	return max(now.Sub(time.Unix(last, 0)), 0)
}

// fields splits a line of Sessions' output into count fields: runs of tabs separate them, tabs
// around the line are dropped, and the last, the home and the directory (see cutHome), takes the
// rest of it.
func fields(line string, count int) []string {
	rest := strings.Trim(line, "\t")
	var split []string
	for len(split) < count-1 {
		field, after, _ := strings.Cut(rest, "\t")
		split, rest = append(split, field), strings.TrimLeft(after, "\t")
	}
	return append(split, rest)
}

// cutHome splits the end of a line of Sessions' output, rest, into the session's home and the
// directory after it, at the home's length. A home cut short with the line, by a newline in it,
// is the whole of rest.
func cutHome(length, rest string) (home, directory string) {
	n, err := strconv.Atoi(length)
	if err != nil || n < 0 {
		return "", rest
	}
	n = min(n, len(rest))
	return rest[:n], rest[n:]
}
