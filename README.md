# Erupe

[![Build and Test](https://github.com/Mezeporta/Erupe/actions/workflows/go.yml/badge.svg)](https://github.com/Mezeporta/Erupe/actions/workflows/go.yml)
[![CodeQL](https://github.com/Mezeporta/Erupe/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/Mezeporta/Erupe/actions/workflows/github-code-scanning/codeql)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Mezeporta/Erupe)](https://go.dev/)
[![Latest Release](https://img.shields.io/github/v/release/Mezeporta/Erupe)](https://github.com/Mezeporta/Erupe/releases/latest)

Erupe is a community-maintained server emulator for Monster Hunter Frontier written in Go. It is a complete reverse-engineered solution to self-host a Monster Hunter Frontier server, using no code from Capcom.

## Quick Start

Pick one of two installation methods, then continue to [Quest & Scenario Files](#quest--scenario-files).

### Option A: Pre-compiled Binary

1. Download the latest release for your platform from [GitHub Releases](https://github.com/Mezeporta/Erupe/releases/latest):
   - `erupe-ce` for Linux
   - `erupe.exe` for Windows

2. Set up PostgreSQL and create a database:

   ```bash
   createdb -U postgres erupe
   ```

   Alternatively, when using pgAdmin4, right click the PostgreSQL version listed under "Servers" and click "Create", then name the database "erupe" and press Save.
   
   The server will automatically apply all schema migrations on first startup.

3. Copy and edit the config:

   ```bash
   cp config.example.json config.json
   # Edit config.json with your database credentials
   ```

   If you're using File Explorer or the likes, rename or create a copy of `config.example.json` titled `config.json` and fill it out with your database credentials.

4. Get [quest/scenario files](#quest--scenario-files) into `game-data/` (or `bin/` if you're upgrading an existing install).

   It should look as follows:
   ```
   game-data
   ├───events
   │   ├───Campaign
   │   ├───Collab
   │   ├───Custom Quests
   │   ├───Daily
   │   ├───Diva Defense
   │   ├───G Event
   │   ├───HR Event
   │   ├───Musou
   │   ├───Raviente
   │   ├───Series
   │   ├───Special Event & April Fools
   │   └───Zenith
   ├───quests
   ├───scenarios
   └───rengoku_data.bin
   ```  

7. Run the server:

   On Linux, open the Terminal in the folder containing Erupe and run: `./erupe-ce`.
   
   On Windows, simply open the file titled `erupe.exe`. 

   Alternatively, open the Command Prompt (cmd) in the folder containing Erupe and run: `erupe`.

### Option B: From Source

Requires [Go 1.25+](https://go.dev/dl/) and [PostgreSQL](https://www.postgresql.org/download/).

1. Clone and build:

   ```bash
   git clone https://github.com/Mezeporta/Erupe.git
   cd Erupe
   go mod download
   go build -o erupe-ce
   ```

2. Set up the database (same as Option A, steps 2–3)

3. Copy and edit the config:

   ```bash
   cp config.example.json config.json
   ```

4. Get [quest/scenario files](#quest--scenario-files) into `game-data/`

5. Run: `./erupe-ce`

## Quest & Scenario Files

These files contain quest definitions, scenario data, and Hunting Road (`rengoku`) config that the server sends to clients during gameplay. **Without them, quests will not load and the client will crash.**

### Option A: Sync (recommended)

Download `binsync` from [GitHub Releases](https://github.com/Mezeporta/Erupe/releases/latest) alongside the server binary, then:

```bash
./binsync --manifest-url https://data.mogapedia.fr/erupe/manifest.json
```

This downloads and installs quest/scenario/road data into `game-data/` (or `docker/game-data/` for Docker installs), verifying each file's hash and content before installing it — see [`docs/binsync-format.md`](docs/binsync-format.md) for how. The same action is available as a "Sync Now" button in the setup wizard's Quest Files step. Re-run it any time to pick up corrections — it only downloads what changed.

### Option B: Manual download

**Download**: [Quest and Scenario Binary Files](https://files.catbox.moe/xf0l7w.7z)

Extract the archive into your `game-data/` directory (or `bin/` if you're upgrading an existing install — both work, see below). The path must match the `BinPath` setting in your config (default: auto-detected, see [`docs/binsync-format.md`](docs/binsync-format.md#the-game-databin-directory-rename)).

### The `game-data`/`bin` rename

The data directory used to be called `bin/` — a name that predates JSON support below and no longer describes what's actually stored there. New installs default to `game-data/`; existing installs with a populated `bin/` directory keep working automatically, with no config changes needed.

### JSON Format Support

As an alternative to opaque `.bin` files, Erupe supports human-readable `.json` files for quests, scenarios, and Hunting Road config. The server always tries `.bin` first and falls back to `.json` automatically — existing binary files work unchanged. This is the format `binsync` distributes.

| File type | Location | Documentation |
|-----------|----------|---------------|
| Quest | `game-data/quests/<name>.json` | Erupe wiki |
| Scenario | `game-data/scenarios/<name>.json` | `docs/scenario-format.md` |
| Hunting Road | `game-data/rengoku_data.json` | Erupe wiki |

JSON quests and scenarios use UTF-8 text (converted to Shift-JIS on the wire), making them diff-friendly and editable without binary tools.

## Client Setup

1. Obtain a Monster Hunter Frontier client (version G10 or later recommended)
2. Point the client to your server by editing `host.txt` or using a launcher to redirect to your server's IP
3. Launch `mhf.exe`, select your server, and create an account

If you have an **installed** copy of Monster Hunter Frontier on an old hard drive, **please** get in contact so we can archive it!

## Updating

```bash
git pull origin main
go mod tidy
go build -o erupe-ce
```

Database schema migrations are applied automatically when the server starts — no manual SQL steps needed.

For Docker users, see [docker/README.md](./docker/README.md).

## Configuration

Edit `config.json` before starting the server. The essential settings are:

```json
{
  "Host": "127.0.0.1",
  "Language": "en",
  "ClientMode": "ZZ",
  "Database": {
    "Host": "localhost",
    "Port": 5432,
    "User": "postgres",
    "Password": "your_password",
    "Database": "erupe"
  }
}
```

| Setting | Description |
|---------|-------------|
| `Host` | IP advertised to clients. Use `127.0.0.1` for local play, your LAN/WAN IP for remote. Leave blank in config to auto-detect |
| `ClientMode` | Target client version (`ZZ`, `G10`, `Forward4`, etc.) |
| `BinPath` | Path to quest/scenario files. Usually left unset — auto-detects `game-data/` or an existing `bin/`, see [`docs/binsync-format.md`](docs/binsync-format.md#the-game-databin-directory-rename) |
| `Language` | Default server language: `"en"`, `"jp"`, `"fr"`, or `"zh"`. Players can override per-session in-game via `!lang <code>` |

### Localization (i18n)

Erupe supports per-session language preferences for quest text, scenario text, and in-game messages. Languages currently available: English (`en`), Japanese (`jp`), French (`fr`), and Chinese (`zh`).

- **Default language** is set via `Language` in `config.json`.
- **Per-player override**: players can switch their own session language in-game with `!lang <code>` (e.g. `!lang fr`).
- **Localized quest/scenario text**: JSON quests and scenarios accept either a plain string or a `{ "en": "...", "jp": "...", "fr": "...", "zh": "..." }` map for any user-facing field (quest titles, descriptions, scenario strings, etc.). The server picks the string matching the session's language and falls back to the default language when a translation is missing. Compiled output is cached per `(questID, language)`.

`config.example.json` is intentionally minimal — all other settings have sane defaults built into the server. For the full configuration reference (gameplay multipliers, debug options, Discord integration, in-game commands, entrance/channel definitions), see [config.reference.json](./config.reference.json) and the [Erupe Wiki](https://github.com/Mezeporta/Erupe/wiki).

## Save Transfers

To move a character from one Erupe instance to another, use the `saveutil` admin tool.

### Build saveutil

```bash
go build -o saveutil ./cmd/saveutil/
```

### Method 1: Direct admin import (recommended)

This method does not require the server to be running.

**On the source server**, export the character:
```bash
./saveutil export --config config.json --char-id <SOURCE_ID> --output my_character.json
```

**On the destination server**, find the target character ID (use pgAdmin or `psql`), then import:
```bash
./saveutil import --config config.json --char-id <DEST_ID> --file my_character.json
```

### Method 2: Player self-service via API

This method lets players import their own save without admin DB access, but requires the admin to grant a one-time token first.

**Admin step** — grant a token (valid for 24 hours by default):
```bash
./saveutil grant-import --config config.json --char-id <DEST_ID> [--ttl 48h]
# → Import token for character 42: abc123...
```

Give the printed token to the player. They then call the import endpoint:
```bash
curl -X POST http://<server>:8080/v2/characters/<DEST_ID>/import \
  -H "Authorization: Bearer <player_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "import_token": "<admin_token>",
    "character": <paste contents of my_character.json .character field here>
  }'
```

The token is consumed on success and cannot be reused. To cancel a pending grant:
```bash
./saveutil revoke-import --config config.json --char-id <DEST_ID>
```

### Troubleshooting

**"savedata integrity check failed"** — the character was imported directly into the DB without going through `saveutil`. Fix by clearing the stored hash:
```sql
UPDATE characters SET savedata_hash = NULL WHERE id = <char_id>;
```
The correct hash will be recomputed on the next save.

## Data Sync Tools

See [Quest & Scenario Files](#quest--scenario-files) for the quick-start version. Full reference: [`docs/binsync-format.md`](docs/binsync-format.md).

### Build binsync

```bash
go build -o binsync ./cmd/binsync/
```

```bash
./binsync --config config.json                                            # reads BinSync.ManifestURL from config.json
./binsync --manifest-url https://data.mogapedia.fr/erupe/manifest.json    # or pass it directly
./binsync --manifest-url <url> --bin-path custom-dir                      # override the install directory
```

Downloads/skips/failures are printed as they happen, with a final `fetched=N skipped=N failed=N orphans=N` summary. Exits non-zero if any file failed. Safe to re-run any time — it only fetches what changed, and never overwrites an existing good file with a bad or invalid one.

### questconv (data curators only)

`questconv` is producer-side tooling for whoever curates the remote manifest — not something a typical server operator runs.

```bash
go build -o questconv ./cmd/questconv/

./questconv export   --bin-path game-data --out export/ --verify   # .bin -> .json, with round-trip validation
./questconv manifest --dir export/ --out export/manifest.json      # hash export/ into a binsync-compatible manifest
```

`--verify` recompiles each exported file and diffs it against the parsed original, flagging mismatches — see [`docs/binsync-format.md`](docs/binsync-format.md#what-this-does-not-solve) for what a large mismatch count means before trusting a data set enough to publish it.

## Features

- **Multi-version Support**: Compatible with all Monster Hunter Frontier versions from Season 6.0 to ZZ
- **Multi-platform**: Supports PC, PlayStation 3, PlayStation Vita, and Wii U (up to Z2)
- **Complete Server Emulation**: Entry/Sign server, Channel server, and Launcher server
- **Gameplay Customization**: Configurable multipliers for experience, currency, and materials
- **Event Systems**: Support for Raviente, MezFes, Diva, Festa, and Tournament events
- **Discord Integration**: Optional real-time Discord bot integration
- **In-game Commands**: Extensible command system with configurable prefixes
- **Developer Tools**: Comprehensive logging, packet debugging, and save data dumps

## Architecture

Erupe consists of three main server components:

- **Sign Server** (Port 53312): Handles authentication and account management
- **Entrance Server** (Port 53310): Manages world/server selection
- **Channel Servers** (Ports 54001+): Handle game sessions, quests, and player interactions

Multiple channel servers can run simultaneously, organized by world types: Newbie, Normal, Cities, Tavern, Return, and MezFes.

## Client Compatibility

### Platforms

- PC
- PlayStation 3
- PlayStation Vita
- Wii U (Up to Z2)

### Versions

- **G10-ZZ** (ClientMode): Extensively tested with great functionality
- **G3-Z2** (Wii U): Tested with good functionality
- **Forward.4**: Basic functionality
- **Season 6.0**: Limited functionality (oldest supported version)

## Database Schemas

Erupe uses an embedded auto-migrating schema system. Migrations in [server/migrations/sql/](./server/migrations/sql/) are applied automatically on startup — no manual SQL steps needed.

- **Migrations**: Numbered SQL files (`0001_init.sql`, `0002_*.sql`, ...) tracked in a `schema_version` table
- **Seed Data**: Demo templates for shops, distributions, events, and gacha in [server/migrations/seed/](./server/migrations/seed/) — applied automatically on fresh databases

## Development

### Branch Strategy

- **main**: Stable branch — released, tested features
- **develop**: Integration branch for in-progress and potentially breaking changes

### Running Tests

```bash
go test -v ./...           # Run all tests
go test -v -race ./...     # Check for race conditions (mandatory before merging)
```

## Troubleshooting

### Server won't start

- Verify PostgreSQL is running: `systemctl status postgresql` (Linux) or `pg_ctl status` (Windows)
- Check database credentials in `config.json`
- Ensure all required ports are available and not blocked by firewall

### Client can't connect

- Verify server is listening: `netstat -an | grep 53310`
- Check firewall rules allow traffic on ports 53310, 53312, and 54001+
- Ensure client's `host.txt` points to correct server IP
- For remote connections, set `"Host"` in config.json to `0.0.0.0` or your server's IP

### Database schema errors

- Schema migrations run automatically on startup — check the server logs for migration errors
- Check PostgreSQL logs for detailed error messages
- Verify database user has sufficient privileges

### Quest files not loading

- Run `./binsync` (see [Quest & Scenario Files](#quest--scenario-files)) or check the setup wizard's Quest Files step for the directory it's actually checking
- If `BinPath` is set explicitly in config.json, confirm it points to extracted quest/scenario files
- Verify binary files match your `ClientMode` setting
- Check file permissions

### Debug Logging

Enable detailed logging in `config.json`:

```json
{
  "DebugOptions": {
    "LogInboundMessages": true,
    "LogOutboundMessages": true
  }
}
```

## Resources

- **Quest/Scenario Files**: sync with `./binsync` (see [Quest & Scenario Files](#quest--scenario-files)), or [download manually (catbox)](https://files.catbox.moe/xf0l7w.7z)
- **Documentation**: [Erupe Wiki](https://github.com/Mezeporta/Erupe/wiki)
- **Discord Communities**:
  - [Mezeporta Square](https://discord.gg/DnwcpXM488)
  - [Mogapedia](https://discord.gg/f77VwBX5w7) (French Monster Hunter community, current Erupe maintainers)
  - [PewPewDojo](https://discord.gg/CFnzbhQ)
- **Community Tools**:
  - [Ferias](https://xl3lackout.github.io/MHFZ-Ferias-English-Project/) — Material and item database
  - [Damage Calculator](https://mh.fist.moe/damagecalc.html) — Online damage calculator
  - [Armor Set Searcher](https://github.com/matthe815/mhfz-ass/releases) — Armor set search application

## Changelog

View [CHANGELOG.md](CHANGELOG.md) for version history and changes.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Authors

A list of authors can be found at [HISTORY.md](HISTORY.md).
