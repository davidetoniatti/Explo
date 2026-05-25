package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"net/url"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type EmbyPaths []struct {
	Name           string         `json:"Name"`
	Locations      []string       `json:"Locations"`
	CollectionType string         `json:"CollectionType"`
	ItemID         string         `json:"ItemId"`
	RefreshStatus  string         `json:"RefreshStatus"`
}

type EmbyItemSearch struct {
	Items            []EmbyItems `json:"Items"`
	TotalRecordCount int     `json:"TotalRecordCount"`
}

type EmbyItems struct {
	Name              string          `json:"Name"`
	ServerID          string          `json:"ServerId"`
	ID                string          `json:"Id"`
	Path			  string		  `json:"Path"`
	Album             string          `json:"Album,omitempty"`
	AlbumArtist       string          `json:"AlbumArtist,omitempty"`
	Artists           []string  	  `json:"Artists"`
}

type EmbyPlaylist struct {
	ID string `json:"Id"`
}

type Emby struct {
	LibraryID string
	HttpClient *util.HttpClient
	Cfg config.ClientConfig
}

func NewEmby(cfg config.ClientConfig, httpClient *util.HttpClient) *Emby {
	return &Emby{Cfg: cfg,
	HttpClient: httpClient}
}

func (c *Emby) AddHeader() error {
	if c.Cfg.Creds.Headers == nil {
		c.Cfg.Creds.Headers = make(map[string]string)
		c.Cfg.Creds.Headers["X-Emby-Client"] = c.Cfg.ClientID
	}

	if c.Cfg.Creds.APIKey != "" {
		c.Cfg.Creds.Headers["X-Emby-Token"] = c.Cfg.Creds.APIKey
		return nil
	}
	return fmt.Errorf("API_KEY not set")
}

func (c *Emby) GetAuth() error {
	return nil
}

func (c *Emby) GetLibrary() error {
	reqParam := "/emby/Library/VirtualFolders"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var paths EmbyPaths
	if err = util.ParseResp(body, &paths); err != nil {
		return err
	}
	for _, path := range paths {
		if path.Name == c.Cfg.LibraryName {
			c.LibraryID = path.ItemID
			return nil
		}
	}

	return fmt.Errorf("failed to find library named %s", c.Cfg.LibraryName)
}

func (c *Emby) AddLibrary() error {
	reqParam := "/emby/Library/VirtualFolders"

	type LibraryOptions struct {
		Enabled               bool `json:"Enabled"`
		EnableRealtimeMonitor bool `json:"EnableRealtimeMonitor"`
		EnableLUFSScan        bool `json:"EnableLUFSScan"`
	}

	type AddLibraryRequest struct {
		Name           string         `json:"Name"`
		CollectionType string         `json:"CollectionType"`
		RefreshLibrary bool           `json:"RefreshLibrary"`
		Paths          []string       `json:"Paths"`
		LibraryOptions LibraryOptions `json:"LibraryOptions"`
	}

	req := AddLibraryRequest{
		Name:           c.Cfg.LibraryName,
		CollectionType: "Music",
		RefreshLibrary: true,
		Paths:          []string{c.Cfg.DownloadDir},
		LibraryOptions: LibraryOptions{
			Enabled:               true,
			EnableRealtimeMonitor: true,
			EnableLUFSScan:        false,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal Emby library request: %w", err)
	}

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, bytes.NewReader(payload), c.Cfg.Creds.Headers); err != nil {
		return fmt.Errorf("failed to add library to Emby using the download path, please define a library name using LIBRARY_NAME in .env: %s", err.Error())
	}
	return nil
}

func (c *Emby) RefreshLibrary() error {
	reqParam := fmt.Sprintf("/emby/Items/%s/Refresh?Recursive=True&MetadataRefreshMode=FullRefresh", c.LibraryID)

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Emby) CheckRefreshState() bool {
	reqParam := "/emby/Library/VirtualFolders"

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		slog.Warn("failed to check Emby refresh state", "error", err)
		return false
	}

	var paths EmbyPaths
	if err = util.ParseResp(body, &paths); err != nil {
		slog.Warn("failed to parse Emby virtual folders for refresh state", "error", err)
		return false
	}

	for _, path := range paths {
		if path.Name == c.Cfg.LibraryName {
			// RefreshStatus is usually empty when not refreshing, or contains percentage/state
			// If it's not empty, it's likely still refreshing.
			return path.RefreshStatus == ""
		}
	}

	return false
}

