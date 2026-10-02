package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// hookGroup is an entry of an event's hooks in claude's settings.
type hookGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookCommand `json:"hooks"`
}

// hookCommand is a hook of a hookGroup.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// runHooks runs the hooks for event in the settings given with --settings, as claude runs them,
// with input (default {}) as their input. A hook that fails or prints anything is an error.
func runHooks(event, input string) error {
	groups, err := settingsHooks(event)
	if err != nil {
		return err
	}
	if input == "" {
		input = "{}"
	}
	var fields struct {
		NotificationType string `json:"notification_type"`
		ToolName         string `json:"tool_name"`
	}
	if err := json.Unmarshal([]byte(input), &fields); err != nil {
		return err
	}
	for _, group := range groups {
		matched, err := group.matches(fields.NotificationType + fields.ToolName)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		for _, hook := range group.Hooks {
			if err := hook.run(input); err != nil {
				return err
			}
		}
	}
	return nil
}

// settingsHooks is the hooks for event in the settings the probe was given with --settings.
func settingsHooks(event string) ([]hookGroup, error) {
	i := slices.Index(os.Args, "--settings")
	if i < 0 || i+1 == len(os.Args) {
		return nil, errors.New("no --settings")
	}
	var settings struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(os.Args[i+1]), &settings); err != nil {
		return nil, err
	}
	return settings.Hooks[event], nil
}

// matches reports whether the group's matcher, where it has one, matches the whole of value: the
// event's notification_type, or tool_name.
func (group hookGroup) matches(value string) (bool, error) {
	if group.Matcher == "" {
		return true, nil
	}
	return regexp.MatchString("^(?:"+group.Matcher+")$", value)
}

// run runs the hook's command through sh -c, in the probe's directory and environment, with input
// as its input. It waits for the command, even one claude runs in the background (async), with
// no timeout. A command that fails or prints anything is an error.
func (hook hookCommand) run(input string) error {
	if hook.Type != "command" {
		return fmt.Errorf("a hook of type %q", hook.Type)
	}
	cmd := exec.Command("/bin/sh", "-c", hook.Command)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) != 0 {
		//nolint:errorlint // err may be nil, which %w would print as %!w(<nil>)
		return fmt.Errorf("%s: %v, printed %q", hook.Command, err, out)
	}
	return nil
}
