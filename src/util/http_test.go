package util

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGetStream_OutlivesAPITimeout ensures GetStream (used for file downloads) is not bound
// by the short Timeout used for regular API calls (Client). Regression test for the bug where
// http.Client.Timeout bounds the entire exchange including reading the response body, which
// aborted multi-second downloads.
func TestGetStream_OutlivesAPITimeout(t *testing.T) {
	const apiTimeoutSeconds = 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// stream slower than the configured API timeout, but well within the download client's bounds
		for i := 0; i < 3; i++ {
			time.Sleep(time.Duration(apiTimeoutSeconds) * time.Second)
			if _, err := w.Write([]byte("chunk")); err != nil {
				t.Errorf("failed writing test response: %v", err)
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	c := NewHttp(HttpClientConfig{Timeout: apiTimeoutSeconds})

	stream, err := c.GetStream(server.URL, nil)
	if err != nil {
		t.Fatalf("GetStream returned error: %v", err)
	}
	defer stream.Close()

	body, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("reading stream body failed (likely truncated by a total timeout): %v", err)
	}

	want := "chunkchunkchunk"
	if string(body) != want {
		t.Fatalf("got body %q, want %q", string(body), want)
	}
}

func TestCoverFilename(t *testing.T) {
	t.Run("is stable for the same url", func(t *testing.T) {
		url := "https://coverartarchive.org/release/abc-123/front-250"

		first, second := coverFilename(url), coverFilename(url)

		if first != second {
			t.Errorf("expected the same url to map to the same cache file, got %q and %q", first, second)
		}
	})

	t.Run("differs between urls", func(t *testing.T) {
		a := coverFilename("https://coverartarchive.org/release/abc/front-250")
		b := coverFilename("https://coverartarchive.org/release/def/front-250")

		if a == b {
			t.Errorf("expected different urls to map to different files, both gave %q", a)
		}
	})

	t.Run("cover size is part of the identity (edge case)", func(t *testing.T) {
		// The same release at two sizes must not share one cache entry.
		small := coverFilename("https://coverartarchive.org/release/abc/front-250")
		large := coverFilename("https://coverartarchive.org/release/abc/front-1200")

		if small == large {
			t.Error("expected different cover sizes to be cached separately")
		}
	})

	t.Run("never produces a path separator (security)", func(t *testing.T) {
		// The name is joined onto the covers directory, so a separator or ".." coming
		// out of a hostile URL would let the write escape it.
		name := coverFilename("https://example.com/../../etc/passwd")

		if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			t.Errorf("cover file name is not filesystem safe: %q", name)
		}
	})
}

func TestDownloadCover(t *testing.T) {
	t.Run("downloads and caches the image", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if _, err := w.Write([]byte("jpeg-bytes")); err != nil {
				t.Errorf("failed writing test response: %v", err)
			}
		}))
		defer srv.Close()

		c := NewHttp(HttpClientConfig{Timeout: 5})
		dir := t.TempDir()

		path, err := c.DownloadCover(srv.URL+"/front-250", dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cover was not written: %v", err)
		}
		if string(data) != "jpeg-bytes" {
			t.Errorf("cover content = %q", data)
		}

		// A second call for the same URL must reuse the cached file.
		if _, err := c.DownloadCover(srv.URL+"/front-250", dir); err != nil {
			t.Fatalf("unexpected error on cached call: %v", err)
		}
		if hits != 1 {
			t.Errorf("expected the cover to be fetched once, got %d requests", hits)
		}
	})

	t.Run("leaves no partial file behind when the server errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewHttp(HttpClientConfig{Timeout: 5})
		dir := t.TempDir()

		if _, err := c.DownloadCover(srv.URL+"/missing", dir); err == nil {
			t.Fatal("expected an error for a 404 response")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed reading covers dir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected no files to be left behind, got %v", entries)
		}
	})

	t.Run("empty url and missing directory are rejected", func(t *testing.T) {
		c := NewHttp(HttpClientConfig{Timeout: 5})

		if _, err := c.DownloadCover("", t.TempDir()); err == nil {
			t.Error("expected an error for an empty url")
		}
		if _, err := c.DownloadCover("https://example.com/a.jpg", ""); err == nil {
			t.Error("expected an error for an empty covers directory")
		}
	})
}

func TestDownloadCoverRejectsEmptyResponses(t *testing.T) {
	// An empty cover would be cached as a valid image and then handed to ffmpeg as a
	// mandatory input, breaking every later conversion that reuses it.
	t.Run("an empty body is not cached", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK) // 200 with no body
		}))
		defer srv.Close()

		c := NewHttp(HttpClientConfig{Timeout: 5})
		dir := t.TempDir()

		if _, err := c.DownloadCover(srv.URL+"/empty", dir); err == nil {
			t.Fatal("expected an error for an empty cover")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed reading covers dir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected nothing to be cached, got %v", entries)
		}
	})

	t.Run("a zero byte cache entry is re-fetched", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte("jpeg-bytes")); err != nil {
				t.Errorf("failed writing test response: %v", err)
			}
		}))
		defer srv.Close()

		c := NewHttp(HttpClientConfig{Timeout: 5})
		dir := t.TempDir()
		coverURL := srv.URL + "/front-250"

		// Seed the cache with the kind of empty file an interrupted earlier run leaves.
		stale := filepath.Join(dir, coverFilename(coverURL))
		if err := os.WriteFile(stale, nil, 0644); err != nil {
			t.Fatalf("failed to seed cache: %v", err)
		}

		path, err := c.DownloadCover(coverURL, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed reading cover: %v", err)
		}
		if string(data) != "jpeg-bytes" {
			t.Errorf("expected the empty cache entry to be replaced, got %q", data)
		}
	})
}
