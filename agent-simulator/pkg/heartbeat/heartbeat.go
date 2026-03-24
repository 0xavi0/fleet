// Package heartbeat sends periodic Cluster status heartbeats to the upstream cluster.
package heartbeat

import (
	"context"
	"time"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Ticker starts a goroutine that sends a heartbeat after initialDelay and then
// every interval until ctx is cancelled.
func Ticker(
	ctx context.Context,
	c client.Client,
	clusterNamespace, clusterName, agentNamespace string,
	initialDelay, interval time.Duration,
) {
	logger := log.FromContext(ctx).WithName("heartbeat").
		WithValues("cluster", clusterName, "namespace", clusterNamespace)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
		}

		logger.V(1).Info("Sending initial heartbeat")
		if err := Patch(ctx, c, clusterNamespace, clusterName, agentNamespace); err != nil {
			logger.Error(err, "failed to send initial heartbeat")
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.V(1).Info("Sending heartbeat")
				if err := Patch(ctx, c, clusterNamespace, clusterName, agentNamespace); err != nil {
					logger.Error(err, "failed to send heartbeat")
				}
			}
		}
	}()
}

// Patch sends a single JSONPatch to update Cluster.Status.Agent.LastSeen.
// This replicates the exact patch format used by internal/cmd/agent/clusterstatus/ticker.go.
func Patch(ctx context.Context, c client.Client, clusterNamespace, clusterName, agentNamespace string) error {
	cluster := &fleet.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterNamespace,
		},
	}

	patch := `[{"op":"add","path":"/status/agent","value":{"lastSeen":"` +
		time.Now().UTC().Format(time.RFC3339) +
		`","namespace":"` + agentNamespace +
		`"}}]`

	return c.Status().Patch(ctx, cluster, client.RawPatch(types.JSONPatchType, []byte(patch)))
}
