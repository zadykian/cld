package project

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

// preferences are the user settings SetupUser writes after permissions, with their values as JSON:
// the author's own, which a new machine then gets from one command (decision 53.3).
var preferences = []configfile.Member{
	{Key: "model", Value: json.RawMessage(`"opus"`)},
	{Key: "effortLevel", Value: json.RawMessage(`"xhigh"`)},
	{Key: "theme", Value: json.RawMessage(`"dark"`)},
	{Key: "editorMode", Value: json.RawMessage(`"normal"`)},
	{Key: "autoCompactEnabled", Value: json.RawMessage(`true`)},
	{Key: "autoUpdatesChannel", Value: json.RawMessage(`"latest"`)},
}

// Channel is a value of claude's preferredNotifChannel, which --notifications takes.
type Channel struct {
	// Name is the value, and Description what completion shows for it.
	Name, Description string
}

// Channels are the values claude 2.1.232 to 2.1.289 take, in their order. claude reads one it
// does not know as unset, without a word, so cld refuses it (decision 53.4).
var Channels = []Channel{
	{"auto", "claude's default, by the terminal: none under tmux"},
	{"iterm2", "iTerm2's notifications, OSC 9"},
	{"terminal_bell", "the bell, for Terminal.app and any other terminal"},
	{"iterm2_with_bell", "iTerm2's notifications, and the bell"},
	{"kitty", "kitty's notifications, OSC 99"},
	{"ghostty", "Ghostty's notifications, OSC 777"},
	{"notifications_disabled", "no notifications"},
}

// SetupUser sets up claude's user settings, with the permission set given and the notification
// channel, where not empty. As Setup does in a project, it adds what the settings lack, writes the
// file only where that changes it, and reports what it changed. It leaves env alone.
func SetupUser(permissions Permissions, channel string) error {
	path, err := userSettings()
	if err != nil {
		return err
	}
	file, err := configfile.ReadJSON(path)
	if err != nil {
		return err
	}
	top, err := seed(file, nil, permissions, preferences)
	if err != nil {
		return err
	}
	if channel != "" {
		top.setMissing("preferredNotifChannel", configfile.String(channel), false)
	}
	file.Members = top.members
	changes := []change{{path, !file.Exists, top.changed, file.Write}}
	if err := write(changes); err != nil {
		return err
	}
	return output.Print(report(changes))
}

// userSettings is the path of claude's user settings: settings.json in $CLAUDE_CONFIG_DIR, by
// default ~/.claude, where claude reads them.
func userSettings() (string, error) {
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
