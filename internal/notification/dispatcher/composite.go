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
// Both dispatchers may be called for the same entry when the event has both an
// admin leg and a user-facing leg (currently none — the separation is clean in
// the event type taxonomy).
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
