//go:build ghostty

package terminal

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	ghostty "go.mitchellh.com/libghostty"
)

// What the Ghostty app does around libghostty-vt, which the driver copies: its environment, its
// answers to tmux's queries, its default modes and colours, and its focus reports. Read in the
// source of the commit tests/ghostty/deps.txt pins (decision 54.1).

// ghosttyVersion is the version in build.zig.zon at the commit of the libghostty-vt the tests
// link. tests/ghostty/build-lib keeps it, as the library's own differs (decision 54.2).
var ghosttyVersion = sync.OnceValues(func() (string, error) {
	dir, err := ghosttyDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "share", "ghostty", "version"))
	return strings.TrimSpace(string(data)), err
})

// ghosttyEnviron is the environment the app gives a program, from env. dir is where
// tests/ghostty/build-lib installs Ghostty's terminfo, as the app ships it beside its resources.
func ghosttyEnviron(env map[string]string, dir, version string) []string {
	vars := maps.Clone(env)
	delete(vars, "VTE_VERSION")
	delete(vars, "GHOSTTY_LOG")
	bin := filepath.Join(dir, "bin")
	maps.Copy(vars, map[string]string{
		"TERM":                   "xterm-ghostty",
		"COLORTERM":              "truecolor",
		"TERM_PROGRAM":           "ghostty",
		"TERM_PROGRAM_VERSION":   version,
		"TERMINFO":               filepath.Join(dir, "share", "terminfo"),
		"GHOSTTY_RESOURCES_DIR":  filepath.Join(dir, "share", "ghostty"),
		"GHOSTTY_BIN_DIR":        bin,
		"GHOSTTY_SHELL_FEATURES": "cursor:blink,path,title",
		"GHOSTTY_SURFACE_ID":     "0x0000000000000001",
		"PATH":                   bin + string(os.PathListSeparator) + env["PATH"],
	})
	environ := make([]string, 0, len(vars))
	for name, value := range vars {
		environ = append(environ, name+"="+value)
	}
	slices.Sort(environ)
	return environ
}

// ghosttyOptions are the app's answers and default modes. Without them libghostty-vt answers
// XTVERSION as "libghostty", and the device attributes as a plain VT220.
func ghosttyOptions(version string) []ghostty.TerminalOption {
	return []ghostty.TerminalOption{
		ghostty.WithTerminfoName("xterm-ghostty"),
		ghostty.WithXtversion(func(*ghostty.Terminal) string { return "ghostty " + version }),
		ghostty.WithDeviceAttributes(func(*ghostty.Terminal) (ghostty.DeviceAttributes, bool) {
			return ghostty.DeviceAttributes{
				Primary: ghostty.DeviceAttributesPrimary{
					ConformanceLevel: 62, Features: [64]uint16{22, 52}, NumFeatures: 2,
				},
				Secondary: ghostty.DeviceAttributesSecondary{DeviceType: 1, FirmwareVersion: 10},
			}, true
		}),
		ghostty.WithModeDefault(ghostty.ModeGraphemeCluster, true),
	}
}

// setDefaults turns the cursor's blinking on, which libghostty-vt does not take as a default, and
// sets the app's default colours, which answer OSC 10 and 11.
func (g *ghosttyTerminal) setDefaults() {
	g.t.Helper()
	if err := g.term.SetMode(ghostty.ModeCursorBlinking, true); err != nil {
		g.t.Fatal(err)
	}
	foreground := &ghostty.ColorRGB{R: 0xff, G: 0xff, B: 0xff}
	background := &ghostty.ColorRGB{R: 0x28, G: 0x2c, B: 0x34}
	if err := g.term.SetColorForeground(foreground); err != nil {
		g.t.Fatal(err)
	}
	if err := g.term.SetColorBackground(background); err != nil {
		g.t.Fatal(err)
	}
}

// focusRequest is how tmux asks for focus reports, again at each answer to its queries.
const focusRequest = "\x1b[?1004h"

// reportFocus sends the focus state at each request for focus reports in output, as the app does
// and libghostty-vt does not (decision 54.5). tail keeps a request split between two outputs.
// Reporting turned on another way, with other modes in one request, gets a report too.
// tmux's request is the exact sequence, its Enfcs (tty-features.c), so only that one is counted.
func (g *ghosttyTerminal) reportFocus(output []byte) {
	data := append(g.tail, output...)
	requests := bytes.Count(data, []byte(focusRequest))
	g.tail = bytes.Clone(data[max(0, len(data)-len(focusRequest)+1):])
	on, err := g.term.Mode(ghostty.ModeFocusEvent)
	if err != nil {
		g.t.Errorf("ghostty: focus reporting: %v", err)
	}
	if on && !g.reporting {
		requests = max(requests, 1)
	}
	g.reporting = on
	for range requests {
		g.sendFocus()
	}
}

// sendFocus sends the focus state. The app starts focused.
func (g *ghosttyTerminal) sendFocus() {
	event := ghostty.FocusGained
	if g.unfocused {
		event = ghostty.FocusLost
	}
	input, err := ghostty.FocusEncode(event)
	if err != nil {
		g.t.Errorf("ghostty: a focus report: %v", err)
		return
	}
	g.write(input)
}

// Focus changes the focus, which the app reports only while the program asks for it.
func (g *ghosttyTerminal) Focus(focused bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.unfocused = !focused
	if g.reporting {
		g.sendFocus()
	}
}
