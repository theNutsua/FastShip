package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/theNutsua/FastShip/internal/engine"
)

// TestSmokeNginx is a real integration test — it needs containerd running
// and network access to pull nginx. It proves the full lifecycle against
// a real backend: pull, start, status, stop.
//
// Run with: go test ./internal/engine/containerd/ -run Smoke -v
func TestSmokeNginx(t *testing.T) {
	eng, err := New()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	// Start nginx.
	h, err := eng.Start(ctx, engine.Spec{
		Name:  "smoke-nginx",
		Image: "docker.io/library/nginx:alpine",
	})

	// Clean up no matter how the test exits. If an assertion below fails,
	// this still runs — so a failed test never leaves an orphan snapshot
	// that blocks the next run. This is why the test is now rerunnable
	// without manual "ctr snapshot rm" between attempts.
	defer eng.Stop(ctx, h, 5*time.Second)

	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Give it a moment to come up.
	time.Sleep(2 * time.Second)

	// It should be running.
	st, err := eng.Status(ctx, h)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if !st.Healthy() {
		t.Errorf("state = %s, want running", st.State)
	}

	// Stop it.
	if err := eng.Stop(ctx, h, 5*time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}

	t.Log("nginx pulled, started, reported running, and stopped cleanly")
}
