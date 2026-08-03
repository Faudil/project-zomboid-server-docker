# Quick Start Guide

## 1. Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) installed
- Ports `16261/UDP` and `16262/UDP` forwarded on your router/firewall
- At least 4GB RAM and 5GB free disk space

## 2. Download the Repository

### Option A: One-liner (recommended for a single server)

No download needed, just run:

```bash
docker run -d \
  --name pz-server \
  -p 16261:16261/udp -p 16262:16262/udp -p 27015:27015 \
  -e ADMIN_PASSWORD=your-secure-admin-password \
  -e RCON_PASSWORD=your-secure-rcon-password \
  -v pz-data:/home/steam/Zomboid \
  faudil/project-zomboid-server:latest
```

Set `ADMIN_PASSWORD` and `RCON_PASSWORD` to secure values before running. First launch downloads the server files via DepotDownloader (~5-10 minutes depending on internet speed). Skip to [Monitor](#6-monitor).

### Option B: Docker Compose (recommended for configuration)

```bash
git clone https://github.com/faudil/project-zomboid-server-docker.git
cd project-zomboid-server-docker
```

## 3. Configure

```bash
cp .env.example .env
```

Edit `.env` with your text editor and set at minimum:

```env
SERVER_NAME=myserver
PUBLIC_NAME=My Awesome Server
ADMIN_PASSWORD=your-secure-admin-password
RCON_PASSWORD=your-secure-rcon-password
TZ=America/New_York
```

## 4. Create Data Directories

The container runs as UID 1000. Create the directories *before* the first
start so Docker does not auto-create them as root:

```bash
mkdir -p data server-files backups
sudo chown -R 1000:1000 data server-files backups
```

If you see `Permission errors - the container cannot write to its volumes`
on start, run the `chown` above and start again.

## 5. Start the Server

```bash
docker compose up -d
```

First launch downloads the server files via DepotDownloader (~5-10 minutes depending on internet speed).

## 6. Monitor

```bash
docker compose logs -f
```

Wait for the log line:

```
LuaNet: Initialization [DONE]
```

## 7. Connect

1. Launch Project Zomboid
2. Click **Join** → **Favorites**
3. Add your server's public IP address
4. Port `16261`
5. Enter any account name and password
6. Save and join

## 8. Administer

Use the in-game admin panel (press `Esc`, click **Admin**) or RCON to manage the server.

To send RCON commands:

```bash
# Using netcat
nc your-server-ip 27015
# Enter RCON password, then commands like:
help
players
save
quit
```

## 9. Stop the Server

```bash
docker compose down
```

This sends a graceful shutdown signal: the server saves the world, then exits.

## 10. Next Steps

- [Add Workshop mods](MODS.md)
- [Set up automatic backups](BACKUP.md)
- [Configure Discord notifications](DISCORD.md)
- [Add a web admin panel](ADMIN_PANEL.md)
- [Run multiple servers](MULTI_INSTANCE.md)
