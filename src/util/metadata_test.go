package util

import (
	"slices"
	"strings"
	"testing"

	"explo/src/models"
)

// tagValue returns the value of the first "key=value" entry matching key.
func tagValue(metadata []string, key string) (string, bool) {
	for _, entry := range metadata {
		if name, value, found := strings.Cut(entry, "="); found && name == key {
			return value, true
		}
	}
	return "", false
}

func TestBuildffmpegMetadata(t *testing.T) {
	t.Run("writes the full picard tag set", func(t *testing.T) {
		track := models.Track{
			Title:                     "Test Title",
			Album:                     "Test Album",
			Artist:                    "Test Artist",
			AlbumArtist:               "Test Album Artist",
			ArtistSort:                "Artist, Test",
			Genres:                    "rock; indie",
			OriginalDate:              "1997-06-16",
			OriginalYear:              1997,
			ReleaseCountry:            "GB",
			ReleaseStatus:             "Official",
			ReleaseType:               "Album",
			Media:                     "CD",
			TrackNumber:               3,
			TrackTotal:                12,
			DiscNumber:                1,
			DiscTotal:                 2,
			MusicBrainzTrackID:        "rec-mbid",
			MusicBrainzReleaseTrackID: "reltrack-mbid",
			MusicBrainzAlbumID:        "rel-mbid",
			MusicBrainzReleaseGroupID: "rg-mbid",
			MusicBrainzArtistID:       "artist-mbid",
			MusicBrainzAlbumArtistID:  "albumartist-mbid",
		}

		metadata := BuildffmpegMetadata(track)

		want := map[string]string{
			"artist":                     "Test Artist",
			"title":                      "Test Title",
			"album":                      "Test Album",
			"albumartist":                "Test Album Artist",
			"artistsort":                 "Artist, Test",
			"date":                       "1997-06-16",
			"genre":                      "rock; indie",
			"releasecountry":             "GB",
			"TMED":                       "CD",
			"MusicBrainz_AlbumType":      "Album",
			"MusicBrainz_AlbumStatus":    "Official",
			"MusicBrainz_TrackId":        "rec-mbid",
			"MusicBrainz_ReleaseTrackId": "reltrack-mbid",
			"MusicBrainz_AlbumId":        "rel-mbid",
			"MusicBrainz_ReleaseGroupId": "rg-mbid",
			"MusicBrainz_ArtistId":       "artist-mbid",
			"MusicBrainz_AlbumArtistId":  "albumartist-mbid",
			"originalyear":               "1997",
			"track":                      "3",
			"Tracktotal":                 "12",
			"disc":                       "1",
			"Disctotal":                  "2",
		}

		for key, expected := range want {
			got, ok := tagValue(metadata, key)
			if !ok {
				t.Errorf("missing tag %q", key)
				continue
			}
			if got != expected {
				t.Errorf("tag %q = %q, want %q", key, got, expected)
			}
		}
	})

	t.Run("multiple artists are joined and win over the single artist field", func(t *testing.T) {
		track := models.Track{
			Artist:  "Should Not Be Used",
			Artists: []string{"First", "Second", "Third"},
		}

		got, ok := tagValue(BuildffmpegMetadata(track), "artist")
		if !ok {
			t.Fatalf("missing artist tag")
		}
		if got != "First; Second; Third" {
			t.Errorf("artist = %q, want %q", got, "First; Second; Third")
		}
	})

	t.Run("empty and zero fields are omitted rather than written blank", func(t *testing.T) {
		metadata := BuildffmpegMetadata(models.Track{Title: "Only Title"})

		if len(metadata) != 1 {
			t.Fatalf("expected only the title tag, got %v", metadata)
		}
		if metadata[0] != "title=Only Title" {
			t.Errorf("got %q, want %q", metadata[0], "title=Only Title")
		}
	})

	t.Run("each ISRC gets its own tag", func(t *testing.T) {
		track := models.Track{ISRCs: []string{"GBAYE9700263", "USRC17607839"}}

		metadata := BuildffmpegMetadata(track)

		if !slices.Contains(metadata, "ISRC=GBAYE9700263") || !slices.Contains(metadata, "ISRC=USRC17607839") {
			t.Errorf("expected one tag per ISRC, got %v", metadata)
		}
	})

	t.Run("a fully empty track produces no tags at all (edge case)", func(t *testing.T) {
		// ffmpeg-go drops an empty []string, so this is not a crash, but writing a
		// list of blank tags for an unpopulated track would still be wrong.
		if metadata := BuildffmpegMetadata(models.Track{}); len(metadata) != 0 {
			t.Errorf("expected no tags for an empty track, got %v", metadata)
		}
	})

	t.Run("blank artist credits do not produce an empty artist tag (edge case)", func(t *testing.T) {
		// A credit list of empty names must fall back to the single artist field
		// rather than writing "artist=".
		track := models.Track{Artist: "Fallback", Artists: []string{"", ""}}

		got, ok := tagValue(BuildffmpegMetadata(track), "artist")
		if !ok || got != "Fallback" {
			t.Errorf("artist = %q (present=%v), want %q", got, ok, "Fallback")
		}
	})

	t.Run("blank entries are dropped from a credit list (edge case)", func(t *testing.T) {
		track := models.Track{Artists: []string{"First", "", "Third"}}

		got, _ := tagValue(BuildffmpegMetadata(track), "artist")
		if got != "First; Third" {
			t.Errorf("artist = %q, want %q", got, "First; Third")
		}
	})

	t.Run("zero valued numbers are skipped (edge case)", func(t *testing.T) {
		// A track with no known position must not be tagged "track=0", which some
		// players render as a real track number.
		metadata := BuildffmpegMetadata(models.Track{Title: "T", TrackNumber: 0, DiscNumber: 0})

		if _, ok := tagValue(metadata, "track"); ok {
			t.Errorf("track tag should be omitted when zero, got %v", metadata)
		}
		if _, ok := tagValue(metadata, "disc"); ok {
			t.Errorf("disc tag should be omitted when zero, got %v", metadata)
		}
	})
}
