package tabs

import (
	"context"
	"errors"

	"github.com/zadykian/cld/internal/ghostty"
)

// Restore reads Ghostty and reports what it finds. This release has the dry-run's report alone:
// whether Ghostty runs (decision 59.11).
func Restore(ctx context.Context) (*Report, error) {
	snap, err := ghostty.Take(ctx)
	if err != nil {
		return nil, failure(err)
	}
	return &Report{Lines: []string{ghosttyLine(snap)}}, nil
}

// failure is err as a restore ends with it: Ghostty's errors with cld's advice, status 1.
func failure(err error) error {
	if e, ok := errors.AsType[*ghostty.Error](err); ok {
		return e.Fail()
	}
	return err
}
