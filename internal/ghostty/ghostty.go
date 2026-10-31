// Package ghostty reads Ghostty on a Mac for cld restore tabs (decision 59.2). Ghostty 1.4's
// AppleScript dictionary is the only interface. cld runs JXA scripts under osascript, each with one
// JSON argument in and one JSON document out. The scripts live in scripts/ and are embedded whole,
// so Go never builds script text. Ghostty's behaviour that these rest on is in
// docs/design/findings/terminals.md. The package prints nothing and holds no record: that is
// internal/tabs's.
package ghostty

// Bundle is Ghostty's bundle identifier: the scripts find its process and address it by it.
const Bundle = "com.mitchellh.ghostty"
