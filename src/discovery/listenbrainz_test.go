package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

func TestBuildFeatTitle(t *testing.T) {
	tests := []struct {
		name        string
		cleanTitle  string
		featArtists []string
		want        string
	}{
		{
			name:       "no featured artists returns the title unchanged",
			cleanTitle: "Song",
			want:       "Song",
		},
		{
			name:        "single featured artist",
			cleanTitle:  "Song",
			featArtists: []string{"Guest"},
			want:        "Song (feat. Guest)",
		},
		{
			name:        "two featured artists are joined with an ampersand",
			cleanTitle:  "Song",
			featArtists: []string{"One", "Two"},
			want:        "Song (feat. One & Two)",
		},
		{
			name:        "three or more use commas then an ampersand",
			cleanTitle:  "Song",
			featArtists: []string{"One", "Two", "Three"},
			want:        "Song (feat. One, Two & Three)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFeatTitle(tt.cleanTitle, tt.featArtists); got != tt.want {
				t.Errorf("buildFeatTitle(%q, %v) = %q, want %q", tt.cleanTitle, tt.featArtists, got, tt.want)
			}
		})
	}
}

func TestTopTags(t *testing.T) {
	t.Run("orders by count descending", func(t *testing.T) {
		tags := []LBTag{{Tag: "rare", Count: 1}, {Tag: "common", Count: 50}, {Tag: "mid", Count: 10}}

		got := topTags(tags, 3)

		want := []string{"common", "mid", "rare"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("topTags = %v, want %v", got, want)
			}
		}
	})

	t.Run("ties break on tag name so runs are reproducible", func(t *testing.T) {
		tags := []LBTag{{Tag: "zebra", Count: 5}, {Tag: "alpha", Count: 5}}

		got := topTags(tags, 2)

		if got[0] != "alpha" || got[1] != "zebra" {
			t.Errorf("topTags = %v, want [alpha zebra]", got)
		}
	})

	t.Run("respects the limit", func(t *testing.T) {
		tags := []LBTag{{Tag: "a", Count: 3}, {Tag: "b", Count: 2}, {Tag: "c", Count: 1}}

		if got := topTags(tags, 2); len(got) != 2 {
			t.Errorf("expected 2 tags, got %v", got)
		}
	})

	t.Run("limit larger than input is clamped (edge case)", func(t *testing.T) {
		tags := []LBTag{{Tag: "only", Count: 1}}

		if got := topTags(tags, 10); len(got) != 1 {
			t.Errorf("expected 1 tag, got %v", got)
		}
	})

	t.Run("empty input and non-positive limit return nothing", func(t *testing.T) {
		if got := topTags(nil, 3); got != nil {
			t.Errorf("expected nil for empty input, got %v", got)
		}
		if got := topTags([]LBTag{{Tag: "a", Count: 1}}, 0); got != nil {
			t.Errorf("expected nil for zero limit, got %v", got)
		}
	})

	t.Run("blank tag names are dropped (edge case)", func(t *testing.T) {
		tags := []LBTag{{Tag: "", Count: 99}, {Tag: "real", Count: 1}}

		got := topTags(tags, 2)

		if len(got) != 1 || got[0] != "real" {
			t.Errorf("expected blank tags to be dropped, got %v", got)
		}
	})

	t.Run("does not reorder the caller's slice", func(t *testing.T) {
		tags := []LBTag{{Tag: "low", Count: 1}, {Tag: "high", Count: 9}}

		topTags(tags, 2)

		if tags[0].Tag != "low" {
			t.Errorf("input slice was reordered: %v", tags)
		}
	})
}

func TestReleaseMBID(t *testing.T) {
	t.Run("prefers the release mbid", func(t *testing.T) {
		rel := ReleaseMetadata{Mbid: "release", CaaReleaseMbid: "cover-release"}

		if got := releaseMBID(rel); got != "release" {
			t.Errorf("releaseMBID = %q, want %q", got, "release")
		}
	})

	t.Run("falls back to the cover art release when the release mbid is absent", func(t *testing.T) {
		rel := ReleaseMetadata{CaaReleaseMbid: "cover-release"}

		if got := releaseMBID(rel); got != "cover-release" {
			t.Errorf("releaseMBID = %q, want %q", got, "cover-release")
		}
	})

	t.Run("returns empty when neither is set", func(t *testing.T) {
		if got := releaseMBID(ReleaseMetadata{}); got != "" {
			t.Errorf("releaseMBID = %q, want empty", got)
		}
	})
}

