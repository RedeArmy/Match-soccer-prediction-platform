// Package email — ResilientSender wraps any Sender with three reliability layers:
//
//  1. OpenTelemetry tracing — one span per Send call recording recipient, subject,
//     message ID on success, and the error on failure.
//
//  2. Circuit breaker — opens after maxFails consecutive 5xx/network failures;
//     short-circuits immediately while open so the outbox worker is not blocked
//     waiting for a hung Resend API.  Rate-limit (429) responses are NOT counted
//     as circuit-breaker failures.
//
//  3. Exponential back-off on 429 — respects the Retry-After header when present;
//     falls back to doubling back-off (1 s → 2 s → 4 s, capped at 30 s) otherwise.
//     Up to maxRetry retry attempts are made before giving up.
package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// ResilientSender wraps a Sender with OTel tracing, a circuit breaker, and
// exponential back-off on HTTP 429.  Construct with NewResilientSender.
type ResilientSender struct {
	inner    Sender
	breaker  *breaker.Breaker
	tracer   trace.Tracer
	maxRetry int // maximum 429 retry attempts (not counting the initial attempt)
	log      *zap.Logger
}

// NewResilientSender wraps inner.  b must not be nil.  maxRetry ≥ 1; values
// below 1 are clamped to 1 so the first attempt is always made.
func NewResilientSender(inner Sender, b *breaker.Breaker, tracer trace.Tracer, maxRetry int, log *zap.Logger) *ResilientSender {
	if maxRetry < 1 {
		maxRetry = 1
	}
	return &ResilientSender{
		inner:    inner,
		breaker:  b,
		tracer:   tracer,
		maxRetry: maxRetry,
		log:      log,
	}
}

// Send delivers msg, respecting the circuit breaker and retrying on 429.
//
// A non-nil error is returned when:
//   - the circuit breaker is open (breaker.ErrOpen)
//   - the inner sender returns a non-429 error (4xx/5xx, network failure)
//   - 429 retries are exhausted
//   - ctx is cancelled during a retry sleep
func (s *ResilientSender) Send(ctx context.Context, msg Message) (string, error) {
	ctx, span := s.tracer.Start(ctx, "email.Send",
		trace.WithAttributes(
			attribute.String("email.to", strings.Join(msg.To, ",")),
			attribute.String("email.subject", msg.Subject),
		),
	)
	defer span.End()

	var last429 *RetryAfterError

	for attempt := 0; attempt <= s.maxRetry; attempt++ {
		last429 = nil

		var msgID string
		breakerErr := s.breaker.Call(func() error {
			id, err := s.inner.Send(ctx, msg)
			msgID = id

			// 429 is a rate-limit, not a service failure — do not penalise the
			// circuit breaker for it.  Store it so the retry loop can handle it.
			var rl *RetryAfterError
			if errors.As(err, &rl) {
				last429 = rl
				return nil
			}
			return err
		})

		switch {
		case errors.Is(breakerErr, breaker.ErrOpen):
			span.RecordError(breakerErr)
			span.SetStatus(codes.Error, "circuit open")
			return "", breakerErr

		case breakerErr != nil:
			// Genuine send failure (5xx, network, 4xx other than 429).
			span.RecordError(breakerErr)
			span.SetStatus(codes.Error, breakerErr.Error())
			return "", breakerErr

		case last429 == nil:
			// Success.
			span.SetAttributes(attribute.String("email.message_id", msgID))
			span.SetStatus(codes.Ok, "")
			return msgID, nil
		}

		// 429 — compute delay and sleep before retrying.
		if attempt == s.maxRetry {
			break
		}
		delay := last429.RetryAfter
		if delay <= 0 {
			delay = rateLimitBackoff(attempt)
		}
		s.log.Warn("email: rate limited; backing off before retry",
			zap.Duration("retry_after", delay),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retry", s.maxRetry),
		)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context cancelled during rate-limit back-off")
			return "", ctx.Err()
		}
	}

	// All retry attempts exhausted on 429.
	finalErr := fmt.Errorf("email: rate limited after %d attempt(s): %w", s.maxRetry+1, last429)
	span.RecordError(finalErr)
	span.SetStatus(codes.Error, "rate limited: retries exhausted")
	return "", finalErr
}

// rateLimitBackoff returns the back-off duration for the given zero-indexed
// attempt number using exponential doubling capped at 30 seconds.
// attempt=0 → 1s, attempt=1 → 2s, attempt=2 → 4s, attempt=3 → 8s, …
func rateLimitBackoff(attempt int) time.Duration {
	d := time.Second
	for i := 0; i < attempt; i++ {
		d *= 2
		if d > 30*time.Second {
			d = 30 * time.Second
			break
		}
	}
	return d
}
