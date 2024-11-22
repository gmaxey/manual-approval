package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/cloudbees-io/manual-approval/internal/manual_approval"
)

var (
	cmd = &cobra.Command{
		Use:   "manual-approval",
		Short: "Request manual approval from users and teams",
		Long:  "Request manual approval from users and teams",
		RunE:  run,
	}
	cfg manual_approval.Config
)

func Execute() error {
	return cmd.Execute()
}

func run(command *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments: %v", args)
	}
	newContext, cancel := context.WithCancel(context.Background())
	osChannel := make(chan os.Signal, 1)
	signal.Notify(osChannel, os.Interrupt)
	go func() {
		<-osChannel
		cancel()
	}()

	return cfg.Run(newContext)
}

func init() {
	// Define flags for configuring the Manual Approval
	cmd.Flags().StringVar(&cfg.Handler, "handler", "", "Handler field allows you to choose particular handler in the manual approval custom job.")
}
