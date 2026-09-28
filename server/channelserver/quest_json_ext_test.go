package channelserver

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

// fullQuestJSON uses every section recovered in #40.
const fullQuestJSON = `{
	"quest_id": 7,
	"title": "Ⅱ Test",
	"time_limit_minutes": 50,
	"time_limit_frames": 90015,
	"objective_main": {"type": "hunt", "target": 11, "count": 1},
	"objective_sub_a": {"type": "0x4001", "target": 3, "count": 2},
	"header_ext": {
		"unk_50": 5, "unk_5c": [1, 2, 3], "em_quest_param": -3,
		"unk_64": [{"a": 1, "b": 2, "c": 3, "d": 4}], "unk_74": 9,
		"unk_80": [1, 0, 3], "unk_88": {"a": 1, "h": 8}, "unk_94": [0, 7], "unk_9c": [0, 0, 5]
	},
	"main_ext": {
		"flags": 305419896, "unk_04": [1, 2], "unk_14": 3, "unk_1a": 4,
		"objective_extra": {"type": "capture", "target": 20, "count": 1},
		"monster_variants": [1, 0, 2], "map_variant": 3, "required_item": 500, "required_item_count": 2,
		"allowed_equip_bitmask": 15, "main_points": 100, "reward_items": [1, 2, 3],
		"unk_b6": [0, 9], "quest_clears_allowed": 5, "unk_c8": [7]
	},
	"flow_script": [{"op": 27}, {"op": 2, "args": [1, 2, 3]}, {"op": -1}, {"op": 9, "args": [150]}, {"op": -2}, {"op": 0}],
	"stages": [{"stage_id": 110, "x": 1.5, "y": 2, "z": -3}, {"stage_id": 111}, {"stage_id": 112}, {"stage_id": 113}],
	"monster_respawn_points": [
		{"stage": 111, "points": [{"angle": 28444, "x": 7150, "y": -59, "z": 10631}, {"x": 1, "y": 2, "z": 3}]},
		{"stage": 115, "points": [{"x": 4, "y": 5, "z": 6}]}
	],
	"supply_main": [{"item": 0, "quantity": 0}, {"item": 1, "quantity": 5}],
	"supply_extra": {"item": 9, "quantity": 1},
	"rewards": [{"table_id": 1, "table_flags": 128, "items": [{"rate": 50, "item": 149, "quantity": 1}]}],
	"large_monsters": [{"id": 11, "unk_02": 2, "spawn_amount": 1, "spawn_stage": 5, "unk_0c": [0, 7], "orientation": 180, "x": 1, "y": 2, "z": 3, "unk_2c": [0, 1]}],
	"quest_area": [
		[{"loaded_stage": 110, "spawn_types": [29, 12, -1, -1], "minion_spawns": [{"monster": 29, "spawn_toggle": 5, "spawn_amount": 1, "orientation": 61961, "x": 1, "y": 2, "z": 3}]}],
		[{"loaded_stage": 111, "spawn_types": [-1, -1, -1, -1]}, {"loaded_stage": 112, "spawn_types": [4, -1, -1, -1]}]
	],
	"area_mappings": [{"area_x": 1, "area_z": 2, "unk_08": 3, "base_x": 4, "base_z": 5, "kn_pos": 6, "unk_1c": 7}],
	"area_transitions": [
		{"transitions": [{"target_stage_id": 111, "current_x": 1, "transition_box": [1, 2, 3, 4, 5], "target_rotation": [32, 0]}]},
		{},
		{"empty_list": true}
	],
	"map_info": {"map_id": 2, "return_bc_id": 1},
	"gathering_points": [{"points": [{"x": 1, "range": 5, "gathering_id": 3, "max_count": 2, "unk_14": 1, "min_count": 1}]}],
	"area_facilities": [{}, {"points": [{"unk_00": 1, "type": 2, "x": 3, "range": 4, "id": 5, "unk_16": 6}]}],
	"some_string": "one", "quest_type_string": "two", "messages_extra": ["three"],
	"gathering_tables": [{"items": [{"rate": 50, "item": 3}]}, {}],
	"fishing_spots": [
		{"area": 110, "spots": [{"x": 1, "y": 2, "z": 3, "radius": 430, "kind": 1, "unk_14": 5}]},
		{"area": 1, "spots": [{"x": 1, "y": 2, "z": 3, "radius": 430, "kind": 1, "unk_14": 5}]}
	],
	"fish_tables": [
		{"tables": [
			{"count": 4, "catches": [[5, 11], [2, 23], [10, 33]]}, {"count": 3}, {"count": 5, "catches": [[60, 0]]},
			{"count": 4, "catches": [[5, 11], [2, 23], [10, 33]]}, {"count": 3}, {"count": 5}
		]},
		null
	]
}`

