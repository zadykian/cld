package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/project"
)

// userLong is setup config user's help.
const userLong = `set up claude's user settings, your own, $CLAUDE_CONFIG_DIR/settings.json, by
default ~/.claude/settings.json, which claude reads in every project: what
claude may do without asking, which --permissions sets, and model opus,
effortLevel xhigh, theme dark, editorMode normal, autoCompactEnabled true and
autoUpdatesChannel latest. With --notifications, cld sets the channel claude
notifies on: under tmux, as in cld's sessions, claude's default sends nothing.

Where the file exists, cld adds what it lacks and keeps everything else, the
values of the settings it would set included. It leaves env alone, which holds
claude's telemetry settings among others.`

// setupUser is cld setup config user (decision 53), named in its messages as typed.
func setupUser(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "user [--permissions SET] [--notifications CHANNEL]",
		Short: "set claude up in your own settings, for every project",
		Long:  userLong,
		Args:  cmdline.NoArguments(typed),
	}
	command.SetFlagErrorFunc(cmdline.FlagError(typed))
	flags := command.Flags()
	permissions := flags.String("permissions", "",
		"what claude may do without asking, in every\n"+
			"project: `SET` is read-only, the default, to read\n"+
			"files and run commands that only read, such as\n"+
			"git status and ls; cld, as cld's own repository\n"+
			"has it, to edit files, run git, go, make, docker\n"+
			"and more, but not merge or push to main; or\n"+
			"none, to add nothing")
	notifications := flags.String("notifications", "",
		"the `CHANNEL` claude notifies on: auto, claude's\n"+
			"default; iterm2 or iterm2_with_bell for iTerm2;\n"+
			"kitty for kitty; ghostty for Ghostty;\n"+
			"terminal_bell, the bell, for any other terminal;\n"+
			"or notifications_disabled. Without it, cld sets\n"+
			"no channel")
	command.RunE = func(c *cobra.Command, _ []string) error {
		set, err := permissionSet(c.Flags().Changed("permissions"), *permissions)
		if err != nil {
			return err
		}
		channel, err := notificationChannel(c.Flags().Changed("notifications"), *notifications)
		if err != nil {
			return err
		}
		return project.SetupUser(set, channel)
	}
	cmdline.CompleteWith(command, "permissions", permissionSets)
	cmdline.CompleteWith(command, "notifications", channelNames)
	return command
}

// notificationChannel is the channel --notifications names, where given, and else none, "".
func notificationChannel(given bool, name string) (string, error) {
	known := func(c project.Channel) bool { return c.Name == name }
	if given && !slices.ContainsFunc(project.Channels, known) {
		var names []string
		for _, c := range project.Channels {
			names = append(names, c.Name)
		}
		return "", fail.Usage(fmt.Sprintf("invalid channel '%s' for --notifications: %s "+
			"(see cld help)", name, or(names)))
	}
	return name, nil
}

// or joins names as a sentence does: "a", "a or b", "a, b or c".
func or(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// channelNames completes the CHANNEL of setup config user --notifications: the channels it takes
// that start with what was typed, each described.
func channelNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion,
	cobra.ShellCompDirective) {
	var names []cobra.Completion
	for _, channel := range project.Channels {
		if strings.HasPrefix(channel.Name, typed) {
			names = append(names, cobra.CompletionWithDesc(channel.Name, channel.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
