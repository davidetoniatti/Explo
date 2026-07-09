package util

import (
	"io"
	"net/http"
	"net/http/httptest"
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
			w.Write([]byte("chunk"))
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
