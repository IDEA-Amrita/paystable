package payu

import (
	"context"
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

// Client is a thin HTTP client for PayU status lookups. BaseURL should be configured
// by the environment or main program. If BaseURL is empty the client returns an error
// from Status so callers can decide how to proceed.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient returns a new PayU client. baseURL may be empty in which case Status returns an error.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Status queries PayU status API for txnID and returns a normalized status, amount, raw response, error.
// The exact endpoint and response format varies by gateway integration; callers should configure BaseURL accordingly.
func (c *Client) Status(ctx context.Context, txnID string) (string, int64, json.RawMessage, error) {
	if c.BaseURL == "" {
		return "", 0, nil, fmt.Errorf("payu status base URL not configured")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", 0, nil, err
	}
	q := u.Query()
	q.Set("txnid", txnID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", 0, nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		// return raw body and parsing error
		return "", 0, json.RawMessage(b), fmt.Errorf("invalid response JSON: %w", err)
	}

	// normalize some common fields
	status := ""
	if v, ok := m["status"].(string); ok {
		status = v
	} else if v, ok := m["payment_status"].(string); ok {
		status = v
	}

	var amount int64
	if v, ok := m["amount"].(float64); ok {
		amount = int64(v)
	} else if v, ok := m["amount"].(string); ok {
		if parsed, err := parseAmount(v); err == nil {
			amount = parsed
		}
	}

	return status, amount, json.RawMessage(b), nil
}

func parseAmount(v string) (int64, error) {
	v = strings.TrimSpace(v)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	if strings.Contains(v, ".") {
		return int64(math.Round(f * 100)), nil
	}
	return int64(f), nil
}
