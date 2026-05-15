package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2/clientcredentials"
)

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// OneDriveFileStore implements FileStore against a OneDrive / SharePoint drive
// via the Microsoft Graph API. Authentication uses the OAuth2 client-credentials
// flow (application identity, no user interaction required).
//
// Keys are mapped to file paths inside the configured drive. The "/" separator
// in a key is treated as a directory separator, so the key
// "bank-transfers/uuid.jpg" resolves to a file named "uuid.jpg" inside the
// "bank-transfers" folder at the root of the drive.
type OneDriveFileStore struct {
	client  *http.Client
	driveID string
}

// NewOneDriveFileStore constructs an OneDriveFileStore from cfg.
//
// Required fields: OneDriveTenantID, OneDriveClientID, OneDriveClientSecret,
// OneDriveDriveID. The OAuth2 token is fetched lazily on the first API call.
func NewOneDriveFileStore(cfg Config) (*OneDriveFileStore, error) {
	if cfg.OneDriveTenantID == "" {
		return nil, fmt.Errorf("storage: OneDriveTenantID is required for onedrive driver")
	}
	if cfg.OneDriveClientID == "" {
		return nil, fmt.Errorf("storage: OneDriveClientID is required for onedrive driver")
	}
	if cfg.OneDriveClientSecret == "" {
		return nil, fmt.Errorf("storage: OneDriveClientSecret is required for onedrive driver")
	}
	if cfg.OneDriveDriveID == "" {
		return nil, fmt.Errorf("storage: OneDriveDriveID is required for onedrive driver")
	}

	cc := &clientcredentials.Config{
		ClientID:     cfg.OneDriveClientID,
		ClientSecret: cfg.OneDriveClientSecret,
		TokenURL: fmt.Sprintf(
			"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
			url.PathEscape(cfg.OneDriveTenantID),
		),
		Scopes: []string{"https://graph.microsoft.com/.default"},
	}

	return &OneDriveFileStore{
		client:  cc.Client(context.Background()),
		driveID: cfg.OneDriveDriveID,
	}, nil
}

// Put uploads r to the drive path indicated by key, overwriting any existing
// object. Files up to 4 MB are uploaded in a single request; larger files
// require upload sessions (not yet supported — bank-transfer proofs are well
// under this limit).
func (s *OneDriveFileStore) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	u := fmt.Sprintf("%s/drives/%s/root:/%s:/content", graphBaseURL, s.driveID, oneDrivePath(key))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, r)
	if err != nil {
		return fmt.Errorf("storage: onedrive put %q: %w", key, err)
	}
	req.Header.Set("Content-Type", contentType)
	if size > 0 {
		req.ContentLength = size
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: onedrive put %q: %w", key, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("storage: onedrive put %q: unexpected status %d", key, resp.StatusCode)
	}
	return nil
}

// Get downloads the object at key. The caller must close the returned
// ReadCloser. Returns ErrNotFound when the key does not exist.
//
// The Graph API may return a 302 redirect to a preauthenticated CDN URL;
// the underlying http.Client follows it transparently.
func (s *OneDriveFileStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	u := fmt.Sprintf("%s/drives/%s/root:/%s:/content", graphBaseURL, s.driveID, oneDrivePath(key))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("storage: onedrive get %q: %w", key, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("storage: onedrive get %q: %w", key, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("storage: onedrive get %q: unexpected status %d", key, resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return resp.Body, ct, nil
}

// Delete removes the object at key. Succeeds silently when the key does not
// exist (Graph API DELETE is treated as idempotent).
func (s *OneDriveFileStore) Delete(ctx context.Context, key string) error {
	itemID, err := s.resolveItemID(ctx, key)
	if err != nil {
		return err
	}
	if itemID == "" {
		return nil // item not found — match idempotent semantics of other drivers
	}

	u := fmt.Sprintf("%s/drives/%s/items/%s", graphBaseURL, s.driveID, url.PathEscape(itemID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("storage: onedrive delete %q: %w", key, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: onedrive delete %q: %w", key, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("storage: onedrive delete %q: unexpected status %d", key, resp.StatusCode)
	}
	return nil
}

// resolveItemID fetches the Graph item ID for a key. Returns ("", nil) when
// the item does not exist.
func (s *OneDriveFileStore) resolveItemID(ctx context.Context, key string) (string, error) {
	u := fmt.Sprintf("%s/drives/%s/root:/%s", graphBaseURL, s.driveID, oneDrivePath(key))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("storage: onedrive resolve %q: %w", key, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage: onedrive resolve %q: %w", key, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("storage: onedrive resolve %q: unexpected status %d", key, resp.StatusCode)
	}

	var item struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return "", fmt.Errorf("storage: onedrive resolve %q: decode response: %w", key, err)
	}
	return item.ID, nil
}

// oneDrivePath URL-encodes each path segment of key individually, preserving
// the "/" directory separators so that "bank-transfers/uuid.jpg" maps to two
// levels in the OneDrive folder hierarchy.
func oneDrivePath(key string) string {
	segments := strings.Split(key, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}
