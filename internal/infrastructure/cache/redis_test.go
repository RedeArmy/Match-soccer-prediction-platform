package cache_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

const fmtUnexpectedErr = "unexpected error: %v"

func TestNewClient_Success(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := cache.NewClient(context.Background(), cache.Config{
		Addr: mr.Addr(),
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	defer client.Close()
}

func TestNewClient_UnreachableAddr_ReturnsError(t *testing.T) {
	_, err := cache.NewClient(context.Background(), cache.Config{
		Addr: "127.0.0.1:19999", // nothing listening here
	})
	if err == nil {
		t.Error("expected error for unreachable Redis address, got nil")
	}
}
