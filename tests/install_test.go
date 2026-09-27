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

// install.sh, the installer each release publishes, piped into a shell as the README has it,
// against an HTTP server of the test's in place of GitHub's releases (CLD_RELEASES_URL). The
// binaries it serves are scripts that print their version as cld --version does, so that every
// platform's runs here, and a fake uname picks one. The installer's PATH is the tools it runs and
// nothing else: no cld of the user's.

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
			_, _ = w.Write([]byte("#"))
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

// fakeBinary is what a release of version serves as cld for platform.
func fakeBinary(version, platform string) string {
	return "#!/bin/sh\n# cld-" + platform + "\necho 'cld " + version + "'\n"
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

// checksumTool is the tool the installer checks the checksum with where both are installed.
func checksumTool() string {
	if _, err := exec.LookPath("sha256sum"); err == nil {
		return "sha256sum"
	}
	return "shasum"
}

// installTools is a directory of the tools the installer runs, for its PATH: the real ones named,
// and a uname that answers os for -s and arch for -m.
func installTools(t *testing.T, s *sandbox.Sandbox, os, arch string, names ...string) string {
	t.Helper()
	dir := s.Tools(names...)
	s.WriteProgram(filepath.Join(dir, "uname"),
		"#!/bin/sh\ncase $1 in\n-s) echo "+os+" ;;\n-m) echo "+arch+" ;;\nesac\n", 0o755)
	return dir
}

// installerTools are the real tools the installer runs, but the checksum's, and those named.
func installerTools(names ...string) []string {
	return append([]string{"awk", "chmod", "curl", "mkdir", "mktemp", "mv", "rm"}, names...)
}

// defaultTools is installTools for Linux on x86_64, with every tool the installer runs.
func defaultTools(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	return installTools(t, s, "Linux", "x86_64", installerTools(checksumTool())...)
}

// runInstaller pipes script into shell, as the README has it, in the sandbox's work directory,
// with the sandbox's environment and extra variables: PATH among them.
func runInstaller(t *testing.T, s *sandbox.Sandbox, shell, script string, extra map[string]string) sandbox.Result {
	t.Helper()
	path, err := exec.LookPath(shell)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path)
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = s.Environ(extra)
	cmd.Dir = s.Work
	return runCommand(t, cmd)
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
	return sandbox.Result{Code: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
}

// installScript is install.sh, as the repository has it.
func installScript(t *testing.T) string {
	t.Helper()
	return repoFile(t, "install.sh")
}

// checkInstalled reports a dir that does not hold cld, and cld alone, with content, executable.
func checkInstalled(t *testing.T, dir, content string) {
	t.Helper()
	checkCld(t, dir, content, 0o755)
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

// Piped into sh or bash, the installer downloads the latest release's binary for the system,
// checked against cld.sha256 with sha256sum or shasum, into ~/.local/bin, and says where it put
// which version; and that ~/.local/bin is not on the PATH, when it is not.
func TestInstall(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, shell, checksum string }{
		{"sh", "sh", checksumTool()},
		{"bash", "bash", checksumTool()},
		{"sha256sum", "sh", "sha256sum"},
		{"shasum", "sh", "shasum"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := exec.LookPath(test.checksum); err != nil {
				t.Skip(test.checksum + " is not installed")
			}
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, "latest/download", "1.2.3")
			tools := installTools(t, s, "Linux", "x86_64", installerTools(test.checksum)...)
			result := runInstaller(t, s, test.shell, installScript(t), map[string]string{
				"PATH": tools, "CLD_RELEASES_URL": r.url,
			})
			dir := filepath.Join(s.Home, ".local", "bin")
			want := sandbox.Result{
				Stdout: "installed cld 1.2.3 as " + dir + "/cld\n",
				Stderr: "install.sh: " + dir + " is not on your PATH: add it there, in your shell's profile\n",
			}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkInstalled(t, dir, fakeBinary("1.2.3", "linux-amd64"))
			checkAsked(t, r, "/latest/download/cld.sha256", "/latest/download/cld-linux-amd64")
		})
	}
}

