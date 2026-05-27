package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alesr/worker-scaler-controller/internal/pkg/logutil"
	"github.com/caarlos0/env/v11"
	"github.com/redis/go-redis/v9"
)

type config struct {
	RedisAddr     string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	StreamName    string        `env:"STREAM_NAME" envDefault:"orders_stream"`
	ProducerDelay time.Duration `env:"PRODUCER_DELAY" envDefault:"1s"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logutil.NewLogger("producer")
	slog.SetDefault(logger)

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("Failed to parse configuration", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	logger.Info("Producer started", "delay", cfg.ProducerDelay, "stream", cfg.StreamName)

	// loop for testing backpressure which should
	// add jobs to the stream and therefore trigger the k8s autoscaler
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutdown received")
			return
		default:
			jobData := map[string]any{
				"job_type":  "image_processing",
				"timestamp": time.Now().UnixNano(),
			}

			if err := rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: cfg.StreamName,
				Values: jobData,
			}).Err(); err != nil {
				logger.Error("Failed to add job", "error", err)
			} else {
				logger.Info("Job added to stream")
			}
			time.Sleep(cfg.ProducerDelay)
		}
	}
}
