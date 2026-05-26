package kyc_test

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/kyc"
)

func newAdapter(t *testing.T) *kyc.ManualReviewAdapter {
	t.Helper()
	return kyc.NewManualReviewAdapter(zap.NewNop())
}

func TestManualReviewAdapter_Name(t *testing.T) {
	if got := newAdapter(t).Name(); got != "manual_review" {
		t.Errorf("Name: want manual_review, got %q", got)
	}
}

func TestManualReviewAdapter_Submit_ReturnsUserIDAsSessionID(t *testing.T) {
	a := newAdapter(t)
	req := kyc.VerificationRequest{UserID: 42}
	sessionID, err := a.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := fmt.Sprintf("%d", req.UserID)
	if sessionID != want {
		t.Errorf("sessionID: want %q, got %q", want, sessionID)
	}
}

func TestManualReviewAdapter_Submit_DifferentUserIDs(t *testing.T) {
	a := newAdapter(t)
	for _, uid := range []int{1, 100, 99999} {
		sid, err := a.Submit(context.Background(), kyc.VerificationRequest{UserID: uid})
		if err != nil {
			t.Fatalf("uid=%d: unexpected error: %v", uid, err)
		}
		if sid != fmt.Sprintf("%d", uid) {
			t.Errorf("uid=%d: want %q, got %q", uid, fmt.Sprintf("%d", uid), sid)
		}
	}
}

func TestManualReviewAdapter_GetResult_ReturnsNilWhilePending(t *testing.T) {
	a := newAdapter(t)
	result, err := a.GetResult(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("GetResult: expected nil for in-progress session, got %+v", result)
	}
}

func TestManualReviewAdapter_ImplementsProviderInterface(t *testing.T) {
	var _ kyc.Provider = kyc.NewManualReviewAdapter(zap.NewNop())
}
