package dispatcher_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
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

// coverageMemberLister returns two member IDs so broadcast events have a
// recipient other than the actor (userID=1 in sample payloads).
type coverageMemberLister struct{}

func (coverageMemberLister) ListActiveMemberIDsByGroup(_ context.Context, _ int) ([]int, error) {
	return []int{1, 2}, nil
}

// TestUserEventBuilders_BothLocalesNonEmpty asserts that every user-facing
// event type registered in contentRegistry produces non-empty title and body
// strings for both "en" and "es". A missing or empty string means the
// notification will be delivered with a blank title or body, which breaks
// the push/in-app UX for all users of that locale.
//
// Events whose sample payloads lack a `user_id` field (e.g. match events that
// are fan-out by the worker with per-user outbox entries) are skipped — they
// are covered by the admin-event email builder tests and integration tests.
func TestUserEventBuilders_BothLocalesNonEmpty(t *testing.T) {
	t.Parallel()
	for _, locale := range []domain.Locale{domain.LocaleEN, domain.LocaleES} {
		locale := locale
		for _, et := range notification.AllEventTypes() {
			if notification.IsAdminEvent(et) {
				continue
			}
			et := et
			t.Run(string(et)+"/"+string(locale), func(t *testing.T) {
				t.Parallel()
				assertBuilderStringsNonEmpty(t, et, locale)
			})
		}
	}
}

// assertBuilderStringsNonEmpty dispatches a sample outbox entry for the given
// event type and locale and asserts that the persisted title and body are
// non-empty. Extracted to keep TestUserEventBuilders_BothLocalesNonEmpty below
// the gocognit threshold.
func assertBuilderStringsNonEmpty(t *testing.T, et notification.EventType, locale domain.Locale) {
	t.Helper()

	payload := notification.SamplePayload(et)
	if payload == nil {
		t.Skipf("no sample payload registered for %q", et)
	}

	notifRepo := &stubNotifRepo{inserted: true}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: string(locale)},
	}
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:    notifRepo,
		PrefRepo:     &stubPrefRepo{pref: &domain.NotificationPreference{ChannelInApp: true}},
		DLQRepo:      &recordingDLQRepo{},
		Hub:          hub.New(),
		Params:       params,
		MemberLister: coverageMemberLister{},
		Log:          zap.NewNop(),
	})

	entry := &notification.OutboxEntry{ID: 1, EventType: et, Payload: payload}
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		// Events whose sample payload has no user_id are fan-out by the worker;
		// the dispatcher cannot resolve a recipient and drops them silently.
		t.Skipf("event %q: no recipient in sample payload — covered by worker fan-out path", et)
	}
	if notifRepo.last.Title == "" {
		t.Errorf("locale=%s: Title is empty for event %q", locale, et)
	}
	if notifRepo.last.Body == "" {
		t.Errorf("locale=%s: Body is empty for event %q", locale, et)
	}
}
