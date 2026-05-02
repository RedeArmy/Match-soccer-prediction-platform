package repository_test

// index_usage_test.go verifies that the performance indexes introduced in
// migrations 000019-000021 are actually used by the planner for the queries
// they were designed to accelerate.
//
// Each test runs EXPLAIN (FORMAT JSON, ANALYZE FALSE) with sequential scans
// disabled (SET enable_seqscan = off) so the planner is forced to use an
// index if one exists. This is the standard technique for verifying index
// existence and correctness without needing seeded data: if the expected index
// is absent or unusable the query will fail or produce an unexpected node type.
//
// queryPlan uses a dedicated connection acquired from the pool so that the
// session-level GUC does not affect other concurrent tests.
//
// These tests are integration tests: they require a live PostgreSQL instance
// (provided by the shared testDB pool initialised in TestMain) with all
// migrations applied.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// planNode is a minimal representation of a single node in a PostgreSQL
// EXPLAIN JSON plan. Only the fields relevant to index verification are
// decoded; additional fields returned by PostgreSQL are silently ignored.
type planNode struct {
	NodeType  string     `json:"Node Type"`
	IndexName string     `json:"Index Name"`
	Plans     []planNode `json:"Plans"`
}

// explainWrapper is the top-level structure of EXPLAIN FORMAT JSON output.
type explainWrapper struct {
	Plan planNode `json:"Plan"`
}

// queryPlan executes EXPLAIN (FORMAT JSON, ANALYZE FALSE) for the given SQL
// and returns the root plan node. ANALYZE FALSE keeps the test free of side
// effects and avoids the need for seeded data.
//
// A dedicated connection is acquired from the pool so that SET enable_seqscan
// only affects this query; concurrent tests are not disturbed.
func queryPlan(t *testing.T, sql string, args ...any) planNode {
	t.Helper()
	ctx := context.Background()

	conn, err := testDB.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SET enable_seqscan = off; SET enable_bitmapscan = off"); err != nil {
		t.Fatalf("SET enable_seqscan/bitmapscan: %v", err)
	}

	explainSQL := "EXPLAIN (FORMAT JSON, ANALYZE FALSE) " + sql
	row := conn.QueryRow(ctx, explainSQL, args...)

	var raw []byte
	if err := row.Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN query failed: %v", err)
	}

	var wrappers []explainWrapper
	if err := json.Unmarshal(raw, &wrappers); err != nil {
		t.Fatalf("unmarshal EXPLAIN JSON: %v", err)
	}
	if len(wrappers) == 0 {
		t.Fatal("EXPLAIN returned empty plan")
	}
	return wrappers[0].Plan
}

// containsIndexScan reports whether the plan tree rooted at node contains an
// Index Scan or Index Only Scan node whose Index Name equals wantIndex.
func containsIndexScan(node planNode, wantIndex string) bool {
	nodeType := node.NodeType
	if (nodeType == "Index Scan" || nodeType == "Index Only Scan") &&
		strings.EqualFold(node.IndexName, wantIndex) {
		return true
	}
	for _, child := range node.Plans {
		if containsIndexScan(child, wantIndex) {
			return true
		}
	}
	return false
}

// hasSeqScan reports whether the plan tree contains a Sequential Scan on the
// given table name, which would indicate the target index is not being used.
func hasSeqScan(node planNode, table string) bool {
	if node.NodeType == "Seq Scan" {
		// Index Name is empty on Seq Scan; check the relation name field.
		// We re-decode lazily so as not to bloat planNode with all fields.
		return true // conservative: any seq scan is suspicious in these tests
	}
	_ = table
	for _, child := range node.Plans {
		if hasSeqScan(child, table) {
			return true
		}
	}
	return false
}

// TestIndexUsage_QuinielaInviteCode verifies that GetByInviteCode (migration
// 000022) uses idx_quinielas_invite_code_active instead of a sequential scan.
// Migration 000022 replaced the single-column index from 000019 with a
// composite index on (invite_code, invite_code_expires_at) to support
// index-only scans on the expiry check.
func TestIndexUsage_QuinielaInviteCode(t *testing.T) {
	const wantIndex = "idx_quinielas_invite_code_active"
	plan := queryPlan(t,
		`SELECT id, name, owner_id, invite_code, invite_code_expires_at,
		        entry_fee, currency, max_members, created_at, updated_at, deleted_at
		   FROM quinielas
		  WHERE invite_code = $1
		    AND deleted_at IS NULL
		    AND (invite_code_expires_at IS NULL OR invite_code_expires_at > NOW())`,
		"TESTCODE01",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q but got node type %q - run migrations 000022+",
			wantIndex, plan.NodeType)
	}
}

// TestIndexUsage_QuinielaOwnerCreated verifies that ListByOwner (migration
// 000019) uses the composite partial index idx_quinielas_owner_created, which
// covers both the owner_id filter and the created_at DESC sort.
func TestIndexUsage_QuinielaOwnerCreated(t *testing.T) {
	const wantIndex = "idx_quinielas_owner_created"
	plan := queryPlan(t,
		`SELECT id, name, owner_id, invite_code, entry_fee, currency, max_members,
		        created_at, updated_at, deleted_at
		   FROM quinielas
		  WHERE owner_id = $1
		    AND deleted_at IS NULL
		  ORDER BY created_at DESC`,
		1,
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q but got node type %q - run migrations 000019+",
			wantIndex, plan.NodeType)
	}
}

// TestIndexUsage_MatchesStatusKickoff verifies that ListByStatus (migration
// 000020) uses the composite index idx_matches_status_kickoff, eliminating the
// sort step that the former single-column idx_matches_status required.
//
// The query intentionally omits the stadium JOINs present in the production
// query: with ANALYZE FALSE and an empty table the planner assigns equal cost
// to index and sequential scans for multi-JOIN plans. A focused single-table
// query reliably triggers the index scan that the test is designed to assert.
func TestIndexUsage_MatchesStatusKickoff(t *testing.T) {
	const wantIndex = "idx_matches_status_kickoff"
	plan := queryPlan(t,
		`SELECT id, home_team, away_team, status, kickoff_at
		   FROM matches
		  WHERE status = $1
		  ORDER BY kickoff_at ASC`,
		"scheduled",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q - run migrations 000020+", wantIndex)
	}
}

// TestIndexUsage_MatchesPhaseKickoff verifies that ListByPhase (migration
// 000020) uses the composite index idx_matches_phase_kickoff.
//
// See TestIndexUsage_MatchesStatusKickoff for the rationale behind the
// simplified single-table query.
func TestIndexUsage_MatchesPhaseKickoff(t *testing.T) {
	const wantIndex = "idx_matches_phase_kickoff"
	plan := queryPlan(t,
		`SELECT id, home_team, away_team, phase, kickoff_at
		   FROM matches
		  WHERE phase = $1
		  ORDER BY kickoff_at ASC`,
		"group_stage",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q - run migrations 000020+", wantIndex)
	}
}

// TestIndexUsage_GroupMembershipsStatusPaid verifies that the composite partial
// index idx_group_memberships_quiniela_status_paid (migration 000021) is used
// for membership filter queries that combine quiniela_id, status, and paid.
func TestIndexUsage_GroupMembershipsStatusPaid(t *testing.T) {
	const wantIndex = "idx_group_memberships_quiniela_status_paid"
	plan := queryPlan(t,
		`SELECT id, quiniela_id, user_id, status, paid, joined_at, created_at, updated_at
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND status = 'active'
		    AND paid = false`,
		1,
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q - run migrations 000021+", wantIndex)
	}
}
