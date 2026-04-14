package repository

// activeOnly is the SQL fragment appended after an existing WHERE condition
// to exclude soft-deleted rows from any query result. Tables that support
// soft deletion (users, quinielas) carry a nullable deleted_at column; a
// non-NULL value marks the row as logically deleted.
//
// Correct usage — appended after an existing predicate:
//
//	`SELECT ` + cols + ` FROM users WHERE id=$1` + activeOnly
//
// Every repository method that reads from a soft-deletable table must include
// this constant. The companion activeFilter constant is provided for queries
// where deleted_at IS NULL is the only WHERE predicate.
const activeOnly = " AND deleted_at IS NULL"

// activeFilter is the standalone form used when deleted_at IS NULL is the
// entire WHERE clause (i.e. there is no other predicate before it).
//
// Example:
//
//	`SELECT ` + cols + ` FROM users WHERE ` + activeFilter + ` ORDER BY id`
const activeFilter = "deleted_at IS NULL"
