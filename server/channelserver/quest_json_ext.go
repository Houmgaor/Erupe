package channelserver

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Quest file sections and fields recovered from the client (#40).
//
// Layouts come from the Wii U build (mhfo-wiiu.rpx, which keeps its debug
// symbols): cv_quest walks every section of a quest file to byte-swap it,
// and _SwapEndianStructure's type descriptors give the width of every field.
// Meanings come from the functions that read each section; a field named
// unk_XX has a known width but no known meaning. The PC ZZ main quest
// properties are longer than the Wii U ones (320 vs 160 bytes); from +0x5C
// on they follow questfile.bin.hexpat, checked against retail files.
// docs/quest-file-format.md has the full map.

// byteList is a byte array that marshals as a JSON array of numbers rather
// than base64, so unknown byte runs stay readable and editable.
type byteList []uint8

func (b byteList) MarshalJSON() ([]byte, error) {
	ints := make([]int, len(b))
	for i, v := range b {
		ints[i] = int(v)
	}
	return json.Marshal(ints)
}

func (b *byteList) UnmarshalJSON(data []byte) error {
	var ints []int
	if err := json.Unmarshal(data, &ints); err != nil {
		return err
	}
	out := make(byteList, len(ints))
	for i, v := range ints {
		if v < 0 || v > 0xFF {
			return fmt.Errorf("byte value %d out of range", v)
		}
		out[i] = uint8(v)
	}
	*b = out
	return nil
}

// trimBytes drops trailing zero bytes, so all-zero runs are omitted.
func trimBytes(b []byte) byteList {
	n := len(b)
	for n > 0 && b[n-1] == 0 {
		n--
	}
	if n == 0 {
		return nil
	}
	return append(byteList(nil), b[:n]...)
}

// isZero reports whether v equals its type's zero value.
func isZero[T any](v T) bool {
	var zero T
	return reflect.DeepEqual(v, zero)
}

// trimInt32s drops trailing zeros.
func trimInt32s(v []int32) []int32 {
	n := len(v)
	for n > 0 && v[n-1] == 0 {
		n--
	}
	if n == 0 {
		return nil
	}
	return append([]int32(nil), v[:n]...)
}

// QuestHeaderExtJSON holds the header fields (0x44–0xBF) not covered by
// QuestJSON's named fields. The header is 0xC0 bytes (client type 0xC2);
// counts at 0x76–0x7F are derived from the section lengths.
type QuestHeaderExtJSON struct {
	Unk50 uint32   `json:"unk_50,omitempty"`
	Unk5C byteList `json:"unk_5c,omitempty"` // 5 × u8 at 0x5C–0x60
	// EmQuestParam (0x62) is read by em_quest_datprm, used by monster
	// damage and demo-skip code.
	EmQuestParam int16                  `json:"em_quest_param,omitempty"`
	Unk64        []QuestHeaderUnk64JSON `json:"unk_64,omitempty"` // 2 entries at 0x64
	Unk74        uint16                 `json:"unk_74,omitempty"`
	Unk80        []uint16               `json:"unk_80,omitempty"` // 4 × u16 at 0x80
	Unk88        *QuestHeaderUnk88JSON  `json:"unk_88,omitempty"`
	Unk94        []uint32               `json:"unk_94,omitempty"` // 2 × u32 at 0x94
	Unk9C        byteList               `json:"unk_9c,omitempty"` // 36 bytes at 0x9C
}

// QuestHeaderUnk64JSON is one 8-byte entry at header 0x64 (client type 0x9E).
type QuestHeaderUnk64JSON struct {
	A uint32 `json:"a,omitempty"`
	B uint16 `json:"b,omitempty"`
	C uint8  `json:"c,omitempty"`
	D uint8  `json:"d,omitempty"`
}

// QuestHeaderUnk88JSON is the 12-byte struct at header 0x88 (client type 0xC0).
type QuestHeaderUnk88JSON struct {
	A uint8  `json:"a,omitempty"`
	B uint8  `json:"b,omitempty"`
	C uint16 `json:"c,omitempty"`
	D uint16 `json:"d,omitempty"`
	E uint8  `json:"e,omitempty"`
	F uint8  `json:"f,omitempty"`
	G uint16 `json:"g,omitempty"`
	H uint16 `json:"h,omitempty"`
}

// QuestMainExtJSON holds the main quest property fields (320 bytes at
// questTypeFlagsPtr) not covered by QuestJSON's named fields.
type QuestMainExtJSON struct {
	Flags uint32   `json:"flags,omitempty"`  // +0x00
	Unk04 byteList `json:"unk_04,omitempty"` // 4 × u8
	Unk0A byteList `json:"unk_0a,omitempty"` // 2 × u8
	Unk14 uint32   `json:"unk_14,omitempty"` // carts or reward reduction (hexpat)
	Unk1A uint16   `json:"unk_1a,omitempty"` // high half of the sub A reward
	Unk2C byteList `json:"unk_2c,omitempty"` // 2 × u8
	Unk48 byteList `json:"unk_48,omitempty"` // 2 × u8
	Unk4A uint16   `json:"unk_4a,omitempty"`
	// ObjectiveExtra (+0x54) has the objective layout (client type 0x9C).
	ObjectiveExtra      *QuestObjectiveJSON `json:"objective_extra,omitempty"`
	Unk8C               uint32              `json:"unk_8c,omitempty"`
	MonsterVariants     byteList            `json:"monster_variants,omitempty"` // 3 × u8 at +0x90
	MapVariant          uint8               `json:"map_variant,omitempty"`
	RequiredItem        uint16              `json:"required_item,omitempty"`
	RequiredItemCount   uint8               `json:"required_item_count,omitempty"`
	Unk9B               byteList            `json:"unk_9b,omitempty"` // 5 × u8
	AllowedEquipBitmask uint32              `json:"allowed_equip_bitmask,omitempty"`
	MainPoints          uint32              `json:"main_points,omitempty"`
	SubAPoints          uint32              `json:"sub_a_points,omitempty"`
	SubBPoints          uint32              `json:"sub_b_points,omitempty"`
	RewardItems         []uint16            `json:"reward_items,omitempty"` // 3 × u16 at +0xB0
	UnkB6               byteList            `json:"unk_b6,omitempty"`       // 14 bytes, interception settings (hexpat)
	QuestClearsAllowed  uint32              `json:"quest_clears_allowed,omitempty"`
	UnkC8               byteList            `json:"unk_c8,omitempty"` // +0xC8 to the end of the block
}

// QuestFlowOpJSON is one instruction of the quest flow script (header
// 0x10): an opcode and three 16-bit arguments, run by quest_condition_prog.
// Known opcodes: -1 success and -2 failure (entry points found by
// quest_presuccess_ptr_set / quest_failed_ptr_set), 0 check clear,
// 2 quest_target_set(a, b, c), 11 show message a (index into the
// message list, header 0x30), 26 label a, 36 and 48 check target monster
// counts, 37 time over, 38 pre-success.
type QuestFlowOpJSON struct {
	Op   int16   `json:"op"`
	Args []int16 `json:"args,omitempty"`
}

// QuestRespawnStageJSON lists where a monster may respawn on one stage
// (header 0x34): em_revival_rnd_pos_set picks one point at random.
type QuestRespawnStageJSON struct {
	Stage  uint16                  `json:"stage"`
	Points []QuestRespawnPointJSON `json:"points"`
}

// QuestRespawnPointJSON is one respawn point (client type 0xA0). Angle is
// copied to the monster's facing.
type QuestRespawnPointJSON struct {
	Angle uint32  `json:"angle,omitempty"`
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	Z     float32 `json:"z"`
}

// QuestFishingAreaJSON lists fishing spots for one area (header 0x3C,
// read by Fish_pos_data_get).
type QuestFishingAreaJSON struct {
	Area  int32                  `json:"area"`
	Spots []QuestFishingSpotJSON `json:"spots"`
}

// QuestFishingSpotJSON is one fishing spot (client type 0xAE). Kind selects
// the catch tables in FishTables.
type QuestFishingSpotJSON struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Z      float32 `json:"z"`
	Radius float32 `json:"radius"`
	Kind   int32   `json:"kind"`
	Unk14  int32   `json:"unk_14,omitempty"`
}

