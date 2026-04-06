// Package logger provides a factory for constructing a *zap.Logger configured
// for this application's operational environments.
//
// We wrap zap rather than using it directly at every call site for two
// reasons. First, it lets us enforce a consistent encoder configuration
// across the entire application without scattering zap.Config literals
// everywhere. Second, it gives us a single place to extend in future:
// log sampling, redaction of sensitive fields, or integration with an
// error-tracking service (e.g. Sentry) can be added here without touching
// any other package.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config carries the parameters needed to initialise the logger.
//
// This is intentionally a separate type from pkg/config.LoggerConfig to
// avoid importing the config package here. Introducing that import would
// couple the logger package to the entire configuration graph, making it
// harder to use the logger in tests or utilities that do not need the full
// application config.
type Config struct {
	Level    string
	Encoding string
}

// New constructs a *zap.Logger from the provided Config.
//
// Two encoding modes are supported:
//   - "json":    Structured JSON output intended for production. Fields are
//     machine-parseable, which allows log aggregation tools
//     (Datadog, CloudWatch, GCP Logging) to index and alert on
//     specific field values without fragile regex patterns.
//   - "console": Human-readable, coloured output for local development.
//     This mode is substantially slower than JSON encoding and
//     produces output that most log aggregators cannot parse;
//     it must never be used in production.
//
// All log levels are written to stdout, and internal logger errors are
// written to stderr. This follows the twelve-factor app convention of
// treating logs as a stream of events, leaving routing and persistence
// to the execution environment.
func New(cfg Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Level, err)
	}

	zapCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      cfg.Encoding == "console",
		Encoding:         cfg.Encoding,
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	// The development encoder config uses a more readable timestamp format
	// and enables colour-coded level names. It is only applied when the
	// console encoding is requested, keeping production output in the compact
	// epoch-seconds format expected by most log ingestion pipelines.
	if cfg.Encoding == "console" {
		zapCfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	return zapCfg.Build()
}
