package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// receiverFile is the file in $CLD_PROBE_DIR that holds the port of a receiver the fake docker
// started, while it runs (see receiver).
const receiverFile = "docker.receiver"

// startReceiver starts the probe as the receiver of the collector on 127.0.0.1:port, one that
// takes connections if listen. It returns once the receiver has the port, or has found it taken.
func startReceiver(dir, port string, listen bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	started, done, err := os.Pipe()
	if err != nil {
		return err
	}
	mode := "hold"
	if listen {
		mode = "listen"
	}
	cmd := exec.Command(executable, port, filepath.Join(dir, receiverFile), mode)
	cmd.Args[0] = "receiver"
	cmd.ExtraFiles = []*os.File{done}
	// A session of its own, with none of the pipes cld reads docker's output from: cld, and the
	// test, go on without it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = errors.Join(cmd.Start(), done.Close())
	if err == nil {
		err = cmd.Process.Release()
	}
	if err == nil {
		_, err = io.ReadAll(started) // Up to the receiver's closing its end.
	}
	return errors.Join(err, started.Close())
}

// receiver stands in for the receiver of a collector the fake docker started, while FILE exists,
// which it writes with the port. "receiver PORT FILE listen" takes connections on 127.0.0.1:PORT,
// and "hold" binds the port without listening (docs/design/testing.md). It closes descriptor 3
// once it has the port, or once it has found the port taken and fails, as the collector would.
func receiver() error {
	port, file, mode := os.Args[1], os.Args[2], os.Args[3]
	var release func() error
	var err error
	if mode == "hold" {
		release, err = bind(port)
	} else {
		release, err = listen(port)
	}
	if err == nil {
		err = os.WriteFile(file, []byte(port), 0o644)
	}
	if err := errors.Join(err, os.NewFile(3, "started").Close()); err != nil {
		return err
	}
	// No longer than go test's own limit, should a test end without removing its sandbox.
	deadline := time.Now().Add(10 * time.Minute)
	for ; time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(file); err != nil {
			break
		}
	}
	return release()
}

// listen takes connections on 127.0.0.1:port, closing each, and returns what stops it.
func listen(port string) (func() error, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close() //nolint:errcheck // taking the connection is all its client needs
		}
	}()
	return listener.Close, nil
}

// bind binds a TCP socket to 127.0.0.1:port, without listening, and returns what closes it.
func bind(port string) (func() error, error) {
	number, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	address := &syscall.SockaddrInet4{Port: number, Addr: [4]byte{127, 0, 0, 1}}
	if err := syscall.Bind(fd, address); err != nil {
		return nil, errors.Join(err, syscall.Close(fd))
	}
	return func() error { return syscall.Close(fd) }, nil
}

// stopReceiver stops the receiver of the container, if one runs (see receiver), and returns once
// its port is free, as docker rm -f returns once the collector is gone.
func stopReceiver(dir string) error {
	file := filepath.Join(dir, receiverFile)
	port, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		err = os.Remove(file)
	}
	if err != nil {
		return err
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		listener, err := net.Listen("tcp4", "127.0.0.1:"+string(port))
		if err == nil {
			return listener.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the receiver on port %s did not stop: %w", port, err)
		}
	}
}
