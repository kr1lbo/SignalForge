package postgres

import (
	"SignalForge/internal/domain/repository"
	"SignalForge/internal/infra/db/postgres/sqlc"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobRepository implements repository.NotificationJobRepository
type JobRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewJobRepository creates a new JobRepository
func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Insert creates a new notification job
func (r *JobRepository) Insert(ctx context.Context, job *repository.NotificationJob) error {
	params := sqlc.InsertNotificationJobParams{
		AlertID: job.AlertID,
		UserID:  job.UserID,
		Channel: job.Channel,
		Payload: job.Payload,
	}

	if err := r.queries.InsertNotificationJob(ctx, params); err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	return nil
}

// FetchPending retrieves pending jobs ready to run (with row locking)
func (r *JobRepository) FetchPending(ctx context.Context, limit int) ([]*repository.NotificationJob, error) {
	dbJobs, err := r.queries.FetchPendingJobs(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("fetch pending: %w", err)
	}

	jobs := make([]*repository.NotificationJob, len(dbJobs))
	for i, dbJob := range dbJobs {
		jobs[i] = mapJob(dbJob)
	}

	return jobs, nil
}

// MarkProcessing marks a job as being processed
func (r *JobRepository) MarkProcessing(ctx context.Context, jobID int64) error {
	if err := r.queries.MarkJobProcessing(ctx, jobID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	return nil
}

// MarkDone marks a job as successfully completed
func (r *JobRepository) MarkDone(ctx context.Context, jobID int64) error {
	if err := r.queries.MarkJobDone(ctx, jobID); err != nil {
		return fmt.Errorf("mark done: %w", err)
	}

	return nil
}

// Retry schedules a job for retry with exponential backoff
func (r *JobRepository) Retry(ctx context.Context, jobID int64, delaySeconds int, errMsg string) error {
	params := sqlc.RetryJobParams{
		ID:        jobID,
		Column2:   pgtype.Text{String: fmt.Sprintf("%d", delaySeconds), Valid: true},
		LastError: pgtype.Text{String: errMsg, Valid: true},
	}

	if err := r.queries.RetryJob(ctx, params); err != nil {
		return fmt.Errorf("retry job: %w", err)
	}

	return nil
}

// Fail marks a job as permanently failed
func (r *JobRepository) Fail(ctx context.Context, jobID int64, errMsg string) error {
	params := sqlc.FailJobParams{
		ID:        jobID,
		LastError: pgtype.Text{String: errMsg, Valid: true},
	}

	if err := r.queries.FailJob(ctx, params); err != nil {
		return fmt.Errorf("fail job: %w", err)
	}

	return nil
}

// GetStuckJobs retrieves jobs stuck in processing state
func (r *JobRepository) GetStuckJobs(ctx context.Context) ([]*repository.NotificationJob, error) {
	dbJobs, err := r.queries.GetStuckJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get stuck jobs: %w", err)
	}

	jobs := make([]*repository.NotificationJob, len(dbJobs))
	for i, dbJob := range dbJobs {
		jobs[i] = mapJob(dbJob)
	}

	return jobs, nil
}

// ResetStuckJobs resets stuck jobs back to pending
func (r *JobRepository) ResetStuckJobs(ctx context.Context) error {
	if err := r.queries.ResetStuckJobs(ctx); err != nil {
		return fmt.Errorf("reset stuck jobs: %w", err)
	}

	return nil
}

// CleanupOld deletes old completed/failed jobs
func (r *JobRepository) CleanupOld(ctx context.Context) error {
	if err := r.queries.CleanupOldJobs(ctx); err != nil {
		return fmt.Errorf("cleanup old jobs: %w", err)
	}

	return nil
}

// mapJob converts sqlc.NotificationJob to repository.NotificationJob
func mapJob(dbJob sqlc.NotificationJob) *repository.NotificationJob {
	var processingStartedAt *time.Time
	if dbJob.ProcessingStartedAt.Valid {
		processingStartedAt = &dbJob.ProcessingStartedAt.Time
	}

	var completedAt *time.Time
	if dbJob.CompletedAt.Valid {
		completedAt = &dbJob.CompletedAt.Time
	}

	var lastError *string
	if dbJob.LastError.Valid {
		lastError = &dbJob.LastError.String
	}

	return &repository.NotificationJob{
		ID:                  dbJob.ID,
		AlertID:             dbJob.AlertID,
		UserID:              dbJob.UserID,
		Channel:             dbJob.Channel,
		Status:              repository.JobStatus(dbJob.Status),
		Payload:             dbJob.Payload,
		Attempts:            int(dbJob.Attempts),
		RunAt:               dbJob.RunAt.Time,
		LastError:           lastError,
		CreatedAt:           dbJob.CreatedAt.Time,
		UpdatedAt:           dbJob.UpdatedAt.Time,
		ProcessingStartedAt: processingStartedAt,
		CompletedAt:         completedAt,
	}
}
