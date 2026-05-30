package messaging

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// InvokeDLQFallback exposes pushDLQFallback for white-box unit tests so they
// can verify the fallback path without relying on timing between goroutines.
func (b *RedisBus) InvokeDLQFallback(ctx context.Context, envelope events.Envelope, envelopeJSON, handlerErr string, attempts int) {
	b.pushDLQFallback(ctx, envelope, envelopeJSON, handlerErr, attempts)
}

// InvokePushDLQ exposes pushDLQ for white-box unit tests that need to cover
// the json.Marshal error path by passing an unmarshalable Envelope payload.
func (b *RedisBus) InvokePushDLQ(ctx context.Context, envelope events.Envelope, handlerErr error) {
	b.pushDLQ(ctx, envelope, handlerErr)
}
