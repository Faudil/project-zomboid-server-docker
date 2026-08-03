# Mods

This container downloads Steam Workshop mods automatically on server start
and figures out the correct `Mods=` and `WorkshopItems=` settings for you.

## Quick Setup

### Option A: Steam collection (easiest)

1. Create a [Steam Workshop collection](https://steamcommunity.com/workshop/editcollection/?appid=108600) and add your mods
2. Copy the collection ID from its URL: `steamcommunity.com/sharedfiles/filedetails/?id=3101645832`
3. Set it in `.env`:

```env
MOD_WORKSHOP_COLLECTION_IDS=3101645832
```

### Option B: Mod IDs directly

1. Open a mod's Workshop page; the number after `id=` is its Workshop ID: `https://steamcommunity.com/sharedfiles/filedetails/?id=2503743612`
2. Set them in `.env`:

```env
MOD_WORKSHOP_IDS=2160432461;2685168362;2503743612
```

### 3. Restart

```bash
docker compose down && docker compose up -d
```

That's it. `MOD_NAMES` is **optional** -- the entrypoint scans the downloaded
mods, finds each mod folder (`mod.info`), and writes the `Mods=` line itself.
You can set it explicitly to pin a load order:

```env
MOD_NAMES=SkillRecoveryJournal;MoreDescriptionForTraits
```

## How It Works

1. `MOD_WORKSHOP_IDS` and `MOD_WORKSHOP_COLLECTION_IDS` are resolved to a list of item IDs (collections via the public Steam page, or the Steam Web API when `STEAM_API_KEY` is set)
2. With `STEAM_USER`/`STEAM_PASS` set, all items are downloaded in a **single steamcmd session** to `<server-files>/steamapps/workshop/content/108600/<id>/`. Without credentials, Steam rejects anonymous downloads, so the **running server downloads them itself** from `WorkshopItems=` and the container **restarts automatically once** downloads finish so the mods load
3. Items already downloaded are skipped unless `MOD_UPDATE_ON_START=true`
4. If `MOD_NAMES` is empty, mod folder names are auto-detected from the downloads
5. The `.ini` is written with `Mods=` and `WorkshopItems=` and the server starts

```text
Resolved workshop collection 3101645832: 3 item(s)
Downloaded workshop mod 2160432461
Downloaded workshop mod 2685168362
Discovered mod "SkillRecoveryJournal" (workshop 2503743612)
Auto-detected mods (MOD_NAMES): SkillRecoveryJournal
```

## Manual (non-Workshop) Mods

Drop unzipped mod folders into `./data/Workshop/` (mounted at
`/home/steam/Zomboid/Workshop`). Any folder containing a `mod.info` file is
auto-detected and added to `Mods=` -- no extra configuration needed.

```bash
unzip my-cool-mod.zip -d data/Workshop/
docker compose up -d
```

## Debugging

List what the container found on disk and check your `MOD_NAMES` against it:

```bash
docker compose exec zomboid /home/steam/entrypoint mods
```

Example output:

```text
Discovered mod "SkillRecoveryJournal" (workshop 2503743612)
Configured MOD_NAMES: SkillRecoveryJournal;TypoMod
WARNING: MOD_NAMES entry "TypoMod" has no mod folder on disk - check the spelling
```

This catches the most common mistake: mod **folder names** differ from
Workshop **titles** (e.g. "Sapph's Cooking" lives in a folder called
`SapphsCooking`).

## Updating Mods

By default already-downloaded mods are skipped to keep restarts fast. To check
for updates every start:

```env
MOD_UPDATE_ON_START=true
```

steamcmd re-downloads each item (unchanged items resolve quickly).

## Troubleshooting

### Mod not showing up in game

- Run `entrypoint mods` to verify the folder was detected and `MOD_NAMES` matches
- Check server logs: `docker compose logs zomboid | grep -i mod`
- Ensure mods are compatible with your game version (B41 vs B42)
- Some mods require installation on the client side too
- Clients must subscribe to the same Workshop items on Steam

### Download fails

- `WARNING: workshop item <id> did not download` — if the item is public,
  anonymous workshop downloads may be failing (a known Steam-side issue).
  Set `STEAM_USER`/`STEAM_PASS` in `.env` (account owning Project Zomboid)
  and restart
- Check internet connectivity and Steam rate limits (retry after a while)
- Ensure `UPDATE_ON_START=true` (default) so the base game files are present

### Collection won't resolve

- Collections are resolved from the public Steam community page, so no
  `STEAM_API_KEY` is needed. If you see `could not resolve workshop
  collection`, the server starts without those items; common causes:
- Verify the collection is public (private collections cannot be resolved)
- Steam occasionally blocks scraping; set `STEAM_API_KEY` (free, from
  [steamcommunity.com/dev/apikey](https://steamcommunity.com/dev/apikey)) to
  switch resolution to the Steam Web API instead
- Explicit `MOD_WORKSHOP_IDS` never needs a key: you can paste the item IDs
  (from the collection page, or tools like PZ ID Grabber) to skip collection
  resolution entirely

### Server crashes after adding mods

- Check mod load order (some mods must load first) -- set `MOD_NAMES` explicitly
- Disable all mods, enable one at a time to find the culprit
- Check the in-game console for Lua errors

## Map Mods

Map mods require their map names in `MAP_NAMES`:

```env
MAP_NAMES=Muldraugh, KY;West Point, KY;MyCustomMap
```

The container automatically sets the `Map=` field in the `.ini` based on `MAP_NAMES`.
