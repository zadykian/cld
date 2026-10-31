package ghostty

import (
	"context"
	"fmt"
	"time"
)

// An apply's bound: applyBase, applyPerLine for each line it types, in place or as a new
// terminal's initial input, and applyPerBuild for each build; applyLimit at most.
const (
	applyBase     = 30 * time.Second
	applyPerLine  = 3 * time.Second
	applyPerBuild = 5 * time.Second
	applyLimit    = 10 * time.Minute
)

// Plan is what apply.js does in Ghostty: it types each of Typed into its pane, then makes Builds
// in order. A restore orders them so that the window it builds last ends in front.
type Plan struct {
	Pace   Pace     `json:"pace"`
	Typed  []Typing `json:"typed"`
	Builds []Build  `json:"builds"`
}

// Pace spaces the lines that start an ssh, so that sshd's MaxStartups and a fail2ban see no
// burst (decision 59.7).
type Pace struct {
	Gap   float64 `json:"gap"`   // seconds between two lines
	Every int     `json:"every"` // a pause after every Every lines
	Pause float64 `json:"pause"` // seconds the pause adds to the gap
}

// Typing is a line to type into a pane Ghostty kept, whose shell waits at its prompt.
type Typing struct {
	Pane string `json:"pane"` // the terminal's id
	PID  int    `json:"pid"`  // the shell's group, in front in the snapshot; another is busy
	Line string `json:"line"`
}

// Build is a window, a tab or a split to make. A tab opens in the window that holds Anchor, or as
// a window where none does, and a split splits Anchor.
type Build struct {
	Kind   string     `json:"kind"`             // window, tab or pane
	Anchor string     `json:"anchor,omitempty"` // a terminal's id
	Split  string     `json:"split,omitempty"`  // pane: right, down, left or up, the new side
	Index  int        `json:"index,omitempty"`  // tab: the 1-based place to move it to
	Tabs   []TabBuild `json:"tabs"`             // window: its tabs; tab and pane: one
}

// TabBuild is a tab's split tree to make, the leaf it focuses, and whether its window selects it.
type TabBuild struct {
	Selected bool   `json:"selected,omitempty"`
	Focused  string `json:"focused"` // a leaf's key
	Tree     *Node  `json:"tree"`
}

// Node is a split tree: a Leaf, or a split into First, the left or upper part, and Second.
type Node struct {
	Split  string `json:"split,omitempty"` // right or down
	First  *Node  `json:"first,omitempty"`
	Second *Node  `json:"second,omitempty"`
	Leaf   *Leaf  `json:"leaf,omitempty"`
}

// Leaf is a terminal to make: a shell in Cwd, which runs Line as if typed.
type Leaf struct {
	Key  string `json:"key"` // unique in the plan; the result maps it to the terminal's id
	Cwd  string `json:"cwd,omitempty"`
	Line string `json:"line,omitempty"` // "" for the shell alone
}

// Result is what apply.js did: an entry per Typing, in the plan's order. Error is set where it
// did nothing, and Apply returns it as an *Error instead.
type Result struct {
	Typed []TypedResult `json:"typed"`
	Error *ScriptError  `json:"error,omitempty"`
}

// TypedResult is a Typing's outcome: typed, left busy, or failed.
type TypedResult struct {
	Pane  string `json:"pane"`
	Busy  bool   `json:"busy,omitempty"`  // another group was in front: nothing typed
	Error string `json:"error,omitempty"` // JXA's text, then its number in brackets
}

// Apply runs apply.js with plan. A Result whose Error is set is returned as an *Error instead,
// and so is one that lacks an entry for each typing: a later step indexes them.
func Apply(ctx context.Context, plan *Plan) (*Result, error) {
	in := *plan
	if in.Typed == nil {
		in.Typed = []Typing{} // apply.js reads a list: null fails it
	}
	if in.Builds == nil {
		in.Builds = []Build{}
	}
	result := &Result{}
	if err := run(ctx, applyTimeout(&in), applyJS, &in, result); err != nil {
		return nil, err
	}
	switch {
	case result.Error != nil:
		return nil, classify(result.Error)
	case len(result.Typed) != len(in.Typed):
		return nil, failed("", fmt.Sprintf("apply.js answered %d of %d typings",
			len(result.Typed), len(in.Typed)))
	}
	return result, nil
}

// applyTimeout is the bound of plan's apply.
func applyTimeout(plan *Plan) time.Duration {
	lines := len(plan.Typed)
	for _, b := range plan.Builds {
		for _, tab := range b.Tabs {
			lines += tab.Tree.lines()
		}
	}
	limit := applyBase + time.Duration(lines)*applyPerLine +
		time.Duration(len(plan.Builds))*applyPerBuild
	return min(limit, applyLimit)
}

// lines counts the leaves of n that type a line.
func (n *Node) lines() int {
	switch {
	case n == nil:
		return 0
	case n.Leaf != nil && n.Leaf.Line != "":
		return 1
	case n.Leaf != nil:
		return 0
	}
	return n.First.lines() + n.Second.lines()
}
