package network

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	d := NewDNS("10.88.0.1:53")

	d.Register("postgres", "10.88.0.3")
	d.Register("myapp", "10.88.0.2")

	ip, ok := d.lookup("postgres")
	if !ok {
		t.Fatal("postgres should be registered")
	}
	if ip != "10.88.0.3" {
		t.Errorf("postgres = %q, want 10.88.0.3", ip)
	}

	// Unknown name should not resolve.
	if _, ok := d.lookup("nonexistent"); ok {
		t.Error("nonexistent should not resolve")
	}
}

func TestDeregister(t *testing.T) {
	d := NewDNS("10.88.0.1:53")

	d.Register("postgres", "10.88.0.3")
	d.Deregister("postgres")

	// After deregister, the name must not resolve — otherwise it would
	// point at a dead container.
	if _, ok := d.lookup("postgres"); ok {
		t.Error("postgres should not resolve after deregister")
	}
}

func TestValidIP(t *testing.T) {
	if !validIP("10.88.0.3") {
		t.Error("10.88.0.3 should be valid")
	}
	if validIP("not-an-ip") {
		t.Error("garbage should be rejected")
	}
}
