package main

import (
	"explo/src/logging"
	"log"
	"log/slog"
	"os"

	"explo/src/client"
	"explo/src/config"
	"explo/src/discovery"
	"explo/src/downloader"
	"explo/src/models"
	"explo/src/util"
)

type Song struct {
	Title  string
	Artist string
	Album  string
}

func initHttpClient() *util.HttpClient {
	return util.NewHttp(util.HttpClientConfig{
		Timeout: 10,
	})
}

// Sets up logging, warns about deprecated settings and generates the playlist name
func setup(cfg *config.Config) {
	cfg.ApplyDeprecatedLogLevel() // can change the log level, so it precedes Init
	logging.Init(cfg)
	cfg.HandleDeprecation()
	cfg.GenPlaylistName()
}

func main() {
	var cfg config.Config
	if err := cfg.GetFlags(); err != nil {
		log.Fatal(err)
	}
	cfg.ReadEnv()
	cfg.MergeFlags()
	setup(&cfg)

	httpClient := initHttpClient()

	if cfg.Flags.SearchMBID != "" {
		if err := runSearch(&cfg, httpClient); err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		return
	}

	if cfg.Flags.RefreshOnly {
		// Unlike --search-mbid this runs unattended, from a schedule, so a failure
		// belongs in the notification channels.
		if err := client.TriggerRefresh(&cfg); err != nil {
			slog.Error(err.Error(), "notify", true)
			os.Exit(1)
		}
		slog.Info("library refresh triggered", "system", cfg.System)
		return
	}

	slog.Info("Starting Explo...")

	discovery := discovery.NewDiscoverer(cfg.DiscoveryCfg, httpClient)
	tracks, err := discovery.Discover()
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}

	client, err := client.NewClient(&cfg)
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}
	downloader, err := downloader.NewDownloader(&cfg.DownloadCfg, httpClient, cfg.Flags.ExcludeLocal)
	if err != nil {
		slog.Error(err.Error(), "notify", true)
		os.Exit(1)
	}
	if cfg.ReplacePlaylist {
		if err := client.DeletePlaylist(); err != nil {
			slog.Warn(err.Error(), "notify", true)
		}
	}
	if cfg.Flags.CleanDownloads && cfg.DownloadCfg.UseSubDir {
		downloader.DeleteSongs()
	}
	if cfg.Flags.DownloadMode != "force" {
		if err := client.CheckTracks(tracks); err != nil { // Check if tracks exist on system before downloading
			slog.Warn(err.Error(), "notify", true)
		}
	}

	if cfg.Flags.DownloadMode != "skip" {
		downloader.StartDownload(&tracks)
		if len(tracks) == 0 {
			slog.Error("couldn't download any tracks", "notify", true)
			os.Exit(1)
		}
	}

	if err := client.CreatePlaylist(tracks); err != nil {
		slog.Warn(err.Error())
	} else {
		slog.Info("playlist created successfully", "system", cfg.System, "playlistName", cfg.ClientCfg.PlaylistName, "notify", true)
	}
}

// runSearch resolves one MusicBrainz recording ID and reports whether the library
// already holds it. A diagnostic for working out why a track keeps being downloaded,
// or keeps being skipped.
func runSearch(cfg *config.Config, httpClient *util.HttpClient) error {
	lb := discovery.NewListenBrainz(cfg.DiscoveryCfg, httpClient)

	track, err := lb.LookupRecording(cfg.Flags.SearchMBID)
	if err != nil {
		return err
	}
	slog.Info("resolved recording",
		"title", track.CleanTitle,
		"artist", track.MainArtist,
		"album", track.Album,
		"duration_ms", track.Duration,
	)

	c, err := client.NewClient(cfg)
	if err != nil {
		return err
	}

	tracks := []*models.Track{track}
	if err := c.CheckTracks(tracks); err != nil {
		slog.Warn("library search failed", "context", err.Error())
	}

	if track.Present {
		slog.Info("found in library", "system", cfg.System, "id", track.ID)
	} else {
		slog.Info("not found in library", "system", cfg.System)
	}

	return nil
}
