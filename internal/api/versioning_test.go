package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var noopHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// ── VersionHeader ──────────────────────────────────────────────────────────

func TestVersionHeader_SetsHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	VersionHeader("v1")(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("API-Version"); got != "v1" {
		t.Errorf("API-Version header: want %q, got %q", "v1", got)
	}
}

// TestVersionHeader_PropagatesHandlerStatus verifies the middleware does not
// alter the handler's status code.
func TestVersionHeader_PropagatesHandlerStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := httptest.NewRecorder()
	VersionHeader("v1")(handler).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusCreated {
		t.Errorf("VersionHeader must not alter status code; got %d, want 201", rec.Code)
	}
}

// TestVersionHeader_IndependentVersionStrings verifies the header value
// matches the string passed at construction time, supporting future v2, v3.
func TestVersionHeader_IndependentVersionStrings(t *testing.T) {
	for _, version := range []string{"v1", "v2", "v3"} {
		rec := httptest.NewRecorder()
		VersionHeader(version)(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if got := rec.Header().Get("API-Version"); got != version {
			t.Errorf("version %q: API-Version header: want %q, got %q", version, version, got)
		}
	}
}

// ── Deprecated ───────────────────────────────────────────────────────────────

func TestDeprecated_SetsDeprecationHeader(t *testing.T) {
	sunset := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	Deprecated(sunset, "")(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("Deprecation header: want %q, got %q", "true", got)
	}
}

func TestDeprecated_SetsSunsetHeader_RFC1123Format(t *testing.T) {
	sunset := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	Deprecated(sunset, "")(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := sunset.UTC().Format(http.TimeFormat) // RFC 1123 / HTTP-date
	if got := rec.Header().Get("Sunset"); got != want {
		t.Errorf("Sunset header: want %q, got %q", want, got)
	}
}

// TestDeprecated_NormalisesNonUTCZoneToUTC ensures the Sunset header value is
// always in UTC (HTTP-date is defined as GMT per RFC 7231 §7.1.1.1), regardless
// of the timezone of the time.Time passed by the caller.
func TestDeprecated_NormalisesNonUTCZoneToUTC(t *testing.T) {
	loc := time.FixedZone("UTC-6", -6*60*60)
	sunset := time.Date(2026, 9, 1, 0, 0, 0, 0, loc) // 00:00 UTC-6 == 06:00 UTC
	rec := httptest.NewRecorder()
	Deprecated(sunset, "")(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := sunset.UTC().Format(http.TimeFormat)
	if got := rec.Header().Get("Sunset"); got != want {
		t.Errorf("Sunset header must be UTC; want %q, got %q", want, got)
	}
}

// TestDeprecated_DoesNotRejectRequests verifies that the middleware is purely
// advisory: it must not alter the status code or reject the request.
func TestDeprecated_DoesNotRejectRequests(t *testing.T) {
	h := Deprecated(time.Now().Add(24*time.Hour), "")(noopHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("Deprecated middleware must not reject requests; got status %d, want 200", rec.Code)
	}
}

// TestDeprecated_HeadersPresentOnEveryMethod confirms the headers are injected
// for all HTTP methods, not just GET.
func TestDeprecated_HeadersPresentOnEveryMethod(t *testing.T) {
	sunset := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	h := Deprecated(sunset, "")(noopHandler)
	wantSunset := sunset.UTC().Format(http.TimeFormat)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/", nil))

		if got := rec.Header().Get("Deprecation"); got != "true" {
			t.Errorf("%s: Deprecation header missing or wrong: %q", method, got)
		}
		if got := rec.Header().Get("Sunset"); got != wantSunset {
			t.Errorf("%s: Sunset header: want %q, got %q", method, wantSunset, got)
		}
	}
}

// TestDeprecated_LinkHeaderPresentWhenSuccessorProvided verifies that a non-empty
// successorURL is emitted as an RFC 8594 Link header pointing to the replacement.
func TestDeprecated_LinkHeaderPresentWhenSuccessorProvided(t *testing.T) {
	sunset := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	Deprecated(sunset, "/api/v2/resource")(noopHandler).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := `</api/v2/resource>; rel="successor-version"`
	if got := rec.Header().Get("Link"); got != want {
		t.Errorf("Link header: want %q, got %q", want, got)
	}
}

// TestDeprecated_NoLinkHeaderWhenSuccessorEmpty verifies that an empty
// successorURL suppresses the Link header entirely.
func TestDeprecated_NoLinkHeaderWhenSuccessorEmpty(t *testing.T) {
	sunset := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	Deprecated(sunset, "")(noopHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Link"); got != "" {
		t.Errorf("Link header must be absent when successorURL is empty; got %q", got)
	}
}
