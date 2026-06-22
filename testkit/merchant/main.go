package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
)

var offline atomic.Bool

func main() {
	port := envOr("PORT", "9091")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /callback", handleCallback)
	mux.HandleFunc("POST /toggle-offline", handleToggle)
	mux.HandleFunc("GET /status", handleMerchantStatus)
	slog.Info("mock merchant starting", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("mock merchant failed", "error", err)
		os.Exit(1)
	}
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	if offline.Load() {
		slog.Warn("merchant offline, returning 503")
		http.Error(w, "offline", http.StatusServiceUnavailable)
		return
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	slog.Info("callback received",
		"txn_id", payload["txn_id"],
		"status", payload["status"],
		"event", payload["event"],
		"amount", payload["amount"],
		"idempotency_key", r.Header.Get("X-Paystable-Idempotency-Key"),
		"signature", r.Header.Get("X-Paystable-Signature"),
	)
	w.WriteHeader(http.StatusOK)
}

func handleToggle(w http.ResponseWriter, r *http.Request) {
	was := offline.Load()
	offline.Store(!was)
	state := "online"
	if !was {
		state = "offline"
	}
	slog.Info("merchant toggled", "state", state)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"state": state})
}

func handleMerchantStatus(w http.ResponseWriter, _ *http.Request) {
	state := "online"
	if offline.Load() {
		state = "offline"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"state": state})
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
