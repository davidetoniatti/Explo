package downloader

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type NativeQobuzSearchResponse struct {
	Tracks *QobuzTrackList `json:"tracks"`
}

type NativeQobuzDownloadResponse struct {
	Url          string  `json:"url"`
	MimeType     string  `json:"mime_type"`
	BitDepth     int     `json:"bit_depth"`
	SamplingRate float64 `json:"sampling_rate"`
	Sample       bool    `json:"sample"`
}

type Qobuz struct {
	HttpClient  *util.HttpClient
	DownloadDir string
	Cfg         config.Qobuz
	AppID       string
	Secrets     []string
	initMu      sync.Mutex
}

func NewQobuz(cfg config.Qobuz, downloadDir string, httpClient *util.HttpClient) *Qobuz {
	return &Qobuz{
		HttpClient:  httpClient,
		DownloadDir: downloadDir,
		Cfg:         cfg,
	}
}

func decodeBase64(input string) ([]byte, error) {
	input = strings.TrimSpace(input)
	if l := len(input) % 4; l > 0 {
		input += strings.Repeat("=", 4-l)
	}
	return base64.StdEncoding.DecodeString(input)
}

func (c *Qobuz) initializeQobuzBundle() error {
	c.initMu.Lock()
	defer c.initMu.Unlock()

	if c.AppID != "" && len(c.Secrets) > 0 {
		return nil
	}

	slog.Info("Extracting Qobuz App ID and secrets from web bundle...")

	// 1. Get bundle URL from play.qobuz.com/login
	req, err := http.NewRequest("GET", "https://play.qobuz.com/login", nil)
	if err != nil {
		return fmt.Errorf("failed to create bundle login request: %w", err)
	}
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:83.0) Gecko/20100101 Firefox/83.0")

	resp, err := c.HttpClient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch login page: %w", err)
	}
	defer resp.Body.Close()

	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login page body: %w", err)
	}
	html := string(htmlBytes)

	// Regex for bundle JS
	bundleUrlRegex := regexp.MustCompile(`(?i)<script src="(/resources/[^"]+bundle\.js)"></script>`)
	bundleMatches := bundleUrlRegex.FindStringSubmatch(html)
	if len(bundleMatches) < 2 {
		return fmt.Errorf("could not find bundle URL in Qobuz login page")
	}

	bundleURL := "https://play.qobuz.com" + bundleMatches[1]
	slog.Debug("Found bundle URL", "url", bundleURL)

	// 2. Fetch the bundle JS
	reqJS, err := http.NewRequest("GET", bundleURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create bundle JS request: %w", err)
	}
	reqJS.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:83.0) Gecko/20100101 Firefox/83.0")

	respJS, err := c.HttpClient.Client.Do(reqJS)
	if err != nil {
		return fmt.Errorf("failed to fetch bundle JS: %w", err)
	}
	defer respJS.Body.Close()

	jsBytes, err := io.ReadAll(respJS.Body)
	if err != nil {
		return fmt.Errorf("failed to read bundle JS body: %w", err)
	}
	bundleJS := string(jsBytes)

	// 3. Extract App ID
	appIdRegex := regexp.MustCompile(`production:\{api:\{appId:"(\d{9})",appSecret:"\w{32}"`)
	appIdMatches := appIdRegex.FindStringSubmatch(bundleJS)
	if len(appIdMatches) < 2 {
		return fmt.Errorf("could not extract App ID from bundle JS")
	}
	c.AppID = appIdMatches[1]
	slog.Debug("Extracted App ID", "appId", c.AppID)

	// 4. Extract seed/timezone pairs
	seedTimezoneRegex := regexp.MustCompile(`(?i)[a-z]\.initialSeed\("([\w=]+)",window\.utimezone\.([a-z]+)\)`)
	seedMatches := seedTimezoneRegex.FindAllStringSubmatch(bundleJS, -1)

	type timezoneMatch struct {
		timezone string
		seed     string
		info     string
		extras   string
	}

	var matches []timezoneMatch
	seenTimezones := make(map[string]int)

	for _, sm := range seedMatches {
		if len(sm) < 3 {
			continue
		}
		seed := sm[1]
		timezone := strings.ToLower(sm[2])

		if idx, exists := seenTimezones[timezone]; exists {
			matches[idx].seed += seed
		} else {
			seenTimezones[timezone] = len(matches)
			matches = append(matches, timezoneMatch{
				timezone: timezone,
				seed:     seed,
			})
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("could not extract seed/timezone pairs from bundle JS")
	}

	// 5. Reorder timezone keypairs if count > 1 (move second element to first place)
	if len(matches) > 1 {
		matches[0], matches[1] = matches[1], matches[0]
	}

	// 6. Extract info and extras for each timezone
	var tzNames []string
	for _, m := range matches {
		tzNames = append(tzNames, regexp.QuoteMeta(m.timezone))
	}
	infoExtrasRegexStr := fmt.Sprintf(`(?i)name:"\w+/(%s)",info:"([\w=]+)",extras:"([\w=]+)"`, strings.Join(tzNames, "|"))
	infoExtrasRegex := regexp.MustCompile(infoExtrasRegexStr)
	infoExtrasMatches := infoExtrasRegex.FindAllStringSubmatch(bundleJS, -1)

	for _, iem := range infoExtrasMatches {
		if len(iem) < 4 {
			continue
		}
		timezone := strings.ToLower(iem[1])
		info := iem[2]
		extras := iem[3]

		for i := range matches {
			if matches[i].timezone == timezone {
				matches[i].info = info
				matches[i].extras = extras
				break
			}
		}
	}

	// 7. Decode the secrets
	var decodedSecrets []string
	for _, m := range matches {
		concatenated := m.seed + m.info + m.extras
		if len(concatenated) > 44 {
			concatenated = concatenated[:len(concatenated)-44]
		}

		decodedBytes, err := decodeBase64(concatenated)
		if err != nil {
			slog.Debug("Failed to decode base64 for timezone", "timezone", m.timezone, "error", err)
			continue
		}
		decodedSecrets = append(decodedSecrets, string(decodedBytes))
	}

	if len(decodedSecrets) == 0 {
		return fmt.Errorf("could not decode any secrets from bundle")
	}

	c.Secrets = decodedSecrets
	slog.Info("Successfully extracted Qobuz config", "appId", c.AppID, "secretsCount", len(c.Secrets))
	return nil
}

