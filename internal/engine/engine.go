// Package engine defines the boundary between FastShip and whatever
// actually runs containers.
// This interface is the single most important architectural decision in
// FastShip: everything above it describes WHAT an application is, and
// everything below it decides HOW it runs. containerd is the first
// implementation, but the interface exists precisely so it is not the
// last — Firecracker, WASM, or plain host processes could all satisfy
// it without a single change to the CLI, the config, or the planner.
//
// The rule for this package: nothing here may mention containerd, Linux,
// namespaces, or any other implementation detail. If a concept only makes
// sense for one engine, it does not belong in this interface.
package engine

import (
	"context"
	"io"
	"time"
)

// Engine runs application components and manages their lifecycle.
// An implementation is free to run components however it likes — as
// containers, microVMs, WASM modules, or processes — as long as it
// honours this contract.
type Engine interface {
	// Start brings a component to life and returns a handle to it.
	// It blocks until the component is created and started, but NOT until
	// it is healthy — health is polled separately via Status, because a
	// process being started and a process being ready to serve traffic
	// are two different moments.
	Start(ctx context.Context, spec Spec) (Handle, error)

	// Stop shuts a component down gracefully.
	// drain is how long to allow in-flight work to finish before forcing
	// termination. An engine should stop sending new work immediately,
	// wait up to drain for existing work to complete, then terminate.
	Stop(ctx context.Context, h Handle, drain time.Duration) error

	// Status reports the current state of a component.
	// This is what a health-check loop polls after Start to learn when a
	// component has actually become ready.
	Status(ctx context.Context, h Handle) (Status, error)

	// Logs returns a stream of the component's output.
	// The caller owns the returned reader and must close it. The stream
	// stays open and follows new output until closed or the component
	// exits.
	Logs(ctx context.Context, h Handle) (io.ReadCloser, error)

	// Exec runs a command inside a running component and attaches to it.
	// This is what powers "fastship shell" — an interactive session, or a
	// one-off command, inside an already-running component.
	Exec(ctx context.Context, h Handle, cmd ExecSpec) error
}

// Handle identifies a running component so later calls can refer back to
// it. It is opaque on purpose: the caller never inspects it, only passes
// it back to the engine. What is inside it is the engine's business.
type Handle struct {
	// ID is the engine's own identifier for the component. For containerd
	// this is a container ID; another engine might use something else.
	ID string

	// Name is the component's FastShip name, carried for logging and
	// display so a Handle is human-readable in error messages.
	Name string
}

// Spec is everything an engine needs to start one component.
//
// It is deliberately expressed in FastShip's own vocabulary, not any
// engine's. It is the planner's job to translate a fastship.yaml component
// into a Spec; it is the engine's job to translate a Spec into whatever
// its backend understands.
type Spec struct {
	// Name is the component's identifier and its DNS hostname.
	Name string

	// Image is the resolved image reference to run, e.g.
	// "docker.io/library/postgres:15-alpine". By the time a Spec reaches
	// the engine the image already exists — building and resolving happen
	// upstream in the planner and build engine.
	Image string

	// Cmd overrides the component's default start command. Empty means
	// use whatever the image declares.
	Cmd []string

	// Env is the environment variables to inject. Secrets have already
	// been resolved into plain values here — the engine neither knows nor
	// cares which of these came from the secret store.
	Env map[string]string

	// Ports the component listens on inside its own environment. Mapping
	// these to the outside world is the network layer's job, not the
	// engine's.
	Ports []int

	// Mounts are persistent storage attachments — this is how a database's
	// data survives the component being stopped and recreated.
	Mounts []Mount

	// Resources caps CPU and memory. An engine that cannot enforce a
	// given limit should say so rather than silently ignore it.
	Resources Resources

	// Hardened requests production security posture: non-root user,
	// read-only root filesystem, dropped capabilities. Engines apply this
	// differently, which is exactly why it is a request, not a mechanism.
	Hardened bool

	WorkDir string // working directory inside the container; empty = default (/)
}

// Mount attaches persistent storage into a component.
type Mount struct {
	// Source is the storage identifier on the host side.
	Source string

	// Target is the path inside the component where it appears.
	Target string

	// ReadOnly mounts the storage without write access when true.
	ReadOnly bool
}

// Resources caps what a component may consume. These are intent; the
// engine maps them onto whatever its backend uses (cgroups, VM limits,
// and so on).
type Resources struct {
	// CPU in cores. 1.0 is one full core, 0.5 is half.
	CPU float64

	// MemoryBytes is the hard memory ceiling. Stored as bytes here — the
	// human-friendly "512MB" parsing happens upstream so every engine
	// receives an unambiguous number.
	MemoryBytes int64
}

// ExecSpec describes a command to run inside a running component.
type ExecSpec struct {
	// Cmd is the command and its arguments, e.g. ["psql", "-U", "app"].
	Cmd []string

	// TTY requests an interactive terminal. True for "fastship shell",
	// false for a one-off command whose output is captured.
	TTY bool

	// Stdin, Stdout, Stderr wire the command up to the caller. For an
	// interactive shell these are the user's own terminal streams.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}
