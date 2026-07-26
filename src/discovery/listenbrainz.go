package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"

	"golang.org/x/time/rate"
)

type Recommendations struct {
	Payload struct {
		Count       int    `json:"count"`
		Entity      string `json:"entity"`
		LastUpdated int    `json:"last_updated"`
		Mbids       []struct {
			LatestListenedAt time.Time `json:"latest_listened_at"`
			RecordingMbid    string    `json:"recording_mbid"`
			Score            float64   `json:"score"`
		} `json:"mbids"`
		TotalMbidCount int    `json:"total_mbid_count"`
		UserName       string `json:"user_name"`
	} `json:"payload"`
}

type Metadata struct {
	Tag struct {
		Artist       []LBTag `json:"artist"`
		Recording    []LBTag `json:"recording"`
		ReleaseGroup []LBTag `json:"release_group"`
	} `json:"tag"`
	Artist    ArtistMetadata `json:"artist"`
	Recording struct {
		FirstReleaseDate string   `json:"first_release_date"`
		ISRCs            []string `json:"isrcs"`
		Length           int      `json:"length"`
		Name             string   `json:"name"`
		Rels             []any    `json:"rels"`
	} `json:"recording"`
	Release ReleaseMetadata `json:"release"`
}

type ReleaseMetadata struct {
	AlbumArtistName  string `json:"album_artist_name"`
	CaaID            int64  `json:"caa_id"`
	CaaReleaseMbid   string `json:"caa_release_mbid"`
	Status           string `json:"status"`
	Mbid             string `json:"mbid"`
	Name             string `json:"name"`
	ReleaseGroupMbid string `json:"release_group_mbid"`
	Year             int    `json:"year"`
}

type ArtistMetadata struct {
	ArtistCreditID int        `json:"artist_credit_id"`
	Artists        []LBArtist `json:"artists"`
	Name           string     `json:"name"`
}

type LBArtist struct {
	ArtistMbid string `json:"artist_mbid"`
	BeginYear  int    `json:"begin_year"`
	EndYear    int    `json:"end_year,omitempty"`
	JoinPhrase string `json:"join_phrase"`
	Name       string `json:"name"`
}

// LBTag is a folksonomy tag with the number of users that applied it.
type LBTag struct {
	Count int    `json:"count"`
	Tag   string `json:"tag"`
}

// MBRecording is the subset of a MusicBrainz recording lookup that carries the
// per-release details ListenBrainz doesn't expose: track/disc positions, media
// format, release country and the release-track MBID.
type MBRecording struct {
	ID       string      `json:"id"`
	Releases []MBRelease `json:"releases"`
}

type MBRelease struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Status       string           `json:"status"`
	Country      string           `json:"country"`
	Date         string           `json:"date"`
	ArtistCredit []MBArtistCredit `json:"artist-credit"`
	ReleaseGroup MBReleaseGroup   `json:"release-group"`
	Media        []MBMedia        `json:"media"`
}

type MBArtistCredit struct {
	Name   string `json:"name"`
	Artist struct {
		ID       string `json:"id"`
		SortName string `json:"sort-name"`
	} `json:"artist"`
}

type MBReleaseGroup struct {
	ID          string `json:"id"`
	PrimaryType string `json:"primary-type"`
}

type MBMedia struct {
	Position    int       `json:"position"`
	Format      string    `json:"format"`
	TrackCount  int       `json:"track-count"`
	TrackOffset int       `json:"track-offset"`
	Tracks      []MBTrack `json:"tracks"`
}

type MBTrack struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Number   string `json:"number"`
}

type Recordings map[string]Metadata

type CreatedFor struct {
	Count         int `json:"count"`
	Offset        int `json:"offset"`
	PlaylistCount int `json:"playlist_count"`
	Playlists     []struct {
		Playlist struct {
			Creator   string    `json:"creator"`
			Date      time.Time `json:"date"`
			Extension struct {
				HTTPSJspfPlaylist struct {
					AdditionalMetadata struct {
						AlgorithmMetadata struct {
							SourcePatch string `json:"source_patch"`
						} `json:"algorithm_metadata"`
					} `json:"additional_metadata"`
					CreatedFor string `json:"created_for"`
				} `json:"https://musicbrainz.org/doc/jspf#playlist"`
			} `json:"extension"`
			Identifier string `json:"identifier"`
		} `json:"playlist"`
	} `json:"playlists"`
}

