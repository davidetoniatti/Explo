package downloader

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type DownloadClient struct {
	Cfg         *cfg.DownloadConfig
	Downloaders []Downloader
	paths       *pathClaims
}

type Downloader interface {
	QueryTrack(*models.Track) error
	GetTrack(*models.Track) error
}

// get download services from config and append them to DownloadClient
func NewDownloader(cfg *cfg.DownloadConfig, httpClient *util.HttpClient, filterLocal bool) (*DownloadClient, error) {
	// One set of claims for the whole run: services are tried in order and all write
	// into the same download directory, so a youtube download and a migrated slskd one
	// can collide just as easily as two tracks from one service.
	paths := newPathClaims()

	var downloader []Downloader
	for _, service := range cfg.Services {
		switch service {
		case "youtube":
			downloader = append(downloader, NewYoutube(cfg.Youtube, cfg.Discovery, cfg.DownloadDir, httpClient, paths))
		case "slskd":
			slskdClient := NewSlskd(cfg.Slskd, cfg.DownloadDir)
			slskdClient.AddHeader()
			downloader = append(downloader, slskdClient)
		case "squidwtf-qobuz":
			downloader = append(downloader, NewSquidWTFQobuz(cfg.Qobuz, cfg.DownloadDir, httpClient, paths))
		case "qobuz":
			downloader = append(downloader, NewQobuz(cfg.Qobuz, cfg.DownloadDir, httpClient, paths))
		default:
			return nil, fmt.Errorf("downloader '%s' not supported", service)
		}
	}

	return &DownloadClient{
		Cfg:         cfg,
		Downloaders: downloader,
		paths:       paths}, nil
}

func (c *DownloadClient) StartDownload(tracks *[]*models.Track) {
	if c.Cfg.ExcludeLocal { // remove locally found tracks, so they can't be added to playlist
		filterLocalTracks(tracks, true)
	}
	if c.needsDownloadDir() {
		if err := os.MkdirAll(c.Cfg.DownloadDir, 0755); err != nil {
			slog.Error(err.Error())
			return
		}
	}

	for _, d := range c.Downloaders {
		var g errgroup.Group
		g.SetLimit(3)

		for _, track := range *tracks {
			if track.Present {
				continue
			}

			g.Go(func() error {

				if err := d.QueryTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}
				if err := d.GetTrack(track); err != nil {
					slog.Warn(err.Error())
					return nil
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return
		}

		if m, ok := d.(Monitor); ok {
			err := c.MonitorDownloads(*tracks, m)
			if err != nil {
				slog.Warn(err.Error())
			}
		}
	}
	filterLocalTracks(tracks, false)
}

func (c *DownloadClient) needsDownloadDir() bool {
	for _, svc := range c.Cfg.Services {
		if svc == "youtube" || svc == "qobuz" || svc == "squidwtf-qobuz" {
			return true
		}
	}
	return c.Cfg.Slskd.MigrateDL
}

func (c *DownloadClient) DeleteSongs() {
	entries, err := os.ReadDir(c.Cfg.DownloadDir)
	if err != nil {
		slog.Error("failed to read directory", "context", err.Error())
		return
	}
	for _, entry := range entries {
		if !(entry.IsDir()) {
			err = os.Remove(path.Join(c.Cfg.DownloadDir, entry.Name()))

			if err != nil {
				slog.Error("failed to remove file", "context", err.Error())
			}
		}
	}
}

func filterLocalTracks(tracks *[]*models.Track, preDownload bool) { // filter local tracks
	filteredTracks := (*tracks)[:0]

	for _, t := range *tracks {
		switch {
		case preDownload && !t.Present:
			// keep only unavailable tracks if c.FilterLocal is true
			filteredTracks = append(filteredTracks, t)

		case !preDownload && t.Present:
			// keep only tracks already present locally
			t.Present = false // reset so music system can reuse the field
			filteredTracks = append(filteredTracks, t)
		}
	}

	*tracks = filteredTracks
}

func getFilename(title, artist string) string {
	const maxBytes = 240

	// Remove illegal characters for file naming
	t := util.FilenameSafe(title)
	a := util.FilenameSafe(artist)

	// truncate long filename
	runes := []rune(fmt.Sprintf("%s-%s", t, a))
	for len(runes) > 0 && len(string(runes)) > maxBytes {
		runes = runes[:len(runes)-1]
	}

	return string(runes)
}

// ignore titles that have a specific keyword (defined in .env)
func ContainsKeyword(track models.Track, contentTitle string, filterList []string) bool {
	title := strings.ToLower(track.Title)
	artist := strings.ToLower(track.Artist)
	content := strings.ToLower(contentTitle)

	for _, keyword := range filterList {
		keyword = strings.ToLower(keyword)
		if strings.Contains(title, keyword) || strings.Contains(artist, keyword) {
			continue
		}
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func containsLower(str string, substr string) bool {

	return strings.Contains(
		strings.ToLower(str),
		strings.ToLower(substr),
	)
}

// Move download from the source dir to the dest dir (download dir)
func (c *DownloadClient) MoveDownload(srcDir, destDir, trackPath string, track *models.Track) error {
	trackDir := filepath.Join(srcDir, trackPath)
	srcFile := filepath.Join(trackDir, track.File)

	if c.Cfg.RenameTrack { // Rename file to {title}-{artist} format
		track.File = getFilename(track.CleanTitle, track.MainArtist) + filepath.Ext(track.File)
	}

	if c.Cfg.OverwriteMetadata {
		if err := c.overwriteMetadata(srcFile, track); err != nil {
			slog.Warn("failed to overwrite metadata", "file", srcFile, "context", err.Error())
		}
	}

	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err.Error())
	}

	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Error(fmt.Sprintf("failed to close source file: %s", cerr.Error()))
		}
	}()

	dstFile := filepath.Join(destDir, track.File)
	if c.Cfg.PathTemplate != "" {
		relPath, terr := buildTrackPath(c.Cfg.PathTemplate, track)
		if terr != nil {
			slog.Warn("ignoring path template, writing to the download directory", "context", terr.Error())
		} else {
			dstFile = filepath.Join(destDir, relPath)
			track.File = filepath.Base(relPath)
			track.RelPath = relPath
		}
	}

	dstFile = c.paths.claim(dstFile)
	track.File = filepath.Base(dstFile)
	if track.RelPath != "" {
		track.RelPath = filepath.Join(filepath.Dir(track.RelPath), track.File)
	}

	if err = os.MkdirAll(filepath.Dir(dstFile), os.ModePerm); err != nil {
		return fmt.Errorf("couldn't make download directory: %s", err.Error())
	}

	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("couldn't create destination file: %s", err.Error())
	}

	defer func() {
		if cerr := out.Close(); cerr != nil {
			slog.Error(fmt.Sprintf("failed to close destination file: %s", cerr.Error()))
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy failed: %s", err.Error())
	}

	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync failed: %s", err.Error())
	}

	// Keep permissions, unless specified otherwise in .env (some systems don't support chmod)
	if c.Cfg.KeepPermissions {
		info, err := os.Stat(srcFile)
		if err != nil {
			return fmt.Errorf("stat error: %s", err.Error())
		}
		if err = os.Chmod(dstFile, info.Mode()); err != nil {
			return fmt.Errorf("chmod failed: %s", err.Error())
		}
	}

	// Remove only the moved file, not the directory
	if err = os.Remove(srcFile); err != nil {
		return fmt.Errorf("failed to delete original file: %s", err.Error())
	}

	// to avoid removing additional downloads check if directory is empty before removing
	isEmpty, err := isDirEmpty(trackDir)
	if err != nil {
		return fmt.Errorf("couldn't check if directory is empty: %s", err.Error())
	} else if isEmpty {
		if err = os.Remove(trackDir); err != nil {
			return fmt.Errorf("failed to remove empty directory: %s", err.Error())
		}
	}
	return nil
}