func (c *Emby) SearchSongs(tracks []*models.Track) error {
	for _, track := range tracks {
		reqParam := fmt.Sprintf("/emby/Items?IncludeMediaTypes=Audio&SearchTerm=%s&Recursive=true&Fields=Path", url.QueryEscape(track.CleanTitle))

		body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
		if err != nil {
			return err
		}

		var results EmbyItemSearch
		if err = util.ParseResp(body, &results); err != nil {
			return err
		}

		for _, item := range results.Items {
			if strings.EqualFold(track.MainArtist, item.AlbumArtist) && (strings.EqualFold(item.Name, track.CleanTitle) || (track.File != "" && strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File)))) {
				track.ID = item.ID
				track.Present = true
				break
			}

			if track.File != "" && len(item.Artists) > 0 &&
				strings.Contains(strings.ToLower(item.Artists[0]), strings.ToLower(track.MainArtist)) &&
				strings.Contains(strings.ToLower(item.Path), strings.ToLower(track.File)) {
				track.ID = item.ID
				track.Present = true
				break
			}
		}

		if !track.Present {
			slog.Debug(fmt.Sprintf("[emby] failed to find '%s' by '%s' in album '%s'", track.Title, track.Artist, track.Album))
		}
	}
	return nil
}

func (c *Emby) SearchPlaylist() error {
	params := fmt.Sprintf("/emby/Items?SearchTerm=%s&Recursive=true&IncludeItemTypes=Playlist", url.QueryEscape(c.Cfg.PlaylistName))

	body, err := c.HttpClient.MakeRequest("GET", c.Cfg.URL+params, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}

	var results EmbyItemSearch
	if err = util.ParseResp(body, &results); err != nil {
		return err
	}

	if len(results.Items) != 0 {
		c.Cfg.PlaylistID = results.Items[0].ID
		return nil
	} else {
		return fmt.Errorf("no results found for %s", c.Cfg.PlaylistName)
	}
}

func (c *Emby) CreatePlaylist(tracks []*models.Track) error {
	songIDs := formatEmbySongs(tracks)

	reqParam := fmt.Sprintf("/emby/Playlists?Name=%s&Ids=%s&MediaType=Music", url.QueryEscape(c.Cfg.PlaylistName), songIDs)


	body, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers)
	if err != nil {
		return err
	}
	var playlist EmbyPlaylist
	if err = util.ParseResp(body, &playlist); err != nil {
		return err
	}
	c.Cfg.PlaylistID = playlist.ID
	return nil
}

func (c *Emby) UpdatePlaylist() error {
	time.Sleep(5 * time.Second) // small buffer between playlist creation and updating, Emby doesn't update playlist otherwise
	reqParam := fmt.Sprintf("/emby/Items/%s", c.Cfg.PlaylistID)

	type UpdatePlaylistRequest struct {
		ID          string            `json:"Id"`
		Name        string            `json:"Name"`
		Overview    string            `json:"Overview"`
		ProviderIDs map[string]string `json:"ProviderIds"`
	}

	req := UpdatePlaylistRequest{
		ID:          c.Cfg.PlaylistID,
		Name:        c.Cfg.PlaylistName,
		Overview:    c.Cfg.PlaylistDescr,
		ProviderIDs: make(map[string]string),
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal Emby playlist update request: %w", err)
	}

	if _, err := c.HttpClient.MakeRequest("POST", c.Cfg.URL+reqParam, bytes.NewBuffer(payload), c.Cfg.Creds.Headers); err != nil {
		return err
	}
	return nil
}

func (c *Emby) DeletePlaylist() error { // Doesn't currently work due to a bug in Emby
	/* reqParam := fmt.Sprintf("/emby/Items/Delete?Ids=%s", c.Cfg.PlaylistID)

	if _, err := util.MakeRequest("POST", c.Cfg.URL+reqParam, nil, c.Cfg.Creds.Headers); err != nil {
		return err
	} */
	return nil
}

func formatEmbySongs(tracks []*models.Track) string {
	songIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.Present {
			songIDs = append(songIDs,track.ID)
		}
	}
	songs := strings.Join(songIDs, ",")

	return songs
}