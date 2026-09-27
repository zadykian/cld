package tests

import (
	"bytes"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld update against releases the test serves (see releases), whose latest redirects to its tag
// as GitHub's does, and whose binaries are scripts that print their version: the one for the
// system the tests run on replaces cld. The cld updated is cld built as release oldRelease, copied
// into a directory of the sandbox's, since update replaces the file it runs from, and run with
// nothing on the PATH: update runs neither tmux nor claude.

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
		releasedCld.err = run("go", "build", "-ldflags", "-X main.version="+oldRelease, "-o", path, "github.com/zadykian/cld/cmd/cld")
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
// and says so: a later release by its numbers, 0.10.0 after 0.4.0. Where cld is the latest
// release, or newer, it says so and downloads nothing.
func TestUpdate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		latest string
		mode   os.FileMode
		// stdout is what update prints, with {path} for the file it replaces; "" for an update.
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
			want := sandbox.Result{Stdout: "Updated cld " + oldRelease + " to " + test.latest + ": " + path + "\n"}
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
// and leaves the link.
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
	if want := (sandbox.Result{Stdout: "Updated cld 0.4.0 to 0.5.0: " + filepath.Join(dir, "cld") + "\n"}); result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, fakeBinary("0.5.0", runtime.GOOS+"-"+runtime.GOARCH), 0o755)
	if target, err := os.Readlink(link); err != nil || target != "../opt/cld" {
		t.Errorf("the link leads to %q (%v)", target, err)
	}
}

// Once cld is replaced, update has the new cld print anew each completion script that is where
// setup completion writes one - where the variables that move it say - and writes it where it
// differs, saying so of each. A script without descriptions stays one. A script the new cld prints
// the same, a file that does not start as cobra's script does, a script elsewhere and .zshrc are
// left as they were. Without a script, update says nothing more (see TestUpdate).
func TestUpdateRefreshesCompletion(t *testing.T) {
	t.Parallel()
	const old = "# old\n"
	for _, test := range []struct {
		name string
		env  map[string]string
		// files are what the files hold, by their paths under the root, before update; want what
		// they hold after, "" where they are left as they were. updated are the files update
		// names, as it names them, shell and path.
		files   map[string]string
		want    map[string]string
		updated [][2]string
	}{
		{
			name: "default places",
			files: map[string]string{
				"home/" + scripts["bash"]: "# bash completion V2 for cld   -*- shell-script -*-\n" + old,
				"home/" + scripts["zsh"]:  "#compdef cld\n# requestComp=\"${words[1]} __completeNoDesc ${words[2,-1]}\"\n",
				"home/" + scripts["fish"]: "complete -c cld -a mine\n",
				"home/.zshrc":             zshLines,
			},
			want: map[string]string{
				"home/" + scripts["bash"]: fakeScript("0.5.0", "completion", "bash"),
				"home/" + scripts["zsh"]:  fakeScript("0.5.0", "completion", "zsh", "--no-descriptions"),
			},
			updated: [][2]string{{"bash", "home/" + scripts["bash"]}, {"zsh", "home/" + scripts["zsh"]}},
		},
		{
			name: "moved by variables",
			env:  map[string]string{"XDG_DATA_HOME": "{root}/data", "XDG_CONFIG_HOME": "{root}/config"},
			files: map[string]string{
				"data/bash-completion/completions/cld": fakeScript("0.5.0", "completion", "bash"),
				"config/fish/completions/cld.fish":     "# fish completion for cld \n" + old,
				"home/" + scripts["fish"]:              "# fish completion for cld \n" + old,
			},
			want:    map[string]string{"config/fish/completions/cld.fish": fakeScript("0.5.0", "completion", "fish")},
			updated: [][2]string{{"fish", "config/fish/completions/cld.fish"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := latestRelease(t, s, "0.5.0")
			path := filepath.Join(s.Root, "bin", "cld")
			placeCld(t, s, path, oldCld(t), 0o755)
			for name, content := range test.files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(s.Root, name)), 0o755); err != nil {
					t.Fatal(err)
				}
				s.WriteFile(filepath.Join(s.Root, name), content)
			}
			cmd := updateCommand(s, path, r.url)
			for name, value := range test.env {
				cmd.Env = append(cmd.Env, name+"="+strings.ReplaceAll(value, "{root}", s.Root))
			}
			want := "Updated cld 0.4.0 to 0.5.0: " + path + "\n"
			for _, updated := range test.updated {
				want += "Updated the completion script for " + updated[0] + ": " + filepath.Join(s.Root, updated[1]) + "\n"
			}
			if result := runCommand(t, cmd); result != (sandbox.Result{Stdout: want}) {
				t.Errorf("got %+v, want stdout\n%s", result, want)
			}
			for name, content := range test.files {
				if updated, ok := test.want[name]; ok {
					content = updated
				}
				checkContent(t, filepath.Join(s.Root, name), content)
			}
		})
	}
}

