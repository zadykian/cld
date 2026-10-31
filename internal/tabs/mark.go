package tabs

import (
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// mark records in r, at time now, what apply did of plan p, under the record's lock (decision
// 59.6). res is apply's result, nil where it gave none.
func mark(r *Record, p *Planned, res *ghostty.Result, now time.Time) {
	if res == nil {
		return
	}
	now = now.UTC().Truncate(time.Second)
	for i := range min(len(res.Typed), len(p.typed)) {
		if t := res.Typed[i]; t.Error == "" && !t.Busy {
			typedAt(r.Panes[p.typed[i].pane], now)
		}
	}
}

// typedAt notes on pane p that a restore typed its Last's line at time now. A tick leaves the Last
// until the line has had its time to join (decision 59.4).
func typedAt(p *Pane, now time.Time) {
	if p != nil && p.Last != nil {
		p.Last.Typed = new(now)
	}
}
