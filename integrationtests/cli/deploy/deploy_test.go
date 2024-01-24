package deploy_test

import (
	"github.com/onsi/gomega/gbytes"

	clihelper "github.com/rancher/fleet/integrationtests/cli"
	"github.com/rancher/fleet/integrationtests/utils"
	"github.com/rancher/fleet/internal/cmd/cli"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fleet CLI Deploy", func() {
	var args []string

	act := func(args []string) (*gbytes.Buffer, error) {
		cmd := cli.NewDeploy()
		args = append([]string{"--kubeconfig", kubeconfigPath}, args...)
		cmd.SetArgs(args)

		buf := gbytes.NewBuffer()
		cmd.SetOutput(buf)
		err := cmd.Execute()
		return buf, err
	}

	BeforeEach(func() {
		var err error
		namespace, err = utils.NewNamespaceName()
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).ToNot(HaveOccurred())

		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			})).ToNot(HaveOccurred())
		})
	})

	When("Deploying to a cluster", func() {
		BeforeEach(func() {
			args = []string{
				"--input-file", clihelper.AssetsPath + "bundledeployment/bd.yaml",
			}
		})

		It("creates resources", func() {
			buf, err := act(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf).To(gbytes.Say("defaultNamespace: default"))
			Expect(buf).To(gbytes.Say("objects"))
			Expect(buf).To(gbytes.Say("- apiVersion: v1"))

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-simple-chart-config"}, cm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("Specifying a namespace", func() {
		BeforeEach(func() {
			args = []string{
				"--input-file", clihelper.AssetsPath + "bundledeployment/bd.yaml",
				"--namespace", namespace,
			}
		})

		It("creates resources", func() {
			buf, err := act(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf).To(gbytes.Say("defaultNamespace: " + namespace))
			Expect(buf).To(gbytes.Say("objects"))
			Expect(buf).To(gbytes.Say("- apiVersion: v1"))

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-simple-chart-config"}, cm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("Printing results with --dry-run", func() {
		BeforeEach(func() {
			args = []string{
				"--input-file", clihelper.AssetsPath + "bundledeployment/bd.yaml",
				"--dry-run",
			}
		})

		It("prints a manifest and bundledeployment", func() {
			buf, err := act(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf).To(gbytes.Say("- apiVersion: v1"))
			Expect(buf).To(gbytes.Say("  data:"))
			Expect(buf).To(gbytes.Say("    name: example-value"))

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-simple-chart-config"}, cm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
