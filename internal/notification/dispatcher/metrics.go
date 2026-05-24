package dispatcher

import (
	"go.opentelemetry.io/otel/metric"
)

// dispatcherInstruments holds the OTel instruments shared by UserDispatcher
// and AdminDispatcher.  All fields are nil when RegisterMetrics has not been
// called, so every call site guards with an existence check.
type dispatcherInstruments struct {
	events   metric.Int64Counter       // dispatcher_events_total
	duration metric.Float64Histogram   // dispatcher_dispatch_duration_seconds
	emails   metric.Int64Counter       // email_deliveries_total
	pushes   metric.Int64Counter       // push_notifications_total (user only)
}

// newDispatcherInstruments allocates all instruments against meter.
func newDispatcherInstruments(meter metric.Meter) (dispatcherInstruments, error) {
	var inst dispatcherInstruments
	var err error

	inst.events, err = meter.Int64Counter(
		"dispatcher.events",
		metric.WithDescription("Total number of outbox entries dispatched, by event_type and status."),
	)
	if err != nil {
		return inst, err
	}

	inst.duration, err = meter.Float64Histogram(
		"dispatcher.dispatch.duration",
		metric.WithDescription("End-to-end dispatch duration per outbox entry."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5,
		),
	)
	if err != nil {
		return inst, err
	}

	inst.emails, err = meter.Int64Counter(
		"email.deliveries",
		metric.WithDescription("Total email delivery attempts, by status (sent/failed)."),
	)
	if err != nil {
		return inst, err
	}

	inst.pushes, err = meter.Int64Counter(
		"push.notifications",
		metric.WithDescription("Total Web Push delivery attempts per subscription, by status."),
	)
	return inst, err
}

// RegisterMetrics wires OTel instruments into the UserDispatcher.
// Call once after construction; safe to skip in tests.
func (d *UserDispatcher) RegisterMetrics(meter metric.Meter) error {
	inst, err := newDispatcherInstruments(meter)
	if err != nil {
		return err
	}
	d.instruments = inst
	return nil
}

// RegisterMetrics wires OTel instruments into the AdminDispatcher.
// Call once after construction; safe to skip in tests.
func (d *AdminDispatcher) RegisterMetrics(meter metric.Meter) error {
	inst, err := newDispatcherInstruments(meter)
	if err != nil {
		return err
	}
	d.instruments = inst
	return nil
}
