package channelserver

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// maxLargeMonsters is the most large monster spawns a retail quest has.
const maxLargeMonsters = 5

// questBuf is a little-endian byte builder with pointer patching.
type questBuf struct{ b []byte }

func (q *questBuf) off() int      { return len(q.b) }
func (q *questBuf) u8(v uint8)    { q.b = append(q.b, v) }
func (q *questBuf) u16(v uint16)  { q.b = binary.LittleEndian.AppendUint16(q.b, v) }
func (q *questBuf) i16(v int16)   { q.u16(uint16(v)) }
func (q *questBuf) u32(v uint32)  { q.b = binary.LittleEndian.AppendUint32(q.b, v) }
func (q *questBuf) i32(v int32)   { q.u32(uint32(v)) }
func (q *questBuf) f32(v float32) { q.u32(math.Float32bits(v)) }
func (q *questBuf) raw(b []byte)  { q.b = append(q.b, b...) }
func (q *questBuf) zero(n int)    { q.b = append(q.b, make([]byte, n)...) }

func (q *questBuf) align4() {
	for len(q.b)%4 != 0 {
		q.b = append(q.b, 0)
	}
}

// fixed writes b zero-padded or truncated to exactly n bytes.
func (q *questBuf) fixed(b []byte, n int) {
	if len(b) > n {
		b = b[:n]
	}
	q.raw(b)
	q.zero(n - len(b))
}

// reserve writes a u32 placeholder and returns its offset.
func (q *questBuf) reserve() int {
	o := len(q.b)
	q.u32(0)
	return o
}

// patchHere points the u32 at off to the current end of the buffer.
func (q *questBuf) patchHere(off int) { q.putU32(off, uint32(len(q.b))) }

func (q *questBuf) putU8(off int, v uint8)   { q.b[off] = v }
func (q *questBuf) putU16(off int, v uint16) { binary.LittleEndian.PutUint16(q.b[off:], v) }
func (q *questBuf) putU32(off int, v uint32) { binary.LittleEndian.PutUint32(q.b[off:], v) }

