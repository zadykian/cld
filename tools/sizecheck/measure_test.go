package main

import (
	"reflect"
	"testing"
)

var measureTests = []struct {
	name, src string
	want      measurement
}{
	{name: "empty", src: ""},
	{name: "lines", src: "a\n\nb\n", want: measurement{values: values{3, 0, 0}}},
	{name: "no final newline", src: "a\nb", want: measurement{values: values{2, 0, 0}}},
}

func TestMeasure(t *testing.T) {
	t.Parallel()
	for _, tt := range measureTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := measure([]byte(tt.src)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("measure(%q) = %+v, want %+v", tt.src, got, tt.want)
			}
		})
	}
}
