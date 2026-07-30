package build

import (
	"github.com/moby/buildkit/client/llb"
)

// pythonRecipe builds a Python application into a runnable image.
//
// Like Node, Python is interpreted — it needs its runtime present to run,
// so the final image keeps the python runtime plus the app's source and
// installed dependencies. (Contrast Go, which compiles to a standalone
// binary on scratch.)
//
// The build copies the app in and installs dependencies from
// requirements.txt if present. The app then runs via its detected start
// command (e.g. "python app.py") from /app.
func pythonRecipe(srcPath string, imageName string) llb.State {
	// python:alpine has python + pip. Copy the whole app in, then install
	// dependencies. --no-cache-dir keeps the image smaller by not leaving
	// pip's download cache behind.
	built := llb.Image("docker.io/library/python:3.12-alpine").
		Dir("/app").
		File(llb.Copy(llb.Local("context"), ".", "/app")).
		// Install deps if requirements.txt exists. The shell guard means the
		// build does not fail for apps that have no requirements file.
		Run(llb.Shlex(`sh -c "if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi"`)).
		Root()

	return built
}