// The binary is the one for uname's system and machine, and on macOS for Apple silicon where
// Rosetta 2 translates the shell; other systems and machines are refused before any download.
func TestInstallPlatforms(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		os, arch, translated string
		platform, refusal    string
	}{
		{os: "Linux", arch: "x86_64", platform: "linux-amd64"},
		{os: "Linux", arch: "aarch64", platform: "linux-arm64"},
		{os: "Darwin", arch: "x86_64", platform: "darwin-amd64"},
		{os: "Darwin", arch: "x86_64", translated: "0", platform: "darwin-amd64"},
		{os: "Darwin", arch: "x86_64", translated: "1", platform: "darwin-arm64"},
		{os: "Darwin", arch: "arm64", platform: "darwin-arm64"},
		{os: "Linux", arch: "amd64", platform: "linux-amd64"},
		{os: "Linux", arch: "arm64", platform: "linux-arm64"},
		{os: "FreeBSD", arch: "amd64", refusal: "cld is released for Linux and macOS, not FreeBSD"},
		{os: "MINGW64_NT-10.0-26100", arch: "x86_64", refusal: "cld is released for Linux and macOS, not MINGW64_NT-10.0-26100"},
		{os: "Linux", arch: "i686", refusal: "cld is released for x86_64 and arm64, not i686"},
		{os: "Darwin", arch: "ppc", refusal: "cld is released for x86_64 and arm64, not ppc"},
	} {
		t.Run(test.os+"-"+test.arch+test.translated, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, "latest/download", "1.2.3")
			tools := installTools(t, s, test.os, test.arch, installerTools(checksumTool())...)
			if test.translated != "" {
				s.WriteProgram(filepath.Join(tools, "sysctl"),
					"#!/bin/sh\n[ \"$*\" = '-n sysctl.proc_translated' ] && echo "+test.translated+"\n", 0o755)
			}
			dir := filepath.Join(s.Root, "bin")
			result := runInstaller(t, s, "sh", installScript(t), map[string]string{
				"PATH": tools + ":" + dir, "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir,
			})
			if test.refusal != "" {
				want := sandbox.Result{Code: 1, Stderr: "install.sh: " + test.refusal + "\n"}
				if result != want {
					t.Errorf("got %+v, want %+v", result, want)
				}
				checkAsked(t, r)
				if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s exists (%v)", dir, err)
				}
				return
			}
			want := sandbox.Result{Stdout: "installed cld 1.2.3 as " + dir + "/cld\n"}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkInstalled(t, dir, fakeBinary("1.2.3", test.platform))
			checkAsked(t, r, "/latest/download/cld.sha256", "/latest/download/cld-"+test.platform)
		})
	}
}

// CLD_VERSION picks a release, with or without its tag's v; unset, empty or latest, the latest.
// The releases before 0.4.0 published a script, not binaries, and anything but X.Y.Z is no
// version: both are refused before any download.
func TestInstallVersion(t *testing.T) {
	t.Parallel()
	const script = "was a script, not a binary: see https://github.com/zadykian/cld/blob/main/docs/guide.md#upgrading"
	for _, test := range []struct {
		version, path, installed, refusal string
	}{
		{version: "0.4.0", path: "download/v0.4.0", installed: "0.4.0"},
		{version: "v0.4.0", path: "download/v0.4.0", installed: "0.4.0"},
		{version: "10.20.30", path: "download/v10.20.30", installed: "10.20.30"},
		{version: "", path: "latest/download", installed: "1.2.3"},
		{version: "latest", path: "latest/download", installed: "1.2.3"},
		{version: "0.3.0", refusal: "cld 0.3.0 " + script},
		{version: "v0.1.1", refusal: "cld v0.1.1 " + script},
		{version: "0.0.1", refusal: "cld 0.0.1 " + script},
		{version: "1.2", refusal: "CLD_VERSION is not a version, X.Y.Z: 1.2"},
		{version: "1.2.3-rc1", refusal: "CLD_VERSION is not a version, X.Y.Z: 1.2.3-rc1"},
		{version: "1.2.3/../../x", refusal: "CLD_VERSION is not a version, X.Y.Z: 1.2.3/../../x"},
		{version: "vv1.2.3", refusal: "CLD_VERSION is not a version, X.Y.Z: vv1.2.3"},
		{version: "LATEST", refusal: "CLD_VERSION is not a version, X.Y.Z: LATEST"},
	} {
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, "latest/download", "1.2.3")
			for _, version := range []string{"0.4.0", "10.20.30"} {
				r.publish(t, "download/v"+version, version)
			}
			dir := filepath.Join(s.Root, "bin")
			result := runInstaller(t, s, "sh", installScript(t), map[string]string{
				"PATH": defaultTools(t, s) + ":" + dir, "CLD_RELEASES_URL": r.url,
				"CLD_INSTALL_DIR": dir, "CLD_VERSION": test.version,
			})
			if test.refusal != "" {
				want := sandbox.Result{Code: 1, Stderr: "install.sh: " + test.refusal + "\n"}
				if result != want {
					t.Errorf("got %+v, want %+v", result, want)
				}
				checkAsked(t, r)
				return
			}
			want := sandbox.Result{Stdout: "installed cld " + test.installed + " as " + dir + "/cld\n"}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkInstalled(t, dir, fakeBinary(test.installed, "linux-amd64"))
			checkAsked(t, r, "/"+test.path+"/cld.sha256", "/"+test.path+"/cld-linux-amd64")
		})
	}
}

