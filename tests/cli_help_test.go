package tests

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The help's text, as testdata/help has it, and the version.

var update = flag.Bool("update", false,
	"rewrite the help in testdata/help from what cld help prints")

// helpTopics are what cld help takes, in the order the help lists them, "" for none. Each command
// of cld's comes before its own, as "setup project", and theirs, as "setup completion zsh".
var helpTopics = []string{
	"", "join", "detach", "kill", "list", "restore",
	"setup", "setup project", "setup completion",
	"setup completion bash", "setup completion zsh", "setup completion fish", "setup restore",
	"update", "completion", "help", "version",
}

// goldenHelp is the file in testdata/help holding what cld help topic prints: cld.txt for no
// topic, and otherwise the topic with "-" for each space.
func goldenHelp(topic string) string {
	if topic == "" {
		topic = "cld"
	}
	return filepath.Join("testdata", "help", strings.ReplaceAll(topic, " ", "-")+".txt")
}

// subtopics are the commands the help of topic lists: the topics that name one after it.
func subtopics(topic string) []string {
	var names []string
	for _, other := range helpTopics[1:] {
		parent, name := "", other
		if last := strings.LastIndex(other, " "); last >= 0 {
			parent, name = other[:last], other[last+1:]
		}
		if parent == topic {
			names = append(names, name)
		}
	}
	return names
}

// The help of cld and of each command, byte for byte as testdata/help has it, which -update
// rewrites: cobra generates it from each command's texts and options. Lines stay within 80
// columns, and usage lines name no option after an argument (decisions 12.1 and 12.4).
func TestHelpText(t *testing.T) {
	t.Parallel()
	for _, topic := range helpTopics {
		args := append([]string{"help"}, strings.Fields(topic)...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(nil, args...)
			if result.Code != 0 || result.Stderr != "" {
				t.Fatalf("exit %d, stderr %q", result.Code, result.Stderr)
			}
			checkGoldenHelp(t, topic, result.Stdout)
			checkHelpWidth(t, result.Stdout)
			if late := optionsAfterArguments(result.Stdout); len(late) > 0 {
				t.Errorf("the usage line names %q after an argument, where cld reads no options",
					late)
			}
			// completion's commands are cobra's, whose help is cobra's too (see
			// TestCompletionScripts).
			listed, want := listedCommands(result.Stdout), subtopics(topic)
			if topic != "completion" && !slices.Equal(listed, want) {
				t.Errorf("the help lists %q, want %q: one file in testdata/help for each",
					listed, want)
			}
		})
	}
}

// checkGoldenHelp reports a help other than goldenHelp's file of topic, which -update rewrites
// first.
func checkGoldenHelp(t *testing.T, topic, help string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(goldenHelp(topic), []byte(help), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenHelp(topic))
	if err != nil {
		t.Fatal(err)
	}
	if help != string(want) {
		t.Errorf("stdout\n%s\nwant, as in %s\n%s", help, goldenHelp(topic), want)
	}
}

// checkHelpWidth reports each line of help wider than 80 columns, but for cobra's last line,
// which names the command. That line is cobra's template, 81 columns for setup completion
// (decision 22.6).
func checkHelpWidth(t *testing.T, help string) {
	t.Helper()
	const cobraLine = ` [command] --help" for more information about a command.`
	for number, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, `Use "cld `) && strings.HasSuffix(line, cobraLine) {
			continue
		}
		if width := utf8.RuneCountInString(line); width > 80 {
			t.Errorf("line %d is %d columns wide: %q", number+1, width, line)
		}
	}
}

// listedCommands are the commands the help of cld lists, in its order.
func listedCommands(help string) []string {
	_, listing, _ := strings.Cut(help, "\nAvailable Commands:\n")
	listing, _, _ = strings.Cut(listing, "\n\n")
	var names []string
	for line := range strings.SplitSeq(listing, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// usageWord is a word of a usage line: one in brackets, such as [-n NAME], or a plain one.
var usageWord = regexp.MustCompile(`\[[^]]*\]|\S+`)

// optionsAfterArguments are the options the usage lines of help name after an argument. An option
// is [flags] or a word starting with "-" or "[-"; a plain one takes the next word as its value.
// An argument is another word in brackets or capitals, or what follows "--", such as [-- ARGS...].
func optionsAfterArguments(help string) []string {
	_, usage, _ := strings.Cut(help, "\nUsage:\n")
	usage, _, _ = strings.Cut(usage, "\n\n")
	var late []string
	for line := range strings.SplitSeq(usage, "\n") {
		argument, value := false, false
		for _, word := range usageWord.FindAllString(line, -1) {
			switch {
			case value:
				value = false
			case word == "--" || strings.HasPrefix(word, "[-- "):
				argument = true
			case word == "[flags]" || strings.HasPrefix(word, "[-") || strings.HasPrefix(word, "-"):
				if argument {
					late = append(late, word)
				}
				value = strings.HasPrefix(word, "-")
			case strings.HasPrefix(word, "[") || strings.ToUpper(word) == word:
				argument = true
			}
		}
	}
	return late
}

func TestVersion(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, command := range []string{"version", "-V", "--version"} {
		if result := s.RunCld(nil, command); result.Code != 0 || result.Stdout != "cld dev\n" {
			t.Errorf("cld %s: exit %d, stdout %q", command, result.Code, result.Stdout)
		}
	}
}
