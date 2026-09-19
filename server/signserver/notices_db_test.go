package signserver

import (
	"os"
	"testing"

	"erupe-ce/server/migrations"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// Runs only with ERUPE_TEST_DSN set.
func TestNoticeRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("ERUPE_TEST_DSN")
	if dsn == "" {
		t.Skip("ERUPE_TEST_DSN not set")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrations.Migrate(db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notices (body, expires_at) VALUES ('live', NULL), ('gone', now() - interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(`DELETE FROM notices WHERE body IN ('live','gone')`) }()

	got, err := NewSignNoticeRepository(db).ActiveNotices()
	if err != nil || len(got) != 1 || got[0] != "live" {
		t.Fatalf("ActiveNotices = %v, %v", got, err)
	}
}
