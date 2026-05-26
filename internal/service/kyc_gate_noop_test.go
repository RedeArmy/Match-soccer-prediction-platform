package service

import (
	"context"
	"testing"
)

func TestNoopKYCGate_CheckWithdrawal_AlwaysNil(t *testing.T) {
	gate := NoopKYCGate{}
	if err := gate.CheckWithdrawal(context.Background(), 1, 1000); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestNoopKYCGate_CheckDeposit_AlwaysNil(t *testing.T) {
	gate := NoopKYCGate{}
	if err := gate.CheckDeposit(context.Background(), 1, 1000); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestNoopKYCGate_ExceedsAMLThreshold_AlwaysFalse(t *testing.T) {
	exceeds, err := NoopKYCGate{}.ExceedsAMLThreshold(context.Background(), 99_999_999)
	if err != nil || exceeds {
		t.Errorf("expected (false, nil), got (%v, %v)", exceeds, err)
	}
}

func TestNoopKYCGate_ExceedsCumulativeAMLThreshold_AlwaysFalse(t *testing.T) {
	exceeds, err := NoopKYCGate{}.ExceedsCumulativeAMLThreshold(context.Background(), 1, 99_999_999)
	if err != nil || exceeds {
		t.Errorf("expected (false, nil), got (%v, %v)", exceeds, err)
	}
}

func TestNoopKYCGate_CheckDepositVelocity_AlwaysNil(t *testing.T) {
	gate := NoopKYCGate{}
	if err := gate.CheckDepositVelocity(context.Background(), 1, 1000); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestNoopKYCGate_CheckWithdrawalVelocity_AlwaysNil(t *testing.T) {
	gate := NoopKYCGate{}
	if err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1000); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestNoopKYCGate_CheckWinFreeze_NeverFreezes(t *testing.T) {
	freeze, reason, err := NoopKYCGate{}.CheckWinFreeze(context.Background(), 1, 99_999_999)
	if err != nil || freeze || reason != "" {
		t.Errorf("expected (false, \"\", nil), got (%v, %q, %v)", freeze, reason, err)
	}
}

func TestNoopKYCGate_ImplementsKYCGate(t *testing.T) {
	var _ KYCGate = NoopKYCGate{}
}
