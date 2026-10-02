package tests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Which release and which of its binaries install.sh downloads (decisions 20.1 and 20.3).

// The binary is the one for uname's system and machine, or Apple silicon's where Rosetta 2
// translates the shell on macOS. Other systems and machines are refused before any download.
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
		{
			os: "MINGW64_NT-10.0-26100", arch: "x86_64",
			refusal: "cld is released for Linux and macOS, not MINGW64_NT-10.0-26100",
		},
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
	const script = "was a script, not a binary: see " +
		"https://github.com/zadykian/cld/blob/main/docs/guide.md#upgrading"
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
