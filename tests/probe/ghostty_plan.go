package main

// The plan apply.js takes, and the result it prints (#176). A nil Typed or Builds is null or
// missing, which apply.js throws on, and a nil Pace fails its step. The fake keeps no pace: a test
// reads the plan's in osascript.jsonl.
type (
	plan struct {
		Pace   *struct{} `json:"pace"`
		Typed  []typing  `json:"typed"`
		Builds []build   `json:"builds"`
	}
	typing struct {
		Pane string `json:"pane"`
		PID  int    `json:"pid"`
		Line string `json:"line"`
	}
	build struct {
		Kind   string     `json:"kind"`
		Anchor string     `json:"anchor"`
		Split  string     `json:"split"`
		Index  int        `json:"index"`
		Tabs   []tabBuild `json:"tabs"`
	}
	tabBuild struct {
		Selected bool   `json:"selected"`
		Focused  string `json:"focused"`
		Tree     *node  `json:"tree"`
	}
	node struct {
		Split  string `json:"split"`
		First  *node  `json:"first"`
		Second *node  `json:"second"`
		Leaf   *leaf  `json:"leaf"`
	}
	leaf struct {
		Key  string `json:"key"`
		Cwd  string `json:"cwd"`
		Line string `json:"line"`
	}
	result struct {
		Typed  []typed `json:"typed"`
		Builds []built `json:"builds"`
	}
	typed struct {
		Pane  string `json:"pane"`
		Busy  bool   `json:"busy,omitempty"`
		Error string `json:"error,omitempty"`
	}
	built struct {
		Keys   map[string]string `json:"keys"`
		Window bool              `json:"window,omitempty"`
		Index  int               `json:"index,omitempty"`
		Error  string            `json:"error,omitempty"`
	}
)

// applyFailures are the failure modes an apply answers with an error document. apply.js reads the
// terminals first, and its message gives the error's number in brackets.
var applyFailures = map[string]scriptError{
	"denied": {"count", -1743, "Not authorized to send Apple events to Ghostty. (-1743)", ""},
	"gone":   {"count", -600, "Application isn't running. (-600)", ""},
}

// applyError is a typing's or a build's error, worded as apply.js's message words it: the text,
// and any number in brackets.
type applyError string

func (e applyError) Error() string { return string(e) }

// The errors of typings and builds. Ghostty's texts are main's (ScriptTerminal.swift and
// AppDelegate+AppleScript.swift). JXA's and JavaScriptCore's stand in until #177's spike reads
// them, as for an id Ghostty lacks (its check 4): cld only shows them.
const (
	errNoObject applyError = "Can't get object. (-1728)"
	errOff      applyError = "AppleScript is disabled by the macos-applescript " +
		"configuration. (-1743)"
	errNoTerminal applyError = "no terminal appeared" // apply.js's created
	errSplit      applyError = "Failed to create split. (-10000)"
	errDirection  applyError = "Missing or unknown split direction. (-1715)"
	errKind       applyError = "BUILDERS[b.kind] is not a function"
	errNoTab      applyError = "undefined is not an object (evaluating 'tab.tree')"
	errNoNode     applyError = "undefined is not an object (evaluating 'node.leaf')"
	errNoPace     applyError = "undefined is not an object (evaluating 'pace.every')"
)

// halves are Ghostty's split directions: the split each makes, and whether the new terminal takes
// its first half, the left or upper one.
var halves = map[string]struct {
	split string
	first bool
}{"right": {"right", false}, "down": {"down", false}, "left": {"right", true}, "up": {"down", true}}

// firstLeaf is the tree's first leaf, left or top first, or apply.js's error where a node on the
// way is missing.
func firstLeaf(n *node) (*leaf, error) {
	switch {
	case n == nil:
		return nil, errNoNode
	case n.Leaf != nil:
		return n.Leaf, nil
	}
	return firstLeaf(n.First)
}
