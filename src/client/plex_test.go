package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"explo/src/models"
	"explo/src/util"
)

func mustParsePlexSearch(t *testing.T, raw string) PlexSearch {
	t.Helper()
	var ps PlexSearch
	if err := json.Unmarshal([]byte(raw), &ps); err != nil {
		t.Fatalf("failed to unmarshal test fixture: %v", err)
	}
	return ps
}

func TestGetPlexSong(t *testing.T) {
	// No track in these cases carries a MusicBrainz id, so the metadata lookup that
	// would need an HTTP client is never reached.
	c := &Plex{}

	t.Run("matches via metadata (title + album)", func(t *testing.T) {
		track := &models.Track{
			Title:      "My Song",
			MainArtist: "My Artist",
			Album:      "My Album",
		}
		results := mustParsePlexSearch(t, `{
			"MediaContainer": {
				"SearchResult": [
					{
						"Metadata": {
							"type": "track",
							"key": "/library/metadata/123",
							"title": "My Song",
							"parentTitle": "My Album",
							"grandparentTitle": "My Artist"
						}
					}
				]
			}
		}`)

		key, err := c.getPlexSong(track, results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "/library/metadata/123" {
			t.Errorf("getPlexSong() = %q, want %q", key, "/library/metadata/123")
		}
	})

	t.Run("matches via file path and duration when metadata doesn't match", func(t *testing.T) {
		track := &models.Track{
			Title:      "Different Title In LB",
			MainArtist: "My Artist",
			Album:      "My Album",
			File:       "01 - My Song.flac",
			Duration:   200000, // 200s
		}
		results := mustParsePlexSearch(t, `{
			"MediaContainer": {
				"SearchResult": [
					{
						"Metadata": {
							"type": "track",
							"key": "/library/metadata/456",
							"title": "My Song",
							"parentTitle": "Some Other Album",
							"Media": [
								{
									"duration": 201000,
									"Part": [
										{"file": "/music/My Artist/My Album/01 - My Song.flac"}
									]
								}
							]
						}
					}
				]
			}
		}`)

		key, err := c.getPlexSong(track, results)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "/library/metadata/456" {
			t.Errorf("getPlexSong() = %q, want %q", key, "/library/metadata/456")
		}
	})

	t.Run("no match returns an error", func(t *testing.T) {
		track := &models.Track{
			Title:      "Unrelated Song",
			MainArtist: "Unrelated Artist",
			Album:      "Unrelated Album",
		}
		results := mustParsePlexSearch(t, `{
			"MediaContainer": {
				"SearchResult": [
					{
						"Metadata": {
							"type": "track",
							"key": "/library/metadata/789",
							"title": "My Song",
							"parentTitle": "My Album",
							"grandparentTitle": "My Artist"
						}
					}
				]
			}
		}`)

		_, err := c.getPlexSong(track, results)
		if err == nil {
			t.Fatalf("expected an error when no result matches, got nil")
		}
	})

	t.Run("non-track metadata types are skipped (edge case)", func(t *testing.T) {
		track := &models.Track{
			Title:      "My Song",
			MainArtist: "My Artist",
			Album:      "My Album",
		}
		results := mustParsePlexSearch(t, `{
			"MediaContainer": {
				"SearchResult": [
					{
						"Metadata": {
							"type": "album",
							"key": "/library/metadata/999",
							"title": "My Song",
							"parentTitle": "My Album",
							"grandparentTitle": "My Artist"
						}
					}
				]
			}
		}`)

		_, err := c.getPlexSong(track, results)
		if err == nil {
			t.Fatalf("expected an error since the only result is type \"album\", not \"track\"")
		}
	})

	t.Run("empty search results return an error (edge case)", func(t *testing.T) {
		track := &models.Track{Title: "My Song"}
		results := mustParsePlexSearch(t, `{"MediaContainer": {"SearchResult": []}}`)

		_, err := c.getPlexSong(track, results)
		if err == nil {
			t.Fatalf("expected an error for empty search results")
		}
	})
}

func TestPlexSongMBID(t *testing.T) {
	t.Run("reads the mbid guid and asks for guids to be included", func(t *testing.T) {
		var gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			if _, err := w.Write([]byte(`{"MediaContainer":{"Metadata":[{"Guid":[
				{"id":"imdb://tt123"},
				{"id":"mbid://rec-abc"}
			]}]}}`)); err != nil {
				t.Errorf("failed writing response: %v", err)
			}
		}))
		defer srv.Close()

		c := &Plex{HttpClient: util.NewHttp(util.HttpClientConfig{Timeout: 5})}
		c.Cfg.URL = srv.URL

		if got := c.songMBID("123"); got != "rec-abc" {
			t.Errorf("songMBID = %q, want %q", got, "rec-abc")
		}
		// Without this parameter Plex omits Guid entirely and the feature silently
		// never fires, with no error to show why.
		if !strings.Contains(gotQuery, "includeGuids=1") {
			t.Errorf("expected includeGuids=1 in the request, got query %q", gotQuery)
		}
	})

	t.Run("returns empty when Plex holds no mbid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte(`{"MediaContainer":{"Metadata":[{"Guid":[{"id":"imdb://tt123"}]}]}}`)); err != nil {
				t.Errorf("failed writing response: %v", err)
			}
		}))
		defer srv.Close()

		c := &Plex{HttpClient: util.NewHttp(util.HttpClientConfig{Timeout: 5})}
		c.Cfg.URL = srv.URL

		if got := c.songMBID("123"); got != "" {
			t.Errorf("songMBID = %q, want empty", got)
		}
	})

	t.Run("survives an empty rating key and a missing client", func(t *testing.T) {
		if got := (&Plex{}).songMBID(""); got != "" {
			t.Errorf("songMBID = %q, want empty", got)
		}
		// Must not panic: a nil client is reachable from tests and from any future
		// caller that builds a Plex without one.
		if got := (&Plex{}).songMBID("123"); got != "" {
			t.Errorf("songMBID = %q, want empty", got)
		}
	})
}

