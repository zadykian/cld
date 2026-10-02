package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The collector's port, in decision 18.2's order: --port unless another program holds it, then
// the running or paused collector's. Then a stopped one's that nothing holds, else the kernel's
// pick.
func TestSetupTelemetryPort(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	t.Run("--port in use", telemetryPortInUse)
	t.Run("the running collector's, an endpoint's", telemetryPortRunningEndpoint)
	t.Run("a stopped collector's, an endpoint's", telemetryPortStoppedEndpoint)
	for _, test := range telemetryPortChoices {
		t.Run(test.name, test.run)
	}
	t.Run("again", telemetryPortAgain)
}

// telemetryPortInUse runs a setup given a --port that another program holds.
func telemetryPortInUse(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	port := busyPort(t)
	result := s.RunCld(nil, "setup", "telemetry", "--local", localURL, "--port", strconv.Itoa(port))
	checkTelemetryRefused(t, result, fmt.Sprintf("cld: port %d on 127.0.0.1 is in use: "+
		"choose another --port, or leave it out\n", port))
	checkCalls(t, s.DockerCalls(), setupCalls(port, false)[:1])
	checkNoClaudeDir(t, s)
}

// An endpoint that leads to 127.0.0.1 on the collector's port would have it send to itself
// (decision 18.2). So cld refuses the running collector's port before it changes anything.
func telemetryPortRunningEndpoint(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	port := strconv.Itoa(busyPort(t))
	url := "http://localhost:" + port
	result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": "running " + port},
		"setup", "telemetry", "--local", url)
	checkTelemetryRefused(t, result, "cld: --local "+url+" is where the collector listens "+
		"(cld-telemetry has port "+port+"): give the receiver's port, or another --port\n")
	checkCalls(t, s.DockerCalls(), setupCalls(0, false)[:1])
	checkNoClaudeDir(t, s)
}

// telemetryPortStoppedEndpoint runs a setup whose --local URL has a stopped collector's port,
// which cld passes over.
func telemetryPortStoppedEndpoint(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	port := freePort(t)
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	result := s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": "exited " + strconv.Itoa(port)},
		"setup", "telemetry", "--local", url)
	checkTelemetryDone(t, result)
	calls := s.DockerCalls()
	chosen := startedPort(t, calls)
	if chosen == port {
		t.Fatalf("the collector gets port %d, the endpoint's", port)
	}
	checkCalls(t, calls, setupCalls(chosen, false))
	want := collectorConfig(chosen, localExporterFor("127.0.0.1:"+strconv.Itoa(port)), "")
	if got := calls[3].Env["CLD_TELEMETRY_CONFIG"]; got != want {
		t.Errorf("config\n%s\nwant\n%s", got, want)
	}
	checkSettings(t, settingsPath(s), settingsFor(chosen, true))
}

// telemetryPortChoice is a collector before a setup, {port} in container for the port the test
// holds if held or leaves free, passed with --port if given, and whether the collector keeps
// that port.
type telemetryPortChoice struct {
	name              string
	container         string
	held, given, kept bool
}

// telemetryPortChoices are the cases of TestSetupTelemetryPort that pick a port.
var telemetryPortChoices = []telemetryPortChoice{
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
}

// run runs a setup with the collector of test before it.
func (test telemetryPortChoice) run(t *testing.T) {
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
	container := strings.ReplaceAll(test.container, "{port}", strconv.Itoa(port))
	checkTelemetryDone(t, s.RunCld(map[string]string{"CLD_FAKE_DOCKER_CONTAINER": container}, args...))
	calls := s.DockerCalls()
	chosen := startedPort(t, calls)
	// Without a port in the label or --port, the one the kernel picks may be any.
	pinned := test.given || strings.Contains(test.container, "{port}")
	if kept := chosen == port; kept != test.kept && pinned {
		t.Errorf("the collector gets port %d, the test's being %d", chosen, port)
	}
	checkCalls(t, calls, setupCalls(chosen, false))
	want := collectorConfig(chosen, localExporter, "")
	if got := calls[3].Env["CLD_TELEMETRY_CONFIG"]; got != want {
		t.Errorf("config\n%s\nwant\n%s", got, want)
	}
	// The test's own listener holds a port that cld does not check: the running collector's.
	if free := calls[3].PortFree; !test.held && (free == nil || !*free) {
		t.Error("the collector's port was not free as it started")
	}
	checkSettings(t, settingsPath(s), settingsFor(chosen, true))
}

// Run again, setup keeps the collector's port and the settings, and says so. It does not write
// them, so that a file laid out otherwise, by hand on one line, keeps its layout.
func telemetryPortAgain(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := settingsPath(s)
	var compact string
	for run := 1; run <= 2; run++ {
		result := s.RunCld(nil, "setup", "telemetry", "--local", localURL, "--remote", remoteURL)
		if result.Code != 0 || result.Stderr != "" {
			t.Fatalf("run %d: exit %d, stderr %q", run, result.Code, result.Stderr)
		}
		if run == 1 {
			compact = compactClaudeSettings(t, s, path)
		}
		already := "\n" + path + " has these settings already.\n"
		if run == 2 && !strings.HasSuffix(result.Stdout, already) {
			t.Errorf("run 2: stdout\n%s", result.Stdout)
		}
	}
	checkSettings(t, path, compact)
	calls := s.DockerCalls()
	if len(calls) != 10 {
		t.Fatalf("docker calls %v", calls)
	}
	first, second := calls[3].Argv, calls[8].Argv
	if !slices.Equal(first, second) || !slices.Contains(first, "--label") {
		t.Errorf("the collector ran with\n%q\nthen with\n%q", first, second)
	}
}

// compactClaudeSettings writes the settings at path again on one line, and returns them.
func compactClaudeSettings(t *testing.T, s *sandbox.Sandbox, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, written); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(path, compact.String())
	return compact.String()
}
