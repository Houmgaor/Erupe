package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// Operator endpoints under /v2/admin. They reuse the player session token
// from /v2/login; AdminMiddleware additionally requires users.op = true, the
// same flag the in-game !ban / !rights commands check. Every mutation is
// logged with the acting operator's user ID so the server log doubles as an
// audit trail.

const (
	adminMaxSearchResults = 200
	adminMaxNoticeLength  = 4096
)

// AdminMiddleware runs after AuthMiddleware and rejects non-operators.
func (s *APIServer) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok || s.adminRepo == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
			return
		}
		op, err := s.adminRepo.IsOp(r.Context(), userID)
		if err != nil || !op {
			writeError(w, http.StatusForbidden, "forbidden", "Operator rights required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerAdminRoutes mounts the operator endpoints on the /v2 subrouter.
// Shared with the routing tests so both see the same table.
func (s *APIServer) registerAdminRoutes(v2 *mux.Router) {
	admin := v2.PathPrefix("/admin").Subrouter()
	admin.Use(s.AuthMiddleware, s.AdminMiddleware)

	admin.HandleFunc("/users", s.AdminSearchUsers).Methods("GET")
	admin.HandleFunc("/users/{id:[0-9]+}", s.AdminGetUser).Methods("GET")
	admin.HandleFunc("/users/{id:[0-9]+}", s.AdminUpdateUser).Methods("PATCH")
	admin.HandleFunc("/users/{id:[0-9]+}/password", s.AdminSetPassword).Methods("POST")
	admin.HandleFunc("/users/{id:[0-9]+}/ban", s.AdminBan).Methods("POST")
	admin.HandleFunc("/users/{id:[0-9]+}/ban", s.AdminUnban).Methods("DELETE")

	admin.HandleFunc("/notices", s.AdminListNotices).Methods("GET")
	admin.HandleFunc("/notices", s.AdminCreateNotice).Methods("POST")
	admin.HandleFunc("/notices/{id:[0-9]+}", s.AdminDeleteNotice).Methods("DELETE")

	admin.HandleFunc("/distributions", s.AdminListDistributions).Methods("GET")
	admin.HandleFunc("/distributions", s.AdminCreateDistribution).Methods("POST")
	admin.HandleFunc("/distributions/{id:[0-9]+}", s.AdminGetDistribution).Methods("GET")
	admin.HandleFunc("/distributions/{id:[0-9]+}", s.AdminDeleteDistribution).Methods("DELETE")

	admin.HandleFunc("/events", s.AdminListEvents).Methods("GET")
	admin.HandleFunc("/events/{type}", s.AdminStartEvent).Methods("PUT")
	admin.HandleFunc("/events/{type}", s.AdminStopEvent).Methods("DELETE")
}

// audit logs a mutation with the operator who made it.
func (s *APIServer) audit(r *http.Request, action string, fields ...zap.Field) {
	opID, _ := UserIDFromContext(r.Context())
	s.logger.Info("Admin: "+action, append([]zap.Field{zap.Uint32("operator", opID)}, fields...)...)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return false
	}
	return true
}

func pathID(r *http.Request) int {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	return id
}

// ── Users ────────────────────────────────────────────────────────────────────

// AdminSearchUsers handles GET /v2/admin/users?q=<substring>&limit=<n>.
func (s *APIServer) AdminSearchUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > adminMaxSearchResults {
		limit = 50
	}
	rows, err := s.adminRepo.SearchUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		s.logger.Error("Admin: user search failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": rows})
}

// AdminUserDetail is the GET /v2/admin/users/{id} payload.
type AdminUserDetail struct {
	AdminUserRow
	Characters []Character `json:"characters"`
}

