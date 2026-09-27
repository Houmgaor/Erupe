package migrations

import (
	"bytes"
	"io/fs"
	"testing"
)

// TestMigrationsAreASCII keeps non-ASCII bytes out of migrations, comments
// included. PostgreSQL converts the whole statement to the database encoding
// before parsing it, so on a WIN1252 database a single katakana or arrow in a
// comment makes the migration fail and the server refuse to start (#198, #25).
func TestMigrationsAreASCII(t *testing.T) {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	for _, name := range files {
		data, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			for _, b := range line {
				if b >= 0x80 {
					t.Errorf("%s:%d has a non-ASCII byte: %q", name, i+1, line)
					break
				}
			}
		}
	}
}
