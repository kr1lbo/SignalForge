package notifier

import (
	"SignalForge/internal/domain/notify"
	"SignalForge/internal/domain/ratelimit"
	"SignalForge/internal/domain/repository"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Service processes notification jobs from the outbox queue
type Service struct {
	logger *slog.Logger

	// Dependencies
	jobRepo    repository.NotificationJobRepository
	senders    map[notify.Channel]notify.Sender
	rateLimits map[notify.Channel]ratelimit.Limiter

	// Config
	workers      int
	pollInterval time.Duration
	maxRetries   int

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new notifier service
func New(
	logger *slog.Logger,
	jobRepo repository.NotificationJobRepository,
	senders map[notify.Channel]notify.Sender,
	rateLimits map[notify.Channel]ratelimit.Limiter,
	workers int,
	pollInterval time.Duration,
	maxRetries int,
) *Service {
	return &Service{
		logger:       logger,
		jobRepo:      jobRepo,
		senders:      senders,
		rateLimits:   rateLimits,
		workers:      workers,
		pollInterval: pollInterval,
		maxRetries:   maxRetries,
	}
}

// Start begins processing notification jobs
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("notifier starting", "workers", s.workers)

	// Start worker pool
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}

	// Start stuck job recovery
	s.wg.Add(1)
	go s.stuckJobRecovery()

	s.logger.Info("notifier started")
	return nil
}

// Stop gracefully stops the notifier
func (s *Service) Stop() error {
	s.logger.Info("notifier stopping")

	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
	s.logger.Info("notifier stopped")
	return nil
}

func (s *Service) worker(id int) {
	defer s.wg.Done()

	logger := s.logger.With("worker", id)
	logger.Info("worker started")

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("worker stopped")
			return
		case <-ticker.C:
			s.processBatch(logger)
		}
	}
}

func (s *Service) processBatch(logger *slog.Logger) {
	jobs, err := s.jobRepo.FetchPending(s.ctx, 10)
	if err != nil {
		logger.Error("failed to fetch pending jobs", "error", err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	logger.Debug("processing jobs", "count", len(jobs))

	for _, job := range jobs {
		if err := s.processJob(logger, job); err != nil {
			logger.Error("failed to process job",
				"job_id", job.ID,
				"channel", job.Channel,
				"error", err)
		}
	}
}

func (s *Service) processJob(logger *slog.Logger, job *repository.NotificationJob) error {
	// Mark as processing
	if err := s.jobRepo.MarkProcessing(s.ctx, job.ID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	channel := notify.Channel(job.Channel)

	// Check rate limit
	limiter, ok := s.rateLimits[channel]
	if ok {
		allowed, err := limiter.Allow(s.ctx, fmt.Sprintf("ratelimit:%s:global", channel))
		if err != nil {
			return s.retryJob(job, fmt.Sprintf("rate limit check failed: %v", err))
		}
		if !allowed {
			return s.retryJob(job, "rate limit exceeded")
		}
	}

	// Get sender
	sender, ok := s.senders[channel]
	if !ok {
		return s.failJob(job, fmt.Sprintf("no sender for channel: %s", channel))
	}

	// Parse payload - it contains all the necessary data
	var payload map[string]interface{}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return s.failJob(job, fmt.Sprintf("invalid payload: %v", err))
	}

	// Build notification message from payload
	msg := notify.Message{
		UserID:    job.UserID,
		Title:     getStringFromPayload(payload, "title"),
		Body:      getStringFromPayload(payload, "body"),
		Priority:  getIntFromPayload(payload, "priority"),
		Exchange:  getStringFromPayload(payload, "exchange"),
		Symbol:    getStringFromPayload(payload, "symbol"),
		Price:     getFloatFromPayload(payload, "price"),
		Direction: getStringFromPayload(payload, "direction"),
		Notes:     getStringFromPayload(payload, "notes"),
	}

	// Channel-specific fields
	if channel == notify.ChannelTelegram {
		msg.TelegramID = getInt64FromPayload(payload, "telegram_id")
	} else if channel == notify.ChannelPushover {
		msg.PushoverKey = getStringFromPayload(payload, "pushover_key")
	}

	// Send notification
	if err := sender.Send(s.ctx, msg); err != nil {
		return s.retryJob(job, fmt.Sprintf("send failed: %v", err))
	}

	// Mark as done
	if err := s.jobRepo.MarkDone(s.ctx, job.ID); err != nil {
		logger.Error("failed to mark job done", "job_id", job.ID, "error", err)
		return err
	}

	logger.Info("job completed",
		"job_id", job.ID,
		"channel", channel,
		"attempts", job.Attempts+1)

	return nil
}

// Helper functions to safely extract values from payload
func getStringFromPayload(payload map[string]interface{}, key string) string {
	if val, ok := payload[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt64FromPayload(payload map[string]interface{}, key string) int64 {
	if val, ok := payload[key]; ok {
		if num, ok := val.(float64); ok {
			return int64(num)
		}
	}
	return 0
}

func getIntFromPayload(payload map[string]interface{}, key string) int {
	if val, ok := payload[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
	}
	return 0
}

func getFloatFromPayload(payload map[string]interface{}, key string) float64 {
	if val, ok := payload[key]; ok {
		if num, ok := val.(float64); ok {
			return num
		}
	}
	return 0
}

func (s *Service) retryJob(job *repository.NotificationJob, errMsg string) error {
	if job.Attempts+1 >= s.maxRetries {
		return s.failJob(job, fmt.Sprintf("max retries exceeded: %s", errMsg))
	}

	// Exponential backoff: 2^attempts seconds
	delay := 1 << job.Attempts
	if delay > 300 {
		delay = 300 // Max 5 minutes
	}

	return s.jobRepo.Retry(s.ctx, job.ID, delay, errMsg)
}

func (s *Service) failJob(job *repository.NotificationJob, errMsg string) error {
	s.logger.Warn("job failed permanently",
		"job_id", job.ID,
		"channel", job.Channel,
		"attempts", job.Attempts+1,
		"error", errMsg)

	return s.jobRepo.Fail(s.ctx, job.ID, errMsg)
}

func (s *Service) stuckJobRecovery() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.jobRepo.ResetStuckJobs(s.ctx); err != nil {
				s.logger.Error("failed to reset stuck jobs", "error", err)
			}
		}
	}
}
