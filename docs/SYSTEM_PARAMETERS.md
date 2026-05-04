# System Parameters Documentation

## Overview

The World Cup Quiniela application uses a **dual-layer configuration system**:

1. **Hard-coded defaults** in `internal/domain/constants.go` - Used as fallback values when the database is unavailable or a parameter is missing
2. **Database-driven overrides** in the `system_params` table - Allows operators to tune values at runtime without code changes

This document describes the complete system parameters catalog, validation procedures, and maintenance guidelines.

---

## Architecture

### Two-Layer Configuration Model

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                      │
├─────────────────────────────────────────────────────────────┤
│  Service calls:                                             │
│  paramSvc.GetInt(ctx, ParamKeyConflictMaxScan,             │
│                       DefaultConflictMaxScan)               │
│                                                             │
│  ┌─────────────────────┐         ┌──────────────────────┐ │
│  │  1. Try DB lookup   │────────▶│ system_params table  │ │
│  │     (runtime value) │         │ key = "conflict..."  │ │
│  └─────────────────────┘         └──────────────────────┘ │
│           │                                                 │
│           │ If not found / DB unavailable                   │
│           ▼                                                 │
│  ┌─────────────────────┐         ┌──────────────────────┐ │
│  │  2. Use fallback    │────────▶│ constants.go         │ │
│  │     (hard-coded)    │         │ const Default...     │ │
│  └─────────────────────┘         └──────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Benefits

- **Graceful degradation**: App works even if database is temporarily unavailable
- **Runtime tunability**: Change values without code deploy or process restart (runtime params only)
- **Type safety**: Go compiler validates default values at compile time
- **Auditability**: All param changes are logged via audit trail
- **Testability**: Tests can override params without touching the database

---

## Parameter Catalog

### 📊 Complete Parameter List (23 parameters)

| Key | Default | Category | Runtime | Description |
|-----|---------|----------|---------|-------------|
| `scoring.exact_score` | 5 | scoring | ✅ | Points awarded for exact score match |
| `scoring.correct_outcome` | 2 | scoring | ✅ | Points for correct outcome (win/loss/draw) |
| `scoring.goal_difference` | 1 | scoring | ✅ | Bonus point for correct goal margin |
| `prediction.deadline_minutes` | 5 | prediction | ✅ | Minutes before kickoff when predictions lock |
| `group.min_members_for_active` | 3 | group | ✅ | Minimum active members for prize eligibility |
| `group.default_prize_threshold` | 3 | group | ✅ | Prize distribution ratio (winners = members / threshold) |
| `group.invite_code_length` | 10 | group | ✅ | Length of generated invite codes |
| `conflict.stale_days` | 7 | conflict | ✅ | Days before pending items flagged as stale |
| `conflict.max_scan` | 5000 | conflict | ✅ | Max conflicts loaded in ConflictSummary |
| `pagination.default_limit` | 50 | pagination | ✅ | Default page size when ?limit not specified |
| `pagination.max_limit` | 200 | pagination | ✅ | Maximum allowed page size |
| `tournament.win_points` | 3 | tournament | ✅ | Standing points for group-stage win |
| `admin.bulk_max_items` | 1000 | admin | ✅ | Max IDs in bulk operations |
| `cache.match_ttl_seconds` | 300 | cache | ❌ | Match cache TTL (restart to apply) |
| `cache.leaderboard_ttl_seconds` | 60 | cache | ❌ | Leaderboard cache TTL (restart to apply) |
| `cache.dashboard_ttl_seconds` | 30 | cache | ✅ | Dashboard cache TTL (runtime tunable) |
| `audit.write_timeout_seconds` | 5 | system | ❌ | Audit log write timeout (restart to apply) |
| `auth.validation_timeout_seconds` | 5 | system | ❌ | JWKS warm-up timeout (restart to apply) |
| `dlq.sample_size` | 5 | dlq | ❌ | Max DLQ entries in Stats sample |
| `dlq.replay_default_limit` | 10 | dlq | ❌ | Default DLQ replay batch size |
| `messaging.max_retries` | 3 | messaging | ❌ | Event handler retry attempts |
| `messaging.stream_max_len` | 600000 | messaging | ❌ | Redis Stream MAXLEN cap |
| `system.purge_retention_days` | 30 | system | ❌ | Days before soft-deleted items purged |

