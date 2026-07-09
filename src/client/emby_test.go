package client

import (
	"testing"

	"explo/src/models"
)

func TestFormatEmbySongs(t *testing.T) {
	tests := []struct {
		name   string
		tracks []*models.Track
		want   string
	}{
		{
			name: "only present tracks are included",
			tracks: []*models.Track{
				{ID: "1", Present: true},
				{ID: "2", Present: false},
				{ID: "3", Present: true},
			},
			want: "1,3",
		},
		{
			name: "no present tracks yields an empty string (edge case)",
			tracks: []*models.Track{
				{ID: "1", Present: false},
			},
			want: "",
		},
		{
			name:   "empty input yields an empty string (edge case)",
			tracks: []*models.Track{},
			want:   "",
		},
		{
			name: "single present track has no trailing comma",
			tracks: []*models.Track{
				{ID: "42", Present: true},
			},
			want: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatEmbySongs(tt.tracks); got != tt.want {
				t.Errorf("formatEmbySongs(%+v) = %q, want %q", tt.tracks, got, tt.want)
			}
		})
	}
}
