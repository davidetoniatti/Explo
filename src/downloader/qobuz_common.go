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

	ffmpeg "github.com/u2takey/ffmpeg-go"
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

// saveStreamWithMetadata downloads downloadURL to a temp file in downloadDir, then uses
// ffmpeg to copy the audio stream and write id3/vorbis metadata to track.File. If ffmpeg
// fails, the temp file is moved into place as-is rather than lost. Shared by the native
// qobuz and squidwtf-qobuz downloaders. service is used only for log context.
func saveStreamWithMetadata(httpClient *util.HttpClient, downloadDir, downloadURL string, track *models.Track, service string) error {
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

	// Use ffmpeg to write metadata
	cmd := ffmpeg.Input(tempFile).Output(destPath, ffmpeg.KwArgs{
		"map":      "0:a",
		"c:a":      "copy",
		"metadata": []string{"artist=" + track.Artist, "title=" + track.Title, "album=" + track.Album},
		"loglevel": "error",
	}).OverWriteOutput().ErrorToStdOut()

	if err = cmd.Run(); err != nil {
		slog.Error("failed to write metadata", "service", service, "context", err.Error())
		// If ffmpeg fails, try to at least move the original file so it's not lost
		if rerr := os.Rename(tempFile, destPath); rerr != nil {
			return fmt.Errorf("ffmpeg failed (%s) and fallback rename also failed: %w", err.Error(), rerr)
		}
	} else if rerr := os.Remove(tempFile); rerr != nil {
		slog.Warn("failed to remove temp file", "service", service, "context", rerr.Error())
	}

	return nil
}
