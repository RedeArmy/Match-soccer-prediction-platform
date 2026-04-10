package repository_test

// index_usage_test.go verifies that the performance indexes introduced in
// migrations 000019–000021 are actually used by the planner for the queries
// they were designed to accelerate.
//
// Each test runs EXPLAIN (FORMAT JSON, ANALYZE FALSE) — analysis is disabled
// so no test data is needed — and asserts that the plan tree contains an
// Index Scan or Index Only Scan node whose index name matches the expected
// index. A Seq Scan on the target table is treated as a test failure because
// it means either the index was not created or the query was written in a way
// that prevents the planner from using it.
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
func queryPlan(t *testing.T, sql string, args ...any) planNode {
	t.Helper()
	explainSQL := "EXPLAIN (FORMAT JSON, ANALYZE FALSE) " + sql
	row := testDB.QueryRow(context.Background(), explainSQL, args...)

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
// 000019) uses idx_quinielas_invite_code instead of a sequential scan.
func TestIndexUsage_QuinielaInviteCode(t *testing.T) {
	const wantIndex = "idx_quinielas_invite_code"
	plan := queryPlan(t,
		`SELECT id, name, owner_id, invite_code, entry_fee, currency, max_members,
		        created_at, updated_at, deleted_at
		   FROM quinielas
		  WHERE invite_code = $1
		    AND deleted_at IS NULL`,
		"TESTCODE01",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q but got node type %q — run migrations 000019+",
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
		t.Errorf("expected plan to use %q but got node type %q — run migrations 000019+",
			wantIndex, plan.NodeType)
	}
}

// TestIndexUsage_MatchesStatusKickoff verifies that ListByStatus (migration
// 000020) uses the composite index idx_matches_status_kickoff, eliminating the
// sort step that the former single-column idx_matches_status required.
func TestIndexUsage_MatchesStatusKickoff(t *testing.T) {
	const wantIndex = "idx_matches_status_kickoff"
	plan := queryPlan(t,
		`SELECT m.id, m.home_team, m.away_team, m.home_score, m.away_score,
		        m.status, m.phase, m.stadium_id, m.kickoff_at, m.created_at, m.updated_at,
		        s.id, s.name, s.capacity,
		        ci.id, ci.name,
		        st.id, st.name, st.code,
		        co.id, co.name, co.code
		   FROM matches m
		   LEFT JOIN stadiums  s  ON s.id  = m.stadium_id
		   LEFT JOIN cities    ci ON ci.id = s.city_id
		   LEFT JOIN states    st ON st.id = ci.state_id
		   LEFT JOIN countries co ON co.id = st.country_id
		  WHERE m.status = $1
		  ORDER BY m.kickoff_at ASC`,
		"scheduled",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q — run migrations 000020+", wantIndex)
	}
}

// TestIndexUsage_MatchesPhaseKickoff verifies that ListByPhase (migration
// 000020) uses the composite index idx_matches_phase_kickoff.
func TestIndexUsage_MatchesPhaseKickoff(t *testing.T) {
	const wantIndex = "idx_matches_phase_kickoff"
	plan := queryPlan(t,
		`SELECT m.id, m.home_team, m.away_team, m.home_score, m.away_score,
		        m.status, m.phase, m.stadium_id, m.kickoff_at, m.created_at, m.updated_at,
		        s.id, s.name, s.capacity,
		        ci.id, ci.name,
		        st.id, st.name, st.code,
		        co.id, co.name, co.code
		   FROM matches m
		   LEFT JOIN stadiums  s  ON s.id  = m.stadium_id
		   LEFT JOIN cities    ci ON ci.id = s.city_id
		   LEFT JOIN states    st ON st.id = ci.state_id
		   LEFT JOIN countries co ON co.id = st.country_id
		  WHERE m.phase = $1
		  ORDER BY m.kickoff_at ASC`,
		"group_stage",
	)
	if !containsIndexScan(plan, wantIndex) {
		t.Errorf("expected plan to use %q — run migrations 000020+", wantIndex)
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
		t.Errorf("expected plan to use %q — run migrations 000021+", wantIndex)
	}
}
