package telemetry

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

// extraConfig reads the --collector-config file at path into the variable that takes it to the
// container, CLD_TELEMETRY_EXTRA=CONTENTS. It refuses a file no variable can hold, where docker
// would fail to run (decision 18.4). A NUL byte would end the variable, and Linux takes one of 32
// pages at most, MAX_ARG_STRLEN, the NUL that ends it included.
func extraConfig(path string) (string, error) {
	extra, err := os.ReadFile(path)
	if err != nil {
		return "", fail.Runtime(fmt.Sprintf("cannot read the collector config %s: %v",
			path, reason(err)))
	}
	if bytes.IndexByte(extra, 0) >= 0 {
		return "", fail.Runtime("the collector config " + path + " is not text: it holds a NUL byte")
	}
	variable := extraVariable + "=" + string(extra)
	if most := 32*os.Getpagesize() - 1 - len(extraVariable+"="); len(extra) > most {
		return "", fail.Runtime(fmt.Sprintf("the collector config %s is too large for an "+
			"environment variable: %d bytes, at most %d", path, len(extra), most))
	}
	return variable, nil
}

// collectorConfig is cld's collector config: an OTLP/gRPC receiver on 127.0.0.1:port and an
// exporter per endpoint, otlp_grpc/local and otlp_grpc/remote. The file --collector-config gives
// can refer to them by name. The local one gives up after 30 s, and the collector's own metrics
// are off (decisions 18 and 18.7).
func collectorConfig(port int, local, remote *Endpoint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: %s\n",
		quote(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))))
	b.WriteString("exporters:\n")
	var metrics []string
	for _, exporter := range []struct {
		name     string
		endpoint *Endpoint
		retry    string
	}{
		{"otlp_grpc/local", local, "    retry_on_failure:\n      max_elapsed_time: 30s\n"},
		{"otlp_grpc/remote", remote, ""},
	} {
		if exporter.endpoint == nil {
			continue
		}
		metrics = append(metrics, exporter.name)
		fmt.Fprintf(&b, "  %s:\n    endpoint: %s\n", exporter.name, quote(exporter.endpoint.Address))
		if exporter.endpoint.Plaintext {
			b.WriteString("    tls:\n      insecure: true\n")
		}
		b.WriteString(exporter.retry)
	}
	b.WriteString("service:\n  pipelines:\n")
	fmt.Fprintf(&b, "    metrics:\n      receivers: [otlp]\n      exporters: [%s]\n",
		strings.Join(metrics, ", "))
	if local != nil {
		for _, signal := range []string{"traces", "logs"} {
			fmt.Fprintf(&b, "    %s:\n      receivers: [otlp]\n      exporters: [otlp_grpc/local]\n",
				signal)
		}
	}
	b.WriteString("  telemetry:\n    metrics:\n      level: none\n")
	return b.String()
}

// quote is s as a YAML scalar in double quotes, which a JSON string is: an IPv6 address in
// brackets would otherwise be a list.
func quote(s string) string {
	return strconv.Quote(s)
}
