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
//
// goRecipe builds a Go application into a minimal image.
func goRecipe(srcPath string, imageName string) llb.State {
	// Stage 1 — the builder. golang:alpine has the Go toolchain at
	// /usr/local/go/bin. We must preserve that on PATH for the build step.
	builder := llb.Image("docker.io/library/golang:1.26-alpine").
		AddEnv("PATH", "/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		AddEnv("CGO_ENABLED", "0").
		File(llb.Copy(llb.Local("context"), ".", "/src"))

	// The build script:
	//   1. go list finds the main package's directory
	//   2. if none is found, fail with a clear message
	//   3. build that package to /out/app
	//
	// Written as a shell script so the package discovery happens inside the
	// build container, where the full source and Go toolchain are present.
	buildScript := `set -e
MAIN=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... | head -1)
if [ -z "$MAIN" ]; then
  echo "no main package found — is this a runnable Go application?" >&2
  exit 1
fi
echo "building main package: $MAIN"
go build -o /out/app "$MAIN"
`

	// Run the compile from /src. Setting Dir here (on the Run) rather than
	// on the base state ensures the working directory applies to this exec.
	compiled := builder.
		Dir("/src").
		Run(llb.Args([]string{"sh", "-c", buildScript})).
		Root()

	// Stage 2 — the final image from scratch, holding only the binary.
	final := llb.Scratch().
		File(llb.Copy(compiled, "/out/app", "/app"))

	return final
}
