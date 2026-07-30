package build

import (
	"github.com/moby/buildkit/client/llb"
)

// recipe describes how to build one language into a minimal image.
//
// A recipe is a function that produces an LLB state — BuildKit's
// representation of a build graph. The graph encodes the whole
// multi-stage build: compile in a fat image, copy the result into a tiny
// one. FastShip generates this in code so there is never a Dockerfile.
type recipe func(srcPath string, imageName string) llb.State

// recipes maps a language to its build recipe. Go is the first and only
// one for now; the pattern each recipe follows is identical, so adding
// node and python later is filling in the same shape.
var recipes = map[string]recipe{
	"go":     goRecipe,
	"node":   nodeRecipe,
	"python": pythonRecipe,
}
