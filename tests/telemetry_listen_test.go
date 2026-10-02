package tests

import (
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The ports the telemetry tests listen on, or leave for cld's collector.

// freePort is a port nothing listens on, on 127.0.0.1, and nothing is to take before cld
// listens there: one of testPort's, let go.
func freePort(t *testing.T) int {
	t.Helper()
	listener := testPort(t)
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// busyPort is a port that something listens on, on 127.0.0.1, until the test ends: one of
// testPort's, held.
func busyPort(t *testing.T) int {
	t.Helper()
	listener := testPort(t)
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	return listener.Addr().(*net.TCPAddr).Port
}

// testPorts are the ports testPort hands out, first to last, next the one it tries next.
var testPorts struct {
	sync.Mutex
	first, last, next int
}

// testPort listens on 127.0.0.1 on a port outside the kernel's ephemeral range. Neither a listener
// on port 0 nor a connection's local port takes such a port (docs/design/testing.md). Tests get
// the ports in turn from a random start, so that two go test processes seldom meet.
func testPort(t *testing.T) net.Listener {
	t.Helper()
	testPorts.Lock()
	defer testPorts.Unlock()
	if testPorts.next == 0 {
		low, high := ephemeralPorts(t)
		testPorts.first, testPorts.last = high+1, 65535
		if testPorts.first > testPorts.last {
			testPorts.first, testPorts.last = 1024, low-1
		}
		if testPorts.first > testPorts.last {
			t.Fatalf("no port outside the ephemeral range, %d to %d", low, high)
		}
		testPorts.next = testPorts.first + rand.IntN(testPorts.last-testPorts.first+1)
	}
	for range testPorts.last - testPorts.first + 1 {
		port := testPorts.next
		if testPorts.next++; testPorts.next > testPorts.last {
			testPorts.next = testPorts.first
		}
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		if listener, err := net.Listen("tcp4", address); err == nil {
			return listener
		}
	}
	t.Fatalf("no port free on 127.0.0.1 from %d to %d", testPorts.first, testPorts.last)
	return nil
}

// ephemeralPorts is the kernel's ephemeral range, its first and last port, as Linux has it in the
// tests' network namespace.
func ephemeralPorts(t *testing.T) (int, int) {
	t.Helper()
	const path = "/proc/sys/net/ipv4/ip_local_port_range"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fields := strings.Fields(string(raw)); len(fields) == 2 {
		low, errLow := strconv.Atoi(fields[0])
		high, errHigh := strconv.Atoi(fields[1])
		if errLow == nil && errHigh == nil {
			return low, high
		}
	}
	t.Fatalf("%s: %q", path, raw)
	return 0, 0
}
