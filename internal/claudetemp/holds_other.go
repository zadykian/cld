//go:build !linux

package claudetemp

// holds reports that cld cannot tell which paths processes hold. darwin has no /proc, and lsof
// reads every process's files, which would slow each list and join (decision 67.4).
func holds(string) ([]string, bool) {
	return nil, false
}