// pathSegmentReplacer strips characters that are illegal in file names on common
// filesystems while keeping the name readable, so spaces and accents survive.
var pathSegmentReplacer = strings.NewReplacer(
	"/", "-",
	`\`, "-",
	":", "-",
	"*", "",
	"?", "",
	`"`, "",
	"<", "",
	">", "",
	"|", "",
)

func sanitizePathSegment(s string) string {
	return strings.TrimSpace(pathSegmentReplacer.Replace(s))
}

// buildTrackPath renders a PATH_TEMPLATE for a track into a path relative to the
// download directory. Substituted values cannot introduce directory separators, and
// a template that still resolves outside the download directory is rejected rather
// than silently writing somewhere unexpected.
func buildTrackPath(template string, track *models.Track) (string, error) {
	year := ""
	if track.OriginalYear != 0 {
		year = strconv.Itoa(track.OriginalYear)
	}

	replacements := map[string]string{
		"Artist":      sanitizePathSegment(track.MainArtist),
		"AlbumArtist": sanitizePathSegment(track.AlbumArtist),
		"Album":       sanitizePathSegment(track.Album),
		"AlbumName":   sanitizePathSegment(track.Album),
		"TrackName":   sanitizePathSegment(track.CleanTitle),
		"TrackNumber": fmt.Sprintf("%02d", track.TrackNumber),
		"DiscNumber":  fmt.Sprintf("%02d", track.DiscNumber),
		"Year":        year,
		"File":        sanitizePathSegment(track.File),
		"ext":         strings.TrimPrefix(filepath.Ext(track.File), "."),
	}

	// Absoluteness is judged on the template itself. Checking after substitution would
	// confuse "the user asked for /etc/..." with "a placeholder came out empty".
	if filepath.IsAbs(template) {
		return "", fmt.Errorf("path template %q is absolute, it must be relative to the download directory", template)
	}

	result := template
	for key, value := range replacements {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}

	// A placeholder with nothing to fill it leaves an empty segment: "{{Year}}/{{TrackName}}"
	// on a track with no year would otherwise yield "/Song.flac", an absolute path.
	segments := make([]string, 0, strings.Count(result, "/")+1)
	for _, part := range strings.Split(filepath.ToSlash(result), "/") {
		part = strings.TrimSpace(part)
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("path template %q resolved to a path escaping the download directory", template)
		}
		segments = append(segments, part)
	}

	if len(segments) == 0 {
		return "", fmt.Errorf("path template %q produced an empty path", template)
	}

	return filepath.Join(segments...), nil
}

