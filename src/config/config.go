package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var Version = "dev"

type Config struct {
	DownloadCfg  DownloadConfig
	DiscoveryCfg DiscoveryConfig
	ClientCfg    ClientConfig
	NotifyCfg    NotifyConfig
	Flags        Flags
	// ReplacePlaylistENV and PersistENV are only read to resolve precedence, use
	// ReplacePlaylist for the answer. PERSIST is deprecated and means the opposite.
	ReplacePlaylistENV bool `env:"REPLACE_PLAYLIST" env-default:"true"`
	PersistENV         bool `env:"PERSIST" env-default:"true"`
	ReplacePlaylist    bool
	System             string `env:"EXPLO_SYSTEM"`
	Debug              bool   `env:"DEBUG" env-default:"false"`
	LogLevel           string `env:"LOG_LEVEL" env-default:"INFO"`
}

type Flags struct {
	CfgPath      string
	Playlist     string
	DownloadMode string
	ExcludeLocal bool

	ReplacePlaylist    bool
	ReplacePlaylistSet bool
	CleanDownloads     bool
	RefreshOnly        bool
	SearchMBID         string

	// Persist is deprecated in favour of ReplacePlaylist, which it is the inverse of
	Persist    bool
	PersistSet bool
}

type ClientConfig struct {
	ClientID        string `env:"CLIENT_ID" env-default:"explo"`
	LibraryName     string `env:"LIBRARY_NAME" env-default:"Explo"`
	URL             string `env:"SYSTEM_URL"`
	DownloadDir     string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	PlaylistDir     string `env:"PLAYLIST_DIR"`
	PlaylistName    string
	PlaylistNFormat string `env:"PLAYLISTNAME_FORMAT" env-default:"week"`
	PlaylistDescr   string
	PlaylistID      string
	Sleep           int `env:"SLEEP" env-default:"2"`
	HTTPTimeout     int `env:"CLIENT_HTTP_TIMEOUT" env-default:"10"`
	Creds           Credentials
	AdminCreds      AdminCredentials
	Subsonic        SubsonicConfig
}

type Credentials struct {
	APIKey   string `env:"API_KEY"`
	User     string `env:"SYSTEM_USERNAME"`
	Password string `env:"SYSTEM_PASSWORD"`
	Headers  map[string]string
	Token    string
	Salt     string
}

type AdminCredentials struct {
	User     string `env:"ADMIN_SYSTEM_USERNAME"`
	Password string `env:"ADMIN_SYSTEM_PASSWORD"`
}

type SubsonicConfig struct {
	Version        string `env:"SUBSONIC_VERSION" env-default:"1.16.1"`
	ID             string `env:"CLIENT" env-default:"explo"`
	PublicPlaylist bool   `env:"PUBLIC_PLAYLIST" env-default:"false"`
}