func TestRoundTrip_RecoveredSections(t *testing.T) {
	roundTrip(t, "recovered sections", fullQuestJSON)
}

func TestParseQuestBinary_RecoveredSections(t *testing.T) {
	var want QuestJSON
	if err := json.Unmarshal([]byte(fullQuestJSON), &want); err != nil {
		t.Fatal(err)
	}
	data, err := CompileQuestJSON([]byte(fullQuestJSON), "")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := ParseQuestBinary(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for name, pair := range map[string][2]any{
		"header_ext":             {want.HeaderExt, got.HeaderExt},
		"main_ext":               {want.MainExt, got.MainExt},
		"flow_script":            {want.FlowScript, got.FlowScript},
		"stages":                 {want.Stages, got.Stages},
		"monster_respawn_points": {want.MonsterRespawnPoints, got.MonsterRespawnPoints},
		"supply_main":            {want.SupplyMain, got.SupplyMain},
		"supply_extra":           {want.SupplyExtra, got.SupplyExtra},
		"rewards":                {want.Rewards, got.Rewards},
		"large_monsters":         {want.LargeMonsters, got.LargeMonsters},
		"quest_area":             {want.QuestArea, got.QuestArea},
		"area_mappings":          {want.AreaMappings, got.AreaMappings},
		"area_transitions":       {want.AreaTransitions, got.AreaTransitions},
		"gathering_points":       {want.GatheringPoints, got.GatheringPoints},
		"area_facilities":        {want.AreaFacilities, got.AreaFacilities},
		"messages_extra":         {want.MessagesExtra, got.MessagesExtra},
		"gathering_tables":       {want.GatheringTables, got.GatheringTables},
		"fishing_spots":          {want.FishingSpots, got.FishingSpots},
		"fish_tables":            {want.FishTables, got.FishTables},
		"objective_sub_a":        {want.ObjectiveSubA, got.ObjectiveSubA},
		"time_limit_frames":      {want.TimeLimitFrames, got.TimeLimitFrames},
	} {
		w, _ := json.Marshal(pair[0])
		g, _ := json.Marshal(pair[1])
		if !bytes.Equal(w, g) {
			t.Errorf("%s:\n got  %s\n want %s", name, g, w)
		}
	}

	// Identical fishing spot lists are shared, as in retail files.
	fs := int(binary.LittleEndian.Uint32(data[0x3C:]))
	if a, b := binary.LittleEndian.Uint32(data[fs+4:]), binary.LittleEndian.Uint32(data[fs+12:]); a != b {
		t.Errorf("identical spot lists not shared: 0x%X, 0x%X", a, b)
	}
}

func TestCompileQuestJSON_RomanNumeralsUseIBMCodes(t *testing.T) {
	b, err := toShiftJIS("Ⅰ-Ⅹ")
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0xFA, 0x4A, '-', 0xFA, 0x53, 0}; !bytes.Equal(b, want) {
		t.Errorf("toShiftJIS = % x, want % x", b, want)
	}
}

