package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// DockerCall is a line of docker.jsonl: a call of the fake docker.
type DockerCall struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
	// PortFree is, for run -d, whether the port of its label was free.
	PortFree *bool `json:"portFree,omitempty"`
}

// readyLog is what the collector 0.161.0 logs once it runs, less the fields after it.
const readyLog = "2026-09-25T14:52:07.911Z\tinfo\tservice@v0.161.0/service.go:256\t" +
	"Everything is ready. Begin running and processing data.\n"

// containerID is what run -d prints.
const containerID = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"

// docker is a call of the fake docker, and the one container it keeps, cld-telemetry, as
// docker.container holds it: "STATUS PORT [RESTARTS]", empty for none. The container starts as
// $CLD_FAKE_DOCKER_CONTAINER, none unless a test sets it.
type docker struct {
	dir, statePath         string
	args                   []string
	call                   DockerCall
	status, port, restarts string
}

// fakeDocker fakes the docker calls of cld setup telemetry. It appends each, with its environment,
// to docker.jsonl, a line of JSON per call, then answers it.
func fakeDocker() error {
	d := &docker{dir: os.Getenv("CLD_PROBE_DIR"), args: os.Args[1:]}
	d.call = DockerCall{Argv: d.args, Env: environment()}
	d.statePath = filepath.Join(d.dir, "docker.container")
	if err := d.readState(); err != nil {
		return err
	}
	answer := d.answer() // First: run -d's records in the call whether its port is free.
	if err := appendJSON(filepath.Join(d.dir, "docker.jsonl"), d.call); err != nil {
		return err
	}
	return answer()
}

// readState reads the container from docker.container.
func (d *docker) readState() error {
	state, err := os.ReadFile(d.statePath)
	if os.IsNotExist(err) {
		state, err = []byte(os.Getenv("CLD_FAKE_DOCKER_CONTAINER")), nil
	}
	if err != nil {
		return err
	}
	d.restarts = "0"
	fields := strings.Fields(string(state))
	if len(fields) > 0 {
		d.status = fields[0]
	}
	if len(fields) > 1 {
		d.port = fields[1]
	}
	if len(fields) > 2 {
		d.restarts = fields[2]
	}
	return nil
}

// answer is what answers the call, once recorded. $CLD_FAKE_DOCKER_FAIL=WORD[=STATUS] makes a
// call with the argument WORD fail instead, with status STATUS (default 1).
func (d *docker) answer() func() error {
	args := d.args
	fail, failStatus, _ := strings.Cut(os.Getenv("CLD_FAKE_DOCKER_FAIL"), "=")
	switch {
	case len(args) == 0:
		return nothing
	case fail != "" && slices.Contains(args, fail):
		return func() error {
			failCall(args, failStatus)
			return nil
		}
	case len(args) > 1 && args[0] == "container" && args[1] == "inspect":
		return d.inspect
	case args[0] == "rm":
		return d.remove
	case args[0] == "run" && slices.Contains(args, "-d"):
		return d.start()
	case args[0] == "update":
		return d.update
	case args[0] == "logs":
		return logs
	}
	// run --rm, which checks the collector's config, succeeds, printing nothing.
	return nothing
}

// nothing answers a call that prints nothing and succeeds.
func nothing() error {
	return nil
}

// failCall fails the call args, with status, or 1 where that is no number.
func failCall(args []string, status string) {
	fmt.Fprintf(os.Stderr, "fake docker: %s failed\n", strings.Join(args, " "))
	code, err := strconv.Atoi(status)
	if err != nil {
		code = 1
	}
	os.Exit(code)
}

// name is the container the call names, its last argument.
func (d *docker) name() string {
	return d.args[len(d.args)-1]
}

// noSuchContainer says that there is no container, as Docker does.
func (d *docker) noSuchContainer() {
	fmt.Fprintf(os.Stderr, "Error response from daemon: No such container: %s\n", d.name())
}

// inspect is container inspect: it prints "STATUS RESTARTS PORT", or fails where there is no
// container.
func (d *docker) inspect() error {
	if d.status == "" {
		d.noSuchContainer()
		os.Exit(1)
	}
	fmt.Printf("%s %s %s\n", d.status, d.restarts, d.port)
	return nil
}

// remove is rm -f: it removes the container, once its receiver has stopped, or says there is
// none and succeeds, as Docker 29.6.0 does.
func (d *docker) remove() error {
	if d.status == "" {
		d.noSuchContainer()
		return nil
	}
	fmt.Println(d.name())
	if err := stopReceiver(d.dir); err != nil {
		return err
	}
	return os.WriteFile(d.statePath, nil, 0o644)
}

// update takes the container's restart policy: it prints the container's name, as Docker does,
// or fails where there is no container.
func (d *docker) update() error {
	if d.status == "" {
		d.noSuchContainer()
		os.Exit(1)
	}
	fmt.Println(d.name())
	return nil
}

// logs prints $CLD_FAKE_DOCKER_LOGS on stderr, by default readyLog.
func logs() error {
	text, set := os.LookupEnv("CLD_FAKE_DOCKER_LOGS")
	if !set {
		text = readyLog
	}
	fmt.Fprint(os.Stderr, text)
	return nil
}

// start is run -d. It records whether the port of its label cld.port is free on 127.0.0.1, which
// the collector needs, and returns what starts the container there (see run).
func (d *docker) start() func() error {
	port := d.port
	for i, arg := range d.args {
		if arg == "--label" && i+1 < len(d.args) {
			port, _ = strings.CutPrefix(d.args[i+1], "cld.port=")
		}
	}
	free := portFree(port)
	d.call.PortFree = &free
	return func() error { return d.run(port, free) }
}

// portFree reports whether a listener can have port on 127.0.0.1.
func portFree(port string) bool {
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

// run starts the container on port as $CLD_FAKE_DOCKER_STARTED says, "STATUS [RESTARTS]"
// (default running). One started running on a port that was free gets a receiver, which takes
// connections unless $CLD_FAKE_DOCKER_LISTEN is "no".
func (d *docker) run(port string, free bool) error {
	fmt.Println(containerID)
	if err := writeMeanwhile(); err != nil {
		return err
	}
	started := strings.Fields(os.Getenv("CLD_FAKE_DOCKER_STARTED"))
	if len(started) == 0 {
		started = []string{"running"}
	}
	state := strings.Join(slices.Insert(started, 1, port), " ")
	if err := os.WriteFile(d.statePath, []byte(state), 0o644); err != nil {
		return err
	}
	if started[0] != "running" || !free {
		return nil
	}
	return startReceiver(d.dir, port, os.Getenv("CLD_FAKE_DOCKER_LISTEN") != "no")
}

// writeMeanwhile writes a file as another program may while the collector starts: with
// $CLD_FAKE_DOCKER_WRITE, "FILE" and a newline then the rest, it writes the rest to FILE.
func writeMeanwhile() error {
	write, set := os.LookupEnv("CLD_FAKE_DOCKER_WRITE")
	if !set {
		return nil
	}
	file, content, _ := strings.Cut(write, "\n")
	return os.WriteFile(file, []byte(content), 0o644)
}
