-- ============================================================================
-- USER QUERIES
-- ============================================================================

-- name: GetOrCreateUser :one
INSERT INTO users (telegram_id)
VALUES ($1)
ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = EXCLUDED.telegram_id
RETURNING *;

-- name: GetUserByTelegramID :one
SELECT * FROM users
WHERE telegram_id = $1;

-- name: UpdateUserSettings :exec
UPDATE users
SET pushover_user_key = $2,
    pushover_enabled = $3,
    telegram_enabled = $4,
    updated_at = now()
WHERE id = $1;

-- ============================================================================
-- ALERT QUERIES
-- ============================================================================

-- name: CreateAlert :one
INSERT INTO alerts (user_id, exchange, symbol, price, direction, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListActiveAlerts :many
SELECT * FROM alerts
WHERE user_id = $1
  AND fired_at IS NULL
  AND is_active = true
ORDER BY created_at DESC;

-- name: ListAllUserAlerts :many
SELECT * FROM alerts
WHERE user_id = $1
ORDER BY
    CASE WHEN fired_at IS NULL THEN 0 ELSE 1 END,
    created_at DESC;

-- name: GetAlert :one
SELECT * FROM alerts
WHERE id = $1 AND user_id = $2;

-- name: DeleteAlert :exec
DELETE FROM alerts
WHERE id = $1 AND user_id = $2 AND fired_at IS NULL;

-- name: DeactivateAlert :exec
UPDATE alerts
SET is_active = false, updated_at = now()
WHERE id = $1 AND user_id = $2 AND fired_at IS NULL;

-- name: ActivateAlert :exec
UPDATE alerts
SET is_active = true, updated_at = now()
WHERE id = $1 AND user_id = $2 AND fired_at IS NULL;

-- name: FetchActiveAlertsByKey :many
SELECT
    a.id,
    a.user_id,
    a.exchange,
    a.symbol,
    a.price,
    a.direction,
    a.notes,
    a.created_at,
    u.telegram_id,
    u.pushover_user_key,
    u.pushover_enabled,
    u.telegram_enabled
FROM alerts a
         JOIN users u ON a.user_id = u.id
WHERE a.exchange = $1
  AND a.symbol = $2
  AND a.fired_at IS NULL
  AND a.is_active = true;

-- name: MarkAlertFired :exec
UPDATE alerts
SET fired_at = now(), updated_at = now()
WHERE id = $1 AND fired_at IS NULL;

-- name: CountActiveAlerts :one
SELECT COUNT(*) FROM alerts
WHERE user_id = $1
  AND fired_at IS NULL
  AND is_active = true;

-- name: GetUniqueActiveSubscriptions :many
SELECT DISTINCT exchange, symbol
FROM alerts
WHERE fired_at IS NULL
  AND is_active = true;

-- ============================================================================
-- NOTIFICATION JOB QUERIES
-- ============================================================================

-- name: InsertNotificationJob :exec
INSERT INTO notification_jobs (alert_id, user_id, channel, payload)
VALUES ($1, $2, $3, $4);

-- name: FetchPendingJobs :many
SELECT *
FROM notification_jobs
WHERE status = 'pending'
  AND run_at <= now()
ORDER BY run_at
LIMIT $1
    FOR UPDATE SKIP LOCKED;

-- name: MarkJobProcessing :exec
UPDATE notification_jobs
SET status = 'processing',
    processing_started_at = now(),
    updated_at = now()
WHERE id = $1;

-- name: MarkJobDone :exec
UPDATE notification_jobs
SET status = 'done',
    completed_at = now(),
    updated_at = now()
WHERE id = $1;

-- name: RetryJob :exec
UPDATE notification_jobs
SET status = 'pending',
    attempts = attempts + 1,
    run_at = now() + ($2 || ' seconds')::interval,
    last_error = $3,
    updated_at = now()
WHERE id = $1;

-- name: FailJob :exec
UPDATE notification_jobs
SET status = 'failed',
    last_error = $2,
    completed_at = now(),
    updated_at = now()
WHERE id = $1;

-- name: GetJobsByAlert :many
SELECT * FROM notification_jobs
WHERE alert_id = $1
ORDER BY created_at DESC;

-- name: CleanupOldJobs :exec
DELETE FROM notification_jobs
WHERE status IN ('done', 'failed')
  AND completed_at < now() - interval '7 days';

-- name: GetStuckJobs :many
SELECT * FROM notification_jobs
WHERE status = 'processing'
  AND processing_started_at < now() - interval '5 minutes';

-- name: ResetStuckJobs :exec
UPDATE notification_jobs
SET status = 'pending',
    processing_started_at = NULL,
    run_at = now(),
    updated_at = now()
WHERE status = 'processing'
  AND processing_started_at < now() - interval '5 minutes';

-- ============================================================================
-- STATISTICS QUERIES (полезно для мониторинга)
-- ============================================================================

-- name: GetJobStatsByChannel :many
SELECT
    channel,
    status,
    COUNT(*) as count,
    AVG(attempts) as avg_attempts
FROM notification_jobs
WHERE created_at > now() - interval '24 hours'
GROUP BY channel, status;

-- name: GetAlertStats :one
SELECT
            COUNT(*) FILTER (WHERE fired_at IS NULL AND is_active = true) as active_count,
            COUNT(*) FILTER (WHERE fired_at IS NOT NULL) as fired_count,
            COUNT(*) FILTER (WHERE is_active = false) as deactivated_count
FROM alerts
WHERE user_id = $1;