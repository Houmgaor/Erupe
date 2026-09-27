package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const sampleFile = `{
  "Host": "10.0.0.2",
  "commandprefix": "#",
  "LoginNotices": ["<BODY>Hello"],
  "gameplayoptions": {
    "MaximumRP": 50000,
    "HRPMultiplier": 2
  },
  "Database": {"Password": "pw"}
}`

func TestSetKeysKeepsOrderAndSpelling(t *testing.T) {
	out, err := SetKeys([]byte(sampleFile), map[string]json.RawMessage{
		"GameplayOptions.MaximumRP":   json.RawMessage(`60000`),
		"GameplayOptions.ExtraCarves": json.RawMessage(`1`),
		"Discord.Enabled":             json.RawMessage(`true`),
		"LoginNotices":                json.RawMessage(`["<BODY>Hi & bye"]`),
	}, []string{"Host", "Sign.Port"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"gameplayoptions"`, `"MaximumRP": 60000`, `"ExtraCarves": 1`, `<BODY>Hi & bye`} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, `"Host"`) || strings.Contains(s, `"Sign"`) {
		t.Errorf("unset keys still present:\n%s", s)
	}
	// Existing keys keep their order; new sections go at the end.
	order := []string{`"commandprefix"`, `"LoginNotices"`, `"gameplayoptions"`, `"Database"`, `"Discord"`}
	last := -1
	for _, k := range order {
		i := strings.Index(s, k)
		if i < last {
			t.Fatalf("%s out of order:\n%s", k, s)
		}
		last = i
	}
	c, err := DecodeJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if c.GameplayOptions.MaximumRP != 60000 || c.GameplayOptions.HRPMultiplier != 2 || !c.Discord.Enabled {
		t.Errorf("decoded %+v", c.GameplayOptions)
	}
	if c.CommandPrefix != "#" || c.Database.Password != "pw" {
		t.Error("untouched options changed")
	}
}

// TestDeprecatedKeyIgnored: DisableSoftCrash was renamed because its name
// was misleading. It sets nothing and is reported with its replacement.
func TestDeprecatedKeyIgnored(t *testing.T) {
	data := []byte(`{"DisableSoftCrash": true}`)
	c, err := DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.DisableShutdownCountdown {
		t.Error("DisableSoftCrash must not set DisableShutdownCountdown")
	}
	if got, _ := UnknownKeys(data); len(got) != 1 || got[0] != "DisableSoftCrash" {
		t.Errorf("UnknownKeys = %v", got)
	}
	if Replacement("DisableSoftCrash") != "DisableShutdownCountdown" || Replacement("Host") != "" {
		t.Error("wrong replacement")
	}
	if HasKey(data, "DisableShutdownCountdown") {
		t.Error("the deprecated key must not count as the new option")
	}
}

func TestSetKeysRejectsNonObjectSection(t *testing.T) {
	if _, err := SetKeys([]byte(`{"GameplayOptions": 3}`), map[string]json.RawMessage{
		"GameplayOptions.MaximumRP": json.RawMessage(`1`),
	}, nil); err == nil {
		t.Error("expected an error writing inside a non-object")
	}
}

func TestHasKey(t *testing.T) {
	for key, want := range map[string]bool{
		"GameplayOptions.MaximumRP":   true,
		"GameplayOptions.ExtraCarves": false,
		"Database.Password":           true,
		"Discord.Enabled":             false,
		"host":                        true,
	} {
		if got := HasKey([]byte(sampleFile), key); got != want {
			t.Errorf("HasKey(%s) = %v, want %v", key, got, want)
		}
	}
}

func TestUnknownKeys(t *testing.T) {
	got, err := UnknownKeys([]byte(`{
		"Host": "x",
		"DisableSoftCrash": true,
		"DisableLoginBoost": true,
		"GameplayOptions": {"MaximumRP": 1, "MaxRP": 2},
		"Entrance": {"Entries": [{"Name": "A", "Chanels": [], "Channels": [{"Port": 1, "Enabled": true, "Max": 3}]}]},
		"RealClientMode": 41
	}`))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"DisableLoginBoost", "DisableSoftCrash", "Entrance.Entries[0].Chanels", "Entrance.Entries[0].Channels[0].Max", "GameplayOptions.MaxRP", "RealClientMode"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("UnknownKeys = %v, want %v", got, want)
	}
}

func TestUnknownKeysReferenceFile(t *testing.T) {
	for _, name := range []string{"../config.reference.json", "../config.example.json"} {
		data := readFile(t, name)
		got, err := UnknownKeys(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > 0 {
			t.Errorf("%s has keys Erupe ignores: %v", name, got)
		}
	}
}

func TestValidateValue(t *testing.T) {
	cases := []struct {
		key, raw string
		ok       bool
	}{
		{"GameplayOptions.MaximumRP", `60000`, true},
		{"GameplayOptions.MaximumRP", `70000`, false}, // uint16 overflow
		{"GameplayOptions.MaximumRP", `1.5`, false},
		{"GameplayOptions.MaximumRP", `"5"`, false},
		{"GameplayOptions.RPAccrualNormalSeconds", `0`, false},
		{"GameplayOptions.HRPMultiplier", `2.5`, true},
		{"GameplayOptions.HRPMultiplier", `-1`, false},
		{"Screenshots.UploadQuality", `101`, false},
		{"ClientMode", `"zz"`, true},
		{"ClientMode", `"ZZZ"`, false},
		{"HideLoginNotice", `1`, false},
		{"LoginNotices", `["a", "b"]`, true},
		{"DefaultCourses", `[1, -2]`, false},
		{"Entrance.Entries", `[{"Name": "N", "Type": 1, "Channels": [{"Port": 54001, "MaxPlayers": 100}]}]`, true},
		{"Entrance.Entries", `[{"Name": "N", "Chanels": []}]`, false},
		{"API.Banners", `[{"src": "a.png", "link": "b"}]`, true},
	}
	for _, c := range cases {
		f, ok := FieldByKey(c.key)
		if !ok {
			t.Fatalf("%s: no such field", c.key)
		}
		err := ValidateValue(f, json.RawMessage(c.raw))
		if (err == nil) != c.ok {
			t.Errorf("ValidateValue(%s, %s) = %v, want ok=%v", c.key, c.raw, err, c.ok)
		}
	}
}

func TestDecodeJSONValidates(t *testing.T) {
	if _, err := DecodeJSON([]byte(`{"GameplayOptions": {"RPAccrualCafeSeconds": 0}}`)); err == nil {
		t.Error("expected RP accrual validation error")
	}
	c, err := DecodeJSON([]byte(`{"ClientMode": "g10"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.RealClientMode != G10 || c.GameplayOptions.HRPMultiplier != 1 {
		t.Errorf("mode %v, HRP multiplier %v", c.RealClientMode, c.GameplayOptions.HRPMultiplier)
	}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestLoadConfigRecordsFile: LoadConfig keeps the path and contents of the
// file it read, which the config editor compares with the file on disk.
func TestLoadConfigRecordsFile(t *testing.T) {
	viper.Reset()
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	writeMinimalConfig(t, dir, `{"Host": "127.0.0.1", "LoopDelay": 20}`)
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(c.FilePath) != "config.json" || !strings.Contains(string(c.FileData), `"LoopDelay": 20`) {
		t.Errorf("FilePath %q / FileData %q", c.FilePath, c.FileData)
	}
}
