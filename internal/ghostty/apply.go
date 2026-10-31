package ghostty

// Plan is what apply.js does in Ghostty: it types each of Typed into its pane. A restore plans it
// (decision 59.6).
type Plan struct {
	Pace  Pace     `json:"pace"`
	Typed []Typing `json:"typed"`
}

// Pace spaces the lines that start an ssh, so that sshd's MaxStartups and a fail2ban see no
// burst (decision 59.7).
type Pace struct {
	Gap   float64 `json:"gap"`   // seconds between two lines
	Every int     `json:"every"` // a pause after every Every lines
	Pause float64 `json:"pause"` // seconds the pause adds to the gap
}

// Typing is a line to type into a pane Ghostty kept, whose shell waits at its prompt.
type Typing struct {
	Pane string `json:"pane"` // the terminal's id
	PID  int    `json:"pid"`  // the shell's group, in front in the snapshot; another is busy
	Line string `json:"line"`
}
