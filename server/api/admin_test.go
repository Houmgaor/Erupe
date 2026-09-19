package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"erupe-ce/server/migrations"

	"golang.org/x/crypto/bcrypt"
)

// adminTestServer returns a server whose session token "op-token" resolves
// to user 1 (an operator) and whose admin repo holds users 1, 2 and 3.
func adminTestServer(t *testing.T) (*APIServer, *mockAPIAdminRepo) {
	t.Helper()
	repo := newMockAdminRepo()
	repo.users[1] = &AdminUserRow{ID: 1, Username: "admin", Rights: 30, Op: true}
	repo.users[2] = &AdminUserRow{ID: 2, Username: "hunter", Rights: 12}
	repo.users[3] = &AdminUserRow{ID: 3, Username: "other-hunter", Rights: 12}
	s := &APIServer{
		logger:      NewTestLogger(t),
		erupeConfig: NewTestConfig(),
		sessionRepo: &mockAPISessionRepo{userID: 1},
		userRepo:    &mockAPIUserRepo{lastLogin: time.Now(), returnExpiry: time.Now().Add(24 * time.Hour)},
		charRepo:    &mockAPICharacterRepo{characters: []Character{{ID: 10, Name: "Hunter"}}},
		adminRepo:   repo,
	}
	return s, repo
}

func adminRequest(t *testing.T, s *APIServer, method, target string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Authorization", "Bearer op-token")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	newTestRouter(s).ServeHTTP(rr, req)
	return rr
}

func decodeInto(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decoding %q: %v", rr.Body.String(), err)
	}
}

func TestAdminRequiresTokenAndOp(t *testing.T) {
	s, repo := adminTestServer(t)
	router := newTestRouter(s)

	// No token.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v2/admin/users", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rr.Code)
	}

	// Valid token, not an operator.
	s.sessionRepo = &mockAPISessionRepo{userID: 2}
	rr = adminRequest(t, s, http.MethodGet, "/v2/admin/users", nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-op: %d %s", rr.Code, rr.Body.String())
	}

	// Operator.
	s.sessionRepo = &mockAPISessionRepo{userID: 1}
	rr = adminRequest(t, s, http.MethodGet, "/v2/admin/users", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("op: %d %s", rr.Code, rr.Body.String())
	}

	// Admin repo unavailable (no DB): never 500 into a panic.
	s.adminRepo = nil
	rr = adminRequest(t, s, http.MethodGet, "/v2/admin/users", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("nil repo: %d", rr.Code)
	}
	_ = repo
}

func TestAdminUsers(t *testing.T) {
	s, repo := adminTestServer(t)

	// Search.
	rr := adminRequest(t, s, http.MethodGet, "/v2/admin/users?q=hunter", nil)
	var list struct {
		Users []AdminUserRow `json:"users"`
	}
	decodeInto(t, rr, &list)
	if len(list.Users) != 2 || list.Users[0].Username != "hunter" {
		t.Fatalf("search: %+v", list.Users)
	}

	// Detail with characters.
	rr = adminRequest(t, s, http.MethodGet, "/v2/admin/users/2", nil)
	var detail AdminUserDetail
	decodeInto(t, rr, &detail)
	if detail.Username != "hunter" || len(detail.Characters) != 1 {
		t.Fatalf("detail: %+v", detail)
	}
	if rr := adminRequest(t, s, http.MethodGet, "/v2/admin/users/99", nil); rr.Code != http.StatusNotFound {
		t.Errorf("missing user: %d", rr.Code)
	}

	// Promote and set rights in one PATCH.
	rr = adminRequest(t, s, http.MethodPatch, "/v2/admin/users/2", map[string]interface{}{"op": true, "rights": 62})
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rr.Code, rr.Body.String())
	}
	if !repo.users[2].Op || repo.users[2].Rights != 62 {
		t.Errorf("patch not applied: %+v", repo.users[2])
	}
	// Empty PATCH is rejected.
	if rr := adminRequest(t, s, http.MethodPatch, "/v2/admin/users/2", map[string]interface{}{}); rr.Code != http.StatusBadRequest {
		t.Errorf("empty patch: %d", rr.Code)
	}
	// Self-demotion is rejected.
	rr = adminRequest(t, s, http.MethodPatch, "/v2/admin/users/1", map[string]interface{}{"op": false})
	if rr.Code != http.StatusBadRequest || !repo.users[1].Op {
		t.Errorf("self-demotion: %d op=%v", rr.Code, repo.users[1].Op)
	}

	// Password reset stores a bcrypt hash of the new password.
	rr = adminRequest(t, s, http.MethodPost, "/v2/admin/users/2/password", map[string]string{"password": "new-secret"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("password: %d %s", rr.Code, rr.Body.String())
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.passwords[2]), []byte("new-secret")); err != nil {
		t.Errorf("stored hash does not match: %v", err)
	}
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/users/2/password", map[string]string{"password": ""}); rr.Code != http.StatusBadRequest {
		t.Errorf("empty password: %d", rr.Code)
	}
}

