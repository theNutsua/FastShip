package build

import (
	"github.com/moby/buildkit/client/llb"
)

// staticRecipe builds a static frontend (React, Vue) into an image that
// serves the built files with FastShip's embedded static server.
//
//  1. build the frontend (npm install + the user's build commands)
//  2. final image = alpine + the static server binary + the built files
//
// The static server binary is written into the build context by the build
// engine (from the copy embedded in shipd), so the recipe just copies it
// like any other file — no compile step needed.
func staticRecipe(buildCmds []string, staticDir string) llb.State {
	// Build the frontend in node.
	frontend := llb.Image("docker.io/library/node:20-alpine").
		Dir("/app").
		File(llb.Copy(llb.Local("context"), ".", "/app")).
		Run(llb.Shlex("npm install")).Root()

	for _, cmd := range buildCmds {
		frontend = frontend.Run(llb.Shlex(`sh -c "` + cmd + `"`)).Root()
	}

	// Final image: alpine + the static server + the built files.
	// The server binary was placed in the context at .fastship-staticserver
	// by the build engine.
	final := llb.Image("docker.io/library/alpine:3.20").
		File(llb.Copy(llb.Local("context"), ".fastship-staticserver", "/staticserver")).
		File(llb.Copy(frontend, "/app/"+staticDir, "/static"))

	return final
}
