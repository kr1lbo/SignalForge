-- +goose Up
CREATE TYPE alert_direction AS ENUM ('above', 'below');

CREATE TABLE alerts (
                        id BIGSERIAL PRIMARY KEY,
                        user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Параметры алерта
                        exchange TEXT NOT NULL,
                        symbol TEXT NOT NULL,
                        price NUMERIC(20,8) NOT NULL,
                        direction alert_direction NOT NULL,

    -- Дополнительная информация
                        notes TEXT,
                        is_active BOOLEAN NOT NULL DEFAULT true,

    -- Состояние
                        fired_at TIMESTAMPTZ,

    -- Timestamps
                        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                        updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индекс для быстрого поиска активных алертов по бирже и символу
CREATE INDEX idx_alerts_active_lookup
    ON alerts (exchange, symbol, is_active)
    WHERE fired_at IS NULL AND is_active = true;

-- Индекс для быстрого поиска алертов пользователя
CREATE INDEX idx_alerts_user_active
    ON alerts (user_id, created_at DESC)
    WHERE fired_at IS NULL AND is_active = true;

-- Индекс для сработавших алертов (для истории)
CREATE INDEX idx_alerts_fired
    ON alerts (user_id, fired_at DESC)
    WHERE fired_at IS NOT NULL;

-- Комментарии
COMMENT ON TABLE alerts IS 'Алерты пользователей на достижение цены';
COMMENT ON COLUMN alerts.exchange IS 'Биржа (gate, bybit, binance)';
COMMENT ON COLUMN alerts.symbol IS 'Торговая пара в нормализованном виде (BTCUSDT)';
COMMENT ON COLUMN alerts.notes IS 'Заметка пользователя к алерту';
COMMENT ON COLUMN alerts.is_active IS 'Активен ли алерт (можно отключить без удаления)';
COMMENT ON COLUMN alerts.fired_at IS 'Когда сработал алерт (NULL = ещё не сработал)';

-- +goose Down
DROP TABLE alerts;
DROP TYPE alert_direction;