// CompileQuestJSON parses JSON quest data and compiles it to the MHF quest
// binary format (ZZ, little-endian, uncompressed) in the layout the client
// expects (see quest_json_ext.go): a 0xC0-byte header, the 320-byte main
// quest properties at 0xC0, then the quest strings and every section the
// client walks, each pointer valid and each list terminated. lang selects
// the text language (see LocalizedString.Resolve).
func CompileQuestJSON(data []byte, lang string) ([]byte, error) {
	var q QuestJSON
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parse quest JSON: %w", err)
	}
	if len(q.LargeMonsters) > maxLargeMonsters {
		return nil, fmt.Errorf("too many large monster spawns: %d (max %d)", len(q.LargeMonsters), maxLargeMonsters)
	}
	for _, n := range []struct {
		name string
		len  int
	}{
		{"area_mappings", len(q.AreaMappings)},
		{"area_facilities", len(q.AreaFacilities)},
		{"gathering_points", len(q.GatheringPoints)},
		{"area_transitions", len(q.AreaTransitions)},
	} {
		if n.len > 0xFF {
			return nil, fmt.Errorf("%s: %d entries (max 255)", n.name, n.len)
		}
	}

	objectives := make([][]byte, 0, 4)
	for _, obj := range []QuestObjectiveJSON{q.ObjectiveMain, q.ObjectiveSubA, q.ObjectiveSubB} {
		b, err := objectiveBytes(obj)
		if err != nil {
			return nil, err
		}
		objectives = append(objectives, b)
	}
	extraObjective := make([]byte, 8)
	if q.MainExt != nil && q.MainExt.ObjectiveExtra != nil {
		b, err := objectiveBytes(*q.MainExt.ObjectiveExtra)
		if err != nil {
			return nil, fmt.Errorf("objective_extra: %w", err)
		}
		extraObjective = b
	}

	texts := []LocalizedString{q.Title, q.TextMain, q.TextSubA, q.TextSubB,
		q.SuccessCond, q.FailCond, q.Contractor, q.Description}
	sjisTexts := make([][]byte, len(texts))
	for i, t := range texts {
		b, err := toShiftJIS(t.Resolve(lang))
		if err != nil {
			return nil, err
		}
		sjisTexts[i] = b
	}

	out := &questBuf{}
	out.zero(questHeaderSize)
	writeQuestHeader(out, &q)

	// ── Main quest properties (0xC0) ─────────────────────────────────────
	out.putU32(0x00, uint32(out.off()))
	mainOff := out.off()
	stringsPtrOff := writeQuestMain(out, &q, objectives, extraObjective)
	if out.off()-mainOff != questBodyLenZZ {
		return nil, fmt.Errorf("main quest properties: wrote %d bytes, want %d", out.off()-mainOff, questBodyLenZZ)
	}

	// ── Quest text ───────────────────────────────────────────────────────
	out.patchHere(stringsPtrOff)
	textPtrs := make([]int, len(sjisTexts))
	for i := range sjisTexts {
		textPtrs[i] = out.reserve()
	}
	for i, s := range sjisTexts {
		out.patchHere(textPtrs[i])
		out.raw(s)
	}
	out.align4()

	// ── Flow script (0x10) ───────────────────────────────────────────────
	// Bit 31 marks the file as not yet byte-swapped; every retail quest has
	// it, and the Wii U client converts only files that do.
	out.putU32(0x10, uint32(out.off())|0x80000000)
	for _, op := range q.FlowScript {
		out.i16(op.Op)
		for i := 0; i < questFlowArgCount; i++ {
			var a int16
			if i < len(op.Args) {
				a = op.Args[i]
			}
			out.i16(a)
		}
	}

	// ── Stages (0x04): one per player ────────────────────────────────────
	out.putU32(0x04, uint32(out.off()))
	for i := 0; i < questStageCount; i++ {
		st := QuestStageJSON{}
		if i < len(q.Stages) {
			st = q.Stages[i]
		} else if len(q.Stages) > 0 {
			st = q.Stages[len(q.Stages)-1]
		}
		out.u32(st.StageID)
		out.f32(st.X)
		out.f32(st.Y)
		out.f32(st.Z)
	}

	// ── Monster respawn points (0x34) ────────────────────────────────────
	out.putU32(0x34, uint32(out.off()))
	respawnPtrs := make([]int, len(q.MonsterRespawnPoints))
	for i, st := range q.MonsterRespawnPoints {
		out.u16(st.Stage)
		out.u16(uint16(len(st.Points)))
		respawnPtrs[i] = out.reserve()
	}
	out.u16(0xFFFF)
	out.zero(6)
	for i, st := range q.MonsterRespawnPoints {
		out.patchHere(respawnPtrs[i])
		for _, p := range st.Points {
			out.u32(p.Angle)
			out.f32(p.X)
			out.f32(p.Y)
			out.f32(p.Z)
		}
	}

	// ── Supply box (0x08): 24 main + 8 sub A + 8 sub B + 1 slots ─────────
	out.putU32(0x08, uint32(out.off()))
	writeSupplySlots(out, q.SupplyMain, 24)
	writeSupplySlots(out, q.SupplySubA, 8)
	writeSupplySlots(out, q.SupplySubB, 8)
	if q.SupplyExtra != nil {
		out.u16(q.SupplyExtra.Item)
		out.u16(q.SupplyExtra.Quantity)
	} else {
		out.u32(0)
	}

	// ── Reward tables (0x0C) ─────────────────────────────────────────────
	out.putU32(0x0C, uint32(out.off()))
	rewardPtrs := make([]int, len(q.Rewards))
	for i, t := range q.Rewards {
		out.u16(uint16(t.TableFlags)<<8 | uint16(t.TableID))
		out.zero(2)
		rewardPtrs[i] = out.reserve()
	}
	out.u16(0xFFFF)
	out.zero(6)
	for i, t := range q.Rewards {
		out.patchHere(rewardPtrs[i])
		for _, item := range t.Items {
			out.u16(item.Rate)
			out.u16(item.Item)
			out.u16(item.Quantity)
		}
		out.u16(0xFFFF)
	}
	out.align4()

	// ── Large monsters (0x18): one spawn section ─────────────────────────
	out.putU32(0x18, uint32(out.off()))
	out.u32(1) // stage slot, 1 in every retail quest
	out.u32(0)
	lmIDs := out.reserve()
	lmSpawns := out.reserve()
	out.zero(16) // section list terminator
	out.patchHere(lmIDs)
	// 8 × i32: the monster IDs (the client reads the first 4), then -1, -1,
	// 0 as in every retail quest.
	ids := [8]int32{5: -1, 6: -1}
	for i, m := range q.LargeMonsters {
		ids[i] = int32(m.ID)
	}
	for _, v := range ids {
		out.i32(v)
	}
	out.patchHere(lmSpawns)
	for _, m := range q.LargeMonsters {
		writeSpawn(out, spawnRecord{
			monster: m.ID, unk02: m.Unk02, amount: m.SpawnAmount, unk08: m.SpawnStage,
			unk0C: m.Unk0C, orientation: m.Orientation, x: m.X, y: m.Y, z: m.Z, unk2C: m.Unk2C,
		})
	}
	for i := len(q.LargeMonsters); i < questLargeMonsterSlots; i++ {
		writeSpawnTerminator(out)
	}

	// ── Quest area (0x14): groups of map sections ────────────────────────
	out.putU32(0x14, uint32(out.off()))
	groups := q.QuestArea
	if len(groups) == 0 {
		for _, ms := range q.MapSections {
			groups = append(groups, []QuestMapSectionJSON{ms})
		}
	}
	groupPtrs := make([]int, len(groups))
	for i := range groups {
		groupPtrs[i] = out.reserve()
	}
	out.u32(0)
	for gi, group := range groups {
		out.patchHere(groupPtrs[gi])
		type secPtrs struct{ types, spawns int }
		ptrs := make([]secPtrs, len(group))
		for si, ms := range group {
			out.u32(ms.LoadedStage)
			out.u32(ms.Unk04)
			ptrs[si].types = out.reserve()
			ptrs[si].spawns = out.reserve()
		}
		out.zero(16)
		for si, ms := range group {
			out.patchHere(ptrs[si].types)
			for _, v := range sectionSpawnTypes(ms) {
				out.i32(v)
			}
			out.patchHere(ptrs[si].spawns)
			for _, m := range ms.MinionSpawns {
				writeSpawn(out, spawnRecord{
					monster: m.Monster, unk02: m.SpawnToggle, amount: m.SpawnAmount, unk08: m.Unk08,
					unk0C: m.Unk0C, orientation: m.Orientation, x: m.X, y: m.Y, z: m.Z, unk2C: m.Unk2C,
				})
			}
			writeSpawnTerminator(out)
		}
	}

	// ── Area mappings (0x20), count at 0x7C ──────────────────────────────
	out.putU32(0x20, uint32(out.off()))
	out.putU8(0x7C, uint8(len(q.AreaMappings)))
	for _, am := range q.AreaMappings {
		for _, f := range []float32{am.AreaX, am.AreaZ, am.Unk08, am.Unk0C, am.BaseX, am.BaseZ, am.KnPos, am.Unk1C} {
			out.f32(f)
		}
	}

	// ── Area transitions (0x1C), count at 0x7F ───────────────────────────
	out.putU32(0x1C, uint32(out.off()))
	out.putU8(0x7F, uint8(len(q.AreaTransitions)))
	transPtrs := make([]int, len(q.AreaTransitions))
	for i := range q.AreaTransitions {
		transPtrs[i] = out.reserve()
	}
	for i, zone := range q.AreaTransitions {
		if len(zone.Transitions) == 0 && !zone.EmptyList {
			continue
		}
		out.patchHere(transPtrs[i])
		for _, tr := range zone.Transitions {
			out.i16(tr.TargetStageID1)
			out.i16(tr.StageVariant)
			for _, f := range []float32{tr.CurrentX, tr.CurrentY, tr.CurrentZ} {
				out.f32(f)
			}
			for _, f := range tr.TransitionBox {
				out.f32(f)
			}
			for _, f := range []float32{tr.TargetX, tr.TargetY, tr.TargetZ} {
				out.f32(f)
			}
			out.i16(tr.TargetRotation[0])
			out.i16(tr.TargetRotation[1])
		}
		out.raw(questTransitionTerminator)
	}

	// ── Map info (0x24) ──────────────────────────────────────────────────
	out.putU32(0x24, uint32(out.off()))
	mi := QuestMapInfoJSON{}
	if q.MapInfo != nil {
		mi = *q.MapInfo
	}
	out.u32(mi.MapID)
	out.u32(mi.ReturnBCID)
	out.u32(mi.Unk08)
	out.u32(mi.Unk0C)

	// ── Gathering points (0x28), count at 0x7E ───────────────────────────
	out.putU32(0x28, uint32(out.off()))
	out.putU8(0x7E, uint8(len(q.GatheringPoints)))
	gpPtrs := make([]int, len(q.GatheringPoints))
	for i := range q.GatheringPoints {
		gpPtrs[i] = out.reserve()
	}
	for i, zone := range q.GatheringPoints {
		if len(zone.Points) == 0 && !zone.EmptyList {
			continue
		}
		out.patchHere(gpPtrs[i])
		for _, gp := range zone.Points {
			out.f32(gp.X)
			out.f32(gp.Y)
			out.f32(gp.Z)
			out.f32(gp.Range)
			out.u16(gp.GatheringID)
			out.u16(gp.MaxCount)
			out.u16(gp.Unk14)
			out.u16(gp.MinCount)
		}
		out.f32(-1)
		out.zero(20)
	}

	// ── Area facilities (0x2C), count at 0x7D ────────────────────────────
	out.putU32(0x2C, uint32(out.off()))
	out.putU8(0x7D, uint8(len(q.AreaFacilities)))
	facPtrs := make([]int, len(q.AreaFacilities))
	for i := range q.AreaFacilities {
		facPtrs[i] = out.reserve()
	}
	for i, zone := range q.AreaFacilities {
		if len(zone.Points) == 0 && !zone.EmptyList {
			continue
		}
		out.patchHere(facPtrs[i])
		for _, fp := range zone.Points {
			out.u16(fp.Unk00)
			out.u16(fp.Type)
			out.f32(fp.X)
			out.f32(fp.Y)
			out.f32(fp.Z)
			out.f32(fp.Range)
			out.u16(fp.ID)
			out.u16(fp.Unk16)
		}
		out.raw(questFacilityTerminator)
	}

	// ── Messages (0x30): pointer list ending with a null pointer ─────────
	out.putU32(0x30, uint32(out.off()))
	messages := questMessages(&q)
	msgPtrs := make([]int, len(messages))
	for i := range messages {
		msgPtrs[i] = out.reserve()
	}
	out.u32(0)
	for i, m := range messages {
		b, err := toShiftJIS(m)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		out.patchHere(msgPtrs[i])
		out.raw(b)
	}
	out.align4()

	// ── Gathering tables (0x38), count at 0x78 ───────────────────────────
	out.putU32(0x38, uint32(out.off()))
	out.putU16(0x78, uint16(len(q.GatheringTables)))
	tblPtrs := make([]int, len(q.GatheringTables))
	for i := range q.GatheringTables {
		tblPtrs[i] = out.reserve()
	}
	for i, tbl := range q.GatheringTables {
		if len(tbl.Items) == 0 && !tbl.EmptyList {
			continue
		}
		out.patchHere(tblPtrs[i])
		for _, item := range tbl.Items {
			out.u16(item.Rate)
			out.u16(item.Item)
		}
		out.u16(0xFFFF)
	}
	out.align4()

	// ── Fishing spots (0x3C) ─────────────────────────────────────────────
	out.putU32(0x3C, uint32(out.off()))
	spotPtrs := make([]int, len(q.FishingSpots))
	for i, a := range q.FishingSpots {
		out.i32(a.Area)
		spotPtrs[i] = out.reserve()
	}
	out.zero(8)
	spotLists := map[string]uint32{} // identical lists are shared, as in retail files
	for i, a := range q.FishingSpots {
		key := fmt.Sprint(a.Spots)
		if at, ok := spotLists[key]; ok {
			out.putU32(spotPtrs[i], at)
			continue
		}
		spotLists[key] = uint32(out.off())
		out.patchHere(spotPtrs[i])
		for _, s := range a.Spots {
			out.f32(s.X)
			out.f32(s.Y)
			out.f32(s.Z)
			out.f32(s.Radius)
			out.i32(s.Kind)
			out.i32(s.Unk14)
		}
		out.zero(16)
		out.i32(-1)
		out.zero(4)
	}

	// ── Fish catch tables (0x40), count at 0x7A ──────────────────────────
	out.putU32(0x40, uint32(out.off()))
	out.putU16(0x7A, uint16(len(q.FishTables)))
	setPtrs := make([]int, len(q.FishTables))
	for i := range q.FishTables {
		setPtrs[i] = out.reserve()
	}
	type tablePtr struct {
		off     int
		catches [][2]uint8
	}
	var catchPtrs []tablePtr
	for i, set := range q.FishTables {
		if set == nil {
			continue
		}
		if len(set.Tables) != questFishTablesPer {
			return nil, fmt.Errorf("fish_tables[%d]: %d tables, want %d", i, len(set.Tables), questFishTablesPer)
		}
		out.patchHere(setPtrs[i])
		for _, t := range set.Tables {
			catchPtrs = append(catchPtrs, tablePtr{out.reserve(), t.Catches})
			out.u32(t.Count)
		}
	}
	catchLists := map[string]uint32{}
	for _, cp := range catchPtrs {
		key := fmt.Sprint(cp.catches)
		if at, ok := catchLists[key]; ok {
			out.putU32(cp.off, at)
			continue
		}
		catchLists[key] = uint32(out.off())
		out.patchHere(cp.off)
		for _, c := range cp.catches {
			if c[0] == 0xFF {
				return nil, fmt.Errorf("fish catch weight 255 would end the table")
			}
			out.u8(c[0])
			out.u8(c[1])
		}
		out.u8(0xFF)
	}
	out.align4()

	return out.b, nil
}

