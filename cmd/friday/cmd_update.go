package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/updater"
)

var updateHwd = &UpdateRunner{}

type UpdateRunner struct{}

func (r *UpdateRunner) cmd() *cli.Command {
	return &cli.Command{
		Name:   "update",
		Usage:  "Check for and apply updates from GitHub releases",
		Action: r.run,
	}
}

func (r *UpdateRunner) run(ctx context.Context, _ *cli.Command) error {
	fmt.Printf("Friday %s\n", friday.VERSION)
	fmt.Println("Checking for updates...")

	u := updater.New()
	needs, release, err := u.NeedsUpdate(ctx)
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}
	if !needs {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s\n", release.TagName)
	fmt.Print("Download and install? [y/N] ")

	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Update cancelled.")
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "friday-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("Downloading...")
	binaryPath, err := u.Download(ctx, release, tmpDir)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	fmt.Println("Applying update...")
	if err := u.Apply(binaryPath); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	fmt.Printf("Successfully updated to %s!\n", release.TagName)

	// Check if gateway is running
	if r.isGatewayRunning() {
		fmt.Println("\nNote: The gateway is currently running. Please restart it to use the new version.")
	}

	return nil
}

func (r *UpdateRunner) isGatewayRunning() bool {
	cfgPath := consts.DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return false
	}

	bind := cfg.Gateway.Bind
	if bind == "" {
		return false
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/health", bind))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
