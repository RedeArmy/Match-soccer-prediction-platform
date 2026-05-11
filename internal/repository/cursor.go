package repository

import (
	"encoding/base64"
	"strconv"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// encodeCursor encodes a row's integer ID into an opaque, URL-safe pagination
// cursor token: base64url(decimal(id)).
//
// The cursor is intentionally opaque: callers receive it as a string from the
// previous response and pass it back unchanged. Treating the raw value as an
// ID is a leaky abstraction that prevents future cursor format evolution.
func encodeCursor(id int) string {
	return base64.URLEncoding.EncodeToString([]byte(strconv.Itoa(id)))
}

// decodeCursor reverses encodeCursor. Returns Validation when the token is
// malformed or encodes a non-positive ID.
func decodeCursor(token string) (int, error) {
	b, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, apperrors.Validation("invalid pagination cursor")
	}
	id, err := strconv.Atoi(string(b))
	if err != nil || id <= 0 {
		return 0, apperrors.Validation("invalid pagination cursor")
	}
	return id, nil
}
