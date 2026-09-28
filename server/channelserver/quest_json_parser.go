package channelserver

import (
	"encoding/binary"
	"fmt"
	"math"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// questReader reads a quest binary with bounds checks. The first
// out-of-range read records an error and returns zero; callers check err
// once per section.
type questReader struct {
	d   []byte
	err error
}

func (r *questReader) ok(off, n int, what string) bool {
	if r.err != nil {
		return false
	}
	if off < 0 || n < 0 || off+n > len(r.d) {
		r.err = fmt.Errorf("%s: offset 0x%X len %d out of bounds (file len %d)", what, off, n, len(r.d))
		return false
	}
	return true
}

func (r *questReader) u8(off int) uint8 {
	if !r.ok(off, 1, "u8") {
		return 0
	}
	return r.d[off]
}

func (r *questReader) u16(off int) uint16 {
	if !r.ok(off, 2, "u16") {
		return 0
	}
	return binary.LittleEndian.Uint16(r.d[off:])
}

func (r *questReader) i16(off int) int16 { return int16(r.u16(off)) }

func (r *questReader) u32(off int) uint32 {
	if !r.ok(off, 4, "u32") {
		return 0
	}
	return binary.LittleEndian.Uint32(r.d[off:])
}

func (r *questReader) i32(off int) int32   { return int32(r.u32(off)) }
func (r *questReader) f32(off int) float32 { return math.Float32frombits(r.u32(off)) }

func (r *questReader) bytes(off, n int) []byte {
	if !r.ok(off, n, "bytes") {
		return nil
	}
	return r.d[off : off+n]
}

// sjis reads a null-terminated Shift-JIS string.
func (r *questReader) sjis(off int) (string, error) {
	if off < 0 || off >= len(r.d) {
		return "", fmt.Errorf("string offset 0x%X out of bounds", off)
	}
	end := off
	for end < len(r.d) && r.d[end] != 0 {
		end++
	}
	if end == off {
		return "", nil
	}
	utf8, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), r.d[off:end])
	if err != nil {
		return "", fmt.Errorf("shift-jis decode at 0x%X: %w", off, err)
	}
	return string(utf8), nil
}

// maxQuestListLen bounds terminator-ended lists, so a corrupt file fails
// instead of looping.
const maxQuestListLen = 4096

// ParseQuestBinary reads a MHF quest binary (ZZ, little-endian,
// uncompressed) into a QuestJSON that CompileQuestJSON turns back into an
// equivalent file. It walks the sections the way the client does (see
// quest_json_ext.go): list lengths come from the header counts and each
// list ends where the client stops reading.
func ParseQuestBinary(data []byte) (*QuestJSON, error) {
	if len(data) < questHeaderSize {
		return nil, fmt.Errorf("quest binary too short: %d bytes (minimum 0x%X)", len(data), questHeaderSize)
	}
	r := &questReader{d: data}
	q := &QuestJSON{}

	mainPtr := int(r.u32(0x00))
	if mainPtr == 0 {
		return nil, fmt.Errorf("questTypeFlagsPtr is null; cannot read main quest properties")
	}
	if err := parseQuestHeader(r, q); err != nil {
		return nil, err
	}
	if err := parseQuestMain(r, q, mainPtr); err != nil {
		return nil, err
	}

	steps := []struct {
		name string
		fn   func(*questReader, *QuestJSON) error
	}{
		{"flow script", parseFlowScript},
		{"stages", parseStages},
		{"monster respawn points", parseRespawnPoints},
		{"supply box", parseSupplyBox},
		{"rewards", parseRewards},
		{"large monsters", parseLargeMonsters},
		{"quest area", parseQuestArea},
		{"area mappings", parseAreaMappings},
		{"area transitions", parseAreaTransitions},
		{"map info", parseMapInfo},
		{"gathering points", parseGatheringPoints},
		{"area facilities", parseAreaFacilities},
		{"messages", parseMessages},
		{"gathering tables", parseGatheringTables},
		{"fishing spots", parseFishingSpots},
		{"fish tables", parseFishTables},
	}
	for _, s := range steps {
		if err := s.fn(r, q); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		if r.err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, r.err)
		}
	}
	return q, nil
}

