package repository

import (
	"context"
	"time"
)

// JobStatus represents the status of a notification job
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusDone       JobStatus = "done"
	JobStatusFailed     JobStatus = "failed"
)

// NotificationJob represents a job in the outbox queue
type NotificationJob struct {
	ID                  int64
	AlertID             int64
	UserID              int64
	Channel             string
	Status              JobStatus
	Payload             []byte // JSON
	Attempts            int
	RunAt               time.Time
	LastError           *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ProcessingStartedAt *time.Time
	CompletedAt         *time.Time
}

// NotificationJobRepository handles notification job persistence
type NotificationJobRepository interface {
	// Insert creates a new notification job
	Insert(ctx context.Context, job *NotificationJob) error

	// FetchPending retrieves pending jobs ready to run (with row locking)
	FetchPending(ctx context.Context, limit int) ([]*NotificationJob, error)

	// MarkProcessing marks a job as being processed
	MarkProcessing(ctx context.Context, jobID int64) error

	// MarkDone marks a job as successfully completed
	MarkDone(ctx context.Context, jobID int64) error

	// Retry schedules a job for retry with exponential backoff
	Retry(ctx context.Context, jobID int64, delaySeconds int, errMsg string) error

	// Fail marks a job as permanently failed
	Fail(ctx context.Context, jobID int64, errMsg string) error

	// GetStuckJobs retrieves jobs stuck in processing state
	GetStuckJobs(ctx context.Context) ([]*NotificationJob, error)

	// ResetStuckJobs resets stuck jobs back to pending
	ResetStuckJobs(ctx context.Context) error

	// CleanupOld deletes old completed/failed jobs
	CleanupOld(ctx context.Context) error
}
