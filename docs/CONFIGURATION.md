# Configuration Reference

All server configuration is done through environment variables in `.env`. The container generates the appropriate `.ini` and `.lua` files automatically.

## Server Identity

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SERVER_NAME` | string | `servertest` | Internal server name. Determines save folder name. Restricted to letters, digits, `_` and `-` (it is embedded in file paths) |
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
| `UDP_PORT` | int | `16262` | Steam direct connection port (UDP). Max `65534` (the game uses `UDP_PORT+1` for `SteamPort2`) |
| `BIND_IP` | string | `0.0.0.0` | IP address to bind to |

## Players & Gameplay

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_PLAYERS` | int | `16` | Maximum player slots |
| `PVP` | bool | `true` | Enable player vs player |
| `MAP_NAMES` | string | `Muldraugh, KY` | Semicolon-separated map names |
| `AUTOSAVE_INTERVAL` | int | `15` | Minutes between autosaves |

## JVM & Memory

Applied to the server JVM by patching the game's `ProjectZomboid64.json` on
every start (the launcher passes those vmArgs on the java command line, which
would otherwise override `_JAVA_OPTIONS`). See
[PERFORMANCE.md](PERFORMANCE.md) for sizing guidance.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MAX_RAM` | string | `4096m` | Maximum heap size (`-Xmx`) |
| `MIN_RAM` | string | `4096m` | Initial heap size (`-Xms`); keep equal to `MAX_RAM` |
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
[PZ wiki](https://pzwiki.net/wiki/Server_settings) for valid key names.

Nested tables use dot notation — `ZombieConfig` (advanced zombie options) and
`ZombieLore` (zombie behavior):

```env
SANDBOX_ZombieConfig.PopulationMultiplier=0.5
SANDBOX_ZombieConfig.RallyGroupSize=10
SANDBOX_ZombieLore.Speed=4
```

becomes:

```lua
ZombieConfig = {
    PopulationMultiplier = 0.5,
    RallyGroupSize = 10,
    ...
},
ZombieLore = {
    Speed = 4,
    ...
}
```

`SANDBOX_*` overrides (flat and nested) always win over `SANDBOX_MODE`.

Values are rendered as safe Lua: numbers and `true`/`false` stay raw, any
other value is quoted as a string automatically (no need to quote yourself),
and values containing control characters or keys outside `[A-Za-z0-9_.]` are
rejected with a warning.

`SANDBOX_MODE` applies a preset over the built-in b42 Apocalypse defaults:

| Value | Description |
|-------|-------------|
| `apocalypse` (default) | Vanilla b42 Apocalypse values |
| `performance` | World-cleanup tuning: corpses 48 h (default 9 days), blood 7 d (default never), rotten food 14 d (default never), rats off, ground items 12 h. Near-zero gameplay impact |
| `max` | `performance` plus reduced zombie population and rally groups for the best TPS on long-running worlds (easier difficulty) |

`SANDBOX_*` variables always override `SANDBOX_MODE`.

## Server Options (INI)

Settings that live in `Server/<SERVER_NAME>.ini` (rather than the sandbox) are
written through a generic passthrough. Any environment variable prefixed with
`INI_` is written there with the prefix stripped:

```env
INI_SleepAllowed=true
INI_SleepNeeded=false
INI_Faction=true
```

Keys already managed by dedicated variables (`MAX_PLAYERS`, `PVP`, ...) are
ignored with a warning. Key names are restricted to letters and digits, and
values are sanitized so nothing can inject extra directives into the `.ini`.

The PZ server writes this file from its own defaults on first start; the
container rewrites it on every start, so `INI_*` overrides persist across
restarts. See the [PZ wiki](https://pzwiki.net/wiki/Server_settings) for valid
key names (e.g. `SleepAllowed`, `SleepNeeded`, `FastForwardMultiplier`,
safehouse and faction options).

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

## Automatic Updates

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `MOD_AUTO_UPDATE` | bool | `false` | Check Steam for workshop mod and game updates while the server runs, and restart to apply them |
| `MOD_AUTO_UPDATE_INTERVAL` | int | `60` | Minutes between update checks |
| `MOD_AUTO_UPDATE_ANNOUNCE` | int | `5` | In-game warning (RCON `servermsg`) this many minutes before the restart |
| `MOD_AUTO_UPDATE_WAIT_EMPTY` | bool | `true` | Wait until no players are online before restarting |
| `MOD_AUTO_UPDATE_WAIT_MAX` | int | `2` | Max hours to wait for an empty server, then restart anyway |

With `MOD_AUTO_UPDATE=true` the entrypoint polls Steam (no API key needed)
while the server is healthy: it compares each Workshop mod's last-modified
time and the game app's build ID against the baseline in
`<DATA_DIR>/update-state.json` (persisted in the data volume; the first check
only records the baseline). When an update is found it notifies Discord (if
configured), broadcasts an in-game warning, optionally waits for the server
to empty, then saves the world (RCON `save` + `quit`) and exits cleanly so
the container's restart policy re-runs the boot flow, which downloads and
loads the new versions. The first check after an update is applied records
the new baseline, so each update triggers exactly one restart.

If RCON is unreachable when the restart runs (PZ stalls RCON while the
server is paused-empty; see
[TROUBLESHOOTING.md](TROUBLESHOOTING.md#container-unhealthy-with-reading-rcon-auth-response-eof)),
the graceful save is skipped and the server is force-stopped: the world is
as of the last autosave (`AUTOSAVE_INTERVAL`) and the update is still
applied.

Mod updates are applied on the restarting boot via the Workshop download
(`MOD_UPDATE_ON_START` is forced on while `MOD_AUTO_UPDATE` is set); without
`STEAM_USER`/`STEAM_PASS` the running PZ server refreshes the Workshop items
itself at startup, as it already does for new mods. Game build updates are
applied by the existing server-file download at boot (`UPDATE_ON_START`).

`MOD_AUTO_UPDATE_ANNOUNCE=0` skips the countdown and restarts as soon as the
server is empty. Without `MOD_AUTO_UPDATE_WAIT_EMPTY` the restart happens
immediately after the countdown regardless of players.

## Updates

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `UPDATE_ON_START` | bool | `true` | Download/verify server files on container start |
| `SERVER_BRANCH` | string | (empty) | Beta branch (`unstable`, `legacy41`, etc.) |
| `STEAM_APP_ID` | string | `380870` | PZ Dedicated Server App ID |
| `STEAM_USER` | string | (empty) | Steam account for downloading server files |
| `STEAM_PASS` | string | (empty) | Steam account password |
| `STEAM_GUARD_CODE` | string | (empty) | One-time Steam Guard code from your email (first login only) |

The server files are downloaded with **DepotDownloader**, which uses the same
Steam3 protocol as SteamCMD but downloads anonymously reliably (Steam's
backend intermittently rejects SteamCMD's `app_update` for anonymous
sessions). Anonymous downloads work out of the box. If your account uses
Steam Guard, put the code from your email into `STEAM_GUARD_CODE` on the
first login; the session is remembered afterwards.

## Backups

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `BACKUP_ENABLED` | bool | `false` | Enable automatic backups |
| `BACKUP_INTERVAL` | int | `360` | Minutes between backups |
| `BACKUP_MAX_COUNT` | int | `24` | Max backups to keep |
| `BACKUP_PATH` | string | `/home/steam/Zomboid/backups` | Backup directory. Must resolve inside `DATA_DIR` (validation fails otherwise) |

## Discord

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DISCORD_WEBHOOK_URL` | string | (empty) | Discord webhook URL |
| `DISCORD_NOTIFY_START` | bool | `true` | Notify on server start |
| `DISCORD_NOTIFY_STOP` | bool | `true` | Notify on server stop |
| `DISCORD_NOTIFY_CRASH` | bool | `true` | Notify on server crash |
| `DISCORD_NOTIFY_UPDATE` | bool | `true` | Notify when an automatic restart is triggered by an update |

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
