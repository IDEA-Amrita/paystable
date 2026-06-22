package adminapi

import (
	"database/sql"
	"os"
	"testing"

	"github.com/IDEA-Amrita/paystable/internal/database"
)

func TestMain(m *testing.M) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			if err := db.Ping(); err == nil {
				if err := database.Migrate(db); err != nil {
					panic("adminapi tests: migrate failed: " + err.Error())
				}
			}
			db.Close() //nolint:errcheck
		}
	}
	os.Exit(m.Run())
}
