package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// A single root context means Ctrl-C unwinds every command's RPCs and
	// deferred cleanup rather than killing the process mid-call.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := New()
	if err := cmd.ExecuteContext(ctx); err != nil {
		// SilenceErrors is set on the root, so report the failure here to keep
		// formatting consistent and out of stdout.
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}
