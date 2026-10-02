package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup telemetry's command line: the URLs, the port and the arguments it refuses.

// telemetryNeeded is setup telemetry's message where neither --local nor --remote is given.
const telemetryNeeded = "cld: setup telemetry: --local URL, --remote URL or both are needed " +
	"(see cld help)\n"

// invalidURL is the message for url given to option, which takes the receiver's URL.
func invalidURL(url, option string) string {
	return "cld: invalid URL '" + url + "' for " + option +
		": http://HOST:PORT or https://HOST:PORT (see cld help)\n"
}

// invalidPort is the message for port given to --port.
func invalidPort(port string) string {
	return "cld: invalid port '" + port + "': a number from 1 to 65535 (see cld help)\n"
}

// listening is the message for url given to option, where the collector would listen with --port
// port, and so send to itself.
func listening(option, url, port string) string {
	return "cld: " + option + " " + url + " is where the collector would listen (--port " + port +
		"): give the receiver's port, or another --port (see cld help)\n"
}

// telemetryArgumentCases are setup telemetry's arguments that it refuses, and the message.
var telemetryArgumentCases = []struct {
	args []string
	want string
}{
	{nil, telemetryNeeded},
	{[]string{"--port", "4317"}, telemetryNeeded},
	{[]string{"--collector-config", "extra.yaml"}, telemetryNeeded},
	{[]string{"--local", "127.0.0.1:4319"}, invalidURL("127.0.0.1:4319", "--local")},
	{[]string{"--local", "http://127.0.0.1"}, invalidURL("http://127.0.0.1", "--local")},
	{[]string{"--local", "http://127.0.0.1:4319/"}, invalidURL("http://127.0.0.1:4319/", "--local")},
	{[]string{"--local", "http://127.0.0.1:4319/v1/traces"},
		invalidURL("http://127.0.0.1:4319/v1/traces", "--local")},
	{[]string{"--local", "grpc://127.0.0.1:4319"}, invalidURL("grpc://127.0.0.1:4319", "--local")},
	{[]string{"--local", "http://127.0.0.1:0"}, invalidURL("http://127.0.0.1:0", "--local")},
	{[]string{"--local", "http://127.0.0.1:65536"}, invalidURL("http://127.0.0.1:65536", "--local")},
	{[]string{"--local", "http://user@127.0.0.1:4319"},
		invalidURL("http://user@127.0.0.1:4319", "--local")},
	{[]string{"--local", "http://127.0.0.1:4319?x=1"},
		invalidURL("http://127.0.0.1:4319?x=1", "--local")},
	{[]string{"--local", "http://a b:4319"}, invalidURL("http://a b:4319", "--local")},
	{[]string{"--local", "http://a!b:4319"}, invalidURL("http://a!b:4319", "--local")},
	{[]string{"--local", "http://[fe80::1%25eth0]:4319"},
		invalidURL("http://[fe80::1%25eth0]:4319", "--local")},
	// A label of a host name starts and ends with a letter or digit, the last is not all digits,
	// and a port has no leading zero.
	{[]string{"--local", "http://-ide:4319"}, invalidURL("http://-ide:4319", "--local")},
	{[]string{"--local", "http://ide-.local:4319"}, invalidURL("http://ide-.local:4319", "--local")},
	{[]string{"--local", "http://ide._x:4319"}, invalidURL("http://ide._x:4319", "--local")},
	{[]string{"--local", "http://a..b:4319"}, invalidURL("http://a..b:4319", "--local")},
	{[]string{"--local", "http://127.1:4319"}, invalidURL("http://127.1:4319", "--local")},
	{[]string{"--local", "http://2130706433:4319"}, invalidURL("http://2130706433:4319", "--local")},
	{[]string{"--local", "http://127.0.0.1:04319"}, invalidURL("http://127.0.0.1:04319", "--local")},
	{[]string{"--local="}, invalidURL("", "--local")},
	{[]string{"--remote", "otel.example.com:4317"}, invalidURL("otel.example.com:4317", "--remote")},
	{[]string{"--local", "http://127.0.0.1:4319", "--remote", "https://otel.example.com"},
		invalidURL("https://otel.example.com", "--remote")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port", "0"}, invalidPort("0")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port", "65536"}, invalidPort("65536")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port", "x"}, invalidPort("x")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port", "-1"}, invalidPort("-1")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port="}, invalidPort("")},
	{[]string{"--remote", "https://otel.example.com:4317", "--port", "04317"}, invalidPort("04317")},
	// A URL that leads to 127.0.0.1, where the collector listens, on its --port.
	{[]string{"--local", "http://127.0.0.1:4319", "--port", "4319"},
		listening("--local", "http://127.0.0.1:4319", "4319")},
	{[]string{"--port", "4317", "--remote", "http://localhost:4317"},
		listening("--remote", "http://localhost:4317", "4317")},
	{[]string{"--local", "https://0.0.0.0:4319", "--port", "4319"},
		listening("--local", "https://0.0.0.0:4319", "4319")},
	{[]string{"--local", "http://[::]:4319", "--port", "4319"},
		listening("--local", "http://[::]:4319", "4319")},
	{[]string{"--local", "http://[::ffff:127.0.0.1]:4319", "--port", "4319"},
		listening("--local", "http://[::ffff:127.0.0.1]:4319", "4319")},
	{[]string{"--local", "http://IDE.LocalHost.:4319", "--port", "4319"},
		listening("--local", "http://IDE.LocalHost.:4319", "4319")},
	{[]string{"--local", "http://127.0.0.1:4318", "--remote", "http://localhost:4319",
		"--port", "4319"},
		listening("--remote", "http://localhost:4319", "4319")},
	{[]string{"--remote", "http://localhost:4319", "--local", "http://127.0.0.1:4319",
		"--port", "4319"},
		listening("--local", "http://127.0.0.1:4319", "4319")},
	// The URLs are checked before the port, and both before what is missing.
	{[]string{"--port", "x", "--local", "x"}, invalidURL("x", "--local")},
	{[]string{"--port", "x"}, invalidPort("x")},
	{[]string{"--remote", "https://otel.example.com:4317", "--collector-config="},
		"cld: option '--collector-config' needs a value (see cld help)\n"},
	{[]string{"--local"}, "cld: option '--local' needs a value (see cld help)\n"},
	{[]string{"--remote", "https://otel.example.com:4317", "x"},
		unexpected("setup telemetry", "x")},
	{[]string{"x", "--remote", "https://otel.example.com:4317"},
		unexpected("setup telemetry", "x")},
	{[]string{"--remote", "https://otel.example.com:4317", "--"},
		unexpected("setup telemetry", "--")},
	{[]string{"--bogus"}, unexpected("setup telemetry", "--bogus")},
	{[]string{"-n", "x"}, unexpected("setup telemetry", "-n")},
}

// setup telemetry's mistakes are usage errors, read left to right: no URL or port where one goes,
// neither --local nor --remote, an unknown option or an argument. So is a URL where the collector
// would listen with --port, as it would send to itself. None gets as far as docker.
func TestSetupTelemetryRejectsArguments(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, test := range telemetryArgumentCases {
		args := append([]string{"setup", "telemetry"}, test.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, args...), 2, test.want)
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
		})
	}
}