func parseQuestHeader(r *questReader, q *QuestJSON) error {
	q.MonsterSizeMulti = r.u16(0x44)
	q.SizeRange = r.u16(0x46)
	q.StatTable1 = r.u32(0x48)
	q.MainRankPoints = r.u32(0x4C)
	q.SubARankPoints = r.u32(0x54)
	q.SubBRankPoints = r.u32(0x58)
	q.StatTable2 = r.u8(0x61)

	h := &QuestHeaderExtJSON{
		Unk50:        r.u32(0x50),
		Unk5C:        trimBytes(r.bytes(0x5C, 5)),
		EmQuestParam: r.i16(0x62),
		Unk74:        r.u16(0x74),
		Unk9C:        trimBytes(r.bytes(0x9C, questHeaderSize-0x9C)),
	}
	for i := 0; i < 2; i++ {
		o := 0x64 + i*8
		h.Unk64 = append(h.Unk64, QuestHeaderUnk64JSON{A: r.u32(o), B: r.u16(o + 4), C: r.u8(o + 6), D: r.u8(o + 7)})
	}
	if h.Unk64[0] == (QuestHeaderUnk64JSON{}) && h.Unk64[1] == (QuestHeaderUnk64JSON{}) {
		h.Unk64 = nil
	} else if h.Unk64[1] == (QuestHeaderUnk64JSON{}) {
		h.Unk64 = h.Unk64[:1]
	}
	h.Unk80 = trimUint16s([]uint16{r.u16(0x80), r.u16(0x82), r.u16(0x84), r.u16(0x86)})
	u88 := QuestHeaderUnk88JSON{
		A: r.u8(0x88), B: r.u8(0x89), C: r.u16(0x8A), D: r.u16(0x8C),
		E: r.u8(0x8E), F: r.u8(0x8F), G: r.u16(0x90), H: r.u16(0x92),
	}
	if u88 != (QuestHeaderUnk88JSON{}) {
		h.Unk88 = &u88
	}
	h.Unk94 = trimUint32s([]uint32{r.u32(0x94), r.u32(0x98)})
	if !isZero(*h) {
		q.HeaderExt = h
	}
	return r.err
}

