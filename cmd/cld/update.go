package main

import (
	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/update"
)

// updateLong is update's help.
const updateLong = `update cld to the latest release on GitHub: download the release's cld for
this system, check it against the release's cld.sha256 and that it runs, then
replace the file cld runs from with it - the file a symbolic link leads to.
Where cld is the latest release already, or newer, nothing changes; a cld built
from source, cld dev, is not updated. Once cld is replaced, the scripts that
cld setup completion wrote are written anew where the new cld prints others.`

// newUpdate is cld update (decision 21), which runs neither tmux nor claude, so makes none of
// their checks (decision 21.5).
func newUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "update cld to the latest release",
		Long:  updateLong,
		RunE: func(*cobra.Command, []string) error {
			result, err := update.Run(version)
			if err != nil {
				return err
			}
			if err := output.Print(result.Report()); err != nil || result.File == "" {
				return err
			}
			// The new cld prints the scripts anew; a script not written is a warning
			// (decision 22.5).
			report, warnings := completion.Refresh(result.File)
			for _, warning := range warnings {
				output.Warn(warning)
			}
			if report == "" {
				return nil
			}
			return output.Print(report)
		},
	}
}

// newVersion is cld version. cobra's help lists commands only, so its Long names its other
// spellings.
func newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "show the version",
		Long:  "show the version; cld -V and cld --version show it too",
		RunE: func(*cobra.Command, []string) error {
			return output.Print("cld " + version + "\n")
		},
	}
}
