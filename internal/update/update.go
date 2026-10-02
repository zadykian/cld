// Package update is cld update: it replaces the cld that runs with the latest release's binary,
// the one install.sh installs.
//
//   - A build from source, cld dev, has no release version X.Y.Z to compare, and is refused.
//   - The latest release is the tag that releases/latest redirects to, read from the redirect
//     rather than from GitHub's API, which allows 60 calls an hour without a token.
//   - The new binary must match its line in the release's cld.sha256 and print the release's
//     version, before a rename replaces the file cld runs from.
//
// A failure, or an interrupt before the rename, leaves cld unchanged. Run returns a Result for
// cld update to report: the package prints nothing. Decision 21 gives the reasons.
package update

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/zadykian/cld/internal/fail"
)

// releases is where cld's releases are, unless CLD_RELEASES_URL says otherwise.
const releases = "https://github.com/zadykian/cld/releases"

// Result is what an update did: cld went From its release To the latest one, replacing File. Where
// File is "", cld stayed at From, which is To or newer.
type Result struct {
	From, To string
	File     string
}

// Report is what cld update says of r.
func (r Result) Report() string {
	switch {
	case r.File != "":
		return fmt.Sprintf("Updated cld %s to %s: %s\n", r.From, r.To, r.File)
	case r.From == r.To:
		return fmt.Sprintf("cld %s is the latest release\n", r.From)
	}
	return fmt.Sprintf("cld %s is newer than the latest release, %s\n", r.From, r.To)
}

// Run updates cld, which runs as version, to the latest release.
func Run(version string) (Result, error) {
	u, err := newUpdater(version)
	if err != nil {
		return Result{}, err
	}
	return u.runUntilSignal()
}

// updater is one run of cld update: cld at version current runs from file, and the releases
// publish it for this system as binary.
type updater struct {
	releases string
	current  release
	file     string
	binary   string
}

// newUpdater is the update of the cld that runs as version from its file, symbolic links
// resolved.
func newUpdater(version string) (*updater, error) {
	current, ok := parse(version)
	if !ok {
		return nil, fail.Runtime("version " + version + " is not a release: " +
			"rebuild cld from its clone, or install a release as the README says")
	}
	file, err := os.Executable()
	if err == nil {
		file, err = filepath.EvalSymlinks(file)
	}
	if err != nil {
		return nil, fail.Runtime("cannot find the file cld runs from: " + reason(err))
	}
	u := &updater{
		releases: releases,
		current:  current,
		file:     file,
		binary:   "cld-" + runtime.GOOS + "-" + runtime.GOARCH,
	}
	if address := os.Getenv("CLD_RELEASES_URL"); address != "" {
		u.releases = address
	}
	return u, nil
}

// runUntilSignal runs the update until it returns, or until SIGINT, SIGTERM or SIGHUP cancels it
// (decision 21.4). It waits for a cancelled update, so that the temporary file goes before cld
// exits; a signal once cld is replaced changes nothing.
func (u *updater) runUntilSignal() (Result, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := u.run(ctx)
		done <- outcome{result, err}
	}()
	select {
	case o := <-done:
		return o.result, o.err
	case caught := <-signals:
		cancel()
		if o := <-done; o.err == nil {
			return o.result, nil
		}
		return Result{}, fail.Status(128 + int(caught.(syscall.Signal)))
	}
}

// run replaces cld with the latest release, where cld is older.
func (u *updater) run(ctx context.Context) (Result, error) {
	latest, err := u.latest(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{From: u.current.String(), To: latest.String()}
	if !u.current.before(latest) {
		return result, nil
	}
	if err := u.replace(ctx, latest); err != nil {
		return Result{}, err
	}
	result.File = u.file
	return result, nil
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
		return exitError.String()
	}
	return err.Error()
}
