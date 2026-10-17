package ghostty

import (
	"bytes"
	"context"
	_ "embed" // the scripts, embedded whole
	"encoding/json"
	"errors"
	"time"

	"github.com/zadykian/cld/internal/tool"
)

// snapshotJS is snapshot.js, which reads Ghostty's process and windows for Take.
//
//go:embed scripts/snapshot.js
var snapshotJS string

// applyJS is apply.js, which types into panes and builds windows, tabs and splits for Apply.
//
//go:embed scripts/apply.js
var applyJS string

// run runs script under osascript as JavaScript with input, encoded as one JSON argument, and
// decodes the one JSON document the script prints into out. osascript missing, the deadline of
// limit, a failed run or output that is no JSON document is an *Error; ctx ending otherwise is
// ctx's error. The document may still be the script's own error, for the caller to read.
func run(ctx context.Context, limit time.Duration, script string, input, out any) error {
	arg, err := json.Marshal(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd, err := tool.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", script, string(arg))
	if err != nil {
		return &Error{Kind: NoOsascript, Message: err.Error()}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &stdout, &stderr
	err = cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &Error{Kind: Timeout, limit: limit}
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return failed(stderr.String(), err.Error())
	}
	if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
		return failed(stderr.String(), "printed no JSON document: "+err.Error())
	}
	return nil
}

// failed is osascript's own failure, as its stderr says it on one line, else as why says it.
func failed(stderr, why string) *Error {
	return &Error{Kind: Failed, Message: tool.Said(stderr, why)}
}
