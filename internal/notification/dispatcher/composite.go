package dispatcher

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// CompositeDispatcher routes outbox entries to AdminDispatcher or UserDispatcher
// based on the event namespace:
//
//   - admin.* / system.*  →  AdminDispatcher
//   - all other events    →  UserDispatcher
//
// Dual-dispatch (one business action → admin alert + user notification) is
// handled at the publisher level: services call outbox.PoolWriter.WriteBatch to
// insert two correlated entries atomically (one admin.* event, one user event).
// The worker claims and dispatches each independently, giving per-entry
// at-least-once delivery and retry isolation without coupling the two dispatch
// paths.
type CompositeDispatcher struct {
	admin *AdminDispatcher
	user  *UserDispatcher
}

// NewCompositeDispatcher constructs a CompositeDispatcher that wraps the two
// phase-specific dispatchers.
func NewCompositeDispatcher(admin *AdminDispatcher, user *UserDispatcher) *CompositeDispatcher {
	return &CompositeDispatcher{admin: admin, user: user}
}

// Dispatch satisfies outbox.Dispatcher.
func (c *CompositeDispatcher) Dispatch(ctx context.Context, entry *notification.OutboxEntry) error {
	if notification.IsAdminEvent(entry.EventType) {
		return c.admin.Dispatch(ctx, entry)
	}
	return c.user.Dispatch(ctx, entry)
}
