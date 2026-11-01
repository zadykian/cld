// Package tabs is cld restore tabs on a Mac (decision 59). It keeps cld's record of Ghostty's
// windows, tabs and panes, and reopens what a quit or a close lost. The record is tabs.json in
// tabs/, beside cld's record of its sessions (record.go, store.go). It keys each pane by its
// terminal's id, which Ghostty keeps for the surface's life.
//
// An observation is a snapshot of Ghostty, with ps and the panes' sizes read after it (observe.go).
// ps tells what runs in front of each pane: ssh to a host, or a shell at its prompt. cld's title on
// an ssh pane names the session it shows (classify.go). A merge folds one into the record
// (merge.go, tree.go), with what closed (pending.go) and what a quit lost (quit.go, last.go). The
// sizes give each tab's splits (layout.go). The watcher, started in a Ghostty pane, merges one
// every 5 s (start.go, watch.go). Restore merges one too, opening Ghostty (open.go), and picks the
// losses to reopen (select.go). It plans the lines to type and what to build (dedupe.go, plan.go,
// lines.go). apply.js does it, and Restore records what it made (apply.go, mark.go), then reports
// (report.go, dryrun.go). It joins cld's own pane by exec (own.go). Ghostty's behaviour is in
// docs/design/findings/terminals.md. The package runs neither tmux nor claude, and prints nothing.
package tabs