func parseQuestMain(r *questReader, q *QuestJSON, mp int) error {
	if !r.ok(mp, questBodyLenZZ, "main quest properties") {
		return r.err
	}
	m := &QuestMainExtJSON{
		Flags:               r.u32(mp),
		Unk04:               trimBytes(r.bytes(mp+0x04, 4)),
		Unk0A:               trimBytes(r.bytes(mp+0x0A, 2)),
		Unk14:               r.u32(mp + 0x14),
		Unk1A:               r.u16(mp + 0x1A),
		Unk2C:               trimBytes(r.bytes(mp+0x2C, 2)),
		Unk48:               trimBytes(r.bytes(mp+0x48, 2)),
		Unk4A:               r.u16(mp + 0x4A),
		Unk8C:               r.u32(mp + 0x8C),
		MonsterVariants:     trimBytes(r.bytes(mp+0x90, 3)),
		MapVariant:          r.u8(mp + 0x93),
		RequiredItem:        r.u16(mp + 0x94),
		RequiredItemCount:   r.u8(mp + 0x96),
		Unk9B:               trimBytes(r.bytes(mp+0x9B, 5)),
		AllowedEquipBitmask: r.u32(mp + 0xA0),
		MainPoints:          r.u32(mp + 0xA4),
		SubAPoints:          r.u32(mp + 0xA8),
		SubBPoints:          r.u32(mp + 0xAC),
		RewardItems:         trimUint16s([]uint16{r.u16(mp + 0xB0), r.u16(mp + 0xB2), r.u16(mp + 0xB4)}),
		UnkB6:               trimBytes(r.bytes(mp+0xB6, 14)),
		QuestClearsAllowed:  r.u32(mp + 0xC4),
		UnkC8:               trimBytes(r.bytes(mp+0xC8, questBodyLenZZ-0xC8)),
	}

	q.RankBand = r.u16(mp + 0x08)
	q.Fee = r.u32(mp + 0x0C)
	q.RewardMain = r.u32(mp + 0x10)
	q.RewardSubA = r.u16(mp + 0x18)
	q.RewardSubB = r.u16(mp + 0x1C)
	q.HardHRReq = r.u16(mp + 0x1E)
	frames := r.u32(mp + 0x20)
	q.TimeLimitMinutes = frames / (60 * 30)
	if frames%(60*30) != 0 {
		q.TimeLimitFrames = frames
	}
	q.Map = r.u32(mp + 0x24)
	q.QuestID = r.u16(mp + 0x2E)
	objs := [4]QuestObjectiveJSON{}
	for i, off := range []int{0x30, 0x38, 0x40, 0x54} {
		objs[i] = parseObjective(r, mp+off)
	}
	q.ObjectiveMain, q.ObjectiveSubA, q.ObjectiveSubB = objs[0], objs[1], objs[2]
	if objs[3] != (QuestObjectiveJSON{Type: "none"}) {
		m.ObjectiveExtra = &objs[3]
	}
	q.JoinRankMin = r.u16(mp + 0x4C)
	q.JoinRankMax = r.u16(mp + 0x4E)
	q.PostRankMin = r.u16(mp + 0x50)
	q.PostRankMax = r.u16(mp + 0x52)

	eq := &QuestForcedEquipJSON{}
	off := mp + 0x5C
	for _, slot := range []*[4]uint16{&eq.Legs, &eq.Weapon, &eq.Head, &eq.Chest, &eq.Arms, &eq.Waist} {
		for j := range slot {
			slot[j] = r.u16(off)
			off += 2
		}
	}
	if *eq != (QuestForcedEquipJSON{}) {
		q.ForcedEquipment = eq
	}
	q.QuestVariant1 = r.u8(mp + 0x97)
	q.QuestVariant2 = r.u8(mp + 0x98)
	q.QuestVariant3 = r.u8(mp + 0x99)
	q.QuestVariant4 = r.u8(mp + 0x9A)
	if !isZero(*m) {
		q.MainExt = m
	}

	// Quest text: 8 string pointers. A few retail quests leave leftover
	// non-pointer data in optional slots; read those as empty.
	sp := int(r.u32(mp + 0x28))
	texts := make([]string, questStringCount)
	if sp != 0 && r.ok(sp, questStringCount*4, "quest text table") {
		for i := range texts {
			if p := int(r.u32(sp + i*4)); p != 0 {
				if s, err := r.sjis(p); err == nil {
					texts[i] = s
				}
			}
		}
	}
	q.Title = NewLocalizedPlain(texts[0])
	q.TextMain = NewLocalizedPlain(texts[1])
	q.TextSubA = NewLocalizedPlain(texts[2])
	q.TextSubB = NewLocalizedPlain(texts[3])
	q.SuccessCond = NewLocalizedPlain(texts[4])
	q.FailCond = NewLocalizedPlain(texts[5])
	q.Contractor = NewLocalizedPlain(texts[6])
	q.Description = NewLocalizedPlain(texts[7])
	return r.err
}

// parseObjective reads one 8-byte objective.
func parseObjective(r *questReader, off int) QuestObjectiveJSON {
	goalType := r.u32(off)
	obj := QuestObjectiveJSON{Type: objTypeToString(goalType), Target: r.u16(off + 4)}
	if goalType == questObjBreakPart {
		obj.Part = r.u16(off + 6)
	} else {
		obj.Count = r.u16(off + 6)
	}
	return obj
}

