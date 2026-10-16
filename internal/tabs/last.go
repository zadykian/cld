package tabs

import "time"

// typedWait is how long a typed line has to join its session. ssh to its host past it means a
// refused join, which leaves the user in the host's shell.
const typedWait = 2 * time.Minute

// clearLast clears pane p's Last where seen, at time now, shows the pane in use again (decision
// 59.4). That is cld on any host, or a local program. So is ssh to Last's host where nothing was
// typed, or typedWait after a typed line. A shell at its prompt keeps it, for the next restore.
func clearLast(p *Pane, seen Seen, now time.Time) {
	if p.Last == nil || seen.Keep {
		return
	}
	switch seen.Kind {
	case KindCld, KindOther:
		p.Last = nil
	case KindSSH:
		typed := p.Last.Typed
		if seen.Host == p.Last.Host && (typed == nil || !now.Before(typed.Add(typedWait))) {
			p.Last = nil
		}
	}
}

// cutAtQuit is the quit of run ended at time at, for panes. A Last that another run set clears, as
// its second run change has come. Each pane on ssh to a host gets the binding it had. Within a
// run, nothing sets Last: a dropped connection's cut waits (decision 59.10).
func cutAtQuit(panes map[string]*Pane, ended string, at time.Time) {
	for _, p := range panes {
		if p.Last != nil && p.Last.Run != ended {
			p.Last = nil
		}
		if (p.Kind == KindCld || p.Kind == KindSSH) && p.Host != "" {
			p.Last = &Last{Host: p.Host, Session: p.Session, Worktree: p.Worktree, Cut: CutQuit,
				Run: ended, At: at}
		}
	}
}
