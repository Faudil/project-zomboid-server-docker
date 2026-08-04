# Performance

Project Zomboid b42 servers are mostly bound by **CPU and RAM**, not network.
This guide covers the levers exposed by this image, from the highest-impact
(JVM heap, zombie population) to the smallest.

## TL;DR

| Concern | Setting | Impact |
|---|---|---|
| Heap size | `MAX_RAM`/`MIN_RAM` = half of host RAM (max ~12g) | High |
| GC | `GC_CONFIG=ZGC` (default, b42's own pick) | High on large heaps |
| World clutter | `SANDBOX_MODE=performance` or `max` | High on long-running worlds |
| Zombie population | `SANDBOX_MODE=max` | High; lowers difficulty |
| CPU pinning | compose `cpuset` to physical cores | Medium on shared hosts |
| Container memory | compose `mem_limit` | Medium (avoids OOM-killer) |
| Storage | `data/` on SSD/NVMe, not NFS | Medium (saves/loads) |
| Log rotation | compose `logging:` block (shipped) | Medium (prevents disk exhaustion) |
| Host networking | `network_mode: host` (advanced) | Low-Medium (skips NAT) |
| Restart speed | `UPDATE_ON_START=false` on prod | Startup only |

## 1. JVM settings

The entrypoint patches the game's `ProjectZomboid64.json` on every start so
`MAX_RAM`, `MIN_RAM`, `GC_CONFIG` and `JVM_EXTRA_ARGS` really reach the JVM
(the game ships a hardcoded `-Xmx8g -XX:+UseZGC` that would otherwise win).

Recommended heap for your host:

| Host RAM | `MAX_RAM` / `MIN_RAM` | Notes |
|---|---|---|
| 8 GB | `4096m` | Modded servers will be tight |
| 12 GB | `6144m` | |
| 16 GB | `8192m` | Matches the game's own default |
| 32 GB | `12288m` | Only useful with many mods / players |

Rules of thumb:

- Keep `MIN_RAM` equal to `MAX_RAM` — the JVM pre-allocates and never
  resizes, avoiding mid-game pause spikes.
- The JVM uses roughly 20-30% more than `-Xmx` (metaspace, JIT, ZGC
  internal buffers), so on a 16 GB host with `-Xmx8192m` give the container
  a `mem_limit` of ~12g (see below).
- Keep `GC_CONFIG=ZGC`. ZGC is a low-pause collector and is what the game
  ships with; `G1` is the alternative if you observe high ZGC CPU usage on a
  CPU-limited host: `GC_CONFIG=G1` plus
  `JVM_EXTRA_ARGS="-XX:MaxGCPauseMillis=100"`.

## 2. Sandbox modes

`SANDBOX_MODE` applies presets over the b42 Apocalypse base
(see [CONFIGURATION.md](CONFIGURATION.md) and `.env.example`). `SANDBOX_*`
variables always override the mode.

| Mode | Applies | Gameplay impact |
|---|---|---|
| `apocalypse` (default) | Vanilla b42 Apocalypse values | None |
| `performance` | Corpses removed after 48 h (was 9 days), blood splats after 7 days (was never), rotten food after 14 days (was never), rat meta disabled, ground items after 12 h (was 24 h) | Minimal — invisible on short servers, keeps long-running worlds lean |
| `max` | Everything in `performance`, plus zombie population cut ~half (`PopulationMultiplier 0.65→0.35`, peak `1.5→1.0`), slower redistribution (12→24 h), smaller rally groups (20→10) | Noticeably fewer zombies — this is the single biggest TPS lever |

Notes:

- `ZombieConfig.ZombiesCountBeforeDelete` stays at the recommended 300 in
  all modes. Raising it *hurts* performance.
- Loot `RollsMultiplier` stays 1.0 in all modes (increasing it hurts
  performance).
- Sandbox cleanup options only affect newly spawned items; existing
  clutter in an old save persists until its chunk is reloaded.

## 3. Container tuning (docker-compose)

Example for a 16 GB host, 4 physical cores, `SANDBOX_MODE=max`:

```yaml
services:
  zomboid:
    image: ghcr.io/faudil/project-zomboid-server-docker:latest
    restart: unless-stopped
    # 8g heap + ~25% JVM overhead, plus headroom for the OS cache
    mem_limit: 12g
    # Pin to 4 physical cores; on hosts with hyperthreading avoid
    # siblings (e.g. cores 0-3 instead of 0-7)
    cpuset: "0-3"
    # Tune the CPU share if other containers share the box
    cpu_shares: 1024
    ulimits:
      nofile:
        soft: 65535
        hard: 65535
    environment:
      SANDBOX_MODE: max
      MAX_RAM: 8192m
      MIN_RAM: 8192m
    volumes:
      - ./data:/home/steam/Zomboid
      - ./server-files:/home/steam/pzserver
      - ./backups:/home/steam/Zomboid/backups
```

Notes:

- `mem_limit` must leave room above `-Xmx` (JVM overhead ~25-30%) or the
  container is OOM-killed.
- `cpuset` pins to specific cores. Prefer physical cores over hyperthread
  siblings for latency-sensitive games.
- Keep `data/` (saves) on SSD/NVMe. NFS or spinning disks cause save/load
  hitches and slow chunk streaming.
- `PAUSE_ON_EMPTY=true` (default) frees the CPU while nobody is connected.

## 4. Host kernel hints

All of these are host-level, no game parameter changes:

```sh
# Reduce swap pressure; the game is latency-sensitive (swap-in = stutters)
sudo sysctl vm.swappiness=1

# Transparent huge pages break ZGC's low-pause guarantees (latency spikes).
# "madvise" lets the JVM opt in explicitly, which is what it wants.
echo madvise | sudo tee /sys/kernel/mm/transparent_hugepage/enabled

# Keep the CPU at full frequency instead of power-saving scaling lag
sudo cpupower frequency-set -g performance   # or set the scaling_governor manually

# ZGC creates many virtual memory mappings on large (>8G) heaps; the default
# max_map_count (65530) can be too low and cause "mmap failed" at startup
sudo sysctl vm.max_map_count=262144

# UDP receive/send buffers: helps when 16+ players are connected and avoids
# dropped packets under load
sudo sysctl net.core.rmem_max=26214400
sudo sysctl net.core.wmem_max=26214400

# Mount the data volume without atime updates: the game reads save chunks
# constantly, and atime writes on every read are pure overhead
# (add "noatime" to the fstab entry or mount options)
```

Optional, for very large worlds:
`vm.overcommit_memory=1` may prevent the JVM from failing to reserve its
heap, but most hosts do not need it.

## 5. Container log rotation

The server prints chat, LuaNet messages and mod downloads to stdout, and
Docker stores those in `json-file` logs that grow without bound by default —
on long-running servers they can fill the host disk. Both compose examples
ship with rotation:

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

If you run the container via `docker run` (no compose file), add
`--log-opt max-size=10m --log-opt max-file=3`.

## 6. Host networking (advanced)

The default bridge networking NATs UDP game traffic in the kernel. For the
lowest possible latency on a dedicated host you can bypass the bridge
entirely — the server binds all interfaces, so no port mapping is needed:

```yaml
services:
  zomboid:
    network_mode: host
```

Tradeoffs: the container sees the host's network stack (no per-container
port isolation, no network namespace separation) — only use it on a host
that runs nothing else untrusted.

## 7. Backups and autosave
- Autosave every `AUTOSAVE_INTERVAL=15` minutes is a good balance. Lower
  values add write I/O during play.
- Backups (`BACKUP_ENABLED=true`) tar the whole save on the same disk —
  on very large worlds prefer off-peak scheduling by raising
  `BACKUP_INTERVAL` (default 360 min) or keep backups disabled and rely on
  autosave + nightly host snapshots.

## 8. Restarts

- `UPDATE_ON_START=false` skips the DepotDownloader check and makes restarts
  several minutes faster in production. Run with `true` (default) when you
  want to pick up game/mod updates automatically.
- The container healthcheck starts `start-period=600s` to cover the first-run
  download; later restarts become healthy in ~1-2 minutes.

## 9. Monitoring

- `docker stats` for container CPU/memory; watch that the RSS stays below
  `mem_limit` over a few days (ZGC grows slowly).
- In-game: the admin console / `TPS` command shows ticks per second. A
  healthy PZ server runs at 30-60 TPS; sustained drops mean the CPU-bound
  work (population, clutter) is too high — step down `SANDBOX_MODE` or lower
  `MAX_PLAYERS`.
