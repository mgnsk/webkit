// Package app runs an HTTP server alongside background tasks with graceful
// shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// Task is a background task. It must return when ctx is done.
type Task func(ctx context.Context) error

// ShutdownTimeout is the time given to in-flight requests on shutdown.
var ShutdownTimeout = 10 * time.Second

// Run serves srv and runs tasks until SIGINT or SIGTERM is received, ctx is
// done or any of them fails. The server is then shut down gracefully.
func Run(ctx context.Context, srv *http.Server, tasks ...Task) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	for _, task := range tasks {
		g.Go(func() error {
			return task(ctx)
		})
	}

	g.Go(func() error {
		slog.Info(fmt.Sprintf("listening at %s", srv.Addr))

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("error running server: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		slog.Info("shutting down the server")

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down server: %w", err)
		}

		return nil
	})

	return g.Wait()
}

// Every returns a task that calls f with the tick time every d.
func Every(d time.Duration, f func(ctx context.Context, now time.Time) error) Task {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(d)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil

			case now := <-ticker.C:
				if err := f(ctx, now); err != nil {
					return err
				}
			}
		}
	}
}
