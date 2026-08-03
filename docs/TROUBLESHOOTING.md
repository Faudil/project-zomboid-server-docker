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

The JVM heap size is controlled by `MAX_RAM`. Default is 4096m (4GB). The actual process may use more due to JVM overhead (~20-30% more than `-Xmx`). Reduce `MAX_RAM` or increase host memory.

## Workshop mods not downloading

1. Ensure `MOD_WORKSHOP_IDS` is correctly formatted (semicolons, no trailing spaces)
2. Check internet connectivity from the container:
   ```bash
   docker compose exec zomboid ping google.com
   ```
3. Set `UPDATE_ON_START=true` (default)

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
