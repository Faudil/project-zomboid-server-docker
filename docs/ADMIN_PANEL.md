# Admin Panel Integration

[Zomboid Control Panel](https://github.com/fpsacha/zomboid-control-panel) is a web admin interface for Project Zomboid servers. It provides:

- Server start/stop/restart
- RCON console
- Live world map with player positions
- Workshop mod manager
- Server configuration editor
- Player management (kick, ban, teleport)
- Events & weather control
- Performance telemetry
- Backup management
- Discord bot

## Setup (Separate Container)

1. Clone the ZCP repository:

```bash
git clone https://github.com/fpsacha/zomboid-control-panel.git
cd zomboid-control-panel
```

2. Create a Docker network so the panel can reach the PZ server:

```bash
docker network create pz-net
```

3. Update your PZ server's `docker-compose.yml` to use this network:

```yaml
services:
  zomboid:
    # ... existing config ...
    networks:
      - pz-net

networks:
  pz-net:
    external: true
```

4. Relaunch the PZ server:

```bash
docker compose down && docker compose up -d
```

5. For the ZCP docker-compose, set the PZ server path. The panel needs access to:

- PZ server install files (optional, for server control)
- PZ config/save files (for config editor, PanelBridge)
- RCON connection

In ZCP's `.env`:

```env
PZ_SERVER_PATH=/path/to/your/server-files
PZ_DATA_PATH=/path/to/your/data
RCON_HOST=zomboid
RCON_PORT=27015
RCON_PASSWORD=your-rcon-password
PUID=1000
PGID=1000
```

6. Start ZCP on the same network:

```bash
docker compose up -d
```

7. Open `http://localhost:3001` and create your admin account.

## PanelBridge (Recommended)

PanelBridge is a Lua mod that extends RCON capabilities (teleport, heal, weather control, etc.).

1. In ZCP Settings, enable PanelBridge
2. It will copy `PanelBridge.lua` to your server's Lua directory
3. Set `DoLuaChecksum=false` in your server's `.ini` (you can do this from the ZCP config editor)
4. Restart the PZ server

## Alternative: Host-Installed Panel

If you prefer to run ZCP directly on the host (not Docker):

1. Download from [releases](https://github.com/fpsacha/zomboid-control-panel/releases)
2. Configure `PZ_SERVER_PATH` and `PZ_DATA_PATH` to point at your Docker volumes
3. Set `RCON_HOST=127.0.0.1` if the PZ container publishes RCON to the host
