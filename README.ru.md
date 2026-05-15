# SignalForge

[English version](README.md)

SignalForge - Go-сервис для ценовых алертов по криптовалютным фьючерсам. Он подключается к WebSocket-потокам бирж, отслеживает mark price, позволяет пользователям управлять алертами через Telegram-бота и отправляет уведомления, когда цена достигает заданного уровня.

Текущая реализация - монолитный MVP с тремя основными сервисами:

- `tgbot`: Telegram-интерфейс для пользователей, CRUD алертов и настройки уведомлений.
- `watcher`: WebSocket-наблюдатель за ценами и логика срабатывания алертов.
- `notifier`: пул воркеров, который отправляет уведомления из очереди через Telegram и Pushover.

## Что Делает Проект

- Создает ценовые алерты для фьючерсных торговых пар.
- Поддерживает направления алертов: `above` и `below`.
- Использует mark price из фьючерсных потоков бирж.
- Нормализует символы вроде `BTC/USDT`, `btc_usdt`, `BTC-USDT` в `BTCUSDT`.
- Динамически подписывается и отписывается от WebSocket-потоков бирж.
- Хранит пользователей, алерты и задачи уведомлений в PostgreSQL.
- Использует outbox pattern для надежной доставки уведомлений.
- Использует Redis для rate limiting каналов уведомлений.
- Отдает Prometheus-метрики и health endpoint.

Поддерживаемые биржи:

- Gate.io: `gate`
- Bybit: `bybit`

Запланированная биржа:

- Binance: `binance`

## Технологии

- Язык: Go `1.25.4`
- База данных: PostgreSQL
- Генерация SQL-кода: sqlc
- Миграции БД: SQL-миграции, совместимые с goose
- Redis: rate limiting отправки уведомлений
- WebSocket-клиент: `github.com/gorilla/websocket`
- PostgreSQL-драйвер: `github.com/jackc/pgx/v5`
- Telegram-бот: `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- Метрики: Prometheus client для Go
- Конфигурация: YAML
- Локальная инфраструктура: Docker Compose
- Мониторинг: Prometheus + Grafana

## Структура Проекта

```text
cmd/signalforge/               Точка входа приложения
configs/                       Примеры и локальная конфигурация
internal/app/                  Сборка приложения и lifecycle
internal/domain/               Доменные типы и интерфейсы
internal/infra/                PostgreSQL, Redis, биржи, отправители уведомлений, метрики
internal/services/tgbot/       Telegram-бот, обработчики и UI
internal/services/watcher/     Подписки на цены и срабатывание алертов
internal/services/notifier/    Воркеры задач уведомлений
migrations/                    PostgreSQL-миграции
sql/queries/                   SQL-запросы для sqlc
monitoring/                    Конфигурация Prometheus и Grafana
```

Реальная точка входа приложения находится в `cmd/signalforge/main.go`. Корневой `main.go` сейчас пустой.

## Требования

- Go `1.25.4`
- Docker и Docker Compose
- PostgreSQL `16` или совместимая версия
- Redis `7` или совместимая версия
- Telegram bot token от BotFather
- Pushover API token
- Опционально: goose CLI для применения миграций
- Опционально: sqlc CLI для регенерации database-кода

## Конфигурация

Создайте локальный конфиг из примера:

```powershell
Copy-Item configs\config.example.yaml configs\config.yaml
```

Для Bash:

```bash
cp configs/config.example.yaml configs/config.yaml
```

Затем отредактируйте `configs/config.yaml` и задайте минимум:

- `database.*`: параметры подключения к PostgreSQL.
- `redis.*`: параметры подключения к Redis.
- `telegram.bot_token`: токен Telegram-бота.
- `pushover.api_token`: API token приложения Pushover.
- `metrics.enabled` и `metrics.port`: настройки HTTP-сервера метрик.

Не публикуйте и не коммитьте реальные Telegram и Pushover токены. Для публичного примера используйте `configs/config.example.yaml`.

## Локальный Запуск

Запустите PostgreSQL:

```powershell
docker compose up -d postgres
```

Запустите Redis. Redis требуется приложению, но сейчас не описан в `docker-compose.yml`:

```powershell
docker run -d --name signalforge-redis -p 6379:6379 redis:7
```

Установите goose, если он еще не установлен:

```powershell
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Примените миграции базы данных:

