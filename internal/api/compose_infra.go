package api

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
	"github.com/rede/world-cup-quiniela/pkg/recurrente"
)

// buildResilientCache wraps s.cache with a circuit breaker when the underlying
// store is a *cache.RedisStore. If Redis is unavailable the breaker opens and
// all cache operations degrade to cache-miss / silent no-op, so the service
// layer continues to work directly against PostgreSQL.
//
// When s.cache is not a RedisStore (e.g. MemoryStore in tests or single-node
// deployments without Redis) the original store is returned unchanged.
func (s *Server) buildResilientCache(ctx context.Context, params service.SystemParamService) cache.Store {
	rs, ok := s.cache.(*cache.RedisStore)
	if !ok {
		return s.cache
	}

	cb := breaker.New(
		"redis-cache",
		params.GetInt(ctx, domain.ParamKeyBreakerCacheMaxFails, domain.DefaultBreakerCacheMaxFails),
		time.Duration(params.GetInt(ctx, domain.ParamKeyBreakerCacheCooldownSec, domain.DefaultBreakerCacheCooldownSec))*time.Second,
	)
	if s.notifier != nil {
		cb.SetOnStateChange(func(name string, from, to breaker.State, openedAt time.Time) {
			if to == breaker.StateOpen {
				s.notifier.NotifyCircuitBreakerOpen(context.Background(), name, to.String(), openedAt)
			}
		})
	}
	if err := breaker.RegisterGauge(otel.GetMeterProvider().Meter("wcq"), cb); err != nil {
		s.log.Warn("breaker.RegisterGauge(redis-cache) failed", zap.Error(err))
	}

	if s.breakerRegistry == nil {
		s.breakerRegistry = breaker.NewRegistry()
	}
	s.breakerRegistry.Register(cb)

	return cache.NewResilientStore(rs, cb, s.log)
}

// registerFileBreakerNotifier wires s.notifier into the file-store circuit
// breaker so that an open-circuit event triggers an n8n alert. Extracted from
// buildHandlers to keep its cognitive complexity within the allowed limit.
func (s *Server) registerFileBreakerNotifier(b *breaker.Breaker) {
	if s.notifier == nil {
		return
	}
	b.SetOnStateChange(func(name string, _ breaker.State, to breaker.State, openedAt time.Time) {
		if to == breaker.StateOpen {
			s.notifier.NotifyCircuitBreakerOpen(context.Background(), name, to.String(), openedAt)
		}
	})
}

// buildPayoutEncrypter constructs the AES-256-GCM encrypter for payout_details.
// When hexKey is empty (local development) a no-op passthrough is returned so
// the application starts without a key. In production validateProductionConfig
// rejects an empty key before this function is reached.
func buildPayoutEncrypter(hexKey string, log *zap.Logger) payoutenc.Encrypter {
	if hexKey == "" {
		log.Warn("payout_details encryption disabled: WCQ_PAYMENT_PAYOUTENCRYPTIONKEY is not set — acceptable only in development")
		return payoutenc.Noop
	}
	enc, err := payoutenc.NewAESGCM(hexKey)
	if err != nil {
		// The key was set but is malformed. Fail loudly at startup.
		log.Fatal("invalid WCQ_PAYMENT_PAYOUTENCRYPTIONKEY: must be a 64-char hex-encoded 32-byte key (openssl rand -hex 32)",
			zap.Error(err),
		)
	}
	return enc
}

// buildPaymentIntentHandler constructs a PaymentIntentHandler and, when an
// API key is provided, wires in the Recurrente checkout creator so that
// POST /api/v1/payment-intents with provider=recurrente creates a hosted
// checkout session and returns a redirect URL.
func buildPaymentIntentHandler(
	svc service.PaymentIntentCreator,
	fileStore storage.FileStore,
	maxUploadBytes int64,
	recurrenteAPIKey, recurrenteBaseURL, appBaseURL string,
	log *zap.Logger,
) *handler.PaymentIntentHandler {
	h := handler.NewPaymentIntentHandler(svc, log)
	h.WithFileStore(fileStore, maxUploadBytes)
	if recurrenteAPIKey != "" {
		if appBaseURL == "" {
			log.Warn("Recurrente is configured but WCQ_SERVER_APPBASEURL is not set — redirect URLs will be invalid; set it to your ngrok or production URL")
		}
		client := recurrente.New(recurrenteAPIKey, recurrenteBaseURL)
		h.WithRecurrente(handler.NewRecurrenteCheckoutAdapter(client), appBaseURL)
	} else {
		log.Warn("Recurrente checkout disabled: WCQ_PAYMENT_RECURRENTEAPIKEY is not set")
	}
	return h
}
