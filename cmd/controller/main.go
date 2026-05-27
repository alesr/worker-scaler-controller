// cmd/controller/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alesr/worker-scaler-controller/internal/pkg/logutil"
	"github.com/alesr/worker-scaler-controller/internal/scaler"
	"github.com/caarlos0/env/v11"
	"github.com/redis/go-redis/v9"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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

	logger.Info("Scaler Controller started", "sync_period", cfg.SyncPeriod, "target", cfg.TargetDeployment)

	ticker := time.NewTicker(cfg.SyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down controller")
			return
		case <-ticker.C:
			reconcile(ctx, logger, rdb, k8sScaler, cfg)
		}
	}
}

func reconcile(ctx context.Context, logger *slog.Logger, rdb *redis.Client, s *scaler.Scaler, cfg config) {
	var backlog int64

	// fetch consumer group stats
	groups, err := rdb.XInfoGroups(ctx, cfg.StreamName).Result()
	if err == nil {
		for _, g := range groups {
			if g.Name == cfg.GroupName {
				// backlog = active processing + waiting
				backlog = g.Pending + g.Lag
				break
			}
		}
	} else {
		logger.Debug("Could not fetch group info (stream might be empty)", "error", err)
	}

	desiredReplicas := calculateReplicas(backlog, cfg.TasksPerWorker, cfg.MaxWorkers)

	oldReplicas, err := s.ScaleDeployment(ctx, cfg.TargetDeployment, desiredReplicas)
	if err != nil {
		logger.Error("Failed to scale deployment", "deployment", cfg.TargetDeployment, "error", err)
		return
	}

	if oldReplicas != desiredReplicas {
		logger.Info(
			"Deployment scaled successfully",
			"deployment", cfg.TargetDeployment,
			"backlog", backlog,
			"from_replicas", oldReplicas,
			"to_replicas", desiredReplicas,
		)
	}
}

func calculateReplicas(backlog int64, tasksPerWorker int64, maxWorkers int32) int32 {
	// calculate the required workers
	desired := backlog / tasksPerWorker
	if backlog%tasksPerWorker != 0 {
		desired++
	}

	// "MinReplicas" constraint
	if desired < 1 {
		desired = 1
	}

	// "MaxWorkers" constraint
	if desired > int64(maxWorkers) {
		desired = int64(maxWorkers)
	}
	return int32(desired)
}

func getK8sConfig() (*rest.Config, error) {
	// check if running inside a Cluster pod
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// fallback: compile out-of-cluster config using standard local Kubeconfig
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