// CLD_INSTALL_DIR is where cld goes, made where it is missing, relative to the current directory
// where it is relative. The installer names it as the PATH has it: with no warning when it is on
// the PATH, whatever the spelling given.
func TestInstallDir(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, given, dir string }{
		{"absolute", "{root}/opt/cld/bin", "{root}/opt/cld/bin"},
		{"trailing slash", "{root}/opt/cld/bin/", "{root}/opt/cld/bin"},
		{"relative", "bin", "{work}/bin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			expand := strings.NewReplacer("{root}", s.Root, "{work}", s.Work).Replace
			dir := expand(test.dir)
			r := newReleases(t, s)
			r.publish(t, "latest/download", "1.2.3")
			result := runInstaller(t, s, "sh", installScript(t), map[string]string{
				"PATH": defaultTools(t, s) + ":" + dir, "CLD_RELEASES_URL": r.url,
				"CLD_INSTALL_DIR": expand(test.given),
			})
			want := sandbox.Result{Stdout: "installed cld 1.2.3 as " + dir + "/cld\n"}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkInstalled(t, dir, fakeBinary("1.2.3", "linux-amd64"))
		})
	}
}

// A cld there is replaced, a symbolic link too, never what it points to. A cld that comes first
// on the PATH is named: it is the one the shell runs.
func TestInstallReplaces(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	r.publish(t, "latest/download", "1.2.3")
	dir, other := filepath.Join(s.Root, "bin"), filepath.Join(s.Root, "other")
	for _, d := range []string{dir, other} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.WriteProgram(filepath.Join(other, "cld"), "old", 0o755)
	env := map[string]string{
		"PATH": defaultTools(t, s) + ":" + other + ":" + dir, "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir,
	}
	want := sandbox.Result{
		Stdout: "installed cld 1.2.3 as " + dir + "/cld\n",
		Stderr: "install.sh: " + other + "/cld comes first on your PATH, before " + dir + "/cld\n",
	}

	if err := os.Symlink(filepath.Join(other, "cld"), filepath.Join(dir, "cld")); err != nil {
		t.Fatal(err)
	}
	if result := runInstaller(t, s, "sh", installScript(t), env); result != want {
		t.Errorf("over a symbolic link: got %+v, want %+v", result, want)
	}
	checkInstalled(t, dir, fakeBinary("1.2.3", "linux-amd64"))
	if data, _ := os.ReadFile(filepath.Join(other, "cld")); string(data) != "old" {
		t.Errorf("the cld the link pointed to holds %q", data)
	}

	r.publish(t, "latest/download", "1.2.4")
	want.Stdout = "installed cld 1.2.4 as " + dir + "/cld\n"
	if result := runInstaller(t, s, "sh", installScript(t), env); result != want {
		t.Errorf("over a cld: got %+v, want %+v", result, want)
	}
	checkInstalled(t, dir, fakeBinary("1.2.4", "linux-amd64"))
}