**Legend:**
- ✅ **Runtime** = Changes take effect immediately (no restart needed)
- ❌ **Infrastructure** = Changes require process restart

---

## Categories

### 🎯 Business Parameters (runtime = TRUE)

#### Scoring
Controls point values for prediction outcomes. Modifying these mid-tournament would be unfair - only change before tournament starts.

#### Prediction
Deadline before kickoff when predictions lock. Can be adjusted for specific matches (e.g., extend deadline for delayed matches).

#### Group
Quiniela lifecycle rules. Changing `min_members_for_active` mid-tournament may affect prize distribution.

#### Conflict
Operational health thresholds. Increase `max_scan` if conflict backlog is pathologically large (>5000).

#### Pagination
API page size limits. Safe to adjust based on client behavior or load patterns.

#### Tournament
Standings calculation rules. Should match FIFA regulations; don't change mid-tournament.

#### Admin
Operational limits for bulk admin actions. Lower during high-load periods to protect database.

### 🔧 Infrastructure Parameters (runtime = FALSE)

#### Cache
Redis TTL configuration. Changes require restart except `dashboard_ttl_seconds`.

#### System
Process-level timeouts and lifecycle. Restart required to apply.

#### DLQ
Dead-letter queue behavior. Restart required.

#### Messaging
Event bus retry policy. Restart required.

---

## Validation

### Automated Tests

Run the constants validation suite:

```bash
go test ./internal/domain/... -run ".*Constant.*" -v
```

**Tests include:**
- All ParamKey constants are enumerated
- All Default constants are positive
- No duplicate param keys
- Naming conventions followed (category.snake_case)
- Constants properly paired (ParamKey ↔ Default)

### Database Validation

Run the SQL validation script:

```bash
psql -d quiniela -f scripts/validate_system_params.sql
```

**Validates:**
- All 23 expected parameters are present
- No parameters missing descriptions
- No orphaned parameters (in DB but not in constants.go)
- Categories are correct
- Runtime flags are correct

**Sample Output:**
```
=== System Parameters Validation Report ===

1. Checking total parameter count...
 total_params | expected_params | status
--------------+-----------------+--------
           23 |              23 | ✓ PASS

2. Checking for missing descriptions...
✓ All parameters have descriptions

7. Expected parameters checklist (must all be present)...
        expected_key         | status    | value  | description
-----------------------------+-----------+--------+-------------
 admin.bulk_max_items        | ✓ Present | 1000   | Maximum IDs...
 audit.write_timeout_seconds | ✓ Present | 5      | Maximum time...
 ...
```

---

## Maintenance

### Adding a New Parameter

1. **Define constants in `constants.go`:**
   ```go
   const (
       DefaultMyNewParam = 100
   )
   
   const (
       ParamKeyMyNewParam = "category.my_new_param"
   )
   ```

2. **Create migration** (e.g., `000050_add_my_new_param.up.sql`):
   ```sql
   INSERT INTO system_params (key, value, type, category, is_runtime, description)
   VALUES (
       'category.my_new_param',
       '100',
       'int',
       'category',
       TRUE,
       'Description of what this parameter controls'
   )
   ON CONFLICT (key) DO NOTHING;
   ```

3. **Update validation test** `constants_validation_test.go`:
   ```go
   // Add to TestSystemParamConstants_AllPaired:
   paramKeys["ParamKeyMyNewParam"] = ParamKeyMyNewParam
   
   // Add to defaults map:
   defaults["DefaultMyNewParam"] = DefaultMyNewParam
   
   // Update expectedCount in smoke tests:
   expectedCount := 24 // was 23
   ```

4. **Update validation script** `validate_system_params.sql`:
   ```sql
   -- Add to expected_params array:
   'category.my_new_param'
   ```

5. **Run tests:**
   ```bash
   go test ./internal/domain/... -run ".*Constant.*" -v
   go build ./...
   ```

6. **Apply migration:**
   ```bash
   make migrate-up
   # or: psql -d quiniela -f migrations/000050_add_my_new_param.up.sql
   ```

### Modifying a Parameter Value

**Option 1: Via API (Runtime params only)**
```bash
curl -X PATCH http://api/admin/system-params/conflict.max_scan \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"value": "10000"}'
```

**Option 2: Direct SQL (All params)**
```sql
UPDATE system_params
SET value = '10000'
WHERE key = 'conflict.max_scan';
```

