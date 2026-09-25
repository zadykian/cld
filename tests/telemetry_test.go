package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld setup telemetry against the fake docker (see probe), which records each call, and a
// settings file in the sandbox's HOME. Only macOS runs the refusal: the rest is Linux only.

const (
	collectorImage = "otel/opentelemetry-collector:0.161.0"
	// inspectFormat is what cld asks docker inspect of the collector's container.
	inspectFormat = `{{.State.Status}} {{.RestartCount}} {{index .Config.Labels "cld.port"}}`
	localURL      = "http://127.0.0.1:4319"
	remoteURL     = "https://otel.example.com:4317"
)

// linuxOnly skips a test of what setup telemetry does on a system where it does not run.
func linuxOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("setup telemetry works on Linux only; TestSetupTelemetryLinuxOnly checks the refusal")
	}
}

// On any system but Linux setup telemetry refuses to run, whatever it is given: before any
// other check, reading nothing, running no docker. -h still shows its help.
func TestSetupTelemetryLinuxOnly(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("the refusal is for systems other than Linux")
	}
	for _, args := range [][]string{
		{"--local", localURL},
		{"--remote", remoteURL, "--port", "4317"},
		{},
		{"--local", "x"},
		{"--port", "0", "--remote", remoteURL},
		{"--bogus"},
		{"--local"},
		{"x"},
	} {
		args := append([]string{"setup", "telemetry"}, args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result, want := s.RunCld(nil, args...), "cld: setup telemetry works on Linux only\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
	s := sandbox.New(t)
	if result := s.RunCld(nil, "setup", "telemetry", "-h"); result.Code != 0 || !strings.HasPrefix(result.Stdout, "send claude's telemetry") {
		t.Errorf("-h: exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr)
	}
}

// Completion runs none of setup telemetry's checks, which are in its RunE, not in a hook that
// completion would run: whatever it completes after setup, on any system, it runs no docker -
// the fake one, first on the PATH here - and offers no file names (see TestCompleteCommands).
func TestCompletionRunsNoDocker(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"__complete", "setup", ""},
		{"__complete", "setup", "telemetry", ""},
		{"__complete", "setup", "telemetry", "--local", ""},
		{"__complete", "setup", "telemetry", "--remote", remoteURL, "--collector-config", ""},
		{"__completeNoDesc", "setup", "telemetry", "--local", localURL, "--port", ""},
		{"__complete", "help", "setup", ""},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(nil, args...)
			directive := result.Stdout == ":4\n" || strings.HasSuffix(result.Stdout, "\n:4\n")
			if stderr := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"; result.Code != 0 || !directive || result.Stderr != stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout ending in :4, stderr %q", result.Code, result.Stdout, result.Stderr, stderr)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
}

// collectorConfig is the collector config cld passes for a collector on port, with an exporter
// for each of local and remote given ("" for none): URL, then the exporter's own lines.
func collectorConfig(port int, local, remote string) string {
	config := fmt.Sprintf("receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: \"127.0.0.1:%d\"\nexporters:\n", port)
	var metrics []string
	if local != "" {
		config += "  otlp_grpc/local:\n" + local
		metrics = append(metrics, "otlp_grpc/local")
	}
	if remote != "" {
		config += "  otlp_grpc/remote:\n" + remote
		metrics = append(metrics, "otlp_grpc/remote")
	}
	config += "service:\n  pipelines:\n    metrics:\n      receivers: [otlp]\n      exporters: [" + strings.Join(metrics, ", ") + "]\n"
	if local != "" {
		config += "    traces:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n" +
			"    logs:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n"
	}
	return config + "  telemetry:\n    metrics:\n      level: none\n"
}

// The exporters of http://127.0.0.1:4319 as --local, and of remoteURL and its plaintext variant
// as --remote.
const (
	localExporter           = "    endpoint: \"127.0.0.1:4319\"\n    tls:\n      insecure: true\n    retry_on_failure:\n      max_elapsed_time: 30s\n"
	remoteExporter          = "    endpoint: \"otel.example.com:4317\"\n"
	plaintextRemoteExporter = "    endpoint: \"otel.example.com:4317\"\n    tls:\n      insecure: true\n"
)

