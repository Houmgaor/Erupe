package migrations

import (
	"testing"
)

// TestDedupeShopItemsMigration applies every migration before 0028, seeds the
// duplicate road-shop rows an upgraded server carries, then runs 0028: exact
// copies collapse onto the lowest id with their purchase counters summed,
// while a row that differs in any column is kept.
func TestDedupeShopItemsMigration(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()

	if err := ensureVersionTable(db); err != nil {
		t.Fatal(err)
	}
	all, err := readMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var dedupe *migration
	for i := range all {
		if all[i].version == 28 {
			dedupe = &all[i]
			break
		}
		if err := applyMigration(db, all[i]); err != nil {
			t.Fatalf("applying %s: %v", all[i].filename, err)
		}
	}
	if dedupe == nil {
		t.Fatal("migration 0028 not found")
	}

	insert := `INSERT INTO shop_items (id, shop_type, shop_id, item_id, cost, quantity,
		min_hr, min_sr, min_gr, store_level, max_quantity, road_floors, road_fatalis)
		VALUES ($1, $2, $3, $4, $5, 1, 0, 0, 1, 1, 0, 100, 0)`
	for _, r := range []struct{ id, shopType, shopID, item, cost int }{
		{101, 10, 7, 10750, 3000}, // kept: lowest id of an exact triple
		{140, 10, 7, 10750, 3000},
		{150, 10, 7, 10750, 3000},
		{102, 10, 7, 13508, 3000}, // same key, different cost: not a copy
		{141, 10, 7, 13508, 2500},
		{103, 10, 8, 9958, 20}, // unique row
	} {
		if _, err := db.Exec(insert, r.id, r.shopType, r.shopID, r.item, r.cost); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []struct{ char, item, bought int }{
		{1, 140, 2}, // char 1 bought through two copies: counters add up
		{1, 150, 3},
		{2, 101, 4}, // char 2 already on the kept row, plus a copy
		{2, 150, 1},
		{3, 141, 5}, // the differing row keeps its own counter
	} {
		if _, err := db.Exec(`INSERT INTO shop_items_bought (character_id, shop_item_id, bought)
			VALUES ($1, $2, $3)`, b.char, b.item, b.bought); err != nil {
			t.Fatal(err)
		}
	}

	if err := applyMigration(db, *dedupe); err != nil {
		t.Fatalf("applying %s: %v", dedupe.filename, err)
	}

	var ids []int
	if err := db.Select(&ids, `SELECT id FROM shop_items ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	want := []int{101, 102, 103, 141}
	if len(ids) != len(want) {
		t.Fatalf("shop_items ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("shop_items ids = %v, want %v", ids, want)
		}
	}

	bought := map[[2]int]int{}
	rows, err := db.Query(`SELECT character_id, shop_item_id, bought FROM shop_items_bought`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c, s, n int
		if err := rows.Scan(&c, &s, &n); err != nil {
			t.Fatal(err)
		}
		bought[[2]int{c, s}] = n
	}
	wantBought := map[[2]int]int{{1, 101}: 5, {2, 101}: 5, {3, 141}: 5}
	if len(bought) != len(wantBought) {
		t.Fatalf("shop_items_bought = %v, want %v", bought, wantBought)
	}
	for k, v := range wantBought {
		if bought[k] != v {
			t.Errorf("bought[char %d, item %d] = %d, want %d", k[0], k[1], bought[k], v)
		}
	}
}
