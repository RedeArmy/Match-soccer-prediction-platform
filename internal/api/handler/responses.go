package handler

import "github.com/rede/world-cup-quiniela/internal/middleware"

// ErrorResponse is the standard error envelope returned on all 4xx/5xx responses.
// Defined once in middleware; aliased here so Swagger annotations can reference
// handler.ErrorResponse without import cycles.
type ErrorResponse = middleware.ErrorResponse

// ErrorDetail carries the machine-readable code and human-readable message.
type ErrorDetail = middleware.ErrorDetail

const timeFormat = "2006-01-02T15:04:05Z07:00"

// Paged wraps a paginated list with page metadata.
type Paged[T any] struct {
	Data []T      `json:"data"`
	Page PageMeta `json:"page"`
}

// PageMeta describes the current page of a paginated response.
type PageMeta struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// CursorPaged wraps a cursor-paginated list with navigation metadata.
// NextCursor is an opaque token that clients pass as ?cursor= on the next
// request. It is absent when HasMore is false (the caller is on the last page).
type CursorPaged[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}
