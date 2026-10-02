package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup telemetry edits env in claude's settings in place (decision 18.6). The keys it manages
// take their values where they are or at env's end, or leave. Every other key, in env and around
// it, keeps its place and value, with the file's indentation and the <, > and & of its keys.
func TestSetupTelemetrySettings(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range telemetrySettingsEdits {
		t.Run(test.name, test.run)
	}
	t.Run("CLAUDE_CONFIG_DIR", telemetrySettingsInConfigDir)
	t.Run("changed meanwhile", telemetrySettingsChangedMeanwhile)
	t.Run("invalid meanwhile", telemetrySettingsInvalidMeanwhile)
	t.Run("cannot write", telemetrySettingsCannotWrite)
	t.Run("a key given twice", telemetrySettingsKeyGivenTwice)
	t.Run("symbolic link", telemetrySettingsSymlink)
	t.Run("symbolic link to no file", telemetrySettingsSymlinkToNoFile)
	for _, test := range telemetrySettingsRefusals {
		t.Run(strconv.Quote(test.content), test.run)
	}
}

// telemetrySettingsBefore are the settings each of telemetrySettingsEdits edits.
const telemetrySettingsBefore = `{
    "$schema": "https://json.schemastore.org/claude-code-settings.json",
    "env": {
        "DOTNET_RUNTIME_ID": "linux-x64",
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://jaeger.proxy:4317",
        "OTEL_RESOURCE_ATTRIBUTES": "team=cld",
        "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://elsewhere:4317",
        "OTEL_METRIC_EXPORT_INTERVAL": "10000",
        "OTEL_TRACES_EXPORTER": "none",
        "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "http/protobuf",
        "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
        "OTEL_LOGS_EXPORTER": "console",
        "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://elsewhere:4317",
        "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL": "http/json",
        "OTEL_LOG_TOOL_DETAILS": "1",
        "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "http://elsewhere:4317",
        "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "0",
        "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL": "grpc",
        "<&>": "a key written back as it was"
    },
    "permissions": {
        "allow": ["Bash(make test)", "Read(<&>)"],
        "deny": []
    },
    "model": "opus",
    "a <&> b": true
}
`

// signalKeysRemoved are the report's lines for the six per-signal keys, which always leave.
const signalKeysRemoved = `  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT removed
  OTEL_EXPORTER_OTLP_TRACES_PROTOCOL removed
  OTEL_EXPORTER_OTLP_METRICS_ENDPOINT removed
  OTEL_EXPORTER_OTLP_METRICS_PROTOCOL removed
  OTEL_EXPORTER_OTLP_LOGS_ENDPOINT removed
  OTEL_EXPORTER_OTLP_LOGS_PROTOCOL removed
`

// telemetrySettingsEdit is a setup with flag over telemetrySettingsBefore: env is its env
// afterwards and changes the report's lines for it, {port} for the collector's port.
type telemetrySettingsEdit struct {
	name, flag string
	env        string
	changes    string
}

// telemetrySettingsEdits are the edits of TestSetupTelemetrySettings. Without --local, the four
// keys that only --local sets leave.
var telemetrySettingsEdits = []telemetrySettingsEdit{
	{"local", "--local", `
        "DOTNET_RUNTIME_ID": "linux-x64",
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:{port}",
        "OTEL_RESOURCE_ATTRIBUTES": "team=cld",
        "OTEL_METRIC_EXPORT_INTERVAL": "10000",
        "OTEL_TRACES_EXPORTER": "otlp",
        "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
        "OTEL_LOGS_EXPORTER": "otlp",
        "OTEL_LOG_TOOL_DETAILS": "1",
        "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
        "<&>": "a key written back as it was",
        "OTEL_METRICS_EXPORTER": "otlp",
        "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"
`, `  OTEL_METRICS_EXPORTER=otlp
  OTEL_EXPORTER_OTLP_PROTOCOL=grpc
  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:{port}
  OTEL_TRACES_EXPORTER=otlp
  OTEL_LOGS_EXPORTER=otlp
  CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
` + signalKeysRemoved},
	{"remote", "--remote", `
        "DOTNET_RUNTIME_ID": "linux-x64",
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:{port}",
        "OTEL_RESOURCE_ATTRIBUTES": "team=cld",
        "OTEL_METRIC_EXPORT_INTERVAL": "10000",
        "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
        "<&>": "a key written back as it was",
        "OTEL_METRICS_EXPORTER": "otlp",
        "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"
`, `  OTEL_METRICS_EXPORTER=otlp
  OTEL_EXPORTER_OTLP_PROTOCOL=grpc
  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:{port}
  OTEL_TRACES_EXPORTER removed
  OTEL_LOGS_EXPORTER removed
  CLAUDE_CODE_ENHANCED_TELEMETRY_BETA removed
  OTEL_LOG_TOOL_DETAILS removed
` + signalKeysRemoved},
}

