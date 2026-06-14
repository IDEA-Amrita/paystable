package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL            string
	Gateway                string
	WebhookSecret          string
	GatewayAPIKey          string
	PayuStatusURL          string
	MerchantCallbackSecret string
	AdminAPIKey            string
	Port                   string
	StabilizationN         int
	MaxBackoffS            int
	HoldMaxTTLS            int
	LogLevel               string
}
//1)StabilizationN:What: number of consecutive agreeing verification polls required to declare a terminal state (default 3)
//2)MaxBackoffS:What: maximum backoff time in seconds for retry attempts (default 160)// per-attempt cap, not a cumulative cap,eg for 160=>stops at 160s not when cumSum is 160s
//3)HoldMaxTTLS:What: is the absolute lifetime (seconds) of a hold from creation → expires_at(default 900)
//so when it checking reaches with MaxBackoffS it doesnt expand from there rather maintain there itself.But when cumSum of time taken exceeds HoldMaxTTLS,the checking break.from there it will see last N transactions(StabilizationN) n based on thatitss say whether success or failure
func Load() (*Config, error) {
	c := &Config{
		Port:           envOr("PORT", "8080"),
		StabilizationN: envIntOr("STABILIZATION_N", 3),
		MaxBackoffS:    envIntOr("MAX_BACKOFF_S", 160),
		HoldMaxTTLS:    envIntOr("HOLD_MAX_TTL_S", 900),
		LogLevel:       envOr("LOG_LEVEL", "info"),
	}

	required := map[string]*string{
		"DATABASE_URL":             &c.DatabaseURL,
		"GATEWAY":                  &c.Gateway,
		"WEBHOOK_SECRET":           &c.WebhookSecret,
		"GATEWAY_API_KEY":          &c.GatewayAPIKey,
		"PAYU_STATUS_URL":          &c.PayuStatusURL,
		"MERCHANT_CALLBACK_SECRET": &c.MerchantCallbackSecret,
		"ADMIN_API_KEY":            &c.AdminAPIKey,
	}

	for key, ptr := range required {
		val := os.Getenv(key)
		if val == "" {
			return nil, fmt.Errorf("required env var %s is not set", key)
		}
		*ptr = val
	}

	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
