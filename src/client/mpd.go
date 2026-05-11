package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"log/slog"

	"explo/src/config"
	"explo/src/models"
)

type MPD struct {
	Cfg config.ClientConfig
}

func NewMPD(cfg config.ClientConfig) *MPD {
	return &MPD{Cfg: cfg}
}

func (c *MPD) GetLibrary() error {
	return nil
}

func (c *MPD) GetAuth() error {
	return nil
}

func (c *MPD) AddHeader() error {
	return nil
}

func (c *MPD) AddLibrary() error {
	return nil
}

func (c *MPD) SearchSongs(tracks []*models.Track) error {
	if c.Cfg.DownloadDir == "" {
		return nil
	}

	// Cache files in DownloadDir to avoid walking for each track
	fileMap := make(map[string]string)
	err := filepath.WalkDir(c.Cfg.DownloadDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileMap[d.Name()] = path
		}
		return nil
	})
	if err != nil {
		slog.Warn("failed to walk DownloadDir", "path", c.Cfg.DownloadDir, "error", err)
	}

	for i := range tracks {
		if tracks[i].File == "" {
			continue
		}

		if fullPath, ok := fileMap[tracks[i].File]; ok {
			tracks[i].File = fullPath
			tracks[i].Present = true
		} else {
			slog.Debug("Track not found in DownloadDir", "file", tracks[i].File)
		}
	}
	return nil
}

func (c *MPD) RefreshLibrary() error {
	return nil
}

func (c *MPD) CheckRefreshState() bool {
	return true
}

func (c *MPD) CreatePlaylist(tracks []*models.Track) error {
	f, err := os.OpenFile(c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	for _, track := range tracks {
		if track.Present {
			_, err := f.Write([]byte(track.File+"\n"))
			if err != nil {
				slog.Warn(fmt.Sprintf("failed to write song to file: %s", err.Error()))
			}
		}
	}
	return nil
}

func (c *MPD) SearchPlaylist() error {
	if _, err := os.Stat(c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u"); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("did not find playlist: %s", c.Cfg.PlaylistName)
	} else {
		c.Cfg.PlaylistID = c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u"
		return nil
	}
}

func (c *MPD) UpdatePlaylist() error {
	return nil
}

func (c *MPD) DeletePlaylist() error {
	if c.Cfg.PlaylistID != "" {
		if err := os.Remove(c.Cfg.PlaylistID); err != nil {
			return fmt.Errorf("failed to delete playlist: %s", err.Error())
		}
		return nil
	}
	return fmt.Errorf("playlist not found")
}