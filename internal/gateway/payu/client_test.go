package payu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientStatusParsesPayUAmountString(t *testing.T) {
	var gotTxnID string
	var gotAuth string
	client := NewClient("https://payu.test/status", "test-key")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotTxnID = r.URL.Query().Get("txnid")
		gotAuth = r.Header.Get("Authorization")
		body, _ := json.Marshal(map[string]string{"status": "success", "amount": "499.00"})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Header:     make(http.Header),
		}, nil
	})}

	status, amount, raw, err := client.Status(context.Background(), "txn_123")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if gotTxnID != "txn_123" {
		t.Fatalf("txnid = %q, want txn_123", gotTxnID)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if status != "success" {
		t.Fatalf("status = %q, want success", status)
	}
	if amount != 49900 {
		t.Fatalf("amount = %d, want 49900", amount)
	}
	if len(raw) == 0 {
		t.Fatal("raw response is empty")
	}
}

func TestClientStatusKeepsIntegerAmountAsSmallestUnit(t *testing.T) {
	client := NewClient("https://payu.test/status", "")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"payment_status": "captured", "amount": 49900})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Header:     make(http.Header),
		}, nil
	})}
	status, amount, _, err := client.Status(context.Background(), "txn_123")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status != "captured" {
		t.Fatalf("status = %q, want captured", status)
	}
	if amount != 49900 {
		t.Fatalf("amount = %d, want 49900", amount)
	}
}

func TestClientStatusReturnsRawOnInvalidJSON(t *testing.T) {
	client := NewClient("https://payu.test/status", "")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("not-json")),
			Header:     make(http.Header),
		}, nil
	})}
	_, _, raw, err := client.Status(context.Background(), "txn_123")
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if string(raw) != "not-json" {
		t.Fatalf("raw = %q, want not-json", string(raw))
	}
}
