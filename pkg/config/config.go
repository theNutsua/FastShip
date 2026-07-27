// Package config defines the fastship.yaml schema and everything needed
// to load, validate, and apply defaults to it.
// This package is the single source of truth for what an engineer can
// write in ship.yaml. Every other subsystem build, runtime, network,
// secrets, scale receives a typed *Config from here and never touches
// raw YAML itself.
//
// Design rule: if FastShip can figure something out on its own, the field
// is optional. Empty values here mean "detect it", not "error".
package config

// Config is one application as declared in ship.yaml.
// The same struct serves both a single-app project and each entry under
// the Apps map in a monorepo, which is why it is self-contained.
type Config struct {
	// Name is the app identifier. It becomes the container name, the DNS
	// hostname other services use to reach it, and the key for state.
	// Required.
	Name string `yaml:"name"`

	// Runtime pins the language and version, e.g. "node@18".
	// Optional when empty, pkg/detect scans the repo and infers it from
	// package.json, go.mod, requirements.txt, and so on.
	Runtime string `yaml:"runtime"`

	// Start overrides the command used to launch the app.
	// Optional when empty it is read from the runtime's own convention,
	// e.g. the "start" script in package.json.
	Start string `yaml:"start"`

	// Port is the port the app listens on inside the container.
	// Optional when zero, detection scans source for a listen call.
	// A zero port means the service is internal only and gets no route.
	Port int `yaml:"port"`

	// Path is the subdirectory for this app in a monorepo, e.g. "./api".
	// Ignored for single-app projects.
	Path string `yaml:"path"`

	// Services are dependencies this app needs. Two kinds:
	// managed (FastShip runs postgres for you) and external (FastShip just
	// injects a URL). See the Service type for how both parse.
	Services []Service `yaml:"services"`

	// Secrets lists secret names this app may read. Values never appear
	// here only names. Scoping is per-app, so a secret listed by the
	// worker is not readable by the api.
	Secrets []Secret `yaml:"secrets"`

	// Scale controls instance counts and drain behavior.
	Scale Scale `yaml:"scale"`

	// Env holds plain, non-sensitive environment variables.
	// Anything sensitive belongs in Secrets, not here.
	Env map[string]string `yaml:"env"`

	// Resources caps CPU and memory. Always set defaults are applied
	// when omitted so no container ever runs unlimited.
	Resources Resources `yaml:"resources"`

	// DependsOn names other apps that must be running first.
	// Only meaningful in a monorepo.
	DependsOn []string `yaml:"depends_on"`

	// Apps holds a monorepo's sub-applications keyed by name.
	// When this is non-empty the outer Config is treated as a container
	// for the others rather than an app in its own right.
	Apps map[string]*Config `yaml:"apps"`
}

// Service is a dependency. It accepts two YAML shapes:
//
//	services:
//	  - postgres                        # managed: FastShip runs it
//	  - name: payments                  # external: FastShip injects the URL
//	    url: https://api.stripe.com
//
// The custom unmarshaler below is what allows both forms.
type Service struct {
	// Name is the service identifier and its DNS hostname.
	Name string `yaml:"name"`

	// URL, when set, marks this as an external service. FastShip does not
	// start or manage it, it only injects the address into the app.
	URL string `yaml:"url"`
}

// Managed reports whether FastShip owns this service's lifecycle.
// External services (those with a URL) are not started, stopped, or
// health-checked by FastShip.
func (s Service) Managed() bool {
	return s.URL == ""
}

// Secret is a reference to a secret value. Two YAML shapes:
//
//	secrets:
//	  - JWT_SECRET                      # FastShip's own encrypted store
//	  - name: DATABASE_URL              # external provider (Phase 2)
//	    from: aws-secrets-manager
type Secret struct {
	// Name is the environment variable the value is injected as.
	Name string `yaml:"name"`

	// From names an external secret provider. Empty means FastShip's own
	// encrypted store. Phase 2 adds aws-secrets-manager, vault, 1password.
	From string `yaml:"from"`
}

// Scale declares intent. FastShip decides when and how to act on it.
type Scale struct {
	// Min is the floor. Production defaults to 2 so a single instance
	// dying never takes the app down.
	Min *int `yaml:"min"`

	// Max is the ceiling for auto-scaling.
	Max int `yaml:"max"`

	// DrainTimeout is how long to let in-flight requests finish before
	// terminating an instance during scale-down. Written as "30s".
	DrainTimeout string `yaml:"drain_timeout"`
}

// Resources caps what a container may consume. These map to Linux
// cgroup limits at runtime — the mechanism that stops one container
// starving the whole machine.
type Resources struct {
	// CPU in cores. 1 means one full core, 0.5 means half.
	CPU float64 `yaml:"cpu"`

	// Memory as a human string, e.g. "512MB" or "1GB".
	Memory string `yaml:"memory"`
}

// IsMonorepo reports whether this config declares sub-applications
// rather than being a single app itself.
func (c *Config) IsMonorepo() bool {
	return len(c.Apps) > 0
}
