package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1995parham-learning/Pym/internal/server"
	"github.com/1995parham-learning/Pym/internal/shortener"
	"github.com/1995parham-learning/Pym/internal/shortener/pgstore"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	addr := envOr("ADDR", ":8080")
	baseURL := envOr("BASE_URL", "http://localhost"+addr)
	dsn := envOr("DATABASE_URL", "postgres://pym:pym@localhost:5432/pym?sslmode=disable")

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	store, err := pgstore.New(bootCtx, dsn)
	bootCancel()
	if err != nil {
		logger.Error("failed to initialize postgres store", "error", err)
		os.Exit(1)
	}

	svc := shortener.New(store, baseURL)
	srv := server.New(svc, store, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("server starting", "addr", addr, "base_url", baseURL)
	runErr := srv.Run(ctx, addr, 10*time.Second)
	store.Close()

	if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) && !errors.Is(runErr, context.Canceled) {
		logger.Error("server failed", "error", runErr)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
