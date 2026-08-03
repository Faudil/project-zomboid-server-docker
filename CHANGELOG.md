# Changelog

## [Unreleased]

### Added
- Initial release
- Go-based entrypoint with config generation (ini + lua)
- SteamCMD integration for server install/update
- Workshop mod auto-download
- Graceful shutdown via RCON save + quit
- RCON-based healthcheck
- Automatic world backups with rotation
- Discord webhook notifications (start, stop, crash)
- Multi-instance docker-compose example
- Complete documentation (README, Quickstart, Configuration, Mods, Backups, Discord, Admin Panel, Troubleshooting)
- GitHub Actions CI/CD (build, lint, push to GHCR + Docker Hub)

### Fixed
- SteamCMD no longer reports "Server files up to date" when the download silently failed: output is captured, `ERROR! Failed to install app`-style failures are detected, and the install is verified by checking `start-server.sh` afterwards
- Steam's transient "Missing file permissions" app_update regression (steam-for-linux #10979) is now retried automatically with a backoff instead of failing the container on the first attempt; permanent failures (bad credentials) fail immediately
- `STEAM_USER`/`STEAM_PASS`/`STEAM_GUARD_CODE` supported as the reliable workaround for the Steam-side failure and for workshop downloads when anonymous fails
- Refuses to run when `STEAM_USER` is set without `STEAM_PASS` (steamcmd would otherwise prompt and hang forever)
- Workshop collection resolution warning now points at `STEAM_API_KEY` when the Steam API rejects the keyless request
- Startup now fails fast with an actionable message when the mounted volumes are not writable by UID 1000 (previously a bare `credentials.env: permission denied`); docs cover creating/chowning the host directories before first start
- Crash at startup when `DISCORD_WEBHOOK_URL` is unset (nil-pointer in webhook notifications)
- Healthcheck failing when `RCON_PASSWORD`/`ADMIN_PASSWORD` are auto-generated (passwords are now persisted to `<DATA_DIR>/credentials.env` and shared with the healthcheck)
- Final backup running before the world was saved on shutdown (server is stopped/saved first)
- Duplicate SIGTERM/SIGINT handlers racing during shutdown; `Stop()` now waits with a timeout and escalates to SIGKILL
- Shutdown only killing the bash wrapper instead of the whole process group (java child now terminated too)
- RCON commands waiting out the full read deadline (response is returned at the `RCON:` prompt line)
- Backup rotation exceeding `BACKUP_MAX_COUNT` (rotate now runs after creating the new backup)
- Backups triggered in the same second colliding (names now use nanosecond precision)

### Changed
- `MAX_RAM`, `MIN_RAM`, `GC_CONFIG`, `JVM_EXTRA_ARGS` are now applied to the server JVM via `_JAVA_OPTIONS` (previously ignored)
- `BIND_IP` is now written to the server `.ini` (previously ignored)
- Sandbox gameplay values are configurable via `SANDBOX_*` environment variables; `SandboxVars.lua` output is deterministic
- Auto-generated passwords are no longer printed to logs; they live in `<DATA_DIR>/credentials.env` (mode 0600)
- Scheduled backups issue an RCON `save` first so archives are consistent
- Docker healthcheck start period raised to 600s to cover the first-run SteamCMD install
- Removed non-functional `PUID`/`PGID`, `GAME_VERSION`, and `DISCORD_NOTIFY_PLAYERS` variables
- Health endpoint reports `installing` / `starting` / `healthy` / `stopping` status

### Mods
- `MOD_NAMES` is now optional: mod folder names are auto-detected from downloaded workshop items and manual mods (folders containing `mod.info`)
- `MOD_WORKSHOP_COLLECTION_IDS` resolves Steam workshop collections to item IDs via the Steam Web API (keyless best-effort, `STEAM_API_KEY` supported)
- Workshop items download in a single steamcmd batch instead of one process per mod
- `MOD_UPDATE_ON_START` re-downloads all mods on every start to pick up updates
- Per-item download verification with a warning for private/region-locked/invalid items
- Manual mods: unzip mod folders into `<DATA_DIR>/Workshop/` and they are auto-detected
- `entrypoint mods` subcommand lists discovered mod folders and flags `MOD_NAMES` typos
- Mod list parsing accepts `;`, `,`, or whitespace separators and drops invalid IDs

### Tests
- Added unit tests: config parsing/validation, credential persistence, ini + sandbox generation, backup rotation/archive integrity, RCON protocol (fake server), process-group shutdown, webhook nil-safety
- CI now runs `go test ./...`
