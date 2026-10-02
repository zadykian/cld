package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What install.sh refuses, with status 1, leaving any cld there untouched and nothing of its own
// (decision 20.4).

// installLatest is the path of the latest release under the releases' URL.
const installLatest = "latest/download"

// installRefusal is a case of TestInstallRefuses: what the installer gets, what it says, with
// {url} for the releases' URL, and the files of the latest release it asks for.
type installRefusal struct {
	name string
	// release, unless nil, changes the latest release as published.
	release func(t *testing.T, r *releases)
	// tools are the real tools on the PATH; all the installer runs where nil.
	tools   []string
	refusal string
	asked   []string
}

// installRefusals are TestInstallRefuses' cases.
var installRefusals = []installRefusal{
	{
		name: "no release",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			installRemove(t, r, "")
		},
		refusal: "cannot download {url}/latest/download/cld.sha256",
		asked:   []string{"cld.sha256"},
	},
	{
		name: "no binary",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			installRemove(t, r, "cld-linux-amd64")
		},
		refusal: "cannot download {url}/latest/download/cld-linux-amd64",
		asked:   []string{"cld.sha256", "cld-linux-amd64"},
	},
	{
		name: "no checksum",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			installRemove(t, r, "cld-linux-amd64")
			r.sum(t, installLatest)
		},
		refusal: "{url}/latest/download/cld.sha256 has no checksum for cld-linux-amd64",
		asked:   []string{"cld.sha256"},
	},
	{
		name: "checksum of another binary",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			sums := readFile(t, filepath.Join(r.dir, installLatest, "cld.sha256"))
			r.write(t, installLatest, "cld.sha256",
				strings.ReplaceAll(sums, "cld-linux-amd64", "cld-linux-amd64.old"))
		},
		refusal: "{url}/latest/download/cld.sha256 has no checksum for cld-linux-amd64",
		asked:   []string{"cld.sha256"},
	},
	{
		name: "checksum mismatch",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			r.write(t, installLatest, "cld-linux-amd64", fakeBinary("6.6.6", "linux-amd64"))
		},
		refusal: "cld-linux-amd64 does not match its checksum in {url}/latest/download/cld.sha256",
		asked:   []string{"cld.sha256", "cld-linux-amd64"},
	},
	{
		name: "binary fails",
		release: func(t *testing.T, r *releases) {
			t.Helper()
			r.write(t, installLatest, "cld-linux-amd64", "#!/bin/sh\nexit 3\n")
			r.sum(t, installLatest)
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
}

// installRemove removes the file name of the latest release, or the whole release where name is
// empty.
func installRemove(t *testing.T, r *releases, name string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(r.dir, installLatest, name)); err != nil {
		t.Fatal(err)
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

// The installer refuses a release that lacks the binary or its checksum, or whose binary does not
// match it or does not run. It also refuses where a tool is missing.
func TestInstallRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range installRefusals {
		t.Run(test.name, test.run)
	}
}

// run runs the installer over a cld it must leave untouched.
func (test installRefusal) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	r := newReleases(t, s)
	r.publish(t, installLatest, "1.2.3")
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