// setupCalls are the docker calls of a setup that goes well, for a collector on port, given the
// --collector-config file if extra: the last is the wait's inspect, which finds the new collector
// running, its port then taking connections.
func setupCalls(port int, extra bool) [][]string {
	passed, configs := []string{"-e", "CLD_TELEMETRY_CONFIG"}, []string{"--config=env:CLD_TELEMETRY_CONFIG"}
	if extra {
		passed = append(passed, "-e", "CLD_TELEMETRY_EXTRA")
		configs = append(configs, "--config=env:CLD_TELEMETRY_EXTRA")
	}
	validate := append(append(append([]string{"run", "--rm"}, passed...), collectorImage, "validate"), configs...)
	run := append(append(append([]string{"run", "-d", "--name", "cld-telemetry", "--restart", "unless-stopped",
		"--network", "host", "--label", "cld.port=" + strconv.Itoa(port)}, passed...), collectorImage), configs...)
	inspect := []string{"container", "inspect", "--format", inspectFormat, "cld-telemetry"}
	return [][]string{inspect, validate, {"rm", "-f", "cld-telemetry"}, run, inspect}
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

// settingsFor is the settings file cld writes where there was none, for a collector on port.
func settingsFor(port int, local bool) string {
	settings := "{\n  \"env\": {\n    \"CLAUDE_CODE_ENABLE_TELEMETRY\": \"1\",\n    \"OTEL_METRICS_EXPORTER\": \"otlp\",\n" +
		"    \"OTEL_EXPORTER_OTLP_PROTOCOL\": \"grpc\",\n    \"OTEL_EXPORTER_OTLP_ENDPOINT\": \"http://127.0.0.1:" + strconv.Itoa(port) + "\""
	if local {
		settings += ",\n    \"OTEL_TRACES_EXPORTER\": \"otlp\",\n    \"OTEL_LOGS_EXPORTER\": \"otlp\",\n" +
			"    \"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA\": \"1\",\n    \"OTEL_LOG_TOOL_DETAILS\": \"1\""
	}
	return settings + "\n  }\n}\n"
}

// settingsPath is claude's settings file in the sandbox's HOME.
func settingsPath(s *sandbox.Sandbox) string {
	return filepath.Join(s.Home, ".claude", "settings.json")
}

// writeSettings writes claude's settings file in the sandbox's HOME and returns its path.
func writeSettings(t *testing.T, s *sandbox.Sandbox, content string) string {
	t.Helper()
	path := settingsPath(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(path, content)
	return path
}

// checkSettings reports a settings file at path that does not hold want.
func checkSettings(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s\n%s\nwant\n%s", path, got, want)
	}
}

// checkMode reports a file at path whose permissions are not want.
func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if info, err := os.Stat(path); err != nil {
		t.Error(err)
	} else if info.Mode().Perm() != want {
		t.Errorf("%s: mode %04o, want %04o", path, info.Mode().Perm(), want)
	}
}

// umask is the file mode creation mask of the tests, and of the cld they run, as Linux has it.
func umask(t *testing.T) os.FileMode {
	t.Helper()
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(status)) {
		if value, found := strings.CutPrefix(line, "Umask:"); found {
			mask, err := strconv.ParseUint(strings.TrimSpace(value), 8, 32)
			if err != nil {
				t.Fatalf("/proc/self/status: %q", line)
			}
			return os.FileMode(mask)
		}
	}
	t.Fatal("/proc/self/status gives no umask")
	return 0
}

// freePort is a port nothing listens on, on 127.0.0.1, as the kernel picks one.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// busyPort is a port that something listens on, on 127.0.0.1, until the test ends.
func busyPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

// The docker calls for --local, --remote and both, in order, with the collector config passed to
// validate and to the container alike - its pipelines too - and the settings and report that
// follow. cld holds a port it checks until just before it starts the collector, which then finds
// it free; the collector is ready once its port takes connections, and cld reads no log for that,
// which a --collector-config may quiet.
func TestSetupTelemetry(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range []struct {
		name          string
		args          []string
		local, remote string
		report        string
	}{
		{"local", []string{"--local", localURL}, localExporter, "",
			"  traces to   " + localURL + "\n  metrics to  " + localURL + "\n  logs to     " + localURL + "\n"},
		{"remote", []string{"--remote", "http://otel.example.com:4317"}, "", plaintextRemoteExporter,
			"  metrics to  http://otel.example.com:4317\n"},
		{"both", []string{"--remote", remoteURL, "--local", localURL}, localExporter, remoteExporter,
			"  traces to   " + localURL + "\n  metrics to  " + localURL + ", " + remoteURL + "\n  logs to     " + localURL + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			port := freePort(t)
			result := s.RunCld(nil, append([]string{"setup", "telemetry", "--port", strconv.Itoa(port)}, test.args...)...)
			path := settingsPath(s)
			changes := "  CLAUDE_CODE_ENABLE_TELEMETRY=1\n  OTEL_METRICS_EXPORTER=otlp\n  OTEL_EXPORTER_OTLP_PROTOCOL=grpc\n" +
				"  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:" + strconv.Itoa(port) + "\n"
			if test.local != "" {
				changes += "  OTEL_TRACES_EXPORTER=otlp\n  OTEL_LOGS_EXPORTER=otlp\n  CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1\n  OTEL_LOG_TOOL_DETAILS=1\n"
			}
			report := "The collector cld-telemetry listens on 127.0.0.1:" + strconv.Itoa(port) + " and sends\n" + test.report +
				"Changed in " + path + ", under env:\n" + changes +
				"claude reads these settings as a session starts: sessions running now keep theirs.\n"
			if result.Code != 0 || result.Stdout != report || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s", result.Code, result.Stderr, result.Stdout, report)
			}
			calls := s.DockerCalls()
			checkCalls(t, calls, setupCalls(port, false))
			if len(calls) != 5 {
				t.FailNow()
			}
			config := collectorConfig(port, test.local, test.remote)
			for _, call := range []sandbox.DockerCall{calls[1], calls[3]} {
				if got := call.Env["CLD_TELEMETRY_CONFIG"]; got != config {
					t.Errorf("docker %s gets the config\n%s\nwant\n%s", call.Argv[1], got, config)
				}
				if extra, found := call.Env["CLD_TELEMETRY_EXTRA"]; found {
					t.Errorf("docker %s gets CLD_TELEMETRY_EXTRA %q", call.Argv[1], extra)
				}
			}
			if free := calls[3].PortFree; free == nil || !*free {
				t.Error("the collector's port was not free as it started")
			}
			if container := s.DockerContainer(); container != "running "+strconv.Itoa(port) {
				t.Errorf("container %q", container)
			}
			checkSettings(t, path, settingsFor(port, test.local != ""))
		})
	}
}

