# Quest File Format

> Reference: `server/channelserver/quest_json*.go`, `quest_client_view.go`, issue #40

Quest files (`quests/*.bin`, JKR-compressed, little-endian on PC) hold one quest. This page describes every section the client reads, as recovered from the Wii U build (`mhfo-wiiu.rpx`, which keeps its debug symbols), and how Erupe's JSON maps to it.

## Sources

- **`cv_quest`**: the Wii U client byte-swaps a quest file on load, so it walks every section. The walk gives each section's structure, list terminators and element counts.
- **`_SwapEndianStructure` type descriptors** (table at `0x10290DBC`): one entry per struct type, listing each field's offset and width (8, 16 or 32 bits, raw bytes, or a nested struct). Field widths below come from there.
- **The functions that read each section**, named in the tables below. They give the meaning of a section; fields with a known width but no known meaning are named `unk_XX` in the JSON.
- **ZZ specifics**: the PC ZZ main quest properties are 320 bytes; the Wii U G build's are 160. From `+0x5C` on, the ZZ layout follows `questfile.bin.hexpat`, checked against the retail files.

## Header (0x00–0xBF)

The header is 0xC0 bytes (type `0xC2`); the main quest properties always start at 0xC0.

| Offset | Type | Content |
|---|---|---|
| 0x00 | ptr | Main quest properties |
| 0x04 | ptr | Stages: 4 × 16 bytes, one per player: `u32 stage, f32 x, y, z` (start position) — `Quest_pl_stage_init` |
| 0x08 | ptr | Supply box: 41 × `u16 item, u16 qty` (24 main, 8 sub A, 8 sub B, 1 extra) — `Start_item_data_adrs_get` |
| 0x0C | ptr | Reward tables |
| 0x10 | u32 | Flow script pointer; bit 31 set = not yet byte-swapped (set in every retail file) |
| 0x14 | ptr | Quest area: pointer list → map section lists (small monsters) — `quest_em_init_sub2` |
| 0x18 | ptr | Large monsters: one map section — `quest_em_init_sub` |
| 0x1C | ptr | Area transitions, one list per zone — `Stage_mv_data_get` |
| 0x20 | ptr | Area mappings, 32 bytes each (8 floats) |
| 0x24 | ptr | Map info: 4 × i32 (map ID, return base camp ID, 0, 0) |
| 0x28 | ptr | Gathering points, one list per zone — `Stage_item_data_get` |
| 0x2C | ptr | Area facilities, one list per zone — `Stage_unique_data_get` |
| 0x30 | ptr | Messages: pointer list ending with a null pointer or an empty string; shown by the flow script |
| 0x34 | ptr | Monster respawn points — `em_revival_rnd_pos_set` |
| 0x38 | ptr | Gathering tables — `Stage_item_probability_get` |
| 0x3C | ptr | Fishing spots — `Fish_pos_data_get` |
| 0x40 | ptr | Fish catch tables — `Fish_sel_data_get` |
| 0x44 | u16, u16 | Monster size multiplier, size range |
| 0x48 | 5 × i32 | Stat table 1, main RP, `unk_50`, sub A RP, sub B RP |
| 0x5C | 5 × u8 | `unk_5c` |
| 0x61 | u8 | Stat table 2 |
| 0x62 | i16 | `em_quest_param`, read by `em_quest_datprm` |
| 0x64 | 2 × {u32, u16, u8, u8} | `unk_64` |
| 0x74 | u16 | `unk_74` |
| 0x76 | u16 | Flow script length in 16-bit words (4 per instruction) |
| 0x78 | u16 | Gathering table count |
| 0x7A | u16 | Fish catch table set count |
| 0x7C | u8 | Area mapping count |
| 0x7D | u8 | Facility zone count |
| 0x7E | u8 | Gathering point zone count |
| 0x7F | u8 | Transition zone count |
| 0x80 | 4 × u16 | `unk_80` |
| 0x88 | {u8, u8, u16, u16, u8, u8, u16, u16} | `unk_88` |
| 0x94 | 2 × u32 | `unk_94` |
| 0x9C | 36 bytes | `unk_9c` |

A pointer the client dereferences without a null check (quest area, large monsters, messages, respawn points, fishing spots, rewards) must point to at least a list terminator.

## Main quest properties (320 bytes)

