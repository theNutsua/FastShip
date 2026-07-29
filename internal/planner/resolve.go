package planner

import (
	"fmt"
	"strings"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/pkg/config"
)

// resolvedService is a managed service fully worked out: the spec to run
// it, plus the connection env var the app needs to reach it.
type resolvedService struct {
	Spec       engine.Spec // what to run
	ConnEnvVar string      // e.g. "DATABASE_URL"
	ConnValue  string      // e.g. "postgres://user:pass@postgres:5432/db"
}

// resolveService turns a declared service into a runnable spec, merging a
// preset (if the name has one) with the user's explicit fields, then
// filling in generated credentials.
//
// Three cases:
//   - known service ("- postgres"): the preset supplies everything
//   - custom service (image given, no preset): the user supplies everything
//   - neither preset nor image: an error the user can act on
func resolveService(svc config.Service, appName string, creds credentials) (resolvedService, error) {
	p, hasPreset := presets[svc.Name]

	// Determine the image: user's explicit image wins, else the preset's,
	// else there is nothing to run.
	image := svc.Image
	if image == "" {
		image = p.Image
	}
	if image == "" {
		return resolvedService{}, fmt.Errorf(
			"don't know how to run service %q\n\n"+
				"it's not a known service, so tell me which image to use:\n"+
				"  services:\n"+
				"    - name: %s\n"+
				"      image: some/image:tag\n"+
				"      port: 1234", svc.Name, svc.Name)
	}

	// Port: user's wins, else preset's.
	port := svc.Port
	if port == 0 {
		port = p.Port
	}

	// Data path: user's wins, else preset's. Empty means stateless.
	dataPath := svc.Data
	if dataPath == "" {
		dataPath = p.DataPath
	}

	// Environment: start from the preset's defaults, then layer the user's
	// on top, then fill credential placeholders in the result.
	env := map[string]string{}
	for k, v := range p.Env {
		env[k] = fillPlaceholders(v, creds, svc.Name)
	}
	for k, v := range svc.Env {
		env[k] = v // user overrides win, verbatim
	}

	spec := engine.Spec{
		Name:  svc.Name,
		Image: image,
		Env:   env,
		Ports: portsOf(port),
		Resources: engine.Resources{
			CPU:         1.0,
			MemoryBytes: 512 * 1024 * 1024,
		},
	}

	// A data path means a persistent volume, namespaced per app+service so
	// two apps' databases never collide.
	if dataPath != "" {
		spec.Mounts = []engine.Mount{{
			Source:   fmt.Sprintf("/var/lib/fastship/volumes/%s-%s", appName, svc.Name),
			Target:   dataPath,
			ReadOnly: false,
		}}
	}

	// The connection info the app receives.
	connVar, connVal := connectionInfo(svc, p, hasPreset, creds, port)

	return resolvedService{
		Spec:       spec,
		ConnEnvVar: connVar,
		ConnValue:  connVal,
	}, nil
}

// connectionInfo decides what env var and value the app gets to reach the
// service.
//
//   - preset with a connection format → the rich, correct string
//     (DATABASE_URL=postgres://user:pass@host:5432/db)
//   - no preset → a generic address the app can use
//     (SEARCH_URL=http://search:9200)
//
// Either way the app learns where the service is; the preset just makes it
// richer for services FastShip understands.
func connectionInfo(svc config.Service, p preset, hasPreset bool, creds credentials, port int) (string, string) {
	if hasPreset && p.ConnFormat != "" {
		val := fillPlaceholders(p.ConnFormat, creds, svc.Name)
		return p.ConnEnvVar, val
	}

	// Generic fallback: <NAME>_URL = <scheme>://<host>:<port>
	// http is a reasonable default scheme; the app author knows their
	// service and can use the host:port however they need.
	envVar := strings.ToUpper(svc.Name) + "_URL"
	val := fmt.Sprintf("http://%s:%d", svc.Name, port)
	return envVar, val
}

// fillPlaceholders substitutes credential and host values into a template.
// Placeholders: {{user}} {{pass}} {{db}} {{host}}.
func fillPlaceholders(s string, creds credentials, host string) string {
	r := strings.NewReplacer(
		"{{user}}", creds.User,
		"{{pass}}", creds.Pass,
		"{{db}}", creds.DB,
		"{{host}}", host,
	)
	return r.Replace(s)
}

func portsOf(port int) []int {
	if port > 0 {
		return []int{port}
	}
	return nil
}
