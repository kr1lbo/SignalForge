# SignalForge

[Русская версия](README.ru.md)

SignalForge is a Go service for crypto futures price alerts. It connects to exchange WebSocket streams, tracks mark price updates, lets users manage alerts through a Telegram bot, and sends notifications when a target price is reached.

The current implementation is a monolithic MVP with three main services:

- `tgbot`: Telegram interface for users, alert CRUD and notification settings.
- `watcher`: WebSocket price watcher for exchanges and alert trigger logic.
- `notifier`: worker pool that sends queued notifications through Telegram and Pushover.

## What It Does

- Creates price alerts for futures trading pairs.
- Supports alert directions: `above` and `below`.
- Uses mark price from exchange futures feeds.
- Normalizes symbols like `BTC/USDT`, `btc_usdt`, `BTC-USDT` into `BTCUSDT`.
- Dynamically subscribes and unsubscribes from exchange WebSocket streams.
- Stores users, alerts and notification jobs in PostgreSQL.
- Uses an outbox pattern for reliable notification delivery.
- Uses Redis for rate limiting notification channels.
- Exposes Prometheus metrics and a health endpoint.

Supported exchanges:

- Gate.io: `gate`
- Bybit: `bybit`

Planned exchange:

- Binance: `binance`

## Tech Stack

- Language: Go `1.25.4`
- Database: PostgreSQL
- SQL generation: sqlc
- Database migrations: goose-compatible SQL migrations
- Redis: rate limiting for notification delivery
- WebSocket client: `github.com/gorilla/websocket`
- PostgreSQL driver: `github.com/jackc/pgx/v5`
- Telegram bot: `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- Metrics: Prometheus client for Go
- Config format: YAML
- Local infrastructure: Docker Compose
- Monitoring: Prometheus + Grafana

## Project Structure

```text
cmd/signalforge/               Application entry point
configs/                       Runtime configuration examples and local config
internal/app/                  Application wiring and lifecycle
internal/domain/               Domain types and interfaces
internal/infra/                PostgreSQL, Redis, exchanges, notification senders, metrics
internal/services/tgbot/       Telegram bot handlers and UI
internal/services/watcher/     Price stream subscriptions and alert triggering
internal/services/notifier/    Notification job workers
migrations/                    PostgreSQL migrations
sql/queries/                   sqlc query definitions
monitoring/                    Prometheus and Grafana configuration
```

The real application entry point is `cmd/signalforge/main.go`. The root `main.go` is currently empty.

## Requirements

- Go `1.25.4`
- Docker and Docker Compose
- PostgreSQL `16` or compatible
- Redis `7` or compatible
- Telegram bot token from BotFather
- Pushover API token
- Optional: goose CLI for applying migrations
- Optional: sqlc CLI for regenerating database code

## Configuration

Create a local config from the example:

```powershell
Copy-Item configs\config.example.yaml configs\config.yaml
```

For Bash:

```bash
cp configs/config.example.yaml configs/config.yaml
```

Then edit `configs/config.yaml` and set at least:

- `database.*`: PostgreSQL connection settings.
- `redis.*`: Redis connection settings.
- `telegram.bot_token`: Telegram bot token.
- `pushover.api_token`: Pushover application API token.
- `metrics.enabled` and `metrics.port`: metrics HTTP server settings.

Do not share or commit real Telegram or Pushover tokens. Keep public examples in `configs/config.example.yaml`.

## Local Startup

Start PostgreSQL:

```powershell
docker compose up -d postgres
```

Start Redis. Redis is required by the application but is not currently defined in `docker-compose.yml`:

```powershell
docker run -d --name signalforge-redis -p 6379:6379 redis:7
```

Install goose if it is not installed:

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Apply database migrations:

```powershell
goose -dir migrations postgres "host=localhost port=5432 user=signalforge password=signalforge dbname=signalforge sslmode=disable" up
```

Run the application:

```powershell
go run .\cmd\signalforge
```

The service will start:

- Telegram bot polling.
- Gate.io and Bybit WebSocket streams.
- Notification workers.
- Metrics server, if enabled.

Default local endpoints:

- Metrics: `http://localhost:9090/metrics`
- Health: `http://localhost:9090/health`

