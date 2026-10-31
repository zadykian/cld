package tabs

import (
	"fmt"
	"strings"

	"github.com/zadykian/cld/internal/ghostty"
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

// paneBinding is the binding pane p shows: a cld pane's session on its host, or an ssh pane's
// host. False where p shows neither, or no host that a line can name.
func paneBinding(p *Pane) (Binding, bool) {
	switch {
	case p == nil || p.Host == "":
		return Binding{}, false
	case p.Kind == KindCld:
		return Binding{Host: p.Host, Session: p.Session}, true
	case p.Kind == KindSSH:
		return Binding{Host: p.Host}, true
	}
	return Binding{}, false
}

// fallbackTerm is the TERM of an ssh to a host whose terminfo Ghostty has not cached. It goes past
// Ghostty's ssh wrapper, which would append its install script to the command (decision 59.7).
// uncachedTerm starts the line for such a host.
const (
	fallbackTerm = "xterm-256color"
	uncachedTerm = "TERM=" + fallbackTerm + " command "
)

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
// host's shell expands $SHELL. Names and hosts are validated, and need no quoting. cached is
// whether Ghostty has the host's terminfo.
func Line(b Binding, cached bool) string {
	words := Words(b)
	if len(words) > 2 {
		words[len(words)-1] = "'" + words[len(words)-1] + "'"
	}
	line := strings.Join(words, " ")
	if !cached {
		line = uncachedTerm + line
	}
	return line
}

// uncachedNote is the restore's note on host, whose terminfo Ghostty has not cached.
func uncachedNote(host string) string {
	return host + " has no Ghostty terminfo cached: cld types " + uncachedTerm + "ssh;" +
		" with shell-integration-features = ssh-terminfo, run ssh " + host +
		" once and later restores keep Ghostty's TERM"
}

// cachedHost reports whether Ghostty has host's terminfo, asking once per host, and noting a
// host it lacks.
func (pl *planner) cachedHost(host string) bool {
	cached, asked := pl.hosts[host]
	if !asked {
		cached = pl.cached != nil && pl.cached(host)
		pl.hosts[host] = cached
		if !cached {
			pl.out.uncached = append(pl.out.uncached, host)
		}
	}
	return cached
}

// keyer turns a placement's record trees into apply.js's, keying each leaf with prefix and the
// next number, and keeps each leaf's binding in b.
type keyer struct {
	pl     *planner
	prefix string
	next   int
	b      building
}

// node is tree n as apply.js builds it, keys taking each leaf's terminal id to its key. A leaf
// opens a shell in its pane's directory, which types the line that joins its binding again.
func (k *keyer) node(n *Node, keys map[string]string) *ghostty.Node {
	if !isLeaf(n) {
		return &ghostty.Node{Split: n.Split, First: k.node(n.First, keys),
			Second: k.node(n.Second, keys)}
	}
	key := fmt.Sprintf("%s%d", k.prefix, k.next)
	k.next++
	keys[n.Pane] = key
	leaf := &ghostty.Leaf{Key: key}
	if n.Copy != nil {
		leaf.Cwd = n.Copy.Cwd
	}
	if b, ok := paneBinding(n.Copy); ok {
		k.b.bindings[key] = b
		leaf.Line = Line(b, k.pl.cachedHost(b.Host))
	}
	return &ghostty.Node{Leaf: leaf}
}

// firstKey is the key of tree n's first leaf.
func firstKey(n *ghostty.Node) string {
	for n.Leaf == nil {
		n = n.First
	}
	return n.Leaf.Key
}
