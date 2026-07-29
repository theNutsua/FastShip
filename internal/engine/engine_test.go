package engine

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// fakeEngine is an in-memory Engine used only for tests. Its existence is
// itself a test: if the interface were badly designed, writing even a
// fake implementation would be painful. It also lets subsystems that
// depend on an Engine be tested without containerd or Linux.
type fakeEngine struct {
	started map[string]Spec
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{started: map[string]Spec{}}
}

func (f *fakeEngine) Start(ctx context.Context, spec Spec) (Handle, error) {
	f.started[spec.Name] = spec
	return Handle{ID: "fake-" + spec.Name, Name: spec.Name}, nil
}

func (f *fakeEngine) Stop(ctx context.Context, h Handle, drain time.Duration) error {
	delete(f.started, h.Name)
	return nil
}

func (f *fakeEngine) Status(ctx context.Context, h Handle) (Status, error) {
	if _, ok := f.started[h.Name]; ok {
		return Status{State: StateRunning}, nil
	}
	return Status{State: StateStopped}, nil
}

func (f *fakeEngine) Logs(ctx context.Context, h Handle) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("fake log line\n")), nil
}

func (f *fakeEngine) Exec(ctx context.Context, h Handle, cmd ExecSpec) error {
	return nil
}

// Compile-time proof that fakeEngine satisfies the interface. If a method
// signature drifts, this line stops compiling — a far clearer failure
// than discovering it deep in some other package.
var _ Engine = (*fakeEngine)(nil)

func TestEngineLifecycle(t *testing.T) {
	eng := newFakeEngine()
	ctx := context.Background()

	// Start a component.
	h, err := eng.Start(ctx, Spec{Name: "api", Image: "node:20-alpine"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if h.Name != "api" {
		t.Errorf("handle name = %q, want api", h.Name)
	}

	// It should report running.
	st, _ := eng.Status(ctx, h)
	if !st.Healthy() {
		t.Errorf("state = %s, want running", st.State)
	}

	// Stop it.
	if err := eng.Stop(ctx, h, 30*time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Now it should report stopped.
	st, _ = eng.Status(ctx, h)
	if st.State != StateStopped {
		t.Errorf("state = %s, want stopped", st.State)
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateStarting: "starting",
		StateRunning:  "running",
		StateStopped:  "stopped",
		StateFailed:   "failed",
		StateUnknown:  "unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}
