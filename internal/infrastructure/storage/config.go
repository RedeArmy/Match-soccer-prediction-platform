package storage

// Config holds the configuration required to construct a FileStore.
// Only the fields relevant to the selected Driver are used; the rest are
// silently ignored.
type Config struct {
	// Driver selects the backing store implementation.
	// Accepted values: "local", "s3", "onedrive", "gdrive".
	Driver string

	// ── local ────────────────────────────────────────────────────────────────

	// LocalDir is the filesystem root used by the "local" driver.
	// Defaults to "uploads" relative to the working directory.
	LocalDir string

	// ── s3 ───────────────────────────────────────────────────────────────────

	// S3Bucket is the bucket name. Required for the "s3" driver.
	S3Bucket string
	// S3Endpoint overrides the default AWS endpoint, enabling compatibility
	// with Cloudflare R2 and MinIO. Path-style addressing is enabled
	// automatically when this is set.
	S3Endpoint string
	// S3Region is the AWS region or equivalent (e.g. "auto" for Cloudflare R2).
	// Required for the "s3" driver.
	S3Region string
	// S3AccessKeyID and S3SecretKey provide static credentials. When both are
	// empty the SDK falls back to the standard credential chain (env vars,
	// shared credentials file, IAM instance profile).
	S3AccessKeyID string
	S3SecretKey   string

	// ── onedrive ─────────────────────────────────────────────────────────────

	// OneDriveTenantID is the Azure Active Directory tenant ID (GUID or domain).
	// Required for the "onedrive" driver.
	OneDriveTenantID string
	// OneDriveClientID is the Azure application (client) ID.
	// Required for the "onedrive" driver.
	OneDriveClientID string
	// OneDriveClientSecret is the client secret for the Azure application.
	// Required for the "onedrive" driver.
	OneDriveClientSecret string
	// OneDriveDriveID is the Microsoft Graph drive identifier
	// (e.g. "b!abc123" for a SharePoint document library, or the drive ID from
	// /me/drive). Required for the "onedrive" driver.
	OneDriveDriveID string

	// ── gdrive ───────────────────────────────────────────────────────────────

	// GDriveCredentialsJSON is the raw JSON content of a Google service-account
	// key file. When empty, Application Default Credentials (ADC) are used
	// instead (GOOGLE_APPLICATION_CREDENTIALS env var or GCE metadata server).
	GDriveCredentialsJSON string
	// GDriveFolderID is the Google Drive folder ID that acts as the root for
	// all stored objects. Required for the "gdrive" driver.
	GDriveFolderID string
}
