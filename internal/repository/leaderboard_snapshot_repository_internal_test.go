package repository

// Tests for unexported snapshot versioning helpers.

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

func TestMarshalSnapshotEntries_UnsupportedVersion_ReturnsError(t *testing.T) {
	_, err := marshalSnapshotEntries(999, []domain.LeaderboardSnapshotEntry{})
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestUnmarshalSnapshotEntries_UnsupportedVersion_ReturnsError(t *testing.T) {
	_, err := unmarshalSnapshotEntries(999, []byte(`[]`))
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestUnmarshalSnapshotEntries_V1_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := unmarshalSnapshotEntries(domain.SnapshotSchemaV1, []byte(`not-json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestMarshalSnapshotEntries_V1_RoundTrips(t *testing.T) {
	entries := []domain.LeaderboardSnapshotEntry{
		{UserID: 1, Rank: 1, TotalPoints: 15, PrizeWinner: true},
	}
	b, err := marshalSnapshotEntries(domain.SnapshotSchemaV1, entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := unmarshalSnapshotEntries(domain.SnapshotSchemaV1, b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].UserID != 1 {
		t.Errorf("round-trip: got %+v", got)
	}
}
