package client

import (
	"encoding/json"
	"testing"

	"explo/src/models"
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

		key, err := getPlexSong(track, results)
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

		key, err := getPlexSong(track, results)
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

		_, err := getPlexSong(track, results)
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

		_, err := getPlexSong(track, results)
		if err == nil {
			t.Fatalf("expected an error since the only result is type \"album\", not \"track\"")
		}
	})

	t.Run("empty search results return an error (edge case)", func(t *testing.T) {
		track := &models.Track{Title: "My Song"}
		results := mustParsePlexSearch(t, `{"MediaContainer": {"SearchResult": []}}`)

		_, err := getPlexSong(track, results)
		if err == nil {
			t.Fatalf("expected an error for empty search results")
		}
	})
}
