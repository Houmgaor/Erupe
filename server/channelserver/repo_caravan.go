package channelserver

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// CaravanRepository centralizes all database access for the caravan table
// and guilds.ryoudan_points.
type CaravanRepository struct {
	db *sqlx.DB
}

// NewCaravanRepository creates a new CaravanRepository.
func NewCaravanRepository(db *sqlx.DB) *CaravanRepository {
	return &CaravanRepository{db: db}
}

// CaravanPoints holds a character's caravan stats.
type CaravanPoints struct {
	Points    int32
	PvPPoints int32
	SkillID   int16
}

// CaravanRankEntry is one row of the personal caravan ranking.
type CaravanRankEntry struct {
	CharID uint32 `db:"char_id"`
	Name   string `db:"name"`
	Points int32  `db:"points"`
}

// CaravanGuildRankEntry is one row of the guild ("Ryoudan") caravan ranking.
type CaravanGuildRankEntry struct {
	GuildID uint32 `db:"id"`
	Name    string `db:"name"`
	Points  int32  `db:"ryoudan_points"`
}

// GetPoints returns a character's caravan stats, creating the row if it doesn't exist.
func (r *CaravanRepository) GetPoints(charID uint32) (CaravanPoints, error) {
	var cp CaravanPoints
	err := r.db.QueryRow(
		`SELECT points, pvp_points, skill_id FROM caravan WHERE char_id=$1`, charID,
	).Scan(&cp.Points, &cp.PvPPoints, &cp.SkillID)
	if err != nil {
		if _, insertErr := r.db.Exec(`INSERT INTO caravan (char_id) VALUES ($1) ON CONFLICT DO NOTHING`, charID); insertErr != nil {
			return CaravanPoints{}, fmt.Errorf("insert caravan: %w", insertErr)
		}
		return CaravanPoints{}, nil
	}
	return cp, nil
}

// AddPoints adds delta to a character's caravan points, creating the row if needed.
func (r *CaravanRepository) AddPoints(charID uint32, delta int32) error {
	if _, err := r.db.Exec(`INSERT INTO caravan (char_id) VALUES ($1) ON CONFLICT DO NOTHING`, charID); err != nil {
		return fmt.Errorf("insert caravan: %w", err)
	}
	if _, err := r.db.Exec(`UPDATE caravan SET points=points+$1 WHERE char_id=$2`, delta, charID); err != nil {
		return fmt.Errorf("update caravan points: %w", err)
	}
	return nil
}

// GetPersonalRanking returns all characters with caravan points, highest first.
func (r *CaravanRepository) GetPersonalRanking() ([]CaravanRankEntry, error) {
	var result []CaravanRankEntry
	err := r.db.Select(&result,
		`SELECT c.char_id, ch.name, c.points FROM caravan c
		 JOIN characters ch ON ch.id = c.char_id
		 WHERE c.points > 0
		 ORDER BY c.points DESC`,
	)
	return result, err
}

// GetGuildPoints returns a guild's aggregate ("Ryoudan") caravan points.
func (r *CaravanRepository) GetGuildPoints(guildID uint32) (int32, error) {
	var points int32
	err := r.db.QueryRow(`SELECT ryoudan_points FROM guilds WHERE id=$1`, guildID).Scan(&points)
	return points, err
}

// AddGuildPoints adds delta to a guild's aggregate caravan points.
func (r *CaravanRepository) AddGuildPoints(guildID uint32, delta int32) error {
	_, err := r.db.Exec(`UPDATE guilds SET ryoudan_points=ryoudan_points+$1 WHERE id=$2`, delta, guildID)
	return err
}

// GetGuildRanking returns all guilds with caravan points, highest first.
func (r *CaravanRepository) GetGuildRanking() ([]CaravanGuildRankEntry, error) {
	var result []CaravanGuildRankEntry
	err := r.db.Select(&result,
		`SELECT id, name, ryoudan_points FROM guilds WHERE ryoudan_points > 0 ORDER BY ryoudan_points DESC`,
	)
	return result, err
}
