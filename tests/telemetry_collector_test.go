package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// collectorExtra is the --collector-config file of TestSetupTelemetryCollectorConfig.
const collectorExtra = "exporters:\n  otlp_grpc/remote:\n    headers:\n" +
	"      authorization: Bearer secret\n    compression: gzip\n"

// --collector-config FILE reaches `validate` and the container as a second config in
// CLD_TELEMETRY_EXTRA, which replaces one in cld's own environment (decision 18.4). Without the
// option cld passes none on. A file that cld cannot read, or that no variable can hold, stops cld
// before docker.
func TestSetupTelemetryCollectorConfig(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, given := range []bool{true, false} {
		t.Run("given "+strconv.FormatBool(given), collectorConfigGiven(given))
	}
	// Linux takes a variable CLD_TELEMETRY_EXTRA=CONTENTS of up to 32 pages, its NUL included.
	most := 32*os.Getpagesize() - 1 - len("CLD_TELEMETRY_EXTRA=")
	t.Run("the longest", collectorConfigLongest(most))
	for _, test := range []struct{ name, content, message string }{
		{"too long", strings.Repeat("#", most+1), fmt.Sprintf("cld: the collector config "+
			"extra.yaml is too large for an environment variable: %d bytes, at most %d\n",
			most+1, most)},
		// A NUL would end the variable.
		{"a NUL byte", "a: 1\n\x00b: 2\n",
			"cld: the collector config extra.yaml is not text: it holds a NUL byte\n"},
	} {
		t.Run(test.name, collectorConfigRefused(test.content, test.message))
	}
	t.Run("missing", collectorConfigMissing)
}

// collectorConfigGiven runs a setup with --collector-config if given, CLD_TELEMETRY_EXTRA and
// CLD_TELEMETRY_CONFIG set in cld's environment.
func collectorConfigGiven(given bool) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		s.WriteFile(filepath.Join(s.Work, "extra.yaml"), collectorExtra)
		port := freePort(t)
		args := []string{"setup", "telemetry", "--remote", remoteURL, "--port", strconv.Itoa(port)}
		if given {
			args = append(args, "--collector-config", "extra.yaml")
		}
		inherited := map[string]string{
			"CLD_TELEMETRY_EXTRA": "inherited", "CLD_TELEMETRY_CONFIG": "inherited",
		}
		checkTelemetryDone(t, s.RunCld(inherited, args...))
		calls := s.DockerCalls()
		checkCalls(t, calls, setupCalls(port, given))
		if len(calls) != 5 {
			t.FailNow()
		}
		extra := ""
		if given {
			extra = collectorExtra
		}
		checkConfigs(t, calls, collectorConfig(port, "", remoteExporter), extra)
	}
}

// collectorConfigLongest runs a setup with a file of most bytes, the longest that a variable
// holds, having checked that the kernel refuses one byte more.
func collectorConfigLongest(most int) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		variable := "CLD_TELEMETRY_EXTRA=" + strings.Repeat("#", most+1)
		cmd := exec.Command("/bin/sh", "-c", "exit 0")
		cmd.Env = []string{variable}
		if err := cmd.Run(); !errors.Is(err, syscall.E2BIG) {
			t.Fatalf("a program run with a variable of %d bytes: %v, want %v",
				len(variable), err, syscall.E2BIG)
		}
		s := sandbox.New(t)
		extra := "#" + strings.Repeat(" ", most-2) + "\n"
		s.WriteFile(filepath.Join(s.Work, "extra.yaml"), extra)
		result := s.RunCld(nil,
			"setup", "telemetry", "--remote", remoteURL, "--collector-config", "extra.yaml")
		checkTelemetryDone(t, result)
		calls := s.DockerCalls()
		if len(calls) != 5 || calls[3].Env["CLD_TELEMETRY_EXTRA"] != extra {
			t.Errorf("%d docker calls, want 5, run -d given the file", len(calls))
		}
	}
}

// collectorConfigRefused runs a setup with a file of content that cld refuses with message.
func collectorConfigRefused(content, message string) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		s.WriteFile(filepath.Join(s.Work, "extra.yaml"), content)
		result := s.RunCld(nil,
			"setup", "telemetry", "--remote", remoteURL, "--collector-config", "extra.yaml")
		checkTelemetryRefused(t, result, message)
		checkNoDockerCall(t, s)
	}
}

// collectorConfigMissing runs a setup with a file that does not exist.
func collectorConfigMissing(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCld(nil,
		"setup", "telemetry", "--remote", remoteURL, "--collector-config", "missing.yaml")
	checkTelemetryRefused(t, result,
		"cld: cannot read the collector config missing.yaml: no such file or directory\n")
	checkNoDockerCall(t, s)
	checkNoClaudeDir(t, s)
}