// run runs the setup of test over telemetrySettingsBefore.
func (test telemetrySettingsEdit) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := writeSettings(t, s, telemetrySettingsBefore)
	// Beyond what the umask lets a new file have.
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(freePort(t))
	url := localURL
	if test.flag == "--remote" {
		url = remoteURL
	}
	result := s.RunCld(nil, "setup", "telemetry", test.flag, url, "--port", port)
	checkTelemetryDone(t, result)
	want := "Changed in " + path + ", under env:\n" +
		strings.ReplaceAll(test.changes, "{port}", port) +
		"claude reads these settings as a session starts"
	if !strings.Contains(result.Stdout, want) {
		t.Errorf("stdout\n%s\nwant it to hold\n%s", result.Stdout, want)
	}
	start := strings.Index(telemetrySettingsBefore, "\"env\": {") + len("\"env\": {")
	end := strings.Index(telemetrySettingsBefore, "    },\n    \"permissions\"")
	env := strings.ReplaceAll(test.env, "{port}", port)
	checkSettings(t, path, telemetrySettingsBefore[:start]+env+telemetrySettingsBefore[end:])
	checkMode(t, path, 0o666)
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings.json*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("left %q", leftovers)
	}
}

// A key given twice: cld edits the last env, which claude's JSON.parse reads (see
// internal/configfile). A key it manages keeps its first place there, and its other entries
// leave; the report says what claude read before. Other keys stay, as often as they are given.
func telemetrySettingsKeyGivenTwice(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	const before = `{
  "env": {"OTEL_METRICS_EXPORTER": "none"},
  "model": "opus",
  "env": {
    "OTEL_METRICS_EXPORTER": "otlp",
    "A": "1",
    "OTEL_METRICS_EXPORTER": "none",
    "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://a:4317",
    "A": "2",
    "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://b:4317"
  }
}
`
	path := writeSettings(t, s, before)
	port := strconv.Itoa(freePort(t))
	result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL, "--port", port)
	checkTelemetryDone(t, result)
	changes := "Changed in " + path + ", under env:\n" +
		"  CLAUDE_CODE_ENABLE_TELEMETRY=1\n  OTEL_METRICS_EXPORTER=otlp\n" +
		"  OTEL_EXPORTER_OTLP_PROTOCOL=grpc\n" +
		"  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:" + port + "\n" +
		"  OTEL_EXPORTER_OTLP_METRICS_ENDPOINT removed\n"
	if !strings.Contains(result.Stdout, changes) {
		t.Errorf("stdout\n%s\nwant it to hold\n%s", result.Stdout, changes)
	}
	checkSettings(t, path, `{
  "env": {"OTEL_METRICS_EXPORTER": "none"},
  "model": "opus",
  "env": {
    "OTEL_METRICS_EXPORTER": "otlp",
    "A": "1",
    "A": "2",
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:`+port+`"
  }
}
`)
}

// telemetrySettingsRefusal is a settings file that stops cld before docker, changing nothing:
// what it holds, and what cld says, PATH for its path.
type telemetrySettingsRefusal struct{ content, message string }

// telemetrySettingsRefusals are the files that are no JSON object, or whose env is none.
var telemetrySettingsRefusals = []telemetrySettingsRefusal{
	{"{\n  \"env\": {\n    \"A\": \"1\",\n  }\n}\n", "PATH is not valid JSON: line 4: " +
		"invalid character '}' looking for beginning of object key string"},
	{"{\"env\": {}", "PATH is not valid JSON: line 1: unexpected end of JSON input"},
	{"", "PATH is not valid JSON: it is empty"},
	{"{} {}", "PATH is not valid JSON: more follows the object"},
	{"[]", "PATH holds no JSON object"},
	{"{\"env\": [\"A=1\"]}", "env in PATH is not a JSON object"},
	{"{\"env\": null}", "env in PATH is not a JSON object"},
}

// run runs a setup over the settings of test.
func (test telemetrySettingsRefusal) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := writeSettings(t, s, test.content)
	result := s.RunCld(nil, "setup", "telemetry", "--local", localURL)
	checkTelemetryRefused(t, result, "cld: "+strings.ReplaceAll(test.message, "PATH", path)+"\n")
	checkNoDockerCall(t, s)
	checkSettings(t, path, test.content)
}