// localExporterFor is the exporter of a --local URL whose collector endpoint is address.
func localExporterFor(address string) string {
	return "    endpoint: " + strconv.Quote(address) + "\n    tls:\n      insecure: true\n    retry_on_failure:\n      max_elapsed_time: 30s\n"
}

// The URLs cld takes, and the endpoint the collector gets for each: a name with "_" and "-"
// inside its labels, and a dot at the end; IPv6 addresses; and, on the collector's own port,
// hosts that do not lead to 127.0.0.1, where its receiver listens - ::1, 127.0.0.2, names that
// only look like localhost.
func TestSetupTelemetryEndpoints(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range []struct{ url, address string }{
		{"http://ide_1-a.example.:4319", "ide_1-a.example.:4319"},
		{"http://ide2:4319", "ide2:4319"},
		{"http://[fe80::1]:4319", "[fe80::1]:4319"},
		{"http://[::1]:{port}", "[::1]:{port}"},
		{"http://127.0.0.2:{port}", "127.0.0.2:{port}"},
		{"http://localhost.example:{port}", "localhost.example:{port}"},
		{"http://notlocalhost:{port}", "notlocalhost:{port}"},
	} {
		t.Run(test.url, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			port := strconv.Itoa(freePort(t))
			url := strings.ReplaceAll(test.url, "{port}", port)
			result := s.RunCld(nil, "setup", "telemetry", "--local", url, "--port", port)
			if result.Code != 0 || result.Stderr != "" || !strings.Contains(result.Stdout, "  traces to   "+url+"\n") {
				t.Fatalf("exit %d, stderr %q, stdout\n%s", result.Code, result.Stderr, result.Stdout)
			}
			calls := s.DockerCalls()
			if len(calls) != 5 {
				t.Fatalf("docker calls %v", calls)
			}
			n, _ := strconv.Atoi(port)
			if got, want := calls[3].Env["CLD_TELEMETRY_CONFIG"], collectorConfig(n, localExporterFor(strings.ReplaceAll(test.address, "{port}", port)), ""); got != want {
				t.Errorf("config\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// --collector-config FILE goes to validate and the container as a second config, a copy in
// CLD_TELEMETRY_EXTRA, which replaces one in cld's own environment; without the option cld passes
// none on. A file that cannot be read, or that no environment variable can hold, stops cld before
// docker.
func TestSetupTelemetryCollectorConfig(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	const extra = "exporters:\n  otlp_grpc/remote:\n    headers:\n      authorization: Bearer secret\n    compression: gzip\n"
	for _, given := range []bool{true, false} {
		t.Run("given "+strconv.FormatBool(given), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			s.WriteFile(filepath.Join(s.Work, "extra.yaml"), extra)
			port := freePort(t)
			args := []string{"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port)}
			if given {
				args = append(args, "--collector-config", "extra.yaml")
			}
			result := s.RunCld(map[string]string{"CLD_TELEMETRY_EXTRA": "inherited", "CLD_TELEMETRY_CONFIG": "inherited"}, args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			calls := s.DockerCalls()
			checkCalls(t, calls, setupCalls(port, given))
			if len(calls) != 5 {
				t.FailNow()
			}
			for _, call := range []sandbox.DockerCall{calls[1], calls[3]} {
				if got, want := call.Env["CLD_TELEMETRY_CONFIG"], collectorConfig(port, "", remoteExporter); got != want {
					t.Errorf("docker %s gets the config\n%s\nwant\n%s", call.Argv[1], got, want)
				}
				got, found := call.Env["CLD_TELEMETRY_EXTRA"]
				if given && got != extra {
					t.Errorf("docker %s gets CLD_TELEMETRY_EXTRA\n%s\nwant\n%s", call.Argv[1], got, extra)
				}
				if !given && found {
					t.Errorf("docker %s gets CLD_TELEMETRY_EXTRA %q", call.Argv[1], got)
				}
			}
		})
	}

	// The file goes into an environment variable, CLD_TELEMETRY_EXTRA=CONTENTS, which Linux takes
	// up to 32 pages long, the NUL at its end included: cld refuses a file that is longer, or that
	// holds a NUL, which would end the variable, before docker, which could not run, and says why.
	most := 32*os.Getpagesize() - 1 - len("CLD_TELEMETRY_EXTRA=")
	t.Run("the longest", func(t *testing.T) {
		t.Parallel()
		// The kernel refuses a variable one byte longer.
		variable := "CLD_TELEMETRY_EXTRA=" + strings.Repeat("#", most+1)
		cmd := exec.Command("/bin/sh", "-c", "exit 0")
		cmd.Env = []string{variable}
		if err := cmd.Run(); !errors.Is(err, syscall.E2BIG) {
			t.Fatalf("a program run with a variable of %d bytes: %v, want %v", len(variable), err, syscall.E2BIG)
		}
		s := sandbox.New(t)
		extra := "#" + strings.Repeat(" ", most-2) + "\n"
		s.WriteFile(filepath.Join(s.Work, "extra.yaml"), extra)
		result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL, "--collector-config", "extra.yaml")
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		if calls := s.DockerCalls(); len(calls) != 5 || calls[3].Env["CLD_TELEMETRY_EXTRA"] != extra {
			t.Errorf("%d docker calls, want 5, run -d given the file", len(calls))
		}
	})
	for _, test := range []struct{ name, content, message string }{
		{"too long", strings.Repeat("#", most+1),
			fmt.Sprintf("cld: the collector config extra.yaml is too large for an environment variable: %d bytes, at most %d\n", most+1, most)},
		{"a NUL byte", "a: 1\n\x00b: 2\n", "cld: the collector config extra.yaml is not text: it holds a NUL byte\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			s.WriteFile(filepath.Join(s.Work, "extra.yaml"), test.content)
			result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL, "--collector-config", "extra.yaml")
			if result.Code != 1 || result.Stderr != test.message || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, test.message)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL, "--collector-config", "missing.yaml")
		if want := "cld: cannot read the collector config missing.yaml: no such file or directory\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		if calls := s.DockerCalls(); len(calls) != 0 {
			t.Errorf("docker ran: %q", calls[0].Argv)
		}
		if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("~/.claude: %v, want none", err)
		}
	})
}