// writeQuestHeader fills the header fields (0x44–0xBF). Pointers and the
// counts derived from section lengths are patched in as sections are written.
func writeQuestHeader(out *questBuf, q *QuestJSON) {
	h := q.HeaderExt
	if h == nil {
		h = &QuestHeaderExtJSON{}
	}
	out.putU16(0x44, q.MonsterSizeMulti)
	out.putU16(0x46, q.SizeRange)
	out.putU32(0x48, q.StatTable1)
	out.putU32(0x4C, q.MainRankPoints)
	out.putU32(0x50, h.Unk50)
	out.putU32(0x54, q.SubARankPoints)
	out.putU32(0x58, q.SubBRankPoints)
	copy(out.b[0x5C:0x61], h.Unk5C)
	out.putU8(0x61, q.StatTable2)
	out.putU16(0x62, uint16(h.EmQuestParam))
	for i, e := range h.Unk64 {
		if i >= 2 {
			break
		}
		o := 0x64 + i*8
		out.putU32(o, e.A)
		out.putU16(o+4, e.B)
		out.putU8(o+6, e.C)
		out.putU8(o+7, e.D)
	}
	out.putU16(0x74, h.Unk74)
	out.putU16(0x76, uint16(len(q.FlowScript)*(1+questFlowArgCount)))
	for i, v := range h.Unk80 {
		if i < 4 {
			out.putU16(0x80+i*2, v)
		}
	}
	if u := h.Unk88; u != nil {
		out.putU8(0x88, u.A)
		out.putU8(0x89, u.B)
		out.putU16(0x8A, u.C)
		out.putU16(0x8C, u.D)
		out.putU8(0x8E, u.E)
		out.putU8(0x8F, u.F)
		out.putU16(0x90, u.G)
		out.putU16(0x92, u.H)
	}
	for i, v := range h.Unk94 {
		if i < 2 {
			out.putU32(0x94+i*4, v)
		}
	}
	unk9C := []byte(h.Unk9C)
	if len(unk9C) > questHeaderSize-0x9C {
		unk9C = unk9C[:questHeaderSize-0x9C]
	}
	copy(out.b[0x9C:], unk9C)
}