func parseFlowScript(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x10) & 0x7FFFFFFF)
	n := int(r.u16(0x76)) / (1 + questFlowArgCount)
	for i := 0; i < n; i++ {
		o := ptr + i*8
		args := []int16{r.i16(o + 2), r.i16(o + 4), r.i16(o + 6)}
		for len(args) > 0 && args[len(args)-1] == 0 {
			args = args[:len(args)-1]
		}
		q.FlowScript = append(q.FlowScript, QuestFlowOpJSON{Op: r.i16(o), Args: args})
	}
	return r.err
}

func parseStages(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x04))
	if ptr == 0 {
		return nil
	}
	for i := 0; i < questStageCount; i++ {
		o := ptr + i*16
		q.Stages = append(q.Stages, QuestStageJSON{StageID: r.u32(o), X: r.f32(o + 4), Y: r.f32(o + 8), Z: r.f32(o + 12)})
	}
	return r.err
}

func parseRespawnPoints(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x34))
	if ptr == 0 {
		return nil
	}
	for i := 0; i < maxQuestListLen && r.err == nil; i++ {
		o := ptr + i*8
		if r.i16(o) == -1 {
			return r.err
		}
		st := QuestRespawnStageJSON{Stage: r.u16(o)}
		count := int(r.u16(o + 2))
		pts := int(r.u32(o + 4))
		for j := 0; j < count; j++ {
			p := pts + j*16
			st.Points = append(st.Points, QuestRespawnPointJSON{Angle: r.u32(p), X: r.f32(p + 4), Y: r.f32(p + 8), Z: r.f32(p + 12)})
		}
		q.MonsterRespawnPoints = append(q.MonsterRespawnPoints, st)
	}
	return fmt.Errorf("list not terminated")
}

func parseSupplyBox(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x08))
	if ptr == 0 {
		return nil
	}
	read := func(off, n int) []QuestSupplyItemJSON {
		items := make([]QuestSupplyItemJSON, n)
		last := -1
		for i := range items {
			items[i] = QuestSupplyItemJSON{Item: r.u16(off + i*4), Quantity: r.u16(off + i*4 + 2)}
			if items[i] != (QuestSupplyItemJSON{}) {
				last = i
			}
		}
		return items[:last+1]
	}
	q.SupplyMain = read(ptr, 24)
	q.SupplySubA = read(ptr+24*4, 8)
	q.SupplySubB = read(ptr+32*4, 8)
	if extra := read(ptr+40*4, 1); len(extra) == 1 {
		q.SupplyExtra = &extra[0]
	}
	if len(q.SupplyMain) == 0 {
		q.SupplyMain = nil
	}
	if len(q.SupplySubA) == 0 {
		q.SupplySubA = nil
	}
	if len(q.SupplySubB) == 0 {
		q.SupplySubB = nil
	}
	return r.err
}

func parseRewards(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x0C))
	if ptr == 0 {
		return nil
	}
	for i := 0; i < maxQuestListLen && r.err == nil; i++ {
		o := ptr + i*8
		id := r.u16(o)
		if id == 0xFFFF {
			return r.err
		}
		t := QuestRewardTableJSON{TableID: uint8(id), TableFlags: uint8(id >> 8), Items: []QuestRewardItemJSON{}}
		for p, n := int(r.u32(o+4)), 0; n < maxQuestListLen && r.err == nil; p, n = p+6, n+1 {
			if r.u16(p) == 0xFFFF {
				break
			}
			t.Items = append(t.Items, QuestRewardItemJSON{Rate: r.u16(p), Item: r.u16(p + 2), Quantity: r.u16(p + 4)})
		}
		q.Rewards = append(q.Rewards, t)
	}
	return fmt.Errorf("list not terminated")
}

