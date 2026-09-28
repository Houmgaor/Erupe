package channelserver

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// Objective type constants matching questObjType in questfile.bin.hexpat.
const (
	questObjNone         = uint32(0x00000000)
	questObjHunt         = uint32(0x00000001)
	questObjDeliver      = uint32(0x00000002)
	questObjEsoteric     = uint32(0x00000010)
	questObjCapture      = uint32(0x00000101)
	questObjSlay         = uint32(0x00000201)
	questObjDeliverFlag  = uint32(0x00001002)
	questObjBreakPart    = uint32(0x00004004)
	questObjDamage       = uint32(0x00008004)
	questObjSlayOrDamage = uint32(0x00018004)
	questObjSlayTotal    = uint32(0x00020000)
	questObjSlayAll      = uint32(0x00040000)
)

var questObjTypeMap = map[string]uint32{
	"none":           questObjNone,
	"hunt":           questObjHunt,
	"deliver":        questObjDeliver,
	"esoteric":       questObjEsoteric,
	"capture":        questObjCapture,
	"slay":           questObjSlay,
	"deliver_flag":   questObjDeliverFlag,
	"break_part":     questObjBreakPart,
	"damage":         questObjDamage,
	"slay_or_damage": questObjSlayOrDamage,
	"slay_total":     questObjSlayTotal,
	"slay_all":       questObjSlayAll,
}

// ---- JSON schema types ----

// QuestObjectiveJSON represents a single quest objective.
type QuestObjectiveJSON struct {
	// Type is one of: none, hunt, capture, slay, deliver, deliver_flag,
	// break_part, damage, slay_or_damage, slay_total, slay_all, esoteric.
	Type string `json:"type"`
	// Target is a monster ID for hunt/capture/slay/break_part/damage,
	// or an item ID for deliver/deliver_flag.
	Target uint16 `json:"target"`
	// Count is the quantity required (hunts, item count, etc.).
	Count uint16 `json:"count"`
	// Part is the monster part ID for break_part objectives.
	Part uint16 `json:"part,omitempty"`
}

// QuestRewardItemJSON is one entry in a reward table.
type QuestRewardItemJSON struct {
	Rate     uint16 `json:"rate"`
	Item     uint16 `json:"item"`
	Quantity uint16 `json:"quantity"`
}

// QuestRewardTableJSON is a named reward table with its items.
type QuestRewardTableJSON struct {
	TableID uint8 `json:"table_id"`
	// TableFlags is the high byte of the u16 table ID (0x80 in most retail
	// tables).
	TableFlags uint8                 `json:"table_flags,omitempty"`
	Items      []QuestRewardItemJSON `json:"items"`
}

// QuestMonsterJSON describes one large monster spawn.
//
// It is a 60-byte spawn record (client type 0xA4), the same layout as
// QuestMinionSpawnJSON; the Unk fields hold the parts with no known meaning.
type QuestMonsterJSON struct {
	ID          uint8    `json:"id"`
	Unk02       uint16   `json:"unk_02,omitempty"`
	SpawnAmount uint32   `json:"spawn_amount"`
	SpawnStage  uint32   `json:"spawn_stage"`
	Unk0C       []int32  `json:"unk_0c,omitempty"` // 4 × i32 at +0x0C
	Orientation uint32   `json:"orientation"`
	X           float32  `json:"x"`
	Y           float32  `json:"y"`
	Z           float32  `json:"z"`
	Unk2C       byteList `json:"unk_2c,omitempty"` // 16 bytes at +0x2C
}

// QuestSupplyItemJSON is one supply box entry.
type QuestSupplyItemJSON struct {
	Item     uint16 `json:"item"`
	Quantity uint16 `json:"quantity"`
}

// QuestStageJSON is a loaded stage definition.
//
// There is one entry per player (Quest_pl_stage_init); X/Y/Z is where that
// player starts.
type QuestStageJSON struct {
	StageID uint32  `json:"stage_id"`
	X       float32 `json:"x,omitempty"`
	Y       float32 `json:"y,omitempty"`
	Z       float32 `json:"z,omitempty"`
}

