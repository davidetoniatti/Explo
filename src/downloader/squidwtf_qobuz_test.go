package downloader

import (
	"encoding/json"
	"testing"
)

func TestQobuzName_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    QobuzName
		wantErr bool
	}{
		{name: "plain string value", input: `"Some Artist"`, want: "Some Artist"},
		{name: "object with display field", input: `{"display":"Some Artist"}`, want: "Some Artist"},
		{name: "object without display field yields empty string (edge case)", input: `{"other":"value"}`, want: ""},
		{name: "empty string value", input: `""`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var qn QobuzName
			err := json.Unmarshal([]byte(tt.input), &qn)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error unmarshaling %q, got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error unmarshaling %q: %v", tt.input, err)
			}
			if qn != tt.want {
				t.Errorf("UnmarshalJSON(%q) = %q, want %q", tt.input, qn, tt.want)
			}
		})
	}

	t.Run("used as a struct field via QobuzArtist", func(t *testing.T) {
		var artist QobuzArtist
		if err := json.Unmarshal([]byte(`{"id":1,"name":{"display":"Nested Artist"}}`), &artist); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if artist.Name != "Nested Artist" {
			t.Errorf("artist.Name = %q, want %q", artist.Name, "Nested Artist")
		}
	})
}
