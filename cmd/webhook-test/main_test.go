package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// ── buildPayload ──────────────────────────────────────────────────────────────

func TestBuildPayload_PaymentConfirmed_ReturnsCorrectShape(t *testing.T) {
	payload, err := buildPayload("payment.confirmed", 1, 5000, "GTQ", "ref-001", "ch-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["event_type"] != "payment.confirmed" {
		t.Errorf("event_type = %v, want payment.confirmed", payload["event_type"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field is missing or has wrong type")
	}
	if data["user_id"] != 1 {
		t.Errorf("user_id = %v, want 1", data["user_id"])
	}
	if data["amount_cents"] != 5000 {
		t.Errorf("amount_cents = %v, want 5000", data["amount_cents"])
	}
	if data["reference"] != "ref-001" {
		t.Errorf("reference = %v, want ref-001", data["reference"])
	}
}

func TestBuildPayload_PaymentConfirmed_ZeroUserID_ReturnsError(t *testing.T) {
	_, err := buildPayload("payment.confirmed", 0, 5000, "GTQ", "ref", "ch")
	if err == nil {
		t.Fatal("expected error when user-id is 0 for payment.confirmed")
	}
}

func TestBuildPayload_PaymentIntentSucceeded_ReturnsCorrectShape(t *testing.T) {
	payload, err := buildPayload("payment_intent.succeeded", 5, 10000, "USD", "ref-002", "ch-002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["event_type"] != "payment_intent.succeeded" {
		t.Errorf("event_type = %v, want payment_intent.succeeded", payload["event_type"])
	}
	if payload["amount_in_cents"] != 10000 {
		t.Errorf("amount_in_cents = %v, want 10000", payload["amount_in_cents"])
	}
	checkout, ok := payload["checkout"].(map[string]any)
	if !ok {
		t.Fatal("checkout field is missing or has wrong type")
	}
	meta, ok := checkout["metadata"].(map[string]any)
	if !ok {
		t.Fatal("checkout.metadata is missing or has wrong type")
	}
	if meta["wcq_user_id"] != 5 {
		t.Errorf("wcq_user_id = %v, want 5", meta["wcq_user_id"])
	}
}

func TestBuildPayload_IntentSucceeded_ReturnsCorrectShape(t *testing.T) {
	payload, err := buildPayload("intent.succeeded", 3, 7500, "GTQ", "ref-003", "ch-003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["type"] != "payment" {
		t.Errorf("type = %v, want payment", payload["type"])
	}
	if payload["status"] != "succeeded" {
		t.Errorf("status = %v, want succeeded", payload["status"])
	}
	if payload["amount_in_cents"] != 7500 {
		t.Errorf("amount_in_cents = %v, want 7500", payload["amount_in_cents"])
	}
}

func TestBuildPayload_UnknownEventType_ReturnsError(t *testing.T) {
	_, err := buildPayload("unknown.event", 1, 5000, "GTQ", "ref", "ch")
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention 'unknown', got: %v", err)
	}
}

// ── parseSecret ───────────────────────────────────────────────────────────────

func TestParseSecret_RawString_ReturnsBytesDirectly(t *testing.T) {
	key, err := parseSecret("my-raw-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(key) != "my-raw-secret" {
		t.Errorf("key = %q, want my-raw-secret", key)
	}
}

func TestParseSecret_WhsecFormat_DecodesBase64(t *testing.T) {
	rawKey := []byte("test-key-bytes!!")
	encoded := "whsec_" + base64.StdEncoding.EncodeToString(rawKey)
	key, err := parseSecret(encoded)
	if err != nil {
		t.Fatalf("unexpected error for valid whsec_: %v", err)
	}
	if string(key) != string(rawKey) {
		t.Errorf("key = %q, want %q", key, rawKey)
	}
}

func TestParseSecret_WhsecFormat_InvalidBase64_ReturnsError(t *testing.T) {
	_, err := parseSecret("whsec_!!!notbase64")
	if err == nil {
		t.Fatal("expected error for invalid whsec_ base64 payload")
	}
}

// ── computeSig ────────────────────────────────────────────────────────────────

func TestComputeSig_MatchesManualHMACSHA256(t *testing.T) {
	key := []byte("test-signing-key")
	msgID := "msg_001"
	timestamp := "1700000000"
	body := []byte(`{"event_type":"payment_intent.succeeded"}`)

	got := computeSig(key, msgID, timestamp, body)

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID + "." + timestamp + "."))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Errorf("computeSig = %q, want %q", got, want)
	}
}
