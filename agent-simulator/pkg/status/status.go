// Package status builds BundleDeploymentStatus values for the simulator.
package status

import (
	"fmt"
	"hash/crc32"
	"time"

	"github.com/rancher/fleet/agent-simulator/pkg/resources"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1/summary"
	"github.com/rancher/wrangler/v3/pkg/genericcondition"

	corev1 "k8s.io/api/core/v1"
)

// BuildParams are the inputs for building a BundleDeploymentStatus.
type BuildParams struct {
	// DeploymentID is copied to AppliedDeploymentID.
	DeploymentID string
	// ResourceCount is the total number of fake resources to report.
	ResourceCount int
	// ReadyCount is the number of resources that are ready (0 <= ReadyCount <= ResourceCount).
	ReadyCount int
	// AgentNamespace is the namespace used to generate the Helm release name.
	AgentNamespace string
	// Namespace is the target namespace placed on each fake resource.
	Namespace string
	// BDName is the BundleDeployment name, used to derive deterministic resource names
	// and the Helm release name.
	BDName string
}

// Build constructs a BundleDeploymentStatus from the given params.
func Build(p BuildParams) fleet.BundleDeploymentStatus {
	isReady := p.ReadyCount == p.ResourceCount

	res := resources.Generate(p.BDName, p.Namespace, p.ResourceCount)

	nonReady := buildNonReady(res, p.ReadyCount, p.ResourceCount)

	now := time.Now().UTC().Format(time.RFC3339)
	conditions := buildConditions(isReady, p.ReadyCount, p.ResourceCount, now)

	stateStr := "Ready"
	if !isReady {
		stateStr = "WaitApplied"
	}

	return fleet.BundleDeploymentStatus{
		AppliedDeploymentID: p.DeploymentID,
		Release:             releaseName(p.AgentNamespace, p.BDName),
		Ready:               isReady,
		NonModified:         true,
		Conditions:          conditions,
		Resources:           res,
		ResourceCounts: fleet.ResourceCounts{
			Ready:        p.ReadyCount,
			DesiredReady: p.ResourceCount,
			NotReady:     p.ResourceCount - p.ReadyCount,
		},
		NonReadyStatus: nonReady,
		Display: fleet.BundleDeploymentDisplay{
			Deployed:  fmt.Sprintf("%d/%d deployed", p.ResourceCount, p.ResourceCount),
			Monitored: fmt.Sprintf("%d/%d monitored", p.ResourceCount, p.ResourceCount),
			State:     stateStr,
		},
	}
}

// releaseName returns a deterministic Helm release name derived from the BD name.
// Format: "<agentNamespace>/s-<8hex digits>".
func releaseName(agentNamespace, bdName string) string {
	h := crc32.ChecksumIEEE([]byte(bdName))
	return fmt.Sprintf("%s/s-%08x", agentNamespace, h)
}

func buildConditions(isReady bool, readyCount, resourceCount int, now string) []genericcondition.GenericCondition {
	readyStatus := corev1.ConditionTrue
	readyMsg := ""
	if !isReady {
		readyStatus = corev1.ConditionFalse
		readyMsg = fmt.Sprintf("%d/%d resources ready", readyCount, resourceCount)
	}

	return []genericcondition.GenericCondition{
		{Type: "Deployed", Status: corev1.ConditionTrue, LastUpdateTime: now, Reason: "Deployed", Message: "deployed"},
		{Type: "Installed", Status: corev1.ConditionTrue, LastUpdateTime: now, Reason: "Installed", Message: "installed"},
		{Type: "Monitored", Status: corev1.ConditionTrue, LastUpdateTime: now, Reason: "Monitored", Message: "monitored"},
		{Type: "Ready", Status: readyStatus, LastUpdateTime: now, Reason: "Ready", Message: readyMsg},
	}
}

// buildNonReady populates NonReadyStatus for resources beyond the ready count.
func buildNonReady(res []fleet.BundleDeploymentResource, readyCount, resourceCount int) []fleet.NonReadyStatus {
	if readyCount >= resourceCount {
		return nil
	}
	nonReady := make([]fleet.NonReadyStatus, 0, resourceCount-readyCount)
	for i := readyCount; i < resourceCount && i < len(res); i++ {
		nonReady = append(nonReady, fleet.NonReadyStatus{
			Kind:       res[i].Kind,
			APIVersion: res[i].APIVersion,
			Namespace:  res[i].Namespace,
			Name:       res[i].Name,
			Summary: summary.Summary{
				State:         "WaitApplied",
				Transitioning: true,
			},
		})
	}
	return nonReady
}
