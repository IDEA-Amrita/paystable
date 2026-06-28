package payu

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// roundTripFunc lets us plug a function in as an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// payuResponse builds a PayU-shaped transaction_details response body.
func payuResponse(txnID, status, amount string) string {
	if txnID == "" {
		return `{"status":0,"msg":"No transaction found","transaction_details":{}}`
	}
	return `{"status":1,"msg":"Transaction Fetched Successfully","transaction_details":{"` +
		txnID + `":{"status":"` + status + `","amount":"` + amount + `","txnid":"` + txnID + `"}}}`
}

func newTestClient(rt http.RoundTripper) *Client {
	c := NewClient("https://payu.test/merchant/postservice.php", "test_key", "test_salt")
	c.HTTP = &http.Client{Transport: rt}
	return c
}

// ── Request shape ─────────────────────────────────────────────────────────────

func TestStatus_SendsPOSTWithFormParams(t *testing.T) {
	var got *http.Request
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = r.ParseForm()
		got = r
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_001", "success", "499.00"))),
			Header:     make(http.Header),
		}, nil
	})

	c := newTestClient(rt)
	_, _, _, _ = c.Status(context.Background(), "txn_001")

	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method)
	}
	if got.FormValue("key") != "test_key" {
		t.Errorf("form key = %q, want test_key", got.FormValue("key"))
	}
	if got.FormValue("command") != "verify_payment" {
		t.Errorf("form command = %q, want verify_payment", got.FormValue("command"))
	}
	if got.FormValue("var1") != "txn_001" {
		t.Errorf("form var1 = %q, want txn_001", got.FormValue("var1"))
	}
	if got.FormValue("hash") == "" {
		t.Error("hash field must not be empty")
	}
}

func TestStatus_HashMatchesExpected(t *testing.T) {
	var gotHash string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = r.ParseForm()
		gotHash = r.FormValue("hash")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_hash", "success", "100.00"))),
			Header:     make(http.Header),
		}, nil
	})

	c := newTestClient(rt)
	_, _, _, _ = c.Status(context.Background(), "txn_hash")

	expected := requestHash("test_key", "txn_hash", "test_salt")
	if gotHash != expected {
		t.Errorf("request hash = %q, want %q", gotHash, expected)
	}
}

func TestStatus_SetsFormURLEncodedContentType(t *testing.T) {
	var gotCT string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotCT = r.Header.Get("Content-Type")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_ct", "success", "100.00"))),
			Header:     make(http.Header),
		}, nil
	})
	newTestClient(rt).Status(context.Background(), "txn_ct") //nolint:errcheck
	if !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
}

// ── Status normalization ──────────────────────────────────────────────────────

func TestStatus_SuccessNormalized(t *testing.T) {
	for _, raw := range []string{"success", "captured", "completed", "SUCCESS", "Captured"} {
		t.Run(raw, func(t *testing.T) {
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(payuResponse("txn_s", raw, "100.00"))),
					Header:     make(http.Header),
				}, nil
			})
			status, _, _, err := newTestClient(rt).Status(context.Background(), "txn_s")
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if status != "success" {
				t.Errorf("raw=%q → status=%q, want success", raw, status)
			}
		})
	}
}

func TestStatus_FailedNormalized(t *testing.T) {
	for _, raw := range []string{"failed", "failure", "FAILED", "Failure"} {
		t.Run(raw, func(t *testing.T) {
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(payuResponse("txn_f", raw, "0"))),
					Header:     make(http.Header),
				}, nil
			})
			status, _, _, err := newTestClient(rt).Status(context.Background(), "txn_f")
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if status != "failed" {
				t.Errorf("raw=%q → status=%q, want failed", raw, status)
			}
		})
	}
}

func TestStatus_PendingNormalized(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_p", "pending", "100.00"))),
			Header:     make(http.Header),
		}, nil
	})
	status, _, _, err := newTestClient(rt).Status(context.Background(), "txn_p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
}

