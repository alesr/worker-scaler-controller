package worker

import (
	"context"
	"log/slog"

	"github.com/alesr/worker-scaler-controller/internal/pkg/contexutil"
	"github.com/alesr/workerpool"
)

type WorkerTask struct {
	ID      string
	Payload map[string]string
	Ack     func() error
}

// Do satisfies the workerpool.Task interface
func (w WorkerTask) Do(ctx context.Context) {
	logger := contexutil.LoggerFromContext(ctx)

	defer func() {
		if err := recover(); err != nil {
			logger.Error("Worker panicked", "id", w.ID, "error", err)
		}
	}()

	logger.Info("Processing task", "id", w.ID)

	if err := w.Ack(); err != nil {
		logger.Error("Failed to ack task", "id", w.ID, "error", err)
		return
	}
	logger.Info("Task processed successfully", "id", w.ID)
}

// PoolHandler bridges the adapter and the workerpool
type PoolHandler struct {
	pool   *workerpool.Pool[WorkerTask]
	logger *slog.Logger
}

func NewPoolHandler(pool *workerpool.Pool[WorkerTask], logger *slog.Logger) *PoolHandler {
	return &PoolHandler{pool: pool, logger: logger}
}

func (ph *PoolHandler) Submit(ctx context.Context, id string, payload map[string]string, ack func() error) error {
	return ph.pool.Submit(contexutil.WithLogger(ctx, ph.logger), WorkerTask{
		ID:      id,
		Payload: payload,
		Ack:     ack,
	})
}