// writeQuestMain writes the 320-byte main quest properties and returns the
// offset of the quest text pointer, patched once the text is placed.
func writeQuestMain(out *questBuf, q *QuestJSON, objectives [][]byte, extraObjective []byte) int {
	m := q.MainExt
	if m == nil {
		m = &QuestMainExtJSON{}
	}
	frames := q.TimeLimitMinutes * 60 * 30
	if q.TimeLimitFrames != 0 {
		frames = q.TimeLimitFrames
	}
	out.u32(m.Flags)            // +0x00
	out.fixed(m.Unk04, 4)       // +0x04
	out.u16(q.RankBand)         // +0x08
	out.fixed(m.Unk0A, 2)       // +0x0A
	out.u32(q.Fee)              // +0x0C
	out.u32(q.RewardMain)       // +0x10
	out.u32(m.Unk14)            // +0x14
	out.u16(q.RewardSubA)       // +0x18
	out.u16(m.Unk1A)            // +0x1A
	out.u16(q.RewardSubB)       // +0x1C
	out.u16(q.HardHRReq)        // +0x1E
	out.u32(frames)             // +0x20
	out.u32(q.Map)              // +0x24
	stringsPtr := out.reserve() // +0x28
	out.fixed(m.Unk2C, 2)       // +0x2C
	out.u16(q.QuestID)          // +0x2E
	for _, b := range objectives {
		out.raw(b) // +0x30, 3 × 8 bytes
	}
	out.fixed(m.Unk48, 2)   // +0x48
	out.u16(m.Unk4A)        // +0x4A
	out.u16(q.JoinRankMin)  // +0x4C
	out.u16(q.JoinRankMax)  // +0x4E
	out.u16(q.PostRankMin)  // +0x50
	out.u16(q.PostRankMax)  // +0x52
	out.raw(extraObjective) // +0x54
	eq := q.ForcedEquipment
	if eq == nil {
		eq = &QuestForcedEquipJSON{}
	}
	for _, slot := range [][4]uint16{eq.Legs, eq.Weapon, eq.Head, eq.Chest, eq.Arms, eq.Waist} {
		for _, v := range slot {
			out.u16(v) // +0x5C, 6 × 4 × u16
		}
	}
	out.u32(m.Unk8C)                // +0x8C
	out.fixed(m.MonsterVariants, 3) // +0x90
	out.u8(m.MapVariant)            // +0x93
	out.u16(m.RequiredItem)         // +0x94
	out.u8(m.RequiredItemCount)     // +0x96
	out.u8(q.QuestVariant1)         // +0x97
	out.u8(q.QuestVariant2)
	out.u8(q.QuestVariant3)
	out.u8(q.QuestVariant4)
	out.fixed(m.Unk9B, 5)          // +0x9B
	out.u32(m.AllowedEquipBitmask) // +0xA0
	out.u32(m.MainPoints)          // +0xA4
	out.u32(m.SubAPoints)          // +0xA8
	out.u32(m.SubBPoints)          // +0xAC
	for i := 0; i < 3; i++ {       // +0xB0
		var v uint16
		if i < len(m.RewardItems) {
			v = m.RewardItems[i]
		}
		out.u16(v)
	}
	out.fixed(m.UnkB6, 14)        // +0xB6
	out.u32(m.QuestClearsAllowed) // +0xC4
	out.fixed(m.UnkC8, questBodyLenZZ-0xC8)
	return stringsPtr
}

