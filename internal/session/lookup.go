package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

// mark is, as a format, 1 on a server that cld started and 0 on any other: the server option @cld,
// or the prefix C-q that marks the servers of cld 0.8.2 and earlier (decision 34).
const mark = "#{||:#{@cld},#{==:#{prefix},C-q}}"

// only is a filter for session cld-SUFFIX alone on its server, beside the sessions claude made
// there, and on a server that cld started only (see mark). It compares whole names, where a
// target "cld-rev" would find cld-review.
func only(suffix string) string {
	return "#{&&:#{==:#{session_name},cld-" + suffix + "}," + mark + "}"
}

// panePIDs are the pids of the programs in a session's panes, each followed by a space:
// #{pane_pid} alone, in a list-sessions format, is the pid of the active pane of the session's
// current window only.
const panePIDs = "#{W:#{P:#{pane_pid} }}"

// lookup reports whether the server of session cld-SUFFIX runs, whether the session is on it (see
// only) and, if so, the pids of its panes and its home (see Session). A socket with no server runs
// no tmux (see serverless). The home comes last, after a tab, which -u keeps unchanged. Once ctx
// is done, its tmux is killed.
func (t *Tmux) lookup(ctx context.Context, suffix string) (
	server, session bool, pids []string, home string, err error,
) {
	if serverless(ctx, tmuxDir(), suffix) {
		return false, false, nil, "", nil
	}
	found, err := combinedOutput(t.commandContext(ctx, "-u", "-L", "cld-"+suffix, "list-sessions",
		"-f", only(suffix), "-F", "#{session_name} "+panePIDs+"\t#{@cld-home}"))
	if err != nil {
		if noServer(found) {
			return false, false, nil, "", nil
		}
		return false, false, nil, "", fail.Runtime(found)
	}
	name, rest, _ := strings.Cut(found, " ")
	if name != "cld-"+suffix {
		return true, false, nil, "", nil
	}
	rest, home, _ = strings.Cut(rest, "\t")
	return true, true, strings.Fields(rest), home, nil
}

// occupied is why create refuses the name of session cld-SUFFIX, which is on its server or whose
// server runs without it, or what went wrong looking; nil where its server does not run. Callers
// have mostly looked under the record's lock already (see docs/design/overview.md#joining). Once
// ctx is done, its tmux is killed.
func (t *Tmux) occupied(ctx context.Context, suffix string) error {
	server, exists, _, made, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if exists {
		return t.taken(suffix, made)
	}
	if server {
		_, refused := t.lingering(ctx, suffix)
		return refused
	}
	return nil
}

// taken is how create refuses session cld-SUFFIX, which is on its server, made in the home made:
// join attaches to it.
func (t *Tmux) taken(suffix, made string) error {
	// The home tells a session of another repository of this one's name (decision 37.4).
	if made != "" {
		made = " in " + made
	}
	return fail.Runtime(fmt.Sprintf("session '%s' exists%s; attach to it with cld join %s",
		suffix, made, Options(suffix)))
}

// ended is how detach and kill refuse session cld-SUFFIX, which has ended, where cld's record has
// its entry: join brings its conversation back (decision 40.5). The advice for the command line is
// kept apart (fail.Error's Advice). nil where there is no entry.
func ended(suffix string) error {
	if _, ok := recorded(suffix); !ok {
		return nil
	}
	return &fail.Error{Status: 1, Message: fmt.Sprintf("session '%s' has ended", suffix),
		Advice: "; resume it with cld join " + Options(suffix)}
}

// noSession refuses session cld-SUFFIX, which neither runs nor has ended, with the advice for the
// command line kept apart (fail.Error's Advice).
func noSession(suffix string) error {
	return &fail.Error{Status: 1, Message: fmt.Sprintf("no session '%s'", suffix),
		Advice: " (see cld list)"}
}

// Joinable is the lookup the interactive list's Enter makes while the list owns the terminal, as
// far as join goes without claude: nil where session cld-SUFFIX runs, or has ended in a directory
// that can be entered, and otherwise why join refuses it. The list takes a session from anywhere,
// as -n does. Once ctx is done, its tmux is killed.
func (t *Tmux) Joinable(ctx context.Context, suffix string) error {
	server, exists, _, _, err := t.lookup(ctx, suffix)
	switch {
	case err != nil:
		return err
	case exists:
		return nil
	case server:
		_, refused := t.lingering(ctx, suffix)
		return refused
	}
	r, ok := recorded(suffix)
	if !ok {
		return noSession(suffix)
	}
	return enterable(r)
}

// foreign is how join, detach and kill, named in the advice as command, refuse session cld-SUFFIX,
// made in the home made, where home does not take it (decision 37.2). The advice names the session
// with -n, which they then take from anywhere, and goes apart (fail.Error's Advice).
func foreign(command, suffix, made string, home Home) error {
	if home.Takes(made) {
		return nil
	}
	here := "this directory"
	if home.Repository {
		here = "this repository"
	}
	return &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' belongs to %s, not to %s", suffix, made, here),
		Advice:  fmt.Sprintf("; name it with cld %s %s", command, Options(suffix))}
}
