# Configuration Reference

All server configuration is done through environment variables in `.env`. The container generates the appropriate `.ini` and `.lua` files automatically.

## Server Identity

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SERVER_NAME` | string | `servertest` | Internal server name. Determines save folder name |
| `PUBLIC_NAME` | string | `My PZ Server` | Name shown in server browser |
| `PUBLIC_SERVER` | bool | `true` | List server in public browser |

## Access Control

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SERVER_PASSWORD` | string | (empty) | Server password (empty = no password) |
| `ADMIN_PASSWORD` | string | auto-generated | Admin account password |
| `RCON_PASSWORD` | string | auto-generated | RCON connection password |
| `RCON_PORT` | int | `27015` | RCON TCP port |
| `STEAM_VAC` | bool | `true` | Enable Steam VAC anti-cheat |
| `USE_STEAM` | bool | `true` | Enable Steam networking |
| `PAUSE_ON_EMPTY` | bool | `true` | Pause simulation when no players |

## Network

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DEFAULT_PORT` | int | `16261` | Main game port (UDP) |
| `UDP_PORT` | int | `16262` | Steam direct connection port (UDP) |
| `BIND_IP` | string | `0.0.0.0` | IP address to bind to |

## Players & Gameplay

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_PLAYERS` | int | `16` | Maximum player slots |
| `PVP` | bool | `true` | Enable player vs player |
| `MAP_NAMES` | string | `Muldraugh, KY` | Semicolon-separated map names |
| `AUTOSAVE_INTERVAL` | int | `15` | Minutes between autosaves |

## JVM & Memory

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_RAM` | string | `4096m` | Maximum heap size (`-Xmx`) |
| `MIN_RAM` | string | `4096m` | Initial heap size (`-Xms`) |
| `GC_CONFIG` | string | `ZGC` | Garbage collector (`ZGC`, `G1`, `Serial`) |
| `JVM_EXTRA_ARGS` | string | (empty) | Additional JVM arguments |

## Mods

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MOD_NAMES` | string | (empty) | Semicolon-separated mod names |
| `MOD_WORKSHOP_IDS` | string | (empty) | Semicolon-separated Workshop IDs |

## Updates

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `UPDATE_ON_START` | bool | `true` | Run SteamCMD on container start |
| `SERVER_BRANCH` | string | (empty) | Beta branch (`unstable`, `legacy41`, etc.) |
| `STEAM_APP_ID` | string | `380870` | PZ Dedicated Server App ID |

## Backups

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `BACKUP_ENABLED` | bool | `false` | Enable automatic backups |
| `BACKUP_INTERVAL` | int | `360` | Minutes between backups |
| `BACKUP_MAX_COUNT` | int | `24` | Max backups to keep |
| `BACKUP_PATH` | string | `/home/steam/Zomboid/backups` | Backup directory |

## Discord

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DISCORD_WEBHOOK_URL` | string | (empty) | Discord webhook URL |
| `DISCORD_NOTIFY_START` | bool | `true` | Notify on server start |
| `DISCORD_NOTIFY_STOP` | bool | `true` | Notify on server stop |
| `DISCORD_NOTIFY_CRASH` | bool | `true` | Notify on server crash |
| `DISCORD_NOTIFY_PLAYERS` | bool | `false` | Notify on player join/leave |

## Container Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TZ` | string | `UTC` | Timezone (`America/New_York`, `Europe/London`, etc.) |
| `PUID` | int | `1000` | User ID for file ownership |
| `PGID` | int | `1000` | Group ID for file ownership |

## Generated Files

The following files are auto-generated from environment variables at container start:

- `Server/<SERVER_NAME>.ini` -- Main server settings
- `Server/<SERVER_NAME>_SandboxVars.lua` -- Sandbox/gameplay settings

**Note:** If you edit these files manually, your changes will be overwritten on the next container restart. Use environment variables instead.

## Manual Configuration

For settings not covered by environment variables, you can edit the generated `.ini` and `.lua` files **after the first start**, then disable `UPDATE_ON_START=false` to prevent regeneration. This is not recommended for most users -- prefer environment variables when available.
