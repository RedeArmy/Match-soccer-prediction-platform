package handler

import (
	"net/http"
	"net/url"
	"testing"

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