// A step that fails leaves what it has not reached: when validate refuses the config, or docker
// cannot run it, the collector that runs keeps running and the settings stay as they were. What
// docker said comes first, then what cld has left.
func TestSetupTelemetryDockerFails(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	const settings = "{\n  \"env\": {\n    \"OTEL_EXPORTER_OTLP_ENDPOINT\": \"http://127.0.0.1:1234\"\n  }\n}\n"
	for _, test := range []struct {
		fail string
		// calls is how many of the calls of a setup that goes well cld makes.
		calls  int
		stderr string
		// container is the fake's container afterwards (see sandbox.DockerContainer).
		container string
	}{
		{"inspect", 1, "fake docker: container inspect --format " + inspectFormat + " cld-telemetry failed\n" +
			"cld: cannot inspect the collector cld-telemetry; nothing has changed\n", "untouched"},
		{"validate", 2, "fake docker: run --rm -e CLD_TELEMETRY_CONFIG " + collectorImage + " validate --config=env:CLD_TELEMETRY_CONFIG failed\n" +
			"cld: the collector refused its config (see above); nothing has changed\n", "untouched"},
		{"validate=125", 2, "fake docker: run --rm -e CLD_TELEMETRY_CONFIG " + collectorImage + " validate --config=env:CLD_TELEMETRY_CONFIG failed\n" +
			"cld: docker cannot run the collector to validate its config (see above); nothing has changed\n", "untouched"},
		{"rm", 3, "fake docker: rm -f cld-telemetry failed\n" +
			"cld: cannot remove the collector cld-telemetry; the settings are unchanged\n", "untouched"},
		{"-d", 4, "fake docker: run -d --name cld-telemetry --restart unless-stopped --network host --label cld.port={port} -e CLD_TELEMETRY_CONFIG " +
			collectorImage + " --config=env:CLD_TELEMETRY_CONFIG failed\n" +
			"cld: cannot start the collector; the collector that ran before is gone, and the settings are unchanged\n", ""},
	} {
		t.Run(test.fail, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			path := writeSettings(t, s, settings)
			port := busyPort(t) // The running collector's.
			running := "running " + strconv.Itoa(port)
			result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_FAIL": test.fail, "CLD_FAKE_DOCKER_CONTAINER": running},
				"setup", "telemetry", "--local", localURL)
			if want := strings.ReplaceAll(test.stderr, "{port}", strconv.Itoa(port)); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr\n%s\nwant exit 1, stderr\n%s", result.Code, result.Stdout, result.Stderr, want)
			}
			checkCalls(t, s.DockerCalls(), setupCalls(port, false)[:test.calls])
			if got, want := s.DockerContainer(), strings.ReplaceAll(test.container, "{port}", strconv.Itoa(port)); got != want {
				t.Errorf("container %q, want %q", got, want)
			}
			checkSettings(t, path, settings)
		})
	}
}

