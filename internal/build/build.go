// Package build turns an application's source code into a minimal,
// runnable image — without the engineer ever writing a Dockerfile.
//
// This is the piece that makes FastShip's core promise true. Until now
// the runtime has been starting bare base images (golang:alpine) with no
// application inside them. The build engine is what puts the engineer's
// actual compiled code into a container, and does so in the smallest
// possible image.
//
// The central idea is build-time versus run-time (see the Foundations
// doc): compile in a large image that has the full toolchain, then copy
// only the finished result into a tiny image and throw the large one
// away. A ~300MB Go build image becomes a ~10MB run image holding just
// the binary.
//
// The engine talks to BuildKit — the same build engine Docker uses
// internally — through its Go client. FastShip constructs the build
// graph in code (BuildKit calls this LLB) rather than shelling out to
// any Dockerfile.
package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"

	"github.com/theNutsua/FastShip/pkg/config"
)

// Result is what a successful build produces.
type Result struct {
	// ImageRef is the reference the engine can run, e.g.
	// "fastship/myapp:latest". After a build, the planner points the
	// engine at this instead of a bare base image.
	ImageRef string

	// SizeBytes is the final image size, used to show the engineer how
	// small FastShip made their image.
	SizeBytes int64
}

// Builder compiles source into images via BuildKit.
type Builder struct {
	// address is the BuildKit daemon socket.
	address string
}

// New returns a Builder pointed at the local BuildKit daemon.
func New() *Builder {
	return &Builder{
		// The default rootful buildkitd socket. Matches how we started
		// buildkitd during setup.
		address: "unix:///run/buildkit/buildkitd.sock",
	}
}

// Build compiles the source at srcPath into a runnable image for the
// given config, and returns a reference to it.
//
// It chooses one of two paths:
//   - Static apps (config has `static:` set, e.g. React or Vue) build to
//     plain files served by FastShip's built-in static server. This path
//     is language-independent — a static app is served, not run.
//   - Everything else selects a recipe by language (Go, Node, Python),
//     which knows how to compile that language into a minimal image.
func (b *Builder) Build(ctx context.Context, srcPath string, cfg *config.Config) (*Result, error) {
	imageRef := "fastship/" + cfg.Name + ":latest"

	// Decide the build graph. A static app is served by FastShip's static
	// server, so it uses the static recipe regardless of language.
	var state llb.State
	if cfg.Static != "" {
		state = staticRecipe(cfg.Build, cfg.Static)
	} else {
		lang := languageOf(cfg.Runtime)
		recipe, ok := recipes[lang]
		if !ok {
			return nil, fmt.Errorf(
				"no build recipe for runtime %q\n\nsupported for building: go, node, python", lang)
		}
		state = recipe(srcPath, imageRef)
	}

	// run() needs to know if this is a static build, so it can stage the
	// embedded static server binary into the build context.
	if err := b.run(ctx, state, srcPath, imageRef, cfg.Static != ""); err != nil {
		return nil, err
	}

	return &Result{ImageRef: imageRef}, nil
}

// run solves an LLB build graph through BuildKit and exports the result
// as an image into containerd's image store.
//
// This is the core BuildKit plumbing. Three things happen:
//  1. Marshal the LLB state into a definition BuildKit understands.
//  2. Solve it — BuildKit actually runs the build (pull, compile, copy).
//  3. Export the result as an image containerd can then run.
//
// The build runs in one goroutine while a second streams progress to the
// terminal, which is why errgroup coordinates them.
//
// When isStatic is true, FastShip's embedded static server binary is
// written into the build context first (and removed after), so the static
// recipe can copy it into the final image without a separate build stage.
func (b *Builder) run(ctx context.Context, state llb.State, srcPath, imageRef string, isStatic bool) error {
	c, err := bkclient.New(ctx, b.address)
	if err != nil {
		return fmt.Errorf("connecting to buildkit: %w", err)
	}
	defer c.Close()

	// For static apps, drop the embedded static server binary into the
	// build context so the static recipe can copy it into the final image.
	// It is written only for this build and removed afterward, so the
	// engineer's source directory is not littered. The defer runs even if
	// the build fails.
	if isStatic {
		serverPath := filepath.Join(srcPath, ".fastship-staticserver")
		if err := os.WriteFile(serverPath, staticServerBinary, 0o755); err != nil {
			return fmt.Errorf("staging static server: %w", err)
		}
		defer os.Remove(serverPath)
	}

	def, err := state.Marshal(ctx, llb.LinuxAmd64)
	if err != nil {
		return fmt.Errorf("marshalling build graph: %w", err)
	}

	// Wrap the source directory into an fsutil.FS — newer BuildKit needs
	// this rather than a plain path string. This MUST come before solveOpt
	// because solveOpt references contextFS.
	contextFS, err := fsutil.NewFS(srcPath)
	if err != nil {
		return fmt.Errorf("preparing build context: %w", err)
	}

	solveOpt := bkclient.SolveOpt{
		LocalMounts: map[string]fsutil.FS{
			"context": contextFS,
		},
		Exports: []bkclient.ExportEntry{
			{
				Type: bkclient.ExporterImage,
				Attrs: map[string]string{
					"name": imageRef,
				},
			},
		},
	}

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *bkclient.SolveStatus)

	eg.Go(func() error {
		_, err := c.Solve(ctx, def, solveOpt, ch)
		if err != nil {
			return fmt.Errorf("solving build: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		d, err := progressui.NewDisplay(os.Stderr, progressui.PlainMode)
		if err != nil {
			return err
		}
		_, err = d.UpdateFrom(ctx, ch)
		return err
	})

	return eg.Wait()
}

// languageOf strips the version from a runtime string: "go@1.26" → "go".
func languageOf(runtime string) string {
	for i := 0; i < len(runtime); i++ {
		if runtime[i] == '@' {
			return runtime[:i]
		}
	}
	return runtime
}
