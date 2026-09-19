package api

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
)

// mockAPIUserRepo implements APIUserRepo for testing.
type mockAPIUserRepo struct {
	registerID     uint32
	registerRights uint32
	registerErr    error

	credentialsID       uint32
	credentialsPassword string
	credentialsRights   uint32
	credentialsErr      error

	lastLogin    time.Time
	lastLoginErr error

	returnExpiry    time.Time
	returnExpiryErr error

	updateReturnExpiryErr error
	updateLastLoginErr    error
}

func (m *mockAPIUserRepo) Register(_ context.Context, _, _ string, _ time.Time) (uint32, uint32, error) {
	return m.registerID, m.registerRights, m.registerErr
}

func (m *mockAPIUserRepo) GetCredentials(_ context.Context, _ string) (uint32, string, uint32, error) {
	return m.credentialsID, m.credentialsPassword, m.credentialsRights, m.credentialsErr
}

func (m *mockAPIUserRepo) GetLastLogin(_ uint32) (time.Time, error) {
	return m.lastLogin, m.lastLoginErr
}

func (m *mockAPIUserRepo) GetReturnExpiry(_ uint32) (time.Time, error) {
	return m.returnExpiry, m.returnExpiryErr
}

func (m *mockAPIUserRepo) UpdateReturnExpiry(_ uint32, _ time.Time) error {
	return m.updateReturnExpiryErr
}

func (m *mockAPIUserRepo) UpdateLastLogin(_ uint32, _ time.Time) error {
	return m.updateLastLoginErr
}

// mockAPICharacterRepo implements APICharacterRepo for testing.
type mockAPICharacterRepo struct {
	newCharacter    Character
	newCharacterErr error

	countForUser    int
	countForUserErr error

	createChar    Character
	createCharErr error

	isNewResult bool
	isNewErr    error

	hardDeleteErr error
	softDeleteErr error

	characters    []Character
	charactersErr error

	exportResult map[string]interface{}
	exportErr    error

	grantImportTokenErr  error
	revokeImportTokenErr error
	importSaveErr        error
}

func (m *mockAPICharacterRepo) GetNewCharacter(_ context.Context, _ uint32) (Character, error) {
	return m.newCharacter, m.newCharacterErr
}

func (m *mockAPICharacterRepo) CountForUser(_ context.Context, _ uint32) (int, error) {
	return m.countForUser, m.countForUserErr
}

func (m *mockAPICharacterRepo) Create(_ context.Context, _ uint32, _ uint32) (Character, error) {
	return m.createChar, m.createCharErr
}

func (m *mockAPICharacterRepo) IsNew(_ uint32) (bool, error) {
	return m.isNewResult, m.isNewErr
}

func (m *mockAPICharacterRepo) HardDelete(_ uint32) error {
	return m.hardDeleteErr
}

func (m *mockAPICharacterRepo) SoftDelete(_ uint32) error {
	return m.softDeleteErr
}

func (m *mockAPICharacterRepo) GetForUser(_ context.Context, _ uint32) ([]Character, error) {
	return m.characters, m.charactersErr
}

func (m *mockAPICharacterRepo) ExportSave(_ context.Context, _, _ uint32) (map[string]interface{}, error) {
	return m.exportResult, m.exportErr
}

func (m *mockAPICharacterRepo) GrantImportToken(_ context.Context, _, _ uint32, _ string, _ time.Time) error {
	return m.grantImportTokenErr
}

func (m *mockAPICharacterRepo) RevokeImportToken(_ context.Context, _, _ uint32) error {
	return m.revokeImportTokenErr
}

func (m *mockAPICharacterRepo) ImportSave(_ context.Context, _, _ uint32, _ string, _ SaveBlobs) error {
	return m.importSaveErr
}

// mockAPIEventRepo implements APIEventRepo for testing.
type mockAPIEventRepo struct {
	featureWeapon    *FeatureWeaponRow
	featureWeaponErr error

	events    []EventRow
	eventsErr error
}

func (m *mockAPIEventRepo) GetFeatureWeapon(_ context.Context, _ time.Time) (*FeatureWeaponRow, error) {
	return m.featureWeapon, m.featureWeaponErr
}

func (m *mockAPIEventRepo) GetActiveEvents(_ context.Context, _ string) ([]EventRow, error) {
	return m.events, m.eventsErr
}

// mockAPISessionRepo implements APISessionRepo for testing.
type mockAPISessionRepo struct {
	createTokenID  uint32
	createTokenErr error

	userID    uint32
	userIDErr error
}

func (m *mockAPISessionRepo) CreateToken(_ context.Context, _ uint32, _ string) (uint32, error) {
	return m.createTokenID, m.createTokenErr
}

func (m *mockAPISessionRepo) GetUserIDByToken(_ context.Context, _ string) (uint32, error) {
	return m.userID, m.userIDErr
}

// --- mockAPIAdminRepo ---

// mockAPIAdminRepo is an in-memory APIAdminRepo. Users are keyed by ID;
// notices, distributions and events are appended with incrementing IDs.
type mockAPIAdminRepo struct {
	users     map[uint32]*AdminUserRow
	notices   []NoticeRow
	dists     []DistributionRow
	distItems map[int][]DistributionItemRow
	events    []AdminEventRow
	nextID    int
	passwords map[uint32]string
	failWith  error // when set, every call returns this error
}

