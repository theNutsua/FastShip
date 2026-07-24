package config

// Default values applied when a field is omitted.
// These are deliberately conservative. A container with no memory limit
// can take down a machine, so FastShip always sets one.
const (
	defaultScaleMin     = 1
	defaultScaleMax     = 1
	defaultDrainTimeout = "30s"
	defaultCPU          = 1.0
	defaultMemory       = "512MB"
)

// applyDefaults fills in anything the engineer left out.
// Note what is NOT defaulted here: Runtime, Start, and Port. Those stay
// empty on purpose — empty means "detect it", and detection happens
// later in pkg/detect once the repo is available to scan.
func (c *Config) applyDefaults() {
	// A monorepo's outer config is just a container for its children,
	// so recurse into each app and leave the parent alone.
	if c.IsMonorepo() {
		for name, app := range c.Apps {
			// The map key is the app name, so an engineer never has to
			// repeat it inside the block.
			if app.Name == "" {
				app.Name = name
			}
			app.applyDefaults()
		}
		return
	}

	if c.Scale.Min == 0 {
		c.Scale.Min = defaultScaleMin
	}
	if c.Scale.Max == 0 {
		// Max defaults to Min, not to some arbitrary ceiling. Auto-scaling
		// should never be a surprise the engineer did not ask for.
		c.Scale.Max = maxInt(c.Scale.Min, defaultScaleMax)
	}
	if c.Scale.DrainTimeout == "" {
		c.Scale.DrainTimeout = defaultDrainTimeout
	}

	if c.Resources.CPU == 0 {
		c.Resources.CPU = defaultCPU
	}
	if c.Resources.Memory == "" {
		c.Resources.Memory = defaultMemory
	}

	// Env is read and written by several subsystems. Allocating it here
	// means none of them need a nil check.
	if c.Env == nil {
		c.Env = map[string]string{}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
