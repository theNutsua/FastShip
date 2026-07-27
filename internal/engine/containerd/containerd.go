// Package containerd implements the engine.Engine interface using
// containerd as the backend.
//
// This is the first real engine — the thing that turns FastShip's
// engine-neutral Specs into actual running containers via the Linux
// kernel. Everything in here is containerd-specific and Linux-specific
// on purpose. None of it leaks upward: callers only ever see the
// engine.Engine interface, never containerd types.
//
// The boundary discipline: if a containerd concept has no place in the
// engine interface, it stays contained in this package.
package containerd

import (
	"context"
	"fmt"
	"io"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"

	"github.com/theNutsua/FastShip/internal/engine"
)

// defaultSocket is where containerd listens by default on Linux.
const defaultSocket = "/run/containerd/containerd.sock"

// namespace isolates FastShip's containers from anything else using
// containerd on the same host. containerd namespaces are an
// organizational boundary, not a security one — they keep FastShip's
// containers, images, and snapshots grouped under one name.
const namespace = "fastship"

// Engine is the containerd-backed implementation of engine.Engine.
type Engine struct {
	client *containerd.Client
}

// Compile-time proof that this satisfies the interface. If a method
// signature drifts from the interface, this line stops compiling.
var _ engine.Engine = (*Engine)(nil)

// New connects to the local containerd daemon.
//
// It returns an error rather than panicking if containerd is not
// reachable — a missing or stopped containerd is a common, recoverable
// setup problem, and the caller should be able to report it cleanly.
func New() (*Engine, error) {
	client, err := containerd.New(defaultSocket)
	if err != nil {
		return nil, fmt.Errorf(
			"could not connect to containerd at %s: %w\n\n"+
				"is containerd running? try: sudo systemctl status containerd",
			defaultSocket, err)
	}
	return &Engine{client: client}, nil
}

// Close releases the containerd connection. Callers should defer this.
func (e *Engine) Close() error {
	return e.client.Close()
}

// withNamespace returns a context scoped to FastShip's containerd
// namespace. Every containerd call needs this — without it, containerd
// does not know which namespace to operate in and rejects the call.
func withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, namespace)
}

// Start pulls the image if needed, creates a container from the Spec,
// and starts its task (the running process).
//
// containerd splits this into two concepts that FastShip's interface
// deliberately hides behind one Start call:
//   - a Container: the definition (image, config, spec)
//   - a Task:      the actual running process
//
// You create a container, then create and start its task.
func (e *Engine) Start(ctx context.Context, spec engine.Spec) (engine.Handle, error) {
	ctx = withNamespace(ctx)

	// 1. Pull the image. containerd caches it, so this is a no-op after
	//    the first pull of a given image.
	image, err := e.client.Pull(ctx, spec.Image, containerd.WithPullUnpack)
	if err != nil {
		return engine.Handle{}, fmt.Errorf("pulling %s: %w", spec.Image, err)
	}

	// 2. Build the OCI runtime spec — the low-level description of the
	//    process, its environment, and its isolation. oci.WithImageConfig
	//    pulls sensible defaults (entrypoint, env, working dir) straight
	//    from the image's own metadata.
	opts := []oci.SpecOpts{
		oci.WithImageConfig(image),
	}

	// Override the start command if the Spec provides one.
	if len(spec.Cmd) > 0 {
		opts = append(opts, oci.WithProcessArgs(spec.Cmd...))
	}

	// Inject environment variables. Secrets are already resolved to plain
	// values by this point — the engine neither knows nor cares which of
	// these came from the secret store.
	for k, v := range spec.Env {
		opts = append(opts, oci.WithEnv([]string{k + "=" + v}))
	}

	// 3. Create the container definition.
	container, err := e.client.NewContainer(
		ctx,
		spec.Name, // container ID — FastShip uses the component name
		containerd.WithNewSnapshot(spec.Name+"-snapshot", image),
		containerd.WithNewSpec(opts...),
	)
	if err != nil {
		return engine.Handle{}, fmt.Errorf("creating container %s: %w", spec.Name, err)
	}

	// 4. Create the task — the actual OS process. cio.NullIO discards
	//    output for now; the Logs method will wire real IO later. For
	//    this first milestone we just need the process to run.
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		// Clean up the container we just made, or it leaks.
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return engine.Handle{}, fmt.Errorf("creating task for %s: %w", spec.Name, err)
	}

	// 5. Start the task running.
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return engine.Handle{}, fmt.Errorf("starting task for %s: %w", spec.Name, err)
	}

	return engine.Handle{ID: spec.Name, Name: spec.Name}, nil
}