func TestStatus_UnknownStatusTreatedAsPending(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_u", "bounced", "100.00"))),
			Header:     make(http.Header),
		}, nil
	})
	status, _, _, err := newTestClient(rt).Status(context.Background(), "txn_u")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if status != "pending" {
		t.Errorf("unknown status should be pending, got %q", status)
	}
}

// ── Not found ────────────────────────────────────────────────────────────────

func TestStatus_EmptyTransactionDetails_NotFound(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("", "", ""))),
			Header:     make(http.Header),
		}, nil
	})
	status, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_nf")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if status != "not_found" {
		t.Errorf("status = %q, want not_found", status)
	}
	if amount != 0 {
		t.Errorf("amount = %d, want 0", amount)
	}
}

func TestStatus_TxnIDKeyMissingInDetails_NotFound(t *testing.T) {
	body := `{"status":1,"msg":"ok","transaction_details":{"other_txn":{"status":"success","amount":"100.00"}}}`
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	status, _, _, err := newTestClient(rt).Status(context.Background(), "txn_missing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if status != "not_found" {
		t.Errorf("status = %q, want not_found when txnID key absent from details", status)
	}
}

// ── Amount parsing ────────────────────────────────────────────────────────────

func TestStatus_DecimalRupeesConvertedToPaise(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_amt", "success", "499.00"))),
			Header:     make(http.Header),
		}, nil
	})
	_, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_amt")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if amount != 49900 {
		t.Errorf("amount = %d, want 49900 (499.00 rupees → paise)", amount)
	}
}

func TestStatus_IntegerAmountKeptAsPaise(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_int", "success", "49900"))),
			Header:     make(http.Header),
		}, nil
	})
	_, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_int")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if amount != 49900 {
		t.Errorf("amount = %d, want 49900", amount)
	}
}

func TestStatus_ZeroAmountOnFailed(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_zero", "failed", "0.00"))),
			Header:     make(http.Header),
		}, nil
	})
	_, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_zero")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if amount != 0 {
		t.Errorf("amount = %d, want 0", amount)
	}
}

// ── Error paths ───────────────────────────────────────────────────────────────

func TestStatus_MalformedJSON_ReturnsErrorAndRaw(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("not-json")),
			Header:     make(http.Header),
		}, nil
	})
	_, _, raw, err := newTestClient(rt).Status(context.Background(), "txn_bad")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if string(raw) != "not-json" {
		t.Errorf("raw = %q, want not-json", string(raw))
	}
}

func TestStatus_HTTPError_ReturnsError(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: code,
					Body:       io.NopCloser(strings.NewReader("error")),
					Header:     make(http.Header),
				}, nil
			})
			_, _, raw, err := newTestClient(rt).Status(context.Background(), "txn_err")
			if err == nil {
				t.Errorf("expected error for HTTP %d", code)
			}
			if len(raw) == 0 {
				t.Error("raw response should be preserved even on HTTP error")
			}
		})
	}
}

func TestStatus_NetworkError_ReturnsError(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused")
	})
	_, _, _, err := newTestClient(rt).Status(context.Background(), "txn_net")
	if err == nil {
		t.Fatal("expected error on network failure")
	}
}

func TestStatus_Timeout_ReturnsError(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("should not reach here")
		}
	})
	c := NewClient("https://payu.test/merchant/postservice.php", "k", "s")
	c.HTTP = &http.Client{
		Transport: rt,
		Timeout:   10 * time.Millisecond,
	}
	_, _, _, err := c.Status(context.Background(), "txn_timeout")
	if err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestStatus_EmptyBaseURL_ReturnsError(t *testing.T) {
	c := NewClient("", "k", "s")
	_, _, _, err := c.Status(context.Background(), "txn_nourl")
	if err == nil {
		t.Fatal("expected error when BaseURL is empty")
	}
}

// ── Raw response ──────────────────────────────────────────────────────────────

