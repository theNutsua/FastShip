package config

import "fmt"

// Validate checks that a config can actually be run.
// It runs after defaults are applied, so it judges the config as
// FastShip will execute it. Errors name the offending field and say
// what to do — a validation error the engineer cannot act on is a bug.
func (c *Config) Validate() error {
	if c.IsMonorepo() {
		for name, app := range c.Apps {
			if err := app.Validate(); err != nil {
				// Prefix with the app name so the engineer knows which
				// block in a long ship.yaml is broken.
				return fmt.Errorf("app %q: %w", name, err)
			}
		}
		return nil
	}

	if c.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Port 0 is legal — it means an internal service with no route.
	// Anything outside the valid TCP range is a typo.
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port %d is out of range (0-65535)", c.Port)
	}

	if c.Scale.Min < 0 {
		return fmt.Errorf("scale.min cannot be negative")
	}
	if c.Scale.Max < c.Scale.Min {
		return fmt.Errorf("scale.max (%d) cannot be less than scale.min (%d)",
			c.Scale.Max, c.Scale.Min)
	}

	// Catch duplicate service names early. Two services with the same name
	// would collide in DNS, and that failure is very confusing at runtime.
	seen := map[string]bool{}
	for _, svc := range c.Services {
		if svc.Name == "" {
			return fmt.Errorf("every service needs a name")
		}
		if seen[svc.Name] {
			return fmt.Errorf("service %q declared more than once", svc.Name)
		}
		seen[svc.Name] = true
	}

	for _, sec := range c.Secrets {
		if sec.Name == "" {
			return fmt.Errorf("every secret needs a name")
		}
	}

	return nil
}
