package tests

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The releases that install.sh and cld update download, from an HTTP server of the test's, and
// the commands that build or run what they test.

// platforms are those the releases publish cld for, as they name the binaries: cld-PLATFORM.
var platforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}

// releases stands in for GitHub's releases: the files under dir, a release's at
// latest/download/NAME or download/vX.Y.Z/NAME, served at url, which records the paths asked for.
// latest, once set (see setLatest), redirects to the tag of the latest release, as GitHub's does.
type releases struct {
	dir   string
	url   string
	mu    sync.Mutex
	paths []string
	// latest is the version of the latest release, "" for none.
	latest string
	// held is a path whose download stops after its first byte, until the client goes; arrived
	// gets a value once it has stopped there.
	held    string
	arrived chan struct{}
}

// newReleases serves releases from the sandbox's root until the test ends.
func newReleases(t *testing.T, s *sandbox.Sandbox) *releases {
	t.Helper()
	r := &releases{dir: filepath.Join(s.Root, "releases"), arrived: make(chan struct{}, 1)}
	files := http.FileServer(http.Dir(r.dir))
	closing := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.paths = append(r.paths, req.URL.Path)
		latest, held := r.latest, r.held
		r.mu.Unlock()
		switch {
		case req.URL.Path == "/latest" && latest != "":
			http.Redirect(w, req, "/tag/v"+latest, http.StatusFound)
		case req.URL.Path == held:
			w.Header().Set("Content-Length", "1000000")
			_, _ = w.Write([]byte("#")) //nolint:errcheck // the test checks what the client got
			w.(http.Flusher).Flush()
			r.arrived <- struct{}{}
			select {
			case <-req.Context().Done():
			case <-closing:
			}
		default:
			files.ServeHTTP(w, req)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(closing) }) // before server.Close, which waits for the handlers
	r.url = server.URL
	return r
}

// setLatest makes version the latest release, which latest redirects to.
func (r *releases) setLatest(version string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latest = version
}

// hold stops the download of path after its first byte (see releases.held).
func (r *releases) hold(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held = path
}

// publish makes a release of version at path, latest/download or download/vX.Y.Z: a binary for
// each platform, and cld.sha256.
func (r *releases) publish(t *testing.T, path, version string) {
	t.Helper()
	for _, platform := range platforms {
		r.write(t, path, "cld-"+platform, fakeBinary(version, platform))
	}
	r.sum(t, path)
}

// write writes the file name of the release at path.
func (r *releases) write(t *testing.T, path, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(r.dir, path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, path, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sum writes the cld.sha256 of the release at path as make dist does, with sha256sum: a line for
// each of its binaries.
func (r *releases) sum(t *testing.T, path string) {
	t.Helper()
	binaries, err := filepath.Glob(filepath.Join(r.dir, path, "cld-*"))
	if err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	for _, binary := range binaries {
		data, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), filepath.Base(binary))
	}
	r.write(t, path, "cld.sha256", sums.String())
}

// asked are the paths the installer has asked the releases for, in order.
func (r *releases) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.paths)
}

// fakeBinary is what a release of version serves as cld for platform: a script that prints "cld
// VERSION", as cld --version does, and for cld completion SHELL a script that starts as cobra's
// does (see fakeScript), which cld update writes where setup completion wrote one.
func fakeBinary(version, platform string) string {
	return "#!/bin/sh\n# cld-" + platform + "\n" +
		"case \"$1 $2\" in\n" +
		"'completion bash') printf '# bash completion V2 for cld %s, " + version + "\\n' \"$*\" ;;\n" +
		"'completion zsh') printf '#compdef cld\\n# %s, " + version + "\\n' \"$*\" ;;\n" +
		"'completion fish') printf '# fish completion for cld %s, " + version + "\\n' \"$*\" ;;\n" +
		"*) echo 'cld " + version + "' ;;\n" +
		"esac\n"
}

// fakeScript is what the cld of fakeBinary for version prints when run with args, completion SHELL
// and its options.
func fakeScript(version string, args ...string) string { //nolint:unparam // as fakeBinary takes it
	line := strings.Join(args, " ") + ", " + version + "\n"
	switch args[1] {
	case "bash":
		return "# bash completion V2 for cld " + line
	case "zsh":
		return "#compdef cld\n# " + line
	}
	return "# fish completion for cld " + line
}

// checkCld reports a dir that does not hold cld, and cld alone, with content and the permissions
// mode.
func checkCld(t *testing.T, dir, content string, mode os.FileMode) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"cld"}) {
		t.Errorf("%s holds %q, want cld alone", dir, names)
	}
	info, err := os.Stat(filepath.Join(dir, "cld"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Errorf("cld has mode %v, want %v", info.Mode().Perm(), mode)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("cld holds %q, want %q", data, content)
	}
}

// checkAsked reports releases asked for other paths than want.
func checkAsked(t *testing.T, r *releases, want ...string) {
	t.Helper()
	if got := r.asked(); !slices.Equal(got, want) {
		t.Errorf("asked for %q, want %q", got, want)
	}
}

// runCommand runs cmd, which does not start with a terminal, for what it ends with.
func runCommand(t *testing.T, cmd *exec.Cmd) sandbox.Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("run %s: %v", cmd.Path, err)
	}
	return sandbox.Result{
		Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String(),
	}
}

// run runs name with args, writing its output to the tests' stderr, as TestMain builds cld.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}
