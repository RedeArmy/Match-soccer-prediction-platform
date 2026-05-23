package dispatcher_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
)

// TestAllAdminEvents_HaveEmailBuilder verifies that every admin/system event
// in the catalog has an explicit entry in the email template builder registry.
//
// A missing entry means the event falls back to the generic layout — which is
// safe (the email is still sent) but produces low-quality copy. Catching this
// at test time forces the developer to make a conscious decision: write a real
// template, or document why the generic layout is acceptable for this event.
func TestAllAdminEvents_HaveEmailBuilder(t *testing.T) {
	t.Parallel()
	for _, et := range notification.AllEventTypes() {
		if !notification.IsAdminEvent(et) {
			continue
		}
		if _, ok := dispatcher.EmailBuildersForTest[et]; !ok {
			t.Errorf(
				"admin/system event %q has no custom email builder; "+
					"add a builder function and register it in emailBuilders in dispatcher/templates.go",
				et,
			)
		}
	}
}

// TestBroadcastEvents_AreAllKnown verifies that every EventType registered in
// broadcastEvents is a real entry in the event catalog. A stale or misspelled
// key would silently make that event's fan-out path unreachable.
func TestBroadcastEvents_AreAllKnown(t *testing.T) {
	t.Parallel()
	for et := range dispatcher.BroadcastEventsForTest {
		if _, ok := notification.KnownEventTypes[et]; !ok {
			t.Errorf(
				"broadcastEvents contains unknown EventType %q; "+
					"remove it or add a matching entry to eventSamples in event_samples.go",
				et,
			)
		}
	}
}
