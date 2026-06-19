package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/IDEA-Amrita/paystable/internal/config"
	"github.com/IDEA-Amrita/paystable/internal/database"
	"github.com/IDEA-Amrita/paystable/internal/delivery"
	"github.com/IDEA-Amrita/paystable/internal/gateway"
	"github.com/IDEA-Amrita/paystable/internal/gateway/payu"
	"github.com/IDEA-Amrita/paystable/internal/hold"
	"github.com/IDEA-Amrita/paystable/internal/stabilizer"
	"github.com/IDEA-Amrita/paystable/internal/webhook"
)

//configuring env,database connection and migration,wenbhook handlers
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
	defer db.Close() //nolint:errcheck

	if err := database.Migrate(db); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	holdStore := hold.NewStore(db)
	holdHandler := hold.NewHandler(holdStore, cfg.HoldMaxTTLS)

	// start stabilizer worker (background)
	lag := stabilizer.NewLagEstimator()
	payuClient := payu.NewClient(cfg.PayuStatusURL, cfg.GatewayAPIKey)
	go stabilizer.Run(ctx, db, cfg, lag, func(g string) gateway.GatewayClient {
		if g == "payu" {
			return payuClient
		}
		return nil
	})

	// start delivery worker (background)
	go delivery.Run(ctx, db, delivery.Config{
		CallbackSecret:    cfg.MerchantCallbackSecret,
		AllowInsecure:     cfg.DeliveryAllowInsecure,
		TimeoutS:          cfg.DeliveryTimeoutS,
		WorkerConcurrency: cfg.DeliveryConcurrency,
	})

	mux := http.NewServeMux()

	//public endpoints
	//1)hmac verification n stores gateway signals in db
	mux.Handle("POST /webhooks/{gateway}", webhook.NewHandler(db, cfg.WebhookSecret))
	//2)returns hold transaction state n timeStamp
	mux.HandleFunc("GET /api/v1/transactions/{txn_id}/status", holdHandler.HandleStatus)
	//3)health check n state if current process is active or not
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	//authenticated endpoints
	//4)admin can create hold transaction with this endpoint
	mux.Handle("POST /api/v1/hold", authMiddleware(cfg.AdminAPIKey, http.HandlerFunc(holdHandler.HandleCreate)))

	slog.Info("paystable starting", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

//checks for valid API key in Authorisation header
func authMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer "+apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

//maps logs from local to global slog lvl(for centalised slog error maintainance)
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
