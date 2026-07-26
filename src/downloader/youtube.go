package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cfg "explo/src/config"
	"explo/src/logging"
	"explo/src/models"
	"explo/src/util"

	"github.com/wader/goutubedl"
)

type Videos struct {
	Items []Item `json:"items"`
}

type ID struct {
	VideoID string `json:"videoId"`
}

type Snippet struct {
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
}

type Item struct {
	ID      ID      `json:"id"`
	Snippet Snippet `json:"snippet"`
}

type YTMusicSearchResult struct {
	VideoID string `json:"videoId"`
	Title   string `json:"title"`
}

type Youtube struct {
	DownloadDir string
	HttpClient  *util.HttpClient
	Cfg         cfg.Youtube
	gouTubeOpts goutubedl.Options
	paths       *pathClaims
}

func NewYoutube(cfg cfg.Youtube, discovery, downloadDir string, httpClient *util.HttpClient, paths *pathClaims) *Youtube { // init downloader cfg for youtube
	// check for custom ytdlp options
	if cfg.YtdlpPath != "" {
		goutubedl.Path = cfg.YtdlpPath
	}

	var opts goutubedl.Options
	if _, err := os.Stat(cfg.CookiesPath); err == nil {
		opts.Cookies = cfg.CookiesPath
	}

	return &Youtube{
		DownloadDir: downloadDir,
		Cfg:         cfg,
		HttpClient:  httpClient,
		gouTubeOpts: opts,
		paths:       paths}
}

func (c *Youtube) QueryTrack(track *models.Track) error { // Queries youtube for the song

	query := fmt.Sprintf("%s - %s", track.Title, track.Artist)
	if c.Cfg.APIKey == "" { // if no API key set, use Python YT Music module
		err := queryYTMusic(track, query)
		return err
	}

	escQuery := url.PathEscape(query)
	queryURL := fmt.Sprintf("https://youtube.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=video&videoCategoryId=10&key=%s", escQuery, c.Cfg.APIKey)

	body, err := c.HttpClient.MakeRequest("GET", queryURL, nil, nil)
	if err != nil {
		return err
	}
	var videos Videos
	if err = util.ParseResp(body, &videos); err != nil {
		return fmt.Errorf("failed to unmarshal queryYT body: %s", err.Error())
	}

	id := c.gatherVideo(c.Cfg, videos, *track)
	if id == "" {
		return fmt.Errorf("no YouTube video found for track: %s - %s", track.Title, track.Artist)
	}
	track.ID = id

	return nil
}

// ytMusicScriptPath resolves the path to search_ytmusic.py. It defaults to the
// directory of the running binary (so it works from any CWD, not just the Docker
// image's WORKDIR), with an optional override via YTMUSIC_SCRIPT_PATH.
func ytMusicScriptPath() string {
	if p := os.Getenv("YTMUSIC_SCRIPT_PATH"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		slog.Warn("failed to resolve executable path, falling back to CWD-relative script path", "context", err.Error())
		return "search_ytmusic.py"
	}
	return filepath.Join(filepath.Dir(exe), "search_ytmusic.py")
}

func queryYTMusic(track *models.Track, query string) error {

	slog.Debug(fmt.Sprintf("Querying YTMusic for track %s", query))

	cmd := exec.Command("python3", ytMusicScriptPath(), query, "1")

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ytmusicapi subprocess failed: %w", err)
	}

	var results []YTMusicSearchResult
	if err := json.Unmarshal(out, &results); err != nil {
		return fmt.Errorf("failed to parse ytmusicapi JSON: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no YouTube Music track found for: %s", query)
	}

	track.ID = results[0].VideoID
	//log.Printf("Matched track %s => videoId %s", query, track.ID) keeping this until I improve logging (good trace)

	return nil
}

func (c *Youtube) GetTrack(track *models.Track) error {
	ctx := context.Background() // ctx for yt-dlp

	track.File = fmt.Sprintf("%s.%s", getFilename(track.Title, track.Artist), c.Cfg.FileExtension)
	track.Present = fetchAndSaveVideo(ctx, *c, track)

	if track.Present {
		slog.Info("download finished", "service", "youtube", "track", track.File)
		return nil
	}
	return fmt.Errorf("failed to download track: %s - %s", track.Title, track.Artist)
}

func (c *Youtube) MonitorDownloads(track []*models.Track) error { // No need to monitor yt-dlp downloads, there is no queue for them
	slog.Info("no further monitoring required", "service", "youtube")
	return nil
}

