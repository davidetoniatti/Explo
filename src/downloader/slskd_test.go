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

	t.Run("disallowed extension combined with a filtered keyword is excluded", func(t *testing.T) {
		c := newTestSlskd([]string{"flac", "mp3"}, 0, 0, 3, []string{"live"})
		results := SearchResults{
			{
				FileCount:         1,
				HasFreeUploadSlot: true,
				Files: []File{
					// extension "wav" isn't in the allowed list AND the filename contains
					// the filtered keyword "live" -> both conditions combine to exclude it,
					// even though it otherwise matches artist/title.
					{Name: "Test Artist - Test Title (Live).wav", Length: 200},
				},
			},
		}

		_, err := c.CollectFiles(track, results)
		if err == nil {
			t.Fatalf("expected an error when the only candidate is filtered out by extension+keyword")
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

	t.Run("bitrate exactly at the minimum is rejected (edge case)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 256, 0, 3, nil)
		files := []File{
			{Extension: "flac", BitRate: 256}, // == MinBitRate, condition is "<=" so it's rejected
		}

		_, err := c.filterFiles(files)
		if err == nil {
			t.Fatalf("expected an error since bitrate == MinBitRate should be rejected")
		}
	})

	t.Run("bitdepth exactly at the minimum is rejected (edge case)", func(t *testing.T) {
		c := newTestSlskd([]string{"flac"}, 0, 16, 3, nil)
		files := []File{
			{Extension: "flac", BitDepth: 16}, // == MinBitDepth, condition is "<=" so it's rejected
		}

		_, err := c.filterFiles(files)
		if err == nil {
			t.Fatalf("expected an error since bitdepth == MinBitDepth should be rejected")
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