// readyLog is the line the collector 0.161.0 logs once it is ready, less the fields after it.
const readyLog = "2026-09-25T14:52:07.911Z\tinfo\tservice@v0.161.0/service.go:256\tEverything is ready. Begin running and processing data.\n"

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

// A collector that does not get ready leaves the settings alone: cld shows the end of its log, the
// last 20 lines, or says that it is empty, and says that the collector that ran before, if any, is
// gone. It waits 10 s for one that keeps running but takes no connections on its port, whatever
// its log says - a --collector-config may have moved its receiver, out of claude's reach - and
// then names the port. It does not wait for one that stops, or that Docker has restarted: that
// its port takes connections, the port of the collector that ran before, does not make it ready.
// Docker is to stop restarting a collector that stopped, which cld says, and says how to remove it
// when Docker refuses. A log that cannot be read ends cld as well.
func TestSetupTelemetryCollectorNotReady(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	var log strings.Builder
	for line := 1; line <= 24; line++ {
		fmt.Fprintf(&log, "log line %d\n", line)
	}
	log.WriteString(readyLog)
	const (
		settings = "{\"env\": {}}"
		above    = "(the end of its log is above; docker logs cld-telemetry shows it all)"
		stopped  = "cld: the collector stopped before it was ready " + above + ", and Docker no longer restarts it; "
		// {chosen} is the port of the collector cld starts.
		noConnections = "cld: the collector takes no connections on 127.0.0.1:{chosen} after 10 s "
		unchanged     = "the settings are unchanged\n"
		gone          = "the collector that ran before is gone, and " + unchanged
	)
	for _, test := range []struct {
		name string
		// container is the collector before, {port} for a port the test holds.
		container string
		started   string
		log       string
		fail      string
		shown     string
		message   string
		// waits is whether cld waits the whole 10 s; update whether it runs docker update last.
		waits, update bool
	}{
		{"not ready", "exited 1234", "running", log.String(), "", log.String()[strings.Index(log.String(), "log line 6\n"):],
			noConnections + above + "; " + gone, true, false},
		{"not ready without a word", "", "running", "", "", "", noConnections + "(its log is empty); " + unchanged, true, false},
		{"stopped", "", "exited", "Error: failed to start\n", "", "Error: failed to start\n", stopped + unchanged, false, true},
		{"stopped without a word", "", "exited", "", "", "",
			"cld: the collector stopped before it was ready (its log is empty), and Docker no longer restarts it; " + unchanged, false, true},
		{"restarted", "running {port}", "running 1", "Error: cannot start pipelines: listen tcp 127.0.0.1:{port}: bind: address already in use\n", "",
			"Error: cannot start pipelines: listen tcp 127.0.0.1:{port}: bind: address already in use\n", stopped + gone, false, true},
		{"Docker keeps restarting it", "", "restarting 1", "Error: failed to start\n", "update",
			"Error: failed to start\nfake docker: update --restart no cld-telemetry failed\n",
			"cld: the collector stopped before it was ready " + above + ", and cld cannot stop Docker restarting it: " +
				"docker rm -f cld-telemetry removes it; " + unchanged, false, true},
		{"its log unreadable", "", "exited", "", "logs", "fake docker: logs cld-telemetry failed\n",
			"cld: cannot read the log of the collector; " + unchanged, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			path := writeSettings(t, s, settings)
			port := "none"
			if strings.Contains(test.container, "{port}") {
				port = strconv.Itoa(busyPort(t))
			}
			start := time.Now()
			result := s.RunCld(map[string]string{
				"CLD_FAKE_DOCKER_CONTAINER": strings.ReplaceAll(test.container, "{port}", port),
				"CLD_FAKE_DOCKER_STARTED":   test.started,
				"CLD_FAKE_DOCKER_LISTEN":    "no",
				"CLD_FAKE_DOCKER_LOGS":      strings.ReplaceAll(test.log, "{port}", port),
				"CLD_FAKE_DOCKER_FAIL":      test.fail,
			}, "setup", "telemetry", "--remote", remoteURL)
			elapsed := time.Since(start)
			calls := s.DockerCalls()
			chosen := startedPort(t, calls)
			want := strings.ReplaceAll(strings.ReplaceAll(test.shown+test.message, "{port}", port), "{chosen}", strconv.Itoa(chosen))
			if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr\n%s\nwant exit 1, stderr\n%s", result.Code, result.Stdout, result.Stderr, want)
			}
			if waited := elapsed >= 10*time.Second; waited != test.waits {
				t.Errorf("cld took %v", elapsed)
			}
			// The setup's calls up to run -d, then inspect until cld gives up - at once for a
			// collector that stopped - then logs.
			inspects := 0
			for _, call := range calls[4:] {
				if call.Argv[0] == "container" {
					inspects++
				}
			}
			if inspects == 0 || (inspects > 1) != test.waits {
				t.Errorf("cld inspected the collector it started %d times", inspects)
			}
			wantCalls := setupCalls(chosen, false)[:4]
			for range inspects {
				wantCalls = append(wantCalls, setupCalls(chosen, false)[0])
			}
			wantCalls = append(wantCalls, []string{"logs", "cld-telemetry"})
			if test.update {
				wantCalls = append(wantCalls, []string{"update", "--restart", "no", "cld-telemetry"})
			}
			checkCalls(t, calls, wantCalls)
			checkSettings(t, path, settings)
		})
	}
}

