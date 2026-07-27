package config

import "testing"

// TestParseMinimal covers the config most engineers will actually write.
func TestParseMinimal(t *testing.T) {
	yaml := `
name: myapp
port: 3000
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "myapp" {
		t.Errorf("name = %q, want myapp", cfg.Name)
	}
	if cfg.Port != 3000 {
		t.Errorf("port = %d, want 3000", cfg.Port)
	}

	// Defaults should have been applied.
	if *cfg.Scale.Min != 1 {
		t.Errorf("scale.min = %d, want 1", cfg.Scale.Min)
	}
	if cfg.Resources.Memory != "512MB" {
		t.Errorf("memory = %q, want 512MB", cfg.Resources.Memory)
	}

	// Runtime must stay empty — empty means "detect it later".
	if cfg.Runtime != "" {
		t.Errorf("runtime = %q, want empty for auto-detection", cfg.Runtime)
	}
}

// TestServiceBothForms is the important one: the polymorphic unmarshaler
// is the only clever code in this package, so it needs the most coverage.
func TestServiceBothForms(t *testing.T) {
	yaml := `
name: myapp
services:
  - postgres
  - name: payments
    url: https://api.stripe.com
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(cfg.Services))
	}

	// Bare string form → managed service.
	if cfg.Services[0].Name != "postgres" {
		t.Errorf("services[0].name = %q, want postgres", cfg.Services[0].Name)
	}
	if !cfg.Services[0].Managed() {
		t.Error("postgres should be managed by FastShip")
	}

	// Object form → external service.
	if cfg.Services[1].Name != "payments" {
		t.Errorf("services[1].name = %q, want payments", cfg.Services[1].Name)
	}
	if cfg.Services[1].Managed() {
		t.Error("payments has a URL, so it should not be managed")
	}
}

func TestSecretBothForms(t *testing.T) {
	yaml := `
name: myapp
secrets:
  - JWT_SECRET
  - name: DATABASE_URL
    from: aws-secrets-manager
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Secrets[0].Name != "JWT_SECRET" {
		t.Errorf("secrets[0].name = %q, want JWT_SECRET", cfg.Secrets[0].Name)
	}
	if cfg.Secrets[0].From != "" {
		t.Error("JWT_SECRET should use FastShip's own store")
	}
	if cfg.Secrets[1].From != "aws-secrets-manager" {
		t.Errorf("secrets[1].from = %q, want aws-secrets-manager", cfg.Secrets[1].From)
	}
}

func TestMonorepo(t *testing.T) {
	yaml := `
apps:
  api:
    path: ./api
    port: 3000
  worker:
    path: ./worker
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IsMonorepo() {
		t.Fatal("should be detected as a monorepo")
	}

	// The map key should have become the app's name automatically.
	if cfg.Apps["api"].Name != "api" {
		t.Errorf("api name = %q, want api", cfg.Apps["api"].Name)
	}

	// Defaults must reach nested apps too.
	if cfg.Apps["worker"].Resources.Memory != "512MB" {
		t.Error("defaults did not reach nested apps")
	}
}

// TestValidationErrors checks that bad configs fail with useful messages
// rather than blowing up somewhere deep in the runtime.
func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"missing name", `port: 3000`},
		{"port out of range", `{name: app, port: 99999}`},
		{"max below min", `{name: app, scale: {min: 5, max: 2}}`},
		{"duplicate service", `{name: app, services: [postgres, postgres]}`},
		{"bad drain timeout", `{name: app, scale: {drain_timeout: "30 seconds"}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.yaml)); err == nil {
				t.Error("expected an error, got none")
			}
		})
	}
}
