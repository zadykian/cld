package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// install.sh piped into a shell, as the README has it, against releases an HTTP server of the
// test's serves (decision 20.6). Each binary is a script that prints its version, so every
// platform's runs here, and a fake uname picks one. The installer's PATH holds only the tools it
// runs, no cld of the user's.

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
func runInstaller(
	t *testing.T, s *sandbox.Sandbox, shell, script string, extra map[string]string,
) sandbox.Result {
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

// Piped into sh or bash, the installer downloads the latest release's binary for the system into
// ~/.local/bin, checked against cld.sha256 with sha256sum or shasum. It says where it put which
// version, and warns where ~/.local/bin is missing from the PATH.
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
				Stderr: "install.sh: " + dir +
					" is not on your PATH: add it there, in your shell's profile\n",
			}
			if result != want {
				t.Errorf("got %+v, want %+v", result, want)
			}
			checkInstalled(t, dir, fakeBinary("1.2.3", "linux-amd64"))
			checkAsked(t, r, "/latest/download/cld.sha256", "/latest/download/cld-linux-amd64")
		})
	}
}

// CLD_INSTALL_DIR is where cld goes, made if missing, and resolved against the current directory
// when relative. The installer names it as the PATH has it, and warns of nothing while the PATH
// holds it, whatever the spelling given.
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

// A cld there is replaced, a symbolic link too, never what the link points to (decision 20.4).
// A cld that comes first on the PATH is named, as the shell runs that one.
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
		"PATH":             defaultTools(t, s) + ":" + other + ":" + dir,
		"CLD_RELEASES_URL": r.url,
		"CLD_INSTALL_DIR":  dir,
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
	if data, err := os.ReadFile(filepath.Join(other, "cld")); err != nil || string(data) != "old" {
		t.Errorf("the cld the link pointed to holds %q (%v)", data, err)
	}

	r.publish(t, "latest/download", "1.2.4")
	want.Stdout = "installed cld 1.2.4 as " + dir + "/cld\n"
	if result := runInstaller(t, s, "sh", installScript(t), env); result != want {
		t.Errorf("over a cld: got %+v, want %+v", result, want)
	}
	checkInstalled(t, dir, fakeBinary("1.2.4", "linux-amd64"))
}

// A download of the installer cut short, at the end of any of its lines, runs nothing: no
// download starts and nothing is installed. Only the whole script does (decision 20.2).
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
			env := map[string]string{
				"PATH": defaultTools(t, s), "CLD_RELEASES_URL": r.url, "CLD_INSTALL_DIR": dir,
			}
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