**Option 3: Via Migration (Permanent change)**
```sql
-- migrations/000051_increase_conflict_max_scan.up.sql
UPDATE system_params
SET value = '10000'
WHERE key = 'conflict.max_scan';
```

### Deprecating a Parameter

1. **Mark as deprecated** (don't delete from DB):
   ```sql
   UPDATE system_params
   SET description = '[DEPRECATED] ' || description
   WHERE key = 'old.param';
   ```

2. **Remove from constants.go** (but keep fallback for backwards compatibility):
   ```go
   // const ParamKeyOldParam = "old.param" // DEPRECATED - remove in v2.0
   ```

3. **Update validation test** to exclude deprecated param

4. **Document in migration**:
   ```sql
   -- NOTE: old.param is deprecated but retained for backwards compatibility.
   -- Will be removed in v2.0.
   ```

---

## Troubleshooting

### ❌ Parameter Not Found in Database

**Symptom:** App uses default value even though you set a value in the DB.

**Cause:** Typo in param key, or migration not applied.

**Fix:**
```bash
# 1. Verify key spelling
psql -d quiniela -c "SELECT key FROM system_params WHERE key LIKE '%conflict%';"

# 2. Check if migration was applied
psql -d quiniela -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 10;"

# 3. Apply missing migration
make migrate-up
```

### ❌ Test Fails: "ParamKey enumeration may be incomplete"

**Symptom:** `TestSystemParamConstants_AllPaired` fails with count mismatch.

**Cause:** You added a new ParamKey but didn't update the test.

**Fix:** Follow steps in "Adding a New Parameter" section above.

### ❌ Validation Script Shows Orphaned Parameter

**Symptom:** `validate_system_params.sql` reports a parameter not in constants.go.

**Cause:** Parameter was removed from code but still exists in DB.

**Fix:**
```sql
-- Option 1: Mark as deprecated
UPDATE system_params
SET description = '[DEPRECATED] ' || description
WHERE key = 'orphaned.param';

-- Option 2: Delete (DANGEROUS - verify it's truly unused first)
DELETE FROM system_params WHERE key = 'orphaned.param';
```

---

## Migration Strategy

### Initial Seed (000040)
Seeds all original params without descriptions.

### Incremental Additions (000041-000048)
Add individual params as features are developed.

### Self-Healing (000045)
Re-inserts all params to recover from accidental deletions.

### Description Backfill (000049)
Adds descriptions to all params for documentation.

### Idempotency
All migrations use `ON CONFLICT (key) DO NOTHING` or `DO UPDATE` to ensure:
- Safe to re-run
- Safe to apply against production with operator overrides
- Values are never overwritten (only metadata like description)

---

## API Endpoints

### List All Parameters
```bash
GET /api/v1/admin/system-params
Authorization: Bearer <admin-token>
```

**Response:**
```json
{
  "data": [
    {
      "key": "conflict.max_scan",
      "value": "5000",
      "type": "int",
      "category": "conflict",
      "is_runtime": true,
      "description": "Maximum conflicts loaded..."
    }
  ]
}
```

### Get Single Parameter
```bash
GET /api/v1/admin/system-params/conflict.max_scan
```

### Update Parameter
```bash
PATCH /api/v1/admin/system-params/conflict.max_scan
Content-Type: application/json

{"value": "10000"}
```

**Note:** Only admins can modify parameters. All changes are audit-logged.

---

## Best Practices

### ✅ DO
- Use runtime params for business rules that may need tuning
- Use infrastructure params for values read once at startup
- Document every parameter with a clear description
- Test param changes in staging before production
- Use migrations for permanent changes
- Use API for temporary tuning during incidents

### ❌ DON'T
- Change scoring params mid-tournament
- Set infrastructure params via API (restart needed anyway)
- Delete params from DB (mark deprecated instead)
- Use 0 or negative values (validation will fail)
- Hardcode param values in code (defeats the purpose)

---

## References

- **Constants Definition:** `internal/domain/constants.go`
- **Validation Tests:** `internal/domain/constants_validation_test.go`
- **SQL Validation:** `scripts/validate_system_params.sql`
- **Main Migration:** `migrations/000049_complete_system_params_with_descriptions.up.sql`
- **API Docs:** See `/swagger` endpoint for interactive documentation

---

**Last Updated:** 2026-05-03  
**Total Parameters:** 23  
**Coverage:** 100%
