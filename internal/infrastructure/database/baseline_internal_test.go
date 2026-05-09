package database

import (
	"testing"
	"testing/fstest"
)

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
