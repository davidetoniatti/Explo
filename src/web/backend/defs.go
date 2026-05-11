// place for confs/variables in use by the UI

package backend

// configFields describes the settings this web UI currently owns.
// VisibleWhen / RequiredWhen drive the settings UI; the wizard
// uses bespoke HTML but references the same logical rules.

// FieldDef describes a single configurable env var.
// Injected into the page as window.__FIELDS__ for the settings UI to consume.
type FieldDef struct {
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Type         string     `json:"type"`    // text | password | url | select
	Section      string     `json:"section"` // discovery | system | downloader
	Placeholder  string     `json:"placeholder,omitempty"`
	Hint         string     `json:"hint,omitempty"`
	Required     bool       `json:"required,omitempty"`
	Options      []Option   `json:"options,omitempty"`      // for type=select
	VisibleWhen  *Condition `json:"visibleWhen,omitempty"`  // hide field when condition is false
	RequiredWhen *Condition `json:"requiredWhen,omitempty"` // conditionally required
}

/* var netSystems = []string{"jellyfin", "emby", "plex", "subsonic"}
var apiKeySystems = []string{"jellyfin", "emby", "plex"} */

// playlistDef is the single source of truth for a supported playlist type.
// To add a new playlist: append one entry here and add the matching entry in
// PLAYLISTS in the frontend Settings.jsx.
type playlistDef struct {
	EnvPrefix       string // e.g. "WEEKLY_EXPLORATION"
	DefaultSchedule string // cron expression
	DefaultFlags    string // CLI flags for the run
}

var playlistDefs = map[string]playlistDef{
	"weekly-exploration": {"WEEKLY_EXPLORATION", "15 00 * * 2", "--playlist weekly-exploration"},
	"weekly-jams":        {"WEEKLY_JAMS", "30 00 * * 1", "--playlist weekly-jams"},
	"daily-jams":         {"DAILY_JAMS", "15 01 * * *", "--playlist daily-jams"},
	"on-repeat":          {"ON_REPEAT", "0 12 1 * *", "--playlist on-repeat"},
}

// allConfigKeys is the complete set of env keys the web UI reads and writes.
var allConfigKeys = []string{
	"LISTENBRAINZ_USER", "LISTENBRAINZ_DISCOVERY",
	"WEEKLY_EXPLORATION_SCHEDULE", "WEEKLY_EXPLORATION_FLAGS",
	"WEEKLY_JAMS_SCHEDULE", "WEEKLY_JAMS_FLAGS",
	"DAILY_JAMS_SCHEDULE", "DAILY_JAMS_FLAGS",
	"ON_REPEAT_SCHEDULE", "ON_REPEAT_FLAGS",
	"EXPLO_SYSTEM", "SYSTEM_URL", "API_KEY", "LIBRARY_NAME",
	"SYSTEM_USERNAME", "SYSTEM_PASSWORD", "PLAYLIST_DIR", "SLEEP", "PUBLIC_PLAYLIST",
	"DOWNLOAD_DIR", "USE_SUBDIRECTORY",
	"DOWNLOAD_SERVICES", "YOUTUBE_API_KEY", "TRACK_EXTENSION", "FILTER_LIST",
	"QOBUZ_QUALITY",
	"SLSKD_URL", "SLSKD_API_KEY",
	"WIZARD_COMPLETE",
}
