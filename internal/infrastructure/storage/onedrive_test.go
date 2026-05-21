package storage_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

func TestNewOneDriveFileStore_MissingTenantID(t *testing.T) {
	_, err := storage.NewOneDriveFileStore(context.Background(), storage.Config{
		Driver:               "onedrive",
		OneDriveClientID:     "client-id",
		OneDriveClientSecret: "secret",
		OneDriveDriveID:      "drive-id",
	})
	if err == nil {
		t.Fatal("expected error for missing OneDriveTenantID, got nil")
	}
}

func TestNewOneDriveFileStore_MissingClientID(t *testing.T) {
	_, err := storage.NewOneDriveFileStore(context.Background(), storage.Config{
		Driver:               "onedrive",
		OneDriveTenantID:     "tenant-id",
		OneDriveClientSecret: "secret",
		OneDriveDriveID:      "drive-id",
	})
	if err == nil {
		t.Fatal("expected error for missing OneDriveClientID, got nil")
	}
}

func TestNewOneDriveFileStore_MissingClientSecret(t *testing.T) {
	_, err := storage.NewOneDriveFileStore(context.Background(), storage.Config{
		Driver:           "onedrive",
		OneDriveTenantID: "tenant-id",
		OneDriveClientID: "client-id",
		OneDriveDriveID:  "drive-id",
	})
	if err == nil {
		t.Fatal("expected error for missing OneDriveClientSecret, got nil")
	}
}

func TestNewOneDriveFileStore_MissingDriveID(t *testing.T) {
	_, err := storage.NewOneDriveFileStore(context.Background(), storage.Config{
		Driver:               "onedrive",
		OneDriveTenantID:     "tenant-id",
		OneDriveClientID:     "client-id",
		OneDriveClientSecret: "secret",
	})
	if err == nil {
		t.Fatal("expected error for missing OneDriveDriveID, got nil")
	}
}

func TestNewOneDriveFileStore_AllFieldsPresent_Constructs(t *testing.T) {
	store, err := storage.NewOneDriveFileStore(context.Background(), storage.Config{
		Driver:               "onedrive",
		OneDriveTenantID:     "tenant-id",
		OneDriveClientID:     "client-id",
		OneDriveClientSecret: "secret",
		OneDriveDriveID:      "drive-id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil OneDriveFileStore, got nil")
	}
}

func TestNew_OneDriveDriverConstructsStore(t *testing.T) {
	store, err := storage.New(context.Background(), storage.Config{
		Driver:               "onedrive",
		OneDriveTenantID:     "tenant-id",
		OneDriveClientID:     "client-id",
		OneDriveClientSecret: "secret",
		OneDriveDriveID:      "drive-id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil FileStore, got nil")
	}
}
