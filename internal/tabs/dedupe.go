package tabs

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// want is a lost leaf, or a pane Ghostty kept, that a restore would join to its binding again,
// by terminal id, and when that binding was lost.
type want struct {
	id string
	at time.Time
}

// wants are the wants of the panes kept and the leaves of lost, by binding. A leaf's binding was
// lost with its loss, and a kept pane's at its Last.
func wants(kept []keptPane, lost []pruned) map[Binding][]want {
	out := map[Binding][]want{}
	for _, k := range kept {
		out[k.binding] = append(out[k.binding], want{id: k.pane, at: k.at})
	}
	for _, l := range lost {
		for id, p := range windowCopies(l.windows) {
			if b, ok := paneBinding(p); ok {
				out[b] = append(out[b], want{id: id, at: l.loss.At})
			}
		}
	}
	return out
}

// dedupe is the wants, by terminal id, whose binding shows in a pane of panes opened since
// (decision 59.6). Per binding, each cld or ssh pane on it, earliest since first, takes the
// earliest want lost at or before its since. So two tabs on one session come back as two, less
// those joined again since.
func dedupe(panes map[string]*Pane, wants map[Binding][]want) map[string]bool {
	open := map[Binding][]time.Time{}
	for _, p := range panes {
		if b, ok := paneBinding(p); ok {
			open[b] = append(open[b], p.Since)
		}
	}
	deduped := map[string]bool{}
	for b, list := range wants {
		since := open[b]
		slices.SortFunc(since, time.Time.Compare)
		slices.SortFunc(list, func(x, y want) int {
			return cmp.Or(x.at.Compare(y.at), strings.Compare(x.id, y.id))
		})
		taken := 0
		for _, s := range since {
			if taken < len(list) && !list[taken].at.After(s) {
				deduped[list[taken].id] = true
				taken++
			}
		}
	}
	return deduped
}

// skip is a pane Ghostty kept, or a lost leaf, that a restore leaves a shell, for the report's
// note. Its binding shows in a pane opened since, or it named no host.
type skip struct {
	binding Binding // Host "" for no host
	cwd     string
	kept    bool // a pane Ghostty kept, left unchanged; else a lost leaf, which a shell replaces
}

// note is the report's note on s, c writing its directory.
func (s skip) note(c clock) string {
	what := "a shell takes its place in " + c.dir(s.cwd)
	if s.kept {
		what = "left the pane in " + c.dir(s.cwd) + " as it is"
	}
	switch {
	case s.binding.Host != "":
		return s.binding.String() + " shows in a pane opened since: " + what
	case s.kept:
		return "a pane's last ssh named no host cld can type: " + what
	}
	return "a lost pane showed ssh to no host cld can type: " + what
}

// notes are the plan's notes, c writing directories: the panes and leaves left a shell, and the
// hosts whose terminfo Ghostty has not cached.
func (p *Planned) notes(c clock) []string {
	var notes []string
	for _, s := range p.skipped {
		notes = append(notes, s.note(c))
	}
	for _, host := range p.uncached {
		notes = append(notes, uncachedNote(host))
	}
	return notes
}