// readSpawns reads a 60-byte spawn list ending with monster -1.
func readSpawns(r *questReader, ptr int) ([]spawnRecord, error) {
	var out []spawnRecord
	for i := 0; i < maxQuestListLen && r.err == nil; i++ {
		o := ptr + i*questSpawnEntrySize
		if r.i16(o) == -1 {
			return out, r.err
		}
		out = append(out, spawnRecord{
			monster:     uint8(r.u16(o)),
			unk02:       r.u16(o + 2),
			amount:      r.u32(o + 4),
			unk08:       r.u32(o + 8),
			unk0C:       trimInt32s([]int32{r.i32(o + 0x0C), r.i32(o + 0x10), r.i32(o + 0x14), r.i32(o + 0x18)}),
			orientation: r.u32(o + 0x1C),
			x:           r.f32(o + 0x20),
			y:           r.f32(o + 0x24),
			z:           r.f32(o + 0x28),
			unk2C:       trimBytes(r.bytes(o+0x2C, 16)),
		})
	}
	if r.err != nil {
		return nil, r.err
	}
	return nil, fmt.Errorf("spawn list at 0x%X not terminated", ptr)
}

func parseLargeMonsters(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x18))
	if ptr == 0 || r.u32(ptr) == 0 {
		return r.err
	}
	spawns, err := readSpawns(r, int(r.u32(ptr+12)))
	if err != nil {
		return err
	}
	for _, s := range spawns {
		q.LargeMonsters = append(q.LargeMonsters, QuestMonsterJSON{
			ID: s.monster, Unk02: s.unk02, SpawnAmount: s.amount, SpawnStage: s.unk08,
			Unk0C: s.unk0C, Orientation: s.orientation, X: s.x, Y: s.y, Z: s.z, Unk2C: byteList(s.unk2C),
		})
	}
	return r.err
}

func parseQuestArea(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x14))
	if ptr == 0 {
		return nil
	}
	for g := 0; g < maxQuestListLen && r.err == nil; g++ {
		gp := int(r.u32(ptr + g*4))
		if gp == 0 {
			return r.err
		}
		group := []QuestMapSectionJSON{}
		for s := 0; s < maxQuestListLen && r.err == nil; s++ {
			o := gp + s*16
			stage := r.u32(o)
			if stage == 0 {
				break
			}
			ms := QuestMapSectionJSON{LoadedStage: stage, Unk04: r.u32(o + 4)}
			if tp := int(r.u32(o + 8)); tp != 0 {
				ms.SpawnTypes = []int32{r.i32(tp), r.i32(tp + 4), r.i32(tp + 8), r.i32(tp + 12)}
			}
			if sp := int(r.u32(o + 12)); sp != 0 {
				spawns, err := readSpawns(r, sp)
				if err != nil {
					return err
				}
				for _, sr := range spawns {
					ms.MinionSpawns = append(ms.MinionSpawns, QuestMinionSpawnJSON{
						Monster: sr.monster, SpawnToggle: sr.unk02, SpawnAmount: sr.amount, Unk08: sr.unk08,
						Unk0C: sr.unk0C, Orientation: sr.orientation, X: sr.x, Y: sr.y, Z: sr.z, Unk2C: byteList(sr.unk2C),
					})
				}
			}
			group = append(group, ms)
		}
		q.QuestArea = append(q.QuestArea, group)
	}
	return fmt.Errorf("list not terminated")
}

func parseAreaMappings(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x20))
	n := int(r.u8(0x7C))
	for i := 0; i < n && ptr != 0; i++ {
		o := ptr + i*32
		q.AreaMappings = append(q.AreaMappings, QuestAreaMappingJSON{
			AreaX: r.f32(o), AreaZ: r.f32(o + 4), Unk08: r.f32(o + 8), Unk0C: r.f32(o + 12),
			BaseX: r.f32(o + 16), BaseZ: r.f32(o + 20), KnPos: r.f32(o + 24), Unk1C: r.f32(o + 28),
		})
	}
	return r.err
}