func (c *Qobuz) QueryTrack(track *models.Track) error {
	if err := c.initializeQobuzBundle(); err != nil {
		return fmt.Errorf("qobuz init failed: %w", err)
	}

	query := fmt.Sprintf("%s - %s", track.CleanTitle, track.Artist)
	escQuery := url.QueryEscape(query)
	queryURL := fmt.Sprintf("https://www.qobuz.com/api.json/0.2/track/search?query=%s&limit=20&app_id=%s", escQuery, c.AppID)

	headers := make(map[string]string)
	if c.Cfg.UserAuthToken != "" {
		headers["X-User-Auth-Token"] = c.Cfg.UserAuthToken
	}
	headers["X-App-Id"] = c.AppID

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, headers)
	if err != nil {
		return err
	}

	var searchResp NativeQobuzSearchResponse
	if err := util.ParseResp(body, &searchResp); err != nil {
		return err
	}

	if searchResp.Tracks == nil || len(searchResp.Tracks.Items) == 0 {
		return fmt.Errorf("no Qobuz tracks found for: %s", query)
	}

	var bestTrack *QobuzTrack
	for _, qobuzTrack := range searchResp.Tracks.Items {
		if ContainsKeyword(*track, qobuzTrack.Title, c.Cfg.Filters.FilterList) {
			continue
		}

		// If duration is available, check for a match within 10 seconds
		if track.Duration > 0 && qobuzTrack.Duration > 0 {
			if util.Abs(track.Duration/1000-qobuzTrack.Duration) > 10 {
				continue
			}
		}

		bestTrack = &qobuzTrack
		break
	}

	if bestTrack == nil {
		return fmt.Errorf("no suitable Qobuz tracks found after filtering for: %s", query)
	}

	track.ID = fmt.Sprintf("%d", bestTrack.ID)
	if track.Album == "" && bestTrack.Album != nil {
		track.Album = bestTrack.Album.Title
	}
	slog.Debug("Qobuz direct track found", "id", track.ID, "title", bestTrack.Title)

	return nil
}

func (c *Qobuz) GetTrack(track *models.Track) error {
	if err := c.initializeQobuzBundle(); err != nil {
		return fmt.Errorf("qobuz init failed: %w", err)
	}

	quality := c.Cfg.Quality
	downloadURL, formatId, err := c.getDownloadURL(track.ID, quality)
	if err != nil {
		return err
	}

	track.File = fmt.Sprintf("%s.%s", getFilename(track.Title, track.Artist), c.getExtension(formatId))

	err = c.downloadAndSave(downloadURL, track)
	if err != nil {
		return err
	}

	track.Present = true
	slog.Info("download finished", "service", "qobuz", "track", track.File)
	return nil
}

