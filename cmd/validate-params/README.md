# System Parameters Validator

**Purpose:** Automated validation tool that ensures `internal/domain/constants.go` stays synchronized with the `system_params` table.

## What It Validates

1. **Completeness**: Every `ParamKey*` constant has a corresponding row in `system_params`
2. **Type Safety**: Database `type` column matches expected data type (`int`, `string`, `bool`, `duration`)
3. **Categorization**: Each param is in the correct category (scoring, group, cache, etc.)
4. **Documentation**: All params have non-empty `description` field
5. **Default Values**: Reports when DB values differ from code defaults (operator overrides)

## Usage

### Local Validation

```bash
# Set DATABASE_URL to your local/test database
export DATABASE_URL="postgres://user:pass@localhost:5432/quiniela_test"

# Run validator
go run cmd/validate-params/main.go
```

### CI Integration

Add to `.github/workflows/test.yml`:

```yaml
- name: Validate System Parameters
  run: |
    export DATABASE_URL="${{ secrets.TEST_DATABASE_URL }}"
    go run cmd/validate-params/main.go
```

## Output Examples

### ✅ Success

```
✅ scoring.exact_score = 5 (int, scoring)
✅ scoring.correct_outcome = 2 (int, scoring)
✅ prediction.deadline_minutes = 5 (int, prediction)
...
✅ VALIDATION PASSED: All 24 system parameters are correctly configured
```

### ❌ Failure

```
❌ MISSING: cache.dashboard_ttl_seconds (expected default: 30)
❌ TYPE MISMATCH: pagination.max_limit (expected: int, got: string)
❌ CATEGORY MISMATCH: conflict.max_scan (expected: conflict, got: admin)
❌ MISSING DESCRIPTION: dlq.sample_size

❌ VALIDATION FAILED:
system_params validation failed with 4 error(s)
```

### ⚠️ Warnings

```
⚠️  VALUE OVERRIDE: pagination.max_limit (code default: 200, DB value: 100) — operator override detected
⚠️  UNEXPECTED PARAM IN DB: legacy.deprecated_flag (not defined in constants.go) — consider removing
```

## When to Run

- **Pre-commit**: Before committing changes to `constants.go`
- **Post-migration**: After running `migrate up` to verify seed data
- **CI pipeline**: On every PR to catch drift early
- **Production deploy**: As a pre-flight check before releasing

## Maintenance

When adding a new system parameter:

1. Add `Default*` constant to `internal/domain/constants.go`
2. Add `ParamKey*` constant to `internal/domain/constants.go`
3. Add seed row to `migrations/000049_complete_system_params_with_descriptions.up.sql`
4. Add `paramSpec` to `cmd/validate-params/main.go:allParams`
5. Run validator to confirm: `go run cmd/validate-params/main.go`

## Architecture

```
┌─────────────────────┐
│  constants.go       │
│  - Default values   │ ← Single source of truth
│  - ParamKey names   │
└──────────┬──────────┘
           │
           ├─────────────────────────────────┐
           │                                 │
           ▼                                 ▼
┌─────────────────────┐           ┌─────────────────────┐
│  Migration 000049   │           │  validate-params    │
│  - Seed data        │           │  - Audits sync      │
│  - ON CONFLICT      │◄──────────┤  - CI enforcement   │
└─────────────────────┘           └─────────────────────┘
           │
           ▼
┌─────────────────────┐
│  system_params      │
│  - Runtime config   │
│  - Operator tuning  │
└─────────────────────┘
```

## Exit Codes

- **0**: All validations passed
- **1**: Validation failed or database connection error

## Dependencies

- **DATABASE_URL**: PostgreSQL connection string (required)
- **Go 1.21+**: For building/running the validator
- **pgx/v5**: PostgreSQL driver (already in go.mod)

## Related Files

- `internal/domain/constants.go` — Code constants
- `migrations/000049_complete_system_params_with_descriptions.up.sql` — DB seed
- `SYSTEM_PARAMS_VALIDATION.md` — Manual validation report
- `internal/service/system_param_service.go` — Runtime param reader

---

**SDE III Best Practice:** This validator enforces the "single source of truth" principle at build/deploy time, preventing runtime failures from stale configuration.
