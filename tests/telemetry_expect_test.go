package tests

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What the telemetry tests expect of a setup: its docker calls, the collector's config, claude's
// settings and what cld prints.

const (
	collectorImage = "otel/opentelemetry-collector:0.161.0"
	// inspectFormat is what cld asks docker inspect of the collector's container.
	inspectFormat = `{{.State.Status}} {{.RestartCount}} {{index .Config.Labels "cld.port"}}`
	localURL      = "http://127.0.0.1:4319"
	remoteURL     = "https://otel.example.com:4317"
)

// The exporters of localURL as --local, and of remoteURL and its plaintext variant as --remote.
const (
	localExporter           = "    endpoint: \"127.0.0.1:4319\"\n" + insecureTLS + localRetry
	remoteExporter          = "    endpoint: \"otel.example.com:4317\"\n"
	plaintextRemoteExporter = remoteExporter + insecureTLS
	insecureTLS             = "    tls:\n      insecure: true\n"
	localRetry              = "    retry_on_failure:\n      max_elapsed_time: 30s\n"
)

// localExporterFor is the exporter of a --local URL whose collector endpoint is address.
func localExporterFor(address string) string {
	return "    endpoint: " + strconv.Quote(address) + "\n" + insecureTLS + localRetry
}

// collectorConfig is the collector config cld passes for a collector on port, with an exporter
// for each of local and remote given ("" for none). Each gives the URL, then the exporter's own
// lines.
func collectorConfig(port int, local, remote string) string {
	config := "receivers:\n  otlp:\n    protocols:\n      grpc:\n" +
		fmt.Sprintf("        endpoint: \"127.0.0.1:%d\"\n", port) + "exporters:\n"
	var metrics []string
	if local != "" {
		config += "  otlp_grpc/local:\n" + local
		metrics = append(metrics, "otlp_grpc/local")
	}
	if remote != "" {
		config += "  otlp_grpc/remote:\n" + remote
		metrics = append(metrics, "otlp_grpc/remote")
	}
	config += "service:\n  pipelines:\n    metrics:\n      receivers: [otlp]\n" +
		"      exporters: [" + strings.Join(metrics, ", ") + "]\n"
	if local != "" {
		config += "    traces:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n" +
			"    logs:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n"
	}
	return config + "  telemetry:\n    metrics:\n      level: none\n"
}

// setupCalls are the docker calls of a setup that goes well, for a collector on port, given the
// --collector-config file if extra. The last is the wait's inspect, which finds the new collector
// running, its port then taking connections.
func setupCalls(port int, extra bool) [][]string {
	passed := []string{"-e", "CLD_TELEMETRY_CONFIG"}
	configs := []string{"--config=env:CLD_TELEMETRY_CONFIG"}
	if extra {
		passed = append(passed, "-e", "CLD_TELEMETRY_EXTRA")
		configs = append(configs, "--config=env:CLD_TELEMETRY_EXTRA")
	}
	validate := slices.Concat([]string{"run", "--rm"}, passed,
		[]string{collectorImage, "validate"}, configs)
	run := slices.Concat([]string{"run", "-d", "--name", "cld-telemetry",
		"--restart", "unless-stopped", "--network", "host",
		"--label", "cld.port=" + strconv.Itoa(port)}, passed, []string{collectorImage}, configs)
	inspect := []string{"container", "inspect", "--format", inspectFormat, "cld-telemetry"}
	return [][]string{inspect, validate, {"rm", "-f", "cld-telemetry"}, run, inspect}
}

// settingsFor is the settings file cld writes where there was none, for a collector on port.
func settingsFor(port int, local bool) string {
	settings := "{\n  \"env\": {\n" +
		"    \"CLAUDE_CODE_ENABLE_TELEMETRY\": \"1\",\n    \"OTEL_METRICS_EXPORTER\": \"otlp\",\n" +
		"    \"OTEL_EXPORTER_OTLP_PROTOCOL\": \"grpc\",\n" +
		"    \"OTEL_EXPORTER_OTLP_ENDPOINT\": \"http://127.0.0.1:" + strconv.Itoa(port) + "\""
	if local {
		settings += ",\n    \"OTEL_TRACES_EXPORTER\": \"otlp\",\n" +
			"    \"OTEL_LOGS_EXPORTER\": \"otlp\",\n" +
			"    \"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA\": \"1\",\n    \"OTEL_LOG_TOOL_DETAILS\": \"1\""
	}
	return settings + "\n  }\n}\n"
}

