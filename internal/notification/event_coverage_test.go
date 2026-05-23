package notification_test

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// TestAllEventTypes_AdminClassificationMatchesPrefixConvention verifies that
// IsAdminEvent() is consistent with the EventType naming convention:
//   - events prefixed "admin." or "system." must be classified as admin
//   - all other events must not be classified as admin
//
// This test catches a new constant that uses the wrong namespace (e.g. naming
// an admin event "notification.admin_foo" instead of "admin.foo"), which would
// cause it to be silently dropped by UserDispatcher.
func TestAllEventTypes_AdminClassificationMatchesPrefixConvention(t *testing.T) {
	t.Parallel()
	for _, et := range notification.AllEventTypes() {
		s := string(et)
		wantAdmin := strings.HasPrefix(s, "admin.") || strings.HasPrefix(s, "system.")
		if got := notification.IsAdminEvent(et); got != wantAdmin {
			t.Errorf(
				"IsAdminEvent(%q) = %v; want %v — event namespace does not match routing expectation",
				et, got, wantAdmin,
			)
		}
	}
}

// TestPriorityTable_CoversAllKnownEvents verifies that every known EventType
// has an explicit entry in priorityTable.
//
// Events absent from the table silently inherit PriorityP2Medium, which is wrong
// for new P0/P1 financial or security events (e.g. a new payment-failure event
// that should trigger immediate email would not do so at the correct urgency).
func TestPriorityTable_CoversAllKnownEvents(t *testing.T) {
	t.Parallel()
	for _, et := range notification.AllEventTypes() {
		if _, ok := notification.PriorityTableForTest[et]; !ok {
			t.Errorf(
				"event type %q has no explicit priority; "+
					"add it to priorityTable in priority.go (choose P0–P3)",
				et,
			)
		}
	}
}
