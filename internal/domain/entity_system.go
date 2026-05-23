package domain

import "time"

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
