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
	MerchantCallbackSecret string
	AdminAPIKey            string
	Port                   string
	StabilizationN         int
	MaxBackoffS            int
	HoldMaxTTLS            int
	LogLevel               string
}

func Load() (*Config, error) {
	c := &Config{
		Port:           envOr("PORT", "8080"),
		StabilizationN: envIntOr("STABILIZATION_N", 3),
		MaxBackoffS:    envIntOr("MAX_BACKOFF_S", 160),
		HoldMaxTTLS:    envIntOr("HOLD_MAX_TTL_S", 900),
		LogLevel:       envOr("LOG_LEVEL", "info"),
	}

	required := map[string]*string{
		"DATABASE_URL":            &c.DatabaseURL,
		"GATEWAY":                 &c.Gateway,
		"WEBHOOK_SECRET":          &c.WebhookSecret,
		"GATEWAY_API_KEY":         &c.GatewayAPIKey,
		"MERCHANT_CALLBACK_SECRET": &c.MerchantCallbackSecret,
		"ADMIN_API_KEY":           &c.AdminAPIKey,
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