// Where the release lacks the binary or its checksum, or the binary does not match it or does not
// run, and where a tool is missing or the directory cannot be made, the installer says so and
// exits with status 1, leaving any cld there as it was and nothing of its own.
func TestInstallRefuses(t *testing.T) {
	t.Parallel()
	const latest = "latest/download"
	for _, test := range []struct {
		name string
		// release changes the latest release, as published, where it is not nil.
		release func(t *testing.T, r *releases)
		// tools are the real tools on the PATH; all the installer runs where nil.
		tools   []string
		refusal string
		asked   []string
	}{
		{
			name:    "no release",
			release: func(t *testing.T, r *releases) { _ = os.RemoveAll(filepath.Join(r.dir, latest)) },
			refusal: "cannot download {url}/latest/download/cld.sha256",
			asked:   []string{"cld.sha256"},
		},
		{
			name: "no binary",
			release: func(t *testing.T, r *releases) {
				_ = os.Remove(filepath.Join(r.dir, latest, "cld-linux-amd64"))
			},
			refusal: "cannot download {url}/latest/download/cld-linux-amd64",
			asked:   []string{"cld.sha256", "cld-linux-amd64"},
		},
		{
			name: "no checksum",
			release: func(t *testing.T, r *releases) {
				_ = os.Remove(filepath.Join(r.dir, latest, "cld-linux-amd64"))
				r.sum(t, latest)
			},
			refusal: "{url}/latest/download/cld.sha256 has no checksum for cld-linux-amd64",
			asked:   []string{"cld.sha256"},
		},
		{
			name: "checksum of another binary",
			release: func(t *testing.T, r *releases) {
				r.write(t, latest, "cld.sha256", strings.ReplaceAll(
					readFile(t, filepath.Join(r.dir, latest, "cld.sha256")), "cld-linux-amd64", "cld-linux-amd64.old"))
			},
			refusal: "{url}/latest/download/cld.sha256 has no checksum for cld-linux-amd64",
			asked:   []string{"cld.sha256"},
		},
		{
			name: "checksum mismatch",
			release: func(t *testing.T, r *releases) {
				r.write(t, latest, "cld-linux-amd64", fakeBinary("6.6.6", "linux-amd64"))
			},
			refusal: "cld-linux-amd64 does not match its checksum in {url}/latest/download/cld.sha256",
			asked:   []string{"cld.sha256", "cld-linux-amd64"},
		},
		{
			name: "binary fails",
			release: func(t *testing.T, r *releases) {
				r.write(t, latest, "cld-linux-amd64", "#!/bin/sh\nexit 3\n")
				r.sum(t, latest)
			},
			refusal: "the cld-linux-amd64 downloaded does not run",
			asked:   []string{"cld.sha256", "cld-linux-amd64"},
		},
		{
			name:    "no curl",
			tools:   []string{"awk", "chmod", "mkdir", "mktemp", "mv", "rm", checksumTool()},
			refusal: "curl is not installed",
		},
		{
			name:    "no checksum tool",
			tools:   installerTools(),
			refusal: "sha256sum or shasum is needed to check cld's checksum",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, latest, "1.2.3")
			if test.release != nil {
				test.release(t, r)
			}
			names := test.tools
			if names == nil {
				names = installerTools(checksumTool())
			}
			tools := installTools(t, s, "Linux", "x86_64", names...)
			dir := filepath.Join(s.Root, "bin")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			s.WriteProgram(filepath.Join(dir, "cld"), "old", 0o755)
			result := runInstaller(t, s, "sh", installScript(t), map[string]string{
				"PATH": tools + ":" + dir, "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir,
			})
			refusal := "install.sh: " + strings.ReplaceAll(test.refusal, "{url}", r.url) + "\n"
			if result.Code != 1 || result.Stdout != "" || !strings.HasSuffix(result.Stderr, refusal) {
				t.Errorf("got %+v, want status 1 and stderr ending in %q", result, refusal)
			}
			checkInstalled(t, dir, "old")
			var asked []string
			for _, name := range test.asked {
				asked = append(asked, "/latest/download/"+name)
			}
			checkAsked(t, r, asked...)
		})
	}
}

// A directory that cannot be made is refused, after mkdir's message.
func TestInstallCannotCreateDir(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	r.publish(t, "latest/download", "1.2.3")
	file := filepath.Join(s.Root, "file")
	s.WriteFile(file, "")
	dir := filepath.Join(file, "bin")
	result := runInstaller(t, s, "sh", installScript(t), map[string]string{
		"PATH": defaultTools(t, s), "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir,
	})
	refusal := "install.sh: cannot create " + dir + "\n"
	if result.Code != 1 || result.Stdout != "" || !strings.HasSuffix(result.Stderr, refusal) {
		t.Errorf("got %+v, want status 1 and stderr ending in %q", result, refusal)
	}
	checkAsked(t, r, "/latest/download/cld.sha256")
}

// A download of the installer cut short, at the end of any of its lines, runs nothing: no
// download starts and nothing is installed. Only the whole script does.
func TestInstallCutShort(t *testing.T) {
	t.Parallel()
	script := installScript(t)
	lines := strings.SplitAfter(strings.TrimSuffix(script, "\n"), "\n")
	for _, shell := range []string{"sh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			r := newReleases(t, s)
			r.publish(t, "latest/download", "1.2.3")
			dir := filepath.Join(s.Root, "bin")
			env := map[string]string{"PATH": defaultTools(t, s), "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir}
			for end := range lines {
				runInstaller(t, s, shell, strings.Join(lines[:end], ""), env)
				if asked := r.asked(); len(asked) > 0 {
					t.Fatalf("the first %d lines asked for %q", end, asked)
				}
				if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("the first %d lines made %s (%v)", end, dir, err)
				}
			}
			if result := runInstaller(t, s, shell, script, env); result.Code != 0 {
				t.Errorf("the whole script: %+v", result)
			}
		})
	}
}

// readFile is the content of the file at path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
