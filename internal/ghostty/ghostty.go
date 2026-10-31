// Package ghostty reads and drives Ghostty on a Mac for cld restore tabs (decision 59.2). Ghostty
// 1.4's AppleScript dictionary is the only interface. cld runs JXA scripts under osascript, each
// with one JSON argument in and one JSON document out (script.go). The scripts live in scripts/
// and are embedded whole, so Go never builds script text. snapshot.js reads Ghostty's process and
// its windows front to back, each tab's split tree as its leaves (snapshot.go). Source hands the
// watcher one such snapshot per tick (source.go). apply.js types into the panes Ghostty kept, and
// builds the windows, tabs and splits a restore reopens (apply.go).
//
// Ghostty's errors come back by the step that failed and JXA's error number. They are never told
// apart by English, but for macos-applescript's text; errors.go gives each kind cld's message and
// advice. cli.go runs Ghostty's own CLI actions, such as +show-config. Ghostty's behaviour that
// these rest on is in docs/design/findings/terminals.md. The package prints nothing and holds no
// record: that is internal/tabs's.
package ghostty

// Bundle is Ghostty's bundle identifier: the scripts find its process and address it by it.
const Bundle = "com.mitchellh.ghostty"
