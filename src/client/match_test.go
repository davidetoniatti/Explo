package client

import (
	"testing"

	"explo/src/models"
)

func TestMusicBrainzMatch(t *testing.T) {
	t.Run("matches the recording id", func(t *testing.T) {
		track := &models.Track{MusicBrainzTrackID: "rec-1"}

		if !musicBrainzMatch(track, "rec-1") {
			t.Error("expected the recording id to match")
		}
	})

	t.Run("matches the release track id", func(t *testing.T) {
		track := &models.Track{MusicBrainzReleaseTrackID: "reltrack-1"}

		if !musicBrainzMatch(track, "reltrack-1") {
			t.Error("expected the release track id to match")
		}
	})

	t.Run("either of ours matches any of theirs", func(t *testing.T) {
		// Systems disagree about which id lands in which field, so the comparison is
		// deliberately cross-wise rather than field-to-field.
		track := &models.Track{MusicBrainzTrackID: "rec-1", MusicBrainzReleaseTrackID: "reltrack-1"}

		if !musicBrainzMatch(track, "", "reltrack-1") {
			t.Error("expected a match on the second id")
		}
	})

	t.Run("ids are compared case insensitively", func(t *testing.T) {
		track := &models.Track{MusicBrainzTrackID: "ABC-123"}

		if !musicBrainzMatch(track, "abc-123") {
			t.Error("expected case-insensitive comparison")
		}
	})

	t.Run("a track with no ids never matches (edge case)", func(t *testing.T) {
		// Otherwise every untagged library item would match every untagged track.
		if musicBrainzMatch(&models.Track{}, "", "") {
			t.Error("a track without MusicBrainz ids must not match")
		}
		if musicBrainzMatch(&models.Track{}, "some-id") {
			t.Error("a track without MusicBrainz ids must not match")
		}
	})

	t.Run("an item with no ids never matches (edge case)", func(t *testing.T) {
		track := &models.Track{MusicBrainzTrackID: "rec-1"}

		if musicBrainzMatch(track) {
			t.Error("no ids supplied should not match")
		}
		if musicBrainzMatch(track, "", "") {
			t.Error("empty ids should not match")
		}
	})

	t.Run("different ids do not match", func(t *testing.T) {
		track := &models.Track{MusicBrainzTrackID: "rec-1"}

		if musicBrainzMatch(track, "rec-2") {
			t.Error("unrelated ids must not match")
		}
	})
}

func TestArtistMatches(t *testing.T) {
	t.Run("matches the album artist exactly", func(t *testing.T) {
		if !artistMatches("Radiohead", "radiohead", nil) {
			t.Error("expected a case-insensitive album artist match")
		}
	})

	t.Run("matches a credited artist as a substring", func(t *testing.T) {
		if !artistMatches("Radiohead", "Various Artists", []string{"Radiohead & Friends"}) {
			t.Error("expected a credited artist match")
		}
	})

	t.Run("checks every credited artist, not just the first", func(t *testing.T) {
		if !artistMatches("Guest", "Main", []string{"Main", "Guest"}) {
			t.Error("expected later credits to be considered")
		}
	})

	t.Run("an empty main artist never matches (edge case)", func(t *testing.T) {
		// EqualFold("", "") is true, which would match every untagged library item.
		if artistMatches("", "", nil) {
			t.Error("an empty main artist must not match an empty album artist")
		}
		if artistMatches("", "Anyone", []string{"Anyone"}) {
			t.Error("an empty main artist must not match")
		}
	})

	t.Run("an unrelated artist does not match", func(t *testing.T) {
		if artistMatches("Radiohead", "Portishead", []string{"Massive Attack"}) {
			t.Error("unrelated artists must not match")
		}
	})
}

func TestTrackTitles(t *testing.T) {
	t.Run("a credit-only difference collapses to one title", func(t *testing.T) {
		track := &models.Track{CleanTitle: "Song", Title: "Song (feat. Guest)"}

		got := trackTitles(track)

		// Title normalizes to the same thing here, since the credit is stripped
		if len(got) != 1 || got[0] != "song" {
			t.Errorf("trackTitles = %v, want [song]", got)
		}
	})

	t.Run("distinct titles are both kept", func(t *testing.T) {
		track := &models.Track{CleanTitle: "Song", Title: "Song Remix"}

		got := trackTitles(track)

		if len(got) != 2 {
			t.Fatalf("trackTitles = %v, want two entries", got)
		}
	})

	t.Run("an empty title is skipped", func(t *testing.T) {
		track := &models.Track{CleanTitle: "Song"}

		if got := trackTitles(track); len(got) != 1 || got[0] != "song" {
			t.Errorf("trackTitles = %v, want [song]", got)
		}
	})

	t.Run("a track with no titles yields nothing", func(t *testing.T) {
		if got := trackTitles(&models.Track{}); len(got) != 0 {
			t.Errorf("trackTitles = %v, want empty", got)
		}
	})
}

