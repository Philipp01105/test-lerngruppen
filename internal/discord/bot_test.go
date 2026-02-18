package discord

import "testing"

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
