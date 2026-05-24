package webhook

import (
	"testing"
)

func TestParsePayload_JSON(t *testing.T) {
	body := []byte(`{"txnid":"order_1","status":"success","amount":"100.00"}`)

	params, err := parsePayload(body, "application/json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["txnid"] != "order_1" {
		t.Errorf("txnid = %q, want order_1", params["txnid"])
	}
	if params["status"] != "success" {
		t.Errorf("status = %q, want success", params["status"])
	}
}

func TestParsePayload_FormEncoded(t *testing.T) {
	body := []byte("txnid=order_2&status=failure&amount=250.00&email=user%40test.com")

	params, err := parsePayload(body, "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["txnid"] != "order_2" {
		t.Errorf("txnid = %q, want order_2", params["txnid"])
	}
	if params["email"] != "user@test.com" {
		t.Errorf("email = %q, want user@test.com", params["email"])
	}
}

func TestParsePayload_JSONAutoDetect(t *testing.T) {
	body := []byte(`{"txnid":"order_3"}`)

	params, err := parsePayload(body, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["txnid"] != "order_3" {
		t.Errorf("txnid = %q, want order_3", params["txnid"])
	}
}

func TestParsePayload_MalformedJSON(t *testing.T) {
	body := []byte(`{"txnid": broken`)

	_, err := parsePayload(body, "application/json")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParsePayload_EmptyBody(t *testing.T) {
	body := []byte("")

	params, err := parsePayload(body, "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 0 {
		t.Errorf("expected empty params, got %d entries", len(params))
	}
}

func TestParsePayload_FormWithSpecialChars(t *testing.T) {
	body := []byte("productinfo=Test+Product+%26+More&firstname=John+Doe")

	params, err := parsePayload(body, "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["productinfo"] != "Test Product & More" {
		t.Errorf("productinfo = %q, want 'Test Product & More'", params["productinfo"])
	}
	if params["firstname"] != "John Doe" {
		t.Errorf("firstname = %q, want 'John Doe'", params["firstname"])
	}
}
