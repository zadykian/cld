package session

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The oldest tmux and claude cld runs, each raised by hand: decision 6 says why these releases.
var (
	minTmux   = version{3, 5, 1}
	minClaude = version{2, 1, 232}
)

// tmuxVersion and claudeVersion match the start of a version that tmux -V and claude --version
// report: "3.7c", "2.1.282 (Claude Code)". tmux's letter, which marks a bug-fix release, is its
// third number (see parseVersion): 3.5a is 3.5.1, and 3.5 is 3.5.0.
var (
	tmuxVersion   = regexp.MustCompile(`^([0-9]+)\.([0-9]+)([a-z]?)`)
	claudeVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
)

// version is the numbers of a version, the most significant first.
type version []int

// parseVersion reads a version from the start of text, as pattern matches it; false when text
// does not start with one. A letter counts as its place in the alphabet, and a group that matched
// nothing as 0.
func parseVersion(pattern *regexp.Regexp, text string) (version, bool) {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return nil, false
	}
	var v version
	for _, part := range match[1:] {
		switch {
		case part == "":
			v = append(v, 0)
		case part[0] >= 'a':
			v = append(v, int(part[0]-'a')+1)
		default:
			v = append(v, number(part))
		}
	}
	return v, true
}

// before reports whether v is older than oldest, comparing them number by number.
func (v version) before(oldest version) bool { return slices.Compare(v, oldest) < 0 }

func (v version) String() string {
	numbers := make([]string, len(v))
	for i, n := range v {
		numbers[i] = strconv.Itoa(n)
	}
	return strings.Join(numbers, ".")
}

// tmuxRelease names v as tmux names its releases, the third number as a letter: 3.5.1 is 3.5a.
func tmuxRelease(v version) string {
	name := fmt.Sprintf("%d.%d", v[0], v[1])
	if v[2] > 0 {
		name += string(rune('a' + v[2] - 1))
	}
	return name
}

// number is a run of digits as a number, one too large for an int as the largest.
func number(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return math.MaxInt
	}
	return n
}