func TestNamesAgree(t *testing.T) {
	t.Run("equal names agree, ignoring case and padding", func(t *testing.T) {
		if !namesAgree("  Radiohead ", "radiohead") {
			t.Error("expected equal names to agree")
		}
	})

	t.Run("a decorated name agrees with the plain one", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"OK Computer (Remastered)", "OK Computer"},
			{"Radiohead & Friends", "Radiohead"},
			{"Kid A - Deluxe Edition", "Kid A"},
			{"OK Computer", "OK Computer (Remastered)"}, // either side may be decorated
		} {
			if !namesAgree(pair[0], pair[1]) {
				t.Errorf("expected %q and %q to agree", pair[0], pair[1])
			}
		}
	})

	t.Run("a name continuing into another word does not agree", func(t *testing.T) {
		// This is the whole point of the word boundary. A plain substring or prefix
		// test accepts all of these, and combined with a title collision that is
		// enough to point a playlist at the wrong recording.
		for _, pair := range [][2]string{
			{"Ivory Tower", "IV"},
			{"Everything But The Girl", "Eve"},
			{"Airbourne", "Air"},
			{"Live at Wembley", "IV"},
		} {
			if namesAgree(pair[0], pair[1]) {
				t.Errorf("%q must not agree with %q", pair[0], pair[1])
			}
		}
	})

	t.Run("a name followed by a separator does agree", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"Beatles Tribute Band", "Beatles"},
			{"Air - Moon Safari", "Air"},
			{"Eve (Deluxe)", "Eve"},
		} {
			if !namesAgree(pair[0], pair[1]) {
				t.Errorf("expected %q and %q to agree at a word boundary", pair[0], pair[1])
			}
		}
	})

	t.Run("empty names never agree", func(t *testing.T) {
		if namesAgree("", "") || namesAgree("Radiohead", "") || namesAgree("", "Radiohead") {
			t.Error("empty names must not agree")
		}
	})
}

func TestDurationMatches(t *testing.T) {
	t.Run("close enough lengths match", func(t *testing.T) {
		if !durationMatches(200000, 205000) {
			t.Error("expected lengths within tolerance to match")
		}
	})

	t.Run("far apart lengths do not match", func(t *testing.T) {
		if durationMatches(30000, 260000) {
			t.Error("a 30s clip must not match a 260s track")
		}
	})

	t.Run("an unknown length is not a mismatch (edge case)", func(t *testing.T) {
		// Emby and Jellyfin searches do not report a length, so zero has to mean
		// "unknown" rather than "zero seconds".
		if !durationMatches(200000, 0) || !durationMatches(0, 200000) {
			t.Error("an unknown length must not block a match")
		}
	})
}

