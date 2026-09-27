// Package update is cld update: it replaces the cld that runs with the latest release's, the
// binary that install.sh installs:
//
//   - the version cld runs as must be a release's, X.Y.Z: a build from source, cld dev, is not
//     compared with the releases, and is left to the clone it was built from;
//   - the latest release is the one https://github.com/zadykian/cld/releases/latest redirects to,
//     .../releases/tag/vX.Y.Z, read without following the redirect: GitHub's API says the same,
//     but takes 60 calls an hour without a token. Where cld is that release, or a newer one, there
//     is nothing to do;
//   - the binary is the release's cld-GOOS-GOARCH, for the system and machine cld was built for.
//     It is downloaded into a temporary file beside the file cld runs from - the file a symbolic
//     link leads to - and checked against its line in the release's cld.sha256, given the old
//     file's permissions and run with --version, which must print the release's version. Only
//     then is it renamed over that file: the rename replaces cld at once, and a cld running
//     meanwhile keeps its old file.
//
// Anything that fails leaves cld as it was, and the temporary file removed; an interrupt (SIGINT,
// SIGTERM or SIGHUP) before the rename too, ending cld with 128 plus the signal's number, as a
// shell reports one. Redirects stay on HTTPS, as install.sh's do. CLD_RELEASES_URL stands in for
// the releases' address, for the tests.
package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// releases is where cld's releases are, unless CLD_RELEASES_URL says otherwise.
const releases = "https://github.com/zadykian/cld/releases"

// Run updates cld, which runs as version, to the latest release.
func Run(version string) error {
	current, ok := parse(version)
	if !ok {
		return fail.Runtime(fmt.Sprintf("version %s is not a release: rebuild cld from its clone, or install a release as the README says", version))
	}
	file, err := os.Executable()
	if err == nil {
		file, err = filepath.EvalSymlinks(file)
	}
	if err != nil {
		return fail.Runtime("cannot find the file cld runs from: " + reason(err))
	}
	u := &updater{releases: releases, current: current, file: file, binary: "cld-" + runtime.GOOS + "-" + runtime.GOARCH}
	if address := os.Getenv("CLD_RELEASES_URL"); address != "" {
		u.releases = address
	}

	// The update runs until it is done, or until a signal cancels it, which it waits for: the
	// temporary file goes before cld exits. A signal after the rename changes nothing.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- u.run(ctx) }()
	select {
	case err := <-done:
		return err
	case caught := <-signals:
		cancel()
		if err := <-done; err == nil || u.renamed {
			return err
		}
		return fail.Status(128 + int(caught.(syscall.Signal)))
	}
}

// updater is one run of cld update: cld at version current runs from file, and the releases
// publish it for this system as binary.
type updater struct {
	releases string
	current  release
	file     string
	binary   string
	// renamed is whether the new cld has replaced the old one.
	renamed bool
}

func (u *updater) run(ctx context.Context) error {
	latest, err := u.latest(ctx)
	if err != nil {
		return err
	}
	switch {
	case latest == u.current:
		return fail.Print(fmt.Sprintf("cld %s is the latest release\n", u.current))
	case latest.before(u.current):
		return fail.Print(fmt.Sprintf("cld %s is newer than the latest release, %s\n", u.current, latest))
	}
	files := fmt.Sprintf("%s/download/v%s/", u.releases, latest)
	var sums bytes.Buffer
	if err := download(ctx, files+"cld.sha256", &sums); err != nil {
		return err
	}
	want, ok := checksum(sums.String(), u.binary)
	if !ok {
		return fail.Runtime(fmt.Sprintf("%scld.sha256 has no checksum for %s", files, u.binary))
	}
	info, err := os.Stat(u.file)
	if err != nil {
		return fail.Runtime("cannot read " + u.file + ": " + reason(err))
	}
	dir := filepath.Dir(u.file)
	temporary, err := os.CreateTemp(dir, ".cld.")
	if err != nil {
		return fail.Runtime("cannot write to " + dir + ": " + reason(err))
	}
	defer func() {
		if !u.renamed {
			_ = os.Remove(temporary.Name())
		}
	}()
	hash := sha256.New()
	err = download(ctx, files+u.binary, io.MultiWriter(temporary, hash))
	if closeErr := temporary.Close(); err == nil && closeErr != nil {
		err = fail.Runtime("cannot write to " + dir + ": " + reason(closeErr))
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return fail.Runtime(fmt.Sprintf("%s does not match its checksum in %scld.sha256", u.binary, files))
	}
	if err := os.Chmod(temporary.Name(), info.Mode().Perm()); err != nil {
		return fail.Runtime("cannot write to " + dir + ": " + reason(err))
	}
	out, err := exec.CommandContext(ctx, temporary.Name(), "--version").Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fail.Runtime(fmt.Sprintf("the downloaded %s does not run: %s", u.binary, reason(err)))
	}
	if reported := strings.TrimSuffix(string(out), "\n"); reported != "cld "+latest.String() {
		return fail.Runtime(fmt.Sprintf("the downloaded %s reports '%s', not cld %s", u.binary, reported, latest))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := os.Rename(temporary.Name(), u.file); err != nil {
		return fail.Runtime("cannot replace " + u.file + ": " + reason(err))
	}
	u.renamed = true
	return fail.Print(fmt.Sprintf("Updated cld %s to %s: %s\n", u.current, latest, u.file))
}

