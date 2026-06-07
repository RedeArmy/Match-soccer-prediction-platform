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

func main() {
	var (
		url        = flag.String("url", "http://localhost:8080/webhooks/recurrente", "Webhook endpoint URL")
		secret     = flag.String("secret", "", `Svix signing secret (whsec_<base64> or raw string; empty = no signing)`)
		userID     = flag.Int("user-id", 0, "Internal user ID to credit")
		amount     = flag.Int("amount", 5000, "Amount in cents")
		currency   = flag.String("currency", "GTQ", "Currency code")
		reference  = flag.String("reference", "", "Payment reference (auto-generated if empty)")
		checkoutID = flag.String("checkout-id", "", "Checkout ID (auto-generated if empty)")
		eventType  = flag.String("event-type", "payment_intent.succeeded",
			"Event type: payment.confirmed | payment_intent.succeeded | intent.succeeded")
	)
	flag.Parse()

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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error marshalling payload:", err)
		os.Exit(1)
	}

	req, err := http.NewRequest(http.MethodPost, *url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating request:", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	if *secret != "" {
		msgID := fmt.Sprintf("msg_test_%d", time.Now().UnixNano())
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		key, keyErr := parseSecret(*secret)
		if keyErr != nil {
			fmt.Fprintln(os.Stderr, "error parsing secret:", keyErr)
			os.Exit(1)
		}
		sig := computeSig(key, msgID, timestamp, body)
		req.Header.Set("svix-id", msgID)
		req.Header.Set("svix-timestamp", timestamp)
		req.Header.Set("svix-signature", "v1,"+sig)
		fmt.Printf("Svix headers:\n  svix-id:        %s\n  svix-timestamp: %s\n  svix-signature: v1,%s\n\n", msgID, timestamp, sig)
	} else {
		fmt.Println("Warning: no --secret provided; server must be in bypass mode (empty WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET)")
	}

	fmt.Printf("POST %s\n%s\n\n", *url, string(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response: %d\n%s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
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

	case "payment_intent.succeeded":
		return map[string]any{
			"id":              fmt.Sprintf("pa_test_%d", time.Now().UnixNano()),
			"event_type":      "payment_intent.succeeded",
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
		return base64.StdEncoding.DecodeString(secret[6:])
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
