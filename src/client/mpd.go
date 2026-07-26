package client

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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

		// A download organised by PATH_TEMPLATE knows exactly where it was written.
		// Trust that over the file name, which several albums can share once the
		// template drops the artist from it (two "01 - Intro.flac", say).
		if tracks[i].RelPath != "" {
			fullPath := filepath.Join(c.Cfg.DownloadDir, tracks[i].RelPath)
			if _, err := os.Stat(fullPath); err == nil {
				tracks[i].File = fullPath
				tracks[i].Present = true
				continue
			}
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
	playlistPath := c.Cfg.PlaylistDir + c.Cfg.PlaylistName + ".m3u"

	// Read existing entries so re-running (e.g. with --persist) doesn't duplicate lines.
	existing := make(map[string]bool)
	if data, err := os.ReadFile(playlistPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				existing[line] = true
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Warn("failed to read existing playlist", "error", err.Error())
	}

	f, err := os.OpenFile(playlistPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Error(fmt.Sprintf("failed to close playlist file: %s", cerr.Error()))
		}
	}()

	for _, track := range tracks {
		if track.Present && !existing[track.File] {
			_, err := f.Write([]byte(track.File + "\n"))
			if err != nil {
				slog.Warn(fmt.Sprintf("failed to write song to file: %s", err.Error()))
			}
		}
	}
	return nil
}

func (c *MPD) SearchPlaylist() error {
	if _, err := os.Stat(c.Cfg.PlaylistDir + c.Cfg.PlaylistName + ".m3u"); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("did not find playlist: %s", c.Cfg.PlaylistName)
	} else {
		c.Cfg.PlaylistID = c.Cfg.PlaylistDir + c.Cfg.PlaylistName + ".m3u"
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
