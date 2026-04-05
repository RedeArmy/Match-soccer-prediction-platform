package service

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNotify_LogsAndReturnsNil(t *testing.T) {
	svc := NewNotificationService(zap.NewNop())

	if err := svc.Notify(context.Background(), 1, "match started"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
