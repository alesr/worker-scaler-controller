package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alesr/worker-scaler-controller/internal/pkg/logutil"
	"github.com/alesr/worker-scaler-controller/internal/worker"
	"github.com/alesr/workerpool"
	redisadapter "github.com/alesr/workerpool/adapters/redisstream"
	"github.com/caarlos0/env/v11"
	"github.com/redis/go-redis/v9"
)

type config struct {
	RedisAddr  string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	StreamName string `env:"STREAM_NAME" envDefault:"orders_stream"`
	GroupName  string `env:"GROUP_NAME" envDefault:"image_processors"`
	ConsumerID string `env:"CONSUMER_ID"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := logutil.NewLogger("worker")
	slog.SetDefault(logger)

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("Failed to parse configuration", "error", err)
		os.Exit(1)
	}

	if cfg.ConsumerID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			logger.Error("Failed to get hostname for consumer ID", "error", err)
			os.Exit(1)
		}
		cfg.ConsumerID = hostname
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	pool := workerpool.New[worker.WorkerTask](ctx, 3, workerpool.WithBuffer[worker.WorkerTask](100))

	adapter := redisadapter.NewStreamAdapter(
		rdb,
		cfg.StreamName,
		cfg.GroupName,
		cfg.ConsumerID,
		redisadapter.DefaultStreamOptions(),
	)

	if err := adapter.Initialize(ctx, "0"); err != nil {
		logger.Error("Failed to initialize stream adapter", "error", err)
		os.Exit(1)
	}

	handler := worker.NewPoolHandler(pool, logger)
	go func() {
		adapter.Consume(ctx, handler)
	}()

	logger.Info("Worker booted and consuming from redis", "consumerID", cfg.ConsumerID, "stream", cfg.StreamName)

	<-ctx.Done()

	logger.Info("Shutdown signal received")
	pool.GracefulShutdown()
	logger.Info("Shutdown complete")
}
