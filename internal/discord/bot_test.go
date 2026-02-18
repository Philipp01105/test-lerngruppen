package discord

import (
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestParseGroupID(t *testing.T) {
	tests := []struct {
		name     string
		customID string
		prefix   string
		wantID   int64
		wantErr  bool
	}{
		{
			name:     "valid join ID",
			customID: "join_42",
			prefix:   "join_",
			wantID:   42,
			wantErr:  false,
		},
		{
			name:     "valid leave ID",
			customID: "leave_123",
			prefix:   "leave_",
			wantID:   123,
			wantErr:  false,
		},
		{
			name:     "valid add_event ID",
			customID: "add_event_99",
			prefix:   "add_event_",
			wantID:   99,
			wantErr:  false,
		},
		{
			name:     "invalid non-numeric",
			customID: "join_abc",
			prefix:   "join_",
			wantID:   0,
			wantErr:  true,
		},
		{
			name:     "empty after prefix",
			customID: "join_",
			prefix:   "join_",
			wantID:   0,
			wantErr:  true,
		},
		{
			name:     "large ID",
			customID: "join_9999999999",
			prefix:   "join_",
			wantID:   9999999999,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := parseGroupID(tt.customID, tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGroupID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotID != tt.wantID {
				t.Errorf("parseGroupID() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestBuildTagNameToIDMap(t *testing.T) {
	tags := []discordgo.ForumTag{
		{ID: "1", Name: "FIAE"},
		{ID: "2", Name: "FISI"},
		{ID: "3", Name: "FIDP"},
	}

	result := buildTagNameToIDMap(tags)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	if result["FIAE"] != "1" {
		t.Errorf("expected FIAE -> 1, got %q", result["FIAE"])
	}
	if result["FISI"] != "2" {
		t.Errorf("expected FISI -> 2, got %q", result["FISI"])
	}
	if result["FIDP"] != "3" {
		t.Errorf("expected FIDP -> 3, got %q", result["FIDP"])
	}
}

func TestBuildTagNameToIDMap_Empty(t *testing.T) {
	result := buildTagNameToIDMap(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result))
	}
}

func TestFindMissingTags(t *testing.T) {
	existing := map[string]string{
		"FIAE": "1",
		"FISI": "2",
	}

	tests := []struct {
		name     string
		selected []string
		want     []string
	}{
		{
			name:     "all exist",
			selected: []string{"FIAE", "FISI"},
			want:     nil,
		},
		{
			name:     "some missing",
			selected: []string{"FIAE", "FIDP"},
			want:     []string{"FIDP"},
		},
		{
			name:     "all missing",
			selected: []string{"FIDP", "FIDV"},
			want:     []string{"FIDP", "FIDV"},
		},
		{
			name:     "empty selection",
			selected: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMissingTags(tt.selected, existing)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findMissingTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollectTagIDs(t *testing.T) {
	tagMap := map[string]string{
		"FIAE": "1",
		"FISI": "2",
		"FIDP": "3",
	}

	tests := []struct {
		name     string
		selected []string
		want     []string
	}{
		{
			name:     "all found",
			selected: []string{"FIAE", "FISI"},
			want:     []string{"1", "2"},
		},
		{
			name:     "some not found",
			selected: []string{"FIAE", "UNKNOWN"},
			want:     []string{"1"},
		},
		{
			name:     "empty",
			selected: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectTagIDs(tt.selected, tagMap)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectTagIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}
