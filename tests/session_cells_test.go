package tests

import (
	"regexp"
	"slices"
	"strings"
)

// sgr matches an SGR sequence, with its parameters.
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// attribute names the attribute each SGR code sets or clears.
var attribute = map[string]string{
	"1": "intensity", "2": "intensity", "22": "intensity",
	"3": "italic", "23": "italic",
	"4": "underline", "21": "underline", "24": "underline",
	"5": "blink", "6": "blink", "25": "blink",
	"7": "inverse", "27": "inverse",
	"8": "hidden", "28": "hidden",
	"9": "strike", "29": "strike",
	"53": "overline", "55": "overline",
	"38": "fg", "39": "fg", "48": "bg", "49": "bg", "58": "underline-colour", "59": "underline-colour",
}

// clearing lists the SGR codes that turn their attribute off.
var clearing = map[string]bool{
	"22": true, "23": true, "24": true, "4:0": true, "25": true, "27": true, "28": true,
	"29": true, "55": true, "39": true, "49": true, "59": true,
}

// cells turns a Styled screen into lines of runs, "[attribute=code ...]text", whatever the
// SGR sequences that happened to draw them: equal cells give equal lines. Trailing blank cells
// and lines are dropped, so that screens of different sizes compare by what is drawn on them.
func cells(styled string) []string {
	state := map[string]string{} // capture-pane -e carries attributes over line ends
	var lines []string
	for row := range strings.SplitSeq(styled, "\n") {
		lines = append(lines, cellRow(state, row))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// cellRow is a row of a Styled screen as cells writes it, drawn with the attributes of state,
// which the row's SGR sequences update.
func cellRow(state map[string]string, row string) string {
	var attrs, texts []string // one per run
	matches := sgr.FindAllStringSubmatch(row, -1)
	for i, text := range sgr.Split(row, -1) {
		if i > 0 {
			apply(state, matches[i-1][1])
		}
		if text == "" {
			continue
		}
		if n := len(attrs); n > 0 && attrs[n-1] == describe(state) {
			texts[n-1] += text
		} else {
			attrs, texts = append(attrs, describe(state)), append(texts, text)
		}
	}
	if n := len(attrs); n > 0 && attrs[n-1] == "" {
		if texts[n-1] = strings.TrimRight(texts[n-1], " "); texts[n-1] == "" {
			attrs, texts = attrs[:n-1], texts[:n-1]
		}
	}
	var line strings.Builder
	for i := range attrs {
		line.WriteString("[" + attrs[i] + "]" + texts[i])
	}
	return line.String()
}

// apply updates the attributes of the cells after an SGR sequence with parameters params.
func apply(state map[string]string, params string) {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		if code == "" || code == "0" {
			clear(state)
			continue
		}
		base, _, _ := strings.Cut(code, ":")
		// A colour in the ; form takes the codes after it: 5;N or 2;R;G;B.
		if (base == "38" || base == "48" || base == "58") && base == code {
			n := 4
			if i+1 < len(codes) && codes[i+1] == "5" {
				n = 2
			}
			code = strings.Join(codes[i:min(i+n+1, len(codes))], ";")
			i += n
		}
		name, known := attribute[base]
		switch {
		case !known:
		case clearing[code]:
			delete(state, name)
		default:
			state[name] = code
		}
	}
}

// describe is the attributes of state as cells writes them, sorted.
func describe(state map[string]string) string {
	var pairs []string
	for name, code := range state {
		pairs = append(pairs, name+"="+code)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// underlined reports whether any cell of a screen from cells is underlined.
func underlined(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, "underline=")
	})
}
