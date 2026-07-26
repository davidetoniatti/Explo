package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

const QobuzBaseUrl = "https://qobuz.squid.wtf"
const QobuzCountryHeader = "Token-Country"
const QobuzCountryValue = "US"

// Qobuz API Models

type QobuzSearchResponse struct {
	Success bool             `json:"success"`
	Data    *QobuzSearchData `json:"data"`
}

type QobuzSearchData struct {
	Albums  *QobuzAlbumList  `json:"albums"`
	Tracks  *QobuzTrackList  `json:"tracks"`
	Artists *QobuzArtistList `json:"artists"`
}

type QobuzAlbumList struct {
	Items []QobuzAlbum `json:"items"`
}

type QobuzTrackList struct {
	Items []QobuzTrack `json:"items"`
}

type QobuzArtistList struct {
	Items []QobuzArtist `json:"items"`
}

type QobuzAlbum struct {
	ID                  interface{}  `json:"id"` // Can be string or int
	Title               string       `json:"title"`
	Artist              *QobuzArtist `json:"artist"`
	Image               *QobuzImage  `json:"image"`
	TracksCount         int          `json:"tracks_count"`
	ReleasedAt          int64        `json:"released_at"`
	ReleaseDateOriginal string       `json:"release_date_original"`
}

type QobuzTrack struct {
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	Duration    int          `json:"duration"`
	TrackNumber int          `json:"track_number"`
	MediaNumber int          `json:"media_number"`
	Album       *QobuzAlbum  `json:"album"`
	Performer   *QobuzArtist `json:"performer"`
}

type QobuzArtist struct {
	ID          int64       `json:"id"`
	Name        QobuzName   `json:"name"`
	Image       *QobuzImage `json:"image"`
	AlbumsCount int         `json:"albums_count"`
}

type QobuzImage struct {
	Small     string `json:"small"`
	Thumbnail string `json:"thumbnail"`
	Large     string `json:"large"`
}

type QobuzDownloadResponse struct {
	Success bool               `json:"success"`
	Data    *QobuzDownloadData `json:"data"`
}

type QobuzDownloadData struct {
	Url string `json:"url"`
}

// QobuzName handles the "name" field which can be a string or an object {"display": "..."}
type QobuzName string

func (qn *QobuzName) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*qn = QobuzName(s)
		return nil
	}

	var obj struct {
		Display string `json:"display"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		*qn = QobuzName(obj.Display)
		return nil
	}

	return nil
}

type SquidWTFQobuz struct {
	HttpClient    *util.HttpClient
	CaptchaSolver *util.CaptchaSolver
	DownloadDir   string
	Cfg           config.Qobuz
	paths         *pathClaims
}

func NewSquidWTFQobuz(cfg config.Qobuz, downloadDir string, httpClient *util.HttpClient, paths *pathClaims) *SquidWTFQobuz {
	return &SquidWTFQobuz{
		HttpClient:    httpClient,
		CaptchaSolver: util.NewCaptchaSolver(httpClient),
		DownloadDir:   downloadDir,
		Cfg:           cfg,
		paths:         paths,
	}
}

func (c *SquidWTFQobuz) QueryTrack(track *models.Track) error {
	query := fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist)
	escQuery := url.QueryEscape(query)
	queryURL := fmt.Sprintf("%s/api/get-music?q=%s&offset=0", QobuzBaseUrl, escQuery)

	headers := map[string]string{
		QobuzCountryHeader: QobuzCountryValue,
	}

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, headers)
	if err != nil {
		return err
	}

	var searchResp QobuzSearchResponse
	if err := util.ParseResp(body, &searchResp); err != nil {
		return err
	}

	if !searchResp.Success || searchResp.Data == nil || searchResp.Data.Tracks == nil || len(searchResp.Data.Tracks.Items) == 0 {
		return fmt.Errorf("no Qobuz tracks found for: %s", query)
	}

	bestTrack := pickBestQobuzTrack(track, searchResp.Data.Tracks.Items, c.Cfg.Filters.FilterList)
	if bestTrack == nil {
		return fmt.Errorf("no suitable Qobuz tracks found after filtering for: %s", query)
	}

	track.ID = fmt.Sprintf("%d", bestTrack.ID)
	if track.Album == "" && bestTrack.Album != nil {
		track.Album = bestTrack.Album.Title
	}
	slog.Debug("Qobuz track found", "id", track.ID, "title", bestTrack.Title)

	return nil
}

func (c *SquidWTFQobuz) GetTrack(track *models.Track) error {
	formatId := qobuzQualityID(c.Cfg.Quality)
	quality := strconv.Itoa(formatId)
	downloadURL, err := c.getDownloadURL(track.ID, quality, false)
	if err != nil {
		return err
	}

	track.File = fmt.Sprintf("%s.%s", getFilename(track.Title, track.Artist), qobuzExtension(formatId))

	err = c.downloadAndSave(downloadURL, track)
	if err != nil {
		return err
	}

	track.Present = true
	slog.Info("download finished", "service", "squidwtf-qobuz", "track", track.File)
	return nil
}

func (c *SquidWTFQobuz) getDownloadURL(trackID, quality string, forceRefresh bool) (string, error) {
	cookie, err := c.CaptchaSolver.GetCaptchaCookie(QobuzBaseUrl)
	if err != nil {
		return "", err
	}

	downloadApiURL := fmt.Sprintf("%s/api/download-music?track_id=%s&quality=%s", QobuzBaseUrl, trackID, quality)

	req, err := http.NewRequest("GET", downloadApiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add(QobuzCountryHeader, QobuzCountryValue)
	req.Header.Add("Cookie", cookie)
	req.Header.Add("User-Agent", c.HttpClient.UserAgent)

	resp, err := c.HttpClient.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && !forceRefresh {
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(strings.ToLower(string(body)), "captcha required") {
			slog.Info("Qobuz captcha required, refreshing session and retrying")
			if _, err := c.CaptchaSolver.Refresh(QobuzBaseUrl); err != nil {
				return "", err
			}
			return c.getDownloadURL(trackID, quality, true)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Qobuz download API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var dlResp QobuzDownloadResponse
	if err := json.Unmarshal(body, &dlResp); err != nil {
		return "", err
	}

	if !dlResp.Success || dlResp.Data == nil || dlResp.Data.Url == "" {
		return "", fmt.Errorf("failed to get download URL from Qobuz")
	}

	return dlResp.Data.Url, nil
}

func (c *SquidWTFQobuz) downloadAndSave(downloadURL string, track *models.Track) error {
	return saveStreamWithMetadata(c.HttpClient, downloadURL, track, saveOptions{
		DownloadDir:   c.DownloadDir,
		FfmpegPath:    c.Cfg.FfmpegPath,
		PathTemplate:  c.Cfg.PathTemplate,
		CoversDir:     c.Cfg.CoversDir,
		EmbedCoverArt: c.Cfg.EmbedCoverArt,
		Paths:         c.paths,
		Service:       "squidwtf-qobuz",
	})
}
