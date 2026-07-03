// Package api provides internal white-box tests for unexported helpers in the
// composition root.  These tests must live in package api (not api_test) so
// they can reach private functions that are intentionally not part of the
// public API surface.
package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// errFailMeter is a noop Meter that forces Int64Counter to return an error.
// Used to exercise the RegisterMetrics error path in buildIPRateStore.
type errFailMeter struct{ metricnoop.Meter }

var errForcedCounter = errors.New("injected counter failure")

func (errFailMeter) Int64Counter(_ string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errForcedCounter
}

// ── buildIPRateStore ──────────────────────────────────────────────────────────

// TestBuildIPRateStore_NoRedis_ReturnsLimiterStore verifies that when no Redis
// client is configured, buildIPRateStore returns an in-process LimiterStore so
// the server starts without requiring a Redis connection.
func TestBuildIPRateStore_NoRedis_ReturnsLimiterStore(t *testing.T) {
	s := &Server{log: zaptest.NewLogger(t)} // redisClient == nil
	meter := metricnoop.NewMeterProvider().Meter("test")
	store := s.buildIPRateStore(meter, 10.0, 20)
	if _, ok := store.(*middleware.LimiterStore); !ok {
		t.Errorf("expected *middleware.LimiterStore when Redis is not configured, got %T", store)
	}
}

// TestBuildIPRateStore_WithRedis_ReturnsRedisRateStore verifies that when a
// Redis client is available, buildIPRateStore returns a RedisRateStore so
// IP rate limits are enforced cluster-wide across replicas.
func TestBuildIPRateStore_WithRedis_ReturnsRedisRateStore(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
	meter := metricnoop.NewMeterProvider().Meter("test")
	store := s.buildIPRateStore(meter, 10.0, 20)
	if _, ok := store.(*middleware.RedisRateStore); !ok {
		t.Errorf("expected *middleware.RedisRateStore when Redis is configured, got %T", store)
	}
}

// TestBuildIPRateStore_WithRedis_RegisterMetricsError_StillReturnsStore verifies
// the fail-soft behaviour: when OTel counter registration fails, buildIPRateStore
// logs a warning but still returns a valid RedisRateStore so the server can
// continue to serve traffic without metrics.
func TestBuildIPRateStore_WithRedis_RegisterMetricsError_StillReturnsStore(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
	// errFailMeter forces Int64Counter to return an error, exercising the
	// s.log.Warn("RedisRateStore.RegisterMetrics failed...") branch.
	store := s.buildIPRateStore(errFailMeter{}, 5.0, 10)
	if store == nil {
		t.Fatal("buildIPRateStore must not return nil when RegisterMetrics fails")
	}
	if _, ok := store.(*middleware.RedisRateStore); !ok {
		t.Errorf("expected *middleware.RedisRateStore despite metric error, got %T", store)
	}
}

// ── recordRateStoreMode ───────────────────────────────────────────────────────

// errFailGaugeMeter is a noop Meter that forces Int64Gauge to return an
// error, exercising the failed-registration branch in recordRateStoreMode.
type errFailGaugeMeter struct{ metricnoop.Meter }

var errForcedGauge = errors.New("injected gauge failure")

func (errFailGaugeMeter) Int64Gauge(_ string, _ ...metric.Int64GaugeOption) (metric.Int64Gauge, error) {
	return nil, errForcedGauge
}

// TestRecordRateStoreMode_GaugeRegistrationError_DoesNotPanic exercises the
// fail-soft branch: when gauge registration fails, recordRateStoreMode logs a
// warning and returns without recording anything, rather than panicking or
// propagating the error (there's nothing more useful a caller could do with
// it — this metric is best-effort observability, not load-bearing).
func TestRecordRateStoreMode_GaugeRegistrationError_DoesNotPanic(t *testing.T) {
	s := &Server{log: zaptest.NewLogger(t)}
	s.recordRateStoreMode(context.Background(), errFailGaugeMeter{}, "ip")
}

// TestRecordRateStoreMode_NoRedis_RecordsLocalMode verifies that with no Redis
// client configured, the gauge is recorded with mode=local for the given
// limiter label.
func TestRecordRateStoreMode_NoRedis_RecordsLocalMode(t *testing.T) {
	s := &Server{log: zaptest.NewLogger(t)} // redisClient == nil
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	s.recordRateStoreMode(context.Background(), mp.Meter("test"), "user")

	mode, limiter := collectRateStoreModeAttrs(t, reader)
	if mode != "local" || limiter != "user" {
		t.Errorf("expected mode=local limiter=user, got mode=%s limiter=%s", mode, limiter)
	}
}

