package build

import (
	"github.com/moby/buildkit/client/llb"
)

// goRecipe builds a Go application into a minimal image.
//
// Two-stage LLB build:
//
//	Stage 1 (builder): golang:alpine has the compiler. Copy source in,
//	compile a static binary. Large but temporary.
//	Stage 2 (final): scratch — completely empty. Copy only the binary in.
//	A static Go binary needs nothing else. Result: a few MB.
func goRecipe(srcPath string, imageName string) llb.State {
	// Stage 1 — the builder.
	// Set the base image and env FIRST, then copy source, then run build.
	// CGO_ENABLED=0 must be set before the build runs, so it goes on the
	// image state, not after the Run.
	builder := llb.Image("docker.io/library/golang:1.26-alpine").
		AddEnv("CGO_ENABLED", "0").
		Dir("/src").
		File(llb.Copy(llb.Local("context"), ".", "/src"))

	// Run the compile. .Run returns an ExecState; .Root() gets us back to
	// a plain State pointing at the container's root filesystem, which now
	// contains /out/app.
	compiled := builder.
		Run(llb.Shlex("go build -o /out/app ./...")).
		Root()

	// Stage 2 — the final image. scratch is empty.
	// Copy ONLY /out/app from the compiled stage.
	final := llb.Scratch().
		File(llb.Copy(compiled, "/out/app", "/app"))

	return final
}
