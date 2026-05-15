package storage_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
)

// serviceAccountJSON builds a syntactically valid Google service-account
// credential JSON using a freshly generated RSA key. The key is never used
// against real Google APIs; it only needs to satisfy the credential parser.
func serviceAccountJSON(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	raw, _ := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "key-id",
		"private_key":                 string(privPEM),
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        fmt.Sprintf("https://www.googleapis.com/robot/v1/metadata/x509/%s", "test%40test-project.iam.gserviceaccount.com"),
	})
	return string(raw)
}

func TestNewGDriveFileStore_MissingFolderID(t *testing.T) {
	_, err := storage.NewGDriveFileStore(storage.Config{
		Driver:                "gdrive",
		GDriveCredentialsJSON: serviceAccountJSON(t),
	})
	if err == nil {
		t.Fatal("expected error for missing GDriveFolderID, got nil")
	}
}

func TestNewGDriveFileStore_WithCredentialsJSON_Constructs(t *testing.T) {
	store, err := storage.NewGDriveFileStore(storage.Config{
		Driver:                "gdrive",
		GDriveCredentialsJSON: serviceAccountJSON(t),
		GDriveFolderID:        "folder-id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil GDriveFileStore, got nil")
	}
}

func TestNew_GDriveDriverConstructsStore(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver:                "gdrive",
		GDriveCredentialsJSON: serviceAccountJSON(t),
		GDriveFolderID:        "folder-id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil FileStore, got nil")
	}
}
