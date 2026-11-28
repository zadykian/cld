package claudetemp

import (
	"os"
	"path/filepath"
	"strings"
)

// holds is the paths under root that processes hold: their working directories and the files
// they have open, as /proc shows those of cld's user (decision 67.3). It reports whether it could
// read them, as it cannot where /proc does not show cld its own files.
func holds(root string) ([]string, bool) {
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		return nil, false
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	var open []string
	keep := func(link string) {
		if path, err := os.Readlink(link); err == nil &&
			strings.HasPrefix(path, root+string(filepath.Separator)) {
			open = append(open, path)
		}
	}
	for _, process := range processes {
		if strings.Trim(process.Name(), "0123456789") != "" {
			continue
		}
		base := filepath.Join("/proc", process.Name())
		keep(filepath.Join(base, "cwd"))
		fds, err := os.ReadDir(filepath.Join(base, "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			keep(filepath.Join(base, "fd", fd.Name()))
		}
	}
	return open, true
}
