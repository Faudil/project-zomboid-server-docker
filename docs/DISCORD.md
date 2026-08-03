# Discord Webhook

Send server events (start, stop, crash) to a Discord channel via webhook.

## Setup

### 1. Create a Discord Webhook

1. Open your Discord server
2. Go to the channel where you want notifications
3. Click the gear icon (Edit Channel)
4. Select **Integrations** → **Webhooks** → **New Webhook**
5. Name it (e.g., "PZ Server")
6. Click **Copy Webhook URL**

### 2. Configure

Add the URL to your `.env`:

```env
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/XXXXXXXX/YYYYYYYYYYYY

DISCORD_NOTIFY_START=true
DISCORD_NOTIFY_STOP=true
DISCORD_NOTIFY_CRASH=true
```

### 3. Restart

```bash
docker compose down && docker compose up -d
```

## Notification Types

| Event | Color | Description |
|-------|-------|-------------|
| Server started | Green | Sent when the JVM process launches |
| Server stopped | Red | Sent on graceful shutdown (`docker stop`) |
| Server crashed | Yellow | Sent when the process exits unexpectedly |

## Example Embeds

**Server Started:**

```
🟢 Server Started
My Awesome Server is now online
Server: servertest | 2026-01-15T19:30:00Z
```

**Server Stopped:**

```
🔴 Server Stopped
My Awesome Server has shut down
Server: servertest | 2026-01-15T20:00:00Z
```

**Server Crashed:**

```
💥 Server Crashed
My Awesome Server exited unexpectedly: exit status 1
Server: servertest | 2026-01-15T19:45:00Z
```

## Troubleshooting

### Notifications not appearing

- Verify the webhook URL is correct (test with curl)
- Ensure the container can reach Discord's API (no firewall blocking outbound HTTPS)
- Check container logs: `docker compose logs zomboid`

### Test the webhook

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"content":"Test from PZ server"}' \
  YOUR_WEBHOOK_URL
```
