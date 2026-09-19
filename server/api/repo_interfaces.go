package api

import (
	"context"
	"time"
)

// Repository interfaces decouple API server business logic from concrete
// PostgreSQL implementations, enabling mock/stub injection for unit tests.

// SaveBlobs holds the transferable save data columns for a character.
// SavedataHash must be set by the caller (SHA-256 of decompressed Savedata).
type SaveBlobs struct {
	Savedata          []byte
	SavedataHash      []byte
	Decomyset         []byte
	Hunternavi        []byte
	Otomoairou        []byte
	Partner           []byte
	Platebox          []byte
	Platedata         []byte
	Platemyset        []byte
	Rengokudata       []byte
	Savemercenary     []byte
	GachaItems        []byte
	HouseInfo         []byte
	LoginBoost        []byte
	SkinHist          []byte
	Scenariodata      []byte
	Savefavoritequest []byte
	Mezfes            []byte
}

// APIUserRepo defines the contract for user-related data access.
type APIUserRepo interface {
	// Register creates a new user and returns their ID and rights.
	Register(ctx context.Context, username, passwordHash string, returnExpires time.Time) (id uint32, rights uint32, err error)
	// GetCredentials returns the user's ID, password hash, and rights.
	GetCredentials(ctx context.Context, username string) (id uint32, passwordHash string, rights uint32, err error)
	// GetLastLogin returns the user's last login time.
	GetLastLogin(uid uint32) (time.Time, error)
	// GetReturnExpiry returns the user's return expiry time.
	GetReturnExpiry(uid uint32) (time.Time, error)
	// UpdateReturnExpiry sets the user's return expiry time.
	UpdateReturnExpiry(uid uint32, expiry time.Time) error
	// UpdateLastLogin sets the user's last login time.
	UpdateLastLogin(uid uint32, loginTime time.Time) error
}

// APICharacterRepo defines the contract for character-related data access.
type APICharacterRepo interface {
	// GetNewCharacter returns an existing new (unfinished) character for a user.
	GetNewCharacter(ctx context.Context, userID uint32) (Character, error)
	// CountForUser returns the total number of characters for a user.
	CountForUser(ctx context.Context, userID uint32) (int, error)
	// Create inserts a new character and returns it.
	Create(ctx context.Context, userID uint32, lastLogin uint32) (Character, error)
	// IsNew returns whether a character is a new (unfinished) character.
	IsNew(charID uint32) (bool, error)
	// HardDelete permanently removes a character.
	HardDelete(charID uint32) error
	// SoftDelete marks a character as deleted.
	SoftDelete(charID uint32) error
	// GetForUser returns all finalized (non-deleted) characters for a user.
	GetForUser(ctx context.Context, userID uint32) ([]Character, error)
	// ExportSave returns the full character row as a map.
	ExportSave(ctx context.Context, userID, charID uint32) (map[string]interface{}, error)
	// GrantImportToken sets a one-time import token for a character owned by userID.
	GrantImportToken(ctx context.Context, charID, userID uint32, token string, expiry time.Time) error
	// RevokeImportToken clears any pending import token for a character owned by userID.
	RevokeImportToken(ctx context.Context, charID, userID uint32) error
	// ImportSave atomically validates+consumes the import token and writes all save blobs.
	// Returns an error if the token is invalid, expired, or the character doesn't belong to userID.
	ImportSave(ctx context.Context, charID, userID uint32, token string, blobs SaveBlobs) error
}

// APIEventRepo defines the contract for read-only event data access.
type APIEventRepo interface {
	// GetFeatureWeapon returns the feature weapon entry for the given week start time.
	GetFeatureWeapon(ctx context.Context, startTime time.Time) (*FeatureWeaponRow, error)
	// GetActiveEvents returns all events of the given type.
	GetActiveEvents(ctx context.Context, eventType string) ([]EventRow, error)
}

// FeatureWeaponRow holds a single feature_weapon table row.
type FeatureWeaponRow struct {
	StartTime      time.Time `db:"start_time"`
	ActiveFeatures uint32    `db:"featured"`
}

// EventRow holds a single events table row with epoch start time.
type EventRow struct {
	ID        int   `db:"id"`
	StartTime int64 `db:"start_time"`
}

// APISessionRepo defines the contract for session/token data access.
type APISessionRepo interface {
	// CreateToken inserts a new sign session and returns its ID and token.
	CreateToken(ctx context.Context, uid uint32, token string) (tokenID uint32, err error)
	// GetUserIDByToken returns the user ID for a given session token.
	GetUserIDByToken(ctx context.Context, token string) (uint32, error)
}

