package tabs

// Merge folds o into r as a tick does (decision 59.4). A run it has not seen before takes the
// place of the one before.
func Merge(r *Record, o *Observation) {
	switch {
	case r.Run.ID != o.Run:
		r.Run = Run{ID: o.Run, Version: o.Version}
	case r.Run.Version == "":
		r.Run.Version = o.Version
	}
}
