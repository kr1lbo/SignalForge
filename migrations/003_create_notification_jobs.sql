-- +goose Up
CREATE TYPE notification_channel AS ENUM ('telegram', 'pushover');
CREATE TYPE job_status AS ENUM ('pending', 'processing', 'done', 'failed');

CREATE TABLE notification_jobs (
                                   id BIGSERIAL PRIMARY KEY,
                                   alert_id BIGINT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
                                   user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Параметры уведомления
                                   channel notification_channel NOT NULL,
                                   status job_status NOT NULL DEFAULT 'pending',
                                   payload JSONB,

    -- Retry механизм
                                   attempts INT NOT NULL DEFAULT 0,
                                   run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                   last_error TEXT,

    -- Timestamps для мониторинга
                                   created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                   updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                   processing_started_at TIMESTAMPTZ,
                                   completed_at TIMESTAMPTZ
);

-- Индекс для выборки pending jobs
CREATE INDEX idx_jobs_pending
    ON notification_jobs (run_at)
    WHERE status = 'pending';

-- Индекс для мониторинга застрявших jobs
CREATE INDEX idx_jobs_stuck
    ON notification_jobs (processing_started_at)
    WHERE status = 'processing' AND processing_started_at IS NOT NULL;

-- Индекс для статистики по каналам
CREATE INDEX idx_jobs_stats
    ON notification_jobs (channel, status, created_at);

-- Индекс для failed jobs (для анализа проблем)
CREATE INDEX idx_jobs_failed
    ON notification_jobs (created_at DESC)
    WHERE status = 'failed';

-- Комментарии
COMMENT ON TABLE notification_jobs IS 'Очередь уведомлений (outbox pattern)';
COMMENT ON COLUMN notification_jobs.payload IS 'JSON с данными для отправки (текст сообщения, exchange, symbol, price)';
COMMENT ON COLUMN notification_jobs.attempts IS 'Количество попыток отправки';
COMMENT ON COLUMN notification_jobs.run_at IS 'Когда можно выполнить job (для retry с задержкой)';
COMMENT ON COLUMN notification_jobs.processing_started_at IS 'Когда начали обработку (для отслеживания зависших)';
COMMENT ON COLUMN notification_jobs.completed_at IS 'Когда job был завершён (done или failed)';

-- +goose Down
DROP TABLE notification_jobs;
DROP TYPE job_status;
DROP TYPE notification_channel;