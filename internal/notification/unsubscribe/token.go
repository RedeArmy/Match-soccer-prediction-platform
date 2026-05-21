// Package unsubscribe provides HMAC-SHA256 signed tokens used to build
// one-click email unsubscribe links (CAN-SPAM / GDPR compliance).
//
// Token format (URL-safe, no external dependency):
//
//	<userID>.<expUnix>.<hex-HMAC-SHA256>
//
// The payload "<userID>.<expUnix>" is signed with HMAC-SHA256 using a
// shared secret (WCQ_EMAIL_UNSUBSCRIBESECRET).  Tokens are valid for
// TokenTTL (30 days by default).  Constant-time comparison prevents
// timing-side-channel attacks during verification.
package unsubscribe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TokenTTL is the lifetime of an unsubscribe token.
const TokenTTL = 30 * 24 * time.Hour

var (
	errMalformed = errors.New("unsubscribe: malformed token")
	errExpired   = errors.New("unsubscribe: token expired")
	errSignature = errors.New("unsubscribe: invalid signature")
	errUserID    = errors.New("unsubscribe: invalid user id")
)

// SignToken returns a signed unsubscribe token for userID valid for TokenTTL.
// The caller must ensure secret is non-empty before calling; an empty secret
// produces tokens that are trivially forgeable.
func SignToken(userID int, secret string, now time.Time) string {
	exp := now.Add(TokenTTL).Unix()
	msg := fmt.Sprintf("%d.%d", userID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return msg + "." + hex.EncodeToString(mac.Sum(nil))
}

// VerifyToken parses and cryptographically verifies tok, returning the
// embedded user ID.  Returns a non-nil error when the token is malformed,
// expired, or carries an invalid signature.
func VerifyToken(tok, secret string) (int, error) {
	parts := strings.SplitN(tok, ".", 3)
	if len(parts) != 3 {
		return 0, errMalformed
	}

	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	expected := mac.Sum(nil)

	got, err := hex.DecodeString(parts[2])
	if err != nil {
		return 0, errMalformed
	}
	if !hmac.Equal(expected, got) {
		return 0, errSignature
	}

	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errMalformed
	}
	if time.Unix(exp, 0).Before(time.Now()) {
		return 0, errExpired
	}

	userID, err := strconv.Atoi(parts[0])
	if err != nil || userID <= 0 {
		return 0, errUserID
	}
	return userID, nil
}
