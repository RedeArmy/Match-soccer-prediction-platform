// Command reencrypt-payouts finds withdrawal_requests rows whose
// payout_details column still holds legacy plaintext JSON (written before
// WCQ_PAYMENT_PAYOUTENCRYPTIONKEY was configured in production) and encrypts
// them in place using pkg/payoutenc, closing the gap documented in migration
// 000213_payout_details_text.up.sql ("a separate data migration is required
// to remediate those rows").
//
// Usage:
//
//	WCQ_DATABASE_DSN=... WCQ_PAYMENT_PAYOUTENCRYPTIONKEY=... go run ./cmd/reencrypt-payouts
//	WCQ_DATABASE_DSN=... WCQ_PAYMENT_PAYOUTENCRYPTIONKEY=... go run ./cmd/reencrypt-payouts --dry-run
//
// Safe to run repeatedly: rows already carrying the "_enc" envelope are
// excluded by the SQL filter, so a second run finds nothing left to do.
// Each row is re-encrypted and written back inside its own short transaction
// with an optimistic WHERE guard so a concurrent write (e.g. an admin
// approving the withdrawal mid-run) is not silently overwritten.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
)

// legacyRowsQuery selects rows whose payout_details is non-empty and does not
// already carry the encrypted envelope's "_enc" sentinel key.
const legacyRowsQuery = `
	SELECT id, payout_details
	  FROM withdrawal_requests
	 WHERE payout_details IS NOT NULL
	   AND payout_details::text <> ''
	   AND payout_details::text <> '{}'
	   AND payout_details::text NOT LIKE '%"_enc"%'
	 ORDER BY id
	 LIMIT $1
`

// batchSize bounds how many rows are fetched per SELECT. Re-run picks up
// wherever the previous run left off since remediated rows drop out of
// legacyRowsQuery on the next pass.
const batchSize = 500

// dbExecutor is the narrow pgxpool.Pool surface this command depends on,
// letting tests substitute a real pool without importing testcontainers here.
type dbExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// result summarises one remediation run for the operator-facing log line.
type result struct {
	migrated int
	failed   int
}

func main() {
	os.Exit(mainRun())
}

// mainRun contains the exit-code logic previously in main(). Extracted so
// every defer (cancel, pool.Close) always runs — main() calls os.Exit exactly
// once, after this function returns, instead of from inside it.
func mainRun() int {
	dryRun := flag.Bool("dry-run", false, "report legacy plaintext rows without modifying them")
	flag.Parse()

	dsn := os.Getenv("WCQ_DATABASE_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "reencrypt-payouts: WCQ_DATABASE_DSN is required")
		return 1
	}
	hexKey := os.Getenv("WCQ_PAYMENT_PAYOUTENCRYPTIONKEY")
	if hexKey == "" {
		fmt.Fprintln(os.Stderr, "reencrypt-payouts: WCQ_PAYMENT_PAYOUTENCRYPTIONKEY is required — "+
			"this must be the same key the API uses to encrypt payout_details, or the rows this "+
			"tool writes will be unreadable by the running application")
		return 1
	}
	enc, err := payoutenc.NewAESGCM(hexKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reencrypt-payouts: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reencrypt-payouts: connect: %v\n", err)
		return 1
	}
	defer pool.Close()

	res, err := run(ctx, pool, enc, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reencrypt-payouts: %v\n", err)
		return 1
	}
	verb := "migrated"
	if *dryRun {
		verb = "would migrate"
	}
	log.Printf("reencrypt-payouts: done — %s=%d failed=%d", verb, res.migrated, res.failed)
	if res.failed > 0 {
		return 1
	}
	return 0
}

// run repeatedly fetches up to batchSize legacy rows and remediates each one
// until a fetch returns nothing left to do. dryRun reports what would change
// without writing anything.
func run(ctx context.Context, db dbExecutor, enc payoutenc.Encrypter, dryRun bool) (result, error) {
	var res result
	for {
		rows, err := fetchBatch(ctx, db)
		if err != nil {
			return res, err
		}
		if len(rows) == 0 {
			return res, nil
		}
		for _, row := range rows {
			if err := remediateRow(ctx, db, enc, row, dryRun); err != nil {
				log.Printf("reencrypt-payouts: id=%d failed: %v", row.id, err)
				res.failed++
				continue
			}
			res.migrated++
		}
		// dry-run never shrinks the candidate set (nothing is written), so a
		// single batch is enough to report on — fetching again would loop forever.
		if dryRun {
			return res, nil
		}
	}
}

type legacyRow struct {
	id      int64
	details string
}

func fetchBatch(ctx context.Context, db dbExecutor) ([]legacyRow, error) {
	rows, err := db.Query(ctx, legacyRowsQuery, batchSize)
	if err != nil {
		return nil, fmt.Errorf("query legacy rows: %w", err)
	}
	defer rows.Close()

	var out []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.details); err != nil {
			return nil, fmt.Errorf("scan legacy row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy rows: %w", err)
	}
	return out, nil
}

// reencryptDetails decrypts (trivially, since the input is legacy plaintext)
// details and re-encodes it through payoutenc's encrypted envelope.
func reencryptDetails(enc payoutenc.Encrypter, details string) (string, error) {
	m, err := payoutenc.Unmarshal(enc, []byte(details))
	if err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	newDetails, err := payoutenc.Marshal(enc, m)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(newDetails), nil
}

// remediateRow re-encrypts row.details and writes it back guarded by an
// optimistic WHERE on the original value so a concurrent write to the same
// row is not clobbered — the row simply falls through to the next run instead.
func remediateRow(ctx context.Context, db dbExecutor, enc payoutenc.Encrypter, row legacyRow, dryRun bool) error {
	newDetails, err := reencryptDetails(enc, row.details)
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}

	tag, err := db.Exec(ctx,
		`UPDATE withdrawal_requests
		    SET payout_details = $1
		  WHERE id = $2 AND payout_details::text = $3`,
		newDetails, row.id, row.details,
	)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("row changed concurrently; skipped to avoid clobbering a newer write")
	}
	return nil
}
