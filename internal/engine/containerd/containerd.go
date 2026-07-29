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
	"os"
	"path/filepath"
	"strings"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	fsnet "github.com/theNutsua/FastShip/internal/network"

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
	client  *containerd.Client
	network *network // CNI-backed networking
	dns     *fsnet.DNS
}

// Compile-time proof that this satisfies the interface. If a method
// signature drifts from the interface, this line stops compiling.
var _ engine.Engine = (*Engine)(nil)

// resolvConfPath writes a resolv.conf pointing at FastShip's DNS server
// and returns its path. Containers bind-mount this so their name lookups
// (like "postgres") go to FastShip's DNS on the bridge gateway.
//
// It is written once to a known location and reused by every container.
func resolvConfPath() (string, error) {
	dir := "/run/fastship"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "resolv.conf")

	// nameserver 10.88.0.1 → FastShip's DNS
	// The fallback 8.8.8.8 lets containers still reach the public internet
	// for names FastShip doesn't know (e.g. downloading packages).
	content := "nameserver 10.88.0.1\nnameserver 8.8.8.8\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

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

	// Initialize CNI networking. If this fails, the network config or CNI
	// plugins are missing — a setup problem worth reporting clearly.
	net, err := newNetwork()
	if err != nil {
		client.Close()
		return nil, err
	}

	// Start the DNS server on the bridge gateway so containers can resolve
	// each other by name. The gateway (10.88.0.1) matches the subnet in
	// the CNI config; every container on the bridge can reach it.
	dnsServer := fsnet.NewDNS("10.88.0.1:53")
	if err := dnsServer.Start(); err != nil {
		client.Close()
		return nil, fmt.Errorf("starting DNS: %w", err)
	}

	return &Engine{
		client:  client,
		network: net,
		dns:     dnsServer,
	}, nil
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

	// Reconcile first: if a container with this name already exists from a
	// previous run that did not clean up (a crash, a killed daemon, a
	// failed start), remove it before creating a fresh one. Start should
	// drive toward "this component is running", not assume a clean slate —
	// the same idempotence Stop already has.
	e.cleanupExisting(ctx, spec.Name)

	// 1. Pull the image. containerd caches it, so this is a no-op after
	//    the first pull of a given image.
	// Try the local image store first — a freshly built image is already
	// here and does not need pulling.
	image, err := e.client.GetImage(ctx, spec.Image)
	if err != nil {
		// Not local — pull it. Pull unpacks automatically.
		image, err = e.client.Pull(ctx, spec.Image, containerd.WithPullUnpack)
		if err != nil {
			return engine.Handle{}, fmt.Errorf("pulling %s: %w", spec.Image, err)
		}
	} else {
		// The image record exists locally, but a built image may not have
		// its filesystem snapshot unpacked yet. Unpack it so a container
		// can be created from it. This is a no-op if already unpacked.
		unpacked, err := image.IsUnpacked(ctx, "overlayfs")
		if err != nil {
			return engine.Handle{}, fmt.Errorf("checking image unpack: %w", err)
		}
		if !unpacked {
			if err := image.Unpack(ctx, "overlayfs"); err != nil {
				return engine.Handle{}, fmt.Errorf("unpacking %s: %w", spec.Image, err)
			}
		}
	}

	// 2. Build the OCI runtime spec — the low-level description of the
	//    process, its environment, and its isolation. oci.WithImageConfig
	//    pulls sensible defaults (entrypoint, env, working dir) straight
	//    from the image's own metadata.
	opts := []oci.SpecOpts{
		oci.WithImageConfig(image),
	}

	// Point the container's DNS at FastShip's resolver by bind-mounting a
	// resolv.conf that lists the bridge gateway.
	resolvPath, err := resolvConfPath()
	if err != nil {
		return engine.Handle{}, fmt.Errorf("preparing DNS config: %w", err)
	}
	opts = append(opts, oci.WithMounts([]specs.Mount{
		{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      resolvPath,
			Options:     []string{"rbind", "ro"},
		},
	}))

	// Ensure volume host directories exist before mounting them. containerd
	// will not create them, and a bind mount of a missing directory fails.
	for _, m := range spec.Mounts {
		if err := os.MkdirAll(m.Source, 0755); err != nil {
			return engine.Handle{}, fmt.Errorf("creating volume dir %s: %w", m.Source, err)
		}
	}

	// Apply persistent volume mounts. This is what makes a database's data
	// survive the container being destroyed and recreated — the mount
	// points inside the container at a host directory that outlives it.
	for _, m := range spec.Mounts {
		opts = append(opts, oci.WithMounts([]specs.Mount{
			{
				Destination: m.Target,
				Type:        "bind",
				Source:      m.Source,
				Options:     mountOptions(m.ReadOnly),
			},
		}))
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

	// Attach the container to FastShip's network BEFORE starting it, so it
	// has an IP and can reach other services the moment it runs.
	//
	// The netns path comes from the task's process. containerd exposes it
	// at a predictable path based on the task's PID.
	netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
	ip, err := e.network.attach(ctx, spec.Name, netnsPath)
	if err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return engine.Handle{}, fmt.Errorf("networking %s: %w", spec.Name, err)
	}
	fmt.Printf("  %s got IP %s\n", spec.Name, ip)

	// Register this component in DNS so other components can reach it by
	// name. This is what makes "postgres" resolve to the postgres IP.
	e.dns.Register(spec.Name, ip)

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

