package service

import "context"

// NoopKYCGate is a KYCGate implementation that allows every operation.
// Use in tests and in development environments where KYC enforcement is disabled.
type NoopKYCGate struct{}

func (NoopKYCGate) CheckWithdrawal(_ context.Context, _, _ int) error { return nil }
func (NoopKYCGate) CheckDeposit(_ context.Context, _, _ int) error    { return nil }
func (NoopKYCGate) CheckWinFreeze(_ context.Context, _, _ int) (bool, string, error) {
	return false, "", nil
}

var _ KYCGate = NoopKYCGate{}
