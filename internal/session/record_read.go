package session

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// recorded is the entry of session cld-SUFFIX, and whether there is one: none where it has
// expired, or cannot be read.
func recorded(suffix string) (entry, bool) {
	dir, err := stateDir()
	if err != nil {
		return entry{}, false
	}
	return readEntry(entryFile(dir, suffix), suffix)
}

// readEntry reads the entry of session cld-SUFFIX from file. One that names another session is
// none, as a socket directory that ignores case reads another's file (decision 40.7).
func readEntry(file, suffix string) (entry, bool) {
	info, err := os.Stat(file)
	if err != nil || time.Since(info.ModTime()) > expiry {
		return entry{}, false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return entry{}, false
	}
	var r entry
	if json.Unmarshal(data, &r) != nil || r.Name != suffix || !filepath.IsAbs(r.Directory) {
		return entry{}, false
	}
	if !conversationID.MatchString(r.Conversation) {
		r.Conversation = ""
	}
	return r, true
}

// entries are the entries of the record that have not expired, in the order of their names.
func entries() []entry {
	dir, err := stateDir()
	if err != nil {
		return nil
	}
	files, err := os.ReadDir(filepath.Join(dir, "sessions"))
	if err != nil {
		return nil
	}
	var found []entry
	for _, file := range files {
		suffix, ok := strings.CutSuffix(file.Name(), ".json")
		if !ok || !ValidName(suffix) {
			continue
		}
		if r, ok := readEntry(entryFile(dir, suffix), suffix); ok {
			found = append(found, r)
		}
	}
	return found
}

// indexes are the highest indexes given, by the start of the names they follow - "NAME-", or ""
// for the names that are an index alone - less those given longer ago than expiry.
func indexes(dir string) map[string]given {
	data, err := os.ReadFile(filepath.Join(dir, "indexes.json"))
	if err != nil {
		return map[string]given{}
	}
	all := map[string]given{}
	if json.Unmarshal(data, &all) != nil {
		return map[string]given{}
	}
	for prefix, g := range all {
		if time.Since(g.Time) > expiry {
			delete(all, prefix)
		}
	}
	return all
}

// indexOf splits name into the start that Next takes, "" or ending in "-", and the index that
// follows it; false where no index ends the name.
func indexOf(name string) (prefix string, index int, ok bool) {
	prefix = strings.TrimRight(name, "0123456789")
	if prefix == name || prefix != "" && !strings.HasSuffix(prefix, "-") {
		return "", 0, false
	}
	// An index too large for an int, or the largest, which no index is above, is none.
	index, err := strconv.Atoi(name[len(prefix):])
	if err != nil || index == math.MaxInt {
		return "", 0, false
	}
	return prefix, index, true
}

// readStarted reads what the server of session cld-SUFFIX started with, from the record's
// directory dir (see started).
func readStarted(dir, suffix string) (started, error) {
	data, err := os.ReadFile(companion(dir, suffix, environment))
	if err != nil {
		return started{}, reason(err)
	}
	var s started
	if err := json.Unmarshal(data, &s); err != nil || !filepath.IsAbs(s.Claude) {
		return started{}, errors.New(companion(dir, suffix, environment) + ": not what cld writes")
	}
	return s, nil
}
