# System Parameters Quick Reference

**Total:** 23 parameters | **Last Updated:** 2026-05-04

---

## Complete Parameter List

| # | Key | Default | Type | Category | Description |
|---|-----|---------|------|----------|-------------|
| 1 | `scoring.exact_score` | `5` | `int` | `scoring` | Points for exact score prediction |
| 2 | `scoring.correct_outcome` | `2` | `int` | `scoring` | Points for correct winner/draw |
| 3 | `scoring.goal_difference` | `1` | `int` | `scoring` | Bonus for correct goal margin |
| 4 | `prediction.deadline_minutes` | `5` | `int` | `prediction` | Minutes before kickoff to close predictions |
| 5 | `group.min_members_for_active` | `3` | `int` | `group` | Minimum members for active group status |
| 6 | `group.default_prize_threshold` | `3` | `int` | `group` | Members per prize winner |
| 7 | `group.invite_code_length` | `10` | `int` | `group` | Characters in generated invite codes |
| 8 | `conflict.stale_days` | `7` | `int` | `conflict` | Days before flagging stale conflicts |
| 9 | `conflict.max_scan` | `5000` | `int` | `conflict` | Max conflicts loaded in memory |
| 10 | `pagination.default_limit` | `50` | `int` | `pagination` | Default page size for lists |
| 11 | `pagination.max_limit` | `200` | `int` | `pagination` | Maximum page size allowed |
| 12 | `tournament.win_points` | `3` | `int` | `tournament` | Standings points for group stage win |
| 13 | `admin.bulk_max_items` | `1000` | `int` | `admin` | Max IDs in bulk operations |
| 14 | `cache.match_ttl_seconds` | `300` | `int` | `cache` | Match list cache TTL (5 min) |
| 15 | `cache.leaderboard_ttl_seconds` | `60` | `int` | `cache` | Leaderboard cache TTL (1 min) |
| 16 | `cache.dashboard_ttl_seconds` | `30` | `int` | `cache` | Dashboard stats cache TTL (30 sec) |
| 17 | `audit.write_timeout_seconds` | `5` | `int` | `system` | Audit log write timeout |
| 18 | `auth.validation_timeout_seconds` | `5` | `int` | `system` | JWKS warmup timeout |
| 19 | `system.purge_retention_days` | `30` | `int` | `system` | Soft-delete retention period |
| 20 | `dlq.sample_size` | `5` | `int` | `dlq` | Max DLQ entries in stats sample |
| 21 | `dlq.replay_default_limit` | `10` | `int` | `dlq` | Default replay batch size |
| 22 | `messaging.max_retries` | `3` | `int` | `messaging` | Event handler retry attempts |
| 23 | `messaging.stream_max_len` | `600000` | `int` | `messaging` | Redis Stream MAXLEN cap |

---

## By Category

### Scoring (3)
- `scoring.exact_score` = 5
- `scoring.correct_outcome` = 2
- `scoring.goal_difference` = 1

### Prediction (1)
- `prediction.deadline_minutes` = 5

### Group (3)
- `group.min_members_for_active` = 3
- `group.default_prize_threshold` = 3
- `group.invite_code_length` = 10

### Conflict (2)
- `conflict.stale_days` = 7
- `conflict.max_scan` = 5000

### Pagination (2)
- `pagination.default_limit` = 50
- `pagination.max_limit` = 200

### Tournament (1)
- `tournament.win_points` = 3

### Admin (1)
- `admin.bulk_max_items` = 1000

### Cache (3)
- `cache.match_ttl_seconds` = 300
- `cache.leaderboard_ttl_seconds` = 60
- `cache.dashboard_ttl_seconds` = 30

### System (3)
- `audit.write_timeout_seconds` = 5
- `auth.validation_timeout_seconds` = 5
- `system.purge_retention_days` = 30

### DLQ (2)
- `dlq.sample_size` = 5
- `dlq.replay_default_limit` = 10

### Messaging (2)
- `messaging.max_retries` = 3
- `messaging.stream_max_len` = 600000

---

## Validation

**Test:** `go test ./cmd/validate-params -v`

**Database Check:** `go run ./cmd/validate-params` (requires `DATABASE_URL`)

**Expected:** All 23 parameters present in `system_params` table with matching defaults.

---

## Code References

| File | Purpose |
|------|---------|
| `internal/domain/constants.go` | Default value constants (source of truth) |
| `internal/domain/constants.go` | ParamKey string constants |
| `cmd/validate-params/main.go` | Validation tool mapping |
| `cmd/validate-params/main_test.go` | Validation tests (6 tests) |
| `migrations/000040_seed_system_params.up.sql` | Database seed migration |

---

**Status:** ✅ All 23 parameters validated and documented
