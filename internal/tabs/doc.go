// Package tabs is cld restore tabs on a Mac (decision 59). It keeps cld's record of Ghostty's
// windows, tabs and panes, and shows it; a restore that reopens what a quit or a close lost
// follows (59.6). The record is tabs.json in tabs/, beside cld's record of its sessions (record.go,
// store.go). It keys each pane by its terminal's id, which Ghostty keeps for the surface's life.
//
// An observation is a snapshot of Ghostty, with ps and each pane's size read around it
// (observe.go). ps tells what runs in front of each pane: ssh to a host, or a shell at its prompt.
// cld's title on an ssh pane names the session it shows (classify.go). A merge folds an
// observation into the record (merge.go): the panes, and the windows with their split trees
// (tree.go). It also records what closed (pending.go) and what a quit lost (quit.go, last.go).
// The watcher, which Start runs from a Ghostty pane, merges one every 5 s (start.go, watch.go).
// Restore merges one too, then reports (restore.go, report.go). Ghostty's behaviour that these
// rest on is in docs/design/findings/terminals.md. The package runs neither tmux nor claude, and
// prints nothing.
package tabs
