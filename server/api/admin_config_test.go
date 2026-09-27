package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfigFile = `{
  "Host": "10.0.0.2",
  "DisableSoftCrash": true,
  "LoginNotices": ["<BODY>Hello"],
  "GameplayOptions": {"MaximumRP": 50000, "MaxRP": 1},
  "Database": {"Password": "hunter2"}
}
`

// configTestServer returns an operator test server started from a config
// file holding testConfigFile.
func configTestServer(t *testing.T) (*APIServer, string) {
	t.Helper()
	s, _ := adminTestServer(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(testConfigFile), 0640); err != nil {
		t.Fatal(err)
	}
	s.erupeConfig.FilePath = path
	s.erupeConfig.FileData = []byte(testConfigFile)
	return s, path
}

func fieldState(t *testing.T, st ConfigState, key string) ConfigFieldState {
	t.Helper()
	for _, f := range st.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("no field %s", key)
	return ConfigFieldState{}
}

func TestAdminGetConfig(t *testing.T) {
	s, _ := configTestServer(t)
	rr := adminRequest(t, s, "GET", "/v2/admin/config", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "hunter2") {
		t.Fatal("secret value sent to the browser")
	}
	var st ConfigState
	decodeInto(t, rr, &st)
	if !st.Editable || st.Pending || st.FileError != "" {
		t.Errorf("editable %v pending %v error %q", st.Editable, st.Pending, st.FileError)
	}
	if len(st.UnknownKeys) != 2 || st.UnknownKeys[0] != "DisableSoftCrash" || st.UnknownKeys[1] != "GameplayOptions.MaxRP" {
		t.Errorf("unknown keys %v", st.UnknownKeys)
	}
	if len(st.Replaced) != 1 || st.Replaced["DisableSoftCrash"] != "DisableShutdownCountdown" {
		t.Errorf("replaced %v", st.Replaced)
	}
	if fieldState(t, st, "DisableShutdownCountdown").Set {
		t.Error("the deprecated key must not count as setting DisableShutdownCountdown")
	}
	rp := fieldState(t, st, "GameplayOptions.MaximumRP")
	if !rp.Set || rp.Value.(float64) != 50000 || rp.Default.(float64) != 50000 {
		t.Errorf("MaximumRP %+v", rp)
	}
	hrp := fieldState(t, st, "GameplayOptions.HRPMultiplier")
	if hrp.Set || hrp.Value.(float64) != 1 {
		t.Errorf("HRPMultiplier %+v", hrp)
	}
	pw := fieldState(t, st, "Database.Password")
	if !pw.Secret || !pw.Set || pw.Value != nil {
		t.Errorf("Database.Password %+v", pw)
	}
}