// The port the collector listens on: --port, unless something else listens there - the running
// collector does not count; else the port in the label of the running collector, so that
// claude sessions keep sending there; else the label's port of a collector that stopped if
// nothing else holds it; else a port the kernel picks. A paused collector counts as running: it
// keeps its port.
func TestSetupTelemetryPort(t *testing.T) {
	t.Parallel()
	linuxOnly(t)

	t.Run("--port in use", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		port := busyPort(t)
		result := s.RunCld(nil, "setup", "telemetry", "--local", localURL, "--port", strconv.Itoa(port))
		if want := fmt.Sprintf("cld: port %d on 127.0.0.1 is in use: choose another --port, or leave it out\n", port); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		checkCalls(t, s.DockerCalls(), setupCalls(port, false)[:1])
		if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("~/.claude: %v, want none", err)
		}
	})

	// An endpoint that leads to 127.0.0.1, where the collector listens, on the collector's port
	// would have it send to itself: cld refuses the port of the running collector, which claude
	// sessions send to, before it changes anything, and passes over a stopped collector's.
	t.Run("the running collector's, an endpoint's", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		port := strconv.Itoa(busyPort(t))
		url := "http://localhost:" + port
		result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": "running " + port}, "setup", "telemetry", "--local", url)
		want := "cld: --local " + url + " is where the collector listens (cld-telemetry has port " + port + "): give the receiver's port, or another --port\n"
		if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		checkCalls(t, s.DockerCalls(), setupCalls(0, false)[:1])
		if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("~/.claude: %v, want none", err)
		}
	})
	t.Run("a stopped collector's, an endpoint's", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		port := freePort(t)
		url := "http://127.0.0.1:" + strconv.Itoa(port)
		result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": "exited " + strconv.Itoa(port)}, "setup", "telemetry", "--local", url)
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		calls := s.DockerCalls()
		chosen := startedPort(t, calls)
		if chosen == port {
			t.Fatalf("the collector gets port %d, the endpoint's", port)
		}
		checkCalls(t, calls, setupCalls(chosen, false))
		if got, want := calls[3].Env["CLD_TELEMETRY_CONFIG"], collectorConfig(chosen, localExporterFor("127.0.0.1:"+strconv.Itoa(port)), ""); got != want {
			t.Errorf("config\n%s\nwant\n%s", got, want)
		}
		checkSettings(t, settingsPath(s), settingsFor(chosen, true))
	})

	for _, test := range []struct {
		name string
		// container is the collector's before, {port} for the port the test holds or leaves free.
		container string
		held      bool
		// given passes the port with --port.
		given bool
		// kept is whether the collector keeps that port.
		kept bool
	}{
		{"--port of the running collector", "running {port}", true, true, true},
		{"--port of a paused collector", "paused {port}", true, true, true},
		{"--port where a collector stopped", "exited {port}", false, true, true},
		{"the running collector's", "running {port}", true, false, true},
		{"a paused collector's", "paused {port}", true, false, true},
		{"a stopped collector's, free", "exited {port}", false, false, true},
		{"a stopped collector's, held by another program", "exited {port}", true, false, false},
		{"a restarting collector's, held by another program", "restarting {port}", true, false, false},
		{"another container's", "running", false, false, false},
		{"no collector", "", false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			port := freePort(t)
			if test.held {
				port = busyPort(t)
			}
			args := []string{"setup", "telemetry", "--local", localURL}
			if test.given {
				args = append(args, "--port", strconv.Itoa(port))
			}
			result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": strings.ReplaceAll(test.container, "{port}", strconv.Itoa(port))}, args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			calls := s.DockerCalls()
			chosen := startedPort(t, calls)
			// Without a port in the label or --port, the one the kernel picks may be any.
			if kept := chosen == port; kept != test.kept && (test.given || strings.Contains(test.container, "{port}")) {
				t.Errorf("the collector gets port %d, the test's being %d", chosen, port)
			}
			checkCalls(t, calls, setupCalls(chosen, false))
			if got, want := calls[3].Env["CLD_TELEMETRY_CONFIG"], collectorConfig(chosen, localExporter, ""); got != want {
				t.Errorf("config\n%s\nwant\n%s", got, want)
			}
			// The test's own listener holds a port that cld does not check: the running collector's.
			if free := calls[3].PortFree; !test.held && (free == nil || !*free) {
				t.Error("the collector's port was not free as it started")
			}
			checkSettings(t, settingsPath(s), settingsFor(chosen, true))
		})
	}

	// Run again, setup keeps the collector's port, and the settings, which it says; it does not
	// write them, so that a file laid out otherwise - by hand, on one line - stays as it is.
	t.Run("again", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		path := settingsPath(s)
		var compact bytes.Buffer
		for run := 1; run <= 2; run++ {
			result := s.RunCld(nil, "setup", "telemetry", "--local", localURL, "--remote", remoteURL)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("run %d: exit %d, stderr %q", run, result.Code, result.Stderr)
			}
			if run == 1 {
				written, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Compact(&compact, written); err != nil {
					t.Fatal(err)
				}
				s.WriteFile(path, compact.String())
			}
			if run == 2 && !strings.HasSuffix(result.Stdout, "\n"+path+" has these settings already.\n") {
				t.Errorf("run 2: stdout\n%s", result.Stdout)
			}
		}
		checkSettings(t, path, compact.String())
		calls := s.DockerCalls()
		if len(calls) != 10 {
			t.Fatalf("docker calls %v", calls)
		}
		first, second := calls[3].Argv, calls[8].Argv
		if !slices.Equal(first, second) || !slices.Contains(first, "--label") {
			t.Errorf("the collector ran with\n%q\nthen with\n%q", first, second)
		}
	})
}

