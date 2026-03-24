package rollout_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/rollout"
)

func TestRollout(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rollout Suite")
}

var _ = Describe("Tracker", func() {
	var tracker *rollout.Tracker

	BeforeEach(func() {
		tracker = rollout.NewTracker()
	})

	Describe("Next", func() {
		It("advances through steps and signals done on the last step", func() {
			step, done := tracker.Next("ns/bd", "deploy-1", 3)
			Expect(step).To(Equal(1))
			Expect(done).To(BeFalse())

			step, done = tracker.Next("ns/bd", "deploy-1", 3)
			Expect(step).To(Equal(2))
			Expect(done).To(BeFalse())

			step, done = tracker.Next("ns/bd", "deploy-1", 3)
			Expect(step).To(Equal(3))
			Expect(done).To(BeTrue())
		})

		It("returns done immediately for totalSteps=1 (single-step / instant-ready mode)", func() {
			step, done := tracker.Next("ns/bd", "deploy-1", 1)
			Expect(step).To(Equal(1))
			Expect(done).To(BeTrue())
		})

		It("does not advance beyond totalSteps on repeated calls", func() {
			for i := 0; i < 5; i++ {
				tracker.Next("ns/bd", "deploy-1", 2) //nolint:errcheck
			}
			step, done := tracker.Next("ns/bd", "deploy-1", 2)
			Expect(step).To(Equal(2))
			Expect(done).To(BeTrue())
		})

		It("resets state when the deploymentID changes", func() {
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck

			// New deployment ID: starts over from step 1.
			step, done := tracker.Next("ns/bd", "deploy-2", 3)
			Expect(step).To(Equal(1))
			Expect(done).To(BeFalse())
		})

		It("tracks independent keys independently", func() {
			step1, _ := tracker.Next("ns/bd-a", "deploy-1", 3)
			step2, _ := tracker.Next("ns/bd-b", "deploy-1", 3)
			step1b, _ := tracker.Next("ns/bd-a", "deploy-1", 3)

			Expect(step1).To(Equal(1))
			Expect(step2).To(Equal(1))
			Expect(step1b).To(Equal(2))
		})
	})

	Describe("IsInProgress", func() {
		It("returns false when no state exists", func() {
			Expect(tracker.IsInProgress("ns/bd", "deploy-1")).To(BeFalse())
		})

		It("returns true after the first step of a multi-step rollout", func() {
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck
			Expect(tracker.IsInProgress("ns/bd", "deploy-1")).To(BeTrue())
		})

		It("returns false once the rollout is done", func() {
			tracker.Next("ns/bd", "deploy-1", 2) //nolint:errcheck
			tracker.Next("ns/bd", "deploy-1", 2) //nolint:errcheck
			Expect(tracker.IsInProgress("ns/bd", "deploy-1")).To(BeFalse())
		})

		It("returns false when deploymentID does not match stored state", func() {
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck
			Expect(tracker.IsInProgress("ns/bd", "deploy-2")).To(BeFalse())
		})
	})

	Describe("Delete", func() {
		It("removes state so the next Next call starts fresh", func() {
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck
			tracker.Next("ns/bd", "deploy-1", 3) //nolint:errcheck

			tracker.Delete("ns/bd")

			step, done := tracker.Next("ns/bd", "deploy-1", 3)
			Expect(step).To(Equal(1))
			Expect(done).To(BeFalse())
		})
	})
})

var _ = Describe("ReadyCount", func() {
	DescribeTable("computes ready count proportionally",
		func(resourceCount, currentStep, totalSteps, expected int) {
			Expect(rollout.ReadyCount(resourceCount, currentStep, totalSteps)).To(Equal(expected))
		},
		Entry("step 1 of 3, 6 resources", 6, 1, 3, 2),
		Entry("step 2 of 3, 6 resources", 6, 2, 3, 4),
		Entry("step 3 of 3, 6 resources (final)", 6, 3, 3, 6),
		Entry("step 1 of 1 (instant ready)", 5, 1, 1, 5),
		Entry("step beyond totalSteps (clamp)", 5, 10, 3, 5),
		Entry("step 0 of 3 (no resources ready)", 6, 0, 3, 0),
	)
})
