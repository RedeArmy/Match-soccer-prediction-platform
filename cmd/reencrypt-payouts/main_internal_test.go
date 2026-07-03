package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRows is a minimal in-memory pgx.Rows backed by a []legacyRow, letting
// fetchBatch/run be unit-tested without a real Postgres connection. pgx.Rows
// is documented as "an interface instead of a struct to allow tests to mock
// Query" — this is exactly that. Only Next/Scan/Close/Err do anything
// meaningful; fetchBatch never calls the rest.
type fakeRows struct {
	rows    []legacyRow
	pos     int
	scanErr error
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeRows) Next() bool {
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.pos-1]
	idPtr, ok := dest[0].(*int64)
	if !ok {
		return errors.New("fakeRows: dest[0] must be *int64")
	}
	detailsPtr, ok := dest[1].(*string)
	if !ok {
		return errors.New("fakeRows: dest[1] must be *string")
	}
	*idPtr = row.id
	*detailsPtr = row.details
	return nil
}

// queryResponse is one queued response for fakeExecutor.Query, allowing a
// test to simulate run's multi-batch loop (e.g. rows, then empty to stop).
type queryResponse struct {
	rows []legacyRow
	err  error
}

// fakeExecutor implements dbExecutor purely in memory.
type fakeExecutor struct {
	queryResponses []queryResponse // consumed in order; last one repeats once exhausted
	queryCalls     int

	execTag   pgconn.CommandTag
	execErr   error
	execCalls []execCall
}

type execCall struct {
	sql  string
	args []any
}

func (f *fakeExecutor) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	i := f.queryCalls
	if i >= len(f.queryResponses) {
		i = len(f.queryResponses) - 1
	}
	f.queryCalls++
	resp := f.queryResponses[i]
	if resp.err != nil {
		return nil, resp.err
	}
	return &fakeRows{rows: resp.rows}, nil
}

func (f *fakeExecutor) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execCalls = append(f.execCalls, execCall{sql: sql, args: args})
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return f.execTag, nil
}

func legacyPlaintext() string {
	return `{"account_number":"1234567890","bank_name":"Banrural"}`
}

// ── fetchBatch ────────────────────────────────────────────────────────────────