func TestCompileQuestJSON_LegacyMapSections(t *testing.T) {
	var q QuestJSON
	_ = json.Unmarshal([]byte(minimalQuestJSON), &q)
	q.MapSections = []QuestMapSectionJSON{{LoadedStage: 5, SpawnMonsters: []uint8{11, 12}}, {LoadedStage: 6}}
	b, _ := json.Marshal(q)
	data, err := CompileQuestJSON(b, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseQuestBinary(data)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]QuestMapSectionJSON{
		{{LoadedStage: 5, SpawnTypes: []int32{11, 12, -1, -1}}},
		{{LoadedStage: 6, SpawnTypes: []int32{-1, -1, -1, -1}}},
	}
	if !reflect.DeepEqual(got.QuestArea, want) {
		t.Errorf("QuestArea = %+v, want %+v", got.QuestArea, want)
	}
}

func TestObjTypeFromString(t *testing.T) {
	for s, want := range map[string]uint32{"": questObjNone, "slay": questObjSlay, "0x4001": 0x4001, "0x0": 0} {
		if got, err := objTypeFromString(s); err != nil || got != want {
			t.Errorf("objTypeFromString(%q) = 0x%X, %v; want 0x%X", s, got, err, want)
		}
	}
	for _, s := range []string{"bogus", "0x", "0xZZ"} {
		if _, err := objTypeFromString(s); err == nil {
			t.Errorf("objTypeFromString(%q): want error", s)
		}
	}
	if s := objTypeToString(0x4001); s != "0x4001" {
		t.Errorf("objTypeToString(0x4001) = %q", s)
	}
}

func TestCompileQuestJSON_FishTableValidation(t *testing.T) {
	var q QuestJSON
	_ = json.Unmarshal([]byte(minimalQuestJSON), &q)
	q.FishTables = []*QuestFishTableSetJSON{{Tables: make([]QuestFishTableJSON, 5)}}
	b, _ := json.Marshal(q)
	if _, err := CompileQuestJSON(b, ""); err == nil {
		t.Error("5 tables: want error")
	}
	q.FishTables = []*QuestFishTableSetJSON{{Tables: make([]QuestFishTableJSON, 6)}}
	q.FishTables[0].Tables[0].Catches = [][2]uint8{{0xFF, 1}}
	b, _ = json.Marshal(q)
	if _, err := CompileQuestJSON(b, ""); err == nil {
		t.Error("catch weight 255: want error")
	}
}

func TestClientQuestView_LayoutIndependent(t *testing.T) {
	data, err := CompileQuestJSON([]byte(fullQuestJSON), "")
	if err != nil {
		t.Fatal(err)
	}
	v1, err := ClientQuestView(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(v1.Unread) != 0 {
		t.Errorf("compiled quest has %d unread bytes, first at 0x%X", len(v1.Unread), v1.Unread[0])
	}

	// Move the map info block to the end of the file: same view.
	moved := append([]byte(nil), data...)
	mi := int(binary.LittleEndian.Uint32(moved[0x24:]))
	block := append([]byte(nil), moved[mi:mi+questMapInfoSize]...)
	copy(moved[mi:], make([]byte, questMapInfoSize))
	binary.LittleEndian.PutUint32(moved[0x24:], uint32(len(moved)))
	moved = append(moved, block...)
	v2, err := ClientQuestView(moved)
	if err != nil {
		t.Fatal(err)
	}
	if d := DiffQuestViews(v1, v2); d != "" {
		t.Errorf("moving a section changed the view: %s", d)
	}

	// Changing a value the client reads does change it.
	binary.LittleEndian.PutUint32(moved[len(moved)-questMapInfoSize:], 99)
	v3, _ := ClientQuestView(moved)
	if d := DiffQuestViews(v1, v3); d == "" {
		t.Error("changed map id: views still equal")
	} else if v1.Section(len(v1.Bytes)-1) == "" {
		t.Error("Section: empty name")
	}
}
