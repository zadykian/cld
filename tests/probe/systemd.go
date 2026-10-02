package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fakeSystemd fakes the systemctl and loginctl calls of cld setup restore, changing nothing. It
// appends each call, the program's name first, to systemd.jsonl, a line of JSON per call.
// $CLD_FAKE_SYSTEMD_FAIL=WORD makes a call with the argument WORD fail instead, saying so.
func fakeSystemd() error {
	call := append([]string{filepath.Base(os.Args[0])}, os.Args[1:]...)
	calls := filepath.Join(os.Getenv("CLD_PROBE_DIR"), "systemd.jsonl")
	if err := appendJSON(calls, call); err != nil {
		return err
	}
	if fail := os.Getenv("CLD_FAKE_SYSTEMD_FAIL"); fail != "" && slices.Contains(call, fail) {
		fmt.Fprintf(os.Stderr, "fake %s failed\n", strings.Join(call, " "))
		os.Exit(1)
	}
	// As for --property=Linger --value; any other call prints nothing.
	if call[0] == "loginctl" && slices.Contains(call, "show-user") {
		linger, set := os.LookupEnv("CLD_FAKE_LINGER")
		if !set {
			linger = "no"
		}
		fmt.Println(linger)
	}
	return nil
}
