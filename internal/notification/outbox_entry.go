package notification

import (
	"encoding/json"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// OutboxStatus is the lifecycle state of a domain_outbox row.
type OutboxStatus string

// Lifecycle states for a domain_outbox row.
const (
	OutboxStatusPending    OutboxStatus = "pending"
	OutboxStatusProcessing OutboxStatus = "processing"
	OutboxStatusDone       OutboxStatus = "done"
	OutboxStatusFailed     OutboxStatus = "failed"
)

// OutboxEntry is the in-memory representation of a domain_outbox row.
// It is returned by the outbox repository when the worker claims a batch.
type OutboxEntry struct {
	ID            int64
	EventType     EventType
	AggregateID   string
	AggregateType string
	Payload       json.RawMessage
	Status        OutboxStatus
	Attempts      int
	MaxAttempts   int
	ScheduledAt   time.Time
	LockedUntil   *time.Time
	ProcessedAt   *time.Time
	ErrorDetail   *string
	CreatedAt     time.Time
}

// NewOutboxEntry constructs an OutboxEntry by marshalling payload to JSON.
// Returns a validation error when payload cannot be marshalled.
func NewOutboxEntry(eventType EventType, aggregateType, aggregateID string, payload any) (*OutboxEntry, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Validation("outbox: cannot marshal payload: " + err.Error())
	}
	return &OutboxEntry{
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       raw,
		Status:        OutboxStatusPending,
		MaxAttempts:   5,
		ScheduledAt:   time.Now(),
	}, nil
}

// DecodePayload unmarshals the raw JSON payload into dst.
func (e *OutboxEntry) DecodePayload(dst any) error {
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}
