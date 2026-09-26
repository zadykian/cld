package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
)

// Claude Code's user settings, whose env Setup edits. The edit keeps what cld does not manage (see
// internal/configfile): the members of the top-level object and of env are written back in the
// file's order, their values as the file has them, byte for byte, in the file's indentation. A
// key cld sets keeps its place, a new one goes at the end of env. The file is replaced
// atomically, a symbolic link keeps pointing at it, and a missing file and its directory are
// created, as Claude Code would.

// setting is an env key Setup manages: set to value, or removed.
type setting struct {
	key, value string
	remove     bool
}

// settings is claude's settings file, read to be edited.
type settings struct {
	*configfile.JSON
	// env are the members of the env object, the member at envAt, or -1 for none.
	env   []configfile.Member
	envAt int
}

// settingsFile is the path of claude's user settings: settings.json in $CLAUDE_CONFIG_DIR, by
// default ~/.claude.
func settingsFile() (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fail.Runtime("cannot find claude's settings: " + err.Error())
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json"), nil
}

// readSettings reads the settings at path, which need not exist, into what they are made of.
func readSettings(path string) (*settings, error) {
	file, err := configfile.ReadJSON(path)
	if err != nil {
		return nil, err
	}
	// A key given twice: JSON.parse, which claude reads the file with, keeps the last.
	s := &settings{JSON: file, envAt: configfile.Last(file.Members, "env")}
	if s.envAt >= 0 {
		if s.env, err = configfile.Object(file.Members[s.envAt].Value); err != nil {
			return nil, fail.Runtime("env in " + path + " is not a JSON object")
		}
	}
	return s, nil
}

// apply edits env as wanted says and returns what it changed, "KEY=VALUE" for a key set to a new
// value and "KEY removed", in wanted's order. A key given twice keeps its first place, and the
// value set, once.
func (s *settings) apply(wanted []setting) []string {
	var changes []string
	for _, w := range wanted {
		var env []configfile.Member
		var old json.RawMessage // The last, which claude reads.
		at := -1
		for _, m := range s.env {
			if m.Key != w.key {
				env = append(env, m)
				continue
			}
			old = m.Value
			if at < 0 && !w.remove {
				at = len(env)
				env = append(env, m)
			}
		}
		s.env = env
		if w.remove {
			if old != nil {
				changes = append(changes, w.key+" removed")
			}
			continue
		}
		if at < 0 {
			s.env = append(s.env, configfile.Member{Key: w.key})
			at = len(s.env) - 1
		}
		s.env[at].Value = configfile.String(w.value)
		var value string
		if old == nil || json.Unmarshal(old, &value) != nil || value != w.value {
			changes = append(changes, w.key+"="+w.value)
		}
	}
	return changes
}

// write replaces the settings file with the settings: every member as it was, env as apply left
// it.
func (s *settings) write() error {
	members, at := append([]configfile.Member(nil), s.Members...), s.envAt
	if at < 0 {
		members, at = append(members, configfile.Member{Key: "env"}), len(members)
	}
	members[at].Value = s.Object(s.env, 1)
	s.Members, s.envAt = members, at
	return s.JSON.Write()
}
