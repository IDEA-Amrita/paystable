package secrets

import (
	"encoding/hex"
	"testing"
)

func TestGenerateSecureKey_Format(t *testing.T) {
	got, err := GenerateSecureKey()
	if err != nil {
		t.Fatalf("GenerateSecureKey: %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("len = %d, want 64", len(got))
	}
	decoded, err := hex.DecodeString(got)
	if err != nil {
		t.Fatalf("not valid hex: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded len = %d, want 32", len(decoded))
	}
}

func TestGenerateAll_HasKeys(t *testing.T) {
	got, err := GenerateAll()
	if err != nil {
		t.Fatalf("GenerateAll: %v", err)
	}
	want := []string{
		"WEBHOOK_SECRET",
		"MERCHANT_CALLBACK_SECRET",
		"ADMIN_API_KEY",
		"SECRET_ENCRYPTION_KEY",
	}
	for _, key := range want {
		val, ok := got[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if val == "" {
			t.Errorf("%s is empty", key)
		}
		if len(val) != 64 {
			t.Errorf("%s len = %d, want 64", key, len(val))
		}
		if _, err := hex.DecodeString(val); err != nil {
			t.Errorf("%s is not valid hex: %v", key, err)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
}

func TestGenerateAll_Unique(t *testing.T) {
	got, err := GenerateAll()
	if err != nil {
		t.Fatalf("GenerateAll: %v", err)
	}
	seen := make(map[string]string, len(got))
	for key, val := range got {
		if prev, ok := seen[val]; ok {
			t.Fatalf("duplicate value for %q and %q", prev, key)
		}
		seen[val] = key
	}
}
