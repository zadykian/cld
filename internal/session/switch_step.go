package session

import (
	"context"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

// Step is C-q (, C-q ) or C-q L on the terminal of sw, which to names (decisions 51.2 and 51.4).
// The steps previous and next go round the sessions that run, by name, and last goes back where the
// terminal came from. Where there is none to move to, it says so on the terminal's message line.
// Once ctx is done, its tmux is killed.
func (t *Tmux) Step(ctx context.Context, sw *Switch, to string) error {
	sessions, err := t.Sessions(ctx)
	if err != nil {
		return err
	}
	running := runningNames(sessions)
	var target string
	switch to {
	case "previous":
		target = previousSession(running, sw.suffix)
	case "next":
		target = nextSession(running, sw.suffix)
	default:
		last, why, err := t.back(ctx, sw.suffix, sessions, running)
		if err != nil {
			return err
		}
		if why != "" {
			t.Tell(sw, why)
			return nil
		}
		target = last
	}
	if target == "" || target == sw.suffix {
		t.Tell(sw, "no other session runs")
		return nil
	}
	return t.SwitchTo(sw, target)
}

// runningNames are the names of the sessions that run, in the list's order, by name.
func runningNames(sessions []Session) []string {
	var running []string
	for _, s := range sessions {
		if s.State != Ended {
			running = append(running, s.Name)
		}
	}
	return running
}

// previousSession is the session before session cld-SUFFIX among running, going round to the last
// where none is before it: "" where none runs.
func previousSession(running []string, suffix string) string {
	target := ""
	for _, other := range running {
		if other < suffix {
			target = other
		}
	}
	if target == "" && len(running) > 0 {
		target = running[len(running)-1]
	}
	return target
}

// nextSession is the session after session cld-SUFFIX among running, going round to the first
// where none is after it: "" where none runs.
func nextSession(running []string, suffix string) string {
	if i := slices.IndexFunc(running, func(other string) bool { return other > suffix }); i >= 0 {
		return running[i]
	}
	if len(running) > 0 {
		return running[0]
	}
	return ""
}

// back is the session C-q L moves the terminal on session cld-SUFFIX to, the one it recorded (see
// last). Where there is none, why says so: it recorded none, or the one it recorded has ended or
// gone.
func (t *Tmux) back(ctx context.Context, suffix string, sessions []Session,
	running []string) (target, why string, err error) {
	last, err := t.last(ctx, suffix)
	switch {
	case err != nil:
		return "", "", err
	case last == "":
		return "", "no session to go back to", nil
	case slices.Contains(running, last):
		return last, "", nil
	case slices.ContainsFunc(sessions, func(s Session) bool { return s.Name == last }):
		return "", "session '" + last + "' has ended", nil
	}
	return "", "no session '" + last + "'", nil
}

// Tell shows "cld: " and text on the message line of the terminal of sw, as tmux shows its own
// (decision 51.1). It stays three seconds or until a key, which reaches claude. tmux shows the text
// as it stands, and from 3.6 on keeps drawing claude's pane meanwhile.
func (t *Tmux) Tell(sw *Switch, text string) {
	if sw.client == "" {
		return
	}
	args := []string{"-S", sw.socket, "display-message", "-l", "-c", sw.client, "-d", "3000"}
	if !t.older(version{3, 6, 0}) {
		args = append(args, "-C")
	}
	tell := t.command(append(args, "cld: "+text)...)
	tell.Stdout, tell.Stderr = nil, nil
	_ = tell.Run() //nolint:errcheck // nothing else could show it
}

// last is the session that session cld-SUFFIX recorded as @cld-last, the one the terminal on it
// came from (decision 51.4), or "" where it recorded none. Once ctx is done, its tmux is killed.
func (t *Tmux) last(ctx context.Context, suffix string) (string, error) {
	out, err := combinedOutput(t.serverContext(ctx, suffix,
		"list-sessions", "-f", only(suffix), "-F", "#{@cld-last}"))
	if err != nil {
		return "", fail.Runtime(out)
	}
	last, _, _ := strings.Cut(out, "\n")
	if !ValidName(last) {
		return "", nil
	}
	return last, nil
}
