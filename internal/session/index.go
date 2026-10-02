package session

import (
	"cmp"
	"context"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Next is the NAME join gives a session without -s: prefix, then the index above the highest in
// use, or 0 (decisions 24.1 and 40.6). prefix is compared ignoring case, as a socket directory may
// ignore it (decision 13.6). join calls Next under the record's lock. Once ctx is done, its tmux is
// killed.
func (t *Tmux) Next(ctx context.Context, prefix string) (string, error) {
	sockets, err := readSockets()
	if err != nil {
		return "", err
	}
	next := recordedNext(prefix)
	// The highest server that runs decides, mostly the one lookup: a stale socket runs no tmux
	// (decision 38.3).
	for _, socket := range indexedSockets(sockets, prefix, next) {
		server, _, _, _, err := t.lookup(ctx, socket.suffix)
		if err != nil {
			return "", err
		}
		if server {
			return prefix + strconv.Itoa(socket.index+1), nil
		}
	}
	return prefix + strconv.Itoa(next), nil
}

// recordedNext is the index above those that cld's record keeps after prefix, in its entries and
// in the indexes it says were given (see indexes). It gives 0 where there are none.
func recordedNext(prefix string) int {
	next := 0
	for _, r := range entries() {
		if start, index, ok := indexOf(r.Name); ok && strings.EqualFold(start, prefix) {
			next = max(next, index+1)
		}
	}
	if state, err := stateDir(); err == nil {
		for start, g := range indexes(state) {
			if strings.EqualFold(start, prefix) && g.Index < math.MaxInt {
				next = max(next, g.Index+1)
			}
		}
	}
	return next
}

// indexed is the NAME of a server's socket, cld-NAME, and the index it ends in.
type indexed struct {
	suffix string
	index  int
}

// indexedSockets are the sockets named prefix and an index at or above next, the highest first.
func indexedSockets(sockets []os.DirEntry, prefix string, next int) []indexed {
	var named []indexed
	for _, socket := range sockets {
		suffix, found := strings.CutPrefix(socket.Name(), "cld-")
		if !found || !ValidName(suffix) {
			continue
		}
		if index, ok := indexAfter(suffix, prefix); ok && index >= next {
			named = append(named, indexed{suffix, index})
		}
	}
	slices.SortStableFunc(named, func(a, b indexed) int { return cmp.Compare(b.index, a.index) })
	return named
}

// indexAfter is the index that name ends in after prefix, compared ignoring case, and whether
// there is one.
func indexAfter(name, prefix string) (int, bool) {
	if len(name) <= len(prefix) || !strings.EqualFold(name[:len(prefix)], prefix) {
		return 0, false
	}
	digits := name[len(prefix):]
	if strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	// An index too large for an int, or the largest, which no index is above, is none.
	index, err := strconv.Atoi(digits)
	if err != nil || index == math.MaxInt {
		return 0, false
	}
	return index, true
}
