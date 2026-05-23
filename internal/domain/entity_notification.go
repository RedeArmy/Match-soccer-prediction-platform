package domain

import "time"

// ── Admin notification log ────────────────────────────────────────────────────

// AdminNotifStatus is the delivery outcome of an admin email dispatch.
type AdminNotifStatus string

// Delivery outcomes recorded in admin_notification_log.
const (
	AdminNotifStatusSent   AdminNotifStatus = "sent"
	AdminNotifStatusFailed AdminNotifStatus = "failed"
)

// AdminNotificationLog is an immutable record of a single admin email dispatch.
// The table is append-only; rows are never updated or deleted.
type AdminNotificationLog struct {
	ID          int64
	EventType   string
	Recipients  []string
	Subject     string
	Status      AdminNotifStatus
	ResendMsgID string    // populated on successful delivery
	ErrorDetail string    // populated on failure
	CreatedAt   time.Time // set by the database on INSERT
}

// ── Notification DLQ ─────────────────────────────────────────────────────────

// NotificationDLQEntry records a delivery failure for a specific channel
// after all retry attempts have been exhausted. Used for manual replay and
// ops alerting.
type NotificationDLQEntry struct {
	ID          int64
	OutboxID    *int64 // references domain_outbox.id; nil when the outbox row was purged
	Channel     string // 'email' | 'push' | 'sse'
	UserID      *int   // nil for admin/system events that have no target user
	EventType   string
	Payload     []byte // raw JSON payload from the outbox entry
	ErrorDetail string
	Attempts    int
	CreatedAt   time.Time
	LastRetryAt *time.Time
	ResolvedAt  *time.Time
}

// ── User notifications ────────────────────────────────────────────────────────

// UserNotification is a persisted inbox entry for a single user.
// The idempotency_key prevents duplicate rows when the outbox worker retries.
type UserNotification struct {
	ID             int64
	UserID         int
	EventType      string
	Title          string
	Body           string
	ActionURL      string
	Metadata       map[string]any
	IdempotencyKey string
	ReadAt         *time.Time
	CreatedAt      time.Time
}

// IsRead reports whether the notification has been acknowledged by the user.
func (n *UserNotification) IsRead() bool { return n.ReadAt != nil }

// ── Notification preferences ──────────────────────────────────────────────────

// NotificationPreference controls per-user, per-event-type channel opt-in.
// Missing rows default to all channels enabled (opt-out model).
type NotificationPreference struct {
	UserID       int
	EventType    string
	ChannelEmail bool
	ChannelPush  bool
	ChannelInApp bool
	UpdatedAt    time.Time
}

// ── Push subscriptions ────────────────────────────────────────────────────────

// PushSubscription is a Web Push (VAPID) subscription registered by a browser
// or device. A user may have multiple active subscriptions.
type PushSubscription struct {
	ID         int64
	UserID     int
	Endpoint   string
	P256dhKey  string
	AuthKey    string
	UserAgent  string
	Active     bool
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ── Notification templates ────────────────────────────────────────────────────

// NotificationTemplate stores operator-editable Go text/template content for a
// user-facing notification event. Each row overrides the compiled Go default
// for the (EventType, Locale) pair. Deleting a row reverts that combination
// to the compiled fallback without requiring a redeployment.
//
// TitleTmpl, BodyTmpl, and ActionURLTmpl are Go text/template strings.
// Template data = outbox payload decoded as map[string]any (JSON snake_case
// keys). Available functions: formatCents, int.
type NotificationTemplate struct {
	EventType        string
	Locale           string
	TitleTmpl        string
	BodyTmpl         string
	ActionURLTmpl    string
	EmailSubjectTmpl string // overrides title as email subject when non-empty
	EmailHTMLTmpl    string // full html/template replacing userBaseTemplate when non-empty
	UpdatedBy        *int
	UpdatedAt        time.Time
}

// NotificationTemplateHistory is an immutable archive of a previous
// NotificationTemplate state. Rows are written automatically by the
// notification_templates_archive database trigger before each UPDATE on
// notification_templates. The row with the highest ID for a given
// (EventType, Locale) pair is the most recent prior version.
//
// Use the admin rollback API to restore a historical version; the trigger
// will archive the current live row again, preserving the full audit trail.
type NotificationTemplateHistory struct {
	ID               int64
	EventType        string
	Locale           string
	TitleTmpl        string
	BodyTmpl         string
	ActionURLTmpl    string
	EmailSubjectTmpl string
	EmailHTMLTmpl    string
	ChangedBy        *int // user ID of the operator who made the superseded change
	ChangedAt        time.Time
}