type Exploration struct {
	Playlist struct {
		Annotation string    `json:"annotation"`
		Creator    string    `json:"creator"`
		Date       time.Time `json:"date"`
		Identifier string    `json:"identifier"`
		Title      string    `json:"title"`
		Tracks     []struct {
			Album     string `json:"album"`
			Creator   string `json:"creator"`
			Duration  int    `json:"duration"`
			Extension struct {
				HTTPSJspfTrack struct {
					AddedAt            time.Time `json:"added_at"`
					AddedBy            string    `json:"added_by"`
					AdditionalMetadata struct {
						Artists []struct {
							ArtistCreditName string `json:"artist_credit_name"`
							ArtistMbid       string `json:"artist_mbid"`
							JoinPhrase       string `json:"join_phrase"`
						} `json:"artists"`
						CaaID          int64  `json:"caa_id"`
						CaaReleaseMbid string `json:"caa_release_mbid"`
					} `json:"additional_metadata"`
					ArtistIdentifiers []string `json:"artist_identifiers"`
				} `json:"https://musicbrainz.org/doc/jspf#track"`
			} `json:"extension"`
			Identifier []string `json:"identifier"`
			Title      string   `json:"title"`
		} `json:"track"`
	} `json:"playlist"`
}

type TopRecordings struct {
	Payload struct {
		Recordings []struct {
			ArtistName    string `json:"artist_name"`
			RecordingMbid string `json:"recording_mbid"`
			ReleaseMbid   string `json:"release_mbid"`
			ReleaseName   string `json:"release_name"`
			TrackName     string `json:"track_name"`
		} `json:"recordings"`
	} `json:"payload"`
}

type ListenBrainz struct {
	HttpClient *util.HttpClient
	Headers    map[string]string
	cfg        cfg.Listenbrainz
	Separator  string
}

