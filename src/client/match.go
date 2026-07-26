package client

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"explo/src/models"
	"explo/src/util"
)

// durationToleranceMs is how far a library item's length may differ from the track's
// and still be considered the same recording.
const durationToleranceMs = 10000

// libraryItem is the subset of a music system's search result that track matching
// needs, so the matching rules live in one place instead of once per system.
type libraryItem struct {
	Title       string
	Album       string
	AlbumArtist string
	Artists     []string
	Path        string
	Duration    int // milliseconds, 0 when the system does not report it
	// MusicBrainzIDs holds whatever MusicBrainz identifiers the system exposes for
	// the item, in no particular order.
	MusicBrainzIDs []string
}

// trackTitles returns the normalized titles a library item may legitimately carry for
// this track. Callers hoist it out of the result loop, since it is the same for every
// candidate. Both titles are considered: Title carries the featured artists while
// CleanTitle does not, and a library may have tagged the track either way.
func trackTitles(track *models.Track) []string {
	titles := make([]string, 0, 2)
	for _, title := range []string{track.CleanTitle, track.Title} {
		normalized := util.NormalizeTitle(title)
		if normalized == "" {
			continue
		}
		if len(titles) == 1 && titles[0] == normalized {
			continue
		}
		titles = append(titles, normalized)
	}
	return titles
}

// matchesTrack reports whether a library item is the track being looked for.
// normalizedTitles must come from trackTitles(track).
//
// A MusicBrainz identifier settles it on its own. Otherwise the title has to match
// and be corroborated by the album or the artist, since titles alone collide
// constantly across a library: covers, tributes and karaoke records share them. As a
// last resort an already downloaded file is accepted on artist plus path.
func matchesTrack(track *models.Track, normalizedTitles []string, item libraryItem) bool {
	if musicBrainzMatch(track, item.MusicBrainzIDs...) {
		return true
	}

	artistMatch := artistMatches(track.MainArtist, item.AlbumArtist, item.Artists)

	if titleMatches(item.Title, normalizedTitles) {
		if albumMatches(track.Album, item.Album) || artistMatch {
			return true
		}
	}

	if track.File == "" || !util.ContainsFold(item.Path, track.File) {
		return false
	}

	// A file name alone is weak: a 30 second preview and the real track can share one.
	// It needs either the artist or an agreeing length behind it, and a length that
	// actively disagrees rules the item out whatever else lines up. Emby and Jellyfin
	// searches report no length while Plex and Subsonic do, so both routes stay open.
	if !durationMatches(track.Duration, item.Duration) {
		return false
	}

	return artistMatch || (track.Duration > 0 && item.Duration > 0)
}

// musicBrainzMatch reports whether any identifier the library holds for an item
// names this track. Servers disagree about which id goes in which field: Picard
// writes the recording id as MUSICBRAINZ_TRACKID and the track-in-release id as
// MUSICBRAINZ_RELEASETRACKID, and systems map those differently. Jellyfin even
// swapped meaning in 10.11, where MusicBrainzTrack became the release-track id and
// MusicBrainzRecording the recording. Comparing every id of ours against every id of
// theirs sidesteps the whole question.
func musicBrainzMatch(track *models.Track, ids ...string) bool {
	want := make([]string, 0, 2)
	if track.MusicBrainzTrackID != "" {
		want = append(want, track.MusicBrainzTrackID)
	}
	if track.MusicBrainzReleaseTrackID != "" {
		want = append(want, track.MusicBrainzReleaseTrackID)
	}
	if len(want) == 0 {
		return false
	}

	for _, id := range ids {
		if id == "" {
			continue
		}
		for _, w := range want {
			if strings.EqualFold(id, w) {
				return true
			}
		}
	}

	return false
}

// artistMatches compares a track's main artist against the album artist and the
// credited artists of a library item. An empty main artist never matches, otherwise
// every untagged item in the library would.
func artistMatches(mainArtist, albumArtist string, artists []string) bool {
	if mainArtist == "" {
		return false
	}
	if namesAgree(albumArtist, mainArtist) {
		return true
	}
	for _, artist := range artists {
		if namesAgree(artist, mainArtist) {
			return true
		}
	}
	return false
}

// albumMatches compares the track's album against a library item's. Libraries
// routinely decorate the name, "OK Computer" against "OK Computer (Remastered)", so
// one being a leading part of the other counts.
func albumMatches(trackAlbum, itemAlbum string) bool {
	if trackAlbum == "" || itemAlbum == "" {
		return false
	}
	return namesAgree(itemAlbum, trackAlbum)
}

// durationMatches reports whether two lengths describe the same recording. A length
// of zero means unknown, and an unknown length is not treated as a mismatch: Emby and
// Jellyfin searches do not return one.
func durationMatches(trackMs, itemMs int) bool {
	if trackMs == 0 || itemMs == 0 {
		return true
	}
	return util.Abs(trackMs-itemMs) < durationToleranceMs
}

// namesAgree reports whether two names refer to the same thing: either they are
// equal ignoring case, or one begins with the other and the remainder starts at a
// word boundary.
//
// The boundary is what keeps this honest. A plain substring test would match an album
// called "IV" against "Ivory Tower", or an artist called "Eve" against "Everything
// But The Girl", and combined with a title collision that is enough to attach a
// playlist to the wrong recording. Requiring the extra text to begin with something
// other than a letter or digit still accepts "OK Computer (Remastered)" and
// "Radiohead & Friends".
func namesAgree(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))

	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}

	// Either side may be the decorated one.
	return hasPrefixAtBoundary(a, b) || hasPrefixAtBoundary(b, a)
}

func hasPrefixAtBoundary(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(s[len(prefix):])
	return !unicode.IsLetter(next) && !unicode.IsDigit(next)
}

func titleMatches(itemTitle string, normalizedTitles []string) bool {
	normalized := util.NormalizeTitle(itemTitle)
	if normalized == "" {
		return false
	}
	for _, title := range normalizedTitles {
		if normalized == title {
			return true
		}
	}
	return false
}
