package cmdline

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
)

// Directory gives command -C and --directory, the directory it runs as if in, read into dir and
// described by usage (decision 61).
func Directory(command *cobra.Command, dir *string, usage string) {
	command.Flags().StringVarP(dir, "directory", "C", "", usage)
}

// Path is the directory -C's dir names, as POSIX's cd takes it: a relative dir from the directory
// PWD names, ".." taken from the name (decision 61.1). It also returns the current directory, ""
// where Getwd fails. Only a relative dir needs that, and fails without it.
func Path(dir string) (target, from string, err error) {
	from, err = os.Getwd()
	switch {
	case filepath.IsAbs(dir):
		return filepath.Clean(dir), from, nil
	case errors.Is(err, fs.ErrNotExist):
		return "", "", fail.Runtime("the current directory no longer exists")
	case err != nil:
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathError.Err
		}
		return "", "", fail.Runtime("cannot get the current directory: " + err.Error())
	}
	return filepath.Join(from, dir), from, nil
}

// Enter enters dir, -C's, where command was given it, PWD naming it (see Path). It returns the
// directory left, "" where -C is not given or Getwd fails. An empty dir is refused with status 2,
// and one it cannot enter with 1 (decision 61.2).
func Enter(command *cobra.Command, dir string) (string, error) {
	if !command.Flags().Changed("directory") {
		return "", nil
	}
	if dir == "" {
		return "", fail.Usage("option '-C' needs a value (see cld help)")
	}
	target, from, err := Path(dir)
	if err != nil {
		return "", err
	}
	if err := os.Chdir(target); err != nil {
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathError.Err
		}
		return "", fail.Runtime("cannot enter " + dir + ": " + err.Error())
	}
	if err := os.Setenv("PWD", target); err != nil {
		return "", fail.Runtime(err.Error())
	}
	return from, nil
}