func NewListenBrainz(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *ListenBrainz {
	// A user token authenticates the request, which raises the rate limit and is
	// required for anything reading a user's private data.
	var headers map[string]string
	if cfg.Listenbrainz.UserToken != "" {
		headers = map[string]string{
			"Authorization": fmt.Sprintf("Token %s", cfg.Listenbrainz.UserToken),
		}
	}

	return &ListenBrainz{
		cfg:        cfg.Listenbrainz,
		Headers:    headers,
		HttpClient: httpClient,
	}
}
func (c *ListenBrainz) QueryTracks() ([]*models.Track, error) {
	tracks, err := c.queryPlaylistTracks()
	if err != nil {
		return nil, err
	}

	if c.cfg.EnrichTrackMetadata && len(tracks) > 0 {
		if err := c.enrichTracks(tracks, c.cfg.SingleArtist); err != nil {
			// Enrichment is additive, the tracks are still usable without it
			slog.Warn("failed to enrich playlist metadata", "error", err.Error())
		}
	}

	return tracks, nil
}

func (c *ListenBrainz) queryPlaylistTracks() ([]*models.Track, error) {
	// Stats-based playlists bypass the discovery mode switch
	if c.cfg.ImportPlaylist == "on-repeat" {
		return c.getTopRecordings(c.cfg.User)
	}

	switch c.cfg.Discovery {
	case "playlist":
		id, err := c.getImportPlaylist(c.cfg.User)
		if err != nil {
			return nil, err
		}
		return c.parsePlaylist(id, c.cfg.SingleArtist)

	default:
		mbids, err := c.getAPIRecommendations(c.cfg.User)
		if err != nil {
			return nil, err
		}
		return c.getTracks(mbids, c.cfg.SingleArtist)
	}
}

// buildFeatTitle appends the featured artists to a title, in the parenthesised form
// music libraries and tagging tools use: "Song (feat. A, B & C)". featArtists must
// not include the main artist.
func buildFeatTitle(cleanTitle string, featArtists []string) string {
	if len(featArtists) == 0 {
		return cleanTitle
	}

	var b strings.Builder
	b.WriteString(cleanTitle)
	b.WriteString(" (feat. ")

	if len(featArtists) == 1 {
		b.WriteString(featArtists[0])
	} else {
		b.WriteString(strings.Join(featArtists[:len(featArtists)-1], ", "))
		b.WriteString(" & ")
		b.WriteString(featArtists[len(featArtists)-1])
	}

	b.WriteString(")")

	return b.String()
}

// coverArtURL builds a Cover Art Archive front-cover URL for a release MBID.
func (c *ListenBrainz) coverArtURL(releaseMBID string) string {
	if releaseMBID == "" {
		return ""
	}
	return fmt.Sprintf("https://coverartarchive.org/release/%s/front-%s", releaseMBID, c.coverArtSize())
}

func (c *ListenBrainz) coverArtSize() string {
	if c.cfg.CoverArtSize == "" {
		return "250"
	}
	return c.cfg.CoverArtSize
}

func (c *ListenBrainz) getAPIRecommendations(user string) ([]string, error) {
	var mbids []string

	body, err := c.lbRequest(fmt.Sprintf("cf/recommendation/user/%s/recording", user))
	if err != nil {
		return mbids, fmt.Errorf("could not get recommendations from API: %s", err.Error())
	}

	var reccs Recommendations
	err = util.ParseResp(body, &reccs)
	if err != nil {
		return mbids, fmt.Errorf("could not get recommendations from API: %s", err.Error())
	}

	for _, rec := range reccs.Payload.Mbids {
		mbids = append(mbids, rec.RecordingMbid)
	}

	if len(mbids) == 0 {
		return mbids, fmt.Errorf("no recommendations found, exiting")
	}
	return mbids, nil
}

func (c *ListenBrainz) getTopRecordings(user string) ([]*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("stats/user/%s/recordings?count=30&range=month", user))
	if err != nil {
		return nil, fmt.Errorf("getTopRecordings(): %s", err.Error())
	}

	var resp TopRecordings
	if err := util.ParseResp(body, &resp); err != nil {
		return nil, fmt.Errorf("getTopRecordings(): %s", err.Error())
	}

	if len(resp.Payload.Recordings) == 0 {
		return nil, fmt.Errorf("no top recordings found for user %s", user)
	}

	tracks := make([]*models.Track, 0, len(resp.Payload.Recordings))
	for _, rec := range resp.Payload.Recordings {
		tracks = append(tracks, &models.Track{
			Title:              rec.TrackName,
			CleanTitle:         rec.TrackName,
			Artist:             rec.ArtistName,
			MainArtist:         rec.ArtistName,
			Album:              rec.ReleaseName,
			CoverURL:           c.coverArtURL(rec.ReleaseMbid),
			MusicBrainzTrackID: rec.RecordingMbid,
			MusicBrainzAlbumID: rec.ReleaseMbid,
		})
	}

	return tracks, nil
}

// LookupRecording resolves a single MusicBrainz recording ID into a track. Used by
// --search-mbid to check what Explo would look for in the library.
func (c *ListenBrainz) LookupRecording(mbid string) (*models.Track, error) {
	// Uses the configured artist handling, so the title it reports is the one a real
	// run would look for rather than a differently built one.
	tracks, err := c.getTracks([]string{mbid}, c.cfg.SingleArtist)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("ListenBrainz returned no recording for %s", mbid)
	}
	return tracks[0], nil
}

