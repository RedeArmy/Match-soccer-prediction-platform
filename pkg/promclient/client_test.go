package promclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/promclient"
)

func makeQueryServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

// ── Query ─────────────────────────────────────────────────────────────────────

func TestQuery_Success(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1234567890,"42.5"]}]}}`
	srv := makeQueryServer(t, http.StatusOK, body)
	defer srv.Close()

	c := promclient.New(srv.URL)
	qr, err := c.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qr.Status != "success" {
		t.Errorf("expected status success, got %s", qr.Status)
	}
	if len(qr.Data.Result) != 1 {
		t.Errorf("expected 1 result, got %d", len(qr.Data.Result))
	}
}

func TestQuery_ConnectionRefused(t *testing.T) {
	c := promclient.New("http://127.0.0.1:1")
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Error("expected error when Prometheus is unreachable")
	}
}

func TestQuery_NonOKStatus(t *testing.T) {
	srv := makeQueryServer(t, http.StatusServiceUnavailable, "")
	defer srv.Close()

	c := promclient.New(srv.URL)
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Error("expected error on non-200 status from Prometheus")
	}
}

func TestQuery_InvalidJSON(t *testing.T) {
	srv := makeQueryServer(t, http.StatusOK, "not-valid-json")
	defer srv.Close()

	c := promclient.New(srv.URL)
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Error("expected error on invalid JSON response")
	}
}

func TestQuery_ErrorStatus(t *testing.T) {
	body := `{"status":"error","error":"unknown metric name"}`
	srv := makeQueryServer(t, http.StatusOK, body)
	defer srv.Close()

	c := promclient.New(srv.URL)
	_, err := c.Query(context.Background(), "bad_metric{")
	if err == nil {
		t.Error("expected error when Prometheus returns error status")
	}
}

func TestQuery_InvalidBaseURL(t *testing.T) {
	c := promclient.New("://invalid-scheme")
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Error("expected error for invalid base URL")
	}
}

// ── FirstFloat ────────────────────────────────────────────────────────────────

func TestFirstFloat_EmptyResult(t *testing.T) {
	qr := &promclient.QueryResponse{}
	v, err := promclient.FirstFloat(qr)
	if err != nil {
		t.Fatalf("unexpected error for empty result: %v", err)
	}
	if v != 0 {
		t.Errorf("expected 0 for empty result, got %v", v)
	}
}

func TestFirstFloat_ValidSample(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1234567890,"3.14"]}]}}`
	srv := makeQueryServer(t, http.StatusOK, body)
	defer srv.Close()

	c := promclient.New(srv.URL)
	qr, err := c.Query(context.Background(), "some_metric")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	v, err := promclient.FirstFloat(qr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 3.14 {
		t.Errorf("expected 3.14, got %v", v)
	}
}

func TestFirstFloat_BadSampleJSON(t *testing.T) {
	qr := &promclient.QueryResponse{}
	qr.Data.Result = []json.RawMessage{json.RawMessage("not-valid-json")}
	_, err := promclient.FirstFloat(qr)
	if err == nil {
		t.Error("expected error when sample JSON is malformed")
	}
}

func TestFirstFloat_BadValueString(t *testing.T) {
	// value[1] is a JSON object — cannot be unmarshalled into a Go string.
	sample := `{"metric":{},"value":[1234567890,{"nested":"object"}]}`
	qr := &promclient.QueryResponse{}
	qr.Data.Result = []json.RawMessage{json.RawMessage(sample)}
	_, err := promclient.FirstFloat(qr)
	if err == nil {
		t.Error("expected error when value[1] is not a JSON string")
	}
}

func TestFirstFloat_NonFloatValue(t *testing.T) {
	// value[1] is a valid JSON string but not parseable as float.
	sample := `{"metric":{},"value":[1234567890,"not-a-number"]}`
	qr := &promclient.QueryResponse{}
	qr.Data.Result = []json.RawMessage{json.RawMessage(sample)}
	_, err := promclient.FirstFloat(qr)
	if err == nil {
		t.Error("expected error when value string is not a float")
	}
}
