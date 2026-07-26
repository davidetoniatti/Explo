package config

import (
	"fmt"
	"os"
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
	cfg.Persist = false
	cfg.DiscoveryCfg.Listenbrainz.User = "testuser"
	cfg.DownloadCfg.UseSubDir = true
	cfg.DownloadCfg.DownloadDir = "/tmp/explo"

	cfg.GenPlaylistName()

	// Since persist is false, folderName should be "Weekly-Exploration"
	// and PlaylistName should be "Weekly Exploration"
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
		persist      bool
		want         string
	}{
		{
			name:         "non-persist always uses base name regardless of format",
			playlistType: "weekly-exploration",
			format:       "date",
			persist:      false,
			want:         "Weekly-Exploration",
		},
		{
			name:         "explicit date format",
			playlistType: "weekly-exploration",
			format:       "date",
			persist:      true,
			want:         fmt.Sprintf("Weekly-Exploration-%s", now.Format("2006-01-02")),
		},
		{
			name:         "daily-jams special naming",
			playlistType: "daily-jams",
			format:       "week", // format is ignored for daily-jams
			persist:      true,
			want:         fmt.Sprintf("Daily-Jams-%d-Day%d", now.Year(), now.YearDay()),
		},
		{
			name:         "default persistent naming uses ISO week",
			playlistType: "weekly-exploration",
			format:       "week",
			persist:      true,
			want:         fmt.Sprintf("Weekly-Exploration-%d-Week%d", year, week),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPlaylistName(tt.playlistType, tt.format, tt.persist)
			if got != tt.want {
				t.Errorf("getPlaylistName(%q, %q, %v) = %q, want %q", tt.playlistType, tt.format, tt.persist, got, tt.want)
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
