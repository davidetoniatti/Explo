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

func TestContainsFold(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact match", "Radiohead", "Radiohead", true},
		{"different case", "RADIOHEAD", "radiohead", true},
		{"substring", "Radiohead & Friends", "radiohead", true},
		{"absent", "Radiohead", "Portishead", false},
		{"empty substring is always contained (edge case)", "Radiohead", "", true},
		{"empty haystack", "", "Radiohead", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsFold(tt.s, tt.substr); got != tt.want {
				t.Errorf("ContainsFold(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestCleanSearchTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain title is untouched", "Karma Police", "Karma Police"},
		{"trailing feat in parens is dropped", "Song (feat. Guest)", "Song"},
		{"trailing feat in brackets is dropped", "Song [feat. Guest]", "Song"},
		{"ft. abbreviation is dropped", "Song (ft. Guest)", "Song"},
		{"featuring spelled out is dropped", "Song (featuring Guest)", "Song"},
		// "with" is not treated as a credit marker: real titles open a parenthetical
		// with it far more often than credits do.
		{"with is kept, it is not a credit marker", "Song (with Guest)", "Song (with Guest)"},
		{"a title ending in a with-parenthetical keeps it", "Baby (With You)", "Baby (With You)"},
		{"case is ignored", "Song (FEAT. Guest)", "Song"},
		{"multiple guests are dropped together", "Song (feat. A, B & C)", "Song"},
		{"remaster suffix is dropped", "Song - 2011 Remaster", "Song"},
		{"remastered suffix is dropped", "Song - 1999 Remastered", "Song"},
		{"en dash remaster is dropped", "Song – 2011 Remaster", "Song"},
		{"feat and remaster together", "Song (feat. Guest) - 2011 Remaster", "Song"},
		{"remaster then feat, the other order", "Song - 2011 Remaster (feat. Guest)", "Song"},
		// Only a trailing annotation is an annotation. These words are part of the title.
		{"a title that is about featuring keeps its words", "Featuring The Fool", "Featuring The Fool"},
		{"with mid-title is kept", "Dancing with Myself Tonight", "Dancing with Myself Tonight"},
		{"parenthetical that is not a credit is kept", "Song (Live at Wembley)", "Song (Live at Wembley)"},
		{"case is otherwise preserved", "KARMA Police", "KARMA Police"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanSearchTitle(tt.in); got != tt.want {
				t.Errorf("CleanSearchTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	t.Run("two spellings of one title compare equal", func(t *testing.T) {
		pairs := [][2]string{
			{"Song (feat. Guest)", "Song"},
			{"Song - 2011 Remaster", "song"},
			{"Don't Stop", "Dont Stop"},
			{"Sgt. Pepper's", "Sgt Peppers"},
			{"A  Double   Space", "A Double Space"},
		}

		for _, pair := range pairs {
			if NormalizeTitle(pair[0]) != NormalizeTitle(pair[1]) {
				t.Errorf("NormalizeTitle(%q) = %q != NormalizeTitle(%q) = %q",
					pair[0], NormalizeTitle(pair[0]), pair[1], NormalizeTitle(pair[1]))
			}
		}
	})

	t.Run("genuinely different titles stay different", func(t *testing.T) {
		if NormalizeTitle("Karma Police") == NormalizeTitle("Karma Polices") {
			t.Error("expected distinct titles to normalize differently")
		}
	})

	t.Run("accented letters are kept so they still distinguish titles", func(t *testing.T) {
		// AlnumOnly keeps unicode letters, so "Rós" does not collapse to "Rs".
		if got := NormalizeTitle("Sigur Rós"); got != "sigurrós" {
			t.Errorf("NormalizeTitle = %q, want %q", got, "sigurrós")
		}
	})

	t.Run("an empty title normalizes to empty", func(t *testing.T) {
		if got := NormalizeTitle(""); got != "" {
			t.Errorf("NormalizeTitle(\"\") = %q, want empty", got)
		}
	})
}
