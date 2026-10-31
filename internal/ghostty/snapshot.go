package ghostty

import (
	"context"
	"time"
)

// SnapshotTimeout bounds one snapshot.js run. A first Automation prompt can block osascript, so
// the message on a timeout says to answer it and run again.
const SnapshotTimeout = 15 * time.Second

// Snapshot is what snapshot.js reads of Ghostty. Where Running is false, no Ghostty process runs
// and the script sent no Apple Event. Error is set where the script failed, and Take returns it as
// an *Error instead.
type Snapshot struct {
	Running bool         `json:"running"`
	Error   *ScriptError `json:"error,omitempty"`
}

// Take runs snapshot.js. A Snapshot whose Error is set is returned as an *Error instead.
func Take(ctx context.Context) (*Snapshot, error) {
	snap := &Snapshot{}
	if err := run(ctx, SnapshotTimeout, snapshotJS, struct{}{}, snap); err != nil {
		return nil, err
	}
	if snap.Error != nil {
		return nil, classify(snap.Error)
	}
	return snap, nil
}
