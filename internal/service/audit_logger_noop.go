package service

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// NoopAuditLogger is an AuditLogger that discards every entry.
// Use it in the worker process (where audit entries are not required) and in
// tests that do not need to assert on audit trail output.
type NoopAuditLogger struct{}

func (NoopAuditLogger) Log(
	_ context.Context,
	_ *int,
	_ *domain.UserRole,
	_ string,
	_ *string,
	_ *int,
	_ map[string]any,
) {
}

var _ AuditLogger = NoopAuditLogger{}