// latest is the latest release: the tag that the releases' latest redirects to.
func (u *updater) latest(ctx context.Context) (release, error) {
	address := u.releases + "/latest"
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return release{}, fail.Runtime("cannot find the latest release at " + address + ": " + reason(err))
	}
	response, err := client.Do(request)
	if err != nil {
		return release{}, fail.Runtime("cannot find the latest release at " + address + ": " + reason(err))
	}
	_ = response.Body.Close()
	location, err := response.Location()
	if err != nil {
		return release{}, fail.Runtime("cannot find the latest release at " + address + ": " + response.Status)
	}
	tag := path.Base(location.Path)
	latest, ok := parse(strings.TrimPrefix(tag, "v"))
	if !ok || !strings.HasPrefix(tag, "v") {
		return release{}, fail.Runtime(fmt.Sprintf("%s leads to '%s', not to a release", address, tag))
	}
	return latest, nil
}

// transport is how cld update reaches the releases: as Go's default transport does, through a
// proxy the environment names, but for a server that answers nothing within 30 seconds.
var transport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return t
}()

// download writes the file at address to w, following redirects to HTTPS addresses only.
func download(ctx context.Context, address string, w io.Writer) error {
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "https" {
				return errors.New("redirected to " + request.URL.String() + ", which is not HTTPS")
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail.Runtime("cannot download " + address + ": " + response.Status)
	}
	if _, err := io.Copy(w, response.Body); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	return nil
}

// checksum is the SHA-256 checksum that sums, a cld.sha256 as sha256sum writes it, gives the file
// name: the first of its lines "HASH  NAME", or "HASH *NAME".
func checksum(sums, name string) ([]byte, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		return sum, err == nil && len(sum) == sha256.Size
	}
	return nil, false
}

// release is a release's version, X.Y.Z.
type release [3]int

// parse reads a release's version, X.Y.Z, each a decimal number.
func parse(version string) (release, bool) {
	var r release
	parts := strings.Split(version, ".")
	if len(parts) != len(r) {
		return r, false
	}
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return r, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return r, false
		}
		r[i] = n
	}
	return r, true
}

func (r release) before(other release) bool {
	for i := range r {
		if r[i] != other[i] {
			return r[i] < other[i]
		}
	}
	return false
}

func (r release) String() string {
	return fmt.Sprintf("%d.%d.%d", r[0], r[1], r[2])
}

// reason is what err says, without the operation and file a path error names, or the method and
// address of a request.
func reason(err error) string {
	var (
		pathError *fs.PathError
		urlError  *url.Error
		exitError *exec.ExitError
	)
	switch {
	case errors.As(err, &pathError):
		return pathError.Err.Error()
	case errors.As(err, &urlError):
		return urlError.Err.Error()
	case errors.As(err, &exitError):
		return exitError.ProcessState.String()
	}
	return err.Error()
}
