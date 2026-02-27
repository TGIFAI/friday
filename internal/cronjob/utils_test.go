package cronjob

import (
	"testing"
	"time"
)

func TestCalcNextRun_Every(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	job := &Job{ScheduleType: ScheduleEvery, Schedule: "5m"}

	next, err := calcNextRun(job, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := now.Add(5 * time.Minute)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestCalcNextRun_Every_Invalid(t *testing.T) {
	job := &Job{ScheduleType: ScheduleEvery, Schedule: "bad"}
	_, err := calcNextRun(job, time.Now())
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestCalcNextRun_Cron(t *testing.T) {
	// "0 9 * * *" = daily at 09:00
	now := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	job := &Job{ScheduleType: ScheduleCron, Schedule: "0 9 * * *"}

	next, err := calcNextRun(job, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestCalcNextRun_At_Future(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	job := &Job{ScheduleType: ScheduleAt, Schedule: "2026-02-01T09:00:00Z"}

	next, err := calcNextRun(job, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestCalcNextRun_At_Past(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	job := &Job{ScheduleType: ScheduleAt, Schedule: "2026-02-01T09:00:00Z"}

	next, err := calcNextRun(job, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !next.IsZero() {
		t.Errorf("expected zero time for past one-shot, got %v", next)
	}
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		consecutiveErr int
		want           time.Duration
	}{
		{0, 30 * time.Second},
		{1, 30 * time.Second},
		{2, 1 * time.Minute},
		{3, 5 * time.Minute},
		{4, 15 * time.Minute},
		{5, 60 * time.Minute},
		{100, 60 * time.Minute}, // capped
	}
	for _, tt := range tests {
		got := backoffDelay(tt.consecutiveErr)
		if got != tt.want {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.consecutiveErr, got, tt.want)
		}
	}
}
