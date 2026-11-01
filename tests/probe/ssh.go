package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/zadykian/cld/tests/internal/mac"
)

// sshValued are the letters of ssh's options that take a value (ssh(1)).
const sshValued = "BbcDEeFIiJLlmOoPpQRSWw"

// errConfig refuses -G in any form but the one cld runs.
var errConfig = errors.New("-G takes a destination and nothing else, as cld runs it")

// fakeSSH fakes ssh to a host that is the sandbox itself, logging each call to ssh.jsonl. ssh -G
// DEST prints DEST's configuration. Else it runs the remote command with bash in $HOME, SHELL set
// to that bash as sshd sets the login shell, or a login bash where there is none. Where it would
// connect, $CLD_FAKE_SSH_EXIT=N exits with status N instead, 255 as a dropped connection does.
func fakeSSH() error {
	args := os.Args[1:]
	call := mac.SSHCall{Argv: args, Term: os.Getenv("TERM")}
	if err := appendJSON(filepath.Join(os.Getenv("CLD_PROBE_DIR"), mac.SSHLog), call); err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "-G" {
		return printConfig(args[1:])
	}
	command, err := readSSH(args)
	if err != nil {
		unusable(err)
	}
	if code, set := os.LookupEnv("CLD_FAKE_SSH_EXIT"); set {
		status, err := strconv.Atoi(code)
		if err != nil {
			return fmt.Errorf("CLD_FAKE_SSH_EXIT: %w", err)
		}
		os.Exit(status)
	}
	return remote(command)
}

// unusable fails as ssh does on words it cannot use, or a connection it cannot make: status 255.
func unusable(err error) {
	fmt.Fprintln(os.Stderr, "fake ssh:", err)
	os.Exit(255)
}

// printConfig prints what ssh -G prints of the destination, the one word in args. The user is
// user@host's, else $USER or me. The hostname is the host in lower case, as ssh writes it, and
// the port 22. A line HOST HOSTNAME PORT of ssh-hosts gives others, as a Host block would.
func printConfig(args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		unusable(errConfig)
	}
	user, host := cmp.Or(os.Getenv("USER"), "me"), args[0]
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		user, host = host[:at], host[at+1:]
	}
	hostname, port := strings.ToLower(host), "22"
	data, err := os.ReadFile(filepath.Join(os.Getenv("CLD_PROBE_DIR"), mac.SSHHosts))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for line := range strings.Lines(string(data)) {
		if fields := strings.Fields(line); len(fields) == 3 && fields[0] == host {
			hostname, port = fields[1], fields[2]
		}
	}
	_, err = fmt.Printf("user %s\nhostname %s\nport %s\n", user, hostname, port)
	return err
}

// readSSH is the remote command in ssh's words: the words after the destination, the first word
// that is no option, past the options' values. "--" ends the options.
func readSSH(args []string) ([]string, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || len(arg) < 2 || arg[0] != '-' {
			if arg == "--" {
				i++
			}
			if i == len(args) {
				break
			}
			return args[i+1:], nil
		}
		var err error
		if i, err = skipOption(args, i); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("no destination")
}

// skipOption reads the word args[i] letter by letter. A letter of sshValued takes the rest of the
// word as its value, or the next word where the word ends with it, and ends the word. It returns
// the index of the last word read.
func skipOption(args []string, i int) (int, error) {
	word := args[i]
	for j := 1; j < len(word); j++ {
		switch letter := word[j]; {
		case letter == 'G':
			return i, errConfig
		case !strings.ContainsRune(sshValued, rune(letter)):
			continue
		case j+1 == len(word) && i+1 == len(args):
			return i, fmt.Errorf("option requires an argument -- %c", letter)
		case j+1 == len(word):
			return i + 1, nil
		}
		return i, nil
	}
	return i, nil
}

// remote runs command, its words joined by spaces as sshd hands them to the login shell, with
// bash in $HOME; with no command, bash as a login shell. It refuses a HOME with no .profile.
func remote(command []string) error {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return err
	}
	home := os.Getenv("HOME")
	if _, err := os.Stat(filepath.Join(home, ".profile")); err != nil {
		unusable(errors.New("HOME has no .profile to put the tools under test first on the PATH"))
	}
	if err := os.Chdir(home); err != nil {
		return err
	}
	env := []string{"SHELL=" + bash}
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "SHELL=") {
			env = append(env, variable)
		}
	}
	// -l too: a bash reading a pipe is not interactive, and macOS's reads no profile for the "-"
	// of argv[0] alone, where Debian's, built with NON_INTERACTIVE_LOGIN_SHELLS, does.
	argv := []string{"-bash", "-l"}
	if len(command) > 0 {
		argv = []string{"bash", "-c", strings.Join(command, " ")}
	}
	return syscall.Exec(bash, argv, env)
}
