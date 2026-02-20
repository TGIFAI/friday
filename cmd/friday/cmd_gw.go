package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/cronjob"
	"github.com/tgifai/friday/internal/gateway"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/pkg/updater"
)

var gwHwd = &GatewayRunner{}

type GatewayRunner struct{}

func (r *GatewayRunner) cmd() *cli.Command {
	return &cli.Command{
		Name:  "gateway",
		Usage: "Manage the gateway runtime",
		Commands: []*cli.Command{
			{
				Name:   "run",
				Usage:  "Run the gateway runtime with configured providers, agents, and channels",
				Action: r.run,
			},
			// TODO restart
		},
	}
}

func (r *GatewayRunner) run(ctx context.Context, _ *cli.Command) error {
	cfgPath := consts.DefaultConfigPath()

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Println("Friday is not configured yet. Run \"friday onboard\" to get started.")
		return nil
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config error: %w", err)
	}

	if err = r.initLogger(cfg.Logging); err != nil {
		return fmt.Errorf("init logger error: %w", err)
	}

	logs.CtxInfo(ctx, "booting Friday runtime, using config file: %s...", cfgPath)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	gw := gateway.NewGateway(cfg.Gateway)
	if err = gw.Start(ctx); err != nil {
		cancel()
		_ = gw.Stop(context.Background())
		return fmt.Errorf("start gateway: %w", err)
	}

	// --- cronjob scheduler ---
	if err = r.initCronjob(ctx, cfg, gw); err != nil {
		cancel()
		_ = gw.Stop(context.Background())
		return fmt.Errorf("init cronjob: %w", err)
	}

	logs.CtxInfo(ctx, "ALL IS WELL!!! Press Ctrl+C to stop.")

	if friday.VERSION != "n/a" && cfg.Gateway.AutoUpdate {
		logs.CtxInfo(ctx, "auto-update enabled, starting background checker...")
		go updater.StartAutoUpdate(ctx, updater.New(), 0)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case sig := <-signalCh:
		logs.CtxInfo(ctx, "Received shutdown signal (%s). Stopping runtime...", sig.String())
	case <-ctx.Done():
		logs.CtxInfo(ctx, "Context canceled. Stopping runtime...")
	}

	cronjob.Stop(context.Background())

	if err = gw.Stop(context.Background()); err != nil {
		logs.CtxError(ctx, "stop gateway error: %v", err)
	}

	logs.CtxInfo(ctx, "all stopped, good bye!")
	return nil
}

func (r *GatewayRunner) initCronjob(ctx context.Context, cfg *config.Config, gw *gateway.Gateway) error {
	if cfg.Cronjob.Enabled != nil && !*cfg.Cronjob.Enabled {
		logs.CtxInfo(ctx, "[cronjob] disabled, skipping")
		return nil
	}

	cronjob.Init(cfg.Cronjob, gw.Enqueue)

	s := cronjob.Default()
	for id, agCfg := range cfg.Agents {
		hbJob := cronjob.NewHeartbeatJob(id, agCfg.Workspace, 0)
		if err := s.AddJob(hbJob, false); err != nil {
			logs.CtxWarn(ctx, "[cronjob] register heartbeat for agent %s: %v", id, err)
		}

		compactJob := cronjob.NewCompactJob(id, agCfg.Workspace)
		if err := s.AddJob(compactJob, false); err != nil {
			logs.CtxWarn(ctx, "[cronjob] register compact for agent %s: %v", id, err)
		}
	}

	return cronjob.Start(ctx)
}

func (r *GatewayRunner) initLogger(cfg config.LoggingConfig) error {
	return logs.Init(logs.Options{
		Level:      cfg.Level,
		Format:     cfg.Format,
		Output:     cfg.Output,
		File:       cfg.File,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
	})
}
