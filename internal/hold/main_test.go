package hold

import (
	"database/sql"
	"os"
	"testing"

	"github.com/IDEA-Amrita/paystable/internal/database"
	_ "github.com/lib/pq"
)

func TestMain(m *testing.M) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			if err := db.Ping(); err == nil {
				if err := database.Migrate(db); err != nil {
					panic("hold tests: migrate failed: " + err.Error())
				}
			}
			db.Close() //nolint:errcheck
		}
	}
	os.Exit(m.Run())
}

func openTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	return db
}
