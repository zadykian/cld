package tabs

import "github.com/zadykian/cld/internal/ghostty"

// Seen is what a pane shows at an observation (decision 59.5).
type Seen struct {
	Title string
}

// classify reads what terminal t shows. For now that is its title.
func classify(t ghostty.Terminal) Seen {
	return Seen{Title: t.Name}
}
