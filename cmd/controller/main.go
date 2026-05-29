package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alesr/autoscaler-engine/adapters"
	"github.com/alesr/worker-scaler-controller/internal/pkg/logutil"
	"github.com/alesr/worker-scaler-controller/internal/scaler"
	"github.com/alesr/worker-scaler-controller/internal/workload"
	"github.com/caarlos0/env/v11"
	"github.com/redis/go-redis/v9"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	engine "github.com/alesr/autoscaler-engine"
)

type config struct {
	RedisAddr        string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	StreamName       string        `env:"STREAM_NAME" envDefault:"orders_stream"`
	GroupName        string        `env:"GROUP_NAME" envDefault:"image_processors"`
	SyncPeriod       time.Duration `env:"SYNC_PERIOD" envDefault:"10s"`
	TasksPerWorker   int64         `env:"TASKS_PER_WORKER" envDefault:"10"`
	MaxWorkers       int32         `env:"MAX_WORKERS" envDefault:"5"`
	TargetDeployment string        `env:"TARGET_DEPLOYMENT" envDefault:"redis-worker"`
	K8sNamespace     string        `env:"K8S_NAMESPACE" envDefault:"default"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logutil.NewLogger("controller")
	slog.SetDefault(logger)

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("Failed to parse configuration", "error", err)
		os.Exit(1)
	}

	k8sConfig, err := getK8sConfig()
	if err != nil {
		logger.Error("Failed to get Kubernetes config", "error", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		logger.Error("Failed to create Kubernetes clientset", "error", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	k8sScaler := scaler.New(clientset, cfg.K8sNamespace)

	redisBacklogAdapter := adapters.NewRedisBacklog(rdb, cfg.StreamName, cfg.GroupName)

	workloadAdapter := workload.NewK8s(k8sScaler, cfg.TargetDeployment)

	engineCfg := engine.Config{
		TasksPerWorker: cfg.TasksPerWorker,
		MaxWorkers:     cfg.MaxWorkers,
		MinWorkers:     1,
		CooldownPeriod: 30 * time.Second,
	}

	autoscaler := engine.New(logger, redisBacklogAdapter, workloadAdapter, engineCfg)

	logger.Info("Scaler Controller started", "sync_period", cfg.SyncPeriod, "target", cfg.TargetDeployment)

	ticker := time.NewTicker(cfg.SyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down controller")
			return
		case <-ticker.C:
			if err := autoscaler.Reconcile(ctx); err != nil {
				// fine to just log error
				// 1. we're watching the stream queue, not k8s
				// 2. we retry in the next loop cycle
				logger.Error("Reconciliation cycle failed", "error", err)
			}
		}
	}
}

func getK8sConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
