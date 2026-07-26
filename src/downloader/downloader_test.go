package downloader

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"

	ffmpeg "github.com/u2takey/ffmpeg-go"
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

func TestTempAudioFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "extension is preserved so ffmpeg can infer the container",
			path: "/downloads/Artist - Song.flac",
			want: "/downloads/Artist - Song.tmp.flac",
		},
		{
			name: "only the final extension is replaced",
			path: "/downloads/Song.remastered.mp3",
			want: "/downloads/Song.remastered.tmp.mp3",
		},
		{
			name: "a dot in the directory name is not mistaken for an extension",
			path: "/downloads/v1.2/Song.mp3",
			want: "/downloads/v1.2/Song.tmp.mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tempAudioFile(tt.path)
			if got != tt.want {
				t.Errorf("tempAudioFile(%q) = %q, want %q", tt.path, got, tt.want)
			}
			if filepath.Ext(got) != filepath.Ext(tt.path) {
				t.Errorf("extension changed: %q -> %q", filepath.Ext(tt.path), filepath.Ext(got))
			}
		})
	}
}

func TestOverwriteMetadataRejectsExtensionlessFile(t *testing.T) {
	// ffmpeg picks the output container from the extension, so an extensionless
	// source has to be refused up front rather than failing inside ffmpeg.
	c := &DownloadClient{Cfg: &cfg.DownloadConfig{}}

	// The file has to exist, otherwise ffmpeg would fail with "no such file" and the
	// test would pass whether or not the extension guard is there at all.
	srcFile := filepath.Join(t.TempDir(), "no_extension")
	if err := os.WriteFile(srcFile, []byte("not really audio"), 0644); err != nil {
		t.Fatalf("failed to set up test file: %v", err)
	}

	err := c.overwriteMetadata(srcFile, &models.Track{Title: "T"})
	if err == nil {
		t.Fatal("expected an error for a source file with no extension")
	}
	if !strings.Contains(err.Error(), "no extension") {
		t.Errorf("expected the extension guard to reject it, got a different failure: %v", err)
	}
}