```powershell
goose -dir migrations postgres "host=localhost port=5432 user=signalforge password=signalforge dbname=signalforge sslmode=disable" up
```

Запустите приложение:

```powershell
go run .\cmd\signalforge
```

После запуска будут работать:

- polling Telegram-бота.
- WebSocket-потоки Gate.io и Bybit.
- воркеры отправки уведомлений.
- сервер метрик, если он включен.

Локальные endpoint-ы по умолчанию:

- Метрики: `http://localhost:9090/metrics`
- Health: `http://localhost:9090/health`

## Использование Telegram-Бота

Доступные команды:

```text
/start
/help
/new <exchange> <symbol> <price> <direction> [notes]
/list
/delete <alert_id>
/settings
```

Примеры:

```text
/new gate BTCUSDT 100000 above
/new bybit ETHUSDT 3000 below My ETH buy target
```

Также у бота есть кнопки меню:

- `My Alerts`: список активных алертов.
- `New Alert`: подсказка по созданию алерта.
- `Settings`: включение Telegram и Pushover уведомлений, настройка Pushover user key.

## Как Работают Алерты

1. Пользователь создает алерт в Telegram.
2. Алерт сохраняется в PostgreSQL.
3. Watcher подписывается на нужную биржу и символ, если подписки еще нет.
4. WebSocket-потоки бирж отправляют события с mark price.
5. Watcher сравнивает текущую цену с активными алертами.
6. При срабатывании сервис в одной транзакции помечает алерт как fired и создает notification jobs.
7. Notifier-воркеры забирают pending jobs через `FOR UPDATE SKIP LOCKED`.
8. Уведомления отправляются через Telegram и/или Pushover с учетом Redis-backed rate limits.

## База Данных

Миграции создают три основные таблицы:

- `users`: пользователи Telegram и настройки уведомлений.
- `alerts`: активные, сработавшие и отключенные ценовые алерты.
- `notification_jobs`: outbox-очередь для доставки уведомлений.

Сгенерированный код sqlc лежит здесь:

```text
internal/infra/db/postgres/sqlc
```

После изменения миграций или запросов регенерируйте sqlc-код:

```powershell
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

## Мониторинг

SignalForge отдает Prometheus-метрики на `/metrics`, если `metrics.enabled` равен `true`.

Запустите Prometheus и Grafana:

```powershell
docker compose -f docker-compose.monitoring.yml up -d
```

Откройте:

- Prometheus: `http://localhost:9091`
- Grafana: `http://localhost:3000`
- Логин Grafana: `admin`
- Пароль Grafana: `admin`

Prometheus настроен на сбор метрик с `host.docker.internal:9090`. Это подходит для сценария, где Go-приложение запущено на хосте, а Prometheus работает в Docker.

Полезные метрики:

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

Больше деталей по мониторингу есть в `monitoring/README.md`.

## Сборка

Собрать локальный бинарник:

```powershell
go build -o .\bin\signalforge.exe .\cmd\signalforge
```

Запустить:

```powershell
.\bin\signalforge.exe
```

Для Linux или macOS:

```bash
go build -o ./bin/signalforge ./cmd/signalforge
./bin/signalforge
```

## Тесты И Проверки

Запустить все Go-тесты и compile check:

```powershell
go test ./...
```

Сейчас в репозитории в основном сервисный код и сгенерированный database access code, поэтому эта команда также полезна как полная проверка компиляции проекта.

## Заметки Для Разработки

- Новый SQL добавляйте в `sql/queries/queries.sql`, затем запускайте `sqlc generate`.
- Изменения схемы добавляйте новыми файлами в `migrations/`.
- Форматирование символов под конкретные биржи держите в `internal/infra/symbol`.
- Новые биржи добавляйте через реализацию `internal/domain/exchange.Stream`.
- Доставку уведомлений сохраняйте асинхронной через `notification_jobs`.
- `configs/config.example.yaml` используйте как документированную форму конфига.

## Troubleshooting

Если запуск падает с `telegram.bot_token is required` или `pushover.api_token is required`, заполните эти значения в `configs/config.yaml`.

Если запуск падает на Redis ping, убедитесь, что Redis запущен на указанном host и port.

Если PostgreSQL запущен, но таблиц нет, примените миграции через goose перед запуском приложения.

Если Prometheus не видит метрики, сначала проверьте `http://localhost:9090/metrics`, затем проверьте `monitoring/prometheus.yml`.