func (c *ListenBrainz) getTracks(mbids []string, singleArtist bool) ([]*models.Track, error) {
	strMbids := strings.Join(mbids, ",")

	body, err := c.lbRequest(fmt.Sprintf("metadata/recording/?recording_mbids=%s&inc=release+artist", strMbids))
	if err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	var recordings Recordings
	if err := util.ParseResp(body, &recordings); err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	if len(recordings) == 0 {
		return nil, fmt.Errorf("no recordings found for MBIDs: %s", strMbids)
	}

	tracks := make([]*models.Track, 0, len(recordings))
	for mbTrackID, recording := range recordings {
		rec := recording.Recording
		rel := recording.Release

		title := rec.Name
		artist := recording.Artist.Name
		mainArtist := recording.Artist.Name

		recArtists := recording.Artist.Artists

		if len(recArtists) > 1 {
			mainArtist = recArtists[0].Name
			if singleArtist {
				featArtists := make([]string, 0, len(recArtists)-1)
				for _, a := range recArtists[1:] {
					featArtists = append(featArtists, a.Name)
				}
				title = buildFeatTitle(rec.Name, featArtists)
				artist = mainArtist
			}
		}

		artistMBID := ""
		if len(recArtists) > 0 {
			artistMBID = recArtists[0].ArtistMbid
		}

		tracks = append(tracks, &models.Track{
			Album:                     rel.Name,
			AlbumArtist:               rel.AlbumArtistName,
			Artist:                    artist,
			MainArtist:                mainArtist,
			CleanTitle:                rec.Name,
			Title:                     title,
			Duration:                  rec.Length,
			CoverURL:                  c.coverArtURL(rel.CaaReleaseMbid),
			MusicBrainzTrackID:        mbTrackID,
			MusicBrainzAlbumID:        releaseMBID(rel),
			MusicBrainzReleaseGroupID: rel.ReleaseGroupMbid,
			MusicBrainzArtistID:       artistMBID,
		})
	}

	return tracks, nil

}

// releaseMBID returns the release MBID, preferring the release itself over the
// release the cover art was taken from. The two usually match, but caa_release_mbid
// can point at a different release in the same group.
func releaseMBID(rel ReleaseMetadata) string {
	if rel.Mbid != "" {
		return rel.Mbid
	}
	return rel.CaaReleaseMbid
}

// Get user LB playlists and find wanted playlists ID
func (c *ListenBrainz) getImportPlaylist(user string) (string, error) {
	var offset int
	var bestDate time.Time
	var bestID string

	for {
		var body []byte
		var err error

		for retries := range c.cfg.RetryAttempts {
			body, err = c.lbRequest(fmt.Sprintf("user/%s/playlists/createdfor?offset=%d", user, offset))
			if err == nil {
				break
			}
			delay := c.cfg.RetryBaseDelay << retries // exponential backoff: baseDelay * 2^retries
			if delay > c.cfg.RetryMaxDelay || delay <= 0 {
				delay = c.cfg.RetryMaxDelay
			}
			slog.Warn(
				"failed getting response from ListenBrainz, retrying",
				"retry", retries+1,
				"delay", delay,
				"error", err,
			)
			time.Sleep(delay)
		}

		if err != nil {
			return "", fmt.Errorf("failed getting ListenBrainz playlist after retries: %s", err.Error())
		}

		var playlists CreatedFor
		if err = util.ParseResp(body, &playlists); err != nil {
			return "", fmt.Errorf("getImportPlaylist(): %s", err.Error())
		}

		for _, p := range playlists.Playlists {
			meta := p.Playlist.Extension.HTTPSJspfPlaylist.AdditionalMetadata
			if meta.AlgorithmMetadata.SourcePatch != c.cfg.ImportPlaylist {
				continue
			}
			if bestID == "" || p.Playlist.Date.After(bestDate) {
				bestDate = p.Playlist.Date
				parts := strings.Split(p.Playlist.Identifier, "/")
				bestID = parts[len(parts)-1]
			}
		}

		if playlists.Count+playlists.Offset >= playlists.PlaylistCount || playlists.Count == 0 {
			break
		}
		offset += playlists.Count
	}

	if bestID == "" {
		return "", fmt.Errorf("failed to get %s playlist, check if ListenBrainz has generated one", c.cfg.ImportPlaylist)
	}
	return bestID, nil
}

