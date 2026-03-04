# TODO — Production Readiness

---

## 🔴 Critical

### 1. Env validation on startup
**File:** `internal/config/config.go`

Fail fast if required env vars are missing. Right now the app starts silently with empty strings and only fails later (e.g. DB connection error, nil token in headers).

- Add a `Validate()` method on `Config` that checks `DATABASE_URL`, `REDIS_URL`, and `KAI_AUTH_TOKEN` are non-empty
- Call `cfg.Validate()` in `cmd/api/main.go` and `cmd/sync/main.go` immediately after `config.Load()`
- Return a clear error message listing all missing vars at once, not one by one

---

### 2. Database connection pool
**File:** `internal/database/db.go`

GORM uses `database/sql` under the hood. Without pool settings, under load the app will exhaust connections or create too many.

- After `gorm.Open(...)`, get the underlying `*sql.DB` via `db.DB()`
- Set:
  - `SetMaxOpenConns(25)` — max simultaneous connections
  - `SetMaxIdleConns(10)` — connections to keep alive in the pool
  - `SetConnMaxLifetime(5 * time.Minute)` — recycle connections to avoid stale ones
- Tune values based on your PostgreSQL `max_connections` setting

---

### 3. Replace AutoMigrate with proper migrations
**File:** `internal/database/db.go`, new `migrations/` directory

`AutoMigrate` is fine for development but dangerous in production — it can lock tables, silently skip changes, and can't handle column renames or drops.

- Add `github.com/golang-migrate/migrate/v4` to `go.mod`
- Create a `migrations/` directory with numbered SQL files:
  - `000001_create_stations.up.sql` / `000001_create_stations.down.sql`
  - `000002_create_schedules.up.sql` / `000002_create_schedules.down.sql`
- Replace `db.AutoMigrate(...)` with `migrate.Up()` using the embedded SQL files
- Add a `make migrate` and `make migrate-down` target to the Makefile
- Remove `AutoMigrate` call entirely in production mode

---

### 4. Authenticate the `/sync` endpoint
**File:** `cmd/api/main.go`, `internal/config/config.go`

`POST /sync` is currently open to anyone. A bad actor could spam it and overload the KRL API or the database.

- Add `SYNC_SECRET` to `.env` and `Config` struct
- Create a middleware `RequireSyncSecret()` in `internal/middleware/` that checks the `X-Sync-Secret` request header against `cfg.SyncSecret`
- Return `401 Unauthorized` if the header is missing or wrong
- Apply the middleware to the `POST /sync` route only
- Document the header in the README

---

### 5. Request timeout on DB queries
**File:** `internal/handlers/station.go`, `schedule.go`, `route.go`

Handlers currently pass `context.Background()` to the cache and use Gin's context without a deadline. A slow DB query holds the goroutine and connection indefinitely.

- In each handler, derive a context with timeout from the request context:
  ```go
  ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
  defer cancel()
  ```
- Pass `ctx` to all `db.WithContext(ctx).Find(...)` calls and cache operations
- Return `503 Service Unavailable` if the context deadline is exceeded

---

## 🟡 Stability

### 6. Retry logic for KRL API calls
**File:** `internal/sync/station.go`, `internal/sync/schedule.go`

The KRL API is unreliable. A single transient failure currently causes the entire station or schedule sync to fail silently.

- Add a `fetchWithRetry(req *http.Request, maxRetries int) (*http.Response, error)` helper in `internal/sync/`
- Retry up to 3 times on non-2xx responses or network errors
- Use exponential backoff: wait 1s, 2s, 4s between attempts
- Log each retry attempt with the attempt number and error
- Do not retry on 401 (token expired) — log a clear alert and abort instead

---

### 7. Invalidate cache after sync
**File:** `internal/scheduler/scheduler.go` or a new `internal/cache/cache.go` method

After sync completes, Redis still serves the old cached data until midnight. Users who hit the API right after a sync get stale data.

- Add a `Flush()` or `InvalidateAll()` method to `internal/cache/cache.go` that deletes keys matching `station:*`, `schedule:*`, and `route:*` using `SCAN` + `DEL` (avoid `FLUSHDB` which nukes everything)
- Call `cache.InvalidateAll()` at the end of `scheduler.run()` and `scheduler.RunNow()`
- Log how many keys were invalidated

---

### 8. Real health check
**File:** `cmd/api/main.go`

`GET /status` always returns `{"status":"ok"}` even if the DB or Redis is down.

- Ping the DB: `db.Raw("SELECT 1").Error`
- Ping Redis: `redis.Client.Ping(ctx)`
- If either fails, return `503 Service Unavailable` with a body showing which dependency is down:
  ```json
  { "status": "degraded", "postgres": "ok", "redis": "error: connection refused" }
  ```
- Keep response time under 1s — add a 500ms timeout to the health check pings

---

### 9. Detect KRL API token expiry
**File:** `internal/sync/station.go`, `internal/sync/schedule.go`

When the token expires (JWT `exp` claim), the KRL API returns a 401. Currently this is caught as a JSON parse error (HTML response), logged as a generic error, and sync silently writes nothing.

- After `client.Do(req)`, check `resp.StatusCode`
- If `401`, log a prominent warning: `"KAI_AUTH_TOKEN has expired — update the token and re-run sync"` and return a sentinel error
- In `scheduler.run()`, check for this sentinel error and skip the schedule sync entirely (no point trying all 114 stations)

