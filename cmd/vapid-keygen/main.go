// Command vapid-keygen generates a VAPID key pair for Web Push (RFC 8292).
//
// Usage:
//
//	go run ./cmd/vapid-keygen
//
// Output is printed to stdout as shell variable assignments ready to copy
// into .env or a secrets manager.  Run once per deployment; rotate keys
// with a planned migration (all browser subscriptions must be re-registered).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/rede/world-cup-quiniela/pkg/vapid"
)

func main() {
	keys, err := vapid.GenerateKeys()
	if err != nil {
		log.Fatalf("vapid-keygen: %v", err)
	}
	fmt.Fprintf(os.Stdout, "WCQ_NOTIFY_VAPID_PUBLIC_KEY=%s\n", keys.PublicKey)
	fmt.Fprintf(os.Stdout, "WCQ_NOTIFY_VAPID_PRIVATE_KEY=%s\n", keys.PrivateKey)
	fmt.Fprintf(os.Stdout, "WCQ_NOTIFY_VAPID_SUBJECT=mailto:ops@example.com\n")
}