func TestStatus_RawResponsePreservedOnSuccess(t *testing.T) {
	body := payuResponse("txn_raw", "success", "100.00")
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	_, _, raw, err := newTestClient(rt).Status(context.Background(), "txn_raw")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if string(raw) != body {
		t.Errorf("raw = %q, want %q", string(raw), body)
	}
}

// ── Amount field priority (real PayU response shape) ─────────────────────────

// PayU's actual verify_payment response uses "amt", not "amount".
// This test uses the documented response shape to catch field-name drift.
func TestStatus_RealPayUResponseShape_UsesAmt(t *testing.T) {
	body := `{
		"status": 1,
		"msg": "Transaction Fetched Successfully",
		"transaction_details": {
			"txn_real": {
				"mihpayid": "403993715530530",
				"status": "success",
				"amt": "499.00",
				"transaction_amount": "499.00",
				"txnid": "txn_real"
			}
		}
	}`
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	status, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_real")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
	if amount != 49900 {
		t.Errorf("amount = %d, want 49900 (amt field takes priority)", amount)
	}
}

func TestStatus_FallsBackToTransactionAmount(t *testing.T) {
	body := `{"status":1,"msg":"ok","transaction_details":{"txn_ta":{"status":"success","transaction_amount":"100.00","txnid":"txn_ta"}}}`
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	_, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_ta")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if amount != 10000 {
		t.Errorf("amount = %d, want 10000", amount)
	}
}

func TestStatus_FallsBackToAmount(t *testing.T) {
	// "amount" is what the mock gateway returns — keep it as last fallback.
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payuResponse("txn_fb", "success", "200.00"))),
			Header:     make(http.Header),
		}, nil
	})
	_, amount, _, err := newTestClient(rt).Status(context.Background(), "txn_fb")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if amount != 20000 {
		t.Errorf("amount = %d, want 20000", amount)
	}
}

func TestStatus_BadAmountReturnsError(t *testing.T) {
	body := `{"status":1,"msg":"ok","transaction_details":{"txn_bad_amt":{"status":"success","amt":"not-a-number","txnid":"txn_bad_amt"}}}`
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	_, _, raw, err := newTestClient(rt).Status(context.Background(), "txn_bad_amt")
	if err == nil {
		t.Fatal("expected error for unparseable amount")
	}
	if len(raw) == 0 {
		t.Error("raw response should be preserved on amount parse error")
	}
}

// ── Unit tests for helpers ────────────────────────────────────────────────────

func TestNormalizeStatus(t *testing.T) {
	cases := []struct{ in, want string }{
		{"success", "success"},
		{"SUCCESS", "success"},
		{"captured", "success"},
		{"completed", "success"},
		{"failed", "failed"},
		{"failure", "failed"},
		{"FAILED", "failed"},
		{"pending", "pending"},
		{"PENDING", "pending"},
		{"", "not_found"},
		{"   ", "not_found"},
		{"unknown_state", "pending"},
	}
	for _, tc := range cases {
		if got := normalizeStatus(tc.in); got != tc.want {
			t.Errorf("normalizeStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"499.00", 49900},
		{"499.99", 49999},
		{"1.00", 100},
		{"0.00", 0},
		{"49900", 49900},
		{"100", 100},
		{"0", 0},
		{"", 0},
	}
	for _, tc := range cases {
		got, err := parseAmount(tc.in)
		if err != nil {
			t.Errorf("parseAmount(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseAmount(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRequestHash_Deterministic(t *testing.T) {
	h1 := requestHash("key1", "txn_abc", "salt1")
	h2 := requestHash("key1", "txn_abc", "salt1")
	if h1 != h2 {
		t.Error("requestHash must be deterministic")
	}
}

func TestRequestHash_DifferentInputsDifferentHash(t *testing.T) {
	h1 := requestHash("key1", "txn_abc", "salt1")
	h2 := requestHash("key1", "txn_xyz", "salt1")
	if h1 == h2 {
		t.Error("different txnIDs must produce different hashes")
	}
}