func TestAdminPatchConfig(t *testing.T) {
	s, path := configTestServer(t)
	rr := adminRequest(t, s, "PATCH", "/v2/admin/config", map[string]interface{}{
		"set": map[string]interface{}{
			"gameplayoptions.maximumrp":     60000,
			"GameplayOptions.HRPMultiplier": 2.5,
			"Database.Password":             "new pw",
		},
		"unset": []string{"Host"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var st ConfigState
	decodeInto(t, rr, &st)
	if !st.Pending || !fieldState(t, st, "GameplayOptions.MaximumRP").Pending || !fieldState(t, st, "Database.Password").Pending {
		t.Error("saved changes not reported as waiting for a restart")
	}
	if fieldState(t, st, "LoginNotices").Pending {
		t.Error("untouched option reported as pending")
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{`"MaximumRP": 60000`, `"HRPMultiplier": 2.5`, `"Password": "new pw"`, `"MaxRP": 1`, `<BODY>Hello`} {
		if !strings.Contains(got, want) {
			t.Errorf("file lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"Host"`) {
		t.Errorf("Host not removed:\n%s", got)
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != testConfigFile {
		t.Errorf("backup = %q", bak)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0640 {
		t.Errorf("file mode %v, want 0640", info.Mode().Perm())
	}
	if matches, _ := filepath.Glob(path + ".*.tmp"); len(matches) > 0 {
		t.Errorf("temporary files left: %v", matches)
	}
}

func TestAdminPatchConfigRejects(t *testing.T) {
	cases := []struct {
		name   string
		body   map[string]interface{}
		status int
		code   string
	}{
		{"unknown option", map[string]interface{}{"set": map[string]interface{}{"GameplayOptions.MaxRP": 1}}, 400, "unknown_option"},
		{"out of range", map[string]interface{}{"set": map[string]interface{}{"GameplayOptions.MaximumRP": 70000}}, 400, "invalid_value"},
		{"wrong type", map[string]interface{}{"set": map[string]interface{}{"HideLoginNotice": "yes"}}, 400, "invalid_value"},
		{"bad choice", map[string]interface{}{"set": map[string]interface{}{"ClientMode": "ZZZ"}}, 400, "invalid_value"},
		{"zero RP interval", map[string]interface{}{"set": map[string]interface{}{"GameplayOptions.RPAccrualCafeSeconds": 0}}, 400, "invalid_value"},
		{"set and unset", map[string]interface{}{"set": map[string]interface{}{"Host": "a"}, "unset": []string{"host"}}, 400, "invalid_request"},
		{"empty", map[string]interface{}{}, 400, "missing_fields"},
		{"stale version", map[string]interface{}{"version": "abc", "set": map[string]interface{}{"Host": "a"}}, 409, "config_changed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, path := configTestServer(t)
			rr := adminRequest(t, s, "PATCH", "/v2/admin/config", c.body)
			var e ErrorResponse
			decodeInto(t, rr, &e)
			if rr.Code != c.status || e.Error != c.code {
				t.Errorf("got %d %s (%s), want %d %s", rr.Code, e.Error, e.Message, c.status, c.code)
			}
			if data, _ := os.ReadFile(path); string(data) != testConfigFile {
				t.Error("file changed by a rejected request")
			}
		})
	}
}

func TestAdminPatchConfigVersion(t *testing.T) {
	s, _ := configTestServer(t)
	var st ConfigState
	decodeInto(t, adminRequest(t, s, "GET", "/v2/admin/config", nil), &st)
	rr := adminRequest(t, s, "PATCH", "/v2/admin/config", map[string]interface{}{
		"version": st.Version,
		"set":     map[string]json.RawMessage{"LoginNotices": json.RawMessage(`["<BODY>Bye"]`)},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	// The same version is now stale.
	rr = adminRequest(t, s, "PATCH", "/v2/admin/config", map[string]interface{}{
		"version": st.Version,
		"set":     map[string]interface{}{"Host": "b"},
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rr.Code)
	}
}

func TestAdminConfigNotJSON(t *testing.T) {
	s, _ := adminTestServer(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("Host = \"x\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.erupeConfig.FilePath = path
	var st ConfigState
	rr := adminRequest(t, s, "GET", "/v2/admin/config", nil)
	decodeInto(t, rr, &st)
	if rr.Code != http.StatusOK || st.Editable {
		t.Errorf("status %d editable %v", rr.Code, st.Editable)
	}
	rr = adminRequest(t, s, "PATCH", "/v2/admin/config", map[string]interface{}{"set": map[string]interface{}{"Host": "a"}})
	if rr.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rr.Code)
	}
}

func TestAdminConfigBrokenFile(t *testing.T) {
	s, path := configTestServer(t)
	if err := os.WriteFile(path, []byte(`{"GameplayOptions": {"RPAccrualCafeSeconds": 0}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var st ConfigState
	decodeInto(t, adminRequest(t, s, "GET", "/v2/admin/config", nil), &st)
	if st.FileError == "" {
		t.Error("a file that no longer loads must be reported")
	}
	// Fixing the broken option is allowed.
	rr := adminRequest(t, s, "PATCH", "/v2/admin/config", map[string]interface{}{
		"set": map[string]interface{}{"GameplayOptions.RPAccrualCafeSeconds": 900},
	})
	if rr.Code != http.StatusOK {
		t.Errorf("status %d: %s", rr.Code, rr.Body)
	}
}

func TestAdminConfigNoFile(t *testing.T) {
	s, _ := adminTestServer(t)
	if rr := adminRequest(t, s, "GET", "/v2/admin/config", nil); rr.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rr.Code)
	}
}

func TestAdminConfigRequiresOperator(t *testing.T) {
	s, _ := configTestServer(t)
	s.sessionRepo = &mockAPISessionRepo{userID: 2}
	if rr := adminRequest(t, s, "GET", "/v2/admin/config", nil); rr.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rr.Code)
	}
}

func TestAdminPage(t *testing.T) {
	s, _ := adminTestServer(t)
	rr := httptest.NewRecorder()
	s.AdminPage(rr, httptest.NewRequest("GET", "/admin", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status %d type %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Body.String(), "/v2/admin/config") {
		t.Error("page does not use the config API")
	}
}
