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

// Client calls the PayU Transaction Status Check API (verify_payment command).
// Docs: https://docs.payu.in/reference/transaction-status-check-api-2
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
func NewClient(baseURL, merchantKey, salt string) *Client {
	return &Client{
		BaseURL:     baseURL,
		MerchantKey: merchantKey,
		Salt:        salt,
		HTTP:        &http.Client{Timeout: 10 * time.Second},
	}
}

// payuEnvelope is the outer PayU verify_payment response shape.
type payuEnvelope struct {
	Status             int                        `json:"status"`
	Msg                string                     `json:"msg"`
	TransactionDetails map[string]json.RawMessage `json:"transaction_details"`
}

// payuTxnDetail holds the fields we read from inside transaction_details.
type payuTxnDetail struct {
	Status string `json:"status"`
	Amount string `json:"amount"`
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

	txnRaw, ok := envelope.TransactionDetails[txnID]
	if !ok || txnRaw == nil || string(txnRaw) == "null" {
		return "not_found", 0, raw, nil
	}

	var detail payuTxnDetail
	if err := json.Unmarshal(txnRaw, &detail); err != nil {
		return "", 0, raw, fmt.Errorf("invalid transaction detail JSON: %w", err)
	}

	amount, _ := parseAmount(detail.Amount)
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
	case "":
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
