package tests

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup telemetry against the fake docker (see probe), which records each call, and claude's
// settings in the sandbox's HOME. Off Linux only the refusal runs (decision 18.1).

// Off Linux, setup telemetry refuses whatever it gets, before any other check, reading nothing
// and running no docker. -h still shows its help.
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
			result := s.RunCld(nil, args...)
			checkTelemetryRefused(t, result, "cld: setup telemetry works on Linux only\n")
			checkNoDockerCall(t, s)
		})
	}
	s := sandbox.New(t)
	result := s.RunCld(nil, "setup", "telemetry", "-h")
	if result.Code != 0 || !strings.HasPrefix(result.Stdout, "send claude's telemetry") {
		t.Errorf("-h: exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr)
	}
}

// Completion runs none of setup telemetry's checks, so no docker, on any system (decision
// 18.9), and offers no file names (see TestCompleteCommands).
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
			stderr := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"
			if result.Code != 0 || !directive || result.Stderr != stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, "+
					"want exit 0, stdout ending in :4, stderr %q",
					result.Code, result.Stdout, result.Stderr, stderr)
			}
			checkNoDockerCall(t, s)
		})
	}
}

// telemetrySetups are TestSetupTelemetry's cases: the arguments, the exporters of local and
// remote, and the report's lines for the signals.
var telemetrySetups = []struct {
	name          string
	args          []string
	local, remote string
	report        string
}{
	{"local", []string{"--local", localURL}, localExporter, "",
		"  traces to   " + localURL + "\n  metrics to  " + localURL + "\n" +
			"  logs to     " + localURL + "\n"},
	{"remote", []string{"--remote", "http://otel.example.com:4317"}, "", plaintextRemoteExporter,
		"  metrics to  http://otel.example.com:4317\n"},
	{"both", []string{"--remote", remoteURL, "--local", localURL}, localExporter, remoteExporter,
		"  traces to   " + localURL + "\n  metrics to  " + localURL + ", " + remoteURL + "\n" +
			"  logs to     " + localURL + "\n"},
}

// The docker calls for --local, --remote and both, the config `validate` and the container get,
// and the settings and report that follow. The collector's port is free as it starts, and cld
// tells it ready by the port, not the log (decision 18.5).
func TestSetupTelemetry(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range telemetrySetups {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			port := freePort(t)
			args := []string{"setup", "telemetry", "--port", strconv.Itoa(port)}
			result := s.RunCld(nil, append(args, test.args...)...)
			path := settingsPath(s)
			report := setupReport(port, path, test.report, test.local != "")
			if result.Code != 0 || result.Stdout != report || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s",
					result.Code, result.Stderr, result.Stdout, report)
			}
			calls := s.DockerCalls()
			checkCalls(t, calls, setupCalls(port, false))
			if len(calls) != 5 {
				t.FailNow()
			}
			checkConfigs(t, calls, collectorConfig(port, test.local, test.remote), "")
			if free := calls[3].PortFree; free == nil || !*free {
				t.Error("the collector's port was not free as it started")
			}
			checkCollectorRunning(t, s, port)
			checkSettings(t, path, settingsFor(port, test.local != ""))
		})
	}
}

// The URLs cld takes, and the endpoint the collector gets for each. On the collector's own port
// these are hosts that do not lead to 127.0.0.1, where its receiver listens (decision 18.2).
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
			n := freePort(t)
			port := strconv.Itoa(n)
			url := strings.ReplaceAll(test.url, "{port}", port)
			result := s.RunCld(nil, "setup", "telemetry", "--local", url, "--port", port)
			traces := "  traces to   " + url + "\n"
			if result.Code != 0 || result.Stderr != "" || !strings.Contains(result.Stdout, traces) {
				t.Fatalf("exit %d, stderr %q, stdout\n%s", result.Code, result.Stderr, result.Stdout)
			}
			calls := s.DockerCalls()
			if len(calls) != 5 {
				t.Fatalf("docker calls %v", calls)
			}
			address := strings.ReplaceAll(test.address, "{port}", port)
			want := collectorConfig(n, localExporterFor(address), "")
			if got := calls[3].Env["CLD_TELEMETRY_CONFIG"]; got != want {
				t.Errorf("config\n%s\nwant\n%s", got, want)
			}
		})
	}
}
