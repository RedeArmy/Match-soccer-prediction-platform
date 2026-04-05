package service

import (
	"context"

	"go.uber.org/zap"
)

// notificationService is the concrete implementation of NotificationService.
//
// The current implementation is a no-op logger stub. It satisfies the
// interface so that the composition root can wire it without leaving
// the service undefined. Replace this stub with a real push or e-mail
// integration when that requirement is confirmed.
type notificationService struct {
	log *zap.Logger
}

// NewNotificationService constructs a notificationService.
func NewNotificationService(log *zap.Logger) NotificationService {
	return &notificationService{log: log}
}

// Notify logs the notification. No delivery mechanism is wired yet.
func (s *notificationService) Notify(_ context.Context, userID int, message string) error {
	s.log.Info("notification queued",
		zap.Int("user_id", userID),
		zap.String("message", message),
	)
	return nil
}