// Stop kills a container's task (if still running) and removes the
// container and its snapshot.
//
// A container's process may have already exited on its own — a short-lived
// job, or an app that crashed. That is not an error for Stop: the goal is
// "this component is gone and cleaned up", and a process that already
// finished is halfway there. So a missing or dead task is fine; what must
// always happen is the container and snapshot deletion.
func (e *Engine) Stop(ctx context.Context, h engine.Handle, drain time.Duration) error {
	ctx = withNamespace(ctx)

	container, err := e.client.LoadContainer(ctx, h.ID)
	if err != nil {
		// No container at all — nothing to clean up. Treat as success so
		// stopping something already gone is not an error.
		return nil
	}

	// Try to stop the running process, if there is one. Every failure here
	// is non-fatal: the task may already be dead, which is exactly the
	// state we are trying to reach.
	if task, err := container.Task(ctx, nil); err == nil {
		// Detach from the network before tearing down the task.
		netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
		err := e.network.detach(ctx, h.ID, netnsPath)
		if err != nil {
			return err
		}

		// Remove from DNS so the name no longer resolves to a dead container.
		e.dns.Deregister(h.Name)

		// Arm the exit wait before killing so we cannot miss the exit.
		exitCh, waitErr := task.Wait(ctx)

		// Kill it. If it is already finished, that is fine — ignore.
		task.Kill(ctx, 9)

		// Only wait if arming the wait succeeded and the task was alive.
		if waitErr == nil {
			select {
			case <-exitCh:
				// exited
			case <-time.After(10 * time.Second):
				// give up waiting; proceed to delete anyway
			}
		}

		// Delete the task. Ignore errors — if it is already gone, good.
		task.Delete(ctx)
	}

	// This is the part that MUST always run: remove the container and its
	// snapshot. This is what was being skipped when the task was already
	// dead, leaving the orphan snapshot that blocked the next run.
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

// withFastShipDNS configures a container to use FastShip's DNS server.
//
// It writes a resolv.conf pointing at the bridge gateway (10.88.0.1),
// where FastShip's DNS listens. Without this, the container would use the
// host's DNS and never find names like "postgres".
func withFastShipDNS() oci.SpecOpts {
	return oci.WithEnv([]string{}) // placeholder — see note below
}

// cleanupExisting removes any leftover container, task, and snapshot for a
// name, so Start can recreate cleanly. Every step is best-effort: if a
// piece is already gone, that is fine — the goal is simply that nothing
// with this name remains.
func (e *Engine) cleanupExisting(ctx context.Context, name string) {
	container, err := e.client.LoadContainer(ctx, name)
	if err != nil {
		return // nothing here — already clean
	}

	// Kill and delete any running task.
	if task, err := container.Task(ctx, nil); err == nil {
		task.Kill(ctx, 9)
		// Best-effort wait so delete does not race the kill.
		exitCh, _ := task.Wait(ctx)
		select {
		case <-exitCh:
		case <-time.After(5 * time.Second):
		}
		task.Delete(ctx)
	}

	// Delete the container and its snapshot.
	container.Delete(ctx, containerd.WithSnapshotCleanup)

	// Also release any leaked CNI IP for this name, and deregister from DNS
	// was already gone — the IP allocation can outlive the container.
	// (The netns is gone by now, so we clean the allocation record directly.)
	releaseCNIIPs(name)
	e.dns.Deregister(name)
}

// releaseCNIIPs removes any leaked CNI IP allocation for a container name.
//
// CNI records each allocated IP as a file under this directory, named by
// the IP, containing the name of the container that holds it. When a
// container crashes without detaching, the record leaks and CNI refuses
// to reissue that IP. This finds and removes records for the given name.
func releaseCNIIPs(name string) {
	dir := "/var/lib/cni/networks/fastship"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no allocations dir — nothing to clean
	}

	for _, entry := range entries {
		// Skip the bookkeeping files, only look at IP-named allocation files.
		if entry.Name() == "lock" || strings.HasPrefix(entry.Name(), "last_reserved") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// The file contains the owning container's name (possibly with a
		// trailing newline or extra data). If it matches, this IP is ours
		// to release.
		if strings.Contains(string(data), name) {
			os.Remove(path)
		}
	}
}

// mountOptions returns the bind-mount options. rbind makes it a recursive
// bind; ro adds read-only when requested.
func mountOptions(readOnly bool) []string {
	if readOnly {
		return []string{"rbind", "ro"}
	}
	return []string{"rbind", "rw"}
}
