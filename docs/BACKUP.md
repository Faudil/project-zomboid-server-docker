# Backups

The container supports scheduled automatic world backups with rotation.

## Enabling Backups

```env
BACKUP_ENABLED=true
BACKUP_INTERVAL=360
BACKUP_MAX_COUNT=24
```

- `BACKUP_INTERVAL`: Minutes between backups (360 = every 6 hours)
- `BACKUP_MAX_COUNT`: Maximum backup files to keep (oldest deleted when exceeded)

## Where Backups Are Stored

By default, backups are stored in:

```
./backups/
```

Which maps to `/home/steam/Zomboid/backups` in the container.

## Backup Contents

Each backup is a `tar.gz` archive containing the world save directory:

```
<SERVER_NAME>_backup_2026-01-15_14-30-00.123456789.tar.gz
```

The archive contains the full `Saves/Multiplayer/<SERVER_NAME>/` directory.

Before every scheduled backup the server is asked to save the world via RCON,
so archives contain a consistent snapshot rather than mid-write files.

## Manual Backup

To trigger a backup manually, send the `save` command via RCON, then copy the save directory:

```bash
# Save the world
echo "save" | nc localhost 27015

# Copy the save files
docker compose exec zomboid tar -czf /home/steam/Zomboid/backups/manual_backup.tar.gz \
    /home/steam/Zomboid/Saves/Multiplayer/servertest

# Copy to host
docker compose cp zomboid:/home/steam/Zomboid/backups/manual_backup.tar.gz ./
```

## Restoring from Backup

1. Stop the server:

```bash
docker compose down
```

2. Remove or rename the current save:

```bash
mv data/Saves/Multiplayer/servertest data/Saves/Multiplayer/servertest.old
```

3. Extract the backup:

```bash
mkdir -p data/Saves/Multiplayer/servertest
tar -xzf backups/servertest_backup_2026-01-15_14-30-00.tar.gz -C data/Saves/Multiplayer/servertest --strip-components=1
```

4. Start the server:

```bash
docker compose up -d
```

## Backup Schedule Examples

| Setting | Effect |
|---------|--------|
| `BACKUP_INTERVAL=60` | Every hour |
| `BACKUP_INTERVAL=360` | Every 6 hours (default) |
| `BACKUP_INTERVAL=720` | Every 12 hours |
| `BACKUP_INTERVAL=1440` | Daily |

## Off-Site Backups

For off-site backups, mount a remote filesystem to `./backups/` or use a script to sync:

```bash
# Cron job to sync backups to S3
0 */6 * * * aws s3 sync ./backups/ s3://my-pz-backups/
```

## Notes

- A backup is always taken on graceful shutdown (via `docker stop` or SIGTERM); the world is saved first via RCON, then archived
- The number of stored backups never exceeds `BACKUP_MAX_COUNT` (oldest are rotated after each new backup)
- Backups only include the save directory, not server configuration
- Server configuration lives in `./data/Server/` and should be backed up separately
