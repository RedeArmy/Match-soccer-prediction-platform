package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func seedExchangeRate(t *testing.T, repo repository.ExchangeRateRepository, effectiveAt time.Time, source string, isOverride bool, overrideBy *int) *domain.ExchangeRateRecord {
	t.Helper()
	rec := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.7500"),
		BuyRate:       decimal.RequireFromString("7.6338"),
		SellRate:      decimal.RequireFromString("7.9050"),
		BuyMarginPct:  decimal.RequireFromString("0.0150"),
		SellMarginPct: decimal.RequireFromString("0.0200"),
		Source:        source,
		Stale:         false,
		IsOverride:    isOverride,
		OverrideBy:    overrideBy,
		FetchedAt:     effectiveAt,
		EffectiveAt:   effectiveAt,
	}
	if err := repo.Save(context.Background(), rec); err != nil {
		t.Fatalf("seedExchangeRate: %v", err)
	}
	return rec
}

// ── Save ──────────────────────────────────────────────────────────────────────

func TestExchangeRateRepository_Save_InsertsRow(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	rec := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.7500"),
		BuyRate:       decimal.RequireFromString("7.6338"),
		SellRate:      decimal.RequireFromString("7.9050"),
		BuyMarginPct:  decimal.RequireFromString("0.0150"),
		SellMarginPct: decimal.RequireFromString("0.0200"),
		Source:        "banguat",
		Stale:         false,
		IsOverride:    false,
		FetchedAt:     time.Now(),
		EffectiveAt:   time.Now(),
	}
	if err := repo.Save(context.Background(), rec); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	latest, err := repo.GetLatest(context.Background())
	if err != nil {
		t.Fatalf("GetLatest after Save: %v", err)
	}
	if latest == nil {
		t.Fatal("expected row after Save, got nil")
	}
	if latest.Source != "banguat" {
		t.Errorf("Source: want banguat, got %s", latest.Source)
	}
	if !latest.ReferenceRate.Equal(rec.ReferenceRate) {
		t.Errorf("ReferenceRate: want %s, got %s", rec.ReferenceRate, latest.ReferenceRate)
	}
}

func TestExchangeRateRepository_Save_WithOverrideReason_PersistsMetadata(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	rec := &domain.ExchangeRateRecord{
		ReferenceRate:  decimal.RequireFromString("7.90"),
		BuyRate:        decimal.RequireFromString("7.7820"),
		SellRate:       decimal.RequireFromString("8.0580"),
		BuyMarginPct:   decimal.RequireFromString("0.015"),
		SellMarginPct:  decimal.RequireFromString("0.020"),
		Source:         "override",
		IsOverride:     true,
		OverrideReason: "emergency adjustment during bank holiday",
		OverrideBy:     &u.ID,
		FetchedAt:      time.Now(),
		EffectiveAt:    time.Now(),
	}
	if err := repo.Save(context.Background(), rec); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	latest, err := repo.GetLatest(context.Background())
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if !latest.IsOverride {
		t.Error("expected is_override=true")
	}
	if latest.OverrideReason != "emergency adjustment during bank holiday" {
		t.Errorf("override_reason: want %q, got %q", "emergency adjustment during bank holiday", latest.OverrideReason)
	}
	if latest.OverrideBy == nil || *latest.OverrideBy != u.ID {
		t.Errorf("override_by: want %d, got %v", u.ID, latest.OverrideBy)
	}
}

// ── GetLatest ─────────────────────────────────────────────────────────────────

func TestExchangeRateRepository_GetLatest_EmptyTable_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	latest, err := repo.GetLatest(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if latest != nil {
		t.Errorf("expected nil from empty table, got %+v", latest)
	}
}

func TestExchangeRateRepository_GetLatest_ReturnsMostRecentByEffectiveAt(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	base := time.Now().Truncate(time.Second)
	seedExchangeRate(t, repo, base.Add(-2*time.Hour), "banguat", false, nil)
	seedExchangeRate(t, repo, base.Add(-1*time.Hour), "exchangerate-api", false, nil)
	seedExchangeRate(t, repo, base, "openexchangerates", false, nil) // most recent

	latest, err := repo.GetLatest(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if latest == nil {
		t.Fatal("expected a row, got nil")
	}
	if latest.Source != "openexchangerates" {
		t.Errorf("Source: want openexchangerates (most recent), got %s", latest.Source)
	}
}

// ── GetHistory ────────────────────────────────────────────────────────────────

func TestExchangeRateRepository_GetHistory_ReturnsRowsNewestFirst(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	base := time.Now().Truncate(time.Second)
	seedExchangeRate(t, repo, base.Add(-2*time.Hour), "banguat", false, nil)
	seedExchangeRate(t, repo, base.Add(-1*time.Hour), "exchangerate-api", false, nil)
	seedExchangeRate(t, repo, base, "openexchangerates", false, nil)

	rows, err := repo.GetHistory(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// First row should be the most recent.
	if rows[0].Source != "openexchangerates" {
		t.Errorf("rows[0].Source: want openexchangerates, got %s", rows[0].Source)
	}
}

func TestExchangeRateRepository_GetHistory_PaginationLimit(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	base := time.Now().Truncate(time.Second)
	for i := range 5 {
		seedExchangeRate(t, repo, base.Add(time.Duration(-i)*time.Hour), "banguat", false, nil)
	}

	rows, err := repo.GetHistory(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows with limit=2, got %d", len(rows))
	}
}

func TestExchangeRateRepository_GetHistory_EmptyTable_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	rows, err := repo.GetHistory(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows from empty table, got %d", len(rows))
	}
}

// ── GetOverrides ──────────────────────────────────────────────────────────────

func TestExchangeRateRepository_GetOverrides_FiltersAutomatedRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	base := time.Now().Truncate(time.Second)
	seedExchangeRate(t, repo, base.Add(-3*time.Hour), "banguat", false, nil)   // automated
	seedExchangeRate(t, repo, base.Add(-2*time.Hour), "override", true, &u.ID) // override
	seedExchangeRate(t, repo, base.Add(-1*time.Hour), "banguat", false, nil)   // automated
	seedExchangeRate(t, repo, base, "override", true, &u.ID)                   // override

	rows, err := repo.GetOverrides(context.Background(), 10)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 override rows, got %d", len(rows))
	}
	for _, r := range rows {
		if !r.IsOverride {
			t.Errorf("row with source %q should have is_override=true", r.Source)
		}
	}
}

func TestExchangeRateRepository_GetOverrides_EmptyTable_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresExchangeRateRepository(testDB)

	rows, err := repo.GetOverrides(context.Background(), 10)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows from empty table, got %d", len(rows))
	}
}
