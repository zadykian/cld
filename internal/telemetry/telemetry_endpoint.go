package telemetry

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Endpoint is where the collector sends signals: an OTLP/gRPC receiver.
type Endpoint struct {
	// URL is the endpoint as given, http://HOST:PORT or https://HOST:PORT.
	URL string
	// Address is HOST:PORT, the collector's endpoint for it.
	Address string
	// Plaintext is set for http://: gRPC without TLS.
	Plaintext bool
	// port is PORT; local is whether HOST leads to 127.0.0.1, where the collector listens (see
	// Collector).
	port  int
	local bool
}

// Collector is whether e reaches the collector's own receiver on port: the collector would send
// what it receives to itself without end (decision 18.2). HOST leads to 127.0.0.1 as 127.0.0.1,
// 0.0.0.0, ::, localhost or a name under it (see docs/design/findings/environment.md).
func (e Endpoint) Collector(port int) bool {
	return e.local && e.port == port
}

// hostLabel matches a label of a host name: letters, digits, "-" and "_", starting and ending
// with a letter or digit. A name is labels separated by dots, with a dot at the end at most.
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`)

// validHost is whether host is a host name whose last label is not all digits. No top-level
// domain is. Such a name would be an IPv4 address written another way, such as 127.1, which the
// collector would look up as a name.
func validHost(host string) bool {
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	for _, label := range labels {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

// ParseEndpoint reads a URL given with --local or --remote as OTEL_EXPORTER_OTLP_ENDPOINT takes
// it: http://HOST:PORT (plaintext gRPC) or https://HOST:PORT (TLS), with no path, user, query or
// fragment. HOST is a name, an IPv4 address or an IPv6 one in brackets; PORT is read as ParsePort
// reads it.
func ParseEndpoint(raw string) (Endpoint, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Scheme+"://"+u.Host != raw {
		return Endpoint{}, false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil && (strings.HasPrefix(u.Host, "[") || !validHost(host)) {
		return Endpoint{}, false
	}
	port, ok := ParsePort(u.Port())
	if !ok {
		return Endpoint{}, false
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	local := name == "localhost" || strings.HasSuffix(name, ".localhost")
	if ip != nil {
		local = ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.IsUnspecified()
	}
	return Endpoint{
		URL:       raw,
		Address:   net.JoinHostPort(host, strconv.Itoa(port)),
		Plaintext: u.Scheme == "http",
		port:      port,
		local:     local,
	}, true
}

// ParsePort reads a TCP port, from 1 to 65535, in decimal digits without a leading zero: cld would
// otherwise repeat a URL's 04317 where the collector has 4317.
func ParsePort(raw string) (int, bool) {
	if raw == "" || raw[0] == '0' || strings.Trim(raw, "0123456789") != "" {
		return 0, false
	}
	port, err := strconv.Atoi(raw)
	return port, err == nil && port >= 1 && port <= 65535
}