func TestFetchBatch_ReturnsScannedRows(t *testing.T) {
	db := &fakeExecutor{queryResponses: []queryResponse{
		{rows: []legacyRow{{id: 1, details: legacyPlaintext()}, {id: 2, details: "{}"}}},
	}}
	rows, err := fetchBatch(context.Background(), db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 || rows[0].id != 1 || rows[1].id != 2 {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestFetchBatch_QueryError_IsWrapped(t *testing.T) {
	wantErr := errors.New("connection reset")
	db := &fakeExecutor{queryResponses: []queryResponse{{err: wantErr}}}
	_, err := fetchBatch(context.Background(), db)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
}

func TestFetchBatch_ScanError_IsWrapped(t *testing.T) {
	// fakeRows.Scan returns an error for every row when dest types don't match;
	// force that by giving it one row and manually injecting a scanErr via a
	// custom fakeExecutor.Query override is unnecessary — reuse the built-in
	// mismatched-dest-type path by wrapping fetchBatch's own row shape.
	db := &fakeExecutorScanFailure{}
	_, err := fetchBatch(context.Background(), db)
	if err == nil {
		t.Fatal("expected scan error to be wrapped and returned")
	}
}

// fakeExecutorScanFailure returns a single fakeRows configured to fail Scan.
type fakeExecutorScanFailure struct{}

func (fakeExecutorScanFailure) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &fakeRows{rows: []legacyRow{{id: 1, details: "x"}}, scanErr: errors.New("scan boom")}, nil
}
func (fakeExecutorScanFailure) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func TestFetchBatch_EmptyResult_ReturnsNilSlice(t *testing.T) {
	db := &fakeExecutor{queryResponses: []queryResponse{{rows: nil}}}
	rows, err := fetchBatch(context.Background(), db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows, got %d", len(rows))
	}
}

// ── remediateRow ──────────────────────────────────────────────────────────────

func TestRemediateRow_Success_ExecutesUpdate(t *testing.T) {
	db := &fakeExecutor{execTag: pgconn.NewCommandTag("UPDATE 1")}
	enc := testEncrypter(t)
	row := legacyRow{id: 7, details: legacyPlaintext()}

	if err := remediateRow(context.Background(), db, enc, row, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db.execCalls) != 1 {
		t.Fatalf("expected exactly 1 Exec call, got %d", len(db.execCalls))
	}
	call := db.execCalls[0]
	if call.args[1].(int64) != 7 || call.args[2].(string) != legacyPlaintext() {
		t.Errorf("unexpected exec args: %+v", call.args)
	}
}

func TestRemediateRow_DryRun_SkipsExec(t *testing.T) {
	db := &fakeExecutor{execTag: pgconn.NewCommandTag("UPDATE 1")}
	enc := testEncrypter(t)
	row := legacyRow{id: 1, details: legacyPlaintext()}

	if err := remediateRow(context.Background(), db, enc, row, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db.execCalls) != 0 {
		t.Errorf("dry-run must not write anything, got %d Exec calls", len(db.execCalls))
	}
}

func TestRemediateRow_ReencryptError_NeverCallsExec(t *testing.T) {
	db := &fakeExecutor{}
	enc := testEncrypter(t)
	row := legacyRow{id: 1, details: "not-json"}

	if err := remediateRow(context.Background(), db, enc, row, false); err == nil {
		t.Fatal("expected error from invalid legacy JSON")
	}
	if len(db.execCalls) != 0 {
		t.Errorf("must not attempt to write when re-encryption fails, got %d Exec calls", len(db.execCalls))
	}
}

func TestRemediateRow_ConcurrentWrite_ReturnsError(t *testing.T) {
	// RowsAffected()==0 means the optimistic WHERE guard did not match —
	// someone else changed the row between fetch and write.
	db := &fakeExecutor{execTag: pgconn.NewCommandTag("UPDATE 0")}
	enc := testEncrypter(t)
	row := legacyRow{id: 1, details: legacyPlaintext()}

	err := remediateRow(context.Background(), db, enc, row, false)
	if err == nil {
		t.Fatal("expected error when the row changed concurrently")
	}
}

func TestRemediateRow_ExecError_IsWrapped(t *testing.T) {
	wantErr := errors.New("write timeout")
	db := &fakeExecutor{execErr: wantErr}
	enc := testEncrypter(t)
	row := legacyRow{id: 1, details: legacyPlaintext()}

	err := remediateRow(context.Background(), db, enc, row, false)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
}

// ── run ───────────────────────────────────────────────────────────────────────

func TestRun_SingleBatchThenEmpty_MigratesAllRows(t *testing.T) {
	db := &fakeExecutor{
		execTag: pgconn.NewCommandTag("UPDATE 1"),
		queryResponses: []queryResponse{
			{rows: []legacyRow{{id: 1, details: legacyPlaintext()}, {id: 2, details: `{"paypal_email":"a@b.com"}`}}},
			{rows: nil}, // second fetch finds nothing left — loop exits
		},
	}
	enc := testEncrypter(t)

	res, err := run(context.Background(), db, enc, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.migrated != 2 || res.failed != 0 {
		t.Fatalf("expected migrated=2 failed=0, got %+v", res)
	}
	if db.queryCalls != 2 {
		t.Errorf("expected run to re-query after a non-empty batch, got %d Query calls", db.queryCalls)
	}
}

func TestRun_DryRun_StopsAfterFirstBatch(t *testing.T) {
	db := &fakeExecutor{
		queryResponses: []queryResponse{
			{rows: []legacyRow{{id: 1, details: legacyPlaintext()}}},
		},
	}
	enc := testEncrypter(t)

	res, err := run(context.Background(), db, enc, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.migrated != 1 {
		t.Fatalf("dry-run should still report the row as migratable, got %+v", res)
	}
	if db.queryCalls != 1 {
		t.Errorf("dry-run must not loop for a second batch, got %d Query calls", db.queryCalls)
	}
	if len(db.execCalls) != 0 {
		t.Error("dry-run must not write anything")
	}
}

func TestRun_FetchBatchError_ReturnsImmediately(t *testing.T) {
	wantErr := errors.New("db down")
	db := &fakeExecutor{queryResponses: []queryResponse{{err: wantErr}}}
	enc := testEncrypter(t)

	_, err := run(context.Background(), db, enc, false)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
}

func TestRun_OneRowFailsRemediation_ContinuesAndCountsFailure(t *testing.T) {
	db := &fakeExecutor{
		execTag: pgconn.NewCommandTag("UPDATE 1"),
		queryResponses: []queryResponse{
			{rows: []legacyRow{
				{id: 1, details: "not-json"}, // fails reencryptDetails
				{id: 2, details: legacyPlaintext()},
			}},
			{rows: nil},
		},
	}
	enc := testEncrypter(t)

	res, err := run(context.Background(), db, enc, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.migrated != 1 || res.failed != 1 {
		t.Fatalf("expected migrated=1 failed=1, got %+v", res)
	}
}

func TestRun_NoLegacyRows_ReturnsZeroResult(t *testing.T) {
	db := &fakeExecutor{queryResponses: []queryResponse{{rows: nil}}}
	enc := testEncrypter(t)

	res, err := run(context.Background(), db, enc, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.migrated != 0 || res.failed != 0 {
		t.Errorf("expected zero result, got %+v", res)
	}
}

// ── mainRun (pure validation branches — no DB connection reached) ────────────

// resetFlags gives mainRun a fresh, empty flag.CommandLine so it can register
// "-dry-run" again without panicking on "flag redefined" across subtests.
func resetFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = []string{os.Args[0]}
}

func TestMainRun_MissingDSN_Returns1(t *testing.T) {
	resetFlags()
	t.Setenv("WCQ_DATABASE_DSN", "")
	t.Setenv("WCQ_PAYMENT_PAYOUTENCRYPTIONKEY", testKeyHex)

	if code := mainRun(); code != 1 {
		t.Errorf("expected exit code 1 when WCQ_DATABASE_DSN is unset, got %d", code)
	}
}

func TestMainRun_MissingEncryptionKey_Returns1(t *testing.T) {
	resetFlags()
	t.Setenv("WCQ_DATABASE_DSN", "postgres://user:pass@127.0.0.1:5432/db")
	t.Setenv("WCQ_PAYMENT_PAYOUTENCRYPTIONKEY", "")

	if code := mainRun(); code != 1 {
		t.Errorf("expected exit code 1 when WCQ_PAYMENT_PAYOUTENCRYPTIONKEY is unset, got %d", code)
	}
}

func TestMainRun_InvalidEncryptionKey_Returns1(t *testing.T) {
	resetFlags()
	t.Setenv("WCQ_DATABASE_DSN", "postgres://user:pass@127.0.0.1:5432/db")
	t.Setenv("WCQ_PAYMENT_PAYOUTENCRYPTIONKEY", "not-valid-hex")

	if code := mainRun(); code != 1 {
		t.Errorf("expected exit code 1 for a malformed encryption key, got %d", code)
	}
}

func TestMainRun_MalformedDSN_Returns1(t *testing.T) {
	resetFlags()
	// A DSN pgxpool.New itself rejects at parse time (before any network I/O),
	// so this exercises the "connect:" error branch deterministically and fast.
	t.Setenv("WCQ_DATABASE_DSN", "not a valid dsn ::")
	t.Setenv("WCQ_PAYMENT_PAYOUTENCRYPTIONKEY", testKeyHex)

	if code := mainRun(); code != 1 {
		t.Errorf("expected exit code 1 for a DSN pgxpool.New cannot parse, got %d", code)
	}
}
