package ghostty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/tool"
)

// The bounds of the programs cli.go runs: ghostty +show-config, then each of ssh -G and ghostty
// +ssh-cache, then open.
const (
	settingsTimeout = 10 * time.Second
	cacheTimeout    = 5 * time.Second
	openTimeout     = 10 * time.Second
)

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

// TerminfoCached reports whether Ghostty's terminfo cache holds host, as bin +ssh-cache finds the
// key that Ghostty's ssh wrapper gives it. A failure on the way counts as not cached.
func TerminfoCached(ctx context.Context, bin, host string) bool {
	if bin == "" {
		return false
	}
	key, ok := cacheKey(ctx, host)
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	// v1.3.1 takes --host= alone, and 1.4.0 takes it too, deprecated, with a warning on stderr,
	// which is discarded with the output: the status answers.
	return exec.CommandContext(ctx, bin, "+ssh-cache", "--host="+key).Run() == nil
}

// cacheKey is the key of host in Ghostty's terminfo cache, as 1.4.0's src/cli/ssh.zig forms it.
// That is ssh -G's user@hostname, with :port for a port other than 22, and an IPv6 hostname goes
// in brackets.
func cacheKey(ctx context.Context, host string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	cmd, err := tool.CommandContext(ctx, "ssh", "-G", host)
	if err != nil {
		return "", false
	}
	var stdout bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &stdout, nil
	if cmd.Run() != nil {
		return "", false
	}
	fields := map[string]string{}
	for line := range strings.Lines(stdout.String()) {
		key, value, found := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		if _, seen := fields[key]; found && !seen {
			fields[key] = value
		}
	}
	user, hostname, port := fields["user"], fields["hostname"], fields["port"]
	switch {
	case user == "" || hostname == "":
		return "", false
	case port == "" || port == "22":
		return user + "@" + hostname, true
	case strings.Contains(hostname, ":"):
		return user + "@[" + hostname + "]:" + port, true
	}
	return user + "@" + hostname + ":" + port, true
}

// Open starts Ghostty by its bundle identifier, the one the scripts address it by, or brings
// the Ghostty that runs to the front.
func Open(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	cmd, err := tool.CommandContext(ctx, "open", "-b", Bundle)
	if err != nil {
		return errors.New("open is not installed")
	}
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("open -b %s did not answer in %d s", Bundle,
				int(openTimeout.Seconds()))
		}
		return fmt.Errorf("open -b %s: %s", Bundle, tool.Said(stderr.String(), err.Error()))
	}
	return nil
}
