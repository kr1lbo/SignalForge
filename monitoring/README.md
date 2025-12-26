# SignalForge Monitoring

Prometheus + Grafana для мониторинга SignalForge.

## Быстрый старт

### 1. Запусти SignalForge
```bash
go run cmd/signalforge/main.go
```

Приложение экспортирует метрики на http://localhost:9090/metrics

### 2. Запусти Prometheus + Grafana
```bash
docker-compose -f docker-compose.monitoring.yml up -d
```

### 3. Открой Grafana
- URL: http://localhost:3000
- Login: `admin`
- Password: `admin`

### 4. Посмотри метрики

#### Prometheus UI:
- URL: http://localhost:9091
- Здесь можно делать запросы и смотреть графики

#### Примеры запросов (Prometheus Query):

**Общая статистика:**
```promql
# Сколько алертов сработало
signalforge_alerts_triggered_total

# Сколько уведомлений отправлено
signalforge_notifications_sent_total

# Сколько активных WebSocket соединений
signalforge_websocket_connected
```

**По биржам:**
```promql
# Алерты по биржам
sum by (exchange) (signalforge_alerts_triggered_total)

# События по биржам
rate(signalforge_price_events_total[5m])
```

**Проблемы:**
```promql
# Неудачные уведомления
signalforge_notifications_failed_total

# Реконнекты WebSocket
signalforge_websocket_reconnects_total
```

## Создание дашборда в Grafana

1. Зайди в Grafana (http://localhost:3000)
2. Нажми "+" → "Create Dashboard"
3. "Add visualization"
4. Выбери "Prometheus" data source
5. В Query напиши метрику, например:
   ```
   signalforge_alerts_triggered_total
   ```
6. Настрой график как хочешь
7. Сохрани дашборд

## Полезные дашборды

### Dashboard 1: Overview
- Всего пользователей (signalforge_users_total)
- Активных алертов (signalforge_alerts_active)
- WebSocket статус (signalforge_websocket_connected)
- Rate events/sec (rate(signalforge_price_events_total[1m]))

### Dashboard 2: Alerts
- Триггеров по биржам
- Триггеров по направлению (above/below)
- График создания/удаления алертов

### Dashboard 3: Notifications
- Отправлено/провалено по каналам
- Latency уведомлений
- Rate limit hits

## Остановка

```bash
docker-compose -f docker-compose.monitoring.yml down
```

Данные сохранятся в Docker volumes.

## Troubleshooting

**Prometheus не видит метрики:**
- Проверь что SignalForge запущен на порту 9090
- Проверь http://localhost:9090/metrics - метрики должны отдаваться
- В docker на Mac/Windows используй `host.docker.internal` вместо `localhost`

**Grafana не видит Prometheus:**
- Проверь что контейнеры в одной сети
- Проверь URL в datasources: `http://prometheus:9090`