func (c *Qobuz) getQualityFormatID(quality string) int {
	switch strings.ToUpper(quality) {
	case "FLAC_24_192", "FLAC_24", "27":
		return 27
	case "FLAC_24_96", "7":
		return 7
	case "FLAC_16", "FLAC", "6":
		return 6
	case "MP3_320", "MP3", "5":
		return 5
	default:
		return 27
	}
}

func (c *Qobuz) getFormatPriority(preferred int) []int {
	allFormats := []int{27, 7, 6, 5}
	priority := []int{preferred}
	for _, f := range allFormats {
		if f != preferred {
			priority = append(priority, f)
		}
	}
	return priority
}

func (c *Qobuz) getDownloadURL(trackID string, preferredQuality string) (string, int, error) {
	preferredID := c.getQualityFormatID(preferredQuality)
	priority := c.getFormatPriority(preferredID)

	var lastErr error
	for _, secret := range c.Secrets {
		for _, formatId := range priority {
			url, err := c.tryGetDownloadURL(trackID, formatId, secret)
			if err == nil {
				return url, formatId, nil
			}
			lastErr = err
			slog.Debug("Failed to sign track with secret & format", "format", formatId, "error", err)
		}
	}

	return "", 0, fmt.Errorf("failed to get signed download URL with all secrets/formats: %w", lastErr)
}

func (c *Qobuz) tryGetDownloadURL(trackID string, formatId int, secret string) (string, error) {
	timestamp := time.Now().Unix()

	toSign := fmt.Sprintf("trackgetFileUrlformat_id%dintentstreamtrack_id%s%d%s", formatId, trackID, timestamp, secret)
	h := md5.New()
	h.Write([]byte(toSign))
	signature := hex.EncodeToString(h.Sum(nil))

	queryURL := fmt.Sprintf("https://www.qobuz.com/api.json/0.2/track/getFileUrl?format_id=%d&intent=stream&request_ts=%d&track_id=%s&request_sig=%s", formatId, timestamp, trackID, signature)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create getFileUrl request: %w", err)
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:83.0) Gecko/20100101 Firefox/83.0")
	req.Header.Add("X-App-Id", c.AppID)
	if c.Cfg.UserAuthToken != "" {
		req.Header.Add("X-User-Auth-Token", c.Cfg.UserAuthToken)
	}

	resp, err := c.HttpClient.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("getFileUrl request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getFileUrl returned HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read getFileUrl response: %w", err)
	}

	var errCheck map[string]interface{}
	if err := json.Unmarshal(body, &errCheck); err == nil {
		if errMsg, exists := errCheck["error"]; exists {
			return "", fmt.Errorf("Qobuz API error: %v", errMsg)
		}
	}

	var dlResp NativeQobuzDownloadResponse
	if err := json.Unmarshal(body, &dlResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal getFileUrl response: %w", err)
	}

	if dlResp.Url == "" {
		return "", fmt.Errorf("no download URL in response")
	}

	if dlResp.Sample {
		return "", fmt.Errorf("track is only available as a demo/sample")
	}

	return dlResp.Url, nil
}

func (c *Qobuz) getExtension(formatId int) string {
	if formatId == 5 {
		return "mp3"
	}
	return "flac"
}

func (c *Qobuz) downloadAndSave(downloadURL string, track *models.Track) error {
	stream, err := c.HttpClient.GetStream(downloadURL, nil)
	if err != nil {
		return err
	}
	defer stream.Close()

	tempFile := filepath.Join(c.DownloadDir, track.File+".tmp")
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, stream)
	file.Close() // Close before ffmpeg processing
	if err != nil {
		os.Remove(tempFile)
		return err
	}

	destPath := filepath.Join(c.DownloadDir, track.File)

	// Use ffmpeg to write metadata
	cmd := ffmpeg.Input(tempFile).Output(destPath, ffmpeg.KwArgs{
		"map":      "0:a",
		"c:a":      "copy",
		"metadata": []string{"artist=" + track.Artist, "title=" + track.Title, "album=" + track.Album},
		"loglevel": "error",
	}).OverWriteOutput().ErrorToStdOut()

	if err = cmd.Run(); err != nil {
		slog.Error("failed to write metadata", "service", "qobuz", "context", err.Error())
		os.Rename(tempFile, destPath)
	} else {
		os.Remove(tempFile)
	}

	return nil
}

func (c *Qobuz) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	return nil, fmt.Errorf("no monitoring required")
}

func (c *Qobuz) GetConf() (MonitorConfig, error) {
	return MonitorConfig{}, fmt.Errorf("[qobuz] no monitoring required")
}

func (c *Qobuz) Cleanup(track models.Track, ID string) error {
	return nil
}