func TestAdminBans(t *testing.T) {
	s, repo := adminTestServer(t)

	// Permanent ban (empty body).
	rr := adminRequest(t, s, http.MethodPost, "/v2/admin/users/2/ban", nil)
	if rr.Code != http.StatusNoContent || !repo.users[2].Banned || repo.users[2].BanExpires != nil {
		t.Fatalf("permanent ban: %d %+v", rr.Code, repo.users[2])
	}

	// Temporary ban.
	exp := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	rr = adminRequest(t, s, http.MethodPost, "/v2/admin/users/3/ban", map[string]interface{}{"expires": exp})
	if rr.Code != http.StatusNoContent || repo.users[3].BanExpires == nil || !repo.users[3].BanExpires.Equal(exp) {
		t.Fatalf("temp ban: %d %+v", rr.Code, repo.users[3])
	}

	// Past expiry, self-ban and unknown user are rejected.
	past := time.Now().Add(-time.Hour)
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/users/3/ban", map[string]interface{}{"expires": past}); rr.Code != http.StatusBadRequest {
		t.Errorf("past expiry: %d", rr.Code)
	}
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/users/1/ban", nil); rr.Code != http.StatusBadRequest {
		t.Errorf("self ban: %d", rr.Code)
	}
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/users/99/ban", nil); rr.Code != http.StatusNotFound {
		t.Errorf("unknown user: %d", rr.Code)
	}

	// Unban.
	rr = adminRequest(t, s, http.MethodDelete, "/v2/admin/users/2/ban", nil)
	if rr.Code != http.StatusNoContent || repo.users[2].Banned {
		t.Fatalf("unban: %d %+v", rr.Code, repo.users[2])
	}
}

func TestAdminNotices(t *testing.T) {
	s, repo := adminTestServer(t)

	rr := adminRequest(t, s, http.MethodPost, "/v2/admin/notices", map[string]string{"body": "  Maintenance tonight  "})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var created NoticeRow
	decodeInto(t, rr, &created)
	if created.Body != "Maintenance tonight" {
		t.Errorf("body not trimmed: %q", created.Body)
	}

	for _, bad := range []map[string]string{{"body": ""}, {"body": strings.Repeat("x", adminMaxNoticeLength+1)}} {
		if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/notices", bad); rr.Code != http.StatusBadRequest {
			t.Errorf("bad notice %d chars: %d", len(bad["body"]), rr.Code)
		}
	}

	// Expired notices are hidden by default, shown with ?all=1.
	past := time.Now().Add(-time.Hour)
	repo.notices = append(repo.notices, NoticeRow{ID: 99, Body: "old", ExpiresAt: &past})
	var list struct {
		Notices []NoticeRow `json:"notices"`
	}
	decodeInto(t, adminRequest(t, s, http.MethodGet, "/v2/admin/notices", nil), &list)
	if len(list.Notices) != 1 {
		t.Errorf("active list: %+v", list.Notices)
	}
	decodeInto(t, adminRequest(t, s, http.MethodGet, "/v2/admin/notices?all=1", nil), &list)
	if len(list.Notices) != 2 {
		t.Errorf("full list: %+v", list.Notices)
	}

	// The runtime notice reaches the login payload after the static one.
	auth := s.newAuthData(1, 0, 1, "token", nil)
	if len(auth.Notices) != 1 || auth.Notices[0] != "Welcome to Erupe!<PAGE>Maintenance tonight" {
		t.Errorf("login notices: %+v", auth.Notices)
	}

	if rr := adminRequest(t, s, http.MethodDelete, "/v2/admin/notices/"+itoa(created.ID), nil); rr.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rr.Code)
	}
	if rr := adminRequest(t, s, http.MethodDelete, "/v2/admin/notices/12345", nil); rr.Code != http.StatusNotFound {
		t.Errorf("delete missing: %d", rr.Code)
	}
}

func TestAdminDistributions(t *testing.T) {
	s, repo := adminTestServer(t)

	qty := 10
	itemID := 482
	deadline := time.Now().Add(30 * 24 * time.Hour)
	body := map[string]interface{}{
		"type":      1,
		"eventName": "Welcome",
		"deadline":  deadline,
		"items":     []map[string]interface{}{{"itemType": 7, "itemId": itemID, "quantity": qty}},
	}
	rr := adminRequest(t, s, http.MethodPost, "/v2/admin/distributions", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var created AdminDistribution
	decodeInto(t, rr, &created)
	if created.ID == 0 || created.Description == "" || created.TimesAcceptable != 1 {
		t.Errorf("defaults not applied: %+v", created.DistributionRow)
	}
	if len(repo.distItems[created.ID]) != 1 || *repo.distItems[created.ID][0].ItemID != itemID {
		t.Errorf("items not stored: %+v", repo.distItems)
	}

	// No items, or a non-positive quantity, is rejected.
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/distributions", map[string]interface{}{"type": 1}); rr.Code != http.StatusBadRequest {
		t.Errorf("no items: %d", rr.Code)
	}
	zero := 0
	bad := map[string]interface{}{"type": 1, "items": []map[string]interface{}{{"itemType": 7, "itemId": 1, "quantity": zero}}}
	if rr := adminRequest(t, s, http.MethodPost, "/v2/admin/distributions", bad); rr.Code != http.StatusBadRequest {
		t.Errorf("zero quantity: %d", rr.Code)
	}

	// Detail and list.
	rr = adminRequest(t, s, http.MethodGet, "/v2/admin/distributions/"+itoa(created.ID), nil)
	var detail AdminDistribution
	decodeInto(t, rr, &detail)
	if detail.EventName != "Welcome" || len(detail.Items) != 1 {
		t.Errorf("detail: %+v", detail)
	}
	if rr := adminRequest(t, s, http.MethodGet, "/v2/admin/distributions/999", nil); rr.Code != http.StatusNotFound {
		t.Errorf("missing detail: %d", rr.Code)
	}

	// Delete.
	if rr := adminRequest(t, s, http.MethodDelete, "/v2/admin/distributions/"+itoa(created.ID), nil); rr.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rr.Code)
	}
	if len(repo.dists) != 0 {
		t.Errorf("not deleted: %+v", repo.dists)
	}
}

