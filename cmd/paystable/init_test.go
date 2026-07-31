package main

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestRunInit_CreatesEnv(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	prev := doctorOut
	doctorOut = io.Discard
	t.Cleanup(func() { doctorOut = prev })

	if err := runInit(nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	data, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "DATABASE_URL=postgres://paystable:CHANGE_ME@localhost:5432/paystable?sslmode=disable") {
		t.Fatal("missing expected DATABASE_URL")
	}
	if !strings.Contains(content, "GATEWAY_API_KEY=") {
		t.Fatal("missing GATEWAY_API_KEY")
	}
	if !strings.Contains(content, "PAYU_STATUS_URL=") {
		t.Fatal("missing PAYU_STATUS_URL")
	}
	if strings.Contains(content, "__WEBHOOK_SECRET__") {
		t.Fatal("template placeholder left unsubstituted")
	}

	for _, key := range []string{
		"WEBHOOK_SECRET",
		"MERCHANT_CALLBACK_SECRET",
		"ADMIN_API_KEY",
		"SECRET_ENCRYPTION_KEY",
	} {
		val := envValue(content, key)
		if !hex64.MatchString(val) {
			t.Errorf("%s = %q, want 64 hex chars", key, val)
		}
	}

	// GATEWAY_API_KEY must stay empty (merchant-provided).
	if val := envValue(content, "GATEWAY_API_KEY"); val != "" {
		t.Errorf("GATEWAY_API_KEY = %q, want empty", val)
	}
}

func TestRunInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	prev := doctorOut
	doctorOut = io.Discard
	t.Cleanup(func() { doctorOut = prev })

	if err := os.WriteFile(".env", []byte("EXISTING=1\n"), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	err = runInit(nil)
	if err == nil {
		t.Fatal("expected error when .env exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}

	data, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(data) != "EXISTING=1\n" {
		t.Fatalf("existing .env was modified: %q", data)
	}
}

func TestRunInit_Help(t *testing.T) {
	if err := runInit([]string{"--help"}); err != nil {
		t.Fatalf("runInit help: %v", err)
	}
}

func envValue(content, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func TestEnvTemplateEmbedded(t *testing.T) {
	if envTemplate == "" {
		t.Fatal("env.template embed is empty")
	}
	for _, needle := range []string{
		"__WEBHOOK_SECRET__",
		"__MERCHANT_CALLBACK_SECRET__",
		"__ADMIN_API_KEY__",
		"__SECRET_ENCRYPTION_KEY__",
		"GATEWAY_API_KEY=",
		"PAYU_STATUS_URL=",
		"CHANGE_ME",
	} {
		if !strings.Contains(envTemplate, needle) {
			t.Errorf("env.template missing %q", needle)
		}
	}
}
