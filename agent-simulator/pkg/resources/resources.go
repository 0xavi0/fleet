// Package resources generates deterministic fake Kubernetes resources for a BundleDeployment.
package resources

import (
	"fmt"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// resourceTemplate describes one kind of fake resource.
type resourceTemplate struct {
	Kind       string
	APIVersion string
	Suffix     string
}

// templates is the cycling list of resource types used when generating fake resources.
var templates = []resourceTemplate{
	{Kind: "Deployment", APIVersion: "apps/v1", Suffix: "deploy"},
	{Kind: "Service", APIVersion: "v1", Suffix: "svc"},
	{Kind: "ConfigMap", APIVersion: "v1", Suffix: "cm"},
	{Kind: "ServiceAccount", APIVersion: "v1", Suffix: "sa"},
}

// Generate returns a deterministic list of count BundleDeploymentResources for the
// given BD name and target namespace. The same inputs always produce the same output.
func Generate(bdName, namespace string, count int) []fleet.BundleDeploymentResource {
	now := metav1.Now()
	result := make([]fleet.BundleDeploymentResource, 0, count)
	for i := 0; i < count; i++ {
		t := templates[i%len(templates)]
		result = append(result, fleet.BundleDeploymentResource{
			Kind:       t.Kind,
			APIVersion: t.APIVersion,
			Namespace:  namespace,
			Name:       Name(bdName, t.Suffix, i/len(templates)),
			CreatedAt:  now,
		})
	}
	return result
}

// Name returns the deterministic resource name for BD bdName, resource suffix
// and slot index within that suffix group.
func Name(bdName, suffix string, index int) string {
	return fmt.Sprintf("%s-%s-%d", bdName, suffix, index)
}
