package channelserver

import (
	"bytes"
	"fmt"
	"sort"
)

// QuestView is what the client reads from a quest file, independent of
// where each section sits: the walk follows every pointer the way cv_quest
// does (see quest_json_ext.go) and records the bytes it reaches, with
// pointers replaced by their targets. Two files with the same view give the
// client the same quest, so this is what a .bin → JSON → .bin round trip
// must preserve.
type QuestView struct {
	Bytes  []byte
	labels []viewLabel
	// Unread lists the offsets of non-zero bytes the walk never reached:
	// data no section accounts for.
	Unread []int
}

type viewLabel struct {
	pos  int
	name string
}

// Section returns the name of the section that produced Bytes[pos].
func (v *QuestView) Section(pos int) string {
	i := sort.Search(len(v.labels), func(i int) bool { return v.labels[i].pos > pos })
	if i == 0 {
		return "?"
	}
	return v.labels[i-1].name
}

// DiffQuestViews describes the first difference between two views, or
// returns "" when they are equal.
func DiffQuestViews(want, got *QuestView) string {
	if bytes.Equal(want.Bytes, got.Bytes) {
		return ""
	}
	i := 0
	for i < len(want.Bytes) && i < len(got.Bytes) && want.Bytes[i] == got.Bytes[i] {
		i++
	}
	return fmt.Sprintf("%s differs (view byte %d, %d vs %d bytes)", want.Section(i), i, len(got.Bytes), len(want.Bytes))
}

type questWalker struct {
	r    *questReader
	view *QuestView
	seen []bool
}

func (w *questWalker) label(name string) {
	w.view.labels = append(w.view.labels, viewLabel{len(w.view.Bytes), name})
}

// take appends n bytes at off to the view.
func (w *questWalker) take(off, n int) {
	b := w.r.bytes(off, n)
	if b == nil {
		return
	}
	w.view.Bytes = append(w.view.Bytes, b...)
	for i := off; i < off+n; i++ {
		w.seen[i] = true
	}
}

// ptr reads a pointer field: it marks the field read but keeps the value
// out of the view, since only its target matters.
func (w *questWalker) ptr(off int) int {
	v := w.r.u32(off)
	if w.r.err == nil {
		for i := off; i < off+4; i++ {
			w.seen[i] = true
		}
	}
	return int(v)
}

// end appends a list-end marker.
func (w *questWalker) end() { w.view.Bytes = append(w.view.Bytes, 0xEE) }

// list walks entries of size step from off until stop reports the
// terminator, calling each for every entry; the terminator's first
// termLen bytes are part of the view.
// It returns the terminator's offset, or 0.
func (w *questWalker) list(off, step, termLen int, stop func(int) bool, each func(int)) int {
	if off == 0 {
		w.view.Bytes = append(w.view.Bytes, 0x00) // null: distinct from an empty list
		return 0
	}
	for k := 0; off != 0 && k < maxQuestListLen && w.r.err == nil; k, off = k+1, off+step {
		if stop(off) {
			w.take(off, termLen)
			w.end()
			return off
		}
		each(off)
	}
	if w.r.err == nil && off != 0 {
		w.r.err = fmt.Errorf("list at 0x%X not terminated", off)
	}
	return 0
}

// skip marks n bytes at off as structure the client never reads, keeping
// them out of the view.
func (w *questWalker) skip(off, n int) {
	for i := off; i < off+n && i < len(w.seen); i++ {
		w.seen[i] = true
	}
}