// QuestForcedEquipJSON defines forced equipment per slot.
// Each slot is [equipment_id, attach1, attach2, attach3].
// Zero values mean no restriction.
type QuestForcedEquipJSON struct {
	Legs   [4]uint16 `json:"legs,omitempty"`
	Weapon [4]uint16 `json:"weapon,omitempty"`
	Head   [4]uint16 `json:"head,omitempty"`
	Chest  [4]uint16 `json:"chest,omitempty"`
	Arms   [4]uint16 `json:"arms,omitempty"`
	Waist  [4]uint16 `json:"waist,omitempty"`
}

// QuestMinionSpawnJSON is one minion spawn entry within a map section.
type QuestMinionSpawnJSON struct {
	Monster     uint8    `json:"monster"`
	SpawnToggle uint16   `json:"spawn_toggle"`
	SpawnAmount uint32   `json:"spawn_amount"`
	Unk08       uint32   `json:"unk_08,omitempty"`
	Unk0C       []int32  `json:"unk_0c,omitempty"` // 4 × i32 at +0x0C
	Orientation uint32   `json:"orientation,omitempty"`
	X           float32  `json:"x"`
	Y           float32  `json:"y"`
	Z           float32  `json:"z"`
	Unk2C       byteList `json:"unk_2c,omitempty"` // 16 bytes at +0x2C
}

// QuestMapSectionJSON defines one map section with its minion spawns.
// Each section corresponds to a loaded stage area.
//
// Layout (client type 0xA6): u32 stage, u32 unk, pointer to 4 × i32 spawn
// types, pointer to a spawn list ending with monster -1.
type QuestMapSectionJSON struct {
	LoadedStage uint32 `json:"loaded_stage"`
	Unk04       uint32 `json:"unk_04,omitempty"`
	// SpawnTypes is the 4-slot spawn type block (monster IDs, -1 when
	// unused). When empty, SpawnMonsters is used instead.
	SpawnTypes    []int32                `json:"spawn_types,omitempty"`
	SpawnMonsters []uint8                `json:"spawn_monsters,omitempty"` // legacy: monster IDs for the spawn type block
	MinionSpawns  []QuestMinionSpawnJSON `json:"minion_spawns,omitempty"`
}

// QuestAreaTransitionJSON is one zone transition (floatSet).
type QuestAreaTransitionJSON struct {
	TargetStageID1 int16      `json:"target_stage_id"`
	StageVariant   int16      `json:"stage_variant"`
	CurrentX       float32    `json:"current_x"`
	CurrentY       float32    `json:"current_y"`
	CurrentZ       float32    `json:"current_z"`
	TransitionBox  [5]float32 `json:"transition_box"`
	TargetX        float32    `json:"target_x"`
	TargetY        float32    `json:"target_y"`
	TargetZ        float32    `json:"target_z"`
	TargetRotation [2]int16   `json:"target_rotation"`
}

// QuestAreaTransitionsJSON holds the transitions for one area zone entry.
// The pointer may be null (empty transitions list) for zones without transitions.
// A zone with no transitions is a null pointer, unless EmptyList is set
// (a pointer to an empty list, as in a few retail quests).
type QuestAreaTransitionsJSON struct {
	Transitions []QuestAreaTransitionJSON `json:"transitions,omitempty"`
	EmptyList   bool                      `json:"empty_list,omitempty"`
}

// QuestAreaMappingJSON defines coordinate mappings between area and base map.
// Layout: 32 bytes per entry (Area_xPos, Area_zPos, pad8, Base_xPos, Base_zPos, kn_Pos, pad4).
// The client reads 8 floats (type 0x60); Unk08, Unk0C and Unk1C are the
// three hexpat treats as padding.
type QuestAreaMappingJSON struct {
	AreaX float32 `json:"area_x"`
	AreaZ float32 `json:"area_z"`
	Unk08 float32 `json:"unk_08,omitempty"`
	Unk0C float32 `json:"unk_0c,omitempty"`
	BaseX float32 `json:"base_x"`
	BaseZ float32 `json:"base_z"`
	KnPos float32 `json:"kn_pos"`
	Unk1C float32 `json:"unk_1c,omitempty"`
}

