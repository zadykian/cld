package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld update against releases the test serves (see releases), from a cld built as oldRelease
// (decision 21.6). The cld goes into a directory of the sandbox's, as update replaces the file it
// runs from. It runs with nothing on the PATH, since update runs neither tmux nor claude
// (decision 21.5). The helpers the update_*_test.go files share are here too.

// oldRelease is the version of the cld that update replaces.
const oldRelease = "0.4.0"

// hostBinary is the binary update downloads here: the one for the system the tests run on.
var hostBinary = "cld-" + runtime.GOOS + "-" + runtime.GOARCH

var releasedCld struct {
	once sync.Once
	data []byte
	err  error
}

// oldCld is cld built as release oldRelease, as make dist builds it: built once, for every test.
func oldCld(t *testing.T) []byte {
	t.Helper()
	releasedCld.once.Do(func() {
		path := filepath.Join(filepath.Dir(sandbox.Cld), "cld-"+oldRelease)
		releasedCld.err = run("go", "build", "-ldflags", "-X main.version="+oldRelease,
			"-o", path, "github.com/zadykian/cld/cmd/cld")
		if releasedCld.err == nil {
			releasedCld.data, releasedCld.err = os.ReadFile(path)
		}
	})
	if releasedCld.err != nil {
		t.Fatal(releasedCld.err)
	}
	return releasedCld.data
}

// placeCld writes content, a cld, to path with the permissions mode, making its directory.
func placeCld(t *testing.T, s *sandbox.Sandbox, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteProgram(path, string(content), mode)
	if err := os.Chmod(path, mode); err != nil { // whatever the umask
		t.Fatal(err)
	}
}

// updateCommand is cld update run from path, with the releases at url and nothing on the PATH.
func updateCommand(s *sandbox.Sandbox, path, url string) *exec.Cmd {
	cmd := exec.Command(path, "update")
	cmd.Env = s.Environ(map[string]string{"PATH": s.Tools(), "CLD_RELEASES_URL": url})
	cmd.Dir = s.Work
	return cmd
}

// latestRelease is releases whose latest is version, published.
func latestRelease(t *testing.T, s *sandbox.Sandbox, version string) *releases {
	t.Helper()
	r := newReleases(t, s)
	r.publish(t, "download/v"+version, version)
	r.setLatest(version)
	return r
}

// update replaces cld with the latest release's binary for the system, keeping its permissions,
// and says so. Versions compare by number, so 0.10.0 follows 0.4.0. Where cld is the latest
// release, or newer, update says so and downloads nothing (decision 21.1).
func TestUpdate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		latest string
		mode   os.FileMode
		// stdout is what update prints where it downloads nothing, "" for an update.
		stdout string
	}{
		{latest: "0.5.0", mode: 0o755},
		{latest: "0.5.0", mode: 0o700},
		{latest: "0.4.1", mode: 0o755},
		{latest: "0.10.0", mode: 0o755},
		{latest: "1.0.0", mode: 0o750},
		{latest: oldRelease, mode: 0o755, stdout: "cld 0.4.0 is the latest release\n"},
		{latest: "0.3.9", mode: 0o755, stdout: "cld 0.4.0 is newer than the latest release, 0.3.9\n"},
		{latest: "0.3.10", mode: 0o755, stdout: "cld 0.4.0 is newer than the latest release, 0.3.10\n"},
	} {
		t.Run(test.latest+"-"+strconv.FormatUint(uint64(test.mode), 8), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := latestRelease(t, s, test.latest)
			dir := filepath.Join(s.Root, "bin")
			path := filepath.Join(dir, "cld")
			placeCld(t, s, path, oldCld(t), test.mode)
			result := runCommand(t, updateCommand(s, path, r.url))
			if test.stdout != "" {
				if want := (sandbox.Result{Stdout: test.stdout}); result != want {
					t.Errorf("got %+v, want %+v", result, want)
				}
				checkCld(t, dir, string(oldCld(t)), test.mode)
				checkAsked(t, r, "/latest")
				return
			}
			want := sandbox.Result{
				Stdout: "Updated cld " + oldRelease + " to " + test.latest + ": " + path + "\n",
			}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkCld(t, dir, fakeBinary(test.latest, runtime.GOOS+"-"+runtime.GOARCH), test.mode)
			files := "/download/v" + test.latest + "/"
			checkAsked(t, r, "/latest", files+"cld.sha256", files+hostBinary)
		})
	}
}

// Run through a symbolic link, update replaces the file the link leads to, which cld runs from,
// and leaves the link (decision 21.3).
func TestUpdateThroughLink(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := latestRelease(t, s, "0.5.0")
	dir, link := filepath.Join(s.Root, "opt"), filepath.Join(s.Root, "bin", "cld")
	placeCld(t, s, filepath.Join(dir, "cld"), oldCld(t), 0o755)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../opt/cld", link); err != nil {
		t.Fatal(err)
	}
	result := runCommand(t, updateCommand(s, link, r.url))
	want := sandbox.Result{Stdout: "Updated cld 0.4.0 to 0.5.0: " + filepath.Join(dir, "cld") + "\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, fakeBinary("0.5.0", runtime.GOOS+"-"+runtime.GOARCH), 0o755)
	if target, err := os.Readlink(link); err != nil || target != "../opt/cld" {
		t.Errorf("the link leads to %q (%v)", target, err)
	}
}

// SIGINT, SIGTERM or SIGHUP while update downloads ends it with 128 plus the signal's number,
// printing nothing. cld stays untouched, and its temporary file is gone (decision 21.4).
func TestUpdateInterrupted(t *testing.T) {
	t.Parallel()
	for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(signal.String(), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := latestRelease(t, s, "0.5.0")
			r.hold("/download/v0.5.0/" + hostBinary)
			dir := filepath.Join(s.Root, "bin")
			placeCld(t, s, filepath.Join(dir, "cld"), oldCld(t), 0o755)
			cmd := updateCommand(s, filepath.Join(dir, "cld"), r.url)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() }) //nolint:errcheck // update may have ended
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait() //nolint:errcheck // the test reads the exit status from ProcessState
				close(done)
			}()
			select {
			case <-r.arrived:
			case <-done:
				t.Fatalf("update ended before the download stopped: stdout %q, stderr %q",
					stdout.String(), stderr.String())
			case <-time.After(15 * time.Second):
				t.Fatal("timed out waiting for the download")
			}
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("timed out waiting for update to end")
			}
			result := sandbox.Result{
				Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String(),
			}
			if want := (sandbox.Result{Code: 128 + int(signal)}); result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkCld(t, dir, string(oldCld(t)), 0o755)
		})
	}
}