func TestMatchesTrack(t *testing.T) {
	track := func() *models.Track {
		return &models.Track{
			CleanTitle: "Karma Police",
			Title:      "Karma Police",
			MainArtist: "Radiohead",
			Album:      "OK Computer",
		}
	}

	t.Run("a MusicBrainz id matches on its own", func(t *testing.T) {
		// Nothing else lines up: different title, artist and album.
		tr := track()
		tr.MusicBrainzTrackID = "rec-1"

		item := libraryItem{Title: "Totally Different", Album: "Other", MusicBrainzIDs: []string{"rec-1"}}

		if !matchesTrack(tr, trackTitles(tr), item) {
			t.Error("expected the MusicBrainz id alone to be conclusive")
		}
	})

	t.Run("title plus album matches", func(t *testing.T) {
		tr := track()
		item := libraryItem{Title: "Karma Police", Album: "OK Computer (Remastered)"}

		if !matchesTrack(tr, trackTitles(tr), item) {
			t.Error("expected title plus album to match")
		}
	})

	t.Run("title plus artist matches", func(t *testing.T) {
		tr := track()
		item := libraryItem{Title: "Karma Police", Album: "Some Compilation", AlbumArtist: "Radiohead"}

		if !matchesTrack(tr, trackTitles(tr), item) {
			t.Error("expected title plus artist to match")
		}
	})

	t.Run("a title match alone is not enough", func(t *testing.T) {
		// Titles collide constantly across a library, so one on its own proves nothing.
		tr := track()
		item := libraryItem{Title: "Karma Police", Album: "Tribute Covers", AlbumArtist: "Other Band"}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("a title match with no corroboration must not count")
		}
	})

	t.Run("a normalized title still matches", func(t *testing.T) {
		tr := track()
		item := libraryItem{Title: "Karma Police (feat. Nobody) - 2011 Remaster", AlbumArtist: "Radiohead"}

		if !matchesTrack(tr, trackTitles(tr), item) {
			t.Error("expected annotations to be ignored when comparing titles")
		}
	})

	t.Run("an already downloaded file matches on artist and path", func(t *testing.T) {
		tr := track()
		tr.File = "01 - Karma Police.flac"
		item := libraryItem{
			Title:       "Completely Different Tagging",
			AlbumArtist: "Radiohead",
			Path:        "/music/Radiohead/OK Computer/01 - Karma Police.flac",
		}

		if !matchesTrack(tr, trackTitles(tr), item) {
			t.Error("expected the path fallback to match")
		}
	})

	t.Run("a path match without the artist is rejected", func(t *testing.T) {
		tr := track()
		tr.File = "01 - Karma Police.flac"
		item := libraryItem{Title: "Other", Path: "/music/Cover Band/01 - Karma Police.flac"}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("a path match needs the artist to agree")
		}
	})

	t.Run("an empty album does not count as an album match (edge case)", func(t *testing.T) {
		// ContainsFold(anything, "") is true, so an untagged album would otherwise
		// turn every title match into a full match.
		tr := track()
		tr.Album = ""
		item := libraryItem{Title: "Karma Police", Album: "Unrelated Album", AlbumArtist: "Wrong Band"}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("an empty album must not satisfy the album check")
		}
	})

	t.Run("an empty file does not count as a path match (edge case)", func(t *testing.T) {
		tr := track()
		tr.File = ""
		item := libraryItem{Title: "Different", AlbumArtist: "Radiohead", Path: "/music/anything.flac"}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("an empty file name must not satisfy the path check")
		}
	})

	t.Run("a short album name does not match an unrelated one that contains it", func(t *testing.T) {
		// "Live at Wembley" contains "iv" once punctuation is dropped. With a
		// substring album check this cover matches the real track.
		tr := track()
		tr.Album = "IV"
		tr.MainArtist = "Led Zeppelin"
		item := libraryItem{
			Title:       "Karma Police",
			Album:       "Live at Wembley",
			AlbumArtist: "Some Cover Band",
		}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("a coincidental album substring must not produce a match")
		}
	})

	t.Run("a coincidental artist substring among several credits does not match", func(t *testing.T) {
		tr := track()
		tr.MainArtist = "Eminem"
		item := libraryItem{
			Title:       "Karma Police",
			Album:       "Karaoke Classics",
			AlbumArtist: "Karaoke Band",
			Artists:     []string{"Karaoke Band", "Not Eminem Tribute"},
		}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("an artist name appearing mid-string must not produce a match")
		}
	})

	t.Run("a path match with a wildly different length is rejected", func(t *testing.T) {
		// Same artist and a matching file name, but a 30 second preview clip.
		tr := track()
		tr.File = "01 - Karma Police.flac"
		tr.Duration = 260000
		item := libraryItem{
			Title:       "Whatever",
			AlbumArtist: "Radiohead",
			Path:        "/music/previews/01 - Karma Police.flac",
			Duration:    30000,
		}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("a path match must also agree on length when both are known")
		}
	})

	t.Run("a track with no artist and no album never matches on title alone", func(t *testing.T) {
		// Nothing corroborates the title, so this must not match however tempting.
		tr := &models.Track{CleanTitle: "Karma Police", Title: "Karma Police"}
		item := libraryItem{Title: "Karma Police", Album: "Some Album", AlbumArtist: "Someone"}

		if matchesTrack(tr, trackTitles(tr), item) {
			t.Error("an untagged track must not match on a bare title")
		}
	})
}
