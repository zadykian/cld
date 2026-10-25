package main

import "testing"

var measureTests = []struct {
	name, src string
	want      int
}{
	{name: "empty", src: "", want: 0},
	{name: "lines", src: "a\n\nb\n", want: 3},
	{name: "no final newline", src: "a\nb", want: 2},
}

func TestMeasure(t *testing.T) {
	t.Parallel()
	for _, tt := range measureTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := measure([]byte(tt.src)); got != tt.want {
				t.Errorf("measure(%q) = %d, want %d", tt.src, got, tt.want)
			}
		})
	}
}
