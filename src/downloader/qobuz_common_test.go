package downloader

import "testing"

func TestQobuzQualityID(t *testing.T) {
	tests := []struct {
		name    string
		quality string
		want    int
	}{
		{name: "FLAC_24_192 alias", quality: "FLAC_24_192", want: 27},
		{name: "27 numeric string", quality: "27", want: 27},
		{name: "FLAC_24_96 alias", quality: "FLAC_24_96", want: 7},
		{name: "7 numeric string", quality: "7", want: 7},
		{name: "FLAC_16 alias", quality: "FLAC_16", want: 6},
		{name: "FLAC alias", quality: "FLAC", want: 6},
		{name: "MP3_320 alias", quality: "MP3_320", want: 5},
		{name: "lowercase input is case-insensitive", quality: "mp3", want: 5},
		{name: "unknown value defaults to best quality (edge case)", quality: "bogus", want: 27},
		{name: "empty string defaults to best quality (edge case)", quality: "", want: 27},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qobuzQualityID(tt.quality); got != tt.want {
				t.Errorf("qobuzQualityID(%q) = %d, want %d", tt.quality, got, tt.want)
			}
		})
	}
}

func TestQobuzExtension(t *testing.T) {
	tests := []struct {
		name     string
		formatId int
		want     string
	}{
		{name: "format 5 is mp3", formatId: 5, want: "mp3"},
		{name: "format 6 is flac", formatId: 6, want: "flac"},
		{name: "format 7 is flac", formatId: 7, want: "flac"},
		{name: "format 27 is flac", formatId: 27, want: "flac"},
		{name: "unknown format defaults to flac (edge case)", formatId: 0, want: "flac"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qobuzExtension(tt.formatId); got != tt.want {
				t.Errorf("qobuzExtension(%d) = %q, want %q", tt.formatId, got, tt.want)
			}
		})
	}
}

func TestDecodeBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "already-padded input", input: "aGVsbG8=", want: "hello"},
		{name: "missing one padding character (edge case)", input: "aGVsbG8", want: "hello"},
		{name: "no padding needed", input: "aGk=", want: "hi"},
		{name: "surrounding whitespace is trimmed", input: "  aGVsbG8=  ", want: "hello"},
		{name: "invalid base64 returns an error", input: "not-valid-base64!!", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("decodeBase64(%q) expected an error, got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeBase64(%q) unexpected error: %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("decodeBase64(%q) = %q, want %q", tt.input, string(got), tt.want)
			}
		})
	}
}
