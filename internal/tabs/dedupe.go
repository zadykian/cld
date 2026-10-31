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
