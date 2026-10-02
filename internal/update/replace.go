package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

// replace renames release latest's binary over the file cld runs from, once the binary matches
// its checksum and prints that version. It downloads beside that file, so that the rename stays
// on one file system.
func (u *updater) replace(ctx context.Context, latest release) error {
	files := fmt.Sprintf("%s/download/v%s/", u.releases, latest)
	want, err := u.sum(ctx, files)
	if err != nil {
		return err
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
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(temporary.Name()) //nolint:errcheck // best effort: the failure is returned
		}
	}()
	if err := u.fetch(ctx, temporary, files, want); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), info.Mode().Perm()); err != nil {
		return fail.Runtime("cannot write to " + dir + ": " + reason(err))
	}
	if err := u.verify(ctx, temporary.Name(), latest); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), u.file); err != nil {
		return fail.Runtime("cannot replace " + u.file + ": " + reason(err))
	}
	renamed = true
	return nil
}

// sum is the binary's SHA-256 checksum, read from the cld.sha256 among files.
func (u *updater) sum(ctx context.Context, files string) ([]byte, error) {
	var sums bytes.Buffer
	if err := download(ctx, files+"cld.sha256", &sums); err != nil {
		return nil, err
	}
	want, ok := checksum(sums.String(), u.binary)
	if !ok {
		return nil, fail.Runtime(fmt.Sprintf("%scld.sha256 has no checksum for %s", files, u.binary))
	}
	return want, nil
}

// fetch downloads the binary from files into temporary, closes it, and checks the binary against
// its checksum, want.
func (u *updater) fetch(ctx context.Context, temporary *os.File, files string, want []byte) error {
	hash := sha256.New()
	err := download(ctx, files+u.binary, io.MultiWriter(temporary, hash))
	if closeErr := temporary.Close(); err == nil && closeErr != nil {
		err = fail.Runtime("cannot write to " + filepath.Dir(u.file) + ": " + reason(closeErr))
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return fail.Runtime(fmt.Sprintf(
			"%s does not match its checksum in %scld.sha256", u.binary, files))
	}
	return nil
}

// verify runs the downloaded binary at file with --version, which must print release latest's
// version.
func (u *updater) verify(ctx context.Context, file string, latest release) error {
	out, err := exec.CommandContext(ctx, file, "--version").Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fail.Runtime(fmt.Sprintf(
			"the downloaded %s does not run: %s", u.binary, reason(err)))
	}
	if reported := strings.TrimSuffix(string(out), "\n"); reported != "cld "+latest.String() {
		return fail.Runtime(fmt.Sprintf(
			"the downloaded %s reports '%s', not cld %s", u.binary, reported, latest))
	}
	return ctx.Err()
}

// checksum is the SHA-256 checksum that sums, a cld.sha256 as sha256sum writes it, gives the file
// name: the first of its lines "HASH  NAME", or "HASH *NAME".
func checksum(sums, name string) ([]byte, bool) {
	for line := range strings.SplitSeq(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		return sum, err == nil && len(sum) == sha256.Size
	}
	return nil, false
}
