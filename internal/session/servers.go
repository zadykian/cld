package session

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// socketDir is the directory tmux keeps the sockets of -L in: tmux-UID in TMUX_TMPDIR, or in /tmp
// where TMUX_TMPDIR is unset, empty or names nothing, as tmux falls back to /tmp.
func socketDir() string {
	base := os.Getenv("TMUX_TMPDIR")
	if _, err := os.Stat(base); err != nil {
		base = "/tmp"
	}
	return filepath.Join(base, "tmux-"+strconv.Itoa(os.Getuid()))
}

// readSockets reads the socket directory (see socketDir), where no directory is no socket.
func readSockets() ([]os.DirEntry, error) {
	dir := socketDir()
	sockets, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathError.Err
		}
		return nil, fail.Runtime(fmt.Sprintf("cannot read %s: %v", dir, err))
	}
	return sockets, nil
}

// tmuxDir is the socket directory as tmux's socket paths name it, for serverless: absolute, its
// symbolic links resolved as tmux resolves them. It gives "" where tmux would not connect to a
// socket there: in a directory it refuses as unsafe, or in none, which tmux makes (decision 38.1).
func tmuxDir() string {
	dir := socketDir()
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o007 != 0 {
		return ""
	}
	if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Getuid() {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ""
	}
	if resolved, err = filepath.Abs(resolved); err != nil {
		return ""
	}
	return resolved
}

// serverless reports whether no server runs on socket cld-SUFFIX in dir, tmuxDir's, as tmux's
// client would find, without running tmux: it connects, and takes a refused connection or no socket
// for no server (decision 38.1). Anything else, a path too long for sun_path too, it leaves to
// tmux, which says what is wrong.
func serverless(ctx context.Context, dir, suffix string) bool {
	if dir == "" {
		return false
	}
	path := filepath.Join(dir, "cld-"+suffix)
	if len(path) >= len(unix.RawSockaddrUnix{}.Path) {
		return false
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err == nil {
		_ = conn.Close() //nolint:errcheck // a probe's connection, which sent nothing
		return false
	}
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.ENOENT)
}

// lingering is how join, detach and kill refuse session cld-SUFFIX where its server runs without
// it, and whether the server has outlived the session, which kill ends instead (decision 13). One
// cld did not start, or another NAME's in other letters (decision 13.6), gets no kill. The advice
// is kept apart (fail.Error's Advice). Once ctx is done, its tmux is killed.
func (t *Tmux) lingering(ctx context.Context, suffix string) (outlived bool, refused error) {
	// Spaces, not tabs, which tmux writes as "_" to a client without a UTF-8 locale. The mark
	// and the check are 1 or 0, and the path follows them.
	read := t.serverContext(ctx, suffix, "display-message", "-p",
		mark+" "+outlives(suffix)+" #{socket_path}")
	read.Stderr = nil
	out, _ := read.Output() //nolint:errcheck // a read that fails refuses, with no kill
	marked, rest, _ := strings.Cut(strings.TrimSuffix(string(out), "\n"), " ")
	check, path, _ := strings.Cut(rest, " ")
	if marked == "0" {
		return false, notClds(suffix, "; use another name")
	}
	other, found := strings.CutPrefix(filepath.Base(path), "cld-")
	if found && other != suffix && strings.EqualFold(other, suffix) {
		return false, &fail.Error{Status: 1,
			Message: fmt.Sprintf("session name '%s' clashes with session '%s': tmux's socket "+
				"directory ignores case here, so both names reach server cld-%[2]s", suffix, other),
			Advice: fmt.Sprintf(" (see tmux -L cld-%s ls)", other)}
	}
	refusal := &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' has ended, but its tmux server still runs", suffix),
		Advice:  fmt.Sprintf(" (see tmux -L cld-%s ls)", suffix)}
	if check != "1" {
		return false, refusal
	}
	refusal.Advice += "; end it with cld kill " + Options(suffix)
	return true, refusal
}

// notClds refuses server cld-SUFFIX, which cld did not start (see mark), with advice.
func notClds(suffix, advice string) error {
	return &fail.Error{Status: 1,
		Message: fmt.Sprintf("tmux server cld-%s is not one of cld's", suffix), Advice: advice}
}

// outlives is a format that tmux makes 1 on a server that has outlived session cld-SUFFIX, and 0 on
// any other: one of cld's, with sessions, none of them cld-SUFFIX, on the socket cld-SUFFIX. End's
// kill runs under it in the same command as the check (decision 13). Before tmux 3.6, && takes two
// operands and there is no !.
func outlives(suffix string) string {
	return "#{&&:" + mark + ",#{&&:#{S:1},#{&&:#{==:#{N/s:cld-" + suffix + "},0}," +
		"#{==:#{b:socket_path},cld-" + suffix + "}}}}"
}

// noServer reports whether tmux failed, saying message, because no server runs: its socket is
// stale, there is none, or the server exited as tmux asked it, as one does once its session ends.
// Any other error is cld's to report, a socket path too long for sun_path say (see MaxName).
func noServer(message string) bool {
	return strings.HasPrefix(message, "no server running on ") ||
		strings.HasPrefix(message, "error connecting to ") &&
			strings.HasSuffix(message, " (No such file or directory)") ||
		strings.HasSuffix(message, "server exited unexpectedly")
}