// QuestMapInfoJSON contains the map ID and return base camp ID.
// The block is 4 × i32; the last two are zero in every retail quest.
type QuestMapInfoJSON struct {
	MapID      uint32 `json:"map_id"`
	ReturnBCID uint32 `json:"return_bc_id"`
	Unk08      uint32 `json:"unk_08,omitempty"`
	Unk0C      uint32 `json:"unk_0c,omitempty"`
}

// QuestGatheringPointJSON is one gathering point (24 bytes).
type QuestGatheringPointJSON struct {
	X           float32 `json:"x"`
	Y           float32 `json:"y"`
	Z           float32 `json:"z"`
	Range       float32 `json:"range"`
	GatheringID uint16  `json:"gathering_id"`
	MaxCount    uint16  `json:"max_count"`
	Unk14       uint16  `json:"unk_14,omitempty"`
	MinCount    uint16  `json:"min_count"`
}

// QuestAreaGatheringJSON holds up to 4 gathering points for one area zone entry.
// A nil/empty list means the pointer is null for this zone.
type QuestAreaGatheringJSON struct {
	Points    []QuestGatheringPointJSON `json:"points,omitempty"`
	EmptyList bool                      `json:"empty_list,omitempty"` // see QuestAreaTransitionsJSON
}

// QuestFacilityPointJSON is one facility point (24 bytes, facPoint in hexpat).
type QuestFacilityPointJSON struct {
	Unk00 uint16  `json:"unk_00,omitempty"`
	Type  uint16  `json:"type"` // SpecAc: 1=cooking, 2=fishing, 3=bluebox, etc.
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	Z     float32 `json:"z"`
	Range float32 `json:"range"`
	ID    uint16  `json:"id"`
	Unk16 uint16  `json:"unk_16,omitempty"`
}

// QuestAreaFacilitiesJSON holds the facilities block for one area zone entry.
// A nil/empty list means the pointer is null for this zone.
type QuestAreaFacilitiesJSON struct {
	Points    []QuestFacilityPointJSON `json:"points,omitempty"`
	EmptyList bool                     `json:"empty_list,omitempty"` // see QuestAreaTransitionsJSON
}

// QuestGatherItemJSON is one entry in a gathering table.
type QuestGatherItemJSON struct {
	Rate uint16 `json:"rate"`
	Item uint16 `json:"item"`
}

// QuestGatheringTableJSON is one gathering loot table.
// A table with no items is a null pointer unless EmptyList is set.
type QuestGatheringTableJSON struct {
	Items     []QuestGatherItemJSON `json:"items,omitempty"`
	EmptyList bool                  `json:"empty_list,omitempty"`
}

