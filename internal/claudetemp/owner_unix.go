package claudetemp

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByUser reports whether the user running cld owns the file that info describes.
func ownedByUser(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}
