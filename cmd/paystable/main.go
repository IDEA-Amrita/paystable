package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/IDEA-Amrita/paystable/internal/config"
	"github.com/IDEA-Amrita/paystable/internal/database"
	"github.com/IDEA-Amrita/paystable/internal/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /webhooks/{gateway}", webhook.NewHandler(db, cfg.WebhookSecret))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	slog.Info("paystable starting", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