func TestCoverArtURL(t *testing.T) {
	t.Run("uses the configured size", func(t *testing.T) {
		c := &ListenBrainz{cfg: cfg.Listenbrainz{CoverArtSize: "500"}}

		want := "https://coverartarchive.org/release/rel-mbid/front-500"
		if got := c.coverArtURL("rel-mbid"); got != want {
			t.Errorf("coverArtURL = %q, want %q", got, want)
		}
	})

	t.Run("falls back to 250 when unset (edge case)", func(t *testing.T) {
		c := &ListenBrainz{}

		want := "https://coverartarchive.org/release/rel-mbid/front-250"
		if got := c.coverArtURL("rel-mbid"); got != want {
			t.Errorf("coverArtURL = %q, want %q", got, want)
		}
	})

	t.Run("empty mbid yields no url", func(t *testing.T) {
		c := &ListenBrainz{}

		if got := c.coverArtURL(""); got != "" {
			t.Errorf("coverArtURL = %q, want empty", got)
		}
	})
}

func TestApplyLBMetadata(t *testing.T) {
	c := &ListenBrainz{}

	t.Run("an empty response does not blank what the track already had", func(t *testing.T) {
		// Recordings that aren't linked to a canonical release come back with an
		// empty release block. Keeping the playlist's own values beats enriching
		// the track into blanks.
		track := &models.Track{
			CleanTitle: "Known Title",
			Title:      "Known Title",
			Album:      "Known Album",
			Artist:     "Known Artist",
			MainArtist: "Known Artist",
		}

		c.applyLBMetadata(track, Metadata{}, true)

		if track.CleanTitle != "Known Title" || track.Title != "Known Title" {
			t.Errorf("title was blanked: %q / %q", track.CleanTitle, track.Title)
		}
		if track.Album != "Known Album" {
			t.Errorf("album was blanked: %q", track.Album)
		}
		if track.Artist != "Known Artist" || track.MainArtist != "Known Artist" {
			t.Errorf("artist was blanked: %q / %q", track.Artist, track.MainArtist)
		}
	})

	t.Run("an empty response does not blank date, year or genres (edge case)", func(t *testing.T) {
		track := &models.Track{
			OriginalDate: "1990-01-01",
			OriginalYear: 1990,
			Genres:       "shoegaze",
		}

		c.applyLBMetadata(track, Metadata{}, true)

		if track.OriginalDate != "1990-01-01" {
			t.Errorf("OriginalDate was blanked: %q", track.OriginalDate)
		}
		if track.OriginalYear != 1990 {
			t.Errorf("OriginalYear was zeroed: %d", track.OriginalYear)
		}
		if track.Genres != "shoegaze" {
			t.Errorf("Genres was blanked: %q", track.Genres)
		}
	})

	t.Run("an empty artist mbid does not clear an existing one (edge case)", func(t *testing.T) {
		track := &models.Track{MusicBrainzArtistID: "known-artist-mbid"}

		meta := Metadata{}
		meta.Artist.Artists = []LBArtist{{Name: "Artist", ArtistMbid: ""}}

		c.applyLBMetadata(track, meta, true)

		if track.MusicBrainzArtistID != "known-artist-mbid" {
			t.Errorf("MusicBrainzArtistID was cleared: %q", track.MusicBrainzArtistID)
		}
	})

	t.Run("populated fields are copied onto the track", func(t *testing.T) {
		meta := Metadata{}
		meta.Recording.Name = "Enriched Title"
		meta.Recording.ISRCs = []string{"GBAYE9700263"}
		meta.Recording.FirstReleaseDate = "1997-06-16"
		meta.Artist.Name = "Enriched Artist"
		meta.Artist.Artists = []LBArtist{{Name: "Enriched Artist", ArtistMbid: "artist-mbid"}}
		meta.Release = ReleaseMetadata{
			Name:             "Enriched Album",
			AlbumArtistName:  "Enriched Album Artist",
			Mbid:             "release-mbid",
			ReleaseGroupMbid: "rg-mbid",
		}
		meta.Tag.Recording = []LBTag{{Tag: "rock", Count: 10}}

		track := &models.Track{}
		c.applyLBMetadata(track, meta, true)

		if track.Title != "Enriched Title" || track.Album != "Enriched Album" {
			t.Errorf("title/album not applied: %q / %q", track.Title, track.Album)
		}
		if track.MusicBrainzAlbumID != "release-mbid" {
			t.Errorf("MusicBrainzAlbumID = %q, want release-mbid", track.MusicBrainzAlbumID)
		}
		if track.Genres != "rock" {
			t.Errorf("Genres = %q, want rock", track.Genres)
		}
		if track.OriginalYear != 1997 {
			t.Errorf("OriginalYear = %d, want 1997 (derived from first release date)", track.OriginalYear)
		}
		if len(track.ISRCs) != 1 || track.ISRCs[0] != "GBAYE9700263" {
			t.Errorf("ISRCs = %v", track.ISRCs)
		}
	})

	t.Run("cover art uses the release that actually has artwork", func(t *testing.T) {
		// caa_release_mbid is the release ListenBrainz confirmed has an image, the
		// canonical release may have none, so the URL must not be built from it.
		meta := Metadata{Release: ReleaseMetadata{Mbid: "canonical", CaaReleaseMbid: "has-art"}}
		track := &models.Track{}

		c.applyLBMetadata(track, meta, true)

		want := "https://coverartarchive.org/release/has-art/front-250"
		if track.CoverURL != want {
			t.Errorf("CoverURL = %q, want %q", track.CoverURL, want)
		}
		if track.MusicBrainzAlbumID != "canonical" {
			t.Errorf("MusicBrainzAlbumID = %q, want the canonical release", track.MusicBrainzAlbumID)
		}
	})

	t.Run("featured artists are folded into the title when singleArtist is set", func(t *testing.T) {
		meta := Metadata{}
		meta.Recording.Name = "Song"
		meta.Artist.Name = "Main & Guest"
		meta.Artist.Artists = []LBArtist{{Name: "Main"}, {Name: "Guest"}}

		track := &models.Track{}
		c.applyLBMetadata(track, meta, true)

		if track.Title != "Song (feat. Guest)" {
			t.Errorf("Title = %q, want %q", track.Title, "Song (feat. Guest)")
		}
		if track.Artist != "Main" {
			t.Errorf("Artist = %q, want Main", track.Artist)
		}
		if track.Artists != nil {
			t.Errorf("Artists should stay unset when collapsing to a single artist, got %v", track.Artists)
		}
	})

	t.Run("individual credits are kept when singleArtist is not set", func(t *testing.T) {
		meta := Metadata{}
		meta.Recording.Name = "Song"
		meta.Artist.Artists = []LBArtist{{Name: "Main"}, {Name: "Guest"}}

		track := &models.Track{}
		c.applyLBMetadata(track, meta, false)

		if track.Title != "Song" {
			t.Errorf("Title = %q, want the plain title", track.Title)
		}
		if len(track.Artists) != 2 || track.Artists[0] != "Main" || track.Artists[1] != "Guest" {
			t.Errorf("Artists = %v, want [Main Guest]", track.Artists)
		}
	})
}