// Where the new cld cannot print a script that needs writing anew, cld is updated all the same:
// update warns, naming the script, why, and the command to run by hand, goes on with the next
// shell's, and exits with status 0.
func TestUpdateCompletionFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	const files = "download/v0.5.0"
	binary := "#!/bin/sh\n" +
		"case \"$1 $2\" in\n" +
		"'completion bash') echo 'no completion here' >&2; exit 3 ;;\n" +
		"'completion fish') printf '# fish completion for cld %s, 0.5.0\\n' \"$*\" ;;\n" +
		"*) echo 'cld 0.5.0' ;;\n" +
		"esac\n"
	r.write(t, files, hostBinary, binary)
	r.sum(t, files)
	r.setLatest("0.5.0")
	dir := filepath.Join(s.Root, "bin")
	path := filepath.Join(dir, "cld")
	placeCld(t, s, path, oldCld(t), 0o755)
	bash, fish := filepath.Join(s.Home, scripts["bash"]), filepath.Join(s.Home, scripts["fish"])
	for script, content := range map[string]string{bash: "# bash completion V2 for cld \n", fish: "# fish completion for cld \n"} {
		if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
			t.Fatal(err)
		}
		s.WriteFile(script, content)
	}
	result := runCommand(t, updateCommand(s, path, r.url))
	want := sandbox.Result{
		Stdout: "Updated cld 0.4.0 to 0.5.0: " + path + "\nUpdated the completion script for fish: " + fish + "\n",
		Stderr: "cld: warning: cannot update the completion script for bash, " + bash + ": " + path +
			" completion bash: exit status 3: no completion here. Run cld setup completion bash manually\n",
	}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, binary, 0o755)
	checkContent(t, bash, "# bash completion V2 for cld \n")
	checkContent(t, fish, fakeScript("0.5.0", "completion", "fish"))
}

// A cld built from source, cld dev, is no release to compare: update refuses before it asks
// for anything.
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
	want := sandbox.Result{Code: 1, Stderr: "cld: version dev is not a release: rebuild cld from its clone, or install a release as the README says\n"}
	if result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(dev), 0o755)
	checkAsked(t, r)
}

// Where the latest release cannot be found, lacks the binary or its checksum, or the binary does
// not match it, does not run or reports another version, update says so and exits with status 1,
// leaving cld as it was and nothing of its own.
func TestUpdateRefuses(t *testing.T) {
	t.Parallel()
	const files = "download/v0.5.0"
	for _, test := range []struct {
		name string
		// latest is the version latest redirects to, "" for none; release changes the release
		// published, where it is not nil.
		latest  string
		release func(t *testing.T, r *releases)
		// refusal is what cld says, with {url} for the releases' address; asked are the paths
		// update asks for, after /latest.
		refusal string
		asked   []string
	}{
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
			name:    "no release",
			latest:  "0.5.0",
			release: func(t *testing.T, r *releases) { _ = os.RemoveAll(filepath.Join(r.dir, files)) },
			refusal: "cannot download {url}/download/v0.5.0/cld.sha256: 404 Not Found",
			asked:   []string{"cld.sha256"},
		},
		{
			name:   "no checksum",
			latest: "0.5.0",
			release: func(t *testing.T, r *releases) {
				_ = os.Remove(filepath.Join(r.dir, files, hostBinary))
				r.sum(t, files)
			},
			refusal: "{url}/download/v0.5.0/cld.sha256 has no checksum for " + hostBinary,
			asked:   []string{"cld.sha256"},
		},
		{
			name:    "no binary",
			latest:  "0.5.0",
			release: func(t *testing.T, r *releases) { _ = os.Remove(filepath.Join(r.dir, files, hostBinary)) },
			refusal: "cannot download {url}/download/v0.5.0/" + hostBinary + ": 404 Not Found",
			asked:   []string{"cld.sha256", hostBinary},
		},
		{
			name:   "checksum mismatch",
			latest: "0.5.0",
			release: func(t *testing.T, r *releases) {
				r.write(t, files, hostBinary, fakeBinary("6.6.6", "x"))
			},
			refusal: hostBinary + " does not match its checksum in {url}/download/v0.5.0/cld.sha256",
			asked:   []string{"cld.sha256", hostBinary},
		},
		{
			name:   "binary fails",
			latest: "0.5.0",
			release: func(t *testing.T, r *releases) {
				r.write(t, files, hostBinary, "#!/bin/sh\nexit 3\n")
				r.sum(t, files)
			},
			refusal: "the downloaded " + hostBinary + " does not run: exit status 3",
			asked:   []string{"cld.sha256", hostBinary},
		},
		{
			name:   "binary of another version",
			latest: "0.5.0",
			release: func(t *testing.T, r *releases) {
				r.write(t, files, hostBinary, fakeBinary("0.6.0", "x"))
				r.sum(t, files)
			},
			refusal: "the downloaded " + hostBinary + " reports 'cld 0.6.0', not cld 0.5.0",
			asked:   []string{"cld.sha256", hostBinary},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, files, "0.5.0")
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
				asked = append(asked, "/"+files+"/"+name)
			}
			checkAsked(t, r, asked...)
		})
	}
}

// expandURL is text with {url} replaced by url.
func expandURL(text, url string) string {
	return strings.ReplaceAll(text, "{url}", url)
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
	want := sandbox.Result{Code: 1, Stderr: "cld: cannot find the latest release at " + closed.URL + "/latest: dial tcp " +
		address + ": connect: connection refused\n"}
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
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}
	result := runCommand(t, cmd)
	if want := (sandbox.Result{Code: 1, Stderr: "cld: cannot write to " + dir + ": permission denied\n"}); result != want {
		t.Errorf("got %+v, want %+v", result, want)
	}
	checkCld(t, dir, string(oldCld(t)), 0o755)
	checkAsked(t, r, "/latest", "/download/v0.5.0/cld.sha256")
}

// A signal while update downloads - SIGINT, SIGTERM or SIGHUP - ends it with 128 plus the
// signal's number, as a shell reports one, printing nothing, leaving cld as it was and its
// temporary file removed.
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
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			select {
			case <-r.arrived:
			case <-done:
				t.Fatalf("update ended before the download stopped: stdout %q, stderr %q", stdout.String(), stderr.String())
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
			result := sandbox.Result{Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
			if want := (sandbox.Result{Code: 128 + int(signal)}); result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkCld(t, dir, string(oldCld(t)), 0o755)
		})
	}
}
