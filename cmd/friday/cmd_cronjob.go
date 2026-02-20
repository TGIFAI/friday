package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/cronjob"
)

var cronjobHwd = &CronjobRunner{}

type CronjobRunner struct{}

func (r *CronjobRunner) cmd() *cli.Command {
	return &cli.Command{
		Name:  "cronjob",
		Usage: "Manage scheduled cron jobs",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "List all persisted cron jobs",
				Action: r.list,
			},
		},
	}
}

func (r *CronjobRunner) list(_ context.Context, _ *cli.Command) error {
	jobs, err := cronjob.LoadJobsFromStore()
	if err != nil {
		return fmt.Errorf("load jobs: %w", err)
	}

	fmt.Print(cronjob.FormatJobList(jobs))
	return nil
}
