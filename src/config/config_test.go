package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfig_ReadEnv(t *testing.T) {
	// Setup environment variables
	os.Setenv("EXPLO_SYSTEM", "emby")
	os.Setenv("SYSTEM_URL", "http://localhost:8096")
	os.Setenv("API_KEY", "test-api-key")
	os.Setenv("DOWNLOAD_DIR", "/tmp/explo")

	defer func() {
		os.Unsetenv("EXPLO_SYSTEM")
		os.Unsetenv("SYSTEM_URL")
		os.Unsetenv("API_KEY")
		os.Unsetenv("DOWNLOAD_DIR")
	}()

	cfg := &Config{}
	cfg.ReadEnv()

	if cfg.System != "emby" {
		t.Errorf("expected System to be 'emby', got '%s'", cfg.System)
	}
	if cfg.ClientCfg.URL != "http://localhost:8096" {
		t.Errorf("expected URL to be 'http://localhost:8096', got '%s'", cfg.ClientCfg.URL)
	}
	if cfg.ClientCfg.Creds.APIKey != "test-api-key" {
		t.Errorf("expected APIKey to be 'test-api-key', got '%s'", cfg.ClientCfg.Creds.APIKey)
	}
}

func TestConfig_GenPlaylistName(t *testing.T) {
	cfg := &Config{}
	cfg.Flags.Playlist = "weekly-exploration"
	cfg.ClientCfg.PlaylistNFormat = "week"
	cfg.ReplacePlaylist = true
	cfg.DiscoveryCfg.Listenbrainz.User = "testuser"
	cfg.DownloadCfg.UseSubDir = true
	cfg.DownloadCfg.DownloadDir = "/tmp/explo"

	cfg.GenPlaylistName()

	// A replaced playlist keeps a stable name, so folderName should be
	// "Weekly-Exploration" and PlaylistName "Weekly Exploration"
	expectedPlaylistName := "Weekly Exploration"
	expectedDownloadDir := "/tmp/explo/Weekly-Exploration"

	if cfg.ClientCfg.PlaylistName != expectedPlaylistName {
		t.Errorf("expected PlaylistName to be '%s', got '%s'", expectedPlaylistName, cfg.ClientCfg.PlaylistName)
	}

	if cfg.DownloadCfg.DownloadDir != expectedDownloadDir {
		t.Errorf("expected DownloadDir to be '%s', got '%s'", expectedDownloadDir, cfg.DownloadCfg.DownloadDir)
	}
}

func TestGetPlaylistName(t *testing.T) {
	now := time.Now()
	year, week := now.ISOWeek()

	tests := []struct {
		name         string
		playlistType string
		format       string
		replace      bool
		want         string
	}{
		{
			// A playlist replaced in place keeps one name, so there is nothing to date
			name:         "replacing always uses the base name, whatever the format",
			playlistType: "weekly-exploration",
			format:       "date",
			replace:      true,
			want:         "Weekly-Exploration",
		},
		{
			name:         "explicit date format",
			playlistType: "weekly-exploration",
			format:       "date",
			replace:      false,
			want:         fmt.Sprintf("Weekly-Exploration-%s", now.Format("2006-01-02")),
		},
		{
			name:         "daily-jams special naming",
			playlistType: "daily-jams",
			format:       "week", // format is ignored for daily-jams
			replace:      false,
			want:         fmt.Sprintf("Daily-Jams-%d-Day%d", now.Year(), now.YearDay()),
		},
		{
			name:         "keeping past playlists uses the ISO week",
			playlistType: "weekly-exploration",
			format:       "week",
			replace:      false,
			want:         fmt.Sprintf("Weekly-Exploration-%d-Week%d", year, week),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPlaylistName(tt.playlistType, tt.format, tt.replace)
			if got != tt.want {
				t.Errorf("getPlaylistName(%q, %q, %v) = %q, want %q", tt.playlistType, tt.format, tt.replace, got, tt.want)
			}
		})
	}
}

func TestFixDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "empty string stays empty", dir: "", want: ""},
		{name: "adds trailing slash when missing", dir: "/data", want: "/data/"},
		{name: "leaves trailing slash untouched", dir: "/data/", want: "/data/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixDir(tt.dir); got != tt.want {
				t.Errorf("fixDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestFixBaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "empty string stays empty", url: "", want: ""},
		{name: "whitespace-only string becomes empty", url: "   ", want: ""},
		{name: "adds scheme when missing", url: "example.com", want: "http://example.com"},
		{name: "trims trailing slash", url: "https://example.com/", want: "https://example.com"},
		{name: "trims surrounding whitespace", url: "  https://example.com  ", want: "https://example.com"},
		{name: "no scheme and trailing slash combined", url: "example.com/", want: "http://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixBaseURL(tt.url); got != tt.want {
				t.Errorf("fixBaseURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestShareDownloadSettings(t *testing.T) {
	t.Run("output settings reach every downloader", func(t *testing.T) {
		// The downloader constructors only receive their own sub-struct, so anything
		// configured once at the top level has to be copied down or it silently
		// arrives empty.
		cfg := &Config{}
		cfg.DownloadCfg.PathTemplate = "{{Artist}}/{{TrackName}}.{{ext}}"
		cfg.DownloadCfg.EmbedCoverArt = true
		cfg.DownloadCfg.CoversDir = "/tmp/covers"

		cfg.shareDownloadSettings()

		for name, got := range map[string]struct {
			pathTemplate  string
			coversDir     string
			embedCoverArt bool
		}{
			"youtube": {cfg.DownloadCfg.Youtube.PathTemplate, cfg.DownloadCfg.Youtube.CoversDir, cfg.DownloadCfg.Youtube.EmbedCoverArt},
			"qobuz":   {cfg.DownloadCfg.Qobuz.PathTemplate, cfg.DownloadCfg.Qobuz.CoversDir, cfg.DownloadCfg.Qobuz.EmbedCoverArt},
		} {
			if got.pathTemplate != "{{Artist}}/{{TrackName}}.{{ext}}" {
				t.Errorf("%s: PathTemplate = %q", name, got.pathTemplate)
			}
			if got.coversDir != "/tmp/covers" {
				t.Errorf("%s: CoversDir = %q", name, got.coversDir)
			}
			if !got.embedCoverArt {
				t.Errorf("%s: EmbedCoverArt was not propagated", name)
			}
		}
	})

	t.Run("an unset covers dir defaults under the temp dir", func(t *testing.T) {
		cfg := &Config{}

		cfg.shareDownloadSettings()

		if cfg.DownloadCfg.CoversDir == "" {
			t.Fatal("expected a default covers directory")
		}
		if !strings.HasPrefix(cfg.DownloadCfg.CoversDir, os.TempDir()) {
			t.Errorf("covers dir should live under the temp dir, got %q", cfg.DownloadCfg.CoversDir)
		}
	})

	t.Run("a configured covers dir is respected", func(t *testing.T) {
		cfg := &Config{}
		cfg.DownloadCfg.CoversDir = "/custom/covers"

		cfg.shareDownloadSettings()

		if cfg.DownloadCfg.CoversDir != "/custom/covers" {
			t.Errorf("CoversDir = %q, want /custom/covers", cfg.DownloadCfg.CoversDir)
		}
	})
}

func TestResolveReplacePlaylist(t *testing.T) {
	// Precedence: --replace-playlist, then the deprecated --persist, then
	// REPLACE_PLAYLIST, then the deprecated PERSIST, then the flag default.
	tests := []struct {
		name  string
		setup func(*Config)
		env   map[string]string
		want  bool
	}{
		{
			name:  "defaults to replacing",
			setup: func(c *Config) { c.Flags.ReplacePlaylist = true },
			want:  true,
		},
		{
			name: "the flag wins over everything",
			setup: func(c *Config) {
				c.Flags.ReplacePlaylist = false
				c.Flags.ReplacePlaylistSet = true
				c.Flags.Persist = false // would otherwise mean replace=true
				c.Flags.PersistSet = true
			},
			env:  map[string]string{"REPLACE_PLAYLIST": "true", "PERSIST": "false"},
			want: false,
		},
		{
			name: "the deprecated flag is honoured, inverted",
			setup: func(c *Config) {
				c.Flags.Persist = true // keep past playlists
				c.Flags.PersistSet = true
			},
			want: false,
		},
		{
			name: "the deprecated flag set to false means replace",
			setup: func(c *Config) {
				c.Flags.Persist = false
				c.Flags.PersistSet = true
			},
			want: true,
		},
		{
			name: "the deprecated flag beats both variables",
			setup: func(c *Config) {
				c.Flags.Persist = true
				c.Flags.PersistSet = true
				c.ReplacePlaylistENV = true
			},
			env:  map[string]string{"REPLACE_PLAYLIST": "true"},
			want: false,
		},
		{
			name:  "REPLACE_PLAYLIST is used when no flag was given",
			setup: func(c *Config) { c.ReplacePlaylistENV = false },
			env:   map[string]string{"REPLACE_PLAYLIST": "false"},
			want:  false,
		},
		{
			name:  "the deprecated variable is honoured, inverted",
			setup: func(c *Config) { c.PersistENV = true },
			env:   map[string]string{"PERSIST": "true"},
			want:  false,
		},
		{
			name:  "the deprecated variable set to false means replace",
			setup: func(c *Config) { c.PersistENV = false },
			env:   map[string]string{"PERSIST": "false"},
			want:  true,
		},
		{
			name: "REPLACE_PLAYLIST beats the deprecated variable",
			setup: func(c *Config) {
				c.ReplacePlaylistENV = true
				c.PersistENV = true // would otherwise mean replace=false
			},
			env:  map[string]string{"REPLACE_PLAYLIST": "true", "PERSIST": "true"},
			want: true,
		},
		{
			name: "an unset PERSIST does not override the default (edge case)",
			// cleanenv defaults PersistENV to true, which inverted would mean
			// replace=false. Only a variable actually supplied may have that effect.
			setup: func(c *Config) {
				c.Flags.ReplacePlaylist = true
				c.PersistENV = true
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg := &Config{}
			tt.setup(cfg)

			if got := cfg.resolveReplacePlaylist(); got != tt.want {
				t.Errorf("resolveReplacePlaylist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplacePlaylistFromEnvFile(t *testing.T) {
	// The deprecated variable is usually supplied through the .env file rather than
	// the real environment. cleanenv exports the file into the environment as it
	// reads it, which is what lets envIsSet tell "supplied" from "defaulted" apart.
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("EXPLO_SYSTEM=emby\nPERSIST=false\n"), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}
	t.Cleanup(func() {
		os.Unsetenv("EXPLO_SYSTEM")
		os.Unsetenv("PERSIST")
	})

	cfg := &Config{}
	cfg.Flags.CfgPath = envFile
	cfg.Flags.ReplacePlaylist = true // the flag default, not explicitly set
	cfg.ReadEnv()
	cfg.MergeFlags()

	// PERSIST=false is the inverse of REPLACE_PLAYLIST=true
	if !cfg.ReplacePlaylist {
		t.Errorf("expected PERSIST=false in the .env file to mean replace, got %v", cfg.ReplacePlaylist)
	}
	if !envIsSet("PERSIST") {
		t.Error("expected envIsSet to see a variable supplied through the .env file")
	}
}

func TestHandleDeprecationWarnsAboutLostCleanup(t *testing.T) {
	// --persist=false used to delete downloaded tracks as well as the playlist. The
	// warning is the only thing telling an upgrading user that half of what they asked
	// for now needs a second flag.
	capture := func(cfg *Config) string {
		var buf bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
		defer slog.SetDefault(previous)

		cfg.HandleDeprecation()
		return buf.String()
	}

	const want = "clean-downloads"

	t.Run("warns when the deprecated flag asked for a replace", func(t *testing.T) {
		cfg := &Config{}
		cfg.Flags.Persist = false
		cfg.Flags.PersistSet = true

		if got := capture(cfg); !strings.Contains(got, want) {
			t.Errorf("expected a warning mentioning %q, got %q", want, got)
		}
	})

	t.Run("warns when the deprecated variable asked for a replace", func(t *testing.T) {
		t.Setenv("PERSIST", "false")
		cfg := &Config{}
		cfg.PersistENV = false

		if got := capture(cfg); !strings.Contains(got, want) {
			t.Errorf("expected a warning mentioning %q, got %q", want, got)
		}
	})

	t.Run("stays quiet once --clean-downloads is passed", func(t *testing.T) {
		cfg := &Config{}
		cfg.Flags.Persist = false
		cfg.Flags.PersistSet = true
		cfg.Flags.CleanDownloads = true
		cfg.DownloadCfg.UseSubDir = true

		if got := capture(cfg); strings.Contains(got, want) {
			t.Errorf("expected no cleanup warning, got %q", got)
		}
	})

	t.Run("stays quiet when the deprecated setting kept playlists", func(t *testing.T) {
		// --persist=true never deleted anything, so there is nothing to warn about
		cfg := &Config{}
		cfg.Flags.Persist = true
		cfg.Flags.PersistSet = true

		if got := capture(cfg); strings.Contains(got, want) {
			t.Errorf("expected no cleanup warning, got %q", got)
		}
	})

	t.Run("stays quiet when nothing deprecated is used", func(t *testing.T) {
		if got := capture(&Config{}); got != "" {
			t.Errorf("expected no warnings at all, got %q", got)
		}
	})
}