// setup telemetry edits the env of claude's settings in place: the keys it manages take their
// values, where they are or at the end of env, or leave - the four that only --local sets, when it
// is not given, and the six per-signal ones always; every other key - in env and around it -
// stays where and as it was, the file's indentation too, and a key cld writes back keeps <, > and
// & as they are. A file that is no JSON object, or whose env is none, stops cld before docker,
// changing nothing.
func TestSetupTelemetrySettings(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	const before = `{
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
	const removed = `  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT removed
  OTEL_EXPORTER_OTLP_TRACES_PROTOCOL removed
  OTEL_EXPORTER_OTLP_METRICS_ENDPOINT removed
  OTEL_EXPORTER_OTLP_METRICS_PROTOCOL removed
  OTEL_EXPORTER_OTLP_LOGS_ENDPOINT removed
  OTEL_EXPORTER_OTLP_LOGS_PROTOCOL removed
`
	for _, test := range []struct {
		name, flag string
		env        string
		changes    string
	}{
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
` + removed},
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
` + removed},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			path := writeSettings(t, s, before)
			// Beyond what the umask lets a new file have.
			if err := os.Chmod(path, 0o666); err != nil {
				t.Fatal(err)
			}
			port := freePort(t)
			url := localURL
			if test.flag == "--remote" {
				url = remoteURL
			}
			result := s.RunCld(nil, "setup", "telemetry", test.flag, url, "--port", strconv.Itoa(port))
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			changes := strings.ReplaceAll(test.changes, "{port}", strconv.Itoa(port))
			if want := "Changed in " + path + ", under env:\n" + changes + "claude reads these settings as a session starts"; !strings.Contains(result.Stdout, want) {
				t.Errorf("stdout\n%s\nwant it to hold\n%s", result.Stdout, want)
			}
			start, end := strings.Index(before, "\"env\": {")+len("\"env\": {"), strings.Index(before, "    },\n    \"permissions\"")
			after := before[:start] + strings.ReplaceAll(test.env, "{port}", strconv.Itoa(port)) + before[end:]
			checkSettings(t, path, after)
			checkMode(t, path, 0o666)
			if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings.json*")); len(leftovers) != 0 {
				t.Errorf("left %q", leftovers)
			}
		})
	}

	// The settings are $CLAUDE_CONFIG_DIR/settings.json where that is set, as claude has them,
	// created with their directories if missing, as Claude Code creates them: 0644 and 0755, less
	// the umask.
	t.Run("CLAUDE_CONFIG_DIR", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		dir := filepath.Join(s.Root, "config", "claude")
		port := freePort(t)
		result := s.RunCld(map[string]string{"CLAUDE_CONFIG_DIR": dir}, "setup", "telemetry", "--local", localURL, "--port", strconv.Itoa(port))
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		checkSettings(t, filepath.Join(dir, "settings.json"), settingsFor(port, true))
		mask := umask(t)
		checkMode(t, filepath.Join(dir, "settings.json"), 0o644&^mask)
		checkMode(t, dir, 0o755&^mask)
		checkMode(t, filepath.Dir(dir), 0o755&^mask)
		if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("~/.claude: %v, want none", err)
		}
	})

	// cld reads the settings again once the collector is ready, and edits what it read then: what
	// another program wrote meanwhile - claude, say - stays.
	t.Run("changed meanwhile", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		path := writeSettings(t, s, `{"model": "opus"}`)
		port := freePort(t)
		result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_WRITE": path + "\n" + `{"model": "sonnet", "env": {"A": "1"}}`},
			"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		checkSettings(t, path, strings.Replace(settingsFor(port, false), "{\n  \"env\": {\n",
			"{\n  \"model\": \"sonnet\",\n  \"env\": {\n    \"A\": \"1\",\n", 1))
	})

	// Once the new collector runs, settings that cld cannot read again - claude, say, left them
	// invalid meanwhile - or cannot write end cld, saying that the collector runs: the settings
	// stay as they are. The write here fails for its temporary file, whose path is too long, where
	// the settings' own is not.
	t.Run("invalid meanwhile", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		path := writeSettings(t, s, `{"model": "opus"}`)
		port := freePort(t)
		result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_WRITE": path + "\n{"},
			"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
		want := "cld: " + path + " is not valid JSON: line 1: unexpected end of JSON input (the collector cld-telemetry runs, but the settings are unchanged)\n"
		if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		checkCalls(t, s.DockerCalls(), setupCalls(port, false))
		if container := s.DockerContainer(); container != "running "+strconv.Itoa(port) {
			t.Errorf("container %q", container)
		}
		checkSettings(t, path, "{")
	})
	t.Run("cannot write", func(t *testing.T) {
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
		result := s.RunCld(map[string]string{"CLAUDE_CONFIG_DIR": dir}, "setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port))
		want := "cld: cannot write " + path + ": file name too long (the collector cld-telemetry runs, but the settings are unchanged)\n"
		if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		checkCalls(t, s.DockerCalls(), setupCalls(port, false))
		if container := s.DockerContainer(); container != "running "+strconv.Itoa(port) {
			t.Errorf("container %q", container)
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("%s holds %v (%v), want nothing", dir, entries, err)
		}
	})

	// A key given twice: claude reads the file with JSON.parse, which keeps the last. So cld edits
	// the last env, leaving the others as they are, and a key it manages keeps its first place in
	// env, its value set, and loses the others, which would override it; it says what claude
	// read before. Other keys stay, however often given.
	t.Run("a key given twice", func(t *testing.T) {
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
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		changes := "Changed in " + path + ", under env:\n  CLAUDE_CODE_ENABLE_TELEMETRY=1\n  OTEL_METRICS_EXPORTER=otlp\n" +
			"  OTEL_EXPORTER_OTLP_PROTOCOL=grpc\n  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:" + port + "\n" +
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
	})

	// A settings file that is a symbolic link stays one: the file it leads to changes.
	t.Run("symbolic link", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		target := filepath.Join(s.Root, "dotfiles", "claude-settings.json")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		s.WriteFile(target, "{}")
		path := settingsPath(s)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		port := freePort(t)
		if result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port)); result.Code != 0 || result.Stderr != "" {
			t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
		}
		if link, err := os.Readlink(path); err != nil || link != target {
			t.Errorf("%s leads to %q (%v), want %s", path, link, err, target)
		}
		checkSettings(t, target, settingsFor(port, false))
	})

	// A symbolic link to a file that does not exist - dotfiles not checked out, say - stops cld
	// before docker: writing the settings would replace the link with a file.
	t.Run("symbolic link to no file", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		target := filepath.Join(s.Root, "dotfiles", "claude-settings.json")
		path := settingsPath(s)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		result := s.RunCld(nil, "setup", "telemetry", "--remote", remoteURL)
		if want := "cld: " + path + " is a symbolic link to a file that does not exist\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
		}
		if calls := s.DockerCalls(); len(calls) != 0 {
			t.Errorf("docker ran: %q", calls[0].Argv)
		}
		if link, err := os.Readlink(path); err != nil || link != target {
			t.Errorf("%s leads to %q (%v), want %s", path, link, err, target)
		}
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: %v, want none", target, err)
		}
	})

	for _, test := range []struct{ content, message string }{
		{"{\n  \"env\": {\n    \"A\": \"1\",\n  }\n}\n", "PATH is not valid JSON: line 4: invalid character '}' looking for beginning of object key string"},
		{"{\"env\": {}", "PATH is not valid JSON: line 1: unexpected end of JSON input"},
		{"", "PATH is not valid JSON: it is empty"},
		{"{} {}", "PATH is not valid JSON: more follows the object"},
		{"[]", "PATH holds no JSON object"},
		{"{\"env\": [\"A=1\"]}", "env in PATH is not a JSON object"},
		{"{\"env\": null}", "env in PATH is not a JSON object"},
	} {
		t.Run(strconv.Quote(test.content), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			path := writeSettings(t, s, test.content)
			result := s.RunCld(nil, "setup", "telemetry", "--local", localURL)
			if want := "cld: " + strings.ReplaceAll(test.message, "PATH", path) + "\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
			checkSettings(t, path, test.content)
		})
	}
}
