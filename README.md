# Comuline API

Go port of [comuline/api](https://github.com/comuline/api) — Indonesian KRL Commuter Line and MRT Jakarta schedule data, plus a Telegram bot for commuters. Built with Gin + GORM.

## Requirements

- Go 1.22+
- Docker (for PostgreSQL and Redis)

> **Deploying to production?** See [DEPLOYMENT.md](DEPLOYMENT.md) for VPS, Fly.io, and Render guides.

---

## Setup

### 1. Clone and install dependencies

```bash
git clone <your-repo-url>
cd comu-api
go mod download
```

### 2. Get your `KAI_AUTH_TOKEN`

The token is extracted from the official KCI website:

1. Open **https://kci.id/perjalanan-krl** in your browser
2. Open **Developer Tools** (`F12` or `Cmd+Option+I` on Mac)
3. Go to the **Network** tab
4. **Reload the page** (`F5` or `Cmd+R`) to capture requests
5. In the filter/search box, type **`krl-station`** to find the request
6. Click on the `krl-station` request
7. Go to the **Headers** tab of that request
8. Scroll down to **Request Headers** and find the `Authorization` header
9. Copy the value — it looks like `Bearer eyJ0eXAiOi...`
10. Copy only the token part **after** `Bearer ` (without the word "Bearer")

### 3. Configure environment

```bash
cp .env.example .env
```

Open `.env` and fill in your values:

```env
DATABASE_URL=postgres://comu:comu@localhost:5432/comuapi
REDIS_URL=redis://localhost:6379
COMULINE_ENV=development
KAI_AUTH_TOKEN=<paste your token here>
PORT=8080
SYNC_SECRET=<optional secret for POST /sync endpoint>

# Telegram bot (optional — only needed to run the bot)
TELEGRAM_TOKEN=<your BotFather token>
TIMEZONE=Asia/Jakarta
OPEN_METEO_BASE=https://api.open-meteo.com/v1/forecast

# Set to true to auto-sync data daily at 00:10 WIB
AUTO_SYNC=false
```

### 4. Start PostgreSQL and Redis

```bash
make up
```

### 5. Sync data

Populate the database with stations and schedules from the KRL API:

```bash
make sync
```

This runs station sync first, then schedule sync. It may take a few minutes to fetch all schedules.

### 6. Run the API server

```bash
make run
```

The server starts on `http://localhost:8080`.

### 7. Run the Telegram bot (optional)

```bash
make run-bot
```

The bot polls Telegram for updates. Make sure `TELEGRAM_TOKEN` is set in `.env`.

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/status` | Health check (pings DB and Redis) |
| `GET` | `/docs/index.html` | Swagger UI |
| `GET` | `/openapi` | Raw OpenAPI JSON |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/sync` | Trigger manual sync (requires `X-Sync-Secret` header) |
| `GET` | `/v1/station` | List all stations (supports `?page=1&limit=50`) |
| `GET` | `/v1/station/:id` | Get station by ID (e.g. `MRI`) |
| `GET` | `/v1/schedule/:station_id` | Schedules for a station (supports `?page=1&limit=50`) |
| `GET` | `/v1/route/:train_id` | Train route with stop sequence |
| `GET` | `/v1/mrt/stations` | List all MRT stations |
| `GET` | `/v1/mrt/stations/:id` | Get MRT station by ID (e.g. `LBB`) |
| `GET` | `/v1/mrt/schedules/:station_id` | MRT schedules for a station |
| `GET` | `/v1/mrt/routes` | MRT North-South Line route |

---

## Telegram Bot

The bot provides a conversational interface for KRL schedule queries, weather, and commute alerts.

### First-time setup

```
/start          — welcome message and quick overview
/set_route      — set your home & away stations + departure times
/set_schedule   — choose your work days (e.g. mon,tue,wed,thu,fri)
/toggle_notifs  — enable daily push notifications on work days
```

### Checking schedules

```
/go_morning     — trains from home → away around your morning time (± 1 hr) + weather
/go_evening     — trains from away → home around your evening time (± 1 hr) + weather
```

Requires `/set_route` to be configured first.

```
/schedule                   — interactive: bot asks origin, destination, time
/schedule <station>         — skip straight to destination prompt
                              e.g. /schedule Rawa Buaya
```

The `/schedule` command is a 3-step dialogue. After sending the command, **reply with plain text** at each prompt — do not send another `/schedule` command:

```
You:  /schedule
Bot:  From which station?
You:  Rawa Buaya
Bot:  To which station?
You:  Jakarta Kota
Bot:  What time? (HH:MM or 'now')
You:  now
Bot:  🚆 Trains Rawa Buaya → Jakarta Kota (08:15–10:15): ...
```

Time formats accepted: `HH:MM`, `now`, `today HH:MM`, `tomorrow HH:MM`

### One-time alerts

```
/schedule_once  — set a one-time alert: bot will push schedule + weather at the chosen time
/list_alerts    — view all pending alerts with their IDs
/cancel_alert <id>  — cancel a scheduled alert by its ID prefix (first 8 chars)
```

### Other commands

```
/station <query>  — fuzzy search stations by name or code
                    e.g. /station depok  or  /station BW
/settings         — view your current profile (route, times, work days, language)
/lang             — toggle language between English and Bahasa Indonesia
/help             — full command list
```

### Notes

- Station codes work as search queries (e.g. `BW`, `JAKK`, `DP`)
- Weather is fetched from Open-Meteo using station coordinates (free, no API key needed)
- One-time alerts are stored in Redis and delivered within ~1 minute of the scheduled time
- Daily push notifications fire at 06:45 (morning) and 17:15 (evening) WIB on your configured work days

---

## Make targets

```bash
make run              # start the API server
make run-bot          # start the Telegram bot
make build            # compile to bin/api
make sync             # run all syncs (KRL + MRT)
make sync-station     # KRL station sync only
make sync-schedule    # KRL schedule sync only
make sync-mrt         # MRT station + schedule sync
make sync-mrt-station # MRT station sync only
make sync-mrt-schedule # MRT schedule sync only
make up               # start Docker services (postgres + redis)
make down             # stop Docker services
make swag             # regenerate Swagger docs
make tidy             # go mod tidy + verify
make clean            # remove build artifacts
make migrate          # run database migrations up
make migrate-down     # roll back all database migrations
```

For production deployment (`make prod-up`, Fly.io, Render) see [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Regenerating Swagger docs

Install `swag` once:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

Then regenerate:

```bash
make swag
```

---

## Notes

- All API responses use the standard envelope: `{ "metadata": { "success": true }, "data": ... }`
- API responses are cached in Redis until midnight WIB (GMT+7)
- `KAI_AUTH_TOKEN` is only needed for `make sync`, not for running the API server
- `AUTO_SYNC=true` enables automatic daily sync at 00:10 WIB without needing a cron job