// AdminUserRow is the operator-facing view of a users row.
type AdminUserRow struct {
	ID        uint32     `db:"id" json:"id"`
	Username  string     `db:"username" json:"username"`
	Rights    uint32     `db:"rights" json:"rights"`
	Op        bool       `db:"op" json:"op"`
	LastLogin *time.Time `db:"last_login" json:"lastLogin"`
	DiscordID *string    `db:"discord_id" json:"discordId"`
	PSNID     *string    `db:"psn_id" json:"psnId"`
	// BanExpires is nil when the user is not banned; a zero time means a
	// permanent ban (bans.expires IS NULL).
	BanExpires *time.Time `db:"ban_expires" json:"banExpires"`
	Banned     bool       `db:"banned" json:"banned"`
}

// NoticeRow is a runtime login notice.
type NoticeRow struct {
	ID        int        `db:"id" json:"id"`
	Body      string     `db:"body" json:"body"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
	ExpiresAt *time.Time `db:"expires_at" json:"expiresAt"`
}

// DistributionRow is a gift-box distribution header.
type DistributionRow struct {
	ID              int        `db:"id" json:"id"`
	CharacterID     *uint32    `db:"character_id" json:"characterId"`
	Type            int        `db:"type" json:"type"`
	Deadline        *time.Time `db:"deadline" json:"deadline"`
	EventName       string     `db:"event_name" json:"eventName"`
	Description     string     `db:"description" json:"description"`
	TimesAcceptable int        `db:"times_acceptable" json:"timesAcceptable"`
	MinHR           *int       `db:"min_hr" json:"minHr"`
	MaxHR           *int       `db:"max_hr" json:"maxHr"`
	MinSR           *int       `db:"min_sr" json:"minSr"`
	MaxSR           *int       `db:"max_sr" json:"maxSr"`
	MinGR           *int       `db:"min_gr" json:"minGr"`
	MaxGR           *int       `db:"max_gr" json:"maxGr"`
	Rights          *int       `db:"rights" json:"rights"`
	Selection       *bool      `db:"selection" json:"selection"`
}

// DistributionItemRow is one item inside a distribution.
type DistributionItemRow struct {
	ID       int  `db:"id" json:"id,omitempty"`
	ItemType int  `db:"item_type" json:"itemType"`
	ItemID   *int `db:"item_id" json:"itemId"`
	Quantity *int `db:"quantity" json:"quantity"`
}

// AdminEventRow is an events table row with its type.
type AdminEventRow struct {
	ID        int       `db:"id" json:"id"`
	EventType string    `db:"event_type" json:"eventType"`
	StartTime time.Time `db:"start_time" json:"startTime"`
}

// APIAdminRepo is the data access used by the operator endpoints under
// /v2/admin. Everything here is gated by AdminMiddleware.
type APIAdminRepo interface {
	IsOp(ctx context.Context, userID uint32) (bool, error)

	SearchUsers(ctx context.Context, query string, limit int) ([]AdminUserRow, error)
	GetUser(ctx context.Context, userID uint32) (AdminUserRow, error)
	SetOp(ctx context.Context, userID uint32, op bool) error
	SetRights(ctx context.Context, userID uint32, rights uint32) error
	SetPasswordHash(ctx context.Context, userID uint32, hash string) error
	// Ban with a nil expiry is permanent; an existing ban is replaced.
	Ban(ctx context.Context, userID uint32, expires *time.Time) error
	Unban(ctx context.Context, userID uint32) error

	ListNotices(ctx context.Context, activeOnly bool) ([]NoticeRow, error)
	CreateNotice(ctx context.Context, body string, expires *time.Time) (NoticeRow, error)
	DeleteNotice(ctx context.Context, id int) error

	ListDistributions(ctx context.Context) ([]DistributionRow, error)
	GetDistributionItems(ctx context.Context, distributionID int) ([]DistributionItemRow, error)
	CreateDistribution(ctx context.Context, d DistributionRow, items []DistributionItemRow) (int, error)
	DeleteDistribution(ctx context.Context, id int) error

	ListEvents(ctx context.Context) ([]AdminEventRow, error)
	// ReplaceEvent removes every row of eventType and inserts one starting at start.
	ReplaceEvent(ctx context.Context, eventType string, start time.Time) (AdminEventRow, error)
	DeleteEvents(ctx context.Context, eventType string) (int64, error)
}