func TestGetPlexSongMatchesViaMBID(t *testing.T) {
	// Nothing about the metadata lines up, only the MusicBrainz id does.
	var lookups int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups++
		if _, err := w.Write([]byte(`{"MediaContainer":{"Metadata":[{"Guid":[{"id":"mbid://rec-abc"}]}]}}`)); err != nil {
			t.Errorf("failed writing response: %v", err)
		}
	}))
	defer srv.Close()

	c := &Plex{HttpClient: util.NewHttp(util.HttpClientConfig{Timeout: 5})}
	c.Cfg.URL = srv.URL

	track := &models.Track{
		CleanTitle:         "Locally Tagged Differently",
		MainArtist:         "Some Artist",
		Album:              "Some Album",
		MusicBrainzTrackID: "rec-abc",
	}
	results := mustParsePlexSearch(t, `{
		"MediaContainer": {
			"SearchResult": [
				{
					"Metadata": {
						"type": "track",
						"key": "/library/metadata/789",
						"ratingKey": "789",
						"title": "Totally Different Title",
						"parentTitle": "Unrelated Album",
						"grandparentTitle": "Unrelated Artist"
					}
				}
			]
		}
	}`)

	key, err := c.getPlexSong(track, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "/library/metadata/789" {
		t.Errorf("getPlexSong() = %q, want %q", key, "/library/metadata/789")
	}
	if lookups != 1 {
		t.Errorf("expected one metadata lookup, got %d", lookups)
	}
}

func TestGetPlexSongSkipsMBIDLookupsWhenMatchedCheaply(t *testing.T) {
	// Plex omits the MusicBrainz id from search results, so a lookup costs a request.
	// A track found by title and artist must not pay for one.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected metadata lookup for %s", r.URL)
	}))
	defer srv.Close()

	c := &Plex{HttpClient: util.NewHttp(util.HttpClientConfig{Timeout: 5})}
	c.Cfg.URL = srv.URL

	track := &models.Track{
		CleanTitle:         "My Song",
		MainArtist:         "My Artist",
		Album:              "My Album",
		MusicBrainzTrackID: "rec-abc",
	}
	results := mustParsePlexSearch(t, `{
		"MediaContainer": {
			"SearchResult": [
				{
					"Metadata": {
						"type": "track",
						"key": "/library/metadata/123",
						"ratingKey": "123",
						"title": "My Song",
						"parentTitle": "My Album",
						"grandparentTitle": "My Artist"
					}
				}
			]
		}
	}`)

	if _, err := c.getPlexSong(track, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
