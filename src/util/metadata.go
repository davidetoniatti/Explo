package util

import (
	"fmt"
	"strings"

	"explo/src/models"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// Return absolute difference between tracks
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func addStringTag(metadata []string, key, value string) []string {
	if value != "" {
		metadata = append(metadata, key+"="+value)
	}
	return metadata
}

func addIntTag(metadata []string, key string, value int) []string {
	if value != 0 {
		metadata = append(metadata, fmt.Sprintf("%s=%d", key, value))
	}
	return metadata
}

// BuildffmpegMetadata renders a track as a list of "key=value" pairs for ffmpeg's
// -metadata flag. Tag names follow MusicBrainz Picard's conventions, so files land
// in the library tagged the same way Picard would tag them. Empty and zero values
// are skipped rather than written as blanks.
func BuildffmpegMetadata(track models.Track) []string {
	metadata := []string{}

	// Individual credits win over the collapsed artist string, but only once blanks
	// are dropped, otherwise a credit list of empty names writes "artist=".
	named := make([]string, 0, len(track.Artists))
	for _, artist := range track.Artists {
		if artist != "" {
			named = append(named, artist)
		}
	}
	if len(named) > 0 {
		metadata = addStringTag(metadata, "artist", strings.Join(named, "; "))
	} else {
		metadata = addStringTag(metadata, "artist", track.Artist)
	}

	metadata = addStringTag(metadata, "title", track.Title)
	metadata = addStringTag(metadata, "album", track.Album)
	metadata = addStringTag(metadata, "albumartist", track.AlbumArtist)
	metadata = addStringTag(metadata, "artistsort", track.ArtistSort)
	metadata = addStringTag(metadata, "date", track.OriginalDate)
	metadata = addStringTag(metadata, "genre", track.Genres)
	metadata = addStringTag(metadata, "releasecountry", track.ReleaseCountry)
	metadata = addStringTag(metadata, "TMED", track.Media)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumType", track.ReleaseType)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumStatus", track.ReleaseStatus)
	metadata = addStringTag(metadata, "MusicBrainz_TrackId", track.MusicBrainzTrackID)
	metadata = addStringTag(metadata, "MusicBrainz_ReleaseTrackId", track.MusicBrainzReleaseTrackID)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumId", track.MusicBrainzAlbumID)
	metadata = addStringTag(metadata, "MusicBrainz_ReleaseGroupId", track.MusicBrainzReleaseGroupID)
	metadata = addStringTag(metadata, "MusicBrainz_ArtistId", track.MusicBrainzArtistID)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumArtistId", track.MusicBrainzAlbumArtistID)

	metadata = addIntTag(metadata, "originalyear", track.OriginalYear)
	metadata = addIntTag(metadata, "track", track.TrackNumber)
	metadata = addIntTag(metadata, "Tracktotal", track.TrackTotal)
	metadata = addIntTag(metadata, "disc", track.DiscNumber)
	metadata = addIntTag(metadata, "Disctotal", track.DiscTotal)

	for _, isrc := range track.ISRCs {
		metadata = addStringTag(metadata, "ISRC", isrc)
	}

	return metadata
}

// WriteMetadata runs ffmpeg over streams, writing the result to filePath with opts.
// ffmpegPath may be empty, in which case ffmpeg is looked up on PATH.
func WriteMetadata(streams []*ffmpeg.Stream, ffmpegPath, filePath string, opts ffmpeg.KwArgs) error {
	cmd := ffmpeg.Output(streams, filePath, opts).OverWriteOutput().ErrorToStdOut()

	if ffmpegPath != "" {
		cmd.SetFfmpegPath(ffmpegPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}
