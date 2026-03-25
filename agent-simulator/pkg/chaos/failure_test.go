package chaos_test

import (
	"math/rand"
	"testing"

	"github.com/rancher/fleet/agent-simulator/pkg/chaos"
)

func TestBuildNonReadyStatus(t *testing.T) {
	t.Run("returns requested number of entries", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 10, 3, rng)
		if len(got) != 3 {
			t.Errorf("expected 3 entries, got %d", len(got))
		}
	})

	t.Run("clamps to total resource count", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 2, 10, rng)
		if len(got) != 2 {
			t.Errorf("expected 2 entries (clamped to totalCount), got %d", len(got))
		}
	})

	t.Run("returns empty when affectedCount is zero", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 10, 0, rng)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("returns empty when totalCount is zero", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 0, 5, rng)
		if len(got) != 0 {
			t.Errorf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("entries use the provided namespace", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "target-ns", 10, 2, rng)
		for i, s := range got {
			if s.Namespace != "target-ns" {
				t.Errorf("entry %d: expected namespace %q, got %q", i, "target-ns", s.Namespace)
			}
		}
	})

	t.Run("entries have Kind=Pod and APIVersion=v1", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 10, 4, rng)
		for i, s := range got {
			if s.Kind != "Pod" {
				t.Errorf("entry %d: expected Kind %q, got %q", i, "Pod", s.Kind)
			}
			if s.APIVersion != "v1" {
				t.Errorf("entry %d: expected APIVersion %q, got %q", i, "v1", s.APIVersion)
			}
		}
	})

	t.Run("entries have non-empty State and Message", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 10, 4, rng)
		for i, s := range got {
			if s.Summary.State == "" {
				t.Errorf("entry %d: State is empty", i)
			}
			if len(s.Summary.Message) == 0 || s.Summary.Message[0] == "" {
				t.Errorf("entry %d: Message is empty", i)
			}
			if !s.Summary.Error {
				t.Errorf("entry %d: expected Error=true", i)
			}
		}
	})

	t.Run("entries have non-empty pod names derived from BD name", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 10, 2, rng)
		for i, s := range got {
			if s.Name == "" {
				t.Errorf("entry %d: Name is empty", i)
			}
		}
	})

	t.Run("different BD names produce different pod names", func(t *testing.T) {
		rng1 := rand.New(rand.NewSource(42))
		rng2 := rand.New(rand.NewSource(42))
		a := chaos.BuildNonReadyStatus("bd-alpha", "ns", 10, 1, rng1)
		b := chaos.BuildNonReadyStatus("bd-beta", "ns", 10, 1, rng2)
		if len(a) == 0 || len(b) == 0 {
			t.Fatal("expected non-empty results")
		}
		if a[0].Name == b[0].Name {
			t.Errorf("expected different names for different BD names, both got %q", a[0].Name)
		}
	})

	t.Run("error states come from the known pool", func(t *testing.T) {
		rng := rand.New(rand.NewSource(99))
		// Generate a large sample to cover all states.
		got := chaos.BuildNonReadyStatus("my-bd", "ns", 100, 100, rng)
		validStates := map[string]bool{
			"CrashLoopBackOff":    true,
			"ImagePullBackOff":    true,
			"OOMKilled":           true,
			"CreateContainerError": true,
			"ErrImageNeverPull":   true,
		}
		for i, s := range got {
			if !validStates[s.Summary.State] {
				t.Errorf("entry %d: unexpected State %q", i, s.Summary.State)
			}
		}
	})

	t.Run("all error states are reachable over many samples", func(t *testing.T) {
		rng := rand.New(rand.NewSource(7777))
		seen := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			got := chaos.BuildNonReadyStatus("bd", "ns", 1, 1, rng)
			if len(got) > 0 {
				seen[got[0].Summary.State] = true
			}
		}
		want := []string{"CrashLoopBackOff", "ImagePullBackOff", "OOMKilled", "CreateContainerError", "ErrImageNeverPull"}
		for _, state := range want {
			if !seen[state] {
				t.Errorf("state %q never observed in 1000 samples", state)
			}
		}
	})

	t.Run("seeded RNG produces deterministic output", func(t *testing.T) {
		rng1 := rand.New(rand.NewSource(123))
		rng2 := rand.New(rand.NewSource(123))
		a := chaos.BuildNonReadyStatus("bd-det", "ns", 10, 3, rng1)
		b := chaos.BuildNonReadyStatus("bd-det", "ns", 10, 3, rng2)
		if len(a) != len(b) {
			t.Fatalf("expected same length, got %d vs %d", len(a), len(b))
		}
		for i := range a {
			if a[i].Name != b[i].Name || a[i].Summary.State != b[i].Summary.State {
				t.Errorf("entry %d differs: %+v vs %+v", i, a[i], b[i])
			}
		}
	})
}