func (c *ListenBrainz) parsePlaylist(identifier string, singleArtist bool) ([]*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("playlist/%s", identifier))
	if err != nil {
		return nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	var exploration Exploration
	err = util.ParseResp(body, &exploration)
	if err != nil {
		return nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	srcTracks := exploration.Playlist.Tracks
	if len(srcTracks) == 0 {
		return nil, fmt.Errorf("no tracks found in playlist %s", identifier)
	}

	tracks := make([]*models.Track, 0, len(srcTracks))
	for _, track := range srcTracks {
		title := track.Title
		artist := track.Creator
		mainArtist := track.Creator

		trackMeta := track.Extension.HTTPSJspfTrack.AdditionalMetadata
		trackArtists := trackMeta.Artists

		if len(trackMeta.Artists) > 1 {
			mainArtist = trackMeta.Artists[0].ArtistCreditName
			if singleArtist {
				featArtists := make([]string, 0, len(trackArtists)-1)
				for _, a := range trackArtists[1:] {
					featArtists = append(featArtists, a.ArtistCreditName)
				}
				title = buildFeatTitle(track.Title, featArtists)
				artist = trackArtists[0].ArtistCreditName
			}
		}

		// The recording MBID is the last segment of the track's identifier URL
		recordingMBID := ""
		if len(track.Identifier) > 0 {
			parts := strings.Split(track.Identifier[0], "/")
			recordingMBID = parts[len(parts)-1]
		}

		artistMBID := ""
		if len(trackArtists) > 0 {
			artistMBID = trackArtists[0].ArtistMbid
		}

		var coverURL string
		if trackMeta.CaaReleaseMbid != "" && trackMeta.CaaID != 0 {
			coverURL = fmt.Sprintf("https://coverartarchive.org/release/%s/%d-%s.jpg",
				trackMeta.CaaReleaseMbid, trackMeta.CaaID, c.coverArtSize())
		}

		tracks = append(tracks, &models.Track{
			Album:               track.Album,
			MainArtist:          mainArtist,
			Artist:              artist,
			CleanTitle:          track.Title,
			Title:               title,
			Duration:            track.Duration,
			CoverURL:            coverURL,
			MusicBrainzTrackID:  recordingMBID,
			MusicBrainzAlbumID:  trackMeta.CaaReleaseMbid,
			MusicBrainzArtistID: artistMBID,
		})
	}

	return tracks, nil

}

// musicBrainzRate is MusicBrainz's documented limit for anonymous clients: one
// request per second on average. Going faster gets the client throttled or blocked.
const musicBrainzRate = time.Second

// mbLookupAttempts is how often a single recording lookup is retried before the
// track is left with only the metadata ListenBrainz provided.
const mbLookupAttempts = 3

// enrichTracks fills in the metadata the ListenBrainz playlist endpoints don't
// return: genres, ISRCs, release details and track positions. Tracks are updated
// in place, and any track whose lookup fails simply keeps what it already had.
func (c *ListenBrainz) enrichTracks(tracks []*models.Track, singleArtist bool) error {
	mbids := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.MusicBrainzTrackID != "" {
			mbids = append(mbids, track.MusicBrainzTrackID)
		}
	}
	if len(mbids) == 0 {
		return fmt.Errorf("none of the %d tracks carry a MusicBrainz recording ID", len(tracks))
	}

	recordings, err := c.recordingMetadata(mbids)
	if err != nil {
		return err
	}

	// Every track needs its own MusicBrainz lookup, and those are paced to one per
	// second, so give the user an idea of how long the run will sit here.
	slog.Info("enriching tracks with metadata, this may take a moment",
		"tracks", len(mbids), "estimated_seconds", len(mbids))

	limiter := rate.NewLimiter(rate.Every(musicBrainzRate), 1)
	ctx := context.Background()

	for _, track := range tracks {
		recording, ok := recordings[track.MusicBrainzTrackID]
		if !ok {
			continue
		}
		c.applyLBMetadata(track, recording, singleArtist)

		mbData, err := c.lookupRecording(ctx, limiter, track.MusicBrainzTrackID)
		if err != nil {
			slog.Debug("failed to enrich track from MusicBrainz",
				"mbid", track.MusicBrainzTrackID, "error", err.Error())
			continue
		}
		applyMBMetadata(track, mbData)
	}

	return nil
}

