// webhook-test is a development CLI that generates and sends a Svix-signed
// Recurrente webhook payload to a local server endpoint.
//
// Usage:
//
//	go run ./cmd/webhook-test [flags]
//
// Examples:
//
//	# payment_intent.succeeded with whsec_ secret (production-like):
//	go run ./cmd/webhook-test \
//	  --secret whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3Q= \
//	  --user-id 42 --amount 5000 --currency GTQ
//
//	# payment.confirmed (legacy format):
//	go run ./cmd/webhook-test \
//	  --event-type payment.confirmed \
//	  --secret my-raw-secret \
//	  --user-id 42 --amount 5000 --reference deposit-001
//
//	# Bypass mode (empty secret, dev server with WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET unset):
//	go run ./cmd/webhook-test --user-id 42 --amount 5000
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const eventPaymentIntentSucceeded = "payment_intent.succeeded"

func main() {
	if err := run(os.Args, http.DefaultClient, os.Stdout, os.Stderr); err != nil {
		os.Exit(1) // error already printed inside run
	}
}

// run is the testable entry point. It accepts the raw argument list, an HTTP
// client (inject httptest server client in tests), and separate writers for
// stdout and stderr so tests can capture or discard output.
func run(args []string, client *http.Client, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		rawURL     = fs.String("url", "http://localhost:8080/webhooks/recurrente", "Webhook endpoint URL")
		secret     = fs.String("secret", "", `Svix signing secret (whsec_<base64> or raw string; empty = no signing)`)
		userID     = fs.Int("user-id", 0, "Internal user ID to credit")
		amount     = fs.Int("amount", 5000, "Amount in cents")
		currency   = fs.String("currency", "GTQ", "Currency code")
		reference  = fs.String("reference", "", "Payment reference (auto-generated if empty)")
		checkoutID = fs.String("checkout-id", "", "Checkout ID (auto-generated if empty)")
		eventType  = fs.String("event-type", eventPaymentIntentSucceeded,
			"Event type: payment.confirmed | payment_intent.succeeded | intent.succeeded")
	)
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	ref := *reference
	if ref == "" {
		ref = fmt.Sprintf("test-ref-%d", time.Now().UnixNano())
	}
	chID := *checkoutID
	if chID == "" {
		chID = fmt.Sprintf("ch_test_%d", time.Now().UnixNano())
	}

	payload, err := buildPayload(*eventType, *userID, *amount, *currency, ref, chID)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return fmt.Errorf("build payload: %w", err)
	}

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error marshalling payload:", err)
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, *rawURL, bytes.NewReader(body)) //nolint:gosec // G704: SSRF — intentional in a developer-only CLI; the URL is a flag the operator supplies
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error creating request:", err)
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if *secret != "" {
		msgID := fmt.Sprintf("msg_test_%d", time.Now().UnixNano())
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		key, keyErr := parseSecret(*secret)
		if keyErr != nil {
			_, _ = fmt.Fprintln(stderr, "error parsing secret:", keyErr)
			return fmt.Errorf("parse secret: %w", keyErr)
		}
		sig := computeSig(key, msgID, timestamp, body)
		req.Header.Set("svix-id", msgID)
		req.Header.Set("svix-timestamp", timestamp)
		req.Header.Set("svix-signature", "v1,"+sig)
		fmt.Fprintf(stdout, "Svix headers:\n  svix-id:        %s\n  svix-timestamp: %s\n  svix-signature: v1,%s\n\n", msgID, timestamp, sig) //nolint:errcheck
	} else {
		_, _ = fmt.Fprintln(stdout, "Warning: no --secret provided; server must be in bypass mode (empty WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET)")
	}

	fmt.Fprintf(stdout, "POST %s\n%s\n\n", *rawURL, string(body)) //nolint:errcheck

	resp, err := client.Do(req) //nolint:gosec // G704: same justification as NewRequestWithContext above
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "request failed:", err)
		return fmt.Errorf("send request: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	fmt.Fprintf(stdout, "Response: %d\n%s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func buildPayload(eventType string, userID, amount int, currency, ref, chID string) (map[string]any, error) {
	switch eventType {
	case "payment.confirmed":
		if userID <= 0 {
			return nil, fmt.Errorf("--user-id is required for payment.confirmed")
		}
		return map[string]any{
			"event_type": "payment.confirmed",
			"data": map[string]any{
				"reference":    ref,
				"amount_cents": amount,
				"currency":     currency,
				"user_id":      userID,
			},
		}, nil

	case eventPaymentIntentSucceeded:
		return map[string]any{
			"id":              fmt.Sprintf("pa_test_%d", time.Now().UnixNano()),
			"event_type":      eventPaymentIntentSucceeded,
			"amount_in_cents": amount,
			"currency":        currency,
			"checkout": map[string]any{
				"id":     chID,
				"status": "paid",
				"metadata": map[string]any{
					"wcq_user_id":   userID,
					"wcq_reference": ref,
				},
			},
		}, nil

	case "intent.succeeded":
		return map[string]any{
			"id":              fmt.Sprintf("pi_test_%d", time.Now().UnixNano()),
			"event_type":      "intent.succeeded",
			"type":            "payment",
			"status":          "succeeded",
			"amount_in_cents": amount,
			"currency":        currency,
			"checkout": map[string]any{
				"id":     chID,
				"status": "paid",
				"metadata": map[string]any{
					"wcq_user_id":   userID,
					"wcq_reference": ref,
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown --event-type %q; valid: payment.confirmed | payment_intent.succeeded | intent.succeeded", eventType)
	}
}

// parseSecret returns the HMAC key bytes from a signing secret.
// "whsec_<base64>" → base64-decoded bytes; any other string → raw bytes.
func parseSecret(secret string) ([]byte, error) {
	if strings.HasPrefix(secret, "whsec_") {
		key, err := base64.StdEncoding.DecodeString(secret[6:])
		if err != nil {
			return nil, fmt.Errorf("decode whsec_ secret: %w", err)
		}
		return key, nil
	}
	return []byte(secret), nil
}

// computeSig returns the base64-encoded Svix HMAC-SHA256 signature.
func computeSig(key []byte, msgID, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID))
	mac.Write([]byte("."))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