func TestAdminEvents(t *testing.T) {
	s, repo := adminTestServer(t)
	repo.events = []AdminEventRow{{ID: 1, EventType: "diva", StartTime: time.Now().Add(-90 * 24 * time.Hour)}}

	// Restart Diva at an explicit time replaces the old row.
	start := time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)
	rr := adminRequest(t, s, http.MethodPut, "/v2/admin/events/diva", map[string]interface{}{"startTime": start})
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rr.Code, rr.Body.String())
	}
	if len(repo.events) != 1 || !repo.events[0].StartTime.Equal(start) {
		t.Errorf("replace: %+v", repo.events)
	}

	// Empty body starts now.
	if rr := adminRequest(t, s, http.MethodPut, "/v2/admin/events/festa", nil); rr.Code != http.StatusOK {
		t.Errorf("start now: %d %s", rr.Code, rr.Body.String())
	}
	if len(repo.events) != 2 {
		t.Errorf("festa not added: %+v", repo.events)
	}

	if rr := adminRequest(t, s, http.MethodPut, "/v2/admin/events/raviente", nil); rr.Code != http.StatusBadRequest {
		t.Errorf("unknown type: %d", rr.Code)
	}

	// Stop removes every row of the type.
	rr = adminRequest(t, s, http.MethodDelete, "/v2/admin/events/diva", nil)
	var stopped map[string]int64
	decodeInto(t, rr, &stopped)
	if rr.Code != http.StatusOK || stopped["removed"] != 1 || len(repo.events) != 1 {
		t.Errorf("stop: %d %v %+v", rr.Code, stopped, repo.events)
	}

	var list struct {
		Events []AdminEventRow `json:"events"`
	}
	decodeInto(t, adminRequest(t, s, http.MethodGet, "/v2/admin/events", nil), &list)
	if len(list.Events) != 1 || list.Events[0].EventType != "festa" {
		t.Errorf("list: %+v", list.Events)
	}
}

func TestAdminRepoErrorsAre500(t *testing.T) {
	s, repo := adminTestServer(t)
	repo.failWith = errMockAdminDB
	// IsOp itself fails → 403 (fail closed), not 500.
	if rr := adminRequest(t, s, http.MethodGet, "/v2/admin/users", nil); rr.Code != http.StatusForbidden {
		t.Errorf("IsOp error: %d", rr.Code)
	}
}

var errMockAdminDB = errors.New("mock admin database error")

func itoa(i int) string { return strconv.Itoa(i) }

func TestAdminReloadContent(t *testing.T) {
	s, _ := adminTestServer(t)

	s.erupeConfig.ContentPath = filepath.Join(t.TempDir(), "missing")
	rr := adminRequest(t, s, "POST", "/v2/admin/content/reload", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing dir: status %d body %s", rr.Code, rr.Body.String())
	}

	// An existing directory with no files is a successful, empty reload.
	s.erupeConfig.ContentPath = t.TempDir()
	rr = adminRequest(t, s, "POST", "/v2/admin/content/reload", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty dir: status %d body %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Dir   string                     `json:"dir"`
		Files []migrations.ContentResult `json:"files"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Dir != s.erupeConfig.ContentPath || len(resp.Files) != 0 {
		t.Errorf("resp = %+v", resp)
	}

	// A malformed file is reported as the operator's error, with its name.
	if err := os.MkdirAll(filepath.Join(s.erupeConfig.ContentPath, "shop_items"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.erupeConfig.ContentPath, "shop_items", "road.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr = adminRequest(t, s, "POST", "/v2/admin/content/reload", nil)
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "road.json") {
		t.Fatalf("bad file: status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestAdminReloadContent_RequiresOp(t *testing.T) {
	s, _ := adminTestServer(t)
	s.sessionRepo = &mockAPISessionRepo{userID: 2}
	rr := adminRequest(t, s, "POST", "/v2/admin/content/reload", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rr.Code)
	}
}