// mbRelease builds an MBRecording carrying a single release, for applyMBMetadata tests.
func mbRelease(id string, discPosition, trackCount int, trackID string, trackPosition int) MBRecording {
	return MBRecording{
		Releases: []MBRelease{
			{
				ID:      id,
				Country: "GB",
				Status:  "Official",
				ReleaseGroup: MBReleaseGroup{
					ID:          "rg-" + id,
					PrimaryType: "Album",
				},
				Media: []MBMedia{
					{
						Position:   discPosition,
						Format:     "CD",
						TrackCount: trackCount,
						Tracks:     []MBTrack{{ID: trackID, Position: trackPosition}},
					},
				},
			},
		},
	}
}

func TestApplyMBMetadata(t *testing.T) {
	t.Run("nil and empty inputs leave the track untouched", func(t *testing.T) {
		track := &models.Track{Title: "unchanged"}

		applyMBMetadata(track, nil)
		applyMBMetadata(track, &MBRecording{})

		if track.Title != "unchanged" || track.Media != "" || track.TrackNumber != 0 {
			t.Errorf("track was modified: %+v", track)
		}
	})

	t.Run("copies release and track details", func(t *testing.T) {
		mb := mbRelease("rel-1", 1, 12, "reltrack-1", 3)
		track := &models.Track{}

		applyMBMetadata(track, &mb)

		if track.ReleaseCountry != "GB" {
			t.Errorf("ReleaseCountry = %q, want GB", track.ReleaseCountry)
		}
		if track.ReleaseStatus != "Official" {
			t.Errorf("ReleaseStatus = %q, want Official", track.ReleaseStatus)
		}
		if track.ReleaseType != "Album" {
			t.Errorf("ReleaseType = %q, want Album", track.ReleaseType)
		}
		if track.Media != "CD" {
			t.Errorf("Media = %q, want CD", track.Media)
		}
		if track.TrackNumber != 3 || track.TrackTotal != 12 || track.DiscNumber != 1 {
			t.Errorf("positions wrong: track %d/%d disc %d", track.TrackNumber, track.TrackTotal, track.DiscNumber)
		}
		if track.MusicBrainzReleaseTrackID != "reltrack-1" {
			t.Errorf("MusicBrainzReleaseTrackID = %q", track.MusicBrainzReleaseTrackID)
		}
	})

	t.Run("DiscTotal is never guessed from the returned media", func(t *testing.T) {
		// A recording lookup only returns the media holding that recording, so a
		// track on disc 1 of a 2-disc release still comes back with one medium.
		// Writing "1 of 1" there would be wrong, so DiscTotal must stay unset.
		mb := mbRelease("rel-1", 1, 12, "reltrack-1", 3)
		track := &models.Track{}

		applyMBMetadata(track, &mb)

		if track.DiscTotal != 0 {
			t.Errorf("DiscTotal = %d, want 0 (not derivable from a recording lookup)", track.DiscTotal)
		}
	})

	t.Run("prefers the release already chosen by ListenBrainz", func(t *testing.T) {
		first := mbRelease("rel-other", 1, 5, "reltrack-other", 1)
		wanted := mbRelease("rel-wanted", 2, 20, "reltrack-wanted", 7)
		first.Releases = append(first.Releases, wanted.Releases...)

		track := &models.Track{MusicBrainzAlbumID: "rel-wanted"}
		applyMBMetadata(track, &first)

		if track.TrackNumber != 7 || track.TrackTotal != 20 || track.DiscNumber != 2 {
			t.Errorf("expected the matching release to win, got track %d/%d disc %d",
				track.TrackNumber, track.TrackTotal, track.DiscNumber)
		}
	})

	t.Run("falls back to the first release when none matches", func(t *testing.T) {
		mb := mbRelease("rel-1", 1, 12, "reltrack-1", 3)
		track := &models.Track{MusicBrainzAlbumID: "not-present"}

		applyMBMetadata(track, &mb)

		if track.TrackNumber != 3 {
			t.Errorf("expected fallback to the first release, got track %d", track.TrackNumber)
		}
	})

	t.Run("blank MusicBrainz values do not clear existing ones (edge case)", func(t *testing.T) {
		mb := mbRelease("rel-1", 1, 12, "reltrack-1", 3)
		mb.Releases[0].Country = ""
		mb.Releases[0].Status = ""

		track := &models.Track{ReleaseCountry: "US", ReleaseStatus: "Official"}
		applyMBMetadata(track, &mb)

		if track.ReleaseCountry != "US" || track.ReleaseStatus != "Official" {
			t.Errorf("existing values were cleared: country %q status %q", track.ReleaseCountry, track.ReleaseStatus)
		}
	})
}

