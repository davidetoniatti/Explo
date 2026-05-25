package config

import (
	"os"
	"testing"
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