---

### 10. Schedule ID collision handling
**File:** `internal/sync/schedule.go`

The schedule primary key is `{station_id}-{train_id}`. If the KRL API returns two entries for the same train at the same station (can happen for loop lines), the second upsert silently overwrites the first.

- Change the ID to include the departure time: `{station_id}-{train_id}-{HHmm}` (e.g. `MRI-1234-0530`)
- Update the `OnConflict` columns to match the new PK
- This makes each schedule entry uniquely identifiable

---

## 🟢 Observability & Ops

### 11. Structured logging
**Files:** all files using `log.Printf`

The standard `log` package outputs plain text with no levels, no JSON, and no context. Hard to filter in production log aggregators (Loki, Datadog, etc.).

- Replace all `log.Printf` / `log.Println` calls with `slog` (built into Go 1.21+)
- In `config.go`, set the default handler based on env:
  - `development`: `slog.NewTextHandler` (human-readable)
  - `production`: `slog.NewJSONHandler` (structured JSON)
- Log levels: use `slog.Info` for normal events, `slog.Warn` for recoverable issues, `slog.Error` for failures

---

### 12. Prometheus metrics
**File:** new `internal/middleware/metrics.go`, `cmd/api/main.go`

No visibility into request rates, latency, error rates, or cache behaviour.

- Add `github.com/prometheus/client_golang` to `go.mod`
- Instrument:
  - HTTP request count and latency by route and status code (use `promhttp` middleware)
  - Cache hit/miss counter (increment in `internal/cache/cache.go`)
  - Sync duration and success/failure counter (instrument `scheduler.run()`)
- Expose `GET /metrics` endpoint using `promhttp.Handler()`
- Add the metrics port to `docker-compose.prod.yml` and optionally wire up a Grafana + Prometheus stack

---

### 13. Request ID middleware
**File:** `internal/middleware/requestid.go`

Without a request ID, correlating a user-reported error with a specific log line is very hard.

- Create a middleware that reads `X-Request-ID` from the incoming request (set by a load balancer) or generates a new UUID if absent
- Set it on the response header as `X-Request-ID`
- Store it in the Gin context: `c.Set("request_id", id)`
- Include it in all log lines related to that request

---

### 14. Suppress GORM SQL logs in production
**File:** `internal/database/db.go`

GORM currently logs every SQL statement. In production this creates enormous log volume and exposes query internals.

- Change the GORM logger based on `cfg.Env`:
  - `development`: `logger.Info` (current)
  - `production`: `logger.Warn` (only slow queries and errors)
- Optionally set `SlowThreshold: 500 * time.Millisecond` so slow queries are flagged

---

### 15. HTTPS / reverse proxy
**File:** `docker-compose.prod.yml`

The app currently serves plain HTTP. Credentials and data are sent unencrypted.

- Add an Nginx or Caddy service to `docker-compose.prod.yml` as a reverse proxy
- Caddy is the easiest option — it auto-provisions Let's Encrypt TLS certificates with a one-line `Caddyfile`
- Expose only ports 80 and 443 publicly; keep `:8080` internal to the Docker network

---

## 🔵 Nice to Have

### 16. Pagination on list endpoints
**Files:** `internal/handlers/station.go`, `internal/handlers/schedule.go`

`GET /v1/schedule/:station_id` can return 500+ rows in a single response. No way to paginate.

- Add optional `?page=1&limit=50` query params to `/v1/station` and `/v1/schedule/:station_id`
- Default `limit` to 100, cap at 500
- Return pagination metadata in the response envelope:
  ```json
  { "metadata": { "success": true, "page": 1, "limit": 50, "total": 578 }, "data": [...] }
  ```
- Update cache keys to include pagination params: `schedule:MRI:page1:limit50`

---

### 17. Resource limits in docker-compose.prod.yml
**File:** `docker-compose.prod.yml`

Without resource limits, one service (e.g. a runaway sync) can starve the others.

- Add `deploy.resources.limits` to each service:
  - `app`: `memory: 256m`, `cpus: "0.5"`
  - `postgres`: `memory: 512m`, `cpus: "1.0"`
  - `redis`: `memory: 128m`, `cpus: "0.25"`
- Also set `ulimits.nofile` for postgres to handle many connections

---

### 18. Add `.dockerignore`
**File:** new `.dockerignore` at project root

Without it, the Docker build context uploads everything — including `.env` (which contains the KAI token), `bin/`, `.git/`, etc.

- Create `.dockerignore` with at minimum:
  ```
  .env
  bin/
  .git/
  *.md
  docs/swagger.json
  ```

---

### 19. Docker secrets / secrets management
**File:** `docker-compose.prod.yml`, `.env`

Storing `KAI_AUTH_TOKEN` in a `.env` file on disk is a security risk — it can be accidentally committed, leaked in logs, or read by any process on the host.

- For Docker Compose: use `secrets:` block to mount the token as a file, read it in the app via `os.ReadFile`
- For cloud deployments: use the platform's secret manager (AWS Secrets Manager, GCP Secret Manager, etc.) and inject at runtime
- At minimum: ensure `.env` is in `.gitignore` and never committed

---