// QuestFishTableSetJSON is one fish catch table set (header 0x40, read by
// Fish_sel_data_get). Sets are indexed kind*3 + rank (0 low, 1 high,
// 2 G rank); each has 6 tables indexed day_night*3 + season.
type QuestFishTableSetJSON struct {
	Tables []QuestFishTableJSON `json:"tables"`
}

// QuestFishTableJSON is one catch table: weighted (weight, fish) pairs.
type QuestFishTableJSON struct {
	Count   uint32     `json:"count"`
	Catches [][2]uint8 `json:"catches,omitempty"`
}

const (
	questHeaderSize     = 0xC0
	questFishTablesPer  = 6
	questFlowArgCount   = 3
	questStageCount     = 4
	questSupplySlots    = 41 // 24 main + 8 sub A + 8 sub B + 1
	questMapInfoSize    = 16
	questSpawnEntrySize = 60
	// questLargeMonsterSlots is the fixed large monster slot count: up to 5
	// monsters, unused slots holding the list terminator.
	questLargeMonsterSlots = 6
)

// questTransitionTerminator ends every retail area transition list:
// target stage -1, variant -1, x -1.0.
var questTransitionTerminator = func() []byte {
	b := make([]byte, 52)
	copy(b, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x80, 0xBF})
	return b
}()

// questFacilityTerminator is the entry every retail quest ends a facility
// list with (type 0 stops the client's walk).
var questFacilityTerminator = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0xBF, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x68, 0x21, 0x46, 0x00, 0x00, 0xA0, 0x42, 0x00, 0x00, 0x00, 0x00,
}