// setupReport is what a setup prints for a collector on port, with --local if local. signals are
// the report's lines for the signals, and path the settings, which had none of its keys.
func setupReport(port int, path, signals string, local bool) string {
	changes := "  CLAUDE_CODE_ENABLE_TELEMETRY=1\n  OTEL_METRICS_EXPORTER=otlp\n" +
		"  OTEL_EXPORTER_OTLP_PROTOCOL=grpc\n" +
		"  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:" + strconv.Itoa(port) + "\n"
	if local {
		changes += "  OTEL_TRACES_EXPORTER=otlp\n  OTEL_LOGS_EXPORTER=otlp\n" +
			"  CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1\n  OTEL_LOG_TOOL_DETAILS=1\n"
	}
	return "The collector cld-telemetry listens on 127.0.0.1:" + strconv.Itoa(port) +
		" and sends\n" + signals + "Changed in " + path + ", under env:\n" + changes +
		"claude reads these settings as a session starts: sessions running now keep theirs.\n"
}

// startedPort is the port in the label of the collector that cld started, with the fourth of
// calls, run -d.
func startedPort(t *testing.T, calls []sandbox.DockerCall) int {
	t.Helper()
	if len(calls) < 4 || !slices.Contains(calls[3].Argv, "-d") {
		t.Fatalf("docker calls %v: no run -d fourth", calls)
	}
	label := calls[3].Argv[slices.Index(calls[3].Argv, "--label")+1]
	port, err := strconv.Atoi(strings.TrimPrefix(label, "cld.port="))
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("label %q", label)
	}
	return port
}

// checkCalls reports calls whose arguments are not want's, in order.
func checkCalls(t *testing.T, calls []sandbox.DockerCall, want [][]string) {
	t.Helper()
	var got [][]string
	for _, call := range calls {
		got = append(got, call.Argv)
	}
	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Errorf("docker calls\n%q\nwant\n%q", got, want)
	}
}

// checkConfigs reports a `validate` or `run -d` among a setup's calls that does not get config, or
// extra in CLD_TELEMETRY_EXTRA, which "" leaves unset.
func checkConfigs(t *testing.T, calls []sandbox.DockerCall, config, extra string) {
	t.Helper()
	for _, call := range []sandbox.DockerCall{calls[1], calls[3]} {
		if got := call.Env["CLD_TELEMETRY_CONFIG"]; got != config {
			t.Errorf("docker %s gets the config\n%s\nwant\n%s", call.Argv[1], got, config)
		}
		got, found := call.Env["CLD_TELEMETRY_EXTRA"]
		if extra != "" && got != extra {
			t.Errorf("docker %s gets CLD_TELEMETRY_EXTRA\n%s\nwant\n%s", call.Argv[1], got, extra)
		}
		if extra == "" && found {
			t.Errorf("docker %s gets CLD_TELEMETRY_EXTRA %q", call.Argv[1], got)
		}
	}
}

// checkTelemetryDone stops a test whose cld did not exit 0 with nothing on stderr.
func checkTelemetryDone(t *testing.T, result sandbox.Result) {
	t.Helper()
	if result.Code != 0 || result.Stderr != "" {
		t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
	}
}

// checkTelemetryRefused reports a cld that did not exit 1 with stderr and nothing on stdout.
func checkTelemetryRefused(t *testing.T, result sandbox.Result, stderr string) {
	t.Helper()
	if result.Code != 1 || result.Stderr != stderr || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, stderr)
	}
}

// checkTelemetryFailed is checkTelemetryRefused for a stderr of several lines.
func checkTelemetryFailed(t *testing.T, result sandbox.Result, stderr string) {
	t.Helper()
	if result.Code != 1 || result.Stderr != stderr || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr\n%s\nwant exit 1, stderr\n%s",
			result.Code, result.Stdout, result.Stderr, stderr)
	}
}

// checkNoDockerCall reports a call of the fake docker.
func checkNoDockerCall(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	if calls := s.DockerCalls(); len(calls) != 0 {
		t.Errorf("docker ran: %q", calls[0].Argv)
	}
}

// checkNoClaudeDir reports a ~/.claude in the sandbox's HOME.
func checkNoClaudeDir(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("~/.claude: %v, want none", err)
	}
}

// checkCollectorRunning reports a fake container other than a collector running on port.
func checkCollectorRunning(t *testing.T, s *sandbox.Sandbox, port int) {
	t.Helper()
	if container := s.DockerContainer(); container != "running "+strconv.Itoa(port) {
		t.Errorf("container %q", container)
	}
}
