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

If `ADMIN_PASSWORD` / `RCON_PASSWORD` are left empty, they are generated on
first start and persisted to `<DATA_DIR>/credentials.env` (inside the `./data`
volume) so they stay stable across restarts. Retrieve them from that file --
they are intentionally not printed to the container logs.

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

Applied to the server JVM via `_JAVA_OPTIONS` (the PZ start script exposes no
knobs for these):

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_RAM` | string | `4096m` | Maximum heap size (`-Xmx`) |
| `MIN_RAM` | string | `4096m` | Initial heap size (`-Xms`) |
| `GC_CONFIG` | string | `ZGC` | Garbage collector (`ZGC`, `G1`, `Serial`) |
| `JVM_EXTRA_ARGS` | string | (empty) | Additional JVM arguments |

## Sandbox (Gameplay)

Sandbox values are not read from the `.ini`; they live in
`Server/<SERVER_NAME>_SandboxVars.lua`. Any environment variable prefixed with
`SANDBOX_` is written there with the prefix stripped:

```env
SANDBOX_Zombies=2
SANDBOX_DayLength=2
SANDBOX_WaterShutModifier=20
```

Every `SANDBOX_*` variable becomes a key in `SandboxVars.lua` with the same
value. Unset keys fall back to the built-in defaults. See the
[PZ wiki](https://pzwiki.net/wiki/Sandbox_Options) for valid key names.

## Mods

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MOD_NAMES` | string | (empty) | Semicolon-separated mod folder names (auto-derived when empty) |
| `MOD_WORKSHOP_IDS` | string | (empty) | Semicolon-separated Workshop item IDs to download |
| `MOD_WORKSHOP_COLLECTION_IDS` | string | (empty) | Semicolon-separated Workshop collection IDs; items resolved via the Steam Web API at start |
| `MOD_UPDATE_ON_START` | bool | `false` | Re-download all workshop items on every start to pick up updates |
| `STEAM_API_KEY` | string | (empty) | Optional Steam Web API key for collection resolution |

See [MODS.md](MODS.md) for the full guide, including manual (non-Workshop)
mods dropped into `<DATA_DIR>/Workshop/`.

## Updates

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `UPDATE_ON_START` | bool | `true` | Run SteamCMD on container start |
| `SERVER_BRANCH` | string | (empty) | Beta branch (`unstable`, `legacy41`, etc.) |
| `STEAM_APP_ID` | string | `380870` | PZ Dedicated Server App ID |
| `STEAM_USER` | string | (empty) | Steam account for downloading server files |
| `STEAM_PASS` | string | (empty) | Steam account password |
| `STEAM_GUARD_CODE` | string | (empty) | One-time Steam Guard code from your email (first login only) |

**Note:** Steam occasionally fails anonymous `app_update` downloads with a
cryptic `Failed to install app '380870' (Missing file permissions)` error.
This is a known, transient Valve-side regression
([steam-for-linux #10979](https://github.com/ValveSoftware/steam-for-linux/issues/10979))
that also affects owned accounts. The entrypoint retries automatically; if it
persists, set `STEAM_USER` and `STEAM_PASS` (an account that owns Project
Zomboid), which is the reliable workaround. That account is also needed to
download workshop mods when anonymous downloads fail. If your account uses
Steam Guard, put the code from your email into `STEAM_GUARD_CODE` on the
first login; steamcmd remembers the login afterwards.

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

## Container Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TZ` | string | `UTC` | Timezone (`America/New_York`, `Europe/London`, etc.) |

The container always runs as UID 1000 (`steam`); there is no `PUID`/`PGID`
mapping. Set the ownership of the host bind mounts to UID 1000 instead:

```bash
sudo chown -R 1000:1000 data server-files backups
```

Invalid values for numeric/boolean variables are rejected with a warning and
fall back to the documented default.

## Generated Files

The following files are auto-generated from environment variables at container start:

- `Server/<SERVER_NAME>.ini` -- Main server settings
- `Server/<SERVER_NAME>_SandboxVars.lua` -- Sandbox/gameplay settings

**Note:** If you edit these files manually, your changes will be overwritten on the next container restart. Use environment variables instead.

## Manual Configuration

For settings not covered by environment variables, you can edit the generated `.ini` and `.lua` files **after the first start**, then disable `UPDATE_ON_START=false` to prevent regeneration. This is not recommended for most users -- prefer environment variables when available.