## Telegram Bot Usage

Available commands:

```text
/start
/help
/new <exchange> <symbol> <price> <direction> [notes]
/list
/delete <alert_id>
/settings
```

Examples:

```text
/new gate BTCUSDT 100000 above
/new bybit ETHUSDT 3000 below My ETH buy target
```

The bot also provides menu buttons:

- `My Alerts`: list active alerts.
- `New Alert`: show alert creation help.
- `Settings`: toggle Telegram and Pushover notifications, set Pushover user key.

## How Alerts Work

1. A user creates an alert in Telegram.
2. The alert is stored in PostgreSQL.
3. The watcher subscribes to the exchange and symbol if needed.
4. Exchange WebSocket streams emit mark price events.
5. The watcher compares the current price with active alerts.
6. When an alert triggers, the service marks it as fired and creates notification jobs in a single database transaction.
7. The notifier workers fetch pending jobs with `FOR UPDATE SKIP LOCKED`.
8. Notifications are sent through Telegram and/or Pushover, respecting Redis-backed rate limits.

## Database

Migrations create three main tables:

- `users`: Telegram users and notification preferences.
- `alerts`: active, fired and disabled price alerts.
- `notification_jobs`: outbox queue for notification delivery.

The generated sqlc code is stored in:

```text
internal/infra/db/postgres/sqlc
```

Regenerate sqlc code after changing migrations or queries:

```powershell
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

## Monitoring

SignalForge exposes Prometheus metrics on `/metrics` when `metrics.enabled` is `true`.

Start Prometheus and Grafana:

```powershell
docker compose -f docker-compose.monitoring.yml up -d
```

Open:

- Prometheus: `http://localhost:9091`
- Grafana: `http://localhost:3000`
- Grafana login: `admin`
- Grafana password: `admin`

The Prometheus config scrapes SignalForge through `host.docker.internal:9090`, which is suitable when the Go app runs on the host machine and Prometheus runs in Docker.

Useful metrics:

```text
signalforge_alerts_triggered_total
signalforge_alerts_created_total
signalforge_notifications_sent_total
signalforge_notifications_failed_total
signalforge_websocket_connected
signalforge_websocket_reconnects_total
signalforge_price_events_total
signalforge_jobs_processed_total
```

More monitoring details are in `monitoring/README.md`.

## Build

Build a local binary:

```powershell
go build -o .\bin\signalforge.exe .\cmd\signalforge
```

Run it:

```powershell
.\bin\signalforge.exe
```

For Linux or macOS:

```bash
go build -o ./bin/signalforge ./cmd/signalforge
./bin/signalforge
```

## Testing And Checks

Run all Go tests and compile checks:

```powershell
go test ./...
```

At the moment the repository is mostly service code and generated database access code, so this command is also useful as a full project compile check.

## Development Notes

- Add new SQL in `sql/queries/queries.sql`, then run `sqlc generate`.
- Add schema changes as new files in `migrations/`.
- Keep exchange-specific symbol formatting in `internal/infra/symbol`.
- Add new exchanges by implementing `internal/domain/exchange.Stream`.
- Keep notification delivery asynchronous through `notification_jobs`.
- Use `configs/config.example.yaml` as the documented config shape.

## Troubleshooting

If startup fails with `telegram.bot_token is required` or `pushover.api_token is required`, fill those values in `configs/config.yaml`.

If startup fails on Redis ping, make sure Redis is running on the configured host and port.

If PostgreSQL starts but tables are missing, apply migrations with goose before running the app.

If Prometheus does not see metrics, check `http://localhost:9090/metrics` first, then verify `monitoring/prometheus.yml`.
