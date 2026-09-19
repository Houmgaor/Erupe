package api

import (
	"context"
	"os"
	"testing"
	"time"

	"erupe-ce/server/migrations"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// Runs only with ERUPE_TEST_DSN set: applies the real migrations and drives
// every APIAdminRepo method against PostgreSQL.
func TestAdminRepoAgainstPostgres(t *testing.T) {
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
	ctx := context.Background()
	repo := NewAPIAdminRepository(db)

	var uid uint32
	if err := db.QueryRow(`INSERT INTO users (username, password, rights) VALUES ('dbtest', 'x', 12) RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(`DELETE FROM bans WHERE user_id=$1; DELETE FROM users WHERE id=$1`, uid) }()

	if op, err := repo.IsOp(ctx, uid); err != nil || op {
		t.Fatalf("IsOp fresh user: %v %v", op, err)
	}
	if err := repo.SetOp(ctx, uid, true); err != nil {
		t.Fatal(err)
	}
	if op, _ := repo.IsOp(ctx, uid); !op {
		t.Fatal("SetOp not applied")
	}
	if err := repo.SetRights(ctx, uid, 62); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPasswordHash(ctx, uid, "hash"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetOp(ctx, 999999, true); !isNotFound(err) {
		t.Fatalf("SetOp unknown user: %v", err)
	}
	rows, err := repo.SearchUsers(ctx, "dbte", 10)
	if err != nil || len(rows) != 1 || rows[0].Rights != 62 || !rows[0].Op || rows[0].Banned {
		t.Fatalf("SearchUsers: %+v %v", rows, err)
	}
	exp := time.Now().Add(time.Hour)
	if err := repo.Ban(ctx, uid, &exp); err != nil {
		t.Fatal(err)
	}
	if u, _ := repo.GetUser(ctx, uid); !u.Banned || u.BanExpires == nil {
		t.Fatalf("temp ban: %+v", u)
	}
	if err := repo.Ban(ctx, uid, nil); err != nil {
		t.Fatal(err)
	}
	if u, _ := repo.GetUser(ctx, uid); !u.Banned || u.BanExpires != nil {
		t.Fatalf("perm ban: %+v", u)
	}
	if err := repo.Unban(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if u, _ := repo.GetUser(ctx, uid); u.Banned {
		t.Fatalf("unban: %+v", u)
	}

	n, err := repo.CreateNotice(ctx, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	old, _ := repo.CreateNotice(ctx, "old", &past)
	active, _ := repo.ListNotices(ctx, true)
	all, _ := repo.ListNotices(ctx, false)
	if len(active) != 1 || len(all) != 2 {
		t.Fatalf("notices: active=%d all=%d", len(active), len(all))
	}
	_ = repo.DeleteNotice(ctx, n.ID)
	_ = repo.DeleteNotice(ctx, old.ID)
	if err := repo.DeleteNotice(ctx, n.ID); !isNotFound(err) {
		t.Fatalf("delete twice: %v", err)
	}

	item, qty := 482, 3
	did, err := repo.CreateDistribution(ctx, DistributionRow{Type: 1, EventName: "T", Description: "d", TimesAcceptable: 1},
		[]DistributionItemRow{{ItemType: 7, ItemID: &item, Quantity: &qty}})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := repo.GetDistributionItems(ctx, did)
	if len(items) != 1 || *items[0].ItemID != 482 {
		t.Fatalf("items: %+v", items)
	}
	if err := repo.DeleteDistribution(ctx, did); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteDistribution(ctx, did); !isNotFound(err) {
		t.Fatalf("delete dist twice: %v", err)
	}

	start := time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)
	ev, err := repo.ReplaceEvent(ctx, "diva", start)
	if err != nil || !ev.StartTime.Equal(start) {
		t.Fatalf("ReplaceEvent: %+v %v", ev, err)
	}
	ev2, _ := repo.ReplaceEvent(ctx, "diva", start.Add(time.Hour))
	evs, _ := repo.ListEvents(ctx)
	divas := 0
	for _, e := range evs {
		if e.EventType == "diva" {
			divas++
		}
	}
	if divas != 1 || ev2.ID == ev.ID {
		t.Fatalf("ReplaceEvent left %d diva rows", divas)
	}
	if n, _ := repo.DeleteEvents(ctx, "diva"); n != 1 {
		t.Fatalf("DeleteEvents removed %d", n)
	}
}
