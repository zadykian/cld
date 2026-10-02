package tests

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// dockerFailure is a docker call that fails, the fake's CLD_FAKE_DOCKER_FAIL. Its calls are how
// many of the calls of a setup that goes well cld makes, and stderr what cld says, {port} for the
// running collector's port. Its container is the container afterwards (see
// sandbox.DockerContainer).
type dockerFailure struct {
	fail      string
	calls     int
	stderr    string
	container string
}

// validateFailed is what the fake docker says when `validate` fails.
const validateFailed = "fake docker: run --rm -e CLD_TELEMETRY_CONFIG " + collectorImage +
	" validate --config=env:CLD_TELEMETRY_CONFIG failed\n"

// dockerFailures are TestSetupTelemetryDockerFails' cases.
var dockerFailures = []dockerFailure{
	{"inspect", 1, "fake docker: container inspect --format " + inspectFormat +
		" cld-telemetry failed\n" +
		"cld: cannot inspect the collector cld-telemetry; nothing has changed\n", "untouched"},
	{"validate", 2, validateFailed +
		"cld: the collector refused its config (see above); nothing has changed\n", "untouched"},
	{"validate=125", 2, validateFailed + "cld: docker cannot run the collector to validate " +
		"its config (see above); nothing has changed\n", "untouched"},
	{"rm", 3, "fake docker: rm -f cld-telemetry failed\n" +
		"cld: cannot remove the collector cld-telemetry; the settings are unchanged\n", "untouched"},
	{"-d", 4, "fake docker: run -d --name cld-telemetry --restart unless-stopped " +
		"--network host --label cld.port={port} -e CLD_TELEMETRY_CONFIG " + collectorImage +
		" --config=env:CLD_TELEMETRY_CONFIG failed\n" +
		"cld: cannot start the collector; the collector that ran before is gone, " +
		"and the settings are unchanged\n", ""},
}

// A step that fails leaves what it has not reached (decision 18.5): the collector that runs
// keeps running, and the settings stay as they were. docker's message comes first, then what
// cld has left.
func TestSetupTelemetryDockerFails(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range dockerFailures {
		t.Run(test.fail, test.run)
	}
}

// run runs a setup whose docker call test.fail fails, the collector running before.
func (test dockerFailure) run(t *testing.T) {
	t.Parallel()
	const settings = "{\n  \"env\": {\n" +
		"    \"OTEL_EXPORTER_OTLP_ENDPOINT\": \"http://127.0.0.1:1234\"\n  }\n}\n"
	s := sandbox.New(t)
	path := writeSettings(t, s, settings)
	port := busyPort(t) // The running collector's.
	running := "running " + strconv.Itoa(port)
	result := s.RunCld(
		map[string]string{"CLD_FAKE_DOCKER_FAIL": test.fail, "CLD_FAKE_DOCKER_CONTAINER": running},
		"setup", "telemetry", "--local", localURL)
	checkTelemetryFailed(t, result, strings.ReplaceAll(test.stderr, "{port}", strconv.Itoa(port)))
	checkCalls(t, s.DockerCalls(), setupCalls(port, false)[:test.calls])
	want := strings.ReplaceAll(test.container, "{port}", strconv.Itoa(port))
	if got := s.DockerContainer(); got != want {
		t.Errorf("container %q, want %q", got, want)
	}
	checkSettings(t, path, settings)
}

// readyLog is the line the collector 0.161.0 logs once ready, less the fields after it.
const readyLog = "2026-09-25T14:52:07.911Z\tinfo\tservice@v0.161.0/service.go:256\t" +
	"Everything is ready. Begin running and processing data.\n"

// A collector that does not get ready leaves the settings alone, and cld shows the last 20 lines
// of its log (decision 18.5). It waits 10 s for one that runs but takes no connections. It gives
// up at once on one that stopped or that Docker restarted, even where the port of the collector
// before takes connections (docs/design/findings/environment.md, Telemetry).
func TestSetupTelemetryCollectorNotReady(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range collectorNotReadyCases() {
		t.Run(test.name, test.run)
	}
}

// collectorNotReady is a collector that does not get ready. The test gives the fake's container
// before, {port} for a port it holds, the new one's state, its log and the call that fails. cld
// shows shown, says message, waits the whole 10 s if waits and runs docker update if update.
type collectorNotReady struct {
	name                          string
	container, started, log, fail string
	shown, message                string
	waits, update                 bool
}

// collectorNotReadyCases are TestSetupTelemetryCollectorNotReady's cases.
func collectorNotReadyCases() []collectorNotReady {
	var log strings.Builder
	for line := 1; line <= 24; line++ {
		fmt.Fprintf(&log, "log line %d\n", line)
	}
	log.WriteString(readyLog)
	full := log.String()
	const (
		above   = "(the end of its log is above; docker logs cld-telemetry shows it all)"
		stopped = "cld: the collector stopped before it was ready " + above +
			", and Docker no longer restarts it; "
		// {chosen} is the port of the collector cld starts.
		noConnections = "cld: the collector takes no connections on 127.0.0.1:{chosen} after 10 s "
		unchanged     = "the settings are unchanged\n"
		gone          = "the collector that ran before is gone, and " + unchanged
		failed        = "Error: failed to start\n"
		inUse         = "Error: cannot start pipelines: listen tcp 127.0.0.1:{port}: " +
			"bind: address already in use\n"
	)
	return []collectorNotReady{
		{"not ready", "exited 1234", "running", full, "", full[strings.Index(full, "log line 6\n"):],
			noConnections + above + "; " + gone, true, false},
		{"not ready without a word", "", "running", "", "", "",
			noConnections + "(its log is empty); " + unchanged, true, false},
		{"stopped", "", "exited", failed, "", failed, stopped + unchanged, false, true},
		{"stopped without a word", "", "exited", "", "", "",
			"cld: the collector stopped before it was ready (its log is empty), " +
				"and Docker no longer restarts it; " + unchanged, false, true},
		{"restarted", "running {port}", "running 1", inUse, "", inUse, stopped + gone, false, true},
		{"Docker keeps restarting it", "", "restarting 1", failed, "update",
			failed + "fake docker: update --restart no cld-telemetry failed\n",
			"cld: the collector stopped before it was ready " + above +
				", and cld cannot stop Docker restarting it: " +
				"docker rm -f cld-telemetry removes it; " + unchanged, false, true},
		{"its log unreadable", "", "exited", "", "logs", "fake docker: logs cld-telemetry failed\n",
			"cld: cannot read the log of the collector; " + unchanged, false, false},
	}
}

// run runs a setup whose new collector does not get ready.
func (test collectorNotReady) run(t *testing.T) {
	t.Parallel()
	const settings = "{\"env\": {}}"
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
	checkTelemetryFailed(t, result, strings.ReplaceAll(
		strings.ReplaceAll(test.shown+test.message, "{port}", port),
		"{chosen}", strconv.Itoa(chosen)))
	if waited := elapsed >= 10*time.Second; waited != test.waits {
		t.Errorf("cld took %v", elapsed)
	}
	// The setup's calls up to run -d, then inspect until cld gives up - at once for a collector
	// that stopped - then logs.
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
}
