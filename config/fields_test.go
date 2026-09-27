package config

import (
	"encoding/json"
	"testing"
)

// TestEveryFieldDocumented keeps the config editor complete: a new option
// needs an entry in fieldMeta, and entries must not outlive their option.
func TestEveryFieldDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Fields() {
		seen[f.Key] = true
		if f.Description == "" {
			t.Errorf("%s has no description in fieldMeta", f.Key)
		}
	}
	for key := range fieldMeta {
		if !seen[key] {
			t.Errorf("fieldMeta[%q] matches no option", key)
		}
	}
}

func TestFieldsShape(t *testing.T) {
	cases := map[string]struct {
		kind  FieldKind
		group string
	}{
		"Host":                        {KindString, "General"},
		"LoginNotices":                {KindList, "General"},
		"GameplayOptions.MaximumRP":   {KindInt, "GameplayOptions"},
		"GameplayOptions.GUrgentRate": {KindNumber, "GameplayOptions"},
		"DebugOptions.CapLink.Key":    {KindString, "DebugOptions"},
		"Entrance.Entries":            {KindJSON, "Entrance"},
	}
	for key, want := range cases {
		f, ok := FieldByKey(key)
		if !ok {
			t.Errorf("%s: not found", key)
			continue
		}
		if f.Kind != want.kind || f.Group != want.group {
			t.Errorf("%s: kind %s group %s, want %s %s", key, f.Kind, f.Group, want.kind, want.group)
		}
	}
	if f, _ := FieldByKey("Commands"); f.Group != "Commands" {
		t.Errorf("Commands group = %s, want its own section", f.Group)
	}
	if _, ok := FieldByKey("RealClientMode"); ok {
		t.Error("derived RealClientMode must not be editable")
	}
	if f, _ := FieldByKey("gameplayoptions.maximumrp"); *f.Max != 65535 || *f.Min != 0 {
		t.Errorf("MaximumRP range = [%v, %v], want [0, 65535]", *f.Min, *f.Max)
	}
	for _, key := range []string{"Database.Password", "Discord.BotToken", "DebugOptions.CapLink.Key"} {
		if f, _ := FieldByKey(key); !f.Secret {
			t.Errorf("%s must be secret", key)
		}
	}
}

func TestValueOfByteSlices(t *testing.T) {
	c, err := DecodeJSON([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := FieldByKey("GameplayOptions.ClanMemberLimits")
	data, _ := json.Marshal(f.ValueOf(c))
	if string(data) != "[[0,30],[3,40],[7,50],[10,60]]" {
		t.Errorf("ClanMemberLimits encodes as %s", data)
	}
	if err := ValidateValue(f, data); err != nil {
		t.Errorf("its own value does not validate: %v", err)
	}
}
