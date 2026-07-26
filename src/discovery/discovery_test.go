package discovery

import (
	"testing"

	cfg "explo/src/config"
	"explo/src/models"
)

func TestFilterArtists(t *testing.T) {
	newTracks := func() []*models.Track {
		return []*models.Track{
			{CleanTitle: "one", MainArtist: "Blocked Artist", MusicBrainzArtistID: "mbid-blocked"},
			{CleanTitle: "two", MainArtist: "Allowed Artist", MusicBrainzArtistID: "mbid-allowed"},
		}
	}

	newClient := func(blacklist []string) *DiscoverClient {
		return &DiscoverClient{cfg: &cfg.DiscoveryConfig{ArtistBlacklist: blacklist}}
	}

	t.Run("an empty blacklist keeps everything", func(t *testing.T) {
		tracks := newTracks()

		if got := newClient(nil).filterArtists(tracks); len(got) != 2 {
			t.Errorf("expected both tracks, got %d", len(got))
		}
	})

	t.Run("blocks by artist name", func(t *testing.T) {
		got := newClient([]string{"Blocked Artist"}).filterArtists(newTracks())

		if len(got) != 1 || got[0].MainArtist != "Allowed Artist" {
			t.Errorf("expected only the allowed artist, got %+v", got)
		}
	})

	t.Run("blocks by MusicBrainz artist id", func(t *testing.T) {
		got := newClient([]string{"mbid-blocked"}).filterArtists(newTracks())

		if len(got) != 1 || got[0].MainArtist != "Allowed Artist" {
			t.Errorf("expected only the allowed artist, got %+v", got)
		}
	})

	t.Run("names are matched case insensitively", func(t *testing.T) {
		got := newClient([]string{"blocked artist"}).filterArtists(newTracks())

		if len(got) != 1 {
			t.Errorf("expected the blacklisted artist to be dropped, got %+v", got)
		}
	})

	t.Run("surrounding whitespace in an entry is ignored", func(t *testing.T) {
		got := newClient([]string{"  Blocked Artist  "}).filterArtists(newTracks())

		if len(got) != 1 {
			t.Errorf("expected the blacklisted artist to be dropped, got %+v", got)
		}
	})

	t.Run("a blank entry does not block untagged tracks (edge case)", func(t *testing.T) {
		// A trailing comma in ARTIST_BLACKLIST yields an empty entry. It must not
		// match tracks whose artist name or MBID happens to be empty.
		tracks := []*models.Track{{CleanTitle: "untagged", MainArtist: "", MusicBrainzArtistID: ""}}

		if got := newClient([]string{""}).filterArtists(tracks); len(got) != 1 {
			t.Errorf("an empty blacklist entry dropped a track: %+v", got)
		}
	})

	t.Run("does not modify the caller's slice", func(t *testing.T) {
		tracks := newTracks()

		newClient([]string{"Blocked Artist"}).filterArtists(tracks)

		if tracks[0].CleanTitle != "one" || tracks[1].CleanTitle != "two" {
			t.Errorf("input slice was rewritten: %+v", tracks)
		}
	})

	t.Run("blocking every artist yields an empty result", func(t *testing.T) {
		got := newClient([]string{"Blocked Artist", "Allowed Artist"}).filterArtists(newTracks())

		if len(got) != 0 {
			t.Errorf("expected no tracks, got %+v", got)
		}
	})
}
