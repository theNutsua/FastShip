package build

import (
	"github.com/moby/buildkit/client/llb"
)

// nodeRecipe builds a Node.js application into a runnable image.
//
// Node differs fundamentally from Go. Go compiles to a standalone binary
// that runs on nothing, so its final image is empty (scratch) plus one
// file. JavaScript is not compiled to a standalone binary — it needs the
// Node runtime present to execute. So the final image must still contain
// Node itself, along with the app's code and its installed dependencies.
//
// The build still separates concerns to keep the image lean:
//  1. Start from node:alpine (has node + npm).
//  2. Copy package files and install dependencies first — this layer is
//     cached and only rebuilds when dependencies change, not on every
//     code edit.
//  3. Copy the application source.
//
// The result is larger than a Go image (the node runtime is ~40MB) but
// still far smaller than a naive full node image with build tooling.
func nodeRecipe(srcPath string, imageName string) llb.State {
	// Start from node:alpine (has node + npm), copy the whole app in, then
	// install dependencies. Copying everything first is simpler and more
	// robust than staging package.json separately; dependency-layer caching
	// is an optimization for later.
	built := llb.Image("docker.io/library/node:20-alpine").
		Dir("/app").
		File(llb.Copy(llb.Local("context"), ".", "/app")).
		Run(llb.Shlex("npm install --omit=dev")).
		Root()

	return built
}
