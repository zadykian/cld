package main

import (
	"reflect"
	"strings"
	"testing"
)

// goSource is a Go file of a package comment, a blank line, body and a declaration.
func goSource(doc, body string) string {
	return doc + "package a\n\n" + body + "var x int\n"
}

// comment returns a // comment of n lines.
func comment(n int) string {
	return strings.Repeat("// x\n", n)
}

var measureTests = []struct {
	name, path, src string
	want            measurement
	wantErrLine     int // the line of a parse error, 0 for none
}{
	{name: "empty", path: "a.md", src: ""},
	{name: "lines", path: "a.md", src: "a\n\nb\n", want: measurement{values: values{3, 0, 0}}},
	{name: "no final newline", path: "a.md", src: "a\nb", want: measurement{values: values{2, 0, 0}}},
	{
		name: "markdown has no comment blocks", path: "a.md", src: comment(6),
		want: measurement{values: values{6, 0, 0}},
	},
	{
		name: "a block at the cap", path: "a.go", src: goSource("", comment(4)),
		want: measurement{values: values{7, 0, 0}},
	},
	{
		name: "a block over the cap", path: "a.go", src: goSource("", comment(5)),
		want: measurement{values: values{8, 1, 0}, blocks: []block{{3, 5}}},
	},
	{
		name: "a /* */ block over the cap", path: "a.go", src: goSource("", "/*\n1\n2\n3\n*/\n"),
		want: measurement{values: values{8, 1, 0}, blocks: []block{{3, 5}}},
	},
	{
		name: "a blank line splits blocks", path: "a.go",
		src:  goSource("", comment(3)+"\n"+comment(3)),
		want: measurement{values: values{10, 0, 0}},
	},
	{
		name: "a trailing comment is a block of its own", path: "a.go",
		src:  goSource("", "var y int // y\n"+comment(4)),
		want: measurement{values: values{8, 0, 0}},
	},
	{
		name: "blocks inside a function", path: "a.go",
		src:  goSource("", "func f() {\n"+comment(6)+"}\n"),
		want: measurement{values: values{11, 1, 0}, blocks: []block{{4, 6}}},
	},
	{
		name: "the package comment at its cap", path: "a.go", src: goSource(comment(15), ""),
		want: measurement{values: values{18, 0, 15}, pkg: block{1, 15}},
	},
	{
		name: "the package comment over its cap", path: "a.go",
		src: goSource("//go:build linux\n\n"+comment(16), comment(5)),
		want: measurement{
			values: values{26, 1, 16}, pkg: block{3, 16}, blocks: []block{{21, 5}},
		},
	},
	{
		name: "a comment above the package clause, apart from it", path: "a.go",
		src:  comment(5) + "\n" + goSource("", ""),
		want: measurement{values: values{9, 1, 0}, blocks: []block{{1, 5}}},
	},
	{
		name: "a parse error", path: "a.go", src: "package a\n\nfunc {\n",
		want: measurement{values: values{3, 0, 0}}, wantErrLine: 3,
	},
}

func TestMeasure(t *testing.T) {
	t.Parallel()
	for _, tt := range measureTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := measure(tt.path, []byte(tt.src))
			if (got.errMsg != "") != (tt.wantErrLine != 0) || got.errLine != tt.wantErrLine {
				t.Errorf("parse error at %d (%q), want one at %d", got.errLine, got.errMsg,
					tt.wantErrLine)
			}
			got.errLine, got.errMsg = 0, ""
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("measure(%q) = %+v, want %+v", tt.src, got, tt.want)
			}
		})
	}
}
