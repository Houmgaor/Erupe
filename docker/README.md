# Docker for Erupe

## Quick Start

1. From the repository root, copy and edit the config:

   ```bash
   cp config.example.json docker/config.json
   ```

   Edit `docker/config.json` — set `Database.Host` to `"db"` and `Database.Password` to match `docker-compose.yml` (default: `password`). The example config is minimal; see `config.reference.json` for all available options.

2. Place your [quest/scenario files](https://files.catbox.moe/xf0l7w.7z) in `docker/bin/`.

3. Start everything:

   ```bash
   cd docker
   docker compose up
   ```

The database schema is automatically applied on first start via the embedded migration system.

pgAdmin is available at `http://localhost:5050` (default login: `user@pgadmin.com` / `password`).

## Building Locally

By default the server service pulls the prebuilt image from GHCR. To build from source instead, edit `docker-compose.yml`: comment out the `image` line and uncomment the `build` section, then:

```bash
docker compose up --build
```

## Stopping the Server

```bash
docker compose stop     # Stop containers (preserves data)
docker compose down     # Stop and remove containers (preserves data volumes)
```

To delete all persistent data, remove these directories after stopping:

- `docker/db-data/`
- `docker/savedata/`

## Updating

After pulling new changes, rebuild and restart. Schema migrations are applied automatically on startup.

```bash
docker compose down
docker compose build
docker compose up
```

## Troubleshooting

**Postgres won't start on Windows**: Ensure `docker/db-data/` doesn't contain stale data from a different PostgreSQL version. Delete it and restart to reinitialize.
