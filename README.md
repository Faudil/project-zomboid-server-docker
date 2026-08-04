# Project Zomboid Server Docker

[![Build and Publish](https://github.com/faudil/project-zomboid-server-docker/actions/workflows/build.yml/badge.svg)](https://github.com/faudil/project-zomboid-server-docker/actions/workflows/build.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/faudil/project-zomboid-server)](https://hub.docker.com/r/faudil/project-zomboid-server)
[![License](https://img.shields.io/github/license/faudil/project-zomboid-server-docker)](LICENSE)

A Docker container for the Project Zomboid dedicated server. Built from the ground up to be simple, reliable, and well-documented.

## Features

- **Automatic installation** -- Fetches and validates server files via DepotDownloader on first start
- **Graceful shutdown** -- `docker stop` triggers `save` then `quit` via RCON
- **Healthcheck** -- RCON-based health monitoring, Docker native `HEALTHCHECK`
- **Workshop mods** -- Auto-download mods from Steam Workshop on start
- **Automatic backups** -- Scheduled world backups with rotation
- **Discord webhook** -- Server start, stop, and crash notifications
- **All config via env vars** -- No need to edit `.ini` files manually
- **Multi-instance** -- Run multiple servers from one compose file
- **Go entrypoint** -- Single binary, no shell scripts, proper error handling
- **Rootless** -- Runs as the `steam` user (UID 1000), never as root

## Quick Start

One-liner (Docker Hub image):

```bash
docker run -d \
  --name pz-server \
  -p 16261:16261/udp -p 16262:16262/udp -p 27015:27015 \
  -e ADMIN_PASSWORD=your-admin-password \
  -e RCON_PASSWORD=your-rcon-password \
  -v pz-data:/home/steam/Zomboid \
  faudil/project-zomboid-server:latest
```

Or with Docker Compose:

```bash
cp .env.example .env
# Edit .env — ADMIN_PASSWORD and RCON_PASSWORD are optional:
# if left empty they are auto-generated and stored in ./data/credentials.env

mkdir -p data server-files backups
sudo chown -R 1000:1000 data server-files backups  # container runs as UID 1000

docker compose up -d
```

> The container runs as UID 1000 (`steam`). Create the host directories
> **before** `docker compose up` — if they don't exist, Docker creates them
> as root and the container cannot write to them.

The server will download (~5-10 minutes first time), then start. Once you see `LuaNet: Initialization [DONE]` in logs, players can join on port `16261`.

```bash
docker compose logs -f
```

## Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 4GB | 8GB+ |
| Storage | 5GB | 10GB+ |

## Connecting

1. Launch Project Zomboid
2. Click **Join** → **Favorites**
3. Add your server's public IP, port `16261`
4. Enter any account details, save, and join

## Documentation

| Document | Description |
|----------|-------------|
| [QUICKSTART.md](docs/QUICKSTART.md) | Step-by-step setup from zero |
| [CONFIGURATION.md](docs/CONFIGURATION.md) | Complete environment variable reference |
| [PERFORMANCE.md](docs/PERFORMANCE.md) | Tuning for maximum server performance |
| [MODS.md](docs/MODS.md) | Workshop mod installation guide |
| [BACKUP.md](docs/BACKUP.md) | Backup, restore, and rotation |
| [MULTI_INSTANCE.md](docs/MULTI_INSTANCE.md) | Running multiple servers |
| [ADMIN_PANEL.md](docs/ADMIN_PANEL.md) | Integrating with Zomboid Control Panel |
| [DISCORD.md](docs/DISCORD.md) | Discord webhook setup |
| [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Common issues and solutions |

## Volumes

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `./data` | `/home/steam/Zomboid` | Config + saves (persist across restarts) |
| `./server-files` | `/home/steam/pzserver` | Game install (optional) |
| `./backups` | `/home/steam/Zomboid/backups` | World backups (optional) |

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| `16261` | UDP | Game connection |
| `16262` | UDP | Steam direct connection |
| `27015` | TCP | RCON (remote console) |
| `8080` | TCP | Health endpoint (internal) |

## Environment Variables

See [CONFIGURATION.md](docs/CONFIGURATION.md) for the full reference. The most important ones:

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_NAME` | `servertest` | Server/map name |
| `PUBLIC_NAME` | `My PZ Server` | Public display name |
| `ADMIN_PASSWORD` | auto-generated | Admin account password (stored in `data/credentials.env` when unset) |
| `RCON_PASSWORD` | auto-generated | RCON password (stored in `data/credentials.env` when unset) |
| `MAX_PLAYERS` | `16` | Player slots |
| `MAX_RAM` | `4096m` | JVM max heap |
| `MOD_WORKSHOP_IDS` | (empty) | Workshop mod IDs (semicolon-separated) |
| `MOD_WORKSHOP_COLLECTION_IDS` | (empty) | Steam collection IDs; items resolved automatically |
| `MOD_NAMES` | auto-detected | Mod folder names (empty = derived from downloads) |
| `UPDATE_ON_START` | `true` | Auto-download/verify server files on start |
| `SERVER_BRANCH` | (empty) | Beta branch (`unstable`, `legacy41`) |
| `STEAM_USER` / `STEAM_PASS` | (empty) | Optional Steam account (owns Project Zomboid) — only needed for private branches or workshop mods when anonymous downloads fail |
| `BACKUP_ENABLED` | `false` | Enable auto-backups |
| `DISCORD_WEBHOOK_URL` | (empty) | Discord webhook URL |

## Multi-Instance

Use `docker-compose.multi.yml` to run multiple servers sharing the same game install:

```bash
cp .env.example .env
mkdir -p data-server1 data-server2 backups-server1 backups-server2
docker compose -f docker-compose.multi.yml up -d
```

Each server must have unique ports. See [MULTI_INSTANCE.md](docs/MULTI_INSTANCE.md).

## Building

```bash
docker build -t project-zomboid-server .
```

## License

[MIT](LICENSE)

## Disclaimer

This image is not affiliated with The Indie Stone or Valve. Project Zomboid is a trademark of The Indie Stone.
