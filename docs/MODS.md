# Workshop Mods

This container supports automatic Steam Workshop mod downloads on server start.

## Quick Setup

1. Create a Steam Workshop collection with your mods
2. Use [PZ ID Grabber](https://pzidgrabber.com) to extract Mod IDs and Workshop IDs
3. Set the environment variables:

```env
MOD_NAMES=ClaimNonResidential;MoreDescriptionForTraits;SkillRecoveryJournal
MOD_WORKSHOP_IDS=2160432461;2685168362;2503743612
```

4. Restart the server:

```bash
docker compose down && docker compose up -d
```

## How It Works

1. On container start, the entrypoint parses `MOD_WORKSHOP_IDS`
2. For each Workshop ID, it runs `steamcmd workshop_download_item 108600 <id>`
3. Mods are downloaded to `<server-files>/steamapps/workshop/content/108600/<id>/`
4. The `.ini` file is written with `Mods=` and `WorkshopItems=` entries
5. The server loads mods on startup

## Finding Mod IDs

### Method 1: Steam Workshop

1. Browse the [Project Zomboid Workshop](https://steamcommunity.com/app/108600/workshop/)
2. Each mod URL contains its Workshop ID: `https://steamcommunity.com/sharedfiles/filedetails/?id=2160432461`
3. The number after `id=` is the Workshop ID

### Method 2: PZ ID Grabber

1. Create a [Steam Workshop Collection](https://steamcommunity.com/workshop/editcollection/?appid=108600)
2. Add all desired mods to the collection
3. Visit [PZ ID Grabber](https://pzidgrabber.com)
4. Paste your collection URL
5. Copy the `Mods=` and `WorkshopItems=` lines

### Method 3: From an existing server

Check an existing `servertest.ini` for the `Mods=` and `WorkshopItems=` values.

## Adding Mods to a Running Server

Mods are loaded at server start. To add mods:

1. Update `.env` with new `MOD_NAMES` and `MOD_WORKSHOP_IDS`
2. Restart the server:

```bash
docker compose restart
```

## Troubleshooting

### Mod not showing up

- Verify the Workshop ID is correct
- Check server logs: `docker compose logs zomboid | grep -i mod`
- Ensure mods are compatible with your game version (B41 vs B42)
- Some mods require both client and server installation

### Download fails

- Check internet connectivity
- Steam may rate-limit anonymous downloads -- wait and retry
- Ensure `UPDATE_ON_START=true` (default)

### Server crashes after adding mods

- Check mod load order (some mods must load first)
- Disable all mods, enable one at a time to find the culprit
- Check the in-game console for Lua errors

## Map Mods

Map mods require additional entries in your `.ini`. If a mod adds maps, you must add them to `MAP_NAMES`:

```env
MAP_NAMES=Muldraugh, KY;West Point, KY;MyCustomMap
```

The container automatically sets the `Map=` field in the `.ini` based on `MAP_NAMES`.
