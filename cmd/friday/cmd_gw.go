package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/gateway"
	"github.com/tgifai/friday/internal/pkg/logs"
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

	logs.CtxInfo(ctx, "ALL IS WELL!!! Press Ctrl+C to stop.")

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case sig := <-signalCh:
		logs.CtxInfo(ctx, "Received shutdown signal (%s). Stopping runtime...", sig.String())
	case <-ctx.Done():
		logs.CtxInfo(ctx, "Context canceled. Stopping runtime...")
	}

	if err = gw.Stop(context.Background()); err != nil {
		logs.CtxError(ctx, "stop gateway error: %v", err)
	}

	logs.CtxInfo(ctx, "all stopped, good bye!")
	return nil
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

