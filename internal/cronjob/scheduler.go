package cronjob

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	tickInterval     = 15 * time.Second
	defaultStorePath = "cronjob/jobs.json"
)

// EnqueueFunc is the callback the scheduler uses to submit messages into the
// gateway's message queue.
type EnqueueFunc func(ctx context.Context, msg *channel.Message) error

// Scheduler manages periodic and one-shot jobs, persists them to disk, and
// feeds due jobs into the gateway message pipeline.
type Scheduler struct {
	store      *Store
	enqueue    EnqueueFunc
	cfg        config.CronjobConfig
	concurrent chan struct{} // semaphore sized to MaxConcurrentRuns

	runningMu sync.Mutex
	running   map[string]struct{} // jobIDs currently executing (singleton guard)

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates a scheduler backed by the given store path and config.
func NewScheduler(cfg config.CronjobConfig, enqueue EnqueueFunc) *Scheduler {
	storePath := filepath.Join(consts.FridayHomeDir(), defaultStorePath)

	maxConcurrent := cfg.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	return &Scheduler{
		store:      NewStore(storePath),
		enqueue:    enqueue,
		cfg:        cfg,
		concurrent: make(chan struct{}, maxConcurrent),
		running:    make(map[string]struct{}),
	}
}

// Start loads persisted jobs and begins the scheduling loop.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.store.Load(); err != nil {
		return fmt.Errorf("load job store: %w", err)
	}

	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()

	logs.CtxInfo(ctx, "[cronjob] scheduler started (max_concurrent=%d)", cap(s.concurrent))
	return nil
}

// Stop cancels the scheduling loop and waits for in-flight jobs to finish.
func (s *Scheduler) Stop(ctx context.Context) {
	if s.cancel != nil {
		s.cancel()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logs.CtxWarn(ctx, "[cronjob] stop timed out waiting for running jobs")
	}

	if err := s.store.Save(); err != nil {
		logs.CtxWarn(ctx, "[cronjob] save store on shutdown: %v", err)
	}
	logs.CtxInfo(ctx, "[cronjob] scheduler stopped")
}

// AddJob registers a job with the scheduler. If persist is true the job is
// written to the store file. Heartbeat jobs are idempotent — if a heartbeat
// with the same ID already exists, its runtime fields are updated in place.
func (s *Scheduler) AddJob(job Job, persist bool) error {
	now := time.Now()
	if job.NextRunAt == nil {
		next, err := calcNextRun(&job, now)
		if err != nil {
			return fmt.Errorf("calc initial next run: %w", err)
		}
		job.NextRunAt = &next
	}

	if err := s.store.Add(job); err != nil {
		// Heartbeat jobs are re-registered on every startup; silently
		// update the existing entry instead of returning an error.
		if IsHeartbeatJob(job.ID) {
			s.store.Update(job)
		} else {
			return err
		}
	}

	if persist {
		if err := s.store.Save(); err != nil {
			return fmt.Errorf("persist job: %w", err)
		}
	}
	return nil
}

// RemoveJob removes a job by ID and persists the change.
func (s *Scheduler) RemoveJob(jobID string) error {
	s.store.Remove(jobID)
	return s.store.Save()
}

// ListJobs returns all registered jobs.
func (s *Scheduler) ListJobs() []Job {
	return s.store.List()
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

func (s *Scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	for _, job := range s.store.ListDue(now) {
		if !s.tryAcquire() {
			break // hit concurrency limit, try next tick
		}
		if s.isRunning(job.ID) {
			s.release()
			continue // singleton: skip if still executing
		}

		s.markRunning(job.ID)
		j := job // capture for goroutine
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.release()
			defer s.markNotRunning(j.ID)
			s.executeJob(ctx, j, now)
		}()
	}
}

func (s *Scheduler) executeJob(ctx context.Context, job Job, now time.Time) {
	// Apply job timeout to prevent a blocked enqueue from freezing the
	// scheduler's concurrency semaphore.
	timeout := time.Duration(s.cfg.JobTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Heartbeat: build prompt dynamically, skip if empty.
	if IsHeartbeatJob(job.ID) {
		prompt, hasWork := BuildHeartbeatPrompt(job.Workspace)
		if !hasWork {
			logs.CtxDebug(ctx, "[cronjob] heartbeat %s: no work, skipping", job.ID)
			s.reschedule(&job, now)
			return
		}
		job.Prompt = prompt
	}

	msg := s.buildMessage(&job)
	if err := s.enqueue(ctx, msg); err != nil {
		logs.CtxWarn(ctx, "[cronjob] enqueue job %s failed: %v", job.ID, err)
		job.ConsecutiveErr++
		s.rescheduleWithBackoff(&job, now)
		return
	}

	logs.CtxInfo(ctx, "[cronjob] fired job %s (%s)", job.Name, job.ID)
	job.LastRunAt = &now
	job.ConsecutiveErr = 0
	s.reschedule(&job, now)
}

func (s *Scheduler) buildMessage(job *Job) *channel.Message {
	sessionKey := fmt.Sprintf("cron:%s", job.ID)
	if job.SessionTarget == SessionMain {
		// Use a deterministic session key so the heartbeat shares the agent's
		// main conversation context.
		sessionKey = fmt.Sprintf("agent:%s:cron:%s:%s", job.AgentID, job.AgentID, job.ID)
	}

	return &channel.Message{
		ID:          fmt.Sprintf("cron-%s-%d", job.ID, time.Now().UnixMilli()),
		ChannelID:   job.ChannelID,
		ChannelType: channel.Type("cron"),
		ChatID:      job.ChatID,
		Content:     job.Prompt,
		SessionKey:  sessionKey,
		Metadata: map[string]string{
			"cron_job_id":   job.ID,
			"cron_job_name": job.Name,
			"agent_id":      job.AgentID,
		},
	}
}

func (s *Scheduler) reschedule(job *Job, from time.Time) {
	next, err := calcNextRun(job, from)
	if err != nil {
		logs.Warn("[cronjob] reschedule %s failed: %v, disabling", job.ID, err)
		job.Enabled = false
		job.NextRunAt = nil
	} else if next.IsZero() {
		// One-shot (ScheduleAt) that has already passed.
		job.Enabled = false
		job.NextRunAt = nil
	} else {
		job.NextRunAt = &next
	}
	s.store.Update(*job)
	if err := s.store.Save(); err != nil {
		logs.Warn("[cronjob] persist after reschedule %s: %v", job.ID, err)
	}
}

func (s *Scheduler) rescheduleWithBackoff(job *Job, from time.Time) {
	delay := backoffDelay(job.ConsecutiveErr)
	next := from.Add(delay)
	job.NextRunAt = &next
	logs.Warn("[cronjob] job %s backoff %v (errors=%d)", job.ID, delay, job.ConsecutiveErr)
	s.store.Update(*job)
	if err := s.store.Save(); err != nil {
		logs.Warn("[cronjob] persist after backoff %s: %v", job.ID, err)
	}
}

// concurrency helpers

func (s *Scheduler) tryAcquire() bool {
	select {
	case s.concurrent <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Scheduler) release() {
	<-s.concurrent
}

func (s *Scheduler) isRunning(jobID string) bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	_, ok := s.running[jobID]
	return ok
}

func (s *Scheduler) markRunning(jobID string) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	s.running[jobID] = struct{}{}
}

func (s *Scheduler) markNotRunning(jobID string) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	delete(s.running, jobID)
}