// questMessages returns the flow script messages in order.
func questMessages(q *QuestJSON) []string {
	var msgs []string
	if q.SomeString != "" || q.QuestType != "" || len(q.MessagesExtra) > 0 {
		msgs = append(msgs, q.SomeString)
	}
	if q.QuestType != "" || len(q.MessagesExtra) > 0 {
		msgs = append(msgs, q.QuestType)
	}
	return append(msgs, q.MessagesExtra...)
}

func writeSupplySlots(out *questBuf, items []QuestSupplyItemJSON, n int) {
	for i := 0; i < n; i++ {
		if i < len(items) {
			out.u16(items[i].Item)
			out.u16(items[i].Quantity)
		} else {
			out.u32(0)
		}
	}
}

// sectionSpawnTypes returns the 4-slot spawn type block of a map section,
// from SpawnTypes or, for older JSON, from SpawnMonsters.
func sectionSpawnTypes(ms QuestMapSectionJSON) []int32 {
	out := []int32{-1, -1, -1, -1}
	if len(ms.SpawnTypes) > 0 {
		copy(out, ms.SpawnTypes)
		return out
	}
	for i, id := range ms.SpawnMonsters {
		if i < len(out) {
			out[i] = int32(id)
		}
	}
	return out
}

// spawnRecord is the 60-byte spawn layout shared by large monsters and
// map section spawns (client type 0xA4).
type spawnRecord struct {
	monster     uint8
	unk02       uint16
	amount      uint32
	unk08       uint32
	unk0C       []int32
	orientation uint32
	x, y, z     float32
	unk2C       []byte
}

func writeSpawn(out *questBuf, s spawnRecord) {
	out.u16(uint16(s.monster)) // +0x00
	out.u16(s.unk02)           // +0x02
	out.u32(s.amount)          // +0x04
	out.u32(s.unk08)           // +0x08
	for i := 0; i < 4; i++ {   // +0x0C
		var v int32
		if i < len(s.unk0C) {
			v = s.unk0C[i]
		}
		out.i32(v)
	}
	out.u32(s.orientation) // +0x1C
	out.f32(s.x)           // +0x20
	out.f32(s.y)
	out.f32(s.z)
	out.fixed(s.unk2C, 16) // +0x2C
}

// writeSpawnTerminator ends a spawn list: monster -1.
func writeSpawnTerminator(out *questBuf) {
	out.u16(0xFFFF)
	out.zero(questSpawnEntrySize - 2)
}
