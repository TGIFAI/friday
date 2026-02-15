package cronjob

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_AddAndList(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "jobs.json"))

	j := Job{ID: "j1", Name: "test", Enabled: true, CreatedAt: time.Now()}
	if err := s.Add(j); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Duplicate should error.
	if err := s.Add(j); err == nil {
		t.Fatal("expected error on duplicate Add")
	}

	jobs := s.List()
	if len(jobs) != 1 || jobs[0].ID != "j1" {
		t.Fatalf("List: got %v", jobs)
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.json")

	s1 := NewStore(path)
	now := time.Now().Truncate(time.Millisecond)
	j := Job{ID: "j1", Name: "persist", Enabled: true, CreatedAt: now}
	_ = s1.Add(j)
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into a fresh store.
	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	jobs := s2.List()
	if len(jobs) != 1 || jobs[0].ID != "j1" || jobs[0].Name != "persist" {
		t.Fatalf("reloaded jobs: %v", jobs)
	}
}

func TestStore_Remove(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "jobs.json"))
	_ = s.Add(Job{ID: "j1", CreatedAt: time.Now()})
	_ = s.Add(Job{ID: "j2", CreatedAt: time.Now()})

	s.Remove("j1")

	jobs := s.List()
	if len(jobs) != 1 || jobs[0].ID != "j2" {
		t.Fatalf("after Remove: %v", jobs)
	}
}

func TestStore_ListDue(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "jobs.json"))

	past := time.Now().Add(-1 * time.Minute)
	future := time.Now().Add(1 * time.Hour)

	_ = s.Add(Job{ID: "due", Enabled: true, NextRunAt: &past, CreatedAt: time.Now()})
	_ = s.Add(Job{ID: "not-due", Enabled: true, NextRunAt: &future, CreatedAt: time.Now()})
	_ = s.Add(Job{ID: "disabled", Enabled: false, NextRunAt: &past, CreatedAt: time.Now()})

	due := s.ListDue(time.Now())
	if len(due) != 1 || due[0].ID != "due" {
		t.Fatalf("ListDue: got %v", due)
	}
}

func TestStore_Load_MissingFile(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty list on missing file")
	}
}

func TestStore_Save_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	s := NewStore(filepath.Join(dir, "jobs.json"))
	_ = s.Add(Job{ID: "j1", CreatedAt: time.Now()})

	if err := s.Save(); err != nil {
		t.Fatalf("Save should create directories: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "jobs.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
