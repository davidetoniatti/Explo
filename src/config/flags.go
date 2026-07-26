package config

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
	"slices"
	"strings"
)

var (
	validPlaylists    = []string{"weekly-exploration", "weekly-jams", "daily-jams", "on-repeat"}
	validDownloadMode = []string{"normal", "skip", "force"}
)

func (cfg *Config) GetFlags() error {
	var configPath string
	var playlist string
	var downloadMode string
	var excludeLocal bool
	var replacePlaylist bool
	var persist bool
	var cleanDownloads bool
	var refreshOnly bool
	var searchMBID string
	var showVersion bool
	// Long flags
	flag.StringVarP(&configPath, "config", "c", ".env", "Path of the configuration file")
	flag.StringVarP(&playlist, "playlist", "p", "weekly-exploration", "Playlist where to get tracks. Supported: weekly-exploration, weekly-jams, daily-jams, on-repeat")
	flag.StringVarP(&downloadMode, "download-mode", "d", "normal", "Download mode: 'normal' (download only when track is not found locally), 'skip' (skip downloading, only use tracks already found locally), 'force' (always download, don't check for local tracks)")
	flag.BoolVarP(&excludeLocal, "exclude-local", "e", false, "Exclude locally found tracks from the imported playlist")
	flag.BoolVar(&replacePlaylist, "replace-playlist", true, "Replace the existing playlist of the same name instead of creating a date-stamped one")
	flag.BoolVar(&persist, "persist", true, "DEPRECATED, use --replace-playlist instead, which means the opposite")
	flag.BoolVar(&cleanDownloads, "clean-downloads", false, "Delete previously downloaded tracks before downloading new ones (requires USE_SUBDIRECTORY)")
	flag.BoolVar(&refreshOnly, "refresh-only", false, "Trigger a library rescan and exit, skipping discovery and downloads")
	flag.StringVar(&searchMBID, "search-mbid", "", "Resolve a MusicBrainz recording ID through ListenBrainz, look for it in your library, and exit")
	flag.BoolVarP(&showVersion, "version", "v", false, "Print version and exit")

	flag.Parse()

	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	cfg.Flags.CfgPath = configPath
	cfg.Flags.Playlist = playlist
	cfg.Flags.DownloadMode = downloadMode
	cfg.Flags.ExcludeLocal = excludeLocal
	cfg.Flags.ReplacePlaylist = replacePlaylist
	cfg.Flags.ReplacePlaylistSet = flag.Lookup("replace-playlist").Changed
	cfg.Flags.CleanDownloads = cleanDownloads
	cfg.Flags.RefreshOnly = refreshOnly
	cfg.Flags.SearchMBID = searchMBID

	// for deprecation purposes (can be removed at a later date)
	cfg.Flags.Persist = persist
	cfg.Flags.PersistSet = flag.Lookup("persist").Changed

	// --search-mbid looks up a single recording, so the playlist and download flags
	// are irrelevant and their defaults should not be validated against.
	if searchMBID != "" {
		return nil
	}

	// Validation for playlist
	if !contains(validPlaylists, playlist) {
		return fmt.Errorf("flag validation error: invalid playlist %s (must be one of: %s)",
			playlist, strings.Join(validPlaylists, ", "))
	}

	// Validation for download mode
	if !contains(validDownloadMode, downloadMode) {
		return fmt.Errorf("flag validation error: invalid download mode %s (must be one of: %s)",
			downloadMode, strings.Join(validDownloadMode, ", "))
	}

	return nil
}

func (cfg *Config) MergeFlags() {
	cfg.DiscoveryCfg.Listenbrainz.ImportPlaylist = cfg.Flags.Playlist
	cfg.DownloadCfg.ExcludeLocal = cfg.Flags.ExcludeLocal
	cfg.ReplacePlaylist = cfg.resolveReplacePlaylist()
}

// resolveReplacePlaylist decides whether the playlist is replaced in place. The
// deprecated --persist flag and PERSIST variable both mean the opposite of it, and
// are honoured rather than ignored so an existing setup keeps working. Most specific
// source wins: the flag, then the deprecated flag, then the variables.
func (cfg *Config) resolveReplacePlaylist() bool {
	switch {
	case cfg.Flags.ReplacePlaylistSet:
		return cfg.Flags.ReplacePlaylist
	case cfg.Flags.PersistSet:
		return !cfg.Flags.Persist
	case envIsSet("REPLACE_PLAYLIST"):
		return cfg.ReplacePlaylistENV
	case envIsSet("PERSIST"):
		return !cfg.PersistENV
	default:
		return cfg.Flags.ReplacePlaylist
	}
}

func contains(valid []string, val string) bool {
	return slices.Contains(valid, val)
}