func newMockAdminRepo() *mockAPIAdminRepo {
	return &mockAPIAdminRepo{
		users:     map[uint32]*AdminUserRow{},
		distItems: map[int][]DistributionItemRow{},
		passwords: map[uint32]string{},
		nextID:    1,
	}
}

func (m *mockAPIAdminRepo) id() int { m.nextID++; return m.nextID - 1 }

func (m *mockAPIAdminRepo) IsOp(_ context.Context, userID uint32) (bool, error) {
	if m.failWith != nil {
		return false, m.failWith
	}
	u, ok := m.users[userID]
	if !ok {
		return false, sql.ErrNoRows
	}
	return u.Op, nil
}

func (m *mockAPIAdminRepo) SearchUsers(_ context.Context, query string, limit int) ([]AdminUserRow, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	out := []AdminUserRow{}
	for _, u := range m.users {
		if query == "" || strings.Contains(strings.ToLower(u.Username), strings.ToLower(query)) {
			out = append(out, *u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *mockAPIAdminRepo) GetUser(_ context.Context, userID uint32) (AdminUserRow, error) {
	if m.failWith != nil {
		return AdminUserRow{}, m.failWith
	}
	u, ok := m.users[userID]
	if !ok {
		return AdminUserRow{}, sql.ErrNoRows
	}
	return *u, nil
}

func (m *mockAPIAdminRepo) SetOp(_ context.Context, userID uint32, op bool) error {
	u, ok := m.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	u.Op = op
	return nil
}

func (m *mockAPIAdminRepo) SetRights(_ context.Context, userID uint32, rights uint32) error {
	u, ok := m.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	u.Rights = rights
	return nil
}

func (m *mockAPIAdminRepo) SetPasswordHash(_ context.Context, userID uint32, hash string) error {
	if _, ok := m.users[userID]; !ok {
		return sql.ErrNoRows
	}
	m.passwords[userID] = hash
	return nil
}

func (m *mockAPIAdminRepo) Ban(_ context.Context, userID uint32, expires *time.Time) error {
	u, ok := m.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	u.Banned = true
	u.BanExpires = expires
	return nil
}

func (m *mockAPIAdminRepo) Unban(_ context.Context, userID uint32) error {
	if u, ok := m.users[userID]; ok {
		u.Banned = false
		u.BanExpires = nil
	}
	return nil
}

func (m *mockAPIAdminRepo) ListNotices(_ context.Context, activeOnly bool) ([]NoticeRow, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	out := []NoticeRow{}
	for _, n := range m.notices {
		if activeOnly && n.ExpiresAt != nil && n.ExpiresAt.Before(time.Now()) {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

func (m *mockAPIAdminRepo) CreateNotice(_ context.Context, body string, expires *time.Time) (NoticeRow, error) {
	if m.failWith != nil {
		return NoticeRow{}, m.failWith
	}
	n := NoticeRow{ID: m.id(), Body: body, CreatedAt: time.Now(), ExpiresAt: expires}
	m.notices = append(m.notices, n)
	return n, nil
}

func (m *mockAPIAdminRepo) DeleteNotice(_ context.Context, id int) error {
	for i, n := range m.notices {
		if n.ID == id {
			m.notices = append(m.notices[:i], m.notices[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockAPIAdminRepo) ListDistributions(_ context.Context) ([]DistributionRow, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	return append([]DistributionRow{}, m.dists...), nil
}

func (m *mockAPIAdminRepo) GetDistributionItems(_ context.Context, id int) ([]DistributionItemRow, error) {
	return append([]DistributionItemRow{}, m.distItems[id]...), nil
}

func (m *mockAPIAdminRepo) CreateDistribution(_ context.Context, d DistributionRow, items []DistributionItemRow) (int, error) {
	if m.failWith != nil {
		return 0, m.failWith
	}
	d.ID = m.id()
	m.dists = append(m.dists, d)
	m.distItems[d.ID] = items
	return d.ID, nil
}

func (m *mockAPIAdminRepo) DeleteDistribution(_ context.Context, id int) error {
	for i, d := range m.dists {
		if d.ID == id {
			m.dists = append(m.dists[:i], m.dists[i+1:]...)
			delete(m.distItems, id)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockAPIAdminRepo) ListEvents(_ context.Context) ([]AdminEventRow, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	return append([]AdminEventRow{}, m.events...), nil
}

func (m *mockAPIAdminRepo) ReplaceEvent(_ context.Context, eventType string, start time.Time) (AdminEventRow, error) {
	if m.failWith != nil {
		return AdminEventRow{}, m.failWith
	}
	kept := m.events[:0]
	for _, e := range m.events {
		if e.EventType != eventType {
			kept = append(kept, e)
		}
	}
	row := AdminEventRow{ID: m.id(), EventType: eventType, StartTime: start}
	m.events = append(kept, row)
	return row, nil
}

func (m *mockAPIAdminRepo) DeleteEvents(_ context.Context, eventType string) (int64, error) {
	var n int64
	kept := m.events[:0]
	for _, e := range m.events {
		if e.EventType == eventType {
			n++
		} else {
			kept = append(kept, e)
		}
	}
	m.events = kept
	return n, nil
}
