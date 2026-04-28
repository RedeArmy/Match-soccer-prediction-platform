package repository

// group_membership_repository_internal_test.go covers unexported helpers that
// cannot be reached from the external test package (package repository_test).

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// errExecTx is a pgx.Tx stub whose Exec always returns an error.
// syncStatusInTx only calls tx.Exec, so all other methods panic — they are
// never reached and would signal a logic bug if they were.
type errExecTx struct{}

func (errExecTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("injected exec error")
}

func (errExecTx) Begin(_ context.Context) (pgx.Tx, error) { panic("unexpected call: Begin") }
func (errExecTx) Commit(_ context.Context) error          { panic("unexpected call: Commit") }
func (errExecTx) Rollback(_ context.Context) error        { panic("unexpected call: Rollback") }
func (errExecTx) LargeObjects() pgx.LargeObjects          { panic("unexpected call: LargeObjects") }
func (errExecTx) Conn() *pgx.Conn                         { panic("unexpected call: Conn") }
func (errExecTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults {
	panic("unexpected call: SendBatch")
}
func (errExecTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	panic("unexpected call: CopyFrom")
}
func (errExecTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	panic("unexpected call: Prepare")
}
func (errExecTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("unexpected call: Query")
}
func (errExecTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	panic("unexpected call: QueryRow")
}

// TestSyncStatusInTx_ExecError verifies that a database error during the
// quiniela status UPDATE is wrapped and returned (not swallowed).
func TestSyncStatusInTx_ExecError(t *testing.T) {
	t.Parallel()
	err := syncStatusInTx(context.Background(), errExecTx{}, 1, 3)
	if err == nil {
		t.Fatal("expected error from syncStatusInTx when Exec fails, got nil")
	}
}
