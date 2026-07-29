package planner

import (
	"testing"

	"github.com/theNutsua/FastShip/pkg/config"
)

func TestBuildSimpleApp(t *testing.T) {
	cfg := &config.Config{
		Name:      "myapp",
		Runtime:   "node@20",
		Port:      3000,
		Resources: config.Resources{CPU: 1.0, Memory: "512MB"},
	}

	plan, err := Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// One component: the app itself.
	if len(plan.Specs) != 1 {
		t.Fatalf("got %d specs, want 1", len(plan.Specs))
	}

	spec := plan.Specs[0]
	if spec.Name != "myapp" {
		t.Errorf("name = %q, want myapp", spec.Name)
	}
	if spec.Image != "docker.io/library/node:20-alpine" {
		t.Errorf("image = %q, want node:20-alpine", spec.Image)
	}
	// PORT should be injected so the app binds where FastShip expects.
	if spec.Env["PORT"] != "3000" {
		t.Errorf("PORT = %q, want 3000", spec.Env["PORT"])
	}
	// 512MB should have become bytes.
	if spec.Resources.MemoryBytes != 512*1024*1024 {
		t.Errorf("memory = %d bytes, want %d", spec.Resources.MemoryBytes, 512*1024*1024)
	}
}

// The important ordering test: a managed service must be planned BEFORE
// the app, because the app depends on it being up.
func TestBuildServiceOrdering(t *testing.T) {
	cfg := &config.Config{
		Name:      "myapp",
		Runtime:   "node@20",
		Port:      3000,
		Resources: config.Resources{CPU: 1.0, Memory: "512MB"},
		Services: []config.Service{
			{Name: "postgres"},
		},
	}

	plan, err := Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(plan.Specs))
	}

	// postgres first, app second.
	if plan.Specs[0].Name != "postgres" {
		t.Errorf("first spec = %q, want postgres", plan.Specs[0].Name)
	}
	if plan.Specs[1].Name != "myapp" {
		t.Errorf("second spec = %q, want myapp", plan.Specs[1].Name)
	}

	// The app should have DATABASE_URL pointing at the postgres hostname.
	appSpec := plan.Specs[1]
	dbURL := appSpec.Env["DATABASE_URL"]
	if dbURL == "" {
		t.Fatal("DATABASE_URL was not injected into the app")
	}
	if !contains(dbURL, "@postgres:5432") {
		t.Errorf("DATABASE_URL = %q, want it to point at the postgres host", dbURL)
	}
}

// External services start nothing but still inject their URL.
func TestBuildExternalService(t *testing.T) {
	cfg := &config.Config{
		Name:      "myapp",
		Runtime:   "node@20",
		Port:      3000,
		Resources: config.Resources{CPU: 1.0, Memory: "512MB"},
		Services: []config.Service{
			{Name: "payments", URL: "https://api.stripe.com"},
		},
	}

	plan, err := Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the app — the external service starts nothing.
	if len(plan.Specs) != 1 {
		t.Fatalf("got %d specs, want 1 (external service starts nothing)", len(plan.Specs))
	}

	// But its URL is injected.
	if plan.Specs[0].Env["PAYMENTS_URL"] != "https://api.stripe.com" {
		t.Errorf("PAYMENTS_URL = %q, want the stripe url",
			plan.Specs[0].Env["PAYMENTS_URL"])
	}
}

func TestParseMemory(t *testing.T) {
	cases := map[string]int64{
		"512MB": 512 * 1024 * 1024,
		"1GB":   1024 * 1024 * 1024,
		"256MB": 256 * 1024 * 1024,
	}
	for input, want := range cases {
		got, err := parseMemory(input)
		if err != nil {
			t.Errorf("%s: unexpected error %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %d bytes, want %d", input, got, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(sub) == 0 ||
		indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