// gets song under artist topic or personal channel
func getTopic(cfg cfg.Youtube, videos Videos, track models.Track) string {

	for _, v := range videos.Items {
		if (strings.Contains(v.Snippet.ChannelTitle, "- Topic") || v.Snippet.ChannelTitle == track.MainArtist) && !ContainsKeyword(track, v.Snippet.Title, cfg.Filters.FilterList) {
			return v.ID.VideoID
		}
	}
	return ""
}

// gets video stream using yt-dlp
func getVideo(ctx context.Context, c Youtube, videoID string) (*goutubedl.DownloadResult, error) {

	result, err := goutubedl.New(ctx, videoID, c.gouTubeOpts)
	if err != nil {
		return nil, fmt.Errorf("could not create URL for video download (ID: %s): %s", videoID, err.Error())
	}

	downloadResult, err := result.Download(ctx, "bestaudio")
	if err != nil {
		return nil, fmt.Errorf("could not download video: %s", err.Error())
	}

	return downloadResult, nil

}

func saveVideo(c Youtube, track *models.Track, stream *goutubedl.DownloadResult) bool {

	defer func() {
		if err := stream.Close(); err != nil {
			slog.Warn("closing stream failed", "context", err.Error())
		}
	}()

	input := c.paths.claim(filepath.Join(c.DownloadDir, track.File+".tmp"))
	file, err := os.Create(input)
	if err != nil {
		slog.Error("failed to create song file", "context", err.Error())
		return false
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("file close failed", "context", err.Error())
		}
	}()

	if _, err = io.Copy(file, stream); err != nil {
		slog.Error("failed to copy stream to file", "context", err.Error())
		if err = os.Remove(input); err != nil {
			slog.Debug(fmt.Sprintf("failed to remove file %s", input), logging.RuntimeAttr(err.Error()))
		}
		return false
	}

	outputPath := filepath.Join(c.DownloadDir, track.File)
	if c.Cfg.PathTemplate != "" {
		relPath, terr := buildTrackPath(c.Cfg.PathTemplate, track)
		if terr != nil {
			slog.Warn("ignoring path template, writing to the download directory", "context", terr.Error())
		} else {
			outputPath = filepath.Join(c.DownloadDir, relPath)
			// the music system looks the track up by file name, so it has to follow
			// the file to wherever the template put it
			track.File = filepath.Base(relPath)
			track.RelPath = relPath
		}
	}

	if err = os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		slog.Error("failed to create output directory", "context", err.Error())
		if err = os.Remove(input); err != nil {
			slog.Debug(fmt.Sprintf("failed to remove %s", input), logging.RuntimeAttr(err.Error()))
		}
		return false
	}

	coverPath := resolveCover(c.HttpClient, track, outputPath, c.Cfg.CoversDir, c.Cfg.EmbedCoverArt)
	// yt-dlp hands back whatever the best audio stream is, which rarely matches the
	// configured extension, so the audio has to be re-encoded rather than copied.
	streams, opts := buildAudioOutput(input, coverPath, track, false)

	if err = util.WriteMetadata(streams, c.Cfg.FfmpegPath, outputPath, opts); err != nil {
		slog.Error("failed to convert audio", "context", err.Error())
		if err = os.Remove(input); err != nil {
			slog.Debug(fmt.Sprintf("failed to remove %s", input), logging.RuntimeAttr(err.Error()))
		}
		return false
	}
	if err = os.Remove(input); err != nil {
		slog.Debug(fmt.Sprintf("failed to remove %s", input), logging.RuntimeAttr(err.Error()))
	}
	return true
}

// filter out video ID
func (c *Youtube) gatherVideo(cfg cfg.Youtube, videos Videos, track models.Track) string {

	// Try to get the video from the official or topic channel
	if id := getTopic(cfg, videos, track); id != "" {
		return id

	}
	// If official video isn't found, try the first suitable channel
	for _, video := range videos.Items {
		if !ContainsKeyword(track, video.Snippet.Title, c.Cfg.Filters.FilterList) {
			return video.ID.VideoID
		}
	}

	return ""
}

func fetchAndSaveVideo(ctx context.Context, cfg Youtube, track *models.Track) bool {
	stream, err := getVideo(ctx, cfg, track.ID)
	if err != nil {
		slog.Error("failed getting stream for video", "trackID", track.ID, "context", err.Error())
		return false
	}

	if stream != nil {
		return saveVideo(cfg, track, stream)
	}

	slog.Error("stream was empty for video", "trackID", track.ID)
	return false
}
