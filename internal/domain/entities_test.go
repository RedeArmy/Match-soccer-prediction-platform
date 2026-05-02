package domain_test

// Contract tests for typed constants defined in entities.go.
//
// These tests verify that the string values stored in the database match the
// named constants in code. A failing test here signals a breaking change to
// the DB schema or a mismatched backfill - both require a migration.

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── MembershipRole ────────────────────────────────────────────────────────────

func TestMembershipRoleCreateOwner_DBValue(t *testing.T) {
	if got := string(domain.MembershipRoleCreateOwner); got != "owner" {
		t.Errorf("MembershipRoleCreateOwner DB value = %q, want %q", got, "owner")
	}
}

func TestMembershipRoleMember_DBValue(t *testing.T) {
	if got := string(domain.MembershipRoleMember); got != "member" {
		t.Errorf("MembershipRoleMember DB value = %q, want %q", got, "member")
	}
}

// ── SystemParamType ───────────────────────────────────────────────────────────

func TestSystemParamType_DBValues(t *testing.T) {
	cases := []struct {
		constant domain.SystemParamType
		want     string
	}{
		{domain.SystemParamTypeString, "string"},
		{domain.SystemParamTypeInt, "int"},
		{domain.SystemParamTypeBool, "bool"},
		{domain.SystemParamTypeDuration, "duration"},
	}
	for _, tc := range cases {
		if got := string(tc.constant); got != tc.want {
			t.Errorf("SystemParamType constant value = %q, want %q", got, tc.want)
		}
	}
}

// ── PaymentStatus ─────────────────────────────────────────────────────────────

func TestPaymentStatus_DBValues(t *testing.T) {
	cases := []struct {
		constant domain.PaymentStatus
		want     string
	}{
		{domain.PaymentStatusPending, "pending"},
		{domain.PaymentStatusConfirmed, "confirmed"},
		{domain.PaymentStatusRefunded, "refunded"},
		{domain.PaymentStatusRejected, "rejected"},
	}
	for _, tc := range cases {
		if got := string(tc.constant); got != tc.want {
			t.Errorf("PaymentStatus constant value = %q, want %q", got, tc.want)
		}
	}
}
