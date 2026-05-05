# System Parameters Validation Report

**Date:** 2026-05-04  
**Engineer:** Claude Sonnet 4.5  
**Purpose:** Validate synchronization between domain constants and system_params table

---

## Mapping: Constants → System Params

### ✅ Scoring Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `PointsExactScore` | 5 | `scoring.exact_score` | 5 | ✅ Match |
| `PointsCorrectOutcome` | 2 | `scoring.correct_outcome` | 2 | ✅ Match |
| `PointsGoalDifference` | 1 | `scoring.goal_difference` | 1 | ✅ Match |
| `PointsIncorrectResult` | 0 | N/A | N/A | ✅ Not configurable (hardcoded 0) |

### ✅ Prediction Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `PredictionDeadlineOffset` | 5 minutes | `prediction.deadline_minutes` | 5 | ✅ Match |

### ✅ Group / Quiniela Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `MinMembersForActive` | 3 | `group.min_members_for_active` | 3 | ✅ Match |
| `DefaultPrizeThreshold` | 3 | `group.default_prize_threshold` | 3 | ✅ Match |
| `DefaultGroupInviteCodeLength` | 10 | `group.invite_code_length` | 10 | ✅ Match |

### ✅ Conflict Detection Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultConflictStaleDays` | 7 | `conflict.stale_days` | 7 | ✅ Match |
| `DefaultConflictMaxScan` | 5000 | `conflict.max_scan` | 5000 | ✅ Match |

### ✅ Pagination Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultPaginationDefaultLimit` | 50 | `pagination.default_limit` | 50 | ✅ Match |
| `DefaultPaginationMaxLimit` | 200 | `pagination.max_limit` | 200 | ✅ Match |

### ✅ Tournament Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `StandingsWinPoints` | 3 | `tournament.win_points` | 3 | ✅ Match |

### ✅ Admin Bulk Operations

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultAdminBulkMaxItems` | 1000 | `admin.bulk_max_items` | 1000 | ✅ Match |

### ✅ Cache TTL Parameters

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultCacheMatchTTLSeconds` | 300 | `cache.match_ttl_seconds` | 300 | ✅ Match |
| `DefaultCacheLeaderboardTTLSeconds` | 60 | `cache.leaderboard_ttl_seconds` | 60 | ✅ Match |
| `DefaultCacheDashboardTTLSeconds` | 30 | `cache.dashboard_ttl_seconds` | 30 | ✅ Match |

### ✅ Infrastructure Timeouts

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultAuditWriteTimeoutSeconds` | 5 | `audit.write_timeout_seconds` | 5 | ✅ Match |
| `DefaultAuthValidationTimeoutSeconds` | 5 | `auth.validation_timeout_seconds` | 5 | ✅ Match |

### ✅ Dead-Letter Queue

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultDLQSampleSize` | 5 | `dlq.sample_size` | 5 | ✅ Match |
| `DefaultDLQReplayDefaultLimit` | 10 | `dlq.replay_default_limit` | 10 | ✅ Match |

### ✅ Messaging / Event Bus

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultMessagingMaxRetries` | 3 | `messaging.max_retries` | 3 | ✅ Match |
| `DefaultMessagingStreamMaxLen` | 600,000 | `messaging.stream_max_len` | 600000 | ✅ Match |

### ✅ System Lifecycle

| Constant | Value | ParamKey | DB Value | Status |
|----------|-------|----------|----------|--------|
| `DefaultPurgeRetentionDays` | 30 | `system.purge_retention_days` | 30 | ✅ Match |

---

## Validation Constraints

### ❌ Missing Validation Constraints

The following constants define **validation limits** (not runtime configuration) and should **NOT** be in system_params:

| Constant | Value | Reason for Exclusion |
|----------|-------|---------------------|
| `MaxEmailLength` | 320 | Input validation limit (DoS prevention) - must be enforced at application boundary, not configurable |
| `MaxNameLength` | 200 | Input validation limit (DoS prevention) - must be enforced at application boundary, not configurable |
| `MaxTeamNameLength` | 100 | Input validation limit (DoS prevention) - must be enforced at application boundary, not configurable |

**Rationale:** These are **security boundaries**, not business rules. Making them configurable would:
1. Allow operators to accidentally weaken DoS protection
2. Create consistency issues between code and DB schema VARCHAR limits
3. Risk REQUEST_ENTITY_TOO_LARGE middleware bypass

---

## Summary

### ✅ Complete Coverage
- **Total Constants:** 26
- **System Params:** 23
- **Validation Constants (excluded):** 3
- **Match Rate:** 100% (23/23)

### ✅ All Values Synchronized
Every `Default*` constant in `constants.go` has:
1. A corresponding `ParamKey*` constant
2. A matching row in migration `000049_complete_system_params_with_descriptions.up.sql`
3. Identical default value
4. Correct type and category
5. Complete description

### Migration File Status
- **Primary source:** `migrations/000049_complete_system_params_with_descriptions.up.sql`
- **Rows:** 23
- **Categories:** 11 (scoring, prediction, group, conflict, pagination, tournament, admin, cache, system, dlq, messaging)
- **Coverage:** Complete

---

## Compliance Checklist

- [x] Every `ParamKey*` constant has a corresponding `Default*` constant
- [x] Every `Default*` constant value matches the DB seed value
- [x] All params have type annotation (`int`, `string`, `bool`, `duration`)
- [x] All params have category classification
- [x] All params have `is_runtime` flag set correctly
- [x] All params have descriptive documentation in migration
- [x] Validation constants are explicitly NOT in system_params
- [x] Comments in constants.go reference the migration file

---

## Recommendations

### ✅ Already Implemented
1. **Single source of truth:** Migration 000049 is canonical
2. **Graceful degradation:** Services fall back to constants when param is missing
3. **ON CONFLICT protection:** Operator value overrides preserved during migration re-runs
4. **Type safety:** System param repository validates types

### Future Enhancements
1. **Runtime validation tool:** Create `cmd/validate-params` to audit code↔DB sync at CI time
2. **Admin UI:** Build system params management page (currently requires direct SQL)
3. **Audit trail:** Log param changes to `audit_log` table
4. **Version control:** Track param value history for rollback capability

---

## Code References

### Constants Definition
- `internal/domain/constants.go:105-135` - Default values
- `internal/domain/constants.go:142-211` - ParamKey names

### Migration Files
- `migrations/000032_create_system_params.up.sql` - Table schema
- `migrations/000049_complete_system_params_with_descriptions.up.sql` - Complete seed data

### Service Layer Usage
Services read params via `SystemParamService.GetInt()`, `GetString()`, etc., with automatic fallback to constants when DB value is absent or unparseable.

---

**Validation Result:** ✅ **PASS** - All system parameters are correctly defined and synchronized.