func parseAreaTransitions(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x1C))
	n := int(r.u8(0x7F))
	for i := 0; i < n && ptr != 0; i++ {
		zone := QuestAreaTransitionsJSON{}
		for o, k := int(r.u32(ptr+i*4)), 0; o != 0 && k < maxQuestListLen && r.err == nil; o, k = o+52, k+1 {
			if r.i16(o) == -1 {
				zone.EmptyList = k == 0
				break
			}
			tr := QuestAreaTransitionJSON{
				TargetStageID1: r.i16(o), StageVariant: r.i16(o + 2),
				CurrentX: r.f32(o + 4), CurrentY: r.f32(o + 8), CurrentZ: r.f32(o + 12),
				TargetX: r.f32(o + 36), TargetY: r.f32(o + 40), TargetZ: r.f32(o + 44),
				TargetRotation: [2]int16{r.i16(o + 48), r.i16(o + 50)},
			}
			for j := range tr.TransitionBox {
				tr.TransitionBox[j] = r.f32(o + 16 + j*4)
			}
			zone.Transitions = append(zone.Transitions, tr)
		}
		q.AreaTransitions = append(q.AreaTransitions, zone)
	}
	return r.err
}

func parseMapInfo(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x24))
	if ptr == 0 {
		return nil
	}
	q.MapInfo = &QuestMapInfoJSON{MapID: r.u32(ptr), ReturnBCID: r.u32(ptr + 4), Unk08: r.u32(ptr + 8), Unk0C: r.u32(ptr + 12)}
	return r.err
}

func parseGatheringPoints(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x28))
	n := int(r.u8(0x7E))
	for i := 0; i < n && ptr != 0; i++ {
		zone := QuestAreaGatheringJSON{}
		// The client stops at a max count of 0; the terminator also has x = -1.
		for o, k := int(r.u32(ptr+i*4)), 0; o != 0 && k < maxQuestListLen && r.err == nil; o, k = o+24, k+1 {
			if r.u16(o+0x12) == 0 {
				zone.EmptyList = k == 0
				break
			}
			zone.Points = append(zone.Points, QuestGatheringPointJSON{
				X: r.f32(o), Y: r.f32(o + 4), Z: r.f32(o + 8), Range: r.f32(o + 12),
				GatheringID: r.u16(o + 16), MaxCount: r.u16(o + 18), Unk14: r.u16(o + 20), MinCount: r.u16(o + 22),
			})
		}
		q.GatheringPoints = append(q.GatheringPoints, zone)
	}
	return r.err
}

func parseAreaFacilities(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x2C))
	n := int(r.u8(0x7D))
	for i := 0; i < n && ptr != 0; i++ {
		zone := QuestAreaFacilitiesJSON{}
		for o, k := int(r.u32(ptr+i*4)), 0; o != 0 && k < maxQuestListLen && r.err == nil; o, k = o+24, k+1 {
			if r.u16(o+2) == 0 {
				zone.EmptyList = k == 0
				break
			}
			zone.Points = append(zone.Points, QuestFacilityPointJSON{
				Unk00: r.u16(o), Type: r.u16(o + 2), X: r.f32(o + 4), Y: r.f32(o + 8), Z: r.f32(o + 12),
				Range: r.f32(o + 16), ID: r.u16(o + 20), Unk16: r.u16(o + 22),
			})
		}
		q.AreaFacilities = append(q.AreaFacilities, zone)
	}
	return r.err
}

