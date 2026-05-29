package workload

import (
	"context"
	"fmt"

	"github.com/alesr/worker-scaler-controller/internal/scaler"
)

type K8s struct {
	client *scaler.Scaler
	target string
}

func NewK8s(client *scaler.Scaler, target string) *K8s {
	return &K8s{client: client, target: target}
}

func (k *K8s) GetReplicas(ctx context.Context) (int32, error) {
	scale, err := k.client.GetScale(ctx, k.target)
	if err != nil {
		return 0, err
	}
	return scale.Spec.Replicas, nil
}

func (k *K8s) SetReplicas(ctx context.Context, replicas int32) error {
	_, err := k.client.ScaleDeployment(ctx, k.target, replicas)
	return fmt.Errorf("could not scale deployment: %w", err)
}
