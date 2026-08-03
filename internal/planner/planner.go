// Package planner turns a parsed fastship.yaml configuration into concrete
// engine specs the runtime can execute.
// It sits between the declarative world (config: what the engineer wants)
// and the execution world (engine.Spec: what actually runs). This is the
// one place that decides HOW a declaration becomes a running component —
// which image, which env vars, which resource limits.
// The planner never talks to an engine or the kernel. It is pure
// translation: Config in, []engine.Spec out. That keeps it testable
// without containerd and keeps the "decide what to run" logic separate
// from the "actually run it" logic.
package planner

import (
	"fmt"
	"strings"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/secrets"
	"github.com/theNutsua/FastShip/pkg/config"
)

// Plan is the full set of components to run for one application, in the
// order they should start. Ordering matters: a database must be running
// before the app that depends on it.
type Plan struct {
	// Specs are the components to start, already ordered so dependencies
	// come before the things that need them.
	Specs []engine.Spec
}

// Build turns a validated Config into an ordered Plan.
// The Config must already have defaults applied and detection run — the
// planner assumes Runtime, Port, and Resources are populated. Its job is
// translation, not inference.
func Build(cfg *config.Config) (*Plan, error) {
	plan := &Plan{}

	// Managed services first, so they are up before the app that needs
	// them. Each service's connection info is collected to inject into the
	// app below.
	serviceConns := map[string]string{}
	for _, svc := range cfg.Services {
		if !svc.Managed() {
			serviceConns[strings.ToUpper(svc.Name)+"_URL"] = svc.URL
			continue
		}

		// Load existing credentials, or generate and persist new ones.
		// This is what makes the password STABLE across runs — postgres
		// initializes its data with these, and later runs reuse the same
		// ones so authentication keeps working.
		creds, err := loadOrCreateCredentials(cfg.Name, svc.Name)
		if err != nil {
			return nil, err
		}

		resolved, err := resolveService(svc, cfg.Name, creds)
		if err != nil {
			return nil, err
		}

		plan.Specs = append(plan.Specs, resolved.Spec)
		serviceConns[resolved.ConnEnvVar] = resolved.ConnValue
	}

	// The app last, with every service's connection info injected.
	appSpec, err := planApp(cfg, serviceConns)
	if err != nil {
		return nil, err
	}
	plan.Specs = append(plan.Specs, appSpec)
	return plan, nil
}

// planApp builds the engine spec for the main application component.
func planApp(cfg *config.Config, serviceConns map[string]string) (engine.Spec, error) {
	// Resolve the runtime string ("node@20") into a base image. In Phase 1
	// this maps to a public image; the build engine will later produce a
	// custom optimized image instead.

	// Parse the human memory string ("512MB") into bytes for the engine.
	memBytes, err := parseMemory(cfg.Resources.Memory)
	if err != nil {
		return engine.Spec{}, fmt.Errorf("app %s: %w", cfg.Name, err)
	}

	// Assemble environment. Start with the plain env vars the engineer
	// declared, then layer in the connection strings for managed services.
	env := map[string]string{}
	for k, v := range cfg.Env {
		env[k] = v
	}

	// FastShip injects PORT so apps that read process.env.PORT bind to the
	// port FastShip expects, rather than FastShip guessing where they bound.
	if cfg.Port > 0 {
		env["PORT"] = fmt.Sprintf("%d", cfg.Port)
	}
	//// Managed services contribute connection-string env vars. External
	//// services contribute their URL.
	//for _, svc := range cfg.Services {
	//	env[serviceEnvVar(svc)] = serviceConnectionString(svc, cfg.Name)
	//}

	// Inject every service's connection info, already resolved.
	for k, v := range serviceConns {
		env[k] = v
	}

	// Ports the app exposes.
	ports := []int{}
	if cfg.Port > 0 {
		ports = append(ports, cfg.Port)
	}

	// Inject declared secrets. The app lists secret NAMES in its config;
	// FastShip looks each up in the encrypted store and injects the value.
	// Values live only in the store — never in config or plaintext state.
	if len(cfg.Secrets) > 0 {
		store, err := secrets.Open()
		if err != nil {
			return engine.Spec{}, fmt.Errorf("opening secret store: %w", err)
		}
		for _, sec := range cfg.Secrets {
			val, ok := store.Get(sec.Name)
			if !ok {
				return engine.Spec{}, fmt.Errorf(
					"app %s needs secret %q, but it is not set\n"+
						"set it with:  fastship secret set %s <value>",
					cfg.Name, sec.Name, sec.Name)
			}
			env[sec.Name] = val
		}
	}
	// The command to run depends on the runtime. The Go recipe compiles a
	// single binary placed at /app, so Go apps run "/app". Other runtimes
	// (Node, Python) keep their source and run an interpreter command that
	// detection already resolved into cfg.Start, e.g. "node server.js".
	var cmd []string
	var workDir string
	if languageOf(cfg.Runtime) == "go" {
		cmd = []string{"/app"}
		// Go binary is at an absolute path; no working dir needed.
	} else {
		cmd = strings.Fields(cfg.Start)
		// Node/Python run a relative command from where their source lives.
		workDir = "/app"
	}

	return engine.Spec{
		Name:    cfg.Name,
		Image:   "",      //filled in by run after the build engine produces a custom image
		Cmd:     cmd,     // the binary the build engine places at /app
		WorkDir: workDir, // the working directory for the process
		Env:     env,
		Ports:   ports,
		Resources: engine.Resources{
			CPU:         cfg.Resources.CPU,
			MemoryBytes: memBytes,
		},
		// Hardening is off for local runs. The run command will flip this
		// on for production deploys — same plan, different posture.
		Hardened: false,
	}, nil
}

