package payu

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client calls the PayU Verify Payment API (verify_payment command).
// Docs: https://docs.payu.in/reference/verify_payment_api
type Client struct {
	BaseURL     string
	MerchantKey string
	Salt        string
	HTTP        *http.Client
}

// NewClient returns a PayU client ready to call verify_payment.
// baseURL  = PAYU_STATUS_URL
// merchantKey = GATEWAY_API_KEY (the merchant key, not the salt)
// salt     = WEBHOOK_SECRET (the PayU salt, same one used for webhook HMAC)
//
// postservice.php returns a PHP-serialized body unless the request carries
// form=2, which asks for JSON. Rather than trust every deployment's env var
// to include it, we inject it here if it's missing so a bare
// PAYU_STATUS_URL doesn't silently break JSON parsing in production.
func NewClient(baseURL, merchantKey, salt string) *Client {
	return &Client{
		BaseURL:     ensureFormParam(baseURL),
		MerchantKey: merchantKey,
		Salt:        salt,
		HTTP:        &http.Client{Timeout: 10 * time.Second},
	}
}

// ensureFormParam appends form=2 to the URL if not already present.
// Malformed URLs are returned unchanged; the request itself will fail
// with a clear error rather than this constructor silently swallowing it.
func ensureFormParam(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("form") != "" {
		return raw
	}
	q.Set("form", "2")
	u.RawQuery = q.Encode()
	return u.String()
}

// payuEnvelope is the outer PayU verify_payment response shape.
type payuEnvelope struct {
	Status             int                        `json:"status"`
	Msg                string                     `json:"msg"`
	TransactionDetails map[string]json.RawMessage `json:"transaction_details"`
}

// payuTxnDetail holds the fields we read from inside transaction_details.
// PayU's verify_payment response uses amt as the primary amount field,
// falling back to transaction_amount, then amount (used by the mock gateway).
type payuTxnDetail struct {
	Status            string `json:"status"`
	Amt               string `json:"amt"`
	TransactionAmount string `json:"transaction_amount"`
	Amount            string `json:"amount"`
}

// Status implements gateway.GatewayClient.
// It POSTs the verify_payment command to PayU, parses the nested
// transaction_details, and returns a normalized status, amount in paise,
// raw response, and error.
func (c *Client) Status(ctx context.Context, txnID string) (string, int64, json.RawMessage, error) {
	if c.BaseURL == "" {
		return "", 0, nil, fmt.Errorf("payu status base URL not configured")
	}

	form := url.Values{}
	form.Set("key", c.MerchantKey)
	form.Set("command", "verify_payment")
	form.Set("var1", txnID)
	form.Set("hash", requestHash(c.MerchantKey, txnID, c.Salt))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, nil, err
	}
	raw := json.RawMessage(b)

	if resp.StatusCode != http.StatusOK {
		return "", 0, raw, fmt.Errorf("payu status API returned HTTP %d", resp.StatusCode)
	}

	var envelope payuEnvelope
	if err := json.Unmarshal(b, &envelope); err != nil {
		return "", 0, raw, fmt.Errorf("invalid response JSON: %w", err)
	}

	// transaction_details missing from the envelope entirely is a schema
	// violation (wrong command, flat check_payment-shaped body, a routing
	// error, ...), not a "this txn doesn't exist yet" signal. Fail loud
	// with the raw payload attached instead of quietly mapping it to
	// not_found — those two situations need very different handling.
	if envelope.TransactionDetails == nil {
		return "", 0, raw, fmt.Errorf("payu: response missing transaction_details, expected nested verify_payment shape")
	}

	txnRaw, ok := envelope.TransactionDetails[txnID]
	if !ok || txnRaw == nil || string(txnRaw) == "null" {
		return "not_found", 0, raw, nil
	}

	var detail payuTxnDetail
	if err := json.Unmarshal(txnRaw, &detail); err != nil {
		return "", 0, raw, fmt.Errorf("invalid transaction detail JSON: %w", err)
	}

	// PayU represents "no record of this txn yet" as a present entry with
	// status: "Not Found" rather than an absent key. Treat it the same as
	// a missing key (not_found) and skip amount parsing entirely — there
	// is no real amount to extract, and committing a 0 here would pollute
	// verification_polls.gateway_amount with a value that looks like a
	// genuine gateway response instead of "we have nothing yet".
	if normalizeStatus(detail.Status) == "not_found" {
		return "not_found", 0, raw, nil
	}

	rawAmt := detail.Amt
	if rawAmt == "" {
		rawAmt = detail.TransactionAmount
	}
	if rawAmt == "" {
		rawAmt = detail.Amount
	}
	amount, err := parseAmount(rawAmt)
	if err != nil {
		return "", 0, raw, fmt.Errorf("payu: unparseable amount %q: %w", rawAmt, err)
	}
	return normalizeStatus(detail.Status), amount, raw, nil
}

// requestHash computes sha512(merchantKey|"verify_payment"|txnID|salt).
// This is the hash PayU requires on the verify_payment request.
func requestHash(merchantKey, txnID, salt string) string {
	s := merchantKey + "|verify_payment|" + txnID + "|" + salt
	h := sha512.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// normalizeStatus maps raw PayU status strings to the four values the
// stabilizer reasons about: "success", "failed", "pending", "not_found".
// Unknown statuses are treated as "pending" — never as "failed" — so
// we don't mark a payment dead on a string we don't recognise.
func normalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "captured", "completed":
		return "success"
	case "failure", "failed":
		return "failed"
	case "pending":
		return "pending"
	case "", "not found":
		return "not_found"
	default:
		return "pending"
	}
}

// parseAmount converts a PayU amount value to paise (smallest INR unit).
// PayU returns decimal rupees as strings ("499.00"). Integer values are
// assumed to already be in paise.
func parseAmount(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	if strings.Contains(v, ".") {
		return int64(math.Round(f * 100)), nil
	}
	return int64(f), nil
}
