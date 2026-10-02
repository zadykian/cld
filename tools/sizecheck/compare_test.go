package main

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// over is a Go file of 310 lines with two comment blocks over the cap and a package comment of 20.
var over = measurement{
	values: values{310, 2, 20},
	blocks: []block{{30, 5}, {40, 6}},
	pkg:    block{1, 20},
}

// short is a file within every cap.
var short = measurement{values: values{100, 0, 3}, pkg: block{1, 3}}

var compareTests = []struct {
	name   string
	base   baseline
	files  map[string]measurement
	scope  []string // the FILE arguments, nil for all
	create bool
	want   []string // the findings, stale ones with " (stale)" after them
	next   baseline
}{
	{name: "within the caps", files: map[string]measurement{"a.go": short}, next: baseline{}},
	{
		name:  "over the caps without a ceiling",
		files: map[string]measurement{"a.go": over},
		want: []string{
			"a.go:1: 310 lines, over the cap of 300",
			"a.go:1: package comment of 20 lines, over the cap of 15",
			"a.go:30: comment block of 5 lines, over the cap of 4",
			"a.go:40: comment block of 6 lines, over the cap of 4",
		},
		next: baseline{},
	},
	{
		name:   "create adds the overages",
		files:  map[string]measurement{"a.go": over},
		create: true,
		next:   baseline{"a.go": {310, 2, 20}},
	},
	{
		name:  "at or below the ceilings",
		base:  baseline{"a.go": {320, 2, 25}},
		files: map[string]measurement{"a.go": over},
		next:  baseline{"a.go": {310, 2, 20}},
	},
	{
		name:  "above the ceilings",
		base:  baseline{"a.go": {309, 1, 19}},
		files: map[string]measurement{"a.go": over},
		want: []string{
			"a.go:1: 310 lines, above the baseline's ceiling of 309",
			"a.go:1: 2 comment blocks over 4 lines, above the baseline's ceiling of 1",
			"a.go:1: package comment of 20 lines, above the baseline's ceiling of 19",
			"a.go:30: comment block of 5 lines, over the cap of 4",
			"a.go:40: comment block of 6 lines, over the cap of 4",
		},
		next: baseline{"a.go": {309, 1, 19}},
	},
	{
		name:  "stale ceilings",
		base:  baseline{"a.go": {400, 1, 16}},
		files: map[string]measurement{"a.go": short},
		want: []string{
			"a.go:1: stale baseline entry lines=400: now 100, within the cap of 300 (stale)",
			"a.go:1: stale baseline entry comments=1: now 0, within the cap of 0 (stale)",
			"a.go:1: stale baseline entry package=16: now 3, within the cap of 15 (stale)",
		},
		next: baseline{},
	},
	{
		name:  "a stale ceiling beside one that holds",
		base:  baseline{"a.go": {400, 2, 0}},
		files: map[string]measurement{"a.go": {values: values{250, 2, 0}, blocks: over.blocks}},
		want: []string{
			"a.go:1: stale baseline entry lines=400: now 250, within the cap of 300 (stale)",
		},
		next: baseline{"a.go": {0, 2, 0}},
	},
	{
		name: "a file gone",
		base: baseline{"b.md": {400, 0, 0}},
		want: []string{"b.md:1: stale baseline entry: the file is gone or git ignores it (stale)"},
		next: baseline{},
	},
	{
		name:  "entries out of scope are kept",
		base:  baseline{"b.md": {400, 0, 0}, "c.md": {500, 0, 0}},
		files: map[string]measurement{"a.go": short},
		scope: []string{"a.go"},
		next:  baseline{"b.md": {400, 0, 0}, "c.md": {500, 0, 0}},
	},
	{
		name:  "a parse error keeps the entry",
		base:  baseline{"a.go": {0, 3, 0}},
		files: map[string]measurement{"a.go": {values: values{9, 0, 0}, errLine: 5, errMsg: "bad"}},
		want:  []string{"a.go:5: cannot parse: bad"},
		next:  baseline{"a.go": {0, 3, 0}},
	},
	{
		name: "sorted by path, then line",
		base: baseline{"c.md": {400, 0, 0}},
		files: map[string]measurement{
			"d.md": {values: values{301, 0, 0}},
			"b.go": {values: values{10, 1, 0}, blocks: []block{{2, 5}}},
			"a.go": {values: values{10, 0, 16}, pkg: block{3, 16}},
		},
		want: []string{
			"a.go:3: package comment of 16 lines, over the cap of 15",
			"b.go:2: comment block of 5 lines, over the cap of 4",
			"c.md:1: stale baseline entry: the file is gone or git ignores it (stale)",
			"d.md:1: 301 lines, over the cap of 300",
		},
		next: baseline{},
	},
}

func TestCompare(t *testing.T) {
	t.Parallel()
	for _, tt := range compareTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inScope := func(string) bool { return true }
			if tt.scope != nil {
				inScope = func(path string) bool { return slices.Contains(tt.scope, path) }
			}
			findings, next := compare(tt.base, tt.files, inScope, tt.create)
			var got []string
			for _, f := range findings {
				line := fmt.Sprintf("%s:%d: %s", f.path, f.line, f.msg)
				if f.stale {
					line += " (stale)"
				}
				got = append(got, line)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findings:\n%q\nwant\n%q", got, tt.want)
			}
			if !reflect.DeepEqual(next, tt.next) {
				t.Errorf("next baseline %v, want %v", next, tt.next)
			}
		})
	}
}

func TestRefused(t *testing.T) {
	t.Parallel()
	findings := []finding{{path: "a", stale: true}, {path: "b"}, {path: "c", stale: true}}
	if !hasStale(findings) {
		t.Error("hasStale = false, want true")
	}
	if got := refused(findings); !reflect.DeepEqual(got, []finding{{path: "b"}}) {
		t.Errorf("refused = %v, want b alone", got)
	}
	if hasStale(refused(findings)) {
		t.Error("hasStale of the refused findings = true, want false")
	}
}
