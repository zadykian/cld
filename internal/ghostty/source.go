package ghostty

import (
	"context"
	"time"
)

// Source gives the watcher a snapshot per tick. Wait waits for the tick and Next reads it, so
// that the watcher checks its own binary and Ghostty's pid between them, before any Apple Event.
// Poll runs snapshot.js at each Next; decision 59.3 says why it comes before a long-lived script.
type Source interface {
	Wait(ctx context.Context) error              // until the next tick, or ctx's end
	Next(ctx context.Context) (*Snapshot, error) // the tick's snapshot, as Take gives it
	Close() error
}

// Poll is the source that takes a snapshot at each Next. Its first Wait returns at once, and each
// later one an interval after the last Next ended, so that a slow tick never queues more.
func Poll(every time.Duration) Source {
	return &poll{every: every}
}

// poll is Poll's source. due is when Wait returns next, zero before the first tick.
type poll struct {
	every time.Duration
	due   time.Time
}

func (p *poll) Wait(ctx context.Context) error {
	if wait := time.Until(p.due); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	p.due = time.Now().Add(p.every)
	return ctx.Err()
}

func (p *poll) Next(ctx context.Context) (*Snapshot, error) {
	defer func() { p.due = time.Now().Add(p.every) }()
	return Take(ctx)
}

// Close ends nothing: each Next's osascript has ended with it.
func (p *poll) Close() error { return nil }
