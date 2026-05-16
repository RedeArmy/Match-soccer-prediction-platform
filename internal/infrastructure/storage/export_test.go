package storage

import (
	"net/http"

	"google.golang.org/api/drive/v3"
)

// NewOneDriveFileStoreForTest constructs an OneDriveFileStore with a
// pre-configured HTTP client and a custom base URL, bypassing the OAuth2
// credential flow. For unit tests only.
func NewOneDriveFileStoreForTest(client *http.Client, driveID, baseURL string) *OneDriveFileStore {
	return &OneDriveFileStore{client: client, driveID: driveID, baseURL: baseURL}
}

// NewGDriveFileStoreForTest constructs a GDriveFileStore from a pre-built
// Drive service, bypassing the credential resolution logic. For unit tests only.
func NewGDriveFileStoreForTest(svc *drive.Service, folderID string) *GDriveFileStore {
	return &GDriveFileStore{svc: svc, folderID: folderID}
}

// EscapeDriveQuery exposes the package-private escapeDriveQuery for unit testing.
var EscapeDriveQuery = escapeDriveQuery
