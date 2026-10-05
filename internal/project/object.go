package project

import (
	"encoding/json"
	"slices"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
)

// object is a JSON object of a file that setup config edits: its members, its key in the file,
// such as permissions ("" for the file's own), and its depth. It keeps the keys changed so far.
type object struct {
	file    *configfile.JSON
	members []configfile.Member
	path    string
	depth   int
	changed []string
}

// name is key as a key of the file: permissions.allow for allow in permissions.
func (o *object) name(key string) string {
	if o.path == "" {
		return key
	}
	return o.path + "." + key
}

// put puts nested, the object at key that object gave, back at key, as the file holds it, where
// it changed.
func (o *object) put(key string, nested *object) {
	if len(nested.changed) == 0 {
		return
	}
	o.putValue(key, o.file.Object(nested.members, nested.depth), false)
	o.changed = append(o.changed, nested.changed...)
}

// putValue sets key to value, as the file holds it. Where the object has the key, it replaces the
// last of it, which claude reads; else it adds a member at the end, or at the start with first.
func (o *object) putValue(key string, value json.RawMessage, first bool) {
	if at := configfile.Last(o.members, key); at >= 0 {
		o.members[at].Value = value
		return
	}
	member := configfile.Member{Key: key, Value: value}
	if first {
		o.members = slices.Insert(o.members, 0, member)
	} else {
		o.members = append(o.members, member)
	}
}

// set sets key to value, JSON of cld's, where the object lacks it or it differs.
func (o *object) set(key string, value json.RawMessage, first bool) {
	if at := configfile.Last(o.members, key); at >= 0 && configfile.Same(o.members[at].Value, value) {
		return
	}
	o.putValue(key, o.file.Value(value, o.depth+1), first)
	o.changed = append(o.changed, o.name(key))
}

// setMissing sets key to value, JSON of cld's, where the object lacks it: a value the object has
// is the file owner's, and stays.
func (o *object) setMissing(key string, value json.RawMessage, first bool) {
	if configfile.Last(o.members, key) < 0 {
		o.set(key, value, first)
	}
}

// add adds to the array at key the entries, strings, it lacks, at its end; an object without the
// key gets an array of them. Without entries it reads nothing.
func (o *object) add(key string, entries []string) error {
	var elements []json.RawMessage
	if at := configfile.Last(o.members, key); at >= 0 && len(entries) > 0 {
		var err error
		if elements, err = configfile.Array(o.members[at].Value); err != nil {
			return fail.Runtime(o.name(key) + " in " + o.file.Path + " is not a JSON array")
		}
	}
	added := false
	for _, entry := range entries {
		value := configfile.String(entry)
		same := func(e json.RawMessage) bool { return configfile.Same(e, value) }
		if !slices.ContainsFunc(elements, same) {
			elements, added = append(elements, value), true
		}
	}
	if added {
		o.putValue(key, o.file.Array(elements, o.depth+1), false)
		o.changed = append(o.changed, o.name(key))
	}
	return nil
}

// object is the object at key, to be edited and then put back with put; an empty one where the
// object lacks the key.
func (o *object) object(key string) (*object, error) {
	nested := &object{file: o.file, path: o.name(key), depth: o.depth + 1}
	if at := configfile.Last(o.members, key); at >= 0 {
		members, err := configfile.Object(o.members[at].Value)
		if err != nil {
			return nil, fail.Runtime(nested.path + " in " + o.file.Path + " is not a JSON object")
		}
		nested.members = members
	}
	return nested, nil
}