func TestListenBrainzUserToken(t *testing.T) {
	t.Run("a token becomes an Authorization header", func(t *testing.T) {
		c := NewListenBrainz(cfg.DiscoveryConfig{
			Listenbrainz: cfg.Listenbrainz{UserToken: "secret-token"},
		}, nil)

		if got := c.Headers["Authorization"]; got != "Token secret-token" {
			t.Errorf("Authorization = %q, want %q", got, "Token secret-token")
		}
	})

	t.Run("no token means no headers at all", func(t *testing.T) {
		c := NewListenBrainz(cfg.DiscoveryConfig{}, nil)

		if len(c.Headers) != 0 {
			t.Errorf("expected no headers without a token, got %v", c.Headers)
		}
	})

	t.Run("the token is only sent to ListenBrainz", func(t *testing.T) {
		// The token authenticates the user's ListenBrainz account. Sending it to
		// MusicBrainz or the Cover Art Archive would hand a third party a credential.
		var lbAuth, mbAuth string

		lb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lbAuth = r.Header.Get("Authorization")
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("failed writing response: %v", err)
			}
		}))
		defer lb.Close()

		mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mbAuth = r.Header.Get("Authorization")
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("failed writing response: %v", err)
			}
		}))
		defer mb.Close()

		c := NewListenBrainz(cfg.DiscoveryConfig{
			Listenbrainz: cfg.Listenbrainz{UserToken: "secret-token"},
		}, util.NewHttp(util.HttpClientConfig{Timeout: 5}))

		if _, err := c.HttpClient.MakeRequest("GET", lb.URL, nil, c.Headers); err != nil {
			t.Fatalf("listenbrainz request failed: %v", err)
		}
		// mbRequest passes nil headers, mirror that here
		if _, err := c.HttpClient.MakeRequest("GET", mb.URL, nil, nil); err != nil {
			t.Fatalf("musicbrainz request failed: %v", err)
		}

		if lbAuth != "Token secret-token" {
			t.Errorf("ListenBrainz did not receive the token, got %q", lbAuth)
		}
		if mbAuth != "" {
			t.Errorf("token leaked to MusicBrainz: %q", mbAuth)
		}
	})
}