func parseMessages(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x30))
	if ptr == 0 {
		return nil
	}
	var msgs []string
	for i := 0; i < maxQuestListLen && r.err == nil; i++ {
		p := int(r.u32(ptr + i*4))
		if p == 0 || r.u8(p) == 0 {
			break
		}
		s, err := r.sjis(p)
		if err != nil {
			return err
		}
		msgs = append(msgs, s)
	}
	if len(msgs) > 0 {
		q.SomeString = msgs[0]
	}
	if len(msgs) > 1 {
		q.QuestType = msgs[1]
	}
	if len(msgs) > 2 {
		q.MessagesExtra = msgs[2:]
	}
	return r.err
}

func parseGatheringTables(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x38))
	n := int(r.u16(0x78))
	for i := 0; i < n && ptr != 0; i++ {
		tbl := QuestGatheringTableJSON{}
		for o, k := int(r.u32(ptr+i*4)), 0; o != 0 && k < maxQuestListLen && r.err == nil; o, k = o+4, k+1 {
			if r.u16(o) == 0xFFFF {
				tbl.EmptyList = k == 0
				break
			}
			tbl.Items = append(tbl.Items, QuestGatherItemJSON{Rate: r.u16(o), Item: r.u16(o + 2)})
		}
		q.GatheringTables = append(q.GatheringTables, tbl)
	}
	return r.err
}

func parseFishingSpots(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x3C))
	if ptr == 0 {
		return nil
	}
	for i := 0; i < maxQuestListLen && r.err == nil; i++ {
		o := ptr + i*8
		if r.u32(o) == 0 {
			return r.err
		}
		area := QuestFishingAreaJSON{Area: r.i32(o), Spots: []QuestFishingSpotJSON{}}
		for s, k := int(r.u32(o+4)), 0; k < maxQuestListLen && r.err == nil; s, k = s+24, k+1 {
			if r.i32(s+0x10) == -1 {
				break
			}
			area.Spots = append(area.Spots, QuestFishingSpotJSON{
				X: r.f32(s), Y: r.f32(s + 4), Z: r.f32(s + 8), Radius: r.f32(s + 12), Kind: r.i32(s + 16), Unk14: r.i32(s + 20),
			})
		}
		q.FishingSpots = append(q.FishingSpots, area)
	}
	return fmt.Errorf("list not terminated")
}

func parseFishTables(r *questReader, q *QuestJSON) error {
	ptr := int(r.u32(0x40))
	n := int(r.u16(0x7A))
	for i := 0; i < n && ptr != 0; i++ {
		sp := int(r.u32(ptr + i*4))
		// A few irregular arena quests hold an out-of-range value here;
		// it reads as no table.
		if sp == 0 || sp+questFishTablesPer*8 > len(r.d) {
			q.FishTables = append(q.FishTables, nil)
			continue
		}
		set := &QuestFishTableSetJSON{}
		for t := 0; t < questFishTablesPer; t++ {
			tbl := QuestFishTableJSON{Count: r.u32(sp + t*8 + 4)}
			for c, k := int(r.u32(sp+t*8)), 0; k < maxQuestListLen && r.err == nil; c, k = c+2, k+1 {
				if r.u8(c) == 0xFF {
					break
				}
				tbl.Catches = append(tbl.Catches, [2]uint8{r.u8(c), r.u8(c + 1)})
			}
			set.Tables = append(set.Tables, tbl)
		}
		q.FishTables = append(q.FishTables, set)
	}
	return r.err
}

// objTypeToString maps a goal type to its JSON name, or to its value in hex
// when it has no name.
func objTypeToString(t uint32) string {
	for name, v := range questObjTypeMap {
		if v == t {
			return name
		}
	}
	return fmt.Sprintf("0x%X", t)
}

func trimUint16s(v []uint16) []uint16 {
	n := len(v)
	for n > 0 && v[n-1] == 0 {
		n--
	}
	if n == 0 {
		return nil
	}
	return v[:n]
}

func trimUint32s(v []uint32) []uint32 {
	n := len(v)
	for n > 0 && v[n-1] == 0 {
		n--
	}
	if n == 0 {
		return nil
	}
	return v[:n]
}