// Status reports whether a container's task is running, stopped, or
// failed, translating containerd's process status into FastShip's
// engine-neutral State.
func (e *Engine) Status(ctx context.Context, h engine.Handle) (engine.Status, error) {
	ctx = withNamespace(ctx)

	container, err := e.client.LoadContainer(ctx, h.ID)
	if err != nil {
		return engine.Status{State: engine.StateUnknown}, nil
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		// No task means the container exists but nothing is running.
		return engine.Status{State: engine.StateStopped}, nil
	}

	status, err := task.Status(ctx)
	if err != nil {
		return engine.Status{State: engine.StateUnknown, Message: err.Error()}, nil
	}

	// Translate containerd's process status into FastShip's model.
	switch status.Status {
	case containerd.Running:
		return engine.Status{State: engine.StateRunning}, nil
	case containerd.Stopped:
		st := engine.Status{State: engine.StateStopped, ExitCode: int(status.ExitStatus)}
		// A non-zero exit code means it crashed, not finished cleanly.
		if status.ExitStatus != 0 {
			st.State = engine.StateFailed
			st.Message = fmt.Sprintf("exited with code %d", status.ExitStatus)
		}
		return st, nil
	case containerd.Created, containerd.Paused, containerd.Pausing:
		return engine.Status{State: engine.StateStarting}, nil
	default:
		return engine.Status{State: engine.StateUnknown}, nil
	}
}

// Stop kills a container's task and removes the container.
//
// This first version does not yet honour the drain period — it stops
// immediately. Graceful draining (stop new traffic, wait for in-flight
// work, then terminate) comes once the network layer exists to actually
// route traffic. Right now there is no traffic to drain.
// Stop kills a container's task and removes the container.
func (e *Engine) Stop(ctx context.Context, h engine.Handle, drain time.Duration) error {
	ctx = withNamespace(ctx)

	container, err := e.client.LoadContainer(ctx, h.ID)
	if err != nil {
		return fmt.Errorf("loading container %s: %w", h.ID, err)
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		// Set up a wait BEFORE killing, so we don't miss the exit signal.
		// task.Wait returns a channel that fires when the process exits.
		exitCh, err := task.Wait(ctx)
		if err != nil {
			return fmt.Errorf("waiting on task %s: %w", h.ID, err)
		}

		// Send SIGKILL. This returns immediately — the process is not dead
		// yet, it has only been signalled.
		if err := task.Kill(ctx, 9); err != nil {
			return fmt.Errorf("killing task %s: %w", h.ID, err)
		}

		// Block until the process actually exits. This is the missing step
		// — Delete fails if the task is still running.
		<-exitCh

		// Now it is safe to delete the task.
		if _, err := task.Delete(ctx); err != nil {
			return fmt.Errorf("deleting task %s: %w", h.ID, err)
		}
	}

	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}

// Logs streams a container's output. Stubbed for this first milestone —
// it needs the task to be created with a real IO creator instead of
// NullIO, which we wire once the basic lifecycle is proven.
func (e *Engine) Logs(ctx context.Context, h engine.Handle) (io.ReadCloser, error) {
	return nil, fmt.Errorf("logs not yet implemented")
}

// Exec runs a command inside a running container. Stubbed for now —
// it powers "fastship shell" and comes after the basic lifecycle works.
func (e *Engine) Exec(ctx context.Context, h engine.Handle, cmd engine.ExecSpec) error {
	return fmt.Errorf("exec not yet implemented")
}
