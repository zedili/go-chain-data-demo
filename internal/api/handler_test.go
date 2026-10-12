package api

import (
	"math/big"
	"testing"
)

func TestIsHash(t *testing.T) {
	valid := "0x0000000000000000000000000000000000000000000000000000000000000001"
	if !isHash(valid) {
		t.Fatal("expected valid hash")
	}
	for _, value := range []string{"", "0x1", "zz0000000000000000000000000000000000000000000000000000000000000000"} {
		if isHash(value) {
			t.Fatalf("expected invalid hash: %q", value)
		}
	}
}

func TestParseOptionalBlockNumber(t *testing.T) {
	for _, value := range []string{"", "latest"} {
		got, err := parseOptionalBlockNumber(value)
		if err != nil || got != nil {
			t.Fatalf("parse %q = %v, %v", value, got, err)
		}
	}
	got, err := parseOptionalBlockNumber("123")
	if err != nil || got.Cmp(big.NewInt(123)) != 0 {
		t.Fatalf("parse decimal = %v, %v", got, err)
	}
	if _, err := parseOptionalBlockNumber("0x7b"); err == nil {
		t.Fatal("expected hexadecimal input to be rejected")
	}
}

func TestWeiToEther(t *testing.T) {
	got := weiToEther(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	if got != "1.000000000000000000" {
		t.Fatalf("unexpected ether value: %s", got)
	}
}
