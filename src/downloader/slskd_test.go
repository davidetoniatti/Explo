package downloader

import (
	"testing"

	"explo/src/config"
	"explo/src/models"
)

func newTestSlskd(extensions []string, minBitRate, minBitDepth, downloadAttempts int, filterList []string) Slskd {
	return Slskd{
		Cfg: config.Slskd{
			DownloadAttempts: downloadAttempts,
			Filters: config.Filters{
				Extensions:  extensions,
				MinBitRate:  minBitRate,
				MinBitDepth: minBitDepth,
				FilterList:  filterList,
			},
		},
	}
}

func TestSlskd_CollectFiles(t *testing.T) {
	track := models.Track{
		MainArtist: "Test Artist",
		Album:      "Test Album",
		CleanTitle: "Test Title",
		Duration:   200000, // 200s in ms
	}

	t.Run("matches on filename and duration", func(t *testing.T) {
		c := newTestSlskd([]string{"flac", "mp3"}, 0, 0, 3, []string{"live"})
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Username:          "peer1",
				Files: []File{
					{Name: "Test Artist - Test Title.flac", Length: 200},
				},
			},
		}

		files, err := c.CollectFiles(track, results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 || files[0].Username != "peer1" {
			t.Fatalf("expected 1 matching file with username set, got %+v", files)
		}
	})

	t.Run("no free upload slot excludes the whole result", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: false, // edge case: result gated out despite a perfect filename match
				Files: []File{
					{Name: "Test Artist - Test Title.flac", Length: 200},
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when no result has a free upload slot")
		}
	})

	t.Run("filename not matching artist/album/title is excluded", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Files: []File{
					{Name: "Someone Else - Other Song.flac", Length: 200},
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when no file matches artist/album/title")
		}
	})

	t.Run("disallowed extension alone is excluded", func(t *testing.T) {
		c := newTestSlskd([]string{"flac", "mp3"}, 0, 0, 3, nil)
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Files: []File{
					// "wav" isn't in the allowed list, that on its own is disqualifying
					// even though the file matches artist/title and trips no keyword.
					{Name: "Test Artist - Test Title.wav", Length: 200},
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when the only candidate has a disallowed extension")
		}
	})

	t.Run("filtered keyword alone is excluded", func(t *testing.T) {
		c := newTestSlskd([]string{"flac", "mp3"}, 0, 0, 3, []string{"live"})
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Files: []File{
					// allowed extension, but "live" is a filtered keyword absent from the
					// track's own title/artist, so it must not be collected.
					{Name: "Test Artist - Test Title (Live).flac", Length: 200},
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when the only candidate matches a filtered keyword")
		}
	})

	t.Run("filename extension wins over a wrong reported extension", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Username:          "peer1",
				Files: []File{
					// peers frequently report a bogus extension, the name is authoritative
					{Name: "Test Artist - Test Title.flac", Extension: "wav", Length: 200},
				},
			},
		}

		files, err := c.CollectFiles(track, results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 || files[0].Extension != "flac" {
			t.Fatalf("expected the extension to be taken from the filename, got %+v", files)
		}
	})

	t.Run("duration mismatch beyond 10s excludes the file", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Files: []File{
					{Name: "Test Artist - Test Title.flac", Length: 100}, // 100s vs 200s track duration
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when duration differs by more than 10s")
		}
	})
}