// TestRecordRateStoreMode_WithRedis_RecordsRedisMode verifies that with a
// Redis client configured, the gauge is recorded with mode=redis.
func TestRecordRateStoreMode_WithRedis_RecordsRedisMode(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	s.recordRateStoreMode(context.Background(), mp.Meter("test"), "ip")

	mode, limiter := collectRateStoreModeAttrs(t, reader)
	if mode != "redis" || limiter != "ip" {
		t.Errorf("expected mode=redis limiter=ip, got mode=%s limiter=%s", mode, limiter)
	}
}

// collectRateStoreModeAttrs reads the "mode" and "limiter" attributes off the
// single data point of the wcq_ratelimit_store_mode gauge.
func collectRateStoreModeAttrs(t *testing.T, reader *sdkmetric.ManualReader) (mode, limiter string) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "wcq_ratelimit_store_mode" {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || len(g.DataPoints) == 0 {
				t.Fatalf("metric %q found but has unexpected data type or no data points", m.Name)
			}
			attrs := g.DataPoints[0].Attributes
			modeVal, _ := attrs.Value(attribute.Key("mode"))
			limiterVal, _ := attrs.Value(attribute.Key("limiter"))
			return modeVal.AsString(), limiterVal.AsString()
		}
	}
	t.Fatalf("metric wcq_ratelimit_store_mode not found")
	return "", ""
}

// ── jwksIssuerOpt ──────────────────────────────────────────────────────────────

// TestJWKSIssuerOpt_NoIssuerConfigured_ReturnsNil verifies that issuer
// validation stays opt-in when WCQ_CLERK_ISSUER is not set.
func TestJWKSIssuerOpt_NoIssuerConfigured_ReturnsNil(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	if opts := s.jwksIssuerOpt(); opts != nil {
		t.Errorf("expected nil options when issuer is not configured, got %v", opts)
	}
}

// TestJWKSIssuerOpt_IssuerConfigured_ReturnsOneOption verifies that a single
// auth.WithIssuer option is returned when an issuer is configured.
func TestJWKSIssuerOpt_IssuerConfigured_ReturnsOneOption(t *testing.T) {
	s := &Server{cfg: &config.Config{Clerk: config.ClerkConfig{Issuer: "https://clerk.example.com"}}}
	opts := s.jwksIssuerOpt()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 option when issuer is configured, got %d", len(opts))
	}
}

// TestBuildIPRateStore_AllowsRequests verifies that the store returned by
// buildIPRateStore (regardless of Redis availability) implements IPAllower
// and allows requests within the configured burst.
func TestBuildIPRateStore_AllowsRequests(t *testing.T) {
	t.Run("in_process", func(t *testing.T) {
		s := &Server{log: zaptest.NewLogger(t)}
		store := s.buildIPRateStore(metricnoop.NewMeterProvider().Meter("test"), 100.0, 10)
		allowed, _ := store.Allow(t.Context(), "ip:global:1.2.3.4")
		if !allowed {
			t.Error("expected Allow=true for first request within burst, got false")
		}
	})

	t.Run("redis_backed", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = rc.Close() })

		s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
		store := s.buildIPRateStore(metricnoop.NewMeterProvider().Meter("test"), 100.0, 10)
		allowed, _ := store.Allow(t.Context(), "ip:global:5.6.7.8")
		if !allowed {
			t.Error("expected Allow=true for first request within burst, got false")
		}
	})
}

// ── buildPayoutEncrypter ──────────────────────────────────────────────────────

func TestBuildPayoutEncrypter_EmptyKey_ReturnsNoop(t *testing.T) {
	enc := buildPayoutEncrypter("", zaptest.NewLogger(t))
	if enc.IsEnabled() {
		t.Error("expected Noop encrypter for empty key: IsEnabled must be false")
	}
}

func TestBuildPayoutEncrypter_ValidKey_ReturnsEnabledEncrypter(t *testing.T) {
	// 64 hex chars = 32 bytes = valid AES-256 key.
	validKey := strings.Repeat("ab", 32)
	enc := buildPayoutEncrypter(validKey, zaptest.NewLogger(t))
	if !enc.IsEnabled() {
		t.Error("expected AES-GCM encrypter for valid 32-byte key: IsEnabled must be true")
	}
}