// recordingMetadata fetches the ListenBrainz metadata for a batch of recording MBIDs.
func (c *ListenBrainz) recordingMetadata(mbids []string) (Recordings, error) {
	body, err := c.lbRequest(fmt.Sprintf(
		"metadata/recording/?recording_mbids=%s&inc=release+artist+tag+release_group+recording",
		strings.Join(mbids, ",")))
	if err != nil {
		return nil, fmt.Errorf("recordingMetadata(): %s", err.Error())
	}

	var recordings Recordings
	if err := util.ParseResp(body, &recordings); err != nil {
		return nil, fmt.Errorf("recordingMetadata(): %s", err.Error())
	}
	if len(recordings) == 0 {
		return nil, fmt.Errorf("no recordings returned for %d MBIDs", len(mbids))
	}

	return recordings, nil
}

// applyLBMetadata copies the ListenBrainz half of the enriched metadata onto a track.
func (c *ListenBrainz) applyLBMetadata(track *models.Track, recording Metadata, singleArtist bool) {
	rec := recording.Recording
	rel := recording.Release
	recArtists := recording.Artist.Artists

	// Every assignment here is guarded: a recording that isn't linked to a canonical
	// release comes back with an empty release block, and blanking a title or album
	// the track already had would be worse than not enriching it at all.
	if rec.Name != "" {
		track.CleanTitle = rec.Name
		track.Title = rec.Name
	}
	if rel.Name != "" {
		track.Album = rel.Name
	}
	if rel.AlbumArtistName != "" {
		track.AlbumArtist = rel.AlbumArtistName
	}
	if recording.Artist.Name != "" {
		track.Artist = recording.Artist.Name
		track.MainArtist = recording.Artist.Name
	}

	if rel.Status != "" {
		track.ReleaseStatus = rel.Status
	}
	if mbid := releaseMBID(rel); mbid != "" {
		track.MusicBrainzAlbumID = mbid
	}
	if rel.ReleaseGroupMbid != "" {
		track.MusicBrainzReleaseGroupID = rel.ReleaseGroupMbid
	}
	if rec.Length > 0 {
		track.Duration = rec.Length
	}
	if len(rec.ISRCs) > 0 {
		track.ISRCs = append([]string(nil), rec.ISRCs...)
	}
	if len(recArtists) > 0 && recArtists[0].ArtistMbid != "" {
		track.MusicBrainzArtistID = recArtists[0].ArtistMbid
	}

	if len(recArtists) > 1 {
		track.MainArtist = recArtists[0].Name
		if singleArtist {
			featArtists := make([]string, 0, len(recArtists)-1)
			for _, a := range recArtists[1:] {
				featArtists = append(featArtists, a.Name)
			}
			track.Title = buildFeatTitle(track.CleanTitle, featArtists)
			track.Artist = track.MainArtist
			track.Artists = nil
		} else {
			artists := make([]string, 0, len(recArtists))
			for _, a := range recArtists {
				artists = append(artists, a.Name)
			}
			track.Artists = artists
		}
	}

	// Prefer the recording's first release date, it describes the work rather than
	// whichever release ListenBrainz happened to pick.
	if rec.FirstReleaseDate != "" {
		track.OriginalDate = rec.FirstReleaseDate
	}
	if rel.Year != 0 {
		track.OriginalYear = rel.Year
	}
	if track.OriginalYear == 0 && len(track.OriginalDate) >= 4 {
		if year, err := strconv.Atoi(track.OriginalDate[:4]); err == nil {
			track.OriginalYear = year
		}
	}

	genres := topTags(recording.Tag.Recording, 3)
	if len(genres) == 0 {
		genres = topTags(recording.Tag.ReleaseGroup, 3)
	}
	if len(genres) == 0 {
		genres = topTags(recording.Tag.Artist, 3)
	}
	if len(genres) > 0 {
		track.Genres = strings.Join(genres, "; ")
	}

	// Cover art hangs off the release ListenBrainz confirmed has an image, which is
	// not necessarily the canonical release we tag as the album.
	if track.CoverURL == "" {
		track.CoverURL = c.coverArtURL(rel.CaaReleaseMbid)
	}
}