// ClientQuestView walks a quest file the way the client does.
func ClientQuestView(data []byte) (*QuestView, error) {
	if len(data) < questHeaderSize {
		return nil, fmt.Errorf("quest binary too short: %d bytes", len(data))
	}
	w := &questWalker{r: &questReader{d: data}, view: &QuestView{}, seen: make([]bool, len(data))}
	r := w.r

	w.label("header")
	var hp [17]int
	for i := range hp {
		hp[i] = w.ptr(i * 4)
	}
	w.view.Bytes = append(w.view.Bytes, byte(uint32(hp[4])>>31)) // not-yet-swapped flag
	w.take(0x44, questHeaderSize-0x44)

	w.label("main quest properties")
	mp := hp[0]
	w.take(mp, 0x28)
	sp := w.ptr(mp + 0x28)
	w.take(mp+0x2C, questBodyLenZZ-0x2C)

	w.label("quest text")
	if sp != 0 && r.ok(sp, questStringCount*4, "quest text table") {
		for i := 0; i < questStringCount; i++ {
			// Null or leftover slots read as empty text.
			if p := w.ptr(sp + i*4); p > 0 && p < len(data) {
				w.cstring(p)
			} else {
				w.view.Bytes = append(w.view.Bytes, 0)
			}
		}
	}

	w.label("flow script")
	w.take(hp[4]&0x7FFFFFFF, int(r.u16(0x76))*2)

	w.label("stages")
	w.take(hp[1], questStageCount*16)

	w.label("monster respawn points")
	w.list(hp[13], 8, 2, func(o int) bool { return r.i16(o) == -1 }, func(o int) {
		w.take(o, 4)
		w.take(w.ptr(o+4), int(r.u16(o+2))*16)
	})

	w.label("supply box")
	w.take(hp[2], questSupplySlots*4)

	w.label("rewards")
	w.list(hp[3], 8, 2, func(o int) bool { return r.u16(o) == 0xFFFF }, func(o int) {
		w.take(o, 4)
		w.list(w.ptr(o+4), 6, 2, func(p int) bool { return r.u16(p) == 0xFFFF }, func(p int) { w.take(p, 6) })
	})

	spawnSections := func(off, typesLen, slots int) {
		// A section list ends with stage 0; the rest of that entry is
		// leftover data from the authoring tool (often not a valid offset).
		term := w.list(off, 16, 4, func(o int) bool { return r.u32(o) == 0 }, func(o int) {
			w.take(o, 8)
			if t := w.ptr(o + 8); t != 0 {
				w.take(t, typesLen)
			}
			sp := w.ptr(o + 12)
			n := 0
			w.list(sp, questSpawnEntrySize, 2, func(s int) bool { return r.i16(s) == -1 }, func(s int) {
				w.take(s, questSpawnEntrySize)
				n++
			})
			// Fixed slot blocks fill unused slots with terminators; the
			// client stops at the first, so their contents are never read.
			for k := n + 1; sp != 0 && k < slots && r.i16(sp+k*questSpawnEntrySize) == -1; k++ {
				w.skip(sp+k*questSpawnEntrySize, questSpawnEntrySize)
			}
			if n < slots && sp != 0 {
				w.skip(sp+n*questSpawnEntrySize, questSpawnEntrySize)
			}
		})
		if term != 0 {
			w.skip(term, 16)
		}
	}
	w.label("large monsters")
	spawnSections(hp[6], 32, questLargeMonsterSlots)

	w.label("quest area")
	w.list(hp[5], 4, 4, func(o int) bool { return r.u32(o) == 0 }, func(o int) { spawnSections(w.ptr(o), 16, 0) })

	w.label("area mappings")
	if hp[8] != 0 {
		w.take(hp[8], int(r.u8(0x7C))*32)
	}

	w.label("area transitions")
	w.perZone(hp[7], int(r.u8(0x7F)), 52, 52, func(o int) bool { return r.i16(o) == -1 })

	w.label("map info")
	if hp[9] != 0 {
		w.take(hp[9], questMapInfoSize)
	}

	w.label("gathering points")
	w.perZone(hp[10], int(r.u8(0x7E)), 24, 24, func(o int) bool { return r.u16(o+0x12) == 0 })

	w.label("area facilities")
	w.perZone(hp[11], int(r.u8(0x7D)), 24, 24, func(o int) bool { return r.u16(o+2) == 0 })

	w.label("messages")
	if hp[12] != 0 {
		for i := 0; i < maxQuestListLen && r.err == nil; i++ {
			p := w.ptr(hp[12] + i*4)
			if p == 0 || r.u8(p) == 0 {
				break
			}
			w.cstring(p)
		}
		w.end()
	}

	w.label("gathering tables")
	for i := 0; hp[14] != 0 && i < int(r.u16(0x78)); i++ {
		w.list(w.ptr(hp[14]+i*4), 4, 2, func(o int) bool { return r.u16(o) == 0xFFFF }, func(o int) { w.take(o, 4) })
	}

	w.label("fishing spots")
	w.list(hp[15], 8, 4, func(o int) bool { return r.u32(o) == 0 }, func(o int) {
		w.take(o, 4)
		w.list(w.ptr(o+4), 24, 24, func(s int) bool { return r.i32(s+0x10) == -1 }, func(s int) { w.take(s, 24) })
	})

	w.label("fish tables")
	for i := 0; hp[16] != 0 && i < int(r.u16(0x7A)); i++ {
		set := w.ptr(hp[16] + i*4)
		if set == 0 || set+questFishTablesPer*8 > len(data) {
			w.end()
			continue
		}
		for t := 0; t < questFishTablesPer; t++ {
			w.list(w.ptr(set+t*8), 2, 1, func(c int) bool { return r.u8(c) == 0xFF }, func(c int) { w.take(c, 2) })
			w.take(set+t*8+4, 4)
		}
	}

	if r.err != nil {
		return nil, r.err
	}
	for i, b := range data {
		if b != 0 && !w.seen[i] {
			w.view.Unread = append(w.view.Unread, i)
		}
	}
	return w.view, nil
}

// perZone walks a count-sized pointer array whose entries point to lists
// (or are null, read the same as an empty list).
func (w *questWalker) perZone(arr, n, step, termLen int, stop func(int) bool) {
	for i := 0; arr != 0 && i < n; i++ {
		w.list(w.ptr(arr+i*4), step, termLen, stop, func(o int) { w.take(o, step) })
		if w.r.err != nil {
			return
		}
	}
}

// cstring appends a null-terminated string, terminator included.
func (w *questWalker) cstring(off int) {
	end := off
	for end < len(w.r.d) && w.r.d[end] != 0 {
		end++
	}
	if end >= len(w.r.d) {
		w.r.err = fmt.Errorf("string at 0x%X not terminated", off)
		return
	}
	w.take(off, end-off+1)
}
