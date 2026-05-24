package dispatcher

import "github.com/rede/world-cup-quiniela/internal/notification"

// EmailBuildersForTest exposes the emailBuilders dispatch table so that
// exhaustiveness tests can assert every admin/system EventType is registered.
var EmailBuildersForTest = emailBuilders

// BroadcastEventsForTest exposes the broadcastEvents set so that tests can
// assert it contains only known EventTypes from the catalog.
var BroadcastEventsForTest = broadcastEvents

// SetRenderEmailFn replaces the renderFn on d for the duration of a test.
// Call the returned restore function in a defer to reset it.
// Per-instance injection eliminates the shared-global race between parallel tests.
func SetRenderEmailFn(d *AdminDispatcher, fn func(*notification.OutboxEntry) (string, string, error)) (restore func()) {
	prev := d.renderFn
	d.renderFn = fn
	return func() { d.renderFn = prev }
}