// applyMBMetadata copies the per-release details only MusicBrainz exposes.
func applyMBMetadata(track *models.Track, mb *MBRecording) {
	if mb == nil || len(mb.Releases) == 0 {
		return
	}

	// A recording appears on many releases. Prefer the one ListenBrainz already
	// picked, so the tags stay consistent with the album name we set earlier.
	best := mb.Releases[0]
	if track.MusicBrainzAlbumID != "" {
		for _, rel := range mb.Releases {
			if rel.ID == track.MusicBrainzAlbumID {
				best = rel
				break
			}
		}
	}

	if best.Country != "" {
		track.ReleaseCountry = best.Country
	}
	if best.Status != "" {
		track.ReleaseStatus = best.Status
	}
	if best.ReleaseGroup.ID != "" {
		track.MusicBrainzReleaseGroupID = best.ReleaseGroup.ID
	}
	if best.ReleaseGroup.PrimaryType != "" {
		track.ReleaseType = best.ReleaseGroup.PrimaryType
	}
	if len(best.ArtistCredit) > 0 {
		track.MusicBrainzAlbumArtistID = best.ArtistCredit[0].Artist.ID
		track.ArtistSort = best.ArtistCredit[0].Artist.SortName
	}

	if len(best.Media) == 0 {
		return
	}

	// A recording lookup only returns the media that actually contain the recording,
	// so DiscTotal cannot be derived here: a track on disc 1 of a 2-disc release still
	// comes back with a single medium. It is left unset rather than written as a wrong
	// "1 of 1".
	media := best.Media[0]
	track.Media = media.Format
	track.DiscNumber = media.Position
	track.TrackTotal = media.TrackCount

	if len(media.Tracks) > 0 {
		track.TrackNumber = media.Tracks[0].Position
		track.MusicBrainzReleaseTrackID = media.Tracks[0].ID
	}
}

// lookupRecording fetches a recording from MusicBrainz, retrying a few times and
// pacing every attempt through the shared limiter.
func (c *ListenBrainz) lookupRecording(ctx context.Context, limiter *rate.Limiter, mbid string) (*MBRecording, error) {
	var lastErr error

	for range mbLookupAttempts {
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}

		mbData, err := c.mbRequest(fmt.Sprintf(
			"recording/%s?inc=media+releases+artist-credits+release-groups&fmt=json", mbid))
		if err == nil {
			return mbData, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("after %d attempts: %w", mbLookupAttempts, lastErr)
}

// Handle MusicBrainz API requests
func (c *ListenBrainz) mbRequest(path string) (*MBRecording, error) {
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/%s", path)
	body, err := c.HttpClient.MakeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to MusicBrainz API: %s", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("MusicBrainz API returned empty response for: %s", reqURL)
	}

	var recording MBRecording
	if err := util.ParseResp(body, &recording); err != nil {
		return nil, fmt.Errorf("failed to parse MusicBrainz response: %s", err)
	}

	return &recording, nil
}

// topTags returns up to limit tag names, most applied first. Ties break on the tag
// name so the result is stable across runs.
func topTags(tags []LBTag, limit int) []string {
	if len(tags) == 0 || limit <= 0 {
		return nil
	}

	ordered := append([]LBTag(nil), tags...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Count == ordered[j].Count {
			return ordered[i].Tag < ordered[j].Tag
		}
		return ordered[i].Count > ordered[j].Count
	})

	if limit > len(ordered) {
		limit = len(ordered)
	}

	out := make([]string, 0, limit)
	for _, tag := range ordered[:limit] {
		if tag.Tag != "" {
			out = append(out, tag.Tag)
		}
	}

	return out
}

// Handle ListenBrainz API requests
func (c *ListenBrainz) lbRequest(path string) ([]byte, error) {

	reqURL := fmt.Sprintf("https://api.listenbrainz.org/1/%s", path)
	body, err := c.HttpClient.MakeRequest("GET", reqURL, nil, c.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to ListenBrainz API: %s", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("ListenBrainz API returned empty response for: %s", reqURL)
	}

	return body, nil
}
