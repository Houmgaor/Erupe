package migrations

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Content files let a server operator keep game content — shop rows, prize
// lists, exchange lists, event quests… — as files next to the quest data
// instead of hand-written SQL. The format is the seed format (seed_json.go):
// a copy of seed/<table>/<file>.json dropped into <content dir>/<table>/ is
// a valid content file. The difference is what applying it means:
//
//   - seeds insert once, on an empty database;
//   - content files are synchronised on every start and on
//     POST /v2/admin/content/reload: rows are matched on "key", so an
//     existing row is updated in place (its id, and so purchase counters
//     that reference it, survive), a new row is inserted, and a row inside
//     "scope" that the file no longer lists is deleted.
//
// Nothing happens when the directory does not exist, so servers that keep
// editing the database directly see no change.

// ContentResult is what applying one content file did.
type ContentResult struct {
	Path     string `json:"path"`
	Table    string `json:"table"`
	Inserted int64  `json:"inserted"`
	Updated  int64  `json:"updated"`
	Deleted  int64  `json:"deleted"`
}

// ApplyContentDir synchronises every <dir>/<table>/*.json file into its
// table, each file in its own transaction. Files that fail on a foreign-key
// violation are retried after the others, like seeds. A missing directory
// is reported as an error wrapping fs.ErrNotExist; any other error stops
// the run, and the results already applied are returned with it.
func ApplyContentDir(db *sqlx.DB, logger *zap.Logger, dir string) ([]ContentResult, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("content directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("content directory %s: not a directory", dir)
	}
	fsys := os.DirFS(dir)
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading content directory %s: %w", dir, err)
	}
	var tables []string
	for _, e := range entries {
		if e.IsDir() {
			tables = append(tables, e.Name())
		}
	}
	sort.Strings(tables)
	files, err := listJSONTableFiles(fsys, tables)
	if err != nil {
		return nil, err
	}

	var results []ContentResult
	_, err = applyWithFKRetry(files, func(f seedJSONFile) error {
		data, err := fs.ReadFile(fsys, f.path)
		if err != nil {
			return fmt.Errorf("reading content file %s: %w", f.path, err)
		}
		res, err := applyContentJSON(db, f.path, f.table, data)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Content: %s synchronised", f.path),
			zap.Int64("inserted", res.Inserted), zap.Int64("updated", res.Updated), zap.Int64("deleted", res.Deleted))
		results = append(results, res)
		return nil
	})
	return results, err
}

func applyContentJSON(db *sqlx.DB, name, table string, data []byte) (ContentResult, error) {
	block, err := parseSeedJSONBlock(name, table, data)
	if err != nil {
		return ContentResult{}, err
	}
	res, err := syncJSONBlock(db, block)
	if err != nil {
		return ContentResult{}, fmt.Errorf("%s: %w", name, err)
	}
	res.Path = name
	return res, nil
}

// syncTempTable is the per-transaction staging table the file's rows are
// loaded into before the set-based update/insert/delete statements.
const syncTempTable = "content_sync"

// syncJSONBlock makes block.Table's rows in scope match block.Rows, in one
// transaction. The file's rows are staged in a temporary table created
// from the target's own column types, then:
//
//	UPDATE target FROM staging   -- key match, some column differs
//	INSERT INTO target           -- staging rows with no key match
//	DELETE FROM target           -- in scope, no key match in staging
//
// Key columns are compared with IS NOT DISTINCT FROM so a NULL key part
// still matches itself.
func syncJSONBlock(db *sqlx.DB, block seedJSONBlock) (ContentResult, error) {
	if err := validateContentBlock(&block); err != nil {
		return ContentResult{}, err
	}
	res := ContentResult{Table: block.Table}

	tx, err := db.Beginx()
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()

	scopeWhere, scopeArgs := scopeCondition(block.Scope, "t")

	if len(block.Rows) == 0 {
		// Nothing to stage; the file only says "this scope is empty".
		if block.Scope != nil {
			r, err := tx.Exec(`DELETE FROM `+block.Table+` t WHERE `+scopeWhere, scopeArgs...)
			if err != nil {
				return res, fmt.Errorf("deleting: %w", err)
			}
			res.Deleted, _ = r.RowsAffected()
		}
		return res, tx.Commit()
	}

	columns := rowColumns(block.Rows[0])
	colList := strings.Join(columns, ", ")
	if _, err := tx.Exec(fmt.Sprintf(`CREATE TEMP TABLE %s ON COMMIT DROP AS SELECT %s FROM %s WHERE false`,
		syncTempTable, colList, block.Table)); err != nil {
		return res, fmt.Errorf("staging: %w", err)
	}
	staged := block
	staged.Table = syncTempTable
	query, args, err := buildSeedInsert(staged)
	if err != nil {
		return res, err
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return res, fmt.Errorf("staging: %w", err)
	}

	keyMatch := matchCondition(block.Key, "t", "c")
	var valueCols []string
	for _, col := range columns {
		if !slices.Contains(block.Key, col) {
			valueCols = append(valueCols, col)
		}
	}
	if len(valueCols) > 0 {
		var sets, differs []string
		for _, col := range valueCols {
			sets = append(sets, fmt.Sprintf("%s = c.%s", col, col))
			differs = append(differs, fmt.Sprintf("t.%s IS DISTINCT FROM c.%s", col, col))
		}
		r, err := tx.Exec(fmt.Sprintf(`UPDATE %s t SET %s FROM %s c WHERE %s AND (%s)`,
			block.Table, strings.Join(sets, ", "), syncTempTable, keyMatch, strings.Join(differs, " OR ")))
		if err != nil {
			return res, fmt.Errorf("updating: %w", err)
		}
		res.Updated, _ = r.RowsAffected()
	}

	r, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s c WHERE NOT EXISTS (SELECT 1 FROM %s t WHERE %s)`,
		block.Table, colList, colList, syncTempTable, block.Table, keyMatch))
	if err != nil {
		return res, fmt.Errorf("inserting: %w", err)
	}
	res.Inserted, _ = r.RowsAffected()

	if block.Scope != nil {
		r, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s t WHERE %s AND NOT EXISTS (SELECT 1 FROM %s c WHERE %s)`,
			block.Table, scopeWhere, syncTempTable, keyMatch), scopeArgs...)
		if err != nil {
			return res, fmt.Errorf("deleting: %w", err)
		}
		res.Deleted, _ = r.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// validateContentBlock enforces the content-file contract on top of the