func TestSlskd_FilterFiles(t *testing.T) {
	t.Run("keeps files above bitrate/bitdepth thresholds", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 256, 16, 3, nil)
		files := []File{
			{Extension: "flac", BitRate: 320, BitDepth: 24},
		}

		filtered, err := c.filterFiles(files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected 1 file to pass, got %+v", filtered)
		}
	})

	t.Run("bitrate exactly at the minimum is kept (boundary)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 256, 0, 3, nil)
		files := []File{
			{Extension: "flac", BitRate: 256}, // == MinBitRate, the minimum is inclusive
		}

		filtered, err := c.filterFiles(files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected bitrate == MinBitRate to pass, got %+v", filtered)
		}
	})

	t.Run("bitrate below the minimum is rejected", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 256, 0, 3, nil)
		files := []File{
			{Extension: "flac", BitRate: 255},
		}

		_, err := c.filterFiles(files)
		if err == nil {
			t.Fatalf("expected an error since bitrate < MinBitRate should be rejected")
		}
	})

	t.Run("bitdepth exactly at the minimum is kept (boundary)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 16, 3, nil)
		files := []File{
			{Extension: "flac", BitDepth: 16}, // == MinBitDepth, the minimum is inclusive
		}

		filtered, err := c.filterFiles(files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected bitdepth == MinBitDepth to pass, got %+v", filtered)
		}
	})

	t.Run("bitdepth below the minimum is rejected", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 16, 3, nil)
		files := []File{
			{Extension: "flac", BitDepth: 8},
		}

		_, err := c.filterFiles(files)
		if err == nil {
			t.Fatalf("expected an error since bitdepth < MinBitDepth should be rejected")
		}
	})

	t.Run("extension not in the configured list is never selected (edge case)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)
		files := []File{
			{Extension: "wav", BitRate: 320, BitDepth: 24},
		}

		_, err := c.filterFiles(files)
		if err == nil {
			t.Fatalf("expected an error since \"wav\" isn't in the allowed extensions list")
		}
	})

	t.Run("stops once DownloadAttempts is reached (edge case)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 1, nil)
		files := []File{
			{Extension: "flac", BitRate: 320},
			{Extension: "flac", BitRate: 320},
		}

		filtered, err := c.filterFiles(files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filtered) != 1 {
			t.Fatalf("expected filterFiles to stop at DownloadAttempts=1, got %d files", len(filtered))
		}
	})

	t.Run("empty input returns an error", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 0, 3, nil)

		_, err := c.filterFiles(nil)
		if err == nil {
			t.Fatalf("expected an error for empty input")
		}
	})
}

func TestWildcardArtist(t *testing.T) {
	tests := []struct {
		name   string
		artist string
		want   string
	}{
		{
			name:   "first character is wildcarded",
			artist: "Radiohead",
			want:   "*adiohead",
		},
		{
			name:   "leading The is preserved",
			artist: "The Weeknd",
			want:   "The *eeknd",
		},
		{
			name:   "leading The is matched case insensitively",
			artist: "the Prodigy",
			want:   "the *rodigy",
		},
		{
			name:   "short name is returned unchanged",
			artist: "U2",
			want:   "U2",
		},
		{
			name:   "short name after The prefix is returned unchanged",
			artist: "The Do",
			want:   "The Do",
		},
		{
			name:   "multibyte name wildcards a whole rune",
			artist: "Ólafur Arnalds",
			want:   "*lafur Arnalds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wildcardArtist(tt.artist); got != tt.want {
				t.Errorf("wildcardArtist(%q) = %q, want %q", tt.artist, got, tt.want)
			}
		})
	}
}

func TestNormalizeState(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  string
	}{
		{
			name:  "in-progress state is left alone",
			state: "InProgress",
			want:  "InProgress",
		},
		{
			name:  "success is left alone",
			state: "Completed, Succeeded",
			want:  "Completed, Succeeded",
		},
		{
			name:  "rejected collapses to Errored",
			state: "Completed, Rejected",
			want:  "Errored",
		},
		{
			name:  "cancelled collapses to Errored",
			state: "Completed, Cancelled",
			want:  "Errored",
		},
		{
			name:  "timed out collapses to Errored",
			state: "Completed, TimedOut",
			want:  "Errored",
		},
		{
			name:  "unspaced compound state is handled",
			state: "Completed,Errored",
			want:  "Errored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeState(tt.state); got != tt.want {
				t.Errorf("normalizeState(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantFile      string
		wantParentDir string
	}{
		{
			name:          "unix-style path",
			path:          "Music/Artist/Song.flac",
			wantFile:      "Song.flac",
			wantParentDir: "Artist",
		},
		{
			name:          "windows-style path with backslashes",
			path:          `C:\Music\Artist\Song.flac`,
			wantFile:      "Song.flac",
			wantParentDir: "Artist",
		},
		{
			name:          "no parent directory (edge case)",
			path:          "Song.flac",
			wantFile:      "Song.flac",
			wantParentDir: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, parentDir := parsePath(tt.path)
			if file != tt.wantFile || parentDir != tt.wantParentDir {
				t.Errorf("parsePath(%q) = (%q, %q), want (%q, %q)", tt.path, file, parentDir, tt.wantFile, tt.wantParentDir)
			}
		})
	}
}
