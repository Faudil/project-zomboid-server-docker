# Multi-Instance

Run multiple Project Zomboid servers from a single Docker host.

## Using docker-compose.multi.yml

The repository includes `docker-compose.multi.yml` with a two-server example.

### 1. Configure

```bash
cp .env.example .env
```

Set shared settings in `.env` and instance-specific settings in the compose file.

### 2. Create Directories

```bash
mkdir -p data-server1 data-server2 backups-server1 backups-server2 server-files
```

### 3. Start

```bash
docker compose -f docker-compose.multi.yml up -d
```

## Important Rules

1. **Each server must use unique ports** -- default is 16261/16262/27015; next would be 16263/16264/27016
2. **Each server must have a unique `SERVER_NAME`** -- this determines the save directory
3. **Each server needs its own data directory** -- use separate volumes/bind mounts
4. **Sharing server-files is fine** -- multiple instances can share the same game install

## Custom Instance Setup

Add a new service to your compose file:

```yaml
zomboid-hc:
  image: ghcr.io/faudil/project-zomboid-server-docker:latest
  container_name: pz-hardcore
  restart: unless-stopped
  stop_grace_period: 60s
  environment:
    - SERVER_NAME=hardcore
    - PUBLIC_NAME=Hardcore Survival
    - DEFAULT_PORT=16265
    - UDP_PORT=16266
    - RCON_PORT=27017
    - MAX_PLAYERS=32
    - PVP=true
    - MAX_RAM=8192m
  env_file:
    - .env
  ports:
    - "16265:16265/udp"
    - "16266:16266/udp"
    - "27017:27017/tcp"
  volumes:
    - ./data-hardcore:/home/steam/Zomboid
    - ./server-files:/home/steam/pzserver
    - ./backups-hardcore:/home/steam/Zomboid/backups
```

## Port Planning

| Instance | Game Port | Steam Port | RCON Port |
|----------|-----------|------------|-----------|
| Server 1 | 16261 | 16262 | 27015 |
| Server 2 | 16263 | 16264 | 27016 |
| Server 3 | 16265 | 16266 | 27017 |
| ... | +2 | +2 | +1 |

## Resource Planning

Each PZ server needs:
- **CPU**: 2-4 cores
- **RAM**: 4-8GB+ (configurable via `MAX_RAM`)
- **Disk**: ~5GB per instance (shared install files reduce this)

Plan your host resources accordingly. For 3+ servers, consider at least 32GB RAM and 8+ CPU cores.

## Managing Individual Servers

```bash
# Stop only server 2
docker compose -f docker-compose.multi.yml stop zomboid-server2

# Start only server 2
docker compose -f docker-compose.multi.yml up -d zomboid-server2

# View logs for server 1
docker compose -f docker-compose.multi.yml logs -f zomboid-server1

# Restart only server 1
docker compose -f docker-compose.multi.yml restart zomboid-server1
```
