# Deployment Guide

This document covers all supported deployment methods for Comu API and the Telegram bot.

---

## Table of Contents

- [Environment Variables](#environment-variables)
- [Local Development](#local-development)
- [VPS / Self-Hosted (Production)](#vps--self-hosted-production)
- [Fly.io](#flyio)
- [Render](#render)
- [Updating & Maintenance](#updating--maintenance)

---

## Environment Variables

All deployment methods rely on the same set of environment variables.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | — | PostgreSQL connection string |
| `REDIS_URL` | ✅ | — | Redis connection string |
| `KAI_AUTH_TOKEN` | ✅ (sync only) | — | Bearer token from KCI website (for data sync) |
| `COMU_ENV` | — | `development` | Set to `production` for JSON logs and Gin release mode |
| `PORT` | — | `8080` | HTTP port for the API server |
| `SYNC_SECRET` | — | — | Secret for `POST /sync` endpoint. Omit to disable auth. |
| `TELEGRAM_TOKEN` | ✅ (bot only) | — | BotFather token for the Telegram bot |
| `OPEN_METEO_BASE` | — | `https://api.open-meteo.com/v1/forecast` | Weather API base URL |
| `TIMEZONE` | — | `Asia/Jakarta` | Timezone for the bot (WIB) |
| `AUTO_SYNC` | — | `false` | Set to `true` to auto-sync data daily at 00:10 WIB |

---

## Local Development

Uses Docker for PostgreSQL and Redis; Go runs on the host.

```bash
# 1. Start PostgreSQL and Redis
make up

# 2. Sync station and schedule data
make sync

# 3. Run the API server
make run

# 4. Run the Telegram bot (separate terminal)
make run-bot
```

Health check: `curl http://localhost:8080/status`

---

## VPS / Self-Hosted (Production)

Uses `docker-compose.prod.yml` which runs everything in containers: API, bot, PostgreSQL, and Redis.

### Prerequisites

- A VPS with Docker and Docker Compose installed
- A domain pointed at the VPS IP
- SSH access

### Steps

**1. Copy files to the server**

```bash
scp -r . user@your-server:/opt/comu
ssh user@your-server
cd /opt/comu
```

**2. Create your `.env` file**

```bash
cp .env.example .env
```

Edit `.env`:

```env
DATABASE_URL=postgres://comu:yourpassword@postgres:5432/comuapi
REDIS_URL=redis://redis:6379
COMU_ENV=production
KAI_AUTH_TOKEN=your_token_here
PORT=8080
SYNC_SECRET=a-strong-random-secret
TELEGRAM_TOKEN=your_bot_token_here
TIMEZONE=Asia/Jakarta
AUTO_SYNC=true

# Postgres credentials (used by the postgres container)
POSTGRES_USER=comu
POSTGRES_PASSWORD=yourpassword
POSTGRES_DB=comuapi
```

**3. Configure external HTTPS/TLS (optional)**

This stack now exposes the API directly on port `8080`:

- Endpoint: `http://<your-server-ip-or-domain>:8080`
- Health check: `http://<your-server-ip-or-domain>:8080/status`

If you need HTTPS, terminate TLS outside this compose stack (for example with your cloud load balancer, ingress controller, Traefik, or Nginx on the host).

**4. Start all services**

```bash
make prod-up
```

This builds the API and bot images, then starts all services (app, bot, postgres, redis).

**5. Sync data**

Run sync from the host (one-time):

```bash
make sync
```

Or trigger it via the API after deploy:

```bash
curl -X POST http://your-server-ip-or-domain:8080/sync \
  -H "X-Sync-Secret: a-strong-random-secret"
```

With `AUTO_SYNC=true`, data re-syncs automatically every day at 00:10 WIB.

**6. Verify**

```bash
# Check all containers are healthy
docker compose -f docker-compose.prod.yml ps

# Follow logs
make prod-logs

# Health check
curl http://your-server-ip-or-domain:8080/status
```

### Useful prod commands

```bash
make prod-up      # build and start all services
make prod-down    # stop all services (data volumes are preserved)
make prod-logs    # follow logs from all services
```

### Database backup

```bash
docker compose -f docker-compose.prod.yml exec postgres \
  pg_dump -U comu comuapi > backup_$(date +%F).sql
```

To restore:

```bash
cat backup_2026-03-05.sql | docker compose -f docker-compose.prod.yml exec -T postgres \
  psql -U comu comuapi
```

---

## Fly.io

Fly.io runs the API as a single container. PostgreSQL and Redis must be provisioned separately as Fly managed services or external.

### Prerequisites

```bash
brew install flyctl
flyctl auth login
```

### Steps

**1. Provision a Postgres database**

```bash
flyctl postgres create --name comu-db --region sin
flyctl postgres attach comu-db --app comu-api
```

This sets `DATABASE_URL` automatically as a secret.

**2. Provision Redis**

```bash
flyctl redis create --name comu-redis --region sin
```

Copy the connection string and set it as a secret:

```bash
flyctl secrets set REDIS_URL="redis://default:password@your-redis.upstash.io:6379"
```

**3. Set remaining secrets**

```bash
flyctl secrets set \
  KAI_AUTH_TOKEN="your_token_here" \
  SYNC_SECRET="a-strong-random-secret" \
  TELEGRAM_TOKEN="your_bot_token_here" \
  COMU_ENV="production" \
  TIMEZONE="Asia/Jakarta" \
  AUTO_SYNC="true"
```

**4. Deploy**

The `fly.toml` is already configured (region: Singapore, 256 MB RAM, `/status` health check):

```bash
flyctl deploy
```

**5. Sync data after first deploy**

```bash
flyctl ssh console -C "wget -qO- http://localhost:8080/status"

curl -X POST https://comu-api.fly.dev/sync \
  -H "X-Sync-Secret: a-strong-random-secret"
```

**6. Scale**

```bash
# Scale memory if needed
flyctl scale memory 512

# Run multiple instances
flyctl scale count 2
```

> **Note:** The Telegram bot is not included in `fly.toml`. To run the bot on Fly.io, create a separate app pointing to `Dockerfile.bot`.

---

## Render

Render can run the API as a Web Service and the bot as a Background Worker, with managed PostgreSQL and Redis.

### Steps

**1. Create a PostgreSQL database**

In the Render dashboard: New → PostgreSQL. Copy the internal connection string.

**2. Create a Redis instance**

In the Render dashboard: New → Redis. Copy the internal connection string.

**3. Deploy the API (Web Service)**

- New → Web Service → connect your Git repo
- **Build command:** `go build -o bin/api ./cmd/api`
- **Start command:** `./bin/api`
- **Environment:** set all variables from the [Environment Variables](#environment-variables) table
- Set `COMU_ENV=production`

**4. Deploy the bot (Background Worker)**

- New → Background Worker → same repo
- **Build command:** `go build -o bin/bot ./cmd/bot`
- **Start command:** `./bin/bot`
- Set the same environment variables (no `PORT` needed)

**5. Sync data**

After services are live, trigger sync via the API:

```bash
curl -X POST https://your-app.onrender.com/sync \
  -H "X-Sync-Secret: your-sync-secret"
```

Or set `AUTO_SYNC=true` and let the API resync at 00:10 WIB every day automatically.

> **Note:** Render free tier services spin down after inactivity. Use a paid plan or set `auto_stop = false` for production workloads.

---

## Updating & Maintenance

### Redeploy after code changes

```bash
# VPS
make prod-up          # rebuilds images and restarts changed services

# Fly.io
flyctl deploy

# Render
git push origin main  # auto-deploys on push
```

### Run migrations after schema changes

```bash
# Local / VPS (inside container or with DATABASE_URL set)
make migrate

# Fly.io
flyctl ssh console -C "/app/migrate up"
```

### Force a data refresh

```bash
curl -X POST https://your-domain/sync \
  -H "X-Sync-Secret: your-sync-secret"
```

### Rotate `KAI_AUTH_TOKEN`

The KCI token expires periodically. To refresh:

1. Extract a new token from [kci.id](https://kci.id/perjalanan-krl/jadwal-kereta) (see [README](README.md#2-get-your-kai_auth_token))
2. Update it in `.env` (VPS) or as a secret (Fly.io / Render)
3. Restart the service — no rebuild needed
4. Trigger a manual sync to repopulate data
