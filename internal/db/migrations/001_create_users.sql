-- +goose Up
CREATE TABLE users (
                       id BIGSERIAL PRIMARY KEY,
                       telegram_id BIGINT UNIQUE NOT NULL,

    -- Настройки уведомлений
                       pushover_user_key TEXT,
                       pushover_enabled BOOLEAN NOT NULL DEFAULT false,
                       telegram_enabled BOOLEAN NOT NULL DEFAULT true,

    -- Timestamps
                       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                       updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индекс для быстрого поиска пользователей с включенным pushover
CREATE INDEX idx_users_pushover_enabled
    ON users (id)
    WHERE pushover_enabled = true AND pushover_user_key IS NOT NULL;

-- Комментарии для документации
COMMENT ON TABLE users IS 'Пользователи бота';
COMMENT ON COLUMN users.pushover_user_key IS 'Pushover user key для отправки уведомлений';
COMMENT ON COLUMN users.pushover_enabled IS 'Включены ли уведомления через Pushover';
COMMENT ON COLUMN users.telegram_enabled IS 'Включены ли уведомления через Telegram';

-- +goose Down
DROP TABLE users;