package repository

import "fmt"

// applyPagination appends LIMIT/OFFSET clauses to q and corresponding values to args.
// n is the next positional argument index (1-based) for the query being built.
func applyPagination(q string, args []any, n int, p Pagination) (string, []any, int) {
	if p.Limit > 0 {
		q += fmt.Sprintf(` LIMIT $%d`, n)
		args = append(args, p.Limit)
		n++
	}
	if p.Offset > 0 {
		q += fmt.Sprintf(` OFFSET $%d`, n)
		args = append(args, p.Offset)
		n++
	}
	return q, args, n
}
