package discovery

import (
	"log/slog"
	"strings"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type DiscoverClient struct {
	cfg       *cfg.DiscoveryConfig
	Discovery Discovery
}
type Discovery interface {
	QueryTracks() ([]*models.Track, error)
}

func NewDiscoverer(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *DiscoverClient {
	c := &DiscoverClient{cfg: &cfg}

	switch cfg.Discovery {
	case "listenbrainz":
		c.Discovery = NewListenBrainz(cfg, httpClient)
	default:
		return nil
	}
	return c
}

func (c *DiscoverClient) Discover() ([]*models.Track, error) {
	tracks, err := c.Discovery.QueryTracks()
	if err != nil {
		return nil, err
	}

	return c.filterArtists(tracks), nil
}

// filterArtists drops tracks by blacklisted artists. Entries are matched against
// both the artist name and the MusicBrainz artist ID, so either can be configured.
// Matching is case-insensitive on both.
func (c *DiscoverClient) filterArtists(tracks []*models.Track) []*models.Track {
	if len(c.cfg.ArtistBlacklist) == 0 {
		return tracks
	}

	blacklist := make(map[string]struct{}, len(c.cfg.ArtistBlacklist))
	for _, artist := range c.cfg.ArtistBlacklist {
		if artist = strings.TrimSpace(artist); artist != "" {
			blacklist[strings.ToLower(artist)] = struct{}{}
		}
	}
	if len(blacklist) == 0 {
		return tracks
	}

	filtered := make([]*models.Track, 0, len(tracks))
	for _, track := range tracks {
		_, blockedName := blacklist[strings.ToLower(track.MainArtist)]
		_, blockedMBID := blacklist[strings.ToLower(track.MusicBrainzArtistID)]

		if blockedName || blockedMBID {
			slog.Debug("filtered out blacklisted artist",
				"name", track.MainArtist,
				"mbid", track.MusicBrainzArtistID,
				"track", track.CleanTitle,
			)
			continue
		}

		filtered = append(filtered, track)
	}

	return filtered
}
