package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// apiAdminRepository is the PostgreSQL implementation of APIAdminRepo.
type apiAdminRepository struct {
	db *sqlx.DB
}

// NewAPIAdminRepository creates a new admin repository.
func NewAPIAdminRepository(db *sqlx.DB) APIAdminRepo {
	return &apiAdminRepository{db: db}
}

// adminUserSelect is the projection shared by SearchUsers and GetUser. The
// bans join folds the two ban states the sign server distinguishes
// (expires IS NULL = permanent, expires > now() = temporary) into one flag.
const adminUserSelect = `
SELECT u.id, u.username, u.rights, COALESCE(u.op, false) AS op, u.last_login,
       u.discord_id, u.psn_id,
       b.expires AS ban_expires,
       (b.user_id IS NOT NULL AND (b.expires IS NULL OR b.expires > now())) AS banned
  FROM users u
  LEFT JOIN bans b ON b.user_id = u.id`

func (r *apiAdminRepository) IsOp(ctx context.Context, userID uint32) (bool, error) {
	var op sql.NullBool
	err := r.db.QueryRowContext(ctx, `SELECT op FROM users WHERE id=$1`, userID).Scan(&op)
	if err != nil {
		return false, err
	}
	return op.Valid && op.Bool, nil
}

func (r *apiAdminRepository) SearchUsers(ctx context.Context, query string, limit int) ([]AdminUserRow, error) {
	rows := []AdminUserRow{}
	err := r.db.SelectContext(ctx, &rows, adminUserSelect+`
		WHERE ($1 = '' OR u.username ILIKE '%' || $1 || '%')
		ORDER BY u.id
		LIMIT $2`, query, limit)
	return rows, err
}

func (r *apiAdminRepository) GetUser(ctx context.Context, userID uint32) (AdminUserRow, error) {
	var row AdminUserRow
	err := r.db.GetContext(ctx, &row, adminUserSelect+` WHERE u.id=$1`, userID)
	return row, err
}

func (r *apiAdminRepository) SetOp(ctx context.Context, userID uint32, op bool) error {
	return r.updateUser(ctx, `UPDATE users SET op=$1 WHERE id=$2`, op, userID)
}

func (r *apiAdminRepository) SetRights(ctx context.Context, userID uint32, rights uint32) error {
	return r.updateUser(ctx, `UPDATE users SET rights=$1 WHERE id=$2`, rights, userID)
}

func (r *apiAdminRepository) SetPasswordHash(ctx context.Context, userID uint32, hash string) error {
	return r.updateUser(ctx, `UPDATE users SET password=$1 WHERE id=$2`, hash, userID)
}

// updateUser runs a single-row UPDATE and reports a missing user as
// sql.ErrNoRows so handlers can answer 404 instead of a silent no-op.
func (r *apiAdminRepository) updateUser(ctx context.Context, q string, args ...interface{}) error {
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *apiAdminRepository) Ban(ctx context.Context, userID uint32, expires *time.Time) error {
	// Same statements as the in-game !ban command (channelserver.UserRepository.BanUser).
	if expires == nil {
		_, err := r.db.ExecContext(ctx, `INSERT INTO bans (user_id, expires) VALUES ($1, NULL)
			ON CONFLICT (user_id) DO UPDATE SET expires=NULL`, userID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO bans (user_id, expires) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET expires=$2`, userID, *expires)
	return err
}

func (r *apiAdminRepository) Unban(ctx context.Context, userID uint32) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bans WHERE user_id=$1`, userID)
	return err
}

func (r *apiAdminRepository) ListNotices(ctx context.Context, activeOnly bool) ([]NoticeRow, error) {
	rows := []NoticeRow{}
	q := `SELECT id, body, created_at, expires_at FROM notices`
	if activeOnly {
		q += ` WHERE expires_at IS NULL OR expires_at > now()`
	}
	err := r.db.SelectContext(ctx, &rows, q+` ORDER BY id`)
	return rows, err
}

func (r *apiAdminRepository) CreateNotice(ctx context.Context, body string, expires *time.Time) (NoticeRow, error) {
	var row NoticeRow
	err := r.db.GetContext(ctx, &row,
		`INSERT INTO notices (body, expires_at) VALUES ($1, $2)
		 RETURNING id, body, created_at, expires_at`, body, expires)
	return row, err
}

func (r *apiAdminRepository) DeleteNotice(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM notices WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

const distributionColumns = `id, character_id, type, deadline, event_name, description, times_acceptable,
	min_hr, max_hr, min_sr, max_sr, min_gr, max_gr, rights, selection`

func (r *apiAdminRepository) ListDistributions(ctx context.Context) ([]DistributionRow, error) {
	rows := []DistributionRow{}
	err := r.db.SelectContext(ctx, &rows, `SELECT `+distributionColumns+` FROM distribution ORDER BY id`)
	return rows, err
}

func (r *apiAdminRepository) GetDistributionItems(ctx context.Context, distributionID int) ([]DistributionItemRow, error) {
	rows := []DistributionItemRow{}
	err := r.db.SelectContext(ctx, &rows,
		`SELECT id, item_type, item_id, quantity FROM distribution_items WHERE distribution_id=$1 ORDER BY id`,
		distributionID)
	return rows, err
}

func (r *apiAdminRepository) CreateDistribution(ctx context.Context, d DistributionRow, items []DistributionItemRow) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int
	err = tx.QueryRowContext(ctx, `INSERT INTO distribution
		(character_id, type, deadline, event_name, description, times_acceptable,
		 min_hr, max_hr, min_sr, max_sr, min_gr, max_gr, rights, selection)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		d.CharacterID, d.Type, d.Deadline, d.EventName, d.Description, d.TimesAcceptable,
		d.MinHR, d.MaxHR, d.MinSR, d.MaxSR, d.MinGR, d.MaxGR, d.Rights, d.Selection).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO distribution_items (distribution_id, item_type, item_id, quantity) VALUES ($1,$2,$3,$4)`,
			id, it.ItemType, it.ItemID, it.Quantity); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *apiAdminRepository) DeleteDistribution(ctx context.Context, id int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM distribution_items WHERE distribution_id=$1`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM distribution WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (r *apiAdminRepository) ListEvents(ctx context.Context) ([]AdminEventRow, error) {
	rows := []AdminEventRow{}
	err := r.db.SelectContext(ctx, &rows, `SELECT id, event_type, start_time FROM events ORDER BY event_type, id`)
	return rows, err
}

func (r *apiAdminRepository) ReplaceEvent(ctx context.Context, eventType string, start time.Time) (AdminEventRow, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return AdminEventRow{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Same shape as channelserver's Diva/Festa repositories: the game reads
	// the last row of each type, so a single fresh row is the whole state.
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE event_type=$1`, eventType); err != nil {
		return AdminEventRow{}, err
	}
	var row AdminEventRow
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO events (event_type, start_time) VALUES ($1, $2) RETURNING id, event_type, start_time`,
		eventType, start).StructScan(&row)
	if err != nil {
		return AdminEventRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return AdminEventRow{}, err
	}
	return row, nil
}

func (r *apiAdminRepository) DeleteEvents(ctx context.Context, eventType string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM events WHERE event_type=$1`, eventType)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// isNotFound reports whether err is the repository's "no such row" signal.
func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// validEventType mirrors the events.event_type enum in the schema.
func validEventType(t string) error {
	switch t {
	case "festa", "diva", "vs", "mezfes":
		return nil
	}
	return fmt.Errorf("unknown event type %q (festa, diva, vs, mezfes)", t)
}
