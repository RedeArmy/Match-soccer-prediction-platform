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

// ── FloatValue ────────────────────────────────────────────────────────────────

func makeSample(t *testing.T, raw string) *promclient.ScalarSample {
	t.Helper()
	var s promclient.ScalarSample
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("makeSample: unmarshal failed: %v", err)
	}
	return &s
}

func TestFloatValue_Valid(t *testing.T) {
	s := makeSample(t, `{"metric":{},"value":[1234567890,"2.718"]}`)
	v, err := promclient.FloatValue(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 2.718 {
		t.Errorf("expected 2.718, got %v", v)
	}
}

func TestFloatValue_NilElements(t *testing.T) {
	// Zero-value [2]json.RawMessage: both elements are nil — json.Unmarshal(nil) errors.
	var s promclient.ScalarSample
	_, err := promclient.FloatValue(&s)
	if err == nil {
		t.Error("expected error when Value elements are nil JSON")
	}
}

func TestFloatValue_BadValueJSON(t *testing.T) {
	s := makeSample(t, `{"metric":{},"value":[1234567890,{"bad":"json"}]}`)
	_, err := promclient.FloatValue(s)
	if err == nil {
		t.Error("expected error when value[1] is not a JSON string")
	}
}

func TestFloatValue_NonFloatString(t *testing.T) {
	// "abc" is a valid JSON string but fmt.Sscanf cannot parse it as %f.
	s := makeSample(t, `{"metric":{},"value":[1234567890,"abc"]}`)
	_, err := promclient.FloatValue(s)
	if err == nil {
		t.Error("expected error when value string is not parseable as float")
	}
}

// ── AllSamples ────────────────────────────────────────────────────────────────

func TestAllSamples_Multiple(t *testing.T) {
	qr := &promclient.QueryResponse{Status: "success"}
	qr.Data.Result = []json.RawMessage{
		json.RawMessage(`{"metric":{"label":"a"},"value":[1,"1.1"]}`),
		json.RawMessage(`{"metric":{"label":"b"},"value":[2,"2.2"]}`),
	}
	samples, err := promclient.AllSamples(qr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[0].Labels["label"] != "a" {
		t.Errorf("unexpected label on sample 0: %v", samples[0].Labels)
	}
}

func TestAllSamples_Empty(t *testing.T) {
	qr := &promclient.QueryResponse{Status: "success"}
	samples, err := promclient.AllSamples(qr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected 0 samples, got %d", len(samples))
	}
}

func TestAllSamples_InvalidJSON(t *testing.T) {
	qr := &promclient.QueryResponse{}
	qr.Data.Result = []json.RawMessage{json.RawMessage("not-valid-json")}
	_, err := promclient.AllSamples(qr)
	if err == nil {
		t.Error("expected error on malformed sample JSON")
	}
}
