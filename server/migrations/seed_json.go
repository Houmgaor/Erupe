package migrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// seedJSONBlock is the JSON seed format: one file, one table. Files live
// under seed/<table>/*.json — the table is the enclosing directory name,
// never a field inside the file, so there's no string to keep in sync with
// the filesystem layout. Sits alongside top-level seed/*.sql, which remains
// the format for seed data that needs real SQL logic (subqueries,
// NOW()-relative rows, idempotency guards) rather than plain tabular data.
type seedJSONBlock struct {
	// Table is populated by the loader from the enclosing directory name,
	// never read from the file itself (hence no `json` tag).
	Table string
	// Comment is free-text documentation carried over from the original SQL
	// file header; it has no functional effect on seeding.
	Comment string `json:"comment,omitempty"`
	// Truncate empties the table before inserting, matching a handful of
	// seed files that reset their table on every run instead of relying on
	// ON CONFLICT DO NOTHING.
	Truncate   bool   `json:"truncate,omitempty"`
	OnConflict string `json:"onConflict,omitempty"`
	// Rows is one object per row, column name -> value, so every value is
	// self-documented instead of relying on position against a separate
	// column list. Every row in a block must have the same set of keys.
	Rows []map[string]interface{} `json:"rows"`
	// Key names the columns that identify a row across reloads (a natural
	// key such as shop_type+shop_id+item_id, never the serial id, which the
	// database assigns). Required by content files (see content.go), where
	// rows are matched on it so an edited row keeps its database id; on
	// first-boot seeds it only has to be unique.
	Key []string `json:"key,omitempty"`
	// Scope says which existing rows the file owns, as column -> value (or
	// -> list of values). A content file deletes rows in its scope that it
	// no longer lists, and refuses rows outside it; rows may leave scalar
	// scope columns out, they are filled in. {} means the whole table.
	// Absent, a content file only inserts and updates.
	Scope map[string]interface{} `json:"scope,omitempty"`
}

// identifierPattern restricts table/column names to safe SQL identifiers,
// optionally schema-qualified (e.g. "public.shop_items"). These names are
// interpolated directly into the query text since they can't be bound as
// placeholders, so validation is mandatory even though the source is a
// compile-time embedded file rather than user input.
var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// parseSeedJSONBlock parses one seed/<table>/*.json file's content. table is
// the enclosing directory name, supplied by the caller rather than read from
// the file.
func parseSeedJSONBlock(name, table string, data []byte) (seedJSONBlock, error) {
	var block seedJSONBlock
	if err := json.Unmarshal(data, &block); err != nil {
		return seedJSONBlock{}, fmt.Errorf("parsing %s: %w", name, err)
	}
	block.Table = table
	return block, nil
}

func applySeedJSON(db *sqlx.DB, name, table string, data []byte) error {
	block, err := parseSeedJSONBlock(name, table, data)
	if err != nil {
		return err
	}
	if err := block.validateKey(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := applySeedJSONBlock(db, block); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// isForeignKeyViolation reports whether err is Postgres error 23503
// (foreign_key_violation), used by the caller to retry seed directories in a
// different order rather than needing to hardcode table dependency order.
func isForeignKeyViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23503"
}

func applySeedJSONBlock(db *sqlx.DB, block seedJSONBlock) error {
	if !identifierPattern.MatchString(block.Table) {
		return fmt.Errorf("invalid table name %q", block.Table)
	}
	if len(block.Rows) == 0 {
		return nil
	}

	if block.Truncate {
		if _, err := db.Exec(fmt.Sprintf("TRUNCATE %s", block.Table)); err != nil {
			return fmt.Errorf("truncating: %w", err)
		}
	}

	query, args, err := buildSeedInsert(block)
	if err != nil {
		return err
	}
	if _, err := db.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting: %w", err)
	}
	return nil
}

