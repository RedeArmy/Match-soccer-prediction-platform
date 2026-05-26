package service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// KYCMetrics holds the OTel instruments used to observe KYC operations.
// Construct once at startup via RegisterKYCMetrics and pass to KYC-aware
// service wrappers or wire directly into handler observability hooks.
type KYCMetrics struct {
	// SubmissionsTotal counts KYC profile submission attempts, labelled by outcome.
	// Attributes: outcome=success|conflict|validation_error
	SubmissionsTotal metric.Int64Counter
	// ReviewDurationSeconds is a histogram of the time between submission and
	// the first admin decision (approve/reject/escalate), in seconds.
	ReviewDurationSeconds metric.Float64Histogram
	// QueueDepth is an observable gauge reporting the current number of profiles
	// in pending, under_review, or escalated state. Sampled on each scrape.
	QueueDepth metric.Int64ObservableGauge
	// FrozenBalancesTotalGTQ is an observable gauge of the total frozen balance
	// across all frozen accounts, expressed in GTQ (i.e. frozen_amount_cents / 100).
	FrozenBalancesTotalGTQ metric.Float64ObservableGauge
	// FraudFlagsTotal counts the number of times a PEP or sanctions flag was set
	// by an admin, labelled by flag_type=pep|sanctions.
	FraudFlagsTotal metric.Int64Counter
	// GateBlocksTotal counts money-movement operations blocked by KYCGate,
	// labelled by operation=deposit|withdrawal and reason=tier_too_low|amount_exceeds_cap.
	GateBlocksTotal metric.Int64Counter
}

// RegisterKYCMetrics creates and registers all KYC OTel instruments on meter.
// profileQueueReader and frozenReader are called on each Prometheus scrape to
// populate the observable gauges; pass nil to skip the corresponding gauge.
// Returns a ready-to-use KYCMetrics and any registration error.
func RegisterKYCMetrics(
	meter metric.Meter,
	profileQueueReader KYCQueueDepthReader,
	frozenReader KYCFrozenReader,
) (*KYCMetrics, error) {
	m := &KYCMetrics{}
	var err error

	m.SubmissionsTotal, err = meter.Int64Counter(
		"kyc.submissions_total",
		metric.WithDescription("Total KYC profile submission attempts, labelled by outcome."),
	)
	if err != nil {
		return nil, fmt.Errorf("kyc metrics: submissions_total: %w", err)
	}

	m.ReviewDurationSeconds, err = meter.Float64Histogram(
		"kyc.review_duration_seconds",
		metric.WithDescription("Time in seconds from KYC submission to first admin decision (approve/reject/escalate)."),
		metric.WithExplicitBucketBoundaries(60, 300, 900, 3600, 14400, 86400),
	)
	if err != nil {
		return nil, fmt.Errorf("kyc metrics: review_duration_seconds: %w", err)
	}

	m.FraudFlagsTotal, err = meter.Int64Counter(
		"kyc.fraud_flags_total",
		metric.WithDescription("Number of PEP or sanctions flags set by an admin, labelled by flag_type."),
	)
	if err != nil {
		return nil, fmt.Errorf("kyc metrics: fraud_flags_total: %w", err)
	}

	m.GateBlocksTotal, err = meter.Int64Counter(
		"kyc.gate_blocks_total",
		metric.WithDescription("Money-movement operations blocked by KYCGate, labelled by operation and reason."),
	)
	if err != nil {
		return nil, fmt.Errorf("kyc metrics: gate_blocks_total: %w", err)
	}

	if err := registerQueueDepthGauge(m, meter, profileQueueReader); err != nil {
		return nil, err
	}
	if err := registerFrozenBalanceGauge(m, meter, frozenReader); err != nil {
		return nil, err
	}
	return m, nil
}

func registerQueueDepthGauge(m *KYCMetrics, meter metric.Meter, reader KYCQueueDepthReader) error {
	if reader == nil {
		return nil
	}
	var err error
	m.QueueDepth, err = meter.Int64ObservableGauge(
		"kyc.queue_depth",
		metric.WithDescription("Current number of KYC profiles awaiting admin review (pending + under_review + escalated)."),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			n, err := reader.CountReviewQueue(ctx)
			if err != nil {
				return nil
			}
			obs.Observe(n)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("kyc metrics: queue_depth: %w", err)
	}
	return nil
}

func registerFrozenBalanceGauge(m *KYCMetrics, meter metric.Meter, reader KYCFrozenReader) error {
	if reader == nil {
		return nil
	}
	var err error
	m.FrozenBalancesTotalGTQ, err = meter.Float64ObservableGauge(
		"kyc.frozen_balances_total_gtq",
		metric.WithDescription("Total value of frozen KYC balances across all accounts, expressed in GTQ."),
		metric.WithFloat64Callback(func(ctx context.Context, obs metric.Float64Observer) error {
			cents, err := reader.SumFrozenAmountCents(ctx)
			if err != nil {
				return nil
			}
			obs.Observe(float64(cents) / 100)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("kyc metrics: frozen_balances_total_gtq: %w", err)
	}
	return nil
}

// RecordSubmission increments the submissions counter with the given outcome label.
// Outcome values: "success", "conflict", "validation_error".
func (m *KYCMetrics) RecordSubmission(ctx context.Context, outcome string) {
	if m == nil || m.SubmissionsTotal == nil {
		return
	}
	m.SubmissionsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

// RecordReviewDuration records how long (seconds) a profile spent in queue
// before the first admin decision, labelled by the decision type.
func (m *KYCMetrics) RecordReviewDuration(ctx context.Context, seconds float64, decision string) {
	if m == nil || m.ReviewDurationSeconds == nil {
		return
	}
	m.ReviewDurationSeconds.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("decision", decision),
	))
}

// RecordFraudFlag increments the fraud flags counter for the given flag type.
// flagType should be "pep" or "sanctions".
func (m *KYCMetrics) RecordFraudFlag(ctx context.Context, flagType string) {
	if m == nil || m.FraudFlagsTotal == nil {
		return
	}
	m.FraudFlagsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("flag_type", flagType),
	))
}

// RecordGateBlock increments the gate blocks counter.
// operation: "deposit" or "withdrawal". reason: "tier_too_low" or "amount_exceeds_cap".
func (m *KYCMetrics) RecordGateBlock(ctx context.Context, operation, reason string) {
	if m == nil || m.GateBlocksTotal == nil {
		return
	}
	m.GateBlocksTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("reason", reason),
	))
}

// ── Observer interfaces ───────────────────────────────────────────────────────

// KYCQueueDepthReader is satisfied by any type that can count profiles in the
// review queue. It is a narrow interface to decouple the metrics package from
// the full KYCProfileRepository.
type KYCQueueDepthReader interface {
	CountReviewQueue(ctx context.Context) (int64, error)
}

// KYCFrozenReader is satisfied by any type that can sum frozen balance cents.
type KYCFrozenReader interface {
	SumFrozenAmountCents(ctx context.Context) (int64, error)
}
