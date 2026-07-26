package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"explo/src/logging"
)

type HttpClientConfig struct {
	Timeout int
}

type HttpClient struct {
	Client       *http.Client // used for regular API calls, bounded by Timeout
	StreamClient *http.Client // used for GetStream (file downloads); no total timeout, since it also bounds reading the body
	UserAgent    string
}

func NewHttp(cfg HttpClientConfig) *HttpClient {
	return &HttpClient{
		Client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		StreamClient: &http.Client{
			// no overall Timeout: http.Client.Timeout bounds the entire exchange, including
			// reading the response body, which would abort large file downloads mid-stream.
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: time.Duration(cfg.Timeout) * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   time.Duration(cfg.Timeout) * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		UserAgent: "Explo (+https://github.com/davidetoniatti/Explo)",
	}
}

func (c *HttpClient) MakeRequest(method, url string, payload io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %s", err.Error())
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", c.UserAgent)

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %s", err.Error())
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("response body close failed", "context", err.Error())
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err.Error())
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Debug("response info", logging.RuntimeAttr(string(body)))
		return nil, fmt.Errorf("got %d from %s", resp.StatusCode, url)
	}

	return body, nil
}

func (c *HttpClient) GetStream(url string, headers map[string]string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request: %s", err.Error())
	}
	req.Header.Add("User-Agent", c.UserAgent)

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	resp, err := c.StreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %s", err.Error())
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("response body close failed", "context", cerr.Error())
		}
		return nil, fmt.Errorf("got %d from %s", resp.StatusCode, url)
	}

	return resp.Body, nil
}

// DownloadCover fetches coverURL into coversDir and returns the local path.
// Images are cached under a name derived from the URL, so every track from the same
// release reuses one download. An already cached file is returned untouched.
func (c *HttpClient) DownloadCover(coverURL, coversDir string) (string, error) {
	if coverURL == "" {
		return "", fmt.Errorf("no cover art URL")
	}
	if coversDir == "" {
		return "", fmt.Errorf("no covers directory configured")
	}

	destPath := filepath.Join(coversDir, coverFilename(coverURL))
	// An empty cache entry would otherwise be handed to ffmpeg as a valid cover and
	// fail every conversion that reuses it, for as long as the file survives.
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		return destPath, nil
	}

	if err := os.MkdirAll(coversDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create covers directory: %w", err)
	}

	stream, err := c.GetStream(coverURL, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			slog.Warn("cover stream close failed", "context", cerr.Error())
		}
	}()

	// Write to a temp file first, so an interrupted download cannot be picked up as
	// a valid cache entry by the next run.
	tmp, err := os.CreateTemp(coversDir, "cover-*.part")
	if err != nil {
		return "", fmt.Errorf("failed to create temp cover file: %w", err)
	}
	tmpName := tmp.Name()

	written, copyErr := io.Copy(tmp, stream)
	closeErr := tmp.Close()

	if copyErr == nil && closeErr == nil && written == 0 {
		copyErr = fmt.Errorf("cover at %s was empty", coverURL)
	}

	if copyErr != nil || closeErr != nil {
		if rerr := os.Remove(tmpName); rerr != nil {
			slog.Warn("failed to remove temp cover file", "file", tmpName, "context", rerr.Error())
		}
		return "", errors.Join(copyErr, closeErr)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		if rerr := os.Remove(tmpName); rerr != nil {
			slog.Warn("failed to remove temp cover file", "file", tmpName, "context", rerr.Error())
		}
		return "", fmt.Errorf("failed to store cover: %w", err)
	}

	return destPath, nil
}

// coverFilename derives a cache file name from the cover URL. Hashing keeps the name
// filesystem safe whatever the URL looks like, and rules out path traversal.
func coverFilename(coverURL string) string {
	sum := sha256.Sum256([]byte(coverURL))
	return hex.EncodeToString(sum[:8]) + ".jpg"
}

func ParseResp[T any](body []byte, target *T) error {

	if err := json.Unmarshal(body, target); err != nil {
		slog.Debug("response info", logging.RuntimeAttr(string(body)))
		return fmt.Errorf("error unmarshaling response body: %s", err.Error())
	}
	return nil
}
