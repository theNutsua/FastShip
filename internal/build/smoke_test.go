package build

import (
	"context"
	"testing"

	bkclient "github.com/moby/buildkit/client"
)

// TestSmokeConnect proves FastShip can reach the BuildKit daemon. Before
// building anything real, confirm the client connects against this exact
// BuildKit setup — the same "prove the smallest thing first" approach
// that de-risked the containerd engine.
//
// Run with: sudo go test ./internal/build/ -run Smoke -v
func TestSmokeConnect(t *testing.T) {
	ctx := context.Background()

	c, err := bkclient.New(ctx, "unix:///run/buildkit/buildkitd.sock")
	if err != nil {
		t.Fatalf("connect to buildkit: %v", err)
	}
	defer c.Close()

	// List workers — the same thing "buildctl debug workers" does, but
	// through the Go client FastShip will use.
	workers, err := c.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}

	if len(workers) == 0 {
		t.Fatal("no buildkit workers found")
	}

	t.Logf("connected to buildkit: %d worker(s), first platform: %v",
		len(workers), workers[0].Platforms)
}
