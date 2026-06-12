package handler

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

func TestParsePaginationParams_BothAbsent_ReturnsDefaults(t *testing.T) {
	req := &http.Request{URL: &url.URL{}}
	limit, offset := parsePaginationParams(req)

	if limit != domain.DefaultPaginationDefaultLimit {
		t.Errorf("limit: expected %d, got %d", domain.DefaultPaginationDefaultLimit, limit)
	}
	if offset != 0 {
		t.Errorf("offset: expected 0, got %d", offset)
	}
}

func TestParsePaginationParams_OnlyLimit_ReturnsLimitZeroOffset(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=10"}}
	limit, offset := parsePaginationParams(req)

	if limit != 10 {
		t.Errorf("limit: expected 10, got %d", limit)
	}
	if offset != 0 {
		t.Errorf("offset: expected 0, got %d", offset)
	}
}

func TestParsePaginationParams_BothProvided_ReturnsBoth(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=50&offset=100"}}
	limit, offset := parsePaginationParams(req)

	if limit != 50 {
		t.Errorf("limit: expected 50, got %d", limit)
	}
	if offset != 100 {
		t.Errorf("offset: expected 100, got %d", offset)
	}
}

func TestParsePaginationParams_InvalidLimit_ReturnsDefault(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=invalid"}}
	limit, _ := parsePaginationParams(req)

	if limit != domain.DefaultPaginationDefaultLimit {
		t.Errorf("limit for invalid input: expected %d, got %d", domain.DefaultPaginationDefaultLimit, limit)
	}
}

func TestParsePaginationParams_NegativeLimit_ReturnsDefault(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=-5"}}
	limit, _ := parsePaginationParams(req)

	if limit != domain.DefaultPaginationDefaultLimit {
		t.Errorf("negative limit: expected %d, got %d", domain.DefaultPaginationDefaultLimit, limit)
	}
}

func TestParsePaginationParams_ExceedsMaxLimit_CapsAtMax(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=99999"}}
	limit, _ := parsePaginationParams(req)

	if limit != domain.DefaultPaginationMaxLimit {
		t.Errorf("over-max limit: expected %d, got %d", domain.DefaultPaginationMaxLimit, limit)
	}
}

func TestParsePaginationParams_ZeroLimit_ReturnsDefault(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=0"}}
	limit, _ := parsePaginationParams(req)

	if limit != domain.DefaultPaginationDefaultLimit {
		t.Errorf("zero limit: expected default %d, got %d", domain.DefaultPaginationDefaultLimit, limit)
	}
}

func TestParsePaginationParams_NegativeOffset_ClampsToZero(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawQuery: "limit=10&offset=-3"}}
	_, offset := parsePaginationParams(req)

	if offset != 0 {
		t.Errorf("negative offset: expected 0, got %d", offset)
	}
}

func TestApplySlicePagination_UnboundedLimit_ReturnsAllFromOffset(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := applySlicePagination(items, 0, 2)

	expected := []int{3, 4, 5}
	if len(result) != len(expected) {
		t.Fatalf("length: expected %d, got %d", len(expected), len(result))
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestApplySlicePagination_LimitAndOffset_ReturnsSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := applySlicePagination(items, 3, 2)

	expected := []int{3, 4, 5}
	if len(result) != len(expected) {
		t.Fatalf("length: expected %d, got %d", len(expected), len(result))
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestApplySlicePagination_OffsetPastEnd_ReturnsEmpty(t *testing.T) {
	items := []int{1, 2, 3}
	result := applySlicePagination(items, 10, 10)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestApplySlicePagination_LimitExceedsRemaining_ReturnsRemaining(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := applySlicePagination(items, 10, 2)

	expected := []int{3, 4, 5}
	if len(result) != len(expected) {
		t.Fatalf("length: expected %d, got %d", len(expected), len(result))
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestApplySlicePagination_EmptySlice_ReturnsEmpty(t *testing.T) {
	items := []int{}
	result := applySlicePagination(items, 10, 0)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestApplySlicePagination_ZeroOffset_ReturnsFromStart(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	result := applySlicePagination(items, 2, 0)

	expected := []string{"a", "b"}
	if len(result) != len(expected) {
		t.Fatalf("length: expected %d, got %d", len(expected), len(result))
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], result[i])
		}
	}
}

// ── formatUploadStorageKey ────────────────────────────────────────────────────

func TestFormatUploadStorageKey_KYCSelfie(t *testing.T) {
	ts := time.Date(2026, 6, 11, 11, 18, 59, 0, time.UTC)
	got := formatUploadStorageKey(9, "kyc", "selfie", ts, ".jpg")
	want := "kyc_9_selfie_2026_06_11_11:18:59.jpg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatUploadStorageKey_KYCGovID(t *testing.T) {
	ts := time.Date(2026, 6, 11, 10, 34, 45, 0, time.UTC)
	got := formatUploadStorageKey(9, "kyc", "gov_id", ts, ".pdf")
	want := "kyc_9_gov_id_2026_06_11_10:34:45.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatUploadStorageKey_BankTransferVoucher(t *testing.T) {
	ts := time.Date(2026, 6, 11, 15, 0, 0, 0, time.UTC)
	got := formatUploadStorageKey(42, "bank_transfer", "voucher", ts, ".png")
	want := "bank_transfer_42_voucher_2026_06_11_15:00:00.png"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatUploadStorageKey_UsesUTC(t *testing.T) {
	// A time in UTC+5: 14:00 → 09:00 UTC. The key must use UTC.
	loc := time.FixedZone("UTC+5", 5*60*60)
	ts := time.Date(2026, 1, 5, 14, 0, 0, 0, loc) // 09:00 UTC
	got := formatUploadStorageKey(1, "kyc", "selfie", ts, ".jpg")
	want := "kyc_1_selfie_2026_01_05_09:00:00.jpg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
