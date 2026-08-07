# Troubleshooting

## Server fails to start

### Check logs

```bash
docker compose logs zomboid
```

### Common causes:

1. **Port 16261/UDP already in use**
   ```bash
   sudo lsof -i :16261
   ```
   Change `DEFAULT_PORT` in `.env` if needed.

2. **Not enough memory**
   Reduce `MAX_RAM` in `.env` (e.g., `2048m`).

3. **First-run admin password prompt**
   The container auto-generates a password. The first interactive prompt is bypassed. If you see the prompt in logs, it means the server is waiting for input -- check that `ADMIN_PASSWORD` is set.

## Can't connect to server

1. **Port forwarding**: Ensure ports 16261/UDP and 16262/UDP are forwarded on your router
2. **Firewall**: Check that your server's firewall allows these ports
3. **Check server is running**: `docker compose ps`
4. **Check logs for errors**: `docker compose logs --tail=50 zomboid`
5. **Wait for initialization**: The server must show `LuaNet: Initialization [DONE]` before accepting connections

## Server eats all my RAM

The JVM heap size is controlled by `MAX_RAM` (default 4096m / 4GB), applied by
patching the game's `ProjectZomboid64.json` on every start. The actual process
uses ~20-30% more than `-Xmx` due to JVM overhead (metaspace, JIT, ZGC
buffers), so a 4GB heap shows up as roughly 5-5.5GB RSS. Reduce `MAX_RAM`
(and `MIN_RAM`), or increase host memory. If you see a heap larger than
`MAX_RAM` in `docker stats`, your container was started before the json
patching existed — restart it. See [PERFORMANCE.md](PERFORMANCE.md) for sizing.

## Server files not downloading (start-server.sh missing)

If the container exits with:

```text
ERROR installing/updating server: could not download app 380870 after 6 attempts
```

The server files are downloaded with DepotDownloader, which works reliably
anonymously. A persistent failure here is almost always one of:

1. **No network access from the container** — check `docker compose logs`
   for connection errors, and verify the host can reach Steam
2. **`STEAM_USER` set without `STEAM_PASS`** — the entrypoint refuses to run
   to avoid an interactive password prompt hanging forever
3. **Bad credentials or expired Steam Guard code** — if you use
   `STEAM_USER`/`STEAM_PASS`, make sure the account owns Project Zomboid and
   the `STEAM_GUARD_CODE` from your email is current

If the server files were downloaded elsewhere (e.g. via the Steam client),
you can skip the download entirely: place the files in the `server-files`
volume, set `UPDATE_ON_START=false`, and restart. The entrypoint skips
SteamCMD/DepotDownloader whenever `start-server.sh` already exists.

## Workshop mods not downloading

1. Ensure `MOD_WORKSHOP_IDS` is correctly formatted (semicolons, no trailing spaces)
2. Check internet connectivity from the container:
   ```bash
   docker compose exec zomboid ping google.com
   ```
3. Set `UPDATE_ON_START=true` (default)
4. Anonymous Steam sessions can no longer download workshop items (Steam-side
   change). Without `STEAM_USER`/`STEAM_PASS` the running server downloads
   them itself from `WorkshopItems=` -- you should see
   `Workshop: download ...` in the logs and the container restarting once
   when it is done
5. If nothing downloads at all, set `STEAM_USER`/`STEAM_PASS` (a Steam
   account, ideally one owning Project Zomboid) in `.env` and restart --
   downloads then happen before the server starts

## Permission errors

The container runs as UID 1000 (`steam`). If the host directories mounted
into it are owned by root, the entrypoint exits with a message like:

```text
Permission errors - the container cannot write to its volumes:
  - /home/steam/Zomboid is not writable by UID 1000: ...
```

This usually happens because the bind-mount directories (`./data`,
`./server-files`, `./backups`) did not exist when you ran `docker compose up`
-- **Docker auto-creates missing host directories as root**.

Fix ownership from the compose directory:

```bash
sudo chown -R 1000:1000 data/ server-files/ backups/
docker compose up -d
```

To avoid it, create the directories *before* the first start:

```bash
mkdir -p data server-files backups
sudo chown -R 1000:1000 data/ server-files/ backups/
```

Named volumes (e.g. `-v pz-data:/home/steam/Zomboid`) do not have this
problem -- Docker initializes their ownership from the image.

## Backup not working

1. Ensure `BACKUP_ENABLED=true`
2. Verify `BACKUP_PATH` is writable by UID 1000
3. Check container logs for backup errors:
   ```bash
   docker compose logs zomboid | grep -i backup
   ```

## Healthcheck failing

Forcibly restart if the healthcheck keeps failing:

```bash
docker compose restart
```

If persistent, check:
- RCON port is not blocked: `nc -zv localhost 27015`
- RCON password is correct
- Server is actually running (not stuck in startup)

### Container unhealthy with "reading RCON auth response: EOF"

Symptoms: the container shows `(unhealthy)`, healthcheck logs show
`Healthcheck failed: reading RCON auth response: EOF`, and backups log
`RCON connection failed before backup: ... EOF`. Players can still join the
game fine.

This is a **Project Zomboid server-side RCON stall**. PZ accepts at most 5
concurrent RCON connections; exec commands (`hello`, `players`, `save`, ...)
are answered from the game's main loop, which stalls while the server is
paused-empty or still loading. When responses don't come, PZ's RCON client
threads pile up and fill the 5-slot limit, after which **every new
connection is accepted and closed instantly with no log** -- the silent EOF
above. The stall usually clears on its own as soon as a player joins (the
game unpauses and drains the queue); a container restart alone may not fix
it while the server stays empty.

The entrypoint's healthcheck is auth-only by design (it never sends RCON
exec commands), so it does not feed the stall. The backup scheduler and the
auto-update restart (`MOD_AUTO_UPDATE`) tolerate RCON being down: backups
skip the save and still snapshot the on-disk world, and an auto-update
restart proceeds anyway (the world is as of the last autosave).

If the stall persists and you need to recover now: have a player join, or
`docker compose restart` and keep a player online during startup.

## Docker Desktop on Windows (WSL2)

SteamCMD downloads are extremely slow via WSL2 due to filesystem translation overhead. Solutions:
- Move Docker data to a WSL2-managed directory (not a Windows mount)
- Use native Linux (VM, VPS, or bare metal)

## Assertion Failed: Illegal termination of worker thread

This can happen when switching between Build 41 and Build 42:

```bash
docker compose down
rm -rf data/ server-files/
mkdir -p data server-files backups
docker compose up -d
```

Your saves will be lost. Back up first if needed.

## Getting help

- [GitHub Issues](https://github.com/faudil/project-zomboid-server-docker/issues)
- [PZWiki Dedicated Server](https://pzwiki.net/wiki/Dedicated_Server)
- [Project Zomboid Discord](https://discord.gg/theindiestone)
