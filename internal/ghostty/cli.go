package ghostty

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/tool"
)

// settingsTimeout bounds ghostty +show-config.
const settingsTimeout = 10 * time.Second

// Settings are what +show-config says of the settings restore tabs's notes name. It prints only
// what differs from Ghostty's defaults, so a setting it leaves out has its default: UNVERIFIED
// until #177's check 13 (decision 59.12).
type Settings struct {
	Read           bool   // false: no ghostty ran +show-config
	SaveState      string // window-save-state: "default" where no line names it
	Title          bool   // a title line: the config fixes every pane's title
	SSHTerminfo    bool   // shell-integration-features holds ssh-terminfo
	HiddenTitlebar bool   // macos-titlebar-style = hidden
}

// Binary is the ghostty program for Ghostty's CLI actions: ghostty on the PATH, which in Ghostty's
// panes finds the app's own, else the app's executable. Both are the app's one binary, as
// docs/design/findings/terminals.md has it. It reports false where neither is a file.
func Binary(app App) (string, bool) {
	if path, err := tool.LookPath("ghostty"); err == nil {
		return path, true
	}
	if !filepath.IsAbs(app.Executable) {
		return "", false
	}
	info, err := os.Stat(app.Executable)
	return app.Executable, err == nil && info.Mode().IsRegular()
}

// ReadSettings runs bin +show-config, bin being Binary's, and reads its lines KEY = VALUE.
func ReadSettings(ctx context.Context, bin string) (Settings, error) {
	ctx, cancel := context.WithTimeout(ctx, settingsTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "+show-config")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Settings{}, fmt.Errorf("%s +show-config: %w", bin, ctx.Err())
		}
		return Settings{}, fmt.Errorf("%s +show-config: %s", bin,
			tool.Said(stderr.String(), err.Error()))
	}
	settings := Settings{Read: true, SaveState: "default"}
	for line := range strings.Lines(stdout.String()) {
		key, value, _ := strings.Cut(line, "=")
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "window-save-state":
			settings.SaveState = value
		case "title":
			settings.Title = true
		case "shell-integration-features":
			settings.SSHTerminfo = slices.Contains(strings.Split(value, ","), "ssh-terminfo")
		case "macos-titlebar-style":
			settings.HiddenTitlebar = value == "hidden"
		}
	}
	return settings, nil
}
