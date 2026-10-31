// Package tabs is cld restore tabs on a Mac (decision 59). It reads Ghostty and reports what it
// finds, for the dry-run (restore.go, report.go). Ghostty's behaviour that this rests on is in
// docs/design/findings/terminals.md. The package runs neither tmux nor claude, and prints nothing.
package tabs
