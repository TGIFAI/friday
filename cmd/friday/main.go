package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/pkg/logs"
)

func main() {
	cmd := &cli.Command{
		Name:  "friday",
		Usage: "Thank God It's Friday, Your Personal AI Assistant",
		Commands: []*cli.Command{
			gwHwd.cmd(),
			msgHwd.cmd(),
			onboardHwd.cmd(),
			updateHwd.cmd(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		logs.Error("Command execution failed: %v", err)
		os.Exit(1)
	}
}
