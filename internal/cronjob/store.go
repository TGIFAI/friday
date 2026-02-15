package cronjob

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

// Store provides thread-safe persistence of cron jobs to a JSON file.
type Store struct {
	path string
	jobs map[string]Job // keyed by Job.ID
	mu   sync.RWMutex
}

// NewStore creates a Store backed by the given file path.
// If the file does not exist it will be created on the first Save.
func NewStore(path string) *Store {
	return &Store{
		path: path,
		jobs: make(map[string]Job),
	}
}

// Load reads persisted jobs from disk. It is safe to call on a missing file.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // first run, nothing to load
		}
		return fmt.Errorf("read store file: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var jobs []Job
	if err := sonic.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("unmarshal store: %w", err)
	}

	s.jobs = make(map[string]Job, len(jobs))
	for _, j := range jobs {
		// Heartbeat jobs are always re-registered at startup by the
		// gateway with fresh runtime fields (Workspace, etc.). Discard
		// any that were accidentally persisted to avoid stale state.
		if IsHeartbeatJob(j.ID) {
			continue
		}
		s.jobs[j.ID] = j
	}
	return nil
}

// Save writes all jobs to disk atomically (tmp + rename).
func (s *Store) Save() error {
	s.mu.RLock()
	jobs := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.mu.RUnlock()

	data, err := sonic.Marshal(jobs)
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename store: %w", err)
	}
	return nil
}

// Add inserts a new job. Returns an error if the ID already exists.
func (s *Store) Add(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job already exists: %s", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

// Update replaces an existing job by ID.
func (s *Store) Update(job Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

// Remove deletes a job by ID.
func (s *Store) Remove(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, jobID)
}

// Get returns a job by ID.
func (s *Store) Get(jobID string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[jobID]
	return j, ok
}

// List returns all jobs.
func (s *Store) List() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}

// ListDue returns enabled jobs whose NextRunAt is at or before now.
func (s *Store) ListDue(now time.Time) []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var due []Job
	for _, j := range s.jobs {
		if !j.Enabled {
			continue
		}
		if j.NextRunAt != nil && !j.NextRunAt.After(now) {
			due = append(due, j)
		}
	}
	return due
}
