# Quest/Scenario/Road Data Sync (binsync)

> Reference: `server/binsync/`, `cmd/binsync/`, `cmd/questconv/`, `server/setup/wizard.go`

## Overview

Quest, scenario, and Hunting Road (`rengoku`) data is derived from Capcom's original game. Erupe's own git repository never contains this data — the `game-data/` (formerly `bin/`) directory is git-ignored, and quest/scenario/road `.json`/`.bin` files are only ever distributed out-of-band, from infrastructure Mogapedia controls directly (not GitHub). `binsync` is the mechanism for getting that data from a self-hosted manifest onto a server operator's machine, safely and verifiably.

This intentionally keeps Erupe's own git history and GitHub repository free of the data itself: a takedown or complaint about the data can only ever target the host actually serving it, never the codebase.

## Manifest format

A `manifest.json` served over HTTPS alongside the data files themselves:

```json
{
  "version": 1,
  "files": {
    "quests/00001d0.json": {
      "sha256": "dfda41479a79db1b611c47ae6c1c8c7528a53b11b4a1f8328e578b8c4a6c5d0",
      "size": 4821
    },
    "scenarios/0_0_0_0_S0_T100_C0.json": {
      "sha256": "...",
      "size": 1203
    },
    "rengoku_data.json": {
      "sha256": "...",
      "size": 512
    }
  }
}
```

- `version` is the manifest schema version (currently `1`, `binsync.ManifestVersion`). A client build that sees a different version logs a warning and continues — the `files` map shape is expected to stay stable across bumps; only add fields, don't repurpose this one.
- Each key in `files` is a path relative to the data directory (`game-data/` or `bin/`), always forward-slash-separated regardless of host OS.
- File URLs are resolved relative to the manifest URL's own directory (RFC 3986 reference resolution) — e.g. a manifest at `https://data.mogapedia.fr/erupe/manifest.json` serves `quests/00001d0.json` at `https://data.mogapedia.fr/erupe/quests/00001d0.json`.
- Only three path shapes are recognized: `quests/*.json`, `scenarios/*.json`, and `rengoku_data.json`. Anything else in the manifest is refused rather than installed blind (see Validation below).

## Sync algorithm (`server/binsync.Sync`)

For each file listed in the manifest:

1. **Skip if already up to date** — if a local file exists at that path and its SHA-256 already matches the manifest entry, it's left untouched.
2. **Download and hash** — otherwise the file is fetched and hashed while streaming. A hash mismatch (corruption, a bad transfer, a stale CDN edge) is rejected immediately; nothing is written to disk.
3. **Validate content** — before ever touching the local file, the downloaded bytes are run through the *same compiler Erupe uses at load time*:
   - `quests/*.json` → `channelserver.CompileQuestJSON`
   - `scenarios/*.json` → `channelserver.CompileScenarioJSON`
   - `rengoku_data.json` → `json.Unmarshal` into `channelserver.RengokuConfig`, then `channelserver.BuildRengokuBinary`

   This is deliberate: a file can be byte-identical to what the manifest's hash claims and still be semantically broken (truncated mid-authoring, a bad hand-edit, a future format change the compiler doesn't understand). Hash verification alone only proves "this is what the manifest intended to serve," not "this is valid." Reusing the real compiler — rather than a hand-written JSON Schema — also means validation can never drift from the actual format: quest/scenario JSON has union types (`LocalizedString`, either a plain string or a per-language map) and cross-field invariants (reward-table/monster-count limits, area-transition/gathering-point list-length matching) that a schema would have to duplicate and could fall out of sync with.
4. **Install atomically** — a validated file is written to a temp file in the same directory, then renamed into place. A crash mid-write can never leave a truncated file where the server expects a real one.
5. **Report, never delete** — local `.json` files under `quests/`, `scenarios/`, and `rengoku_data.json` that aren't referenced by the manifest are reported as orphans but left alone. An operator may have hand-authored custom content; sync only ever adds or updates, never removes.

A file that fails hash verification or content validation is simply skipped (reported in the failure log) — **an existing good file at that path is never overwritten by a bad fetch.**

## Entry points

| Surface | Use case |
|---|---|
| Setup wizard "Sync Now" (Quest Files step) | First-time, interactive setup. Runs the whole sync synchronously and renders the log, mirroring the existing DB-init step's UI pattern. |
| `cmd/binsync` | Headless/scripted installs, Docker, and periodic re-syncs — quest data gets corrections over time just like any other part of the format's parser/compiler has. |
| `cmd/questconv` | Producer-side only (whoever curates the remote host): bulk-converts existing retail `.bin` files to `.json` via `ParseQuestBinary`/`ParseScenarioBinary`, and builds the `manifest.json` above. Not something a typical server operator runs. |

## The `game-data`/`bin` directory rename

The data directory used to be called `bin/`, a name that predates JSON support and no longer describes what's actually stored there (human-readable `.json` files, not just opaque binaries). New installs now default to `game-data/` — chosen to read clearly to a non-technical server operator dragging files into place.

This is fully backward compatible: `config.ResolveBinPath` (`config/config.go`) only ever treats the literal value `"bin"` as ambiguous (it's indistinguishable between "the operator explicitly chose bin" and "this is just Viper's historic default"). Any other explicitly configured `BinPath` is always honored as-is. When the configured value is `"bin"`:

- If `bin/quests/` already has files in it (an existing install), `bin/` keeps being used — no `config.json` edit required to upgrade.
- Otherwise (a fresh install, or one that's already migrated), `game-data/` is used.

Docker mounts both `./game-data:/app/game-data` and `./bin:/app/bin` (`docker/docker-compose.yml`) so this same detection works inside the container.

## Cache and restart behavior after a sync

Three different code paths read quest/scenario/road data, with different staleness behavior once a sync overwrites files on disk:

- **Direct file transfer** (`MsgSysGetFile` → `loadQuestBinary`/`loadScenarioBinary`) reads fresh from disk on every request — a sync takes effect immediately.
- **Event quest board** (`loadQuestFile`, cached per `(questID, lang)` via `questCache`) may serve stale data for up to `QuestCacheExpiry` seconds (default 300).
- **Hunting Road** (`rengokuBin`) is read once at server startup only, with no re-read path — **a sync has zero effect on a running server until it's restarted.** Both `cmd/binsync` and the wizard say this explicitly when a sync touches any files.

## What this does *not* solve

`binsync` verifies that data came from the manifest unmodified and that it's structurally valid — it does not vouch for the *quality* of the data itself. In particular, `questconv export --verify` round-trips each converted file back through the compiler and diffs it against the parsed original; a large or systematic mismatch (as opposed to occasional edge cases) means the source `.bin` files likely don't fully round-trip through the current `ParseQuestBinary`/`CompileQuestJSON` implementation yet (e.g. a client-version layout difference, or a section the parser doesn't recognize) and should be investigated before that data set is published — publishing a manifest doesn't imply the underlying conversion is complete.
