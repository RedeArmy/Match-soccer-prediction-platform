package email_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/email"
)

func TestResendClient_Send_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resend-msg-001"}`))
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("test-key", srv.URL)
	msgID, err := c.Send(context.Background(), email.Message{
		From:    "noreply@test.com",
		To:      []string{"admin@test.com"},
		Subject: "Test",
		HTML:    "<p>hello</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msgID != "resend-msg-001" {
		t.Errorf("msgID: got %q; want %q", msgID, "resend-msg-001")
	}
}

func TestResendClient_Send_201Created(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"resend-msg-002"}`))
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	msgID, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msgID != "resend-msg-002" {
		t.Errorf("msgID: got %q; want %q", msgID, "resend-msg-002")
	}
}

func TestResendClient_Send_APIError_WithMessage(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"name":"validation_error","message":"invalid from address","statusCode":422}`))
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	_, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error for 422 response; got nil")
	}
}

func TestResendClient_Send_5xx_NoBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	_, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error for 500 response; got nil")
	}
}

func TestResendClient_Send_InvalidResponseJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	_, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error for invalid JSON response; got nil")
	}
}

func TestResendClient_Send_HTTPFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close before sending so the request fails

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	_, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error when server is unreachable; got nil")
	}
}

func TestResendClient_Send_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the request is made

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	_, err := c.Send(ctx, email.Message{To: []string{"x@y.com"}, Subject: "s", HTML: "h"})
	if err == nil {
		t.Fatal("expected error for cancelled context; got nil")
	}
}

func TestResendClient_Send_RequestBodyContainsPayload(t *testing.T) {
	t.Parallel()

	type reqBody struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}

	var captured reqBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()

	c := email.NewResendClientWithBaseURL("key", srv.URL)
	msg := email.Message{
		From:    "from@test.com",
		To:      []string{"a@b.com"},
		Subject: "Hello",
		HTML:    "<b>hi</b>",
	}
	_, _ = c.Send(context.Background(), msg)

	if captured.From != msg.From {
		t.Errorf("From: got %q; want %q", captured.From, msg.From)
	}
	if len(captured.To) != 1 || captured.To[0] != msg.To[0] {
		t.Errorf("To: got %v; want %v", captured.To, msg.To)
	}
}

func TestNoopClient_Send_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	c := email.NoopClient{}
	msgID, err := c.Send(context.Background(), email.Message{To: []string{"x@y.com"}})
	if err != nil {
		t.Errorf("NoopClient.Send returned error: %v", err)
	}
	if msgID != "" {
		t.Errorf("NoopClient.Send msgID: got %q; want empty string", msgID)
	}
}

func TestNewResendClient_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	c := email.NewResendClient("test-api-key")
	if c == nil {
		t.Error("NewResendClient returned nil")
	}
}
