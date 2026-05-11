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
