package tabs

import "fmt"

// Binding is what a pane joins again: Session on Host, or, with Session "", Host alone.
type Binding struct{ Host, Session string }

// String names b as the report does: session 'S' on HOST, or ssh to HOST.
func (b Binding) String() string {
	if b.Session != "" {
		return fmt.Sprintf("session '%s' on %s", b.Session, b.Host)
	}
	return "ssh to " + b.Host
}
