package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"explo/src/models"
)

func TestFilterLocalTracks(t *testing.T) {
	newTracks := func() []*models.Track {
		return []*models.Track{
			{Title: "present", Present: true},
			{Title: "missing", Present: false},
		}
	}

	t.Run("preDownload keeps only unavailable tracks", func(t *testing.T) {
		tracks := newTracks()
		filterLocalTracks(&tracks, true)

		if len(tracks) != 1 || tracks[0].Title != "missing" {
			t.Fatalf("expected only the missing track to remain, got %+v", tracks)
		}
	})

	t.Run("postDownload keeps only present tracks and resets Present", func(t *testing.T) {
		tracks := newTracks()
		filterLocalTracks(&tracks, false)

		if len(tracks) != 1 || tracks[0].Title != "present" {
			t.Fatalf("expected only the present track to remain, got %+v", tracks)
		}
		if tracks[0].Present {
			t.Errorf("expected Present to be reset to false, got true")
		}
	})

	t.Run("empty slice edge case", func(t *testing.T) {
		tracks := []*models.Track{}
		filterLocalTracks(&tracks, true)

		if len(tracks) != 0 {
			t.Fatalf("expected empty slice to remain empty, got %+v", tracks)
		}
	})
}

func TestContainsKeyword(t *testing.T) {
	tests := []struct {
		name         string
		track        models.Track
		contentTitle string
		filterList   []string
		want         bool
	}{
		{
			name:         "keyword found in unrelated content title",
			track:        models.Track{Title: "Song", Artist: "Artist"},
			contentTitle: "Song (Live Version)",
			filterList:   []string{"live"},
			want:         true,
		},
		{
			name:         "no keyword match",
			track:        models.Track{Title: "Song", Artist: "Artist"},
			contentTitle: "Song (Studio Version)",
			filterList:   []string{"live"},
			want:         false,
		},
		{
			name:         "keyword-in-title edge case: keyword is part of the track's own title, so it's not filtered even though the content title matches",
			track:        models.Track{Title: "Live to Tell", Artist: "Artist"},
			contentTitle: "Live to Tell (Live)",
			filterList:   []string{"live"},
			want:         false,
		},
		{
			name:         "keyword-in-artist edge case: keyword is part of the artist name",
			track:        models.Track{Title: "Song", Artist: "Live Band"},
			contentTitle: "Song (Live)",
			filterList:   []string{"live"},
			want:         false,
		},
		{
			name:         "case-insensitive match",
			track:        models.Track{Title: "Song", Artist: "Artist"},
			contentTitle: "Song (REMIX)",
			filterList:   []string{"Remix"},
			want:         true,
		},
		{
			name:         "empty filter list never matches",
			track:        models.Track{Title: "Song", Artist: "Artist"},
			contentTitle: "Song (Live)",
			filterList:   []string{},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsKeyword(tt.track, tt.contentTitle, tt.filterList); got != tt.want {
				t.Errorf("ContainsKeyword(%+v, %q, %v) = %v, want %v", tt.track, tt.contentTitle, tt.filterList, got, tt.want)
			}
		})
	}
}

func TestGetFilename(t *testing.T) {
	t.Run("simple title and artist", func(t *testing.T) {
		got := getFilename("My Song", "My Artist")
		want := "My_Song-My_Artist"
		if got != want {
			t.Errorf("getFilename() = %q, want %q", got, want)
		}
	})

	t.Run("illegal characters are replaced", func(t *testing.T) {
		got := getFilename("Song: Part/2", "Artist?")
		if strings.ContainsAny(got, ":/?") {
			t.Errorf("getFilename() = %q, expected illegal characters to be stripped", got)
		}
	})

	t.Run("truncates long filenames to maxBytes", func(t *testing.T) {
		longTitle := strings.Repeat("a", 300)
		got := getFilename(longTitle, "Artist")
		if len(got) > 240 {
			t.Errorf("getFilename() returned %d bytes, want <= 240", len(got))
		}
	})

	t.Run("truncation respects unicode rune boundaries", func(t *testing.T) {
		// multi-byte runes (each 3 bytes in UTF-8) repeated enough to exceed maxBytes
		longTitle := strings.Repeat("日", 200)
		got := getFilename(longTitle, "Artist")

		if len(got) > 240 {
			t.Errorf("getFilename() returned %d bytes, want <= 240", len(got))
		}
		if !isValidUTF8Runes(got) {
			t.Errorf("getFilename() = %q, expected valid UTF-8 rune sequence after truncation", got)
		}
	})
}

func isValidUTF8Runes(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestIsDirEmpty(t *testing.T) {
	t.Run("empty directory returns true", func(t *testing.T) {
		dir := t.TempDir()

		empty, err := isDirEmpty(dir)
		if err != nil {
			t.Fatalf("isDirEmpty() unexpected error: %v", err)
		}
		if !empty {
			t.Errorf("isDirEmpty() = false, want true for an empty directory")
		}
	})

	t.Run("non-empty directory returns false", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
			t.Fatalf("failed to set up test file: %v", err)
		}

		empty, err := isDirEmpty(dir)
		if err != nil {
			t.Fatalf("isDirEmpty() unexpected error: %v", err)
		}
		if empty {
			t.Errorf("isDirEmpty() = true, want false for a non-empty directory")
		}
	})

	t.Run("nonexistent directory returns an error", func(t *testing.T) {
		_, err := isDirEmpty(filepath.Join(t.TempDir(), "does-not-exist"))
		if err == nil {
			t.Errorf("isDirEmpty() expected an error for a nonexistent path, got nil")
		}
	})
}
