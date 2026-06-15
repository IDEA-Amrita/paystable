package stabilizer

import (
	"database/sql"
	"os"
	"testing"

	"github.com/IDEA-Amrita/paystable/internal/database"
)

// TestMain applies database migrations before running integration tests.
// When DATABASE_URL is unset the integration tests skip themselves, so we
// only migrate when a database is actually available.
func TestMain(m *testing.M) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			if err := db.Ping(); err == nil {
				if err := database.Migrate(db); err != nil {
					panic("stabilizer tests: migrate failed: " + err.Error())
				}
			}
			db.Close() //nolint:errcheck
		}
	}
	os.Exit(m.Run())
}
