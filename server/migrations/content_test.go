package migrations

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestValidateContentBlock_RequiresKey(t *testing.T) {
	block := seedJSONBlock{Table: "shop_items", Rows: []map[string]interface{}{{"item_id": 1.0}}}
	if err := validateContentBlock(&block); err == nil || !strings.Contains(err.Error(), `"key"`) {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestValidateContentBlock_IgnoresSeedOnlyFields(t *testing.T) {
	block := seedJSONBlock{Table: "t", Key: []string{"id"}, Truncate: true, OnConflict: "DO NOTHING"}
	if err := validateContentBlock(&block); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.Truncate || block.OnConflict != "" {
		t.Errorf("seed-only fields not cleared: %+v", block)
	}
}

func TestValidateContentBlock_ScopeFillAndReject(t *testing.T) {
	block := seedJSONBlock{
		Table: "shop_items",
		Key:   []string{"shop_type", "shop_id", "item_id"},
		Scope: map[string]interface{}{"shop_type": 10.0},
		Rows: []map[string]interface{}{
			{"shop_id": 4.0, "item_id": 1.0},
			{"shop_type": 10.0, "shop_id": 4.0, "item_id": 2.0},
		},
	}
	if err := validateContentBlock(&block); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.Rows[0]["shop_type"] != 10.0 {
		t.Errorf("scope column not filled in: %v", block.Rows[0])
	}

	block.Rows = append(block.Rows, map[string]interface{}{"shop_type": 8.0, "shop_id": 4.0, "item_id": 3.0})
	err := validateContentBlock(&block)
	if err == nil || !strings.Contains(err.Error(), "outside the file's scope") {
		t.Fatalf("expected out-of-scope error, got %v", err)
	}
}

func TestValidateContentBlock_ListScope(t *testing.T) {
	block := seedJSONBlock{
		Table: "shop_items",
		Key:   []string{"shop_type", "item_id"},
		Scope: map[string]interface{}{"shop_type": []interface{}{5.0, 6.0}},
		Rows:  []map[string]interface{}{{"shop_type": 6.0, "item_id": 1.0}},
	}
	if err := validateContentBlock(&block); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	block.Rows = append(block.Rows, map[string]interface{}{"item_id": 2.0})
	if err := validateContentBlock(&block); err == nil || !strings.Contains(err.Error(), "list scope") {
		t.Fatalf("expected fill-in refusal for list scope, got %v", err)
	}
}

func TestValidateKey_Duplicates(t *testing.T) {
	block := seedJSONBlock{
		Key: []string{"a", "b"},
		Rows: []map[string]interface{}{
			{"a": 1.0, "b": "x"},
			{"a": 1.0, "b": "y"},
			{"a": 1.0, "b": "x"},
		},
	}
	err := block.validateKey()
	if err == nil || !strings.Contains(err.Error(), "rows 0 and 2") {
		t.Fatalf("expected duplicate-key error naming rows 0 and 2, got %v", err)
	}
	block.Rows[2]["b"] = map[string]interface{}{"raw": "NOW()"}
	if err := block.validateKey(); err == nil {
		t.Error("expected error for raw value in key column")
	}
	delete(block.Rows[2], "b")
	if err := block.validateKey(); err == nil {
		t.Error("expected error for missing key column")
	}
}

func TestScopeCondition(t *testing.T) {
	where, args := scopeCondition(nil, "t")
	if where != "TRUE" || len(args) != 0 {
		t.Errorf("empty scope: %q %v", where, args)
	}
	where, args = scopeCondition(map[string]interface{}{
		"shop_type": 10.0,
		"shop_id":   []interface{}{4.0, 7.0},
		"note":      nil,
	}, "t")
	want := "t.note IS NULL AND t.shop_id IN ($1, $2) AND t.shop_type = $3"
	if where != want {
		t.Errorf("where = %q, want %q", where, want)
	}
	if len(args) != 3 || args[0] != int64(4) || args[1] != int64(7) || args[2] != int64(10) {
		t.Errorf("args = %v", args)
	}
}

func TestMatchCondition(t *testing.T) {
	got := matchCondition([]string{"a", "b"}, "t", "c")
	want := "t.a IS NOT DISTINCT FROM c.a AND t.b IS NOT DISTINCT FROM c.b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyContentDir_Missing(t *testing.T) {
	_, err := ApplyContentDir(nil, zap.NewNop(), filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestApplyContentDir_Empty(t *testing.T) {
	res, err := ApplyContentDir(nil, zap.NewNop(), t.TempDir())
	if err != nil || len(res) != 0 {
		t.Fatalf("empty dir: res=%v err=%v", res, err)
	}
}

// TestSyncJSONBlock_Integration walks the whole lifecycle of a content file
// on the real shop_items table: first apply inserts, an edited file updates
// in place (ids survive), drops what it no longer lists, and leaves other
// scopes alone. A second identical apply is a no-op.
func TestSyncJSONBlock_Integration(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	logger := zap.NewNop()
	if _, err := Migrate(db, logger); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	// A row in another scope that must never be touched.
	if _, err := db.Exec(`INSERT INTO shop_items (shop_type, shop_id, item_id, cost, quantity) VALUES (8, 1, 500, 1, 1)`); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "shop_items"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "shop_items", "road.json")
	write := func(body string) {
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rowCount := func(where string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM shop_items WHERE ` + where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	write(`{
		"key": ["shop_type", "shop_id", "item_id"],
		"scope": {"shop_type": 10},
		"rows": [
			{"shop_id": 4, "item_id": 100, "cost": 1000, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0},
			{"shop_id": 4, "item_id": 101, "cost": 2000, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0},
			{"shop_id": 7, "item_id": 100, "cost": 3000, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0}
		]
	}`)
	res, err := ApplyContentDir(db, logger, dir)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if len(res) != 1 || res[0] != (ContentResult{Path: "shop_items/road.json", Table: "shop_items", Inserted: 3}) {
		t.Fatalf("first apply results = %+v", res)
	}
	var idBefore int
	if err := db.QueryRow(`SELECT id FROM shop_items WHERE shop_type=10 AND shop_id=4 AND item_id=101`).Scan(&idBefore); err != nil {
		t.Fatal(err)
	}

	// Same file again: nothing to do.
	res, err = ApplyContentDir(db, logger, dir)
	if err != nil || res[0].Inserted+res[0].Updated+res[0].Deleted != 0 {
		t.Fatalf("second apply should be a no-op: %+v %v", res, err)
	}

	// Reprice 101, drop 100 from tab 4, add 102.
	write(`{
		"key": ["shop_type", "shop_id", "item_id"],
		"scope": {"shop_type": 10},
		"rows": [
			{"shop_id": 4, "item_id": 101, "cost": 2500, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0},
			{"shop_id": 4, "item_id": 102, "cost": 4000, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0},
			{"shop_id": 7, "item_id": 100, "cost": 3000, "quantity": 1, "min_hr": 0, "min_sr": 0, "min_gr": 1, "store_level": 1, "max_quantity": 0, "road_floors": 0, "road_fatalis": 0}
		]
	}`)
	res, err = ApplyContentDir(db, logger, dir)
	if err != nil {
		t.Fatalf("third apply: %v", err)
	}
	if res[0].Inserted != 1 || res[0].Updated != 1 || res[0].Deleted != 1 {
		t.Fatalf("third apply results = %+v", res)
	}
	var idAfter, cost int
	if err := db.QueryRow(`SELECT id, cost FROM shop_items WHERE shop_type=10 AND shop_id=4 AND item_id=101`).Scan(&idAfter, &cost); err != nil {
		t.Fatal(err)
	}
	if idAfter != idBefore || cost != 2500 {
		t.Errorf("updated row: id %d -> %d, cost %d", idBefore, idAfter, cost)
	}
	if n := rowCount("shop_type=10"); n != 3 {
		t.Errorf("road rows = %d, want 3", n)
	}
	if n := rowCount("shop_type=8"); n != 1 {
		t.Errorf("row outside scope was touched: %d", n)
	}

	// Empty rows with a scope empties the scope.
	write(`{"key": ["shop_type", "shop_id", "item_id"], "scope": {"shop_type": 10}, "rows": []}`)
	res, err = ApplyContentDir(db, logger, dir)
	if err != nil || res[0].Deleted != 3 {
		t.Fatalf("empty apply: %+v %v", res, err)
	}
	if n := rowCount("shop_type=8"); n != 1 {
		t.Errorf("row outside scope was touched: %d", n)
	}

	// A broken file stops the run with its name in the error.
	write(`{"rows": [{"shop_type": 10}]}`)
	if _, err := ApplyContentDir(db, logger, dir); err == nil || !strings.Contains(err.Error(), "road.json") {
		t.Fatalf("expected error naming the file, got %v", err)
	}
}

// TestSyncJSONBlock_ShippedSeedsAreValidContent checks that every embedded
// seed file that declares a key can be dropped into a content directory
// as-is — that is the documented way to start customising a table.
func TestSyncJSONBlock_ShippedSeedsAreValidContent(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	logger := zap.NewNop()
	if _, err := Migrate(db, logger); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if _, err := ApplySeedData(db, logger); err != nil {
		t.Fatalf("ApplySeedData failed: %v", err)
	}

	dir := t.TempDir()
	keyed := 0
	err := fs.WalkDir(seedFS, "seed", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		data, err := fs.ReadFile(seedFS, path)
		if err != nil {
			return err
		}
		block, err := parseSeedJSONBlock(path, "", data)
		if err != nil {
			return err
		}
		if len(block.Key) == 0 {
			return nil
		}
		keyed++
		rel := strings.TrimPrefix(path, "seed/")
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if keyed == 0 {
		t.Fatal("no shipped seed declares a key")
	}

	// Seeds were just inserted, so the same files as content change nothing
	// (apart from NOW()-relative campaign rows, which re-evaluate).
	res, err := ApplyContentDir(db, logger, dir)
	if err != nil {
		t.Fatalf("ApplyContentDir: %v", err)
	}
	if len(res) != keyed {
		t.Fatalf("applied %d files, want %d", len(res), keyed)
	}
	for _, r := range res {
		if r.Inserted != 0 || r.Deleted != 0 {
			t.Errorf("%s: inserted %d deleted %d after seeding, want 0/0", r.Path, r.Inserted, r.Deleted)
		}
		if r.Updated != 0 && r.Table != "campaigns" {
			t.Errorf("%s: updated %d after seeding, want 0", r.Path, r.Updated)
		}
	}
}
