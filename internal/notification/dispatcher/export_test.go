package dispatcher

import "github.com/rede/world-cup-quiniela/internal/notification"

// EmailBuildersForTest exposes the emailBuilders dispatch table so that
// exhaustiveness tests can assert every admin/system EventType is registered.
var EmailBuildersForTest = emailBuilders

// BroadcastEventsForTest exposes the broadcastEvents set so that tests can
// assert it contains only known EventTypes from the catalog.
var BroadcastEventsForTest = broadcastEvents

// SetRenderEmailFn replaces the package-level renderEmailFn for the duration
// of a test. Call the returned restore function in a defer to reset it.
func SetRenderEmailFn(fn func(*notification.OutboxEntry) (string, string, error)) (restore func()) {
	prev := renderEmailFn
	renderEmailFn = fn
	return func() { renderEmailFn = prev }
}
