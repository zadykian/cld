package tabs

import (
	"fmt"
	"strings"

	"github.com/zadykian/cld/internal/session"
)

// Binding is what a pane joins again: Session on Host, or, with Session "", Host alone.
type Binding struct{ Host, Session string }

// String names b as the report does: session 'S' on HOST, or ssh to HOST.
func (b Binding) String() string {
	if b.Session != "" {
		return fmt.Sprintf("session '%s' on %s", b.Session, b.Host)
	}
	return "ssh to " + b.Host
}

// Words is the ssh that joins b again, as argv. A login shell on the host runs cld join
// --existing, never --detach-others, then a shell (decisions 58 and 59.7). In /, -s alone names
// a NAME that does not split. A plain ssh pane gets ssh HOST.
func Words(b Binding) []string {
	if b.Session == "" {
		return []string{"ssh", b.Host}
	}
	opts := session.Options(b.Session)
	join := "cld join --existing " + opts + "; "
	if !strings.HasPrefix(opts, "-n ") {
		join = "cd / && " + join + "cd; "
	}
	return []string{"ssh", "-t", "-o", "ConnectTimeout=10", b.Host,
		`exec $SHELL -lc "` + join + `exec $SHELL -l"`}
}

// Line is Words as typed into a pane's shell, the remote command in single quotes, so that the
// host's shell expands $SHELL. Names and hosts are validated, and need no quoting.
func Line(b Binding) string {
	words := Words(b)
	if len(words) > 2 {
		words[len(words)-1] = "'" + words[len(words)-1] + "'"
	}
	return strings.Join(words, " ")
}
