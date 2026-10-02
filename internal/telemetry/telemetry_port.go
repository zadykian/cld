package telemetry

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/zadykian/cld/internal/fail"
)

// choosePort chooses the collector's port, on 127.0.0.1, in decision 18.2's order. The order is
// --port, the running collector's, the stopped one's, or one the kernel picks. It holds a port it
// checks with a listener, which release closes just before the collector starts, so that nothing
// takes the port meanwhile. The running collector holds its own until docker rm -f.
func choosePort(o Options, running state) (int, net.Listener, error) {
	// A paused collector's receiver keeps the port, to which claude sessions send.
	ours := (running.status == "running" || running.status == "paused") && running.port != 0
	if o.Port != 0 {
		if ours && running.port == o.Port {
			return o.Port, nil, nil
		}
		held, err := holdPort(o.Port)
		if err != nil {
			return 0, nil, err
		}
		return o.Port, held, nil
	}
	if ours {
		if option := o.Collector(running.port); option != "" {
			return 0, nil, fail.Runtime(fmt.Sprintf("%s is where the collector listens "+
				"(%s has port %d): give the receiver's port, or another --port",
				option, container, running.port))
		}
		return running.port, nil, nil
	}
	// After a reboot another program may hold a stopped collector's port, an ephemeral one.
	if running.port != 0 && o.Collector(running.port) == "" {
		if held, err := listen(running.port); err == nil {
			return running.port, held, nil
		}
	}
	return pickPort(o)
}

// holdPort holds the port given with --port.
func holdPort(port int) (net.Listener, error) {
	held, err := listen(port)
	if errors.Is(err, syscall.EADDRINUSE) {
		return nil, fail.Runtime(fmt.Sprintf(
			"port %d on 127.0.0.1 is in use: choose another --port, or leave it out", port))
	}
	if err != nil {
		return nil, fail.Runtime(fmt.Sprintf("cannot listen on 127.0.0.1:%d: %v", port, reason(err)))
	}
	return held, nil
}

// pickPort holds a port the kernel picks, passing over one at which an endpoint of o reaches the
// collector itself. The kernel knows nothing of a receiver that does not run now.
func pickPort(o Options) (int, net.Listener, error) {
	// A port passed over stays held until the kernel has picked another.
	var passed []net.Listener
	defer func() {
		for _, held := range passed {
			release(held)
		}
	}()
	for {
		held, err := listen(0)
		if err != nil {
			return 0, nil, fail.Runtime("cannot listen on 127.0.0.1: " + reason(err).Error())
		}
		port := held.Addr().(*net.TCPAddr).Port
		if o.Collector(port) == "" {
			return port, held, nil
		}
		passed = append(passed, held)
	}
}

// listen listens on 127.0.0.1:port, where the collector will: something listening on
// 0.0.0.0 or [::] holds the port too.
func listen(port int) (net.Listener, error) {
	return net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// release closes held, if cld holds a port.
func release(held net.Listener) {
	if held != nil {
		_ = held.Close() //nolint:errcheck // the port is free after a close, even a failed one
	}
}
