package downloader

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"explo/src/models"
	"explo/src/util"
)

// qobuzQualityID maps a configured quality string (env value or Qobuz numeric format_id
// as a string) to Qobuz's numeric format_id. Shared by the native qobuz and
// squidwtf-qobuz downloaders, which both talk to Qobuz's format_id-based APIs.
func qobuzQualityID(quality string) int {
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

// qobuzExtension maps a Qobuz format_id to the resulting file extension.
func qobuzExtension(formatId int) string {
	if formatId == 5 {
		return "mp3"
	}
	return "flac"
}

// pickBestQobuzTrack returns the first track in items that doesn't match a filtered
// keyword and, if duration is known for both sides, is within 10 seconds of the source
// track's duration. Returns nil if no track qualifies.
func pickBestQobuzTrack(track *models.Track, items []QobuzTrack, filterList []string) *QobuzTrack {
	for _, qobuzTrack := range items {
		if ContainsKeyword(*track, qobuzTrack.Title, filterList) {
			continue
		}

		// If duration is available, check for a match within 10 seconds
		if track.Duration > 0 && qobuzTrack.Duration > 0 {
			if util.Abs(track.Duration/1000-qobuzTrack.Duration) > 10 {
				continue
			}
		}

		result := qobuzTrack
		return &result
	}
	return nil
}

// saveOptions carries the output settings shared by the qobuz downloaders.
type saveOptions struct {
	DownloadDir   string
	FfmpegPath    string // empty to look ffmpeg up on PATH
	PathTemplate  string
	CoversDir     string
	EmbedCoverArt bool
	Service       string // log context only
}

// saveStreamWithMetadata downloads downloadURL to a temp file, then uses ffmpeg to copy
// the audio stream and write id3/vorbis metadata to the destination. If ffmpeg fails,
// the temp file is moved into place as-is rather than lost. Shared by the native qobuz
// and squidwtf-qobuz downloaders.
func saveStreamWithMetadata(httpClient *util.HttpClient, downloadURL string, track *models.Track, opts saveOptions) error {
	downloadDir, service := opts.DownloadDir, opts.Service
	stream, err := httpClient.GetStream(downloadURL, nil)
	if err != nil {
		return err
	}
	defer stream.Close()

	tempFile := filepath.Join(downloadDir, track.File+".tmp")
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(file, stream)
	if cerr := file.Close(); cerr != nil { // Close before ffmpeg processing
		slog.Warn("failed to close temp file", "service", service, "context", cerr.Error())
	}
	if copyErr != nil {
		if rerr := os.Remove(tempFile); rerr != nil {
			slog.Warn("failed to remove temp file", "service", service, "context", rerr.Error())
		}
		return copyErr
	}

	destPath := filepath.Join(downloadDir, track.File)
	if opts.PathTemplate != "" {
		relPath, terr := buildTrackPath(opts.PathTemplate, track)
		if terr != nil {
			slog.Warn("ignoring path template, writing to the download directory", "service", service, "context", terr.Error())
		} else {
			destPath = filepath.Join(downloadDir, relPath)
			track.File = filepath.Base(relPath)
			track.RelPath = relPath
		}
	}

	if err = os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		if rerr := os.Remove(tempFile); rerr != nil {
			slog.Warn("failed to remove temp file", "service", service, "context", rerr.Error())
		}
		return fmt.Errorf("couldn't make download directory: %w", err)
	}

	// Qobuz already delivers the target format, so the audio is copied untouched.
	coverPath := resolveCover(httpClient, track, destPath, opts.CoversDir, opts.EmbedCoverArt)
	streams, ffOpts := buildAudioOutput(tempFile, coverPath, track, true)

	if err = util.WriteMetadata(streams, opts.FfmpegPath, destPath, ffOpts); err != nil {
		slog.Error("saving track failed", "service", service, "context", err.Error())
		// If ffmpeg fails, try to at least move the original file so it's not lost
		if rerr := os.Rename(tempFile, destPath); rerr != nil {
			return fmt.Errorf("ffmpeg failed (%s) and fallback rename also failed: %w", err.Error(), rerr)
		}
	} else if rerr := os.Remove(tempFile); rerr != nil {
		slog.Warn("failed to remove temp file", "service", service, "context", rerr.Error())
	}

	return nil
}
