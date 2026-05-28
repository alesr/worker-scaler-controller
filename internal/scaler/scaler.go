package scaler

import (
	"context"
	"fmt"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Scaler struct {
	clientset kubernetes.Interface
	namespace string
}

func New(clientset kubernetes.Interface, namespace string) *Scaler {
	return &Scaler{
		clientset: clientset,
		namespace: namespace,
	}
}

func (s *Scaler) GetScale(ctx context.Context, deploymentName string) (*autoscalingv1.Scale, error) {
	return s.clientset.AppsV1().Deployments(s.namespace).GetScale(ctx, deploymentName, metav1.GetOptions{})
}

// ScaleDeployment fetches the current scale state and updates it only if it differs
func (s *Scaler) ScaleDeployment(ctx context.Context, deploymentName string, desiredReplicas int32) (int32, error) {
	scale, err := s.clientset.AppsV1().
		Deployments(s.namespace).
		GetScale(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("could not get deployment scale: %w", err)
	}

	currentReplicas := scale.Spec.Replicas
	if currentReplicas == desiredReplicas {
		return currentReplicas, nil // no change needed
	}

	scale.Spec.Replicas = desiredReplicas

	if _, err := s.clientset.AppsV1().
		Deployments(s.namespace).
		UpdateScale(ctx, deploymentName, scale, metav1.UpdateOptions{}); err != nil {
		return 0, fmt.Errorf("could not to update deployment scale: %w", err)
	}
	return currentReplicas, nil
}
