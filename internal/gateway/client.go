package gateway

import (
	"context"
	"encoding/json"
)

// GatewayClient abstracts gateway status checks.
// Status returns normalized status (e.g. "success","failed","pending","not_found"),
// amount in smallest currency unit, raw JSON from gateway, and error.
type GatewayClient interface {
	Status(ctx context.Context, txnID string) (gatewayStatus string, gatewayAmount int64, raw json.RawMessage, err error)
}
