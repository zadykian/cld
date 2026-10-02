package main

import (
	"reflect"
	"strings"
	"testing"
)

var parseBaselineTests = []struct {
	name, data string
	want       baseline
	wantErr    string // a part of the error, "" for none
}{
	{name: "empty", data: "", want: baseline{}},
	{name: "header only", data: baselineHeader, want: baseline{}},
	{
		name: "entries",
		data: "# a comment\n\na.go lines=301 comments=2\n  b.md\tlines=400  \nc.go package=16\n",
		want: baseline{"a.go": {301, 2, 0}, "b.md": {400, 0, 0}, "c.go": {0, 0, 16}},
	},
	{
		name: "keys in any order",
		data: "a.go package=20 lines=310\n",
		want: baseline{"a.go": {310, 0, 20}},
	},
	{
		name: "a quoted path",
		data: `"a b.md" lines=301` + "\n" + `"#c\".md" lines=302` + "\n",
		want: baseline{"a b.md": {301, 0, 0}, `#c".md`: {302, 0, 0}},
	},
	{name: "no ceiling", data: "a.go\n", wantErr: "1: a.go has no ceiling"},
	{name: "no number", data: "a.go lines\n", wantErr: `1: "lines" is not key=N: invalid`},
	{name: "a bad number", data: "a.go lines=x\n", wantErr: `1: "lines=x" is not key=N`},
	{name: "an unknown key", data: "a.go size=400\n", wantErr: `1: "size=400" is not key=N`},
	{name: "lines within the cap", data: "a.go lines=300\n", wantErr: "within the cap of 300"},
	{name: "comments within the cap", data: "a.go comments=0\n", wantErr: "within the cap of 0"},
	{name: "package within the cap", data: "a.go package=15\n", wantErr: "within the cap"},
	{name: "a key twice", data: "a.go lines=301 lines=302\n", wantErr: "1: lines is given twice"},
	{
		name: "a path twice", data: "a.go lines=301\n#\na.go comments=1\n",
		wantErr: "3: a.go is listed twice",
	},
	{name: "a bad quote", data: `"a.go lines=301` + "\n", wantErr: "1: bad quoted path"},
}

func TestParseBaseline(t *testing.T) {
	t.Parallel()
	for _, tt := range parseBaselineTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBaseline([]byte(tt.data))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("parseBaseline: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("parseBaseline: error %v, want one with %q", err, tt.wantErr)
			case tt.wantErr == "" && !reflect.DeepEqual(got, tt.want):
				t.Errorf("parseBaseline = %v, want %v", got, tt.want)
			}
		})
	}
}

var formatBaselineTests = []struct {
	name string
	base baseline
	want string // what follows the header
}{
	{name: "empty", base: baseline{}, want: ""},
	{
		name: "sorted, with keys in a fixed order",
		base: baseline{"b.md": {400, 0, 0}, "a.go": {310, 3, 20}, "a/b.go": {0, 2, 0}},
		want: "a.go lines=310 comments=3 package=20\na/b.go comments=2\nb.md lines=400\n",
	},
	{
		name: "an entry without ceilings is left out",
		base: baseline{"a.go": {}, "b.go": {301, 0, 0}},
		want: "b.go lines=301\n",
	},
	{
		name: "paths quoted where needed",
		base: baseline{"a b.md": {301, 0, 0}, "#a.md": {302, 0, 0}, `a"b.md`: {303, 0, 0}},
		want: `"#a.md" lines=302` + "\n" + `"a b.md" lines=301` + "\n" + `"a\"b.md" lines=303` + "\n",
	},
}

func TestFormatBaseline(t *testing.T) {
	t.Parallel()
	for _, tt := range formatBaselineTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(formatBaseline(tt.base))
			if got != baselineHeader+tt.want {
				t.Fatalf("formatBaseline = %q, want the header and %q", got, tt.want)
			}
			parsed, err := parseBaseline([]byte(got))
			if err != nil {
				t.Fatalf("parseBaseline of formatBaseline's output: %v", err)
			}
			for path, ceilings := range tt.base {
				if ceilings != parsed[path] {
					t.Errorf("%s reads back as %v, want %v", path, parsed[path], ceilings)
				}
			}
		})
	}
}
