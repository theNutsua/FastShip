package build

import _ "embed"

// staticServerBinary is FastShip's pre-compiled static file server, embedded
// into shipd at build time. Static apps (React, Vue) get this binary baked
// into their image to serve the built files. Embedding means shipd carries
// it — no repo or separate download needed at runtime.
//
//go:embed staticserver/staticserver-bin
var staticServerBinary []byte