// seed format and fills scalar scope columns into rows that leave them out.
func validateContentBlock(block *seedJSONBlock) error {
	if !identifierPattern.MatchString(block.Table) {
		return fmt.Errorf("invalid table name %q", block.Table)
	}
	if len(block.Key) == 0 {
		return fmt.Errorf(`content files must declare "key" (the columns that identify a row)`)
	}
	// "truncate" and "onConflict" are seed-time knobs; a content file's
	// scope and key say the same things, so they are ignored rather than
	// rejected and a seed file copies over unchanged.
	block.Truncate, block.OnConflict = false, ""
	for col, want := range block.Scope {
		if !identifierPattern.MatchString(col) {
			return fmt.Errorf("invalid scope column name %q", col)
		}
		if _, isRaw := want.(map[string]interface{}); isRaw {
			return fmt.Errorf("scope column %q cannot be a raw SQL value", col)
		}
		list, isList := want.([]interface{})
		if isList && len(list) == 0 {
			return fmt.Errorf("scope column %q: empty list", col)
		}
		for r, row := range block.Rows {
			got, present := row[col]
			if !present {
				if isList {
					return fmt.Errorf("row %d: missing scope column %q (a list scope cannot be filled in)", r, col)
				}
				row[col] = want
				continue
			}
			if !scopeContains(want, got) {
				return fmt.Errorf("row %d: %s=%v is outside the file's scope", r, col, got)
			}
		}
	}
	return block.validateKey()
}

// scopeContains reports whether got is the scope value want, or one of
// them when want is a list. Values are compared as JSON so 10 and 10.0 (the
// same JSON number) match.
func scopeContains(want, got interface{}) bool {
	if list, ok := want.([]interface{}); ok {
		for _, w := range list {
			if jsonEqual(w, got) {
				return true
			}
		}
		return false
	}
	return jsonEqual(want, got)
}

func jsonEqual(a, b interface{}) bool {
	ea, _ := json.Marshal(a)
	eb, _ := json.Marshal(b)
	return string(ea) == string(eb)
}

// scopeCondition renders scope as a WHERE fragment on alias, with bound
// arguments. An empty scope is "TRUE" (the whole table).
func scopeCondition(scope map[string]interface{}, alias string) (string, []interface{}) {
	if len(scope) == 0 {
		return "TRUE", nil
	}
	cols := make([]string, 0, len(scope))
	for col := range scope {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	var parts []string
	var args []interface{}
	for _, col := range cols {
		switch v := scope[col].(type) {
		case []interface{}:
			ph := make([]string, 0, len(v))
			for _, item := range v {
				args = append(args, normalizeSeedValue(item))
				ph = append(ph, fmt.Sprintf("$%d", len(args)))
			}
			parts = append(parts, fmt.Sprintf("%s.%s IN (%s)", alias, col, strings.Join(ph, ", ")))
		case nil:
			parts = append(parts, fmt.Sprintf("%s.%s IS NULL", alias, col))
		default:
			args = append(args, normalizeSeedValue(v))
			parts = append(parts, fmt.Sprintf("%s.%s = $%d", alias, col, len(args)))
		}
	}
	return strings.Join(parts, " AND "), args
}

// matchCondition renders the key equality between two aliases.
func matchCondition(key []string, a, b string) string {
	parts := make([]string, 0, len(key))
	for _, col := range key {
		parts = append(parts, fmt.Sprintf("%s.%s IS NOT DISTINCT FROM %s.%s", a, col, b, col))
	}
	return strings.Join(parts, " AND ")
}
