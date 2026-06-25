package main

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/IDEA-Amrita/paystable/internal/config"
	"github.com/IDEA-Amrita/paystable/internal/database"
)

const postgresSetupGuide = "https://docs-paystable.vercel.app/guides/getting-started/#set-up-postgres"

var requiredEnv = []string{
	"DATABASE_URL",
	"GATEWAY",
	"WEBHOOK_SECRET",
	"GATEWAY_API_KEY",
	"PAYU_STATUS_URL",
	"MERCHANT_CALLBACK_SECRET",
	"ADMIN_API_KEY",
}

func runDoctor(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			printDoctorUsage()
			return nil
		default:
			return fmt.Errorf("unknown doctor option: %s", args[0])
		}
	}

	infoLine("starting paystable doctor")
	config.LoadDotEnv()
	infoLine("loaded .env if present")

	missing := missingRequiredEnv()
	if len(missing) > 0 {
		warnLine("missing required env vars: " + strings.Join(missing, ", "))
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		infoLine("database setup guide: " + postgresSetupGuide)
		return fmt.Errorf("DATABASE_URL is not set")
	}

	infoLine("database target: " + describeDatabaseURL(dsn))
	db, err := database.Connect(dsn)
	if err != nil {
		infoLine("database setup guide: " + postgresSetupGuide)
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer closeDB(db)
	okLine("connected to postgres")

	infoLine("checking and applying database migrations")
	if err := migrateQuietly(db); err != nil {
		return fmt.Errorf("migration check failed: %w", err)
	}
	okLine("database migrations are ready")

	if len(missing) > 0 {
		infoLine("edit .env and run: ./paystable doctor")
		return fmt.Errorf("doctor found missing configuration")
	}

	okLine("paystable is ready to start")
	return nil
}

func printDoctorUsage() {
	fmt.Println("usage: paystable doctor")
	fmt.Println()
	fmt.Println("checks:")
	fmt.Println("  .env required variables")
	fmt.Println("  Postgres connection")
	fmt.Println("  database migrations")
}

func missingRequiredEnv() []string {
	var missing []string
	for _, key := range requiredEnv {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func describeDatabaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "unparseable DATABASE_URL"
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = "(missing database name)"
	}

	user := u.User.Username()
	if user == "" {
		user = "(missing user)"
	}

	host := u.Host
	if host == "" {
		host = "(missing host)"
	}

	return fmt.Sprintf("user=%s host=%s database=%s", user, host, dbName)
}

func closeDB(db *sql.DB) {
	if err := db.Close(); err != nil {
		warnLine("database close failed: " + err.Error())
	}
}

func migrateQuietly(db *sql.DB) error {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(previous)
	return database.Migrate(db)
}

func infoLine(msg string) {
	fmt.Println("[INFO] " + msg)
}

func okLine(msg string) {
	fmt.Println("[OK] " + msg)
}

func warnLine(msg string) {
	fmt.Println("[WARN] " + msg)
}