| Offset | Type | Content |
|---|---|---|
| +0x00 | u32 | `flags` |
| +0x04 | 4 × u8 | `unk_04` |
| +0x08 | u16 | Rank band |
| +0x0A | 2 × u8 | `unk_0a` |
| +0x0C | u32 | Fee |
| +0x10 | u32 | Main reward |
| +0x14 | u32 | `unk_14` (carts or reward reduction per hexpat) |
| +0x18 | u16, u16 | Sub A reward, `unk_1a` |
| +0x1C | u16, u16 | Sub B reward, hard mode HR requirement |
| +0x20 | u32 | Time limit in 30 Hz frames |
| +0x24 | u32 | Map |
| +0x28 | ptr | Quest text: 8 string pointers (title, main, sub A, sub B, success, failure, contractor, description) |
| +0x2C | 2 × u8, u16 | `unk_2c`, quest ID |
| +0x30 | 3 × objective | Main, sub A, sub B: `u32 type, u16 target, u16 count` (type `0x9C`) |
| +0x48 | 2 × u8, u16 | `unk_48`, `unk_4a` |
| +0x4C | 4 × u16 | Join rank min/max, post rank min/max |
| +0x54 | objective | `objective_extra` |
| +0x5C | 6 × 4 × u16 | Forced equipment (legs, weapon, head, chest, arms, waist) |
| +0x8C | u32 | `unk_8c` |
| +0x90 | 3 × u8, u8 | Monster variants, map variant |
| +0x94 | u16, u8 | Required item, count |
| +0x97 | 4 × u8 | Quest variants |
| +0x9B | 5 × u8 | `unk_9b` |
| +0xA0 | 4 × u32 | Allowed equipment bitmask, main / sub A / sub B points |
| +0xB0 | 3 × u16 | Reward items |
| +0xB6 | 14 bytes | `unk_b6` (interception settings per hexpat) |
| +0xC4 | u32 | Quest clears allowed |
| +0xC8 | 120 bytes | `unk_c8` |

Quest text is CP932. Roman numerals Ⅰ–Ⅹ have two codes in CP932; retail text uses the IBM ones (0xFA4A–0xFA53), not the NEC ones (0x8754–0x875D).

## Sections

**Flow script** (0x10): instructions of 4 × i16, `op, a, b, c`, run by `quest_condition_prog`. Known opcodes:

| Op | Meaning |
|---|---|
| -1 | Success branch (found by `quest_presuccess_ptr_set`) |
| -2 | Failure branch (found by `quest_failed_ptr_set`) |
| 0 | Check whether the quest is cleared |
| 2 | `quest_target_set(a, b, c)` |
| 6 | Check target / sub objective clear |
| 11 | Show message `a` (index into the message list) |
| 24 | `act_ck` |
| 26 | Label `a` |
| 27 | Show a message built with the quest time (`str_gattai`) |
| 31 | Upload the quest time, show messages |
| 32 | `em_set_point_send(a)` |
| 33 | `quest_item_ck` |
| 36, 48 | Check target monster counts (`quest_target_em_num_ck`) |
| 37 | Time over |
| 38 | Pre-success |
| 42 | Check boss HP, set target clear |
| 51 | Arena judgement |

**Map section** (type `0xA6`, 16 bytes): `u32 stage, u32 unk, ptr spawn types, ptr spawns`. A section list ends with stage 0; the rest of that terminator entry is leftover data from the authoring tool and is never read. The spawn type block is 4 × i32 (monster IDs, -1 when unused); for large monsters it is 8 × i32, the monster IDs then `-1, -1, 0`.

**Spawn record** (type `0xA4`, 60 bytes), shared by large monsters and map section spawns: `u16 monster` (-1 ends the list), `u16`, `u32 amount`, `u32` (stage for large monsters), `4 × i32`, `u32 orientation`, `f32 x, y, z`, 16 bytes. Large monsters use 6 fixed slots: up to 5 monsters, the rest terminators.

**Reward tables** (type `0xAC`): `u16 id` (high byte `0x80` in most retail tables), `2 × u8`, `ptr items`; items are `u16 rate, item, quantity` ending with `0xFFFF`.

**Area transitions** (type `0x62`, 52 bytes): `i16 target stage, i16 variant, 11 floats, 2 × u16`; a list ends with target stage -1. Retail terminators are `-1, -1, -1.0f` then zeros.

**Gathering points** (type `0x66`, 24 bytes): `f32 x, y, z, range, u16 gathering ID, max, unk, min`; a list ends with max count 0 (retail terminators also have x = -1.0).

**Area facilities** (type `0x6A`, 24 bytes): `u16 unk, u16 type, f32 x, y, z, range, u16 id, u16 unk`; a list ends with type 0. Every retail file uses the same terminator entry.

For per-zone lists (transitions, gathering points, facilities) and gathering tables, a zone without entries is a null pointer; a few retail transition zones instead point to an empty list (`empty_list` in the JSON).

**Monster respawn points** (0x34, type `0xA2`): `u16 stage, u16 count, ptr` → count × `{u32 angle, f32 x, y, z}`; ends with stage -1. When a monster respawns, `em_revival_rnd_pos_set` picks one point of its stage at random.

**Fishing spots** (0x3C, type `0xB8`): `i32 area, ptr` → spots `{f32 x, y, z, radius, i32 kind, i32 unk}` (type `0xAE`) ending with kind -1; the area list ends with area 0. `kind` selects the catch tables.

**Fish catch tables** (0x40): `u16 @ 0x7A` pointers to sets (type `0xB4`) of 6 × `{ptr catches, u32 count}`. Sets are indexed `kind × 3 + rank` (0 low, 1 high, 2 G rank); tables within a set `day_night × 3 + season`. Catches are `u8 weight, u8 fish` pairs ending with a 0xFF byte.

## Round-trip fidelity

`ClientQuestView` walks a file the way the client does and records what it reads, independent of where sections are placed; `questconv export --verify` and `TestRetailRoundTrip` compare the original and recompiled views, and report original bytes no section reads.

On the full retail set, 54,964 of 54,977 quests round-trip. The other 13 (64551–64553, 65001–65004) hold data no header pointer reaches: an extra text table, orphan reward tables, and a corrupt fish table pointer.
