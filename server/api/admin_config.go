package api

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	cfg "erupe-ce/config"

	"go.uber.org/zap"
)

// The config editor: GET and PATCH /v2/admin/config read and change the
// config file Erupe loaded at start. Changes are written to disk (the
// previous file kept as <name>.bak) and apply at the next restart; each
// option reports whether the file on disk differs from what the running
// server loaded. Secret options are write-only.

//go:embed admin.html
var adminHTML []byte

// AdminPage serves the operator page at /admin. It is static: sign-in and
// every read or change go through the /v2 API with the operator's token.
func (s *APIServer) AdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(adminHTML)
}

// ConfigFieldState is one option as the editor shows it.
type ConfigFieldState struct {
	cfg.Field
	Value   interface{} `json:"value,omitempty"`   // value in the file on disk (omitted for secrets)
	Default interface{} `json:"default,omitempty"` // value when the file does not set it (omitted for secrets)
	Set     bool        `json:"set"`               // the file sets this option itself
	Pending bool        `json:"pending"`           // saved on disk but not yet loaded by the running server
}

// ConfigState is the GET /v2/admin/config payload.
type ConfigState struct {
	File        string             `json:"file"`
	Version     string             `json:"version"`             // SHA-256 of the file; send it back with a PATCH
	Editable    bool               `json:"editable"`            // false for non-JSON config files
	FileError   string             `json:"fileError,omitempty"` // the file on disk no longer loads
	UnknownKeys []string           `json:"unknownKeys"`         // keys Erupe ignores (typos, wrong section)
	Replaced    map[string]string  `json:"replaced,omitempty"`  // deprecated unknown key -> option to use instead
	Pending     bool               `json:"pending"`             // some saved change waits for a restart
	Fields      []ConfigFieldState `json:"fields"`
}

// ConfigPatch is the PATCH /v2/admin/config body.
type ConfigPatch struct {
	Version string                     `json:"version"` // optional: refuse the change if the file changed since
	Set     map[string]json.RawMessage `json:"set"`
	Unset   []string                   `json:"unset"` // options to remove from the file, back to their default
}

func fileVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// configFile returns the path of the loaded config file, or "" when the
// server was not started from one.
func (s *APIServer) configFile() string {
	if s.erupeConfig == nil {
		return ""
	}
	return s.erupeConfig.FilePath
}

func isJSONConfig(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
}

// configState builds the editor view of the file contents data.
func (s *APIServer) configState(path string, data []byte) ConfigState {
	st := ConfigState{
		File:        path,
		Version:     fileVersion(data),
		Editable:    isJSONConfig(path),
		UnknownKeys: []string{},
	}
	defaults, _ := cfg.DecodeJSON([]byte(`{}`))
	var onDisk, running *cfg.Config
	if st.Editable {
		var err error
		if onDisk, err = cfg.DecodeJSON(data); err != nil {
			st.FileError = err.Error()
		}
		if unknown, err := cfg.UnknownKeys(data); err == nil {
			st.UnknownKeys = unknown
			for _, key := range unknown {
				if r := cfg.Replacement(key); r != "" {
					if st.Replaced == nil {
						st.Replaced = map[string]string{}
					}
					st.Replaced[key] = r
				}
			}
		}
		running, _ = cfg.DecodeJSON(s.erupeConfig.FileData)
	}
	for _, f := range cfg.Fields() {
		fs := ConfigFieldState{Field: f, Set: st.Editable && cfg.HasKey(data, f.Key)}
		if onDisk != nil && running != nil {
			fs.Pending = !reflect.DeepEqual(f.ValueOf(onDisk), f.ValueOf(running))
			st.Pending = st.Pending || fs.Pending
		}
		if !f.Secret {
			fs.Default = f.ValueOf(defaults)
			if onDisk != nil {
				fs.Value = f.ValueOf(onDisk)
			} else {
				fs.Value = fs.Default
			}
		}
		st.Fields = append(st.Fields, fs)
	}
	return st
}

// AdminGetConfig handles GET /v2/admin/config.
func (s *APIServer) AdminGetConfig(w http.ResponseWriter, r *http.Request) {
	path := s.configFile()
	if path == "" {
		writeError(w, http.StatusNotFound, "no_config_file", "The server was not started from a config file")
		return
	}
	s.configMu.Lock()
	data, err := os.ReadFile(path)
	s.configMu.Unlock()
	if err != nil {
		s.logger.Error("Admin: read config failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Cannot read the config file")
		return
	}
	writeJSON(w, http.StatusOK, s.configState(path, data))
}

// AdminPatchConfig handles PATCH /v2/admin/config: validates every change,
// checks that the resulting file still loads, then writes it.
func (s *APIServer) AdminPatchConfig(w http.ResponseWriter, r *http.Request) {
	path := s.configFile()
	if path == "" {
		writeError(w, http.StatusNotFound, "no_config_file", "The server was not started from a config file")
		return
	}
	if !isJSONConfig(path) {
		writeError(w, http.StatusConflict, "not_json", "Only a JSON config file can be edited here")
		return
	}
	var body ConfigPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.Set) == 0 && len(body.Unset) == 0 {
		writeError(w, http.StatusBadRequest, "missing_fields", "Nothing to change")
		return
	}

	// Canonical keys, so the file and the audit log use the documented names.
	set := make(map[string]json.RawMessage, len(body.Set))
	var changed []zap.Field
	for key, raw := range body.Set {
		f, ok := cfg.FieldByKey(key)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown_option", fmt.Sprintf("%s: no such option", key))
			return
		}
		if err := cfg.ValidateValue(f, raw); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", err.Error())
			return
		}
		set[f.Key] = raw
		if f.Secret {
			changed = append(changed, zap.String(f.Key, "(secret)"))
		} else {
			changed = append(changed, zap.String(f.Key, string(raw)))
		}
	}
	unset := make([]string, 0, len(body.Unset))
	for _, key := range body.Unset {
		f, ok := cfg.FieldByKey(key)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown_option", fmt.Sprintf("%s: no such option", key))
			return
		}
		if _, both := set[f.Key]; both {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("%s: both set and unset", f.Key))
			return
		}
		unset = append(unset, f.Key)
		changed = append(changed, zap.String(f.Key, "(default)"))
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		s.logger.Error("Admin: read config failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Cannot read the config file")
		return
	}
	if body.Version != "" && body.Version != fileVersion(data) {
		writeError(w, http.StatusConflict, "config_changed", "The config file changed since it was loaded; reload and apply your changes again")
		return
	}
	out, err := cfg.SetKeys(data, set, unset)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", err.Error())
		return
	}
	if _, err := cfg.DecodeJSON(out); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", err.Error())
		return
	}
	if err := writeConfigFile(path, data, out); err != nil {
		s.logger.Error("Admin: write config failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "Cannot write the config file")
		return
	}
	s.audit(r, "edit config", append([]zap.Field{zap.String("file", path)}, changed...)...)
	writeJSON(w, http.StatusOK, s.configState(path, out))
}

// writeConfigFile keeps the previous contents as <path>.bak, then replaces
// path through a temporary file and a rename, so a crash never leaves a
// half-written config behind.
func writeConfigFile(path string, previous, data []byte) error {
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path+".bak", previous, mode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
