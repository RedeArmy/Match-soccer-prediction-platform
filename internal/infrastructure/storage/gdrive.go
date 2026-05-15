package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"golang.org/x/oauth2/google"
)

// GDriveFileStore implements FileStore against Google Drive using a service
// account (or Application Default Credentials when no inline JSON is provided).
//
// Google Drive does not support path-based file addressing; all items are
// identified by opaque IDs. GDriveFileStore stores each object as a file
// whose name equals the key, inside a single configured parent folder. The
// "/" characters in a key become part of the literal filename (Google Drive
// allows "/" in file names); no sub-folder hierarchy is created.
//
// Uniqueness within the folder is enforced on Put by searching for and
// overwriting any existing file with the same name before uploading.
type GDriveFileStore struct {
	svc      *drive.Service
	folderID string
}

// NewGDriveFileStore constructs a GDriveFileStore from cfg.
//
// When GDriveCredentialsJSON is non-empty it is used as a service-account
// JSON credential. Otherwise Application Default Credentials (ADC) are used,
// which resolves via the GOOGLE_APPLICATION_CREDENTIALS environment variable
// or the GCE metadata server. GDriveFolderID is always required.
func NewGDriveFileStore(cfg Config) (*GDriveFileStore, error) {
	if cfg.GDriveFolderID == "" {
		return nil, fmt.Errorf("storage: GDriveFolderID is required for gdrive driver")
	}

	ctx := context.Background()

	var opts []option.ClientOption
	if cfg.GDriveCredentialsJSON != "" {
		creds, err := google.CredentialsFromJSONWithParams(
			ctx,
			[]byte(cfg.GDriveCredentialsJSON),
			google.CredentialsParams{Scopes: []string{drive.DriveScope}},
		)
		if err != nil {
			return nil, fmt.Errorf("storage: gdrive credentials: %w", err)
		}
		opts = append(opts, option.WithCredentials(creds))
	} else {
		// Fall back to ADC (GOOGLE_APPLICATION_CREDENTIALS or GCE metadata).
		opts = append(opts, option.WithScopes(drive.DriveScope))
	}

	svc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: gdrive service: %w", err)
	}

	return &GDriveFileStore{svc: svc, folderID: cfg.GDriveFolderID}, nil
}

// Put uploads r to the configured folder with name equal to key, overwriting
// any existing file with the same name.
func (s *GDriveFileStore) Put(ctx context.Context, key, contentType string, r io.Reader, _ int64) error {
	existingID, err := s.findByName(ctx, key)
	if err != nil {
		return err
	}

	if existingID != "" {
		// Update content in place — avoids a delete+create race and preserves
		// the file ID (which may be referenced externally).
		_, err = s.svc.Files.Update(existingID, &drive.File{}).
			Media(r, googleapi.ContentType(contentType)).
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("storage: gdrive put %q (update): %w", key, err)
		}
		return nil
	}

	_, err = s.svc.Files.Create(&drive.File{
		Name:    key,
		Parents: []string{s.folderID},
	}).
		Media(r, googleapi.ContentType(contentType)).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("storage: gdrive put %q (create): %w", key, err)
	}
	return nil
}

// Get downloads the file named key from the configured folder. The caller must
// close the returned ReadCloser. Returns ErrNotFound when the key does not
// exist.
func (s *GDriveFileStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	fileID, err := s.findByName(ctx, key)
	if err != nil {
		return nil, "", err
	}
	if fileID == "" {
		return nil, "", ErrNotFound
	}

	meta, err := s.svc.Files.Get(fileID).
		Fields("mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return nil, "", fmt.Errorf("storage: gdrive get %q (meta): %w", key, err)
	}

	resp, err := s.svc.Files.Get(fileID).
		Context(ctx).
		Download()
	if err != nil {
		return nil, "", fmt.Errorf("storage: gdrive get %q (download): %w", key, err)
	}

	ct := meta.MimeType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return resp.Body, ct, nil
}

// Delete removes the file named key from the configured folder. Succeeds
// silently when the key does not exist.
func (s *GDriveFileStore) Delete(ctx context.Context, key string) error {
	fileID, err := s.findByName(ctx, key)
	if err != nil {
		return err
	}
	if fileID == "" {
		return nil
	}

	if err := s.svc.Files.Delete(fileID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("storage: gdrive delete %q: %w", key, err)
	}
	return nil
}

// findByName searches the configured folder for a file whose name matches key
// exactly. Returns the file ID when found, or ("", nil) when not found.
func (s *GDriveFileStore) findByName(ctx context.Context, key string) (string, error) {
	q := fmt.Sprintf(
		"name = '%s' and '%s' in parents and trashed = false",
		escapeDriveQuery(key), s.folderID,
	)

	list, err := s.svc.Files.List().
		Q(q).
		Fields("files(id)").
		PageSize(1).
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("storage: gdrive search %q: %w", key, err)
	}

	if len(list.Files) == 0 {
		return "", nil
	}
	return list.Files[0].Id, nil
}

// escapeDriveQuery escapes a string for safe inclusion inside a Google Drive
// Files.list query expression. Backslashes must be escaped before single
// quotes to avoid double-escaping.
func escapeDriveQuery(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}
