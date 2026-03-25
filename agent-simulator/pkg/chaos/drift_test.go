package chaos_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/rancher/fleet/agent-simulator/pkg/chaos"
)

func TestRandomInterval(t *testing.T) {
	t.Run("returns min when max < min", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.RandomInterval(rng, 100*time.Millisecond, 50*time.Millisecond)
		if got != 100*time.Millisecond {
			t.Errorf("expected %v, got %v", 100*time.Millisecond, got)
		}
	})

	t.Run("returns min when max == min", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.RandomInterval(rng, 100*time.Millisecond, 100*time.Millisecond)
		if got != 100*time.Millisecond {
			t.Errorf("expected %v, got %v", 100*time.Millisecond, got)
		}
	})

	t.Run("always within [min, max)", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		min := 100 * time.Millisecond
		max := 500 * time.Millisecond
		for i := 0; i < 1000; i++ {
			got := chaos.RandomInterval(rng, min, max)
			if got < min || got >= max {
				t.Errorf("iteration %d: interval %v out of [%v, %v)", i, got, min, max)
			}
		}
	})

	t.Run("produces values across the full range", func(t *testing.T) {
		rng := rand.New(rand.NewSource(99))
		min := 0 * time.Second
		max := 1 * time.Second
		var saw0, sawNear1 bool
		for i := 0; i < 10000; i++ {
			got := chaos.RandomInterval(rng, min, max)
			if got < 100*time.Millisecond {
				saw0 = true
			}
			if got > 900*time.Millisecond {
				sawNear1 = true
			}
		}
		if !saw0 {
			t.Error("expected values near min; none seen in 10000 samples")
		}
		if !sawNear1 {
			t.Error("expected values near max; none seen in 10000 samples")
		}
	})
}

func TestBuildModifiedStatus(t *testing.T) {
	t.Run("returns requested number of entries", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 10, 3)
		if len(got) != 3 {
			t.Errorf("expected 3 entries, got %d", len(got))
		}
	})

	t.Run("clamps to total resource count", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 2, 10)
		if len(got) != 2 {
			t.Errorf("expected 2 entries (clamped to totalCount), got %d", len(got))
		}
	})

	t.Run("returns empty when affectedCount is zero", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 10, 0)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("returns empty when totalCount is zero", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 0, 5)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("entries use the provided namespace", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "target-ns", 10, 2)
		for i, m := range got {
			if m.Namespace != "target-ns" {
				t.Errorf("entry %d: expected namespace %q, got %q", i, "target-ns", m.Namespace)
			}
		}
	})

	t.Run("entries have a non-empty Patch field", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 10, 1)
		if len(got) == 0 || got[0].Patch == "" {
			t.Error("expected non-empty Patch field")
		}
	})

	t.Run("entries have non-empty Kind and APIVersion", func(t *testing.T) {
		got := chaos.BuildModifiedStatus("my-bd", "ns", 10, 4)
		for i, m := range got {
			if m.Kind == "" {
				t.Errorf("entry %d: Kind is empty", i)
			}
			if m.APIVersion == "" {
				t.Errorf("entry %d: APIVersion is empty", i)
			}
		}
	})

	t.Run("deterministic - same inputs produce same output", func(t *testing.T) {
		a := chaos.BuildModifiedStatus("bd-a", "ns", 10, 3)
		b := chaos.BuildModifiedStatus("bd-a", "ns", 10, 3)
		if len(a) != len(b) {
			t.Fatalf("expected same length, got %d vs %d", len(a), len(b))
		}
		for i := range a {
			if a[i].Name != b[i].Name || a[i].Kind != b[i].Kind || a[i].APIVersion != b[i].APIVersion {
				t.Errorf("entry %d differs: %+v vs %+v", i, a[i], b[i])
			}
		}
	})

	t.Run("different BD names produce different resource names", func(t *testing.T) {
		a := chaos.BuildModifiedStatus("bd-alpha", "ns", 10, 1)
		b := chaos.BuildModifiedStatus("bd-beta", "ns", 10, 1)
		if len(a) == 0 || len(b) == 0 {
			t.Fatal("expected non-empty results")
		}
		if a[0].Name == b[0].Name {
			t.Errorf("expected different names for different BD names, both got %q", a[0].Name)
		}
	})
}