// AdminGetUser handles GET /v2/admin/users/{id}.
func (s *APIServer) AdminGetUser(w http.ResponseWriter, r *http.Request) {
	id := uint32(pathID(r))
	row, err := s.adminRepo.GetUser(r.Context(), id)
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "No such user")
		return
	}
	if err != nil {
		s.logger.Error("Admin: get user failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	chars, err := s.charRepo.GetForUser(r.Context(), id)
	if err != nil {
		s.logger.Error("Admin: list characters failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if chars == nil {
		chars = []Character{}
	}
	writeJSON(w, http.StatusOK, AdminUserDetail{AdminUserRow: row, Characters: chars})
}

// AdminUpdateUser handles PATCH /v2/admin/users/{id} with {"op": bool} and/or {"rights": n}.
func (s *APIServer) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := uint32(pathID(r))
	var body struct {
		Op     *bool   `json:"op"`
		Rights *uint32 `json:"rights"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Op == nil && body.Rights == nil {
		writeError(w, http.StatusBadRequest, "missing_fields", "Provide op and/or rights")
		return
	}
	self, _ := UserIDFromContext(r.Context())
	if body.Op != nil && !*body.Op && id == self {
		// The last operator locking themselves out is the one mistake that
		// cannot be undone through this API.
		writeError(w, http.StatusBadRequest, "self_demotion", "You cannot remove your own operator rights")
		return
	}
	if body.Op != nil {
		if err := s.adminRepo.SetOp(r.Context(), id, *body.Op); err != nil {
			s.adminWriteUpdateError(w, err, "set op")
			return
		}
		s.audit(r, "set op", zap.Uint32("user", id), zap.Bool("op", *body.Op))
	}
	if body.Rights != nil {
		if err := s.adminRepo.SetRights(r.Context(), id, *body.Rights); err != nil {
			s.adminWriteUpdateError(w, err, "set rights")
			return
		}
		s.audit(r, "set rights", zap.Uint32("user", id), zap.Uint32("rights", *body.Rights))
	}
	row, err := s.adminRepo.GetUser(r.Context(), id)
	if err != nil {
		s.adminWriteUpdateError(w, err, "reload user")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *APIServer) adminWriteUpdateError(w http.ResponseWriter, err error, what string) {
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "No such user")
		return
	}
	s.logger.Error("Admin: "+what+" failed", zap.Error(err))
	writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
}

// AdminSetPassword handles POST /v2/admin/users/{id}/password with {"password": "…"}.
func (s *APIServer) AdminSetPassword(w http.ResponseWriter, r *http.Request) {
	id := uint32(pathID(r))
	var body struct {
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Password required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if err := s.adminRepo.SetPasswordHash(r.Context(), id, string(hash)); err != nil {
		s.adminWriteUpdateError(w, err, "set password")
		return
	}
	s.audit(r, "reset password", zap.Uint32("user", id))
	w.WriteHeader(http.StatusNoContent)
}

// AdminBan handles POST /v2/admin/users/{id}/ban with an optional
// {"expires": "<RFC 3339>"}; no expiry means a permanent ban.
func (s *APIServer) AdminBan(w http.ResponseWriter, r *http.Request) {
	id := uint32(pathID(r))
	var body struct {
		Expires *time.Time `json:"expires"`
	}
	if r.ContentLength != 0 && !decodeBody(w, r, &body) {
		return
	}
	if body.Expires != nil && body.Expires.Before(time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid_expiry", "Ban expiry is in the past")
		return
	}
	self, _ := UserIDFromContext(r.Context())
	if id == self {
		writeError(w, http.StatusBadRequest, "self_ban", "You cannot ban yourself")
		return
	}
	if _, err := s.adminRepo.GetUser(r.Context(), id); err != nil {
		s.adminWriteUpdateError(w, err, "ban lookup")
		return
	}
	if err := s.adminRepo.Ban(r.Context(), id, body.Expires); err != nil {
		s.adminWriteUpdateError(w, err, "ban")
		return
	}
	fields := []zap.Field{zap.Uint32("user", id)}
	if body.Expires != nil {
		fields = append(fields, zap.Time("expires", *body.Expires))
	}
	s.audit(r, "ban", fields...)
	w.WriteHeader(http.StatusNoContent)
}

// AdminUnban handles DELETE /v2/admin/users/{id}/ban.
func (s *APIServer) AdminUnban(w http.ResponseWriter, r *http.Request) {
	id := uint32(pathID(r))
	if err := s.adminRepo.Unban(r.Context(), id); err != nil {
		s.adminWriteUpdateError(w, err, "unban")
		return
	}
	s.audit(r, "unban", zap.Uint32("user", id))
	w.WriteHeader(http.StatusNoContent)
}

// ── Notices ──────────────────────────────────────────────────────────────────

// AdminListNotices handles GET /v2/admin/notices[?all=1].
func (s *APIServer) AdminListNotices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.adminRepo.ListNotices(r.Context(), r.URL.Query().Get("all") == "")
	if err != nil {
		s.logger.Error("Admin: list notices failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"notices": rows})
}

// AdminCreateNotice handles POST /v2/admin/notices with {"body": "…", "expires": "<RFC 3339>"?}.
func (s *APIServer) AdminCreateNotice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body    string     `json:"body"`
		Expires *time.Time `json:"expires"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	body.Body = strings.TrimSpace(body.Body)
	if body.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Notice body required")
		return
	}
	if len(body.Body) > adminMaxNoticeLength {
		writeError(w, http.StatusBadRequest, "too_long", "Notice body too long")
		return
	}
	row, err := s.adminRepo.CreateNotice(r.Context(), body.Body, body.Expires)
	if err != nil {
		s.logger.Error("Admin: create notice failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "create notice", zap.Int("notice", row.ID))
	writeJSON(w, http.StatusCreated, row)
}

// AdminDeleteNotice handles DELETE /v2/admin/notices/{id}.
func (s *APIServer) AdminDeleteNotice(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	err := s.adminRepo.DeleteNotice(r.Context(), id)
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "No such notice")
		return
	}
	if err != nil {
		s.logger.Error("Admin: delete notice failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "delete notice", zap.Int("notice", id))
	w.WriteHeader(http.StatusNoContent)
}

// ── Distributions ────────────────────────────────────────────────────────────

// AdminDistribution is a distribution with its items, used for both the
// create request and the detail response.
type AdminDistribution struct {
	DistributionRow
	Items []DistributionItemRow `json:"items"`
}

// AdminListDistributions handles GET /v2/admin/distributions.
func (s *APIServer) AdminListDistributions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.adminRepo.ListDistributions(r.Context())
	if err != nil {
		s.logger.Error("Admin: list distributions failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"distributions": rows})
}

// AdminGetDistribution handles GET /v2/admin/distributions/{id}.
func (s *APIServer) AdminGetDistribution(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	rows, err := s.adminRepo.ListDistributions(r.Context())
	if err != nil {
		s.logger.Error("Admin: list distributions failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	var found *DistributionRow
	for i := range rows {
		if rows[i].ID == id {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "not_found", "No such distribution")
		return
	}
	items, err := s.adminRepo.GetDistributionItems(r.Context(), id)
	if err != nil {
		s.logger.Error("Admin: list distribution items failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, AdminDistribution{DistributionRow: *found, Items: items})
}

// AdminCreateDistribution handles POST /v2/admin/distributions.
func (s *APIServer) AdminCreateDistribution(w http.ResponseWriter, r *http.Request) {
	var body AdminDistribution
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "missing_fields", "At least one item required")
		return
	}
	for _, it := range body.Items {
		if it.Quantity != nil && *it.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_item", "Item quantity must be positive")
			return
		}
	}
	if body.EventName == "" {
		body.EventName = "GM Gift!"
	}
	if body.Description == "" {
		body.Description = "~C05You received a gift!"
	}
	if body.TimesAcceptable <= 0 {
		body.TimesAcceptable = 1
	}
	id, err := s.adminRepo.CreateDistribution(r.Context(), body.DistributionRow, body.Items)
	if err != nil {
		s.logger.Error("Admin: create distribution failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "create distribution", zap.Int("distribution", id),
		zap.String("name", body.EventName), zap.Int("items", len(body.Items)))
	body.ID = id
	writeJSON(w, http.StatusCreated, body)
}

// AdminDeleteDistribution handles DELETE /v2/admin/distributions/{id}.
func (s *APIServer) AdminDeleteDistribution(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	err := s.adminRepo.DeleteDistribution(r.Context(), id)
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "No such distribution")
		return
	}
	if err != nil {
		s.logger.Error("Admin: delete distribution failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "delete distribution", zap.Int("distribution", id))
	w.WriteHeader(http.StatusNoContent)
}

// ── Events ───────────────────────────────────────────────────────────────────

// AdminListEvents handles GET /v2/admin/events.
func (s *APIServer) AdminListEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.adminRepo.ListEvents(r.Context())
	if err != nil {
		s.logger.Error("Admin: list events failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": rows})
}

// AdminStartEvent handles PUT /v2/admin/events/{type} with an optional
// {"startTime": "<RFC 3339>"} (default: now). Any previous run of that
// event type is replaced, which is how the game restarts a Diva Defense or
// Festa cycle.
func (s *APIServer) AdminStartEvent(w http.ResponseWriter, r *http.Request) {
	eventType := mux.Vars(r)["type"]
	if err := validEventType(eventType); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event_type", err.Error())
		return
	}
	var body struct {
		StartTime *time.Time `json:"startTime"`
	}
	if r.ContentLength != 0 && !decodeBody(w, r, &body) {
		return
	}
	start := time.Now()
	if body.StartTime != nil {
		start = *body.StartTime
	}
	row, err := s.adminRepo.ReplaceEvent(r.Context(), eventType, start)
	if err != nil {
		s.logger.Error("Admin: start event failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "start event", zap.String("type", eventType), zap.Time("start", start))
	writeJSON(w, http.StatusOK, row)
}

// AdminStopEvent handles DELETE /v2/admin/events/{type}.
func (s *APIServer) AdminStopEvent(w http.ResponseWriter, r *http.Request) {
	eventType := mux.Vars(r)["type"]
	if err := validEventType(eventType); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event_type", err.Error())
		return
	}
	n, err := s.adminRepo.DeleteEvents(r.Context(), eventType)
	if err != nil {
		s.logger.Error("Admin: stop event failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.audit(r, "stop event", zap.String("type", eventType), zap.Int64("removed", n))
	writeJSON(w, http.StatusOK, map[string]int64{"removed": n})
}
