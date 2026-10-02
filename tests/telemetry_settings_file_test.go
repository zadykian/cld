package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The subtests of TestSetupTelemetrySettings on the settings file itself: where it lives, its
// links, and what changes while the collector starts.

// The settings are $CLAUDE_CONFIG_DIR/settings.json where that is set, as claude reads them,
// created with their directories as Claude Code creates them: 0644 and 0755, less the umask.
func telemetrySettingsInConfigDir(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, "config", "claude")
	port := freePort(t)
	result := s.RunCld(map[string]string{"CLAUDE_CONFIG_DIR": dir},
		"setup", "telemetry", "--local", localURL, "--port", strconv.Itoa(port))
	checkTelemetryDone(t, result)
	checkSettings(t, filepath.Join(dir, "settings.json"), settingsFor(port, true))
	mask := umask(t)
	checkMode(t, filepath.Join(dir, "settings.json"), 0o644&^mask)
	checkMode(t, dir, 0o755&^mask)
	checkMode(t, filepath.Dir(dir), 0o755&^mask)
	checkNoClaudeDir(t, s)
}

// What another program, claude say, wrote while the collector started stays (decision 18.5).
func telemetrySettingsChangedMeanwhile(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := writeSettings(t, s, `{"model": "opus"}`)
	port := freePort(t)
	written := path + "\n" + `{"model": "sonnet", "env": {"A": "1"}}`
	result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_WRITE": written},
		"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
	checkTelemetryDone(t, result)
	checkSettings(t, path, strings.Replace(settingsFor(port, false), "{\n  \"env\": {\n",
		"{\n  \"model\": \"sonnet\",\n  \"env\": {\n    \"A\": \"1\",\n", 1))
}

// Settings that cld cannot read again once the collector runs, or cannot write, end cld, saying
// that the collector runs; the settings stay as they are.
func telemetrySettingsInvalidMeanwhile(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := writeSettings(t, s, `{"model": "opus"}`)
	port := freePort(t)
	result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_WRITE": path + "\n{"},
		"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
	checkTelemetryRefused(t, result, "cld: "+path+" is not valid JSON: line 1: "+
		"unexpected end of JSON input (the collector cld-telemetry runs, "+
		"but the settings are unchanged)\n")
	checkCalls(t, s.DockerCalls(), setupCalls(port, false))
	checkCollectorRunning(t, s, port)
	checkSettings(t, path, "{")
}

// The write fails for its temporary file, whose path is too long where the settings' own is not.
func telemetrySettingsCannotWrite(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// The longest path Linux takes is 4095 bytes: the settings' has that length.
	length := 4095 - len("/settings.json")
	dir := filepath.Join(s.Root, "config")
	for len(dir) < length-202 {
		dir += "/" + strings.Repeat("d", 200)
	}
	dir += "/" + strings.Repeat("d", length-len(dir)-1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	port := freePort(t)
	result := s.RunCld(map[string]string{"CLAUDE_CONFIG_DIR": dir},
		"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
	checkTelemetryRefused(t, result, "cld: cannot write "+path+": file name too long "+
		"(the collector cld-telemetry runs, but the settings are unchanged)\n")
	checkCalls(t, s.DockerCalls(), setupCalls(port, false))
	checkCollectorRunning(t, s, port)
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("%s holds %v (%v), want nothing", dir, entries, err)
	}
}

// A settings file that is a symbolic link stays one: the file it leads to changes.
func telemetrySettingsSymlink(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	target := filepath.Join(s.Root, "dotfiles", "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(target, "{}")
	path := linkClaudeSettings(t, s, target)
	port := freePort(t)
	result := s.RunCld(nil,
		"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
	checkTelemetryDone(t, result)
	checkSettingsLink(t, path, target)
	checkSettings(t, target, settingsFor(port, false))
}

// A symbolic link to a missing file, dotfiles not checked out say, stops cld before docker.
// Writing the settings would replace the link with a file (see internal/configfile).
func telemetrySettingsSymlinkToNoFile(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	target := filepath.Join(s.Root, "dotfiles", "claude-settings.json")
	path := linkClaudeSettings(t, s, target)
	result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL)
	checkTelemetryRefused(t, result,
		"cld: "+path+" is a symbolic link to a file that does not exist\n")
	checkNoDockerCall(t, s)
	checkSettingsLink(t, path, target)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s: %v, want none", target, err)
	}
}

// linkClaudeSettings makes claude's settings file in the sandbox's HOME a symbolic link to
// target, and returns its path.
func linkClaudeSettings(t *testing.T, s *sandbox.Sandbox, target string) string {
	t.Helper()
	path := settingsPath(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// checkSettingsLink reports a path that is no symbolic link to target.
func checkSettingsLink(t *testing.T, path, target string) {
	t.Helper()
	if link, err := os.Readlink(path); err != nil || link != target {
		t.Errorf("%s leads to %q (%v), want %s", path, link, err, target)
	}
}
