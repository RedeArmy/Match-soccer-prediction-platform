package database

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

// errFS is an fs.FS whose Open always returns an error, causing fs.ReadDir to fail.
type errFS struct{}

func (errFS) Open(_ string) (fs.File, error) { return nil, errors.New("permission denied") }

func TestCollectUpVersions_ReturnsAllVersions(t *testing.T) {
	fs := fstest.MapFS{
		"000001_init.up.sql":    &fstest.MapFile{},
		"000002_users.up.sql":   &fstest.MapFile{},
		"000010_matches.up.sql": &fstest.MapFile{},
	}
	versions, err := collectUpVersions(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d: %v", len(versions), versions)
	}
}

func TestCollectUpVersions_EmptyFS_ReturnsEmpty(t *testing.T) {
	versions, err := collectUpVersions(fstest.MapFS{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

func TestCollectUpVersions_SkipsDownMigrations(t *testing.T) {
	fs := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{},
		"000001_init.down.sql": &fstest.MapFile{},
	}
	versions, err := collectUpVersions(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version (up only), got %d: %v", len(versions), versions)
	}
}

func TestCollectUpVersions_SkipsNonMatchingFiles(t *testing.T) {
	fs := fstest.MapFS{
		"README.md":          &fstest.MapFile{},
		"000001_init.up.sql": &fstest.MapFile{},
		"atlas.hcl":          &fstest.MapFile{},
	}
	versions, err := collectUpVersions(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d: %v", len(versions), versions)
	}
}

func TestCollectUpVersions_VersionValuesCorrect(t *testing.T) {
	fs := fstest.MapFS{
		"000007_foo.up.sql": &fstest.MapFile{},
		"000042_bar.up.sql": &fstest.MapFile{},
	}
	versions, err := collectUpVersions(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := make(map[int]bool, len(versions))
	for _, v := range versions {
		got[v] = true
	}
	if !got[7] || !got[42] {
		t.Errorf("expected versions {7, 42}, got %v", versions)
	}
}

func TestCollectUpVersions_ReadDirError_ReturnsError(t *testing.T) {
	_, err := collectUpVersions(errFS{})
	if err == nil {
		t.Fatal("expected error from ReadDir failure, got nil")
	}
}

// ── MarkMigrationsApplied error paths ────────────────────────────────────────

func TestMarkMigrationsApplied_EmptyFS_ReturnsNil(t *testing.T) {
	// Empty FS produces zero versions; the function returns early without using the pool.
	// Passing a nil pool is safe here because pool is not accessed on the early-return path.
	if err := MarkMigrationsApplied(context.Background(), nil, fstest.MapFS{}); err != nil {
		t.Errorf("expected nil for empty FS, got %v", err)
	}
}

func TestMarkMigrationsApplied_ReadDirError_Propagates(t *testing.T) {
	// errFS makes ReadDir fail; the error must propagate before the pool is used.
	err := MarkMigrationsApplied(context.Background(), nil, errFS{})
	if err == nil {
		t.Fatal("expected error when ReadDir fails, got nil")
	}
}