// FFMPEG_PATH is bound here and on each downloader that shells out to ffmpeg, so
// every downloader stays self-contained. cleanenv fills all of them from the one
// variable, they cannot diverge.
type DownloadConfig struct {
	DownloadDir       string `env:"DOWNLOAD_DIR" env-default:"/data/"`
	FfmpegPath        string `env:"FFMPEG_PATH"`
	PathTemplate      string `env:"PATH_TEMPLATE"` // e.g. {{Artist}}/{{Album}}/{{TrackNumber}} - {{TrackName}}.{{ext}}
	EmbedCoverArt     bool   `env:"EMBED_COVER_ART" env-default:"false"`
	CoversDir         string `env:"COVERS_DIR"` // defaults to a directory under the system temp dir
	Youtube           Youtube
	Slskd             Slskd
	Qobuz             Qobuz
	ExcludeLocal      bool
	OverwriteMetadata bool     `env:"OVERWRITE_METADATA" env-default:"false"` // replace downloaded metadata with discovery metadata when migrating
	KeepPermissions   bool     `env:"KEEP_PERMISSIONS" env-default:"true"`    // keep original file permissions when migrating download
	RenameTrack       bool     `env:"RENAME_TRACK" env-default:"false"`       // Rename track in {title}-{artist} format
	UseSubDir         bool     `env:"USE_SUBDIRECTORY" env-default:"true"`
	Discovery         string   `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	Services          []string `env:"DOWNLOAD_SERVICES" env-default:"youtube"`
}

type Filters struct {
	Extensions  []string `env:"EXTENSIONS" env-default:"flac,mp3"`
	MinBitDepth int      `env:"MIN_BIT_DEPTH" env-default:"8"`
	MinBitRate  int      `env:"MIN_BITRATE" env-default:"256"`
	FilterList  []string `env:"FILTER_LIST" env-default:"live,remix,instrumental,extended,clean,acapella"`
}

type Qobuz struct {
	Quality       string `env:"QOBUZ_QUALITY" env-default:"27"`
	UserAuthToken string `env:"QOBUZ_USER_AUTH_TOKEN"`
	UserId        string `env:"QOBUZ_USER_ID"`
	FfmpegPath    string `env:"FFMPEG_PATH"`
	Filters       Filters

	// copied from DownloadConfig by CommonFixes
	PathTemplate  string
	CoversDir     string
	EmbedCoverArt bool
}

type Youtube struct {
	APIKey        string `env:"YOUTUBE_API_KEY"`
	FfmpegPath    string `env:"FFMPEG_PATH"`
	YtdlpPath     string `env:"YTDLP_PATH"`
	FileExtension string `env:"TRACK_EXTENSION" env-default:"opus"`
	CookiesPath   string `env:"COOKIES_PATH" env-default:"./cookies.txt"`
	Filters       Filters

	// copied from DownloadConfig by CommonFixes
	PathTemplate  string
	CoversDir     string
	EmbedCoverArt bool
}

type Slskd struct {
	APIKey           string `env:"SLSKD_API_KEY"`
	URL              string `env:"SLSKD_URL"`
	Retry            int    `env:"SLSKD_RETRY" env-default:"5"`       // Number of times to check search status before skipping the track
	DownloadAttempts int    `env:"SLSKD_DL_ATTEMPTS" env-default:"3"` // Max number of files to attempt downloading per track
	SlskdDir         string `env:"SLSKD_DIR" env-default:"/slskd/"`
	MigrateDL        bool   `env:"MIGRATE_DOWNLOADS" env-default:"false"` // Move downloads from SlskdDir to DownloadDir
	Timeout          int    `env:"SLSKD_TIMEOUT" env-default:"20"`
	Filters          Filters
	MonitorConfig    SlskdMon
}

type SlskdMon struct {
	Interval time.Duration `env:"SLSKD_MONITOR_INTERVAL" env-default:"1m"`
	Duration time.Duration `env:"SLSKD_MONITOR_DURATION" env-default:"15m"`
}

type DiscoveryConfig struct {
	Discovery       string   `env:"DISCOVERY_SERVICE" env-default:"listenbrainz"`
	ArtistBlacklist []string `env:"ARTIST_BLACKLIST"` // artist names or MusicBrainz artist IDs to skip
	Listenbrainz    Listenbrainz
}
type Listenbrainz struct {
	Discovery           string `env:"LISTENBRAINZ_DISCOVERY" env-default:"playlist"`
	User                string `env:"LISTENBRAINZ_USER"`
	UserToken           string `env:"LISTENBRAINZ_USER_TOKEN"`
	ImportPlaylist      string
	SingleArtist        bool          `env:"SINGLE_ARTIST" env-default:"true"`
	CoverArtSize        string        `env:"COVER_ART_SIZE" env-default:"250"`
	EnrichTrackMetadata bool          `env:"ENRICH_TRACK_METADATA" env-default:"false"`
	RetryAttempts       int           `env:"LISTENBRAINZ_RETRY_ATTEMPTS" env-default:"5"`
	RetryBaseDelay      time.Duration `env:"LISTENBRAINZ_RETRY_BASE_DELAY" env-default:"15s"`
	RetryMaxDelay       time.Duration `env:"LISTENBRAINZ_RETRY_MAX_DELAY" env-default:"5m"`
}

type NotifyConfig struct {
	Matrix  MatrixNotif
	Discord DiscordNotif
	Http    HttpNotif
}

type MatrixNotif struct {
	UserID      string `env:"MATRIX_USERID"`
	RoomID      string `env:"MATRIX_ROOMID"`
	HomeServer  string `env:"MATRIX_HOMESERVER_URL"`
	AccessToken string `env:"MATRIX_ACCESSTOKEN"`
}

type DiscordNotif struct {
	BotToken   string   `env:"DISCORD_BOT_TOKEN"`
	ChannelIDs []string `env:"DISCORD_CHANNEL_ID"`
}

type HttpNotif struct {
	ReceiverURLs []string `env:"HTTP_RECEIVER"`
}

func (cfg *Config) ReadEnv() {

	// Try to read from .env file first
	err := cleanenv.ReadConfig(cfg.Flags.CfgPath, cfg)
	if err != nil {
		// If the error is because the file doesn't exist, fallback to env vars
		if errors.Is(err, os.ErrNotExist) {
			if err := cleanenv.ReadEnv(cfg); err != nil {
				slog.Error("failed to load config from env vars", "context", err.Error())
				os.Exit(1)
			}
		} else {
			slog.Error("failed to load config file", "path", cfg.Flags.CfgPath, "context", err.Error())
			os.Exit(1)
		}
	}

	cfg.CommonFixes()
}

func (cfg *Config) CommonFixes() {
	cfg.DownloadCfg.Youtube.FileExtension = strings.TrimPrefix(cfg.DownloadCfg.Youtube.FileExtension, ".")
	cfg.shareDownloadSettings()
	cfg.ClientCfg.URL = fixBaseURL(cfg.ClientCfg.URL)
	cfg.DownloadCfg.Slskd.URL = fixBaseURL(cfg.DownloadCfg.Slskd.URL)
	cfg.NormalizeDir()
}

// shareDownloadSettings copies the output settings that apply to every downloader
// down into the per-downloader configs, which is all their constructors receive.
func (cfg *Config) shareDownloadSettings() {
	if cfg.DownloadCfg.CoversDir == "" {
		// Covers are only staged here on the way into the audio file, so they belong
		// in temp rather than in the user's music library.
		cfg.DownloadCfg.CoversDir = filepath.Join(os.TempDir(), "explo-covers")
	}

	dl := &cfg.DownloadCfg

	dl.Youtube.PathTemplate = dl.PathTemplate
	dl.Youtube.CoversDir = dl.CoversDir
	dl.Youtube.EmbedCoverArt = dl.EmbedCoverArt

	dl.Qobuz.PathTemplate = dl.PathTemplate
	dl.Qobuz.CoversDir = dl.CoversDir
	dl.Qobuz.EmbedCoverArt = dl.EmbedCoverArt
}

func (cfg *Config) NormalizeDir() {
	if cfg.System == "mpd" {
		cfg.ClientCfg.PlaylistDir = fixDir(cfg.ClientCfg.PlaylistDir)
	}
	cfg.DownloadCfg.Slskd.SlskdDir = fixDir(cfg.DownloadCfg.Slskd.SlskdDir)
	cfg.DownloadCfg.DownloadDir = fixDir(cfg.DownloadCfg.DownloadDir)
}

func fixDir(dir string) string {
	if !strings.HasSuffix(dir, "/") && dir != "" {
		return dir + "/"
	}
	return dir
}

func fixBaseURL(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return ""
	}
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	return strings.TrimRight(u, "/")
}

// ApplyDeprecatedLogLevel translates the deprecated DEBUG variable into a log level.
// It has to run before logging is initialised, unlike the other deprecation notices,
// which are only warnings and are better off going through the configured handler.
func (cfg *Config) ApplyDeprecatedLogLevel() {
	if cfg.Debug {
		cfg.LogLevel = "DEBUG"
	}
}

func (cfg *Config) HandleDeprecation() { //
	if cfg.Debug {
		slog.Warn("'DEBUG' variable is deprecated, please use LOG_LEVEL=DEBUG instead")
	}
	if cfg.Flags.PersistSet {
		slog.Warn("'--persist' is deprecated, use '--replace-playlist' instead, which means the opposite",
			"applied_as", fmt.Sprintf("--replace-playlist=%t", !cfg.Flags.Persist))
	}
	if envIsSet("PERSIST") {
		slog.Warn("'PERSIST' is deprecated, use 'REPLACE_PLAYLIST' instead, which means the opposite",
			"applied_as", fmt.Sprintf("REPLACE_PLAYLIST=%t", !cfg.PersistENV))
	}

	// --persist=false used to delete the downloaded tracks along with the playlist.
	// That is a separate setting now, so say so rather than quietly stopping. Not
	// enabled automatically: failing to delete leaves files behind, which is
	// recoverable, whereas deleting files nobody asked to delete is not.
	deprecatedAskedToReplace := (cfg.Flags.PersistSet && !cfg.Flags.Persist) ||
		(envIsSet("PERSIST") && !cfg.PersistENV)
	if deprecatedAskedToReplace && !cfg.Flags.CleanDownloads {
		slog.Warn("deleting downloaded tracks is no longer part of replacing a playlist, pass '--clean-downloads' to keep deleting them")
	}

	if cfg.Flags.CleanDownloads && !cfg.DownloadCfg.UseSubDir {
		slog.Warn("Deleting tracks requires 'USE_SUBDIRECTORY' to be true")
	}
}

// envIsSet reports whether a variable was supplied, as opposed to falling back to its
// default. cleanenv exports the contents of the config file into the environment, so
// this covers both a real variable and one set in the .env file.
func envIsSet(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func (cfg *Config) GenPlaylistName() { // Generate playlist name and description

	folderName := getPlaylistName(cfg.Flags.Playlist, cfg.ClientCfg.PlaylistNFormat, cfg.ReplacePlaylist)
	cfg.ClientCfg.PlaylistName = strings.ReplaceAll(folderName, "-", " ")
	cfg.ClientCfg.PlaylistDescr = fmt.Sprintf("Generated by Explo on %s from %s", time.Now().Format("2006-01-02"), cfg.Flags.Playlist)

	if cfg.DownloadCfg.UseSubDir {
		// add playlist name to downloadDir so all songs get downloaded to a single sub directory.
		cfg.DownloadCfg.DownloadDir = filepath.Join(
			cfg.DownloadCfg.DownloadDir,
			folderName)
	}
}

func getPlaylistName(playlistType, format string, replace bool) string {
	now := time.Now()

	toTitle := cases.Title(language.Und)
	base := toTitle.String(playlistType)

	// A playlist that gets replaced in place keeps one stable name, so there is
	// nothing to date-stamp.
	if replace {
		return base
	}

	// Explicit date-based naming
	if format == "date" {
		return fmt.Sprintf(
			"%s-%s",
			base,
			now.Format("2006-01-02"),
		)
	}

	// Persistent, non-date naming
	if playlistType == "daily-jams" {
		return fmt.Sprintf(
			"%s-%d-Day%d",
			base,
			now.Year(),
			now.YearDay(),
		)
	}

	year, week := now.ISOWeek()
	return fmt.Sprintf(
		"%s-%d-Week%d",
		base,
		year,
		week,
	)
}