func TestSanitizePathSegment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"path separators become dashes", "AC/DC", "AC-DC"},
		{"backslash becomes a dash", `AC\DC`, "AC-DC"},
		{"colon becomes a dash", "Album: The Remix", "Album- The Remix"},
		{"wildcards and quotes are dropped", `What"s *this*?`, "Whats this"},
		{"angle brackets and pipes are dropped", "a<b>c|d", "abcd"},
		{"spaces and accents are kept", "Sigur Rós – Untitled", "Sigur Rós – Untitled"},
		{"surrounding whitespace is trimmed", "  Song  ", "Song"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizePathSegment(tt.in); got != tt.want {
				t.Errorf("sanitizePathSegment(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildTrackPath(t *testing.T) {
	track := &models.Track{
		MainArtist:   "The Artist",
		AlbumArtist:  "The Album Artist",
		Album:        "The Album",
		CleanTitle:   "The Song",
		File:         "whatever.flac",
		TrackNumber:  3,
		DiscNumber:   1,
		OriginalYear: 1997,
	}

	t.Run("substitutes every placeholder", func(t *testing.T) {
		got, err := buildTrackPath("{{Artist}}/{{Album}} ({{Year}})/{{DiscNumber}}-{{TrackNumber}} {{TrackName}}.{{ext}}", track)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join("The Artist", "The Album (1997)", "01-03 The Song.flac")
		if got != want {
			t.Errorf("buildTrackPath = %q, want %q", got, want)
		}
	})

	t.Run("album artist placeholder", func(t *testing.T) {
		got, err := buildTrackPath("{{AlbumArtist}}/{{TrackName}}.{{ext}}", track)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join("The Album Artist", "The Song.flac") {
			t.Errorf("buildTrackPath = %q", got)
		}
	})

	t.Run("separators in values cannot create directories (security)", func(t *testing.T) {
		evil := &models.Track{MainArtist: "../../etc", CleanTitle: "passwd", File: "x.mp3"}

		got, err := buildTrackPath("{{Artist}}/{{TrackName}}.{{ext}}", evil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "/" inside a value becomes "-", so the value stays a single path segment
		if got != filepath.Join("..-..-etc", "passwd.mp3") {
			t.Errorf("separators leaked out of a substituted value: %q", got)
		}
	})

	t.Run("a template that escapes the download directory is rejected (security)", func(t *testing.T) {
		if _, err := buildTrackPath("../{{TrackName}}.{{ext}}", track); err == nil {
			t.Error("expected a template containing .. to be rejected")
		}
	})

	t.Run("an absolute template is rejected (security)", func(t *testing.T) {
		if _, err := buildTrackPath("/etc/{{TrackName}}.{{ext}}", track); err == nil {
			t.Error("expected an absolute template to be rejected")
		}
	})

	t.Run("an empty placeholder collapses instead of making the path absolute", func(t *testing.T) {
		// "{{Year}}/..." on a track with no year would otherwise render "/Song.mp3",
		// which is an absolute path pointing outside the download directory.
		noYear := &models.Track{CleanTitle: "Song", File: "x.mp3"}

		got, err := buildTrackPath("{{Year}}/{{TrackName}}.{{ext}}", noYear)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Song.mp3" {
			t.Errorf("buildTrackPath = %q, want %q", got, "Song.mp3")
		}
		if filepath.IsAbs(got) {
			t.Errorf("path must stay relative, got %q", got)
		}
	})

	t.Run("a value of .. cannot escape the download directory (security)", func(t *testing.T) {
		evil := &models.Track{MainArtist: "..", CleanTitle: "Song", File: "x.mp3"}

		if _, err := buildTrackPath("{{Artist}}/{{TrackName}}.{{ext}}", evil); err == nil {
			t.Error("expected a .. path segment to be rejected")
		}
	})

	t.Run("track and disc numbers are zero padded", func(t *testing.T) {
		got, err := buildTrackPath("{{TrackNumber}}.{{ext}}", track)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "03.flac" {
			t.Errorf("buildTrackPath = %q, want %q", got, "03.flac")
		}
	})
}

func TestSupportsCoverArt(t *testing.T) {
	for _, path := range []string{"a.mp3", "a.flac", "a.M4A", "a.mp4"} {
		if !supportsCoverArt(path) {
			t.Errorf("expected %q to support cover art", path)
		}
	}
	// Opus and Ogg store artwork in a metadata block ffmpeg will not write, so asking
	// for an attached picture there fails the whole conversion.
	for _, path := range []string{"a.opus", "a.ogg", "a.webm", "a"} {
		if supportsCoverArt(path) {
			t.Errorf("expected %q not to support cover art", path)
		}
	}
}

func TestBuildAudioOutput(t *testing.T) {
	track := &models.Track{Title: "Song", Artist: "Artist"}

	t.Run("without a cover it preserves artwork the source already had", func(t *testing.T) {
		streams, opts := buildAudioOutput("in.flac", "", track, true)

		if len(streams) != 1 {
			t.Fatalf("expected a single input, got %d", len(streams))
		}
		maps, _ := opts["map"].([]string)
		if len(maps) != 2 || maps[0] != "0:a" || maps[1] != "0:v?" {
			t.Errorf("map = %v, want [0:a 0:v?]", opts["map"])
		}
		if opts["c:a"] != "copy" {
			t.Errorf("expected the audio to be copied, got %v", opts["c:a"])
		}
	})

	t.Run("with a cover it attaches the image as a picture stream", func(t *testing.T) {
		streams, opts := buildAudioOutput("in.flac", "cover.jpg", track, true)

		if len(streams) != 2 {
			t.Fatalf("expected the cover to be added as a second input, got %d", len(streams))
		}
		if _, ok := opts["map"]; ok {
			// ffmpeg-go maps every stream itself once more than one is passed. A "map"
			// option here would map each input a second time, and ffmpeg refuses the
			// duplicated audio stream.
			t.Errorf("map must come from the stream selectors, not an option: %v", opts["map"])
		}
		if opts["disposition:v"] != "attached_pic" {
			t.Errorf("expected the cover to be marked as an attached picture, got %v", opts["disposition:v"])
		}
	})

	t.Run("the generated command maps each input exactly once", func(t *testing.T) {
		// Guards the failure this feature originally shipped with: duplicate -map flags
		// made ffmpeg reject every cover-embedding conversion.
		streams, opts := buildAudioOutput("in.flac", "cover.jpg", track, true)
		args := ffmpeg.Output(streams, "out.flac", opts).OverWriteOutput().GetArgs()

		var maps []string
		for i, arg := range args {
			if arg == "-map" && i+1 < len(args) {
				maps = append(maps, args[i+1])
			}
		}

		if len(maps) != 2 || maps[0] != "0:a" || maps[1] != "1:v" {
			t.Errorf("expected exactly [-map 0:a -map 1:v], got %v (full args: %v)", maps, args)
		}
	})

	t.Run("re-encoding omits the audio copy flag", func(t *testing.T) {
		// YouTube downloads arrive in whatever format yt-dlp picked, so the audio has
		// to be encoded for the target container rather than copied.
		_, opts := buildAudioOutput("in.webm", "", track, false)

		if _, ok := opts["c:a"]; ok {
			t.Errorf("expected no audio codec flag when re-encoding, got %v", opts["c:a"])
		}
	})

	t.Run("metadata is always written", func(t *testing.T) {
		_, opts := buildAudioOutput("in.flac", "", track, true)

		metadata, _ := opts["metadata"].([]string)
		if len(metadata) == 0 {
			t.Error("expected track metadata to be included")
		}
	})
}

// TestBuildAudioOutputRunsUnderFfmpeg drives real ffmpeg with the generated options.
// The unit tests above only pin the argument list, they cannot show that ffmpeg
// accepts it: the first version of this feature produced a plausible looking command
// that ffmpeg rejected for every single conversion.
func TestBuildAudioOutputRunsUnderFfmpeg(t *testing.T) {
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed, skipping end-to-end conversion test")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "in.flac")
	cover := filepath.Join(dir, "cover.jpg")

	mustRun := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(ffmpegBin, args...).CombinedOutput(); err != nil {
			t.Fatalf("fixture setup failed: %v\n%s", err, out)
		}
	}
	mustRun("-y", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "flac", source)
	mustRun("-y", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=red:s=64x64:d=1", "-frames:v", "1", cover)

	track := &models.Track{Title: "Song", Artist: "Artist", Album: "Album"}

	t.Run("without a cover", func(t *testing.T) {
		out := filepath.Join(dir, "no_cover.flac")
		streams, opts := buildAudioOutput(source, "", track, true)

		if err := util.WriteMetadata(streams, "", out, opts); err != nil {
			t.Fatalf("ffmpeg rejected the generated command: %v", err)
		}
		if info, err := os.Stat(out); err != nil || info.Size() == 0 {
			t.Fatalf("no output written: %v", err)
		}
	})

	t.Run("with an attached cover", func(t *testing.T) {
		out := filepath.Join(dir, "with_cover.flac")
		streams, opts := buildAudioOutput(source, cover, track, true)

		if err := util.WriteMetadata(streams, "", out, opts); err != nil {
			t.Fatalf("ffmpeg rejected the generated command: %v", err)
		}

		probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v",
			"-show_entries", "stream_disposition=attached_pic", "-of", "csv=p=0", out).Output()
		if err != nil {
			t.Skipf("ffprobe unavailable, cannot verify the attached picture: %v", err)
		}
		if !strings.Contains(string(probe), "1") {
			t.Errorf("expected an attached picture in the output, ffprobe said %q", probe)
		}
	})
}