// QuestJSON is the human-readable quest definition.
// Time values: TimeLimitMinutes is converted to frames (×30×60) in the binary.
// Strings: encoded as UTF-8 here, converted to Shift-JIS in the binary.
type QuestJSON struct {
	// Quest identification
	QuestID uint16 `json:"quest_id"`

	// Text (UTF-8; converted to Shift-JIS in binary).
	//
	// Each field accepts either a plain JSON string (single-language, treated
	// as the value for every language) or a language-keyed object:
	//
	//	"title": "リオレウス"
	//	"title": { "jp": "リオレウス", "en": "Rathalos", "fr": "Rathalos" }
	//
	// CompileQuestJSON resolves these based on the compiling session's
	// language preference (see #188 phase B).
	Title       LocalizedString `json:"title"`
	Description LocalizedString `json:"description"`
	TextMain    LocalizedString `json:"text_main"`
	TextSubA    LocalizedString `json:"text_sub_a"`
	TextSubB    LocalizedString `json:"text_sub_b"`
	SuccessCond LocalizedString `json:"success_cond"`
	FailCond    LocalizedString `json:"fail_cond"`
	Contractor  LocalizedString `json:"contractor"`

	// General quest properties (generalQuestProperties section, 0x44–0x85)
	MonsterSizeMulti uint16 `json:"monster_size_multi"` // 100 = 100%
	SizeRange        uint16 `json:"size_range"`
	StatTable1       uint32 `json:"stat_table_1,omitempty"`
	StatTable2       uint8  `json:"stat_table_2,omitempty"`
	MainRankPoints   uint32 `json:"main_rank_points"`
	SubARankPoints   uint32 `json:"sub_a_rank_points"`
	SubBRankPoints   uint32 `json:"sub_b_rank_points"`

	// Main quest properties
	Fee              uint32 `json:"fee"`
	RewardMain       uint32 `json:"reward_main"`
	RewardSubA       uint16 `json:"reward_sub_a"`
	RewardSubB       uint16 `json:"reward_sub_b"`
	TimeLimitMinutes uint32 `json:"time_limit_minutes"`
	// TimeLimitFrames, when set, is the exact limit in 30 Hz frames and
	// overrides TimeLimitMinutes (for limits that aren't whole minutes).
	TimeLimitFrames uint32 `json:"time_limit_frames,omitempty"`
	Map             uint32 `json:"map"`
	RankBand        uint16 `json:"rank_band"`
	HardHRReq       uint16 `json:"hard_hr_req,omitempty"`
	JoinRankMin     uint16 `json:"join_rank_min,omitempty"`
	JoinRankMax     uint16 `json:"join_rank_max,omitempty"`
	PostRankMin     uint16 `json:"post_rank_min,omitempty"`
	PostRankMax     uint16 `json:"post_rank_max,omitempty"`

	// Quest variant flags (see handlers_quest.go makeEventQuest comments)
	QuestVariant1 uint8 `json:"quest_variant1,omitempty"`
	QuestVariant2 uint8 `json:"quest_variant2,omitempty"`
	QuestVariant3 uint8 `json:"quest_variant3,omitempty"`
	QuestVariant4 uint8 `json:"quest_variant4,omitempty"`

	// Objectives
	ObjectiveMain QuestObjectiveJSON `json:"objective_main"`
	ObjectiveSubA QuestObjectiveJSON `json:"objective_sub_a,omitempty"`
	ObjectiveSubB QuestObjectiveJSON `json:"objective_sub_b,omitempty"`

	// Monster spawns
	LargeMonsters []QuestMonsterJSON `json:"large_monsters,omitempty"`

	// Reward tables
	Rewards []QuestRewardTableJSON `json:"rewards,omitempty"`

	// Supply box (main: up to 24, sub_a/sub_b: up to 8 each). Slots keep
	// their position: an entry with item 0 is an empty slot.
	SupplyMain []QuestSupplyItemJSON `json:"supply_main,omitempty"`
	SupplySubA []QuestSupplyItemJSON `json:"supply_sub_a,omitempty"`
	SupplySubB []QuestSupplyItemJSON `json:"supply_sub_b,omitempty"`
	// SupplyExtra is the 41st slot of the 164-byte supply box.
	SupplyExtra *QuestSupplyItemJSON `json:"supply_extra,omitempty"`

	// Loaded stages
	Stages []QuestStageJSON `json:"stages,omitempty"`

	// Forced equipment (optional)
	ForcedEquipment *QuestForcedEquipJSON `json:"forced_equipment,omitempty"`

	// QuestArea (questAreaPtr) is a list of groups, each a list of map
	// sections with their spawns. Retail quests have 3 groups.
	QuestArea [][]QuestMapSectionJSON `json:"quest_area,omitempty"`
	// MapSections is the older flat form: each section is its own group.
	// Used only when QuestArea is empty.
	MapSections []QuestMapSectionJSON `json:"map_sections,omitempty"`

	// Area transitions per zone (areaTransitionsPtr); one entry per zone.
	// Area mappings, transitions, gathering points and facilities each have
	// their own count in the header (0x7C–0x7F).
	AreaTransitions []QuestAreaTransitionsJSON `json:"area_transitions,omitempty"`

	// Area coordinate mappings (areaMappingPtr)
	AreaMappings []QuestAreaMappingJSON `json:"area_mappings,omitempty"`

	// Map info: map ID + return base camp ID (mapInfoPtr)
	MapInfo *QuestMapInfoJSON `json:"map_info,omitempty"`

	// Per-zone gathering points (gatheringPointsPtr); one entry per zone.
	GatheringPoints []QuestAreaGatheringJSON `json:"gathering_points,omitempty"`

	// Per-zone area facilities (areaFacilitiesPtr); one entry per zone.
	AreaFacilities []QuestAreaFacilitiesJSON `json:"area_facilities,omitempty"`

	// Messages (someStringsPtr, header 0x30) shown by the flow script.
	// SomeString and QuestType are the first two, MessagesExtra the rest.
	SomeString    string   `json:"some_string,omitempty"`
	QuestType     string   `json:"quest_type_string,omitempty"`
	MessagesExtra []string `json:"messages_extra,omitempty"`

	// Gathering loot tables (gatheringTablesPtr)
	GatheringTables []QuestGatheringTableJSON `json:"gathering_tables,omitempty"`

	// Sections recovered from the client (#40), see quest_json_ext.go.
	HeaderExt            *QuestHeaderExtJSON      `json:"header_ext,omitempty"`
	MainExt              *QuestMainExtJSON        `json:"main_ext,omitempty"`
	FlowScript           []QuestFlowOpJSON        `json:"flow_script,omitempty"`
	MonsterRespawnPoints []QuestRespawnStageJSON  `json:"monster_respawn_points,omitempty"`
	FishingSpots         []QuestFishingAreaJSON   `json:"fishing_spots,omitempty"`
	FishTables           []*QuestFishTableSetJSON `json:"fish_tables,omitempty"`
}

