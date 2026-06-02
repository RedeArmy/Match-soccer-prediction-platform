package domain

import (
	"strings"
	"time"
)

// SystemParamType constrains the Value interpretation for a SystemParam row.
// The infrastructure layer is responsible for parsing the raw text Value into
// the appropriate Go type before handing it to the service layer.
type SystemParamType string

// Allowed values for SystemParamType.
const (
	SystemParamTypeString   SystemParamType = "string"
	SystemParamTypeInt      SystemParamType = "int"
	SystemParamTypeBool     SystemParamType = "bool"
	SystemParamTypeDuration SystemParamType = "duration"
)

// SystemParam is a key-value configuration entry managed at runtime by
// administrators without requiring a deployment. IsRuntime = true means the
// service layer re-reads the value on each request (or on cache miss); false
// means the value is treated as boot-time configuration and a restart is
// needed to pick up changes.
//
// Category groups related params (e.g. "scoring", "payment", "leaderboard")
// to simplify admin UI rendering and bulk-fetch patterns.
type SystemParam struct {
	Key          string
	Value        string
	DefaultValue string
	Type         SystemParamType
	Category     string
	IsRuntime    bool
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SystemParamHistory is one immutable record of a system_params value change.
// Action is either "set" (operator override) or "reset" (restored to migration default).
// ActorID is always non-zero: only authenticated admin operators can mutate params.
type SystemParamHistory struct {
	ID        int64
	Key       string
	OldValue  string
	NewValue  string
	ActorID   int
	Action    string
	ChangedAt time.Time
}

// ParamDiff holds the projected old→new change for a single system param,
// returned by BulkPreview. IsSensitive is set when the key belongs to a
// category (scoring.* or payment.*) whose change has systemic impact and
// therefore requires explicit operator confirmation in a live BulkSet call.
type ParamDiff struct {
	Key         string
	OldValue    string
	NewValue    string
	IsSensitive bool
}

// sensitiveParamPrefixes are the system-param key prefixes that require
// explicit confirmation before a BulkSet modifies them.
//
// scoring.* — affects every future match result calculation.
// payment.* — affects user-facing financial limits and exchange rates.
// fx.*       — controls live buy/sell exchange rates shown to users.
var sensitiveParamPrefixes = []string{"scoring.", "payment.", "fx."}

// IsSensitiveParamKey returns true when key belongs to a category whose
// incorrect change has systemic or financial impact on all future operations.
func IsSensitiveParamKey(key string) bool {
	for _, prefix := range sensitiveParamPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