// loadOrCreateCredentials returns a service's credentials, generating and
// storing them on first use. They live in the ENCRYPTED secret store, not
// plaintext state — so database passwords are never sitting readable on
// disk. Generated once and reused, so the service's data stays accessible
// across restarts.
func loadOrCreateCredentials(app, service string) (credentials, error) {
	store, err := secrets.Open()
	if err != nil {
		return credentials{}, err
	}

	// Secrets are keyed so each service's credentials are distinct.
	prefix := "svc/" + app + "/" + service + "/"

	user, hasUser := store.Get(prefix + "user")
	pass, hasPass := store.Get(prefix + "pass")
	db, hasDB := store.Get(prefix + "db")

	if hasUser && hasPass && hasDB {
		return credentials{User: user, Pass: pass, DB: db}, nil
	}

	// None (or incomplete) — generate fresh and store encrypted.
	creds := generateCredentials(app)
	store.Set(prefix+"user", creds.User)
	store.Set(prefix+"pass", creds.Pass)
	store.Set(prefix+"db", creds.DB)
	return creds, nil
}

// languageOf strips the version from a runtime string: "go@1.26" → "go".
// A small local copy so the planner can branch on language without
// importing the build package.
func languageOf(runtime string) string {
	if i := strings.IndexByte(runtime, '@'); i >= 0 {
		return runtime[:i]
	}
	return runtime
}

//// planService builds the engine spec for a managed service like postgres.
//func planService(svc config.Service, appName string) (engine.Spec, error) {
//	image, err := serviceToImage(svc.Name)
//	if err != nil {
//		return engine.Spec{}, err
//	}
//
//	env := map[string]string{}
//	switch svc.Name {
//	case "postgres":
//		env["POSTGRES_PASSWORD"] = "changeme"
//		env["POSTGRES_USER"] = "fastship"
//		env["POSTGRES_DB"] = "fastship"
//	}
//
//	spec := engine.Spec{
//		Name:  svc.Name,
//		Image: image,
//		Env:   env,
//		Resources: engine.Resources{
//			CPU:         1.0,
//			MemoryBytes: 512 * 1024 * 1024,
//		},
//	}
//
//	// Stateful services get a persistent volume so their data survives
//	// restarts. The host directory is namespaced by app and service so two
//	// apps' databases never collide.
//	if dataPath, ok := serviceDataPath[svc.Name]; ok {
//		hostPath := fmt.Sprintf("/var/lib/fastship/volumes/%s-%s", appName, svc.Name)
//		spec.Mounts = []engine.Mount{
//			{
//				Source:   hostPath,
//				Target:   dataPath,
//				ReadOnly: false,
//			},
//		}
//	}
//
//	return spec, nil
//}
