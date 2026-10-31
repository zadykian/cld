package mac

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// The files of $CLD_PROBE_DIR that the fakes read, and the logs they append, a line each. The sh
// fakes, tests/fakes/open and tests/fakes/ghostty, spell their files' names alike.
const (
	ModelFile = "ghostty.json"      // the model
	NextFile  = "ghostty.next.json" // the model the fake open puts in place, as Ghostty starts
	// FailFile's first line is the fake osascript's failure mode, read at each call before
	// $CLD_FAKE_OSASCRIPT_FAIL.
	FailFile = "osascript-fail"
	// DelayFile holds the seconds that the next snapshot waits before it answers, and goes then.
	DelayFile  = "osascript-delay"
	ShowConfig = "show-config" // what the fake ghostty +show-config prints
	// SSHCache has the keys the fake ghostty +ssh-cache --host=KEY finds, a line each.
	SSHCache = "ssh-cache"
	// SSHHosts has a line HOST HOSTNAME PORT per host whose ssh -G names another hostname or port.
	SSHHosts     = "ssh-hosts"
	OsascriptLog = "osascript.jsonl"
	PsLog        = "ps.jsonl"
	SSHLog       = "ssh.jsonl"
	OpenLog      = "open.log"    // the fake open's arguments, a line per call
	GhosttyLog   = "ghostty.log" // the fake ghostty's arguments, a line per call
)

// PsArgs are the arguments cld runs ps with, and the only ones the fake ps takes. Each -o has one
// keyword, as BSD's ps runs the text after the = on through any commas.
var PsArgs = []string{"-A", "-ww", "-o", "pid=", "-o", "ppid=", "-o", "pgid=", "-o", "tty=",
	"-o", "lstart=", "-o", "args="}

// WriteFile writes content as the file name of dir by a temporary file renamed into place, so that
// a fake never reads it half written.
func WriteFile(dir, name, content string) error {
	file, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	_, err = file.WriteString(content)
	if err := errors.Join(err, file.Close()); err != nil {
		return errors.Join(err, os.Remove(file.Name()))
	}
	return os.Rename(file.Name(), filepath.Join(dir, name))
}

// OsascriptCall is a call of the fake osascript: its script's marker, snapshot or apply, its
// input and its environment.
type OsascriptCall struct {
	Script string            `json:"script"`
	Input  json.RawMessage   `json:"input"`
	Env    map[string]string `json:"env"`
}

// PsCall is a call of the fake ps: its arguments, and TZ and LC_ALL where they are set.
type PsCall struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
}

// SSHCall is a call of the fake ssh: its arguments and its TERM.
type SSHCall struct {
	Argv []string `json:"argv"`
	Term string   `json:"term"`
}
