package randcode_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/randcode"
)

func TestCrypto_Generate_LengthAndAlphabet(t *testing.T) {
	g := randcode.Crypto{}
	code, err := g.Generate(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 10 {
		t.Errorf("expected length 10, got %d", len(code))
	}
	for _, c := range code {
		if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", c) {
			t.Errorf("unexpected character %q in generated code", c)
		}
	}
}

func TestCrypto_Generate_CustomAlphabet(t *testing.T) {
	g := randcode.Crypto{Alphabet: "AB"}
	code, err := g.Generate(8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range code {
		if c != 'A' && c != 'B' {
			t.Errorf("character %q not in custom alphabet", c)
		}
	}
}

func TestCrypto_Generate_ReaderError_Propagates(t *testing.T) {
	g := randcode.Crypto{Rand: &failReader{}}
	_, err := g.Generate(10)
	if err == nil {
		t.Fatal("expected error from failing reader, got nil")
	}
}

// failReader always returns an error from Read.
type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, fmt.Errorf("injected read error") }

func TestFixed_Generate_ReturnsCode(t *testing.T) {
	g := randcode.Fixed{Code: "TESTCODE"}
	code, err := g.Generate(99) // length param ignored
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "TESTCODE" {
		t.Errorf("expected TESTCODE, got %q", code)
	}
}