// coverArtContainers are the output formats ffmpeg can attach a cover picture to.
// Opus and Ogg carry artwork in a metadata block ffmpeg will not write, so asking
// for an attached picture there fails the whole conversion.
var coverArtContainers = map[string]struct{}{
	".mp3":  {},
	".flac": {},
	".m4a":  {},
	".mp4":  {},
	".aiff": {},
}

func supportsCoverArt(path string) bool {
	_, ok := coverArtContainers[strings.ToLower(filepath.Ext(path))]
	return ok
}

// buildAudioOutput assembles the ffmpeg inputs and options for writing a track:
// metadata from the track, and cover art attached when coverPath is set. When
// copyAudio is false the audio is re-encoded to suit the output container, which is
// what YouTube downloads need since the source rarely matches the target format.
func buildAudioOutput(input, coverPath string, track *models.Track, copyAudio bool) ([]*ffmpeg.Stream, ffmpeg.KwArgs) {
	var streams []*ffmpeg.Stream

	opts := ffmpeg.KwArgs{
		"metadata": util.BuildffmpegMetadata(*track),
		"loglevel": "error",
		"c:v":      "copy",
	}

	if coverPath != "" {
		// ffmpeg-go already emits one -map per stream once more than one is passed, so
		// the selection has to be expressed through the stream selectors. Adding a
		// "map" option on top maps every input twice, and ffmpeg rejects the resulting
		// duplicate audio stream outright.
		streams = []*ffmpeg.Stream{
			ffmpeg.Input(input).Audio(),
			ffmpeg.Input(coverPath).Video(),
		}
		opts["disposition:v"] = "attached_pic"
		opts["metadata:s:v"] = []string{"title=Album cover", "comment=Cover (front)"}
	} else {
		// A lone input is not auto-mapped, so the selection is given as an option here.
		// "?" makes the picture optional since most sources have none, and mapping it
		// keeps artwork the source already carried instead of dropping it.
		streams = []*ffmpeg.Stream{ffmpeg.Input(input)}
		opts["map"] = []string{"0:a", "0:v?"}
	}

	if copyAudio {
		opts["c:a"] = "copy"
	}

	return streams, opts
}

// resolveCover returns a local cover art path for the track, downloading it if
// needed. Returns "" when embedding is off, unavailable, or unsupported for the
// output container.
func resolveCover(httpClient *util.HttpClient, track *models.Track, outputPath, coversDir string, embed bool) string {
	if !embed || track.CoverURL == "" {
		return ""
	}
	if !supportsCoverArt(outputPath) {
		slog.Debug("skipping cover art, container does not support an attached picture",
			"file", filepath.Base(outputPath))
		return ""
	}
	if track.CoverPath != "" {
		return track.CoverPath
	}

	coverPath, err := httpClient.DownloadCover(track.CoverURL, coversDir)
	if err != nil {
		slog.Debug("failed to download cover art", "url", track.CoverURL, "context", err.Error())
		return ""
	}

	track.CoverPath = coverPath
	return coverPath
}

// overwriteMetadata rewrites srcFile's tags from the discovery metadata, replacing
// whatever the download source wrote. ffmpeg cannot edit in place, so the result goes
// to a sibling temp file that is renamed over the original once it is complete.
func (c *DownloadClient) overwriteMetadata(srcFile string, track *models.Track) error {
	// ffmpeg infers the output container from the file extension, so without one it
	// cannot write the temp file at all.
	if filepath.Ext(srcFile) == "" {
		return fmt.Errorf("cannot infer container format for %q, file has no extension", srcFile)
	}

	tmpFile := tempAudioFile(srcFile)

	removeTmp := func() {
		if err := os.Remove(tmpFile); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temp file", "file", tmpFile, "context", err.Error())
		}
	}

	opts := ffmpeg.KwArgs{
		"c":        "copy",
		"metadata": util.BuildffmpegMetadata(*track),
		"loglevel": "error",
	}
	streams := []*ffmpeg.Stream{ffmpeg.Input(srcFile)}

	if err := util.WriteMetadata(streams, c.Cfg.FfmpegPath, tmpFile, opts); err != nil {
		removeTmp() // ffmpeg may have written a partial file before failing
		return err
	}

	if err := os.Rename(tmpFile, srcFile); err != nil {
		// Leaving the temp file behind would also keep the source directory from
		// being cleaned up once the download is moved.
		removeTmp()
		return fmt.Errorf("failed to replace original file: %w", err)
	}

	return nil
}

// tempAudioFile returns a sibling path with the same extension, so ffmpeg keeps
// inferring the container format from it.
func tempAudioFile(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".tmp" + ext
}

func isDirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() {
		if err = f.Close(); err != nil {
			slog.Error(fmt.Sprintf("failed to close directory path: %s", err.Error()))
		}
	}()

	// If we get something other than an err, it's not empty
	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil // no entries
	}
	return false, err
}
