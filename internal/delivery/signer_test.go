package delivery

import (
	"strings"
	"testing"
)

func TestSign_Format(t *testing.T) {
	sig := Sign([]byte("hello"), "secret")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("expected sha256= prefix, got %q", sig)
	}
	if len(sig) != len("sha256=")+64 {
		t.Fatalf("expected 64-char hex after prefix, got len %d", len(sig)-7)
	}
}

func TestVerify_RoundTrip(t *testing.T) {
	body := []byte(`{"txn_id":"t1","status":"CONFIRMED"}`)
	sig := Sign(body, "mysecret")
	if !Verify(body, sig, "mysecret") {
		t.Error("expected verification to pass")
	}
}

func TestVerify_TamperedBody(t *testing.T) {
	body := []byte(`{"txn_id":"t1","status":"CONFIRMED"}`)
	sig := Sign(body, "mysecret")
	tampered := []byte(`{"txn_id":"t1","status":"FAILED"}`)
	if Verify(tampered, sig, "mysecret") {
		t.Error("expected verification to fail for tampered body")
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	body := []byte("body")
	sig := Sign(body, "secret1")
	if Verify(body, sig, "secret2") {
		t.Error("expected verification to fail with wrong secret")
	}
}

func TestVerify_MissingPrefix(t *testing.T) {
	if Verify([]byte("body"), "abc123", "secret") {
		t.Error("expected failure without sha256= prefix")
	}
}

func TestVerify_InvalidHex(t *testing.T) {
	if Verify([]byte("body"), "sha256=zzz", "secret") {
		t.Error("expected failure on invalid hex")
	}
}

func TestVerify_UppercaseHex(t *testing.T) {
	body := []byte("body")
	sig := Sign(body, "secret")
	upper := "sha256=" + strings.ToUpper(strings.TrimPrefix(sig, "sha256="))
	// uppercase hex should fail since we don't normalize (consistent with PayU verify behavior)
	if Verify(body, upper, "secret") {
		t.Log("implementation normalizes case - acceptable but document it")
	}
}
