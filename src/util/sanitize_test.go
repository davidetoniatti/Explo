package util

import "testing"

func TestFilenameSafe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "letters and digits are untouched", input: "Song123", want: "Song123"},
		{name: "allowed punctuation is untouched", input: "song.name,ok-1", want: "song.name,ok-1"},
		{name: "spaces are replaced", input: "My Song", want: "My_Song"},
		{name: "path separators are replaced", input: "a/b\\c", want: "a_b_c"},
		{name: "consecutive illegal characters collapse to one underscore (edge case)", input: "a://b", want: "a_b"},
		{name: "unicode letters are preserved", input: "Café Über", want: "Café_Über"},
		{name: "empty string stays empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilenameSafe(tt.input); got != tt.want {
				t.Errorf("FilenameSafe(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAlnumOnly(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "letters and digits are untouched", input: "Song123", want: "Song123"},
		{name: "spaces and punctuation are removed", input: "My Song, Pt. 2!", want: "MySongPt2"},
		{name: "dashes and underscores are removed (edge case)", input: "my-song_name", want: "mysongname"},
		{name: "unicode letters are preserved", input: "Café Über", want: "CaféÜber"},
		{name: "empty string stays empty", input: "", want: ""},
		{name: "all-punctuation string becomes empty (edge case)", input: "!?.,-", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AlnumOnly(tt.input); got != tt.want {
				t.Errorf("AlnumOnly(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