// toShiftJIS converts a UTF-8 string to a null-terminated Shift-JIS byte slice.
// ASCII-only strings pass through unchanged.
//
// The text is CP932, which gives Roman numerals Ⅰ–Ⅹ two codes: NEC
// 0x8754–0x875D (what the encoder and Windows pick) and IBM 0xFA4A–0xFA53.
// Retail quest text only uses the IBM ones, so they are written that way.
func toShiftJIS(s string) ([]byte, error) {
	var out []byte
	start := 0
	flush := func(end int) error {
		if end > start {
			b, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(s[start:end]))
			if err != nil {
				return fmt.Errorf("shift-jis encode %q: %w", s, err)
			}
			out = append(out, b...)
		}
		return nil
	}
	for i, r := range s {
		if r >= 'Ⅰ' && r <= 'Ⅹ' {
			if err := flush(i); err != nil {
				return nil, err
			}
			out = append(out, 0xFA, byte(0x4A+r-'Ⅰ'))
			start = i + len(string(r))
		}
	}
	if err := flush(len(s)); err != nil {
		return nil, err
	}
	return append(out, 0x00), nil
}

// objectiveBytes serialises one QuestObjectiveJSON to 8 bytes (client type
// 0x9C): u32 goal type, u16 target, u16 count (or part for break_part).
// Type is a name from questObjTypeMap or, for a type with no name, its
// value in hex ("0x4001").
func objectiveBytes(obj QuestObjectiveJSON) ([]byte, error) {
	goalType, err := objTypeFromString(obj.Type)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, goalType)
	binary.LittleEndian.PutUint16(b[4:], obj.Target)
	second := obj.Count
	if goalType == questObjBreakPart {
		second = obj.Part
	}
	binary.LittleEndian.PutUint16(b[6:], second)
	return b, nil
}

// objTypeFromString maps an objective type name, or a hex value, to its
// goal type.
func objTypeFromString(s string) (uint32, error) {
	if s == "" {
		return questObjNone, nil
	}
	if v, ok := questObjTypeMap[s]; ok {
		return v, nil
	}
	if strings.HasPrefix(s, "0x") {
		if v, err := strconv.ParseUint(s[2:], 16, 32); err == nil {
			return uint32(v), nil
		}
	}
	return 0, fmt.Errorf("unknown objective type %q", s)
}