func buildSeedInsert(block seedJSONBlock) (string, []interface{}, error) {
	columns := rowColumns(block.Rows[0])
	for _, col := range columns {
		if !identifierPattern.MatchString(col) {
			return "", nil, fmt.Errorf("invalid column name %q", col)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "INSERT INTO %s (%s) VALUES ", block.Table, strings.Join(columns, ", "))

	var args []interface{}
	for r, row := range block.Rows {
		if !hasExactColumns(row, columns) {
			return "", nil, fmt.Errorf("row %d has columns %v, want %v (matching row 0)", r, rowColumns(row), columns)
		}
		if r > 0 {
			sb.WriteString(", ")
		}
		sb.WriteByte('(')
		for c, col := range columns {
			if c > 0 {
				sb.WriteString(", ")
			}
			val := row[col]
			raw, isRaw, err := rawSQLValue(val)
			if err != nil {
				return "", nil, fmt.Errorf("row %d column %s: %w", r, col, err)
			}
			if isRaw {
				sb.WriteString(raw)
				continue
			}
			args = append(args, normalizeSeedValue(val))
			fmt.Fprintf(&sb, "$%d", len(args))
		}
		sb.WriteByte(')')
	}

	if block.OnConflict != "" {
		fmt.Fprintf(&sb, " ON CONFLICT %s", block.OnConflict)
	}
	return sb.String(), args, nil
}

// rowColumns returns a row's keys in sorted order, giving a deterministic
// column order for the generated INSERT regardless of Go's randomized map
// iteration order.
func rowColumns(row map[string]interface{}) []string {
	cols := make([]string, 0, len(row))
	for k := range row {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// hasExactColumns reports whether row's key set is exactly columns (every
// row in a block must share the same columns, since they're combined into a
// single multi-row INSERT).
func hasExactColumns(row map[string]interface{}, columns []string) bool {
	if len(row) != len(columns) {
		return false
	}
	for _, c := range columns {
		if _, ok := row[c]; !ok {
			return false
		}
	}
	return true
}

// rawSQLValue detects the {"raw": "..."} escape hatch for values JSON has no
// literal syntax for, namely computed SQL expressions like NOW().
func rawSQLValue(val interface{}) (string, bool, error) {
	m, ok := val.(map[string]interface{})
	if !ok {
		return "", false, nil
	}
	rawVal, ok := m["raw"]
	if !ok || len(m) != 1 {
		return "", false, fmt.Errorf(`object value must be {"raw": "<sql>"}, got %v`, m)
	}
	s, ok := rawVal.(string)
	if !ok {
		return "", false, fmt.Errorf(`"raw" value must be a string, got %v`, rawVal)
	}
	return s, true, nil
}

// normalizeSeedValue converts JSON's float64 number decoding back to an
// integer when the source literal had no fractional part, so integer
// columns receive "500000" rather than "500000.0"-shaped parameters.
func normalizeSeedValue(val interface{}) interface{} {
	f, ok := val.(float64)
	if !ok {
		return val
	}
	if f == math.Trunc(f) {
		return int64(f)
	}
	return f
}

// validateKey checks the optional key declaration: every key column must be
// a valid identifier present in each row, and no two rows may share a key.
// Rows are checked as written (before any scope fill-in), so it is safe to
// call on a block straight out of parseSeedJSONBlock.
func (b seedJSONBlock) validateKey() error {
	if len(b.Key) == 0 {
		return nil
	}
	for _, col := range b.Key {
		if !identifierPattern.MatchString(col) {
			return fmt.Errorf("invalid key column name %q", col)
		}
	}
	seen := make(map[string]int, len(b.Rows))
	for r, row := range b.Rows {
		k, err := rowKey(row, b.Key)
		if err != nil {
			return fmt.Errorf("row %d: %w", r, err)
		}
		if first, dup := seen[k]; dup {
			return fmt.Errorf("rows %d and %d have the same key %s", first, r, k)
		}
		seen[k] = r
	}
	return nil
}

// rowKey renders a row's key columns as one comparable string.
func rowKey(row map[string]interface{}, key []string) (string, error) {
	parts := make([]string, 0, len(key))
	for _, col := range key {
		v, ok := row[col]
		if !ok {
			return "", fmt.Errorf("missing key column %q", col)
		}
		if _, isRaw := v.(map[string]interface{}); isRaw {
			return "", fmt.Errorf("key column %q cannot be a raw SQL value", col)
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		parts = append(parts, col+"="+string(enc))
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}
