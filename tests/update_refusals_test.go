package tests

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What cld update refuses, with status 1, leaving cld untouched (decision 21.4).

// updateDownload is the path of the release that update downloads.
const updateDownload = "download/v0.5.0"

// updateRefusal is a case of TestUpdateRefuses. latest is the version latest redirects to, "" for
// none. refusal is what cld says, with {url} for the releases' address. asked are the files of the
// release that update asks for after /latest.
type updateRefusal struct {
	name   string
	latest string
	// release, unless nil, changes the release published.
	release func(t *testing.T, r *releases)
	refusal string
	asked   []string
}

// updateRefusals are TestUpdateRefuses' cases.
var updateRefusals = []updateRefusal{
	{
		name:    "no latest",
		refusal: "cannot find the latest release at {url}/latest: 404 Not Found",
	},
	{
		name:    "latest not a release",
		latest:  "1.2",
		refusal: "{url}/latest leads to 'v1.2', not to a release",
	},
	{
		name:   "no release",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			updateRemove(t, r, "")
		},
		refusal: "cannot download {url}/download/v0.5.0/cld.sha256: 404 Not Found",
		asked:   []string{"cld.sha256"},
	},
	{
		name:   "no checksum",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			updateRemove(t, r, hostBinary)
			r.sum(t, updateDownload)
		},
		refusal: "{url}/download/v0.5.0/cld.sha256 has no checksum for " + hostBinary,
		asked:   []string{"cld.sha256"},
	},
	{
		name:   "no binary",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			updateRemove(t, r, hostBinary)
		},
		refusal: "cannot download {url}/download/v0.5.0/" + hostBinary + ": 404 Not Found",
		asked:   []string{"cld.sha256", hostBinary},
	},
	{
		name:   "checksum mismatch",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			r.write(t, updateDownload, hostBinary, fakeBinary("6.6.6", "x"))
		},
		refusal: hostBinary + " does not match its checksum in {url}/download/v0.5.0/cld.sha256",
		asked:   []string{"cld.sha256", hostBinary},
	},
	{
		name:   "binary fails",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			r.write(t, updateDownload, hostBinary, "#!/bin/sh\nexit 3\n")
			r.sum(t, updateDownload)
		},
		refusal: "the downloaded " + hostBinary + " does not run: exit status 3",
		asked:   []string{"cld.sha256", hostBinary},
	},
	{
		name:   "binary of another version",
		latest: "0.5.0",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			r.write(t, updateDownload, hostBinary, fakeBinary("0.6.0", "x"))
			r.sum(t, updateDownload)
		},
		refusal: "the downloaded " + hostBinary + " reports 'cld 0.6.0', not cld 0.5.0",
		asked:   []string{"cld.sha256", hostBinary},
	},
}

// updateRemove removes the file name of the release update downloads, or the whole release where
// name is empty.
func updateRemove(t *testing.T, r *releases, name string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(r.dir, updateDownload, name)); err != nil {
		t.Fatal(err)
	}
}

// expandURL is text with {url} replaced by url.
func expandURL(text, url string) string {
	return strings.ReplaceAll(text, "{url}", url)
}

// A cld built from source, cld dev, is no release to compare: update refuses before it asks for
// anything (decision 21.1).
func TestUpdateDev(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := latestRelease(t, s, "0.5.0")
	dir := filepath.Join(s.Root, "bin")
	dev, err := os.ReadFile(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	placeCld(t, s, filepath.Join(dir, "cld"), dev, 0o755)
	result := runCommand(t, updateCommand(s, filepath.Join(dir, "cld"), r.url))
	want := sandbox.Result{Code: 1, Stderr: "cld: version dev is not a release: " +
		"rebuild cld from its clone, or install a release as the README says\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(dev), 0o755)
	checkAsked(t, r)
}

// update refuses a latest release it cannot find, or one that lacks the binary or its checksum. It
// refuses a binary that does not match its checksum, does not run or reports another version. It
// leaves nothing of its own.
func TestUpdateRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range updateRefusals {
		t.Run(test.name, test.run)
	}
}

// run updates a cld that update must leave untouched.
func (test updateRefusal) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	r.publish(t, updateDownload, "0.5.0")
	r.setLatest(test.latest)
	if test.release != nil {
		test.release(t, r)
	}
	dir := filepath.Join(s.Root, "bin")
	placeCld(t, s, filepath.Join(dir, "cld"), oldCld(t), 0o755)
	result := runCommand(t, updateCommand(s, filepath.Join(dir, "cld"), r.url))
	want := sandbox.Result{Code: 1, Stderr: "cld: " + expandURL(test.refusal, r.url) + "\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(oldCld(t)), 0o755)
	asked := []string{"/latest"}
	for _, name := range test.asked {
		asked = append(asked, "/"+updateDownload+"/"+name)
	}
	checkAsked(t, r, asked...)
}

// Where no server answers at the releases' address, update says what the connection met.
func TestUpdateUnreachable(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	closed := httptest.NewServer(nil)
	closed.Close()
	dir := filepath.Join(s.Root, "bin")
	placeCld(t, s, filepath.Join(dir, "cld"), oldCld(t), 0o755)
	result := runCommand(t, updateCommand(s, filepath.Join(dir, "cld"), closed.URL))
	address := closed.URL[len("http://"):]
	want := sandbox.Result{Code: 1, Stderr: "cld: cannot find the latest release at " + closed.URL +
		"/latest: dial tcp " + address + ": connect: connection refused\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(oldCld(t)), 0o755)
}

// Where cld's directory takes no new file, update says so, having asked for the checksums alone.
// root writes anywhere, so run as root the test runs cld as nobody (65534), on Linux.
func TestUpdateCannotWrite(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := latestRelease(t, s, "0.5.0")
	dir := filepath.Join(s.Root, "bin")
	placeCld(t, s, filepath.Join(dir, "cld"), oldCld(t), 0o755)
	cmd := updateCommand(s, filepath.Join(dir, "cld"), r.url)
	if os.Geteuid() == 0 {
		if runtime.GOOS != "linux" {
			t.Skip("run as root, the test runs cld as nobody on Linux only")
		}
		// nobody enters the sandbox, and cannot write in root's bin.
		for _, path := range []string{s.Root, s.Work} {
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	} else {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(dir, 0o755) //nolint:errcheck // best effort, as the sandbox's removal
		})
	}
	result := runCommand(t, cmd)
	want := sandbox.Result{Code: 1, Stderr: "cld: cannot write to " + dir + ": permission denied\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(oldCld(t)), 0o755)
	checkAsked(t, r, "/latest", "/download/v0.5.0/cld.sha256")
}
