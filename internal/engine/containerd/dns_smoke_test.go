package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/theNutsua/FastShip/internal/engine"
)

// TestSmokeDNS proves the DNS server learns a running container's IP.
// It starts nginx (which stays alive), then checks FastShip's DNS has
// registered nginx's name → IP mapping.
//
// Run with: sudo go test ./internal/engine/containerd/ -run SmokeDNS -v
func TestSmokeDNS(t *testing.T) {
	eng, err := New()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	h, err := eng.Start(ctx, engine.Spec{
		Name:  "dnstest",
		Image: "docker.io/library/nginx:alpine",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer eng.Stop(ctx, h, 5*time.Second)

	time.Sleep(2 * time.Second)

	// The DNS server should now know "dnstest" → its IP.
	ip, ok := eng.dns.Lookup("dnstest")
	if !ok {
		t.Fatal("dnstest was not registered in DNS")
	}
	t.Logf("DNS resolved dnstest → %s", ip)

	// It should be a bridge IP.
	if ip == "" {
		t.Error("empty IP registered")
	}
}

// TestSmokeDNSResolve proves the DNS server answers real queries over the
// network — not just that the table has the entry, but that a DNS client
// can look up a name and get the IP back.
//
// This runs entirely while the test process is alive, which is why it
// works: the DNS server lives inside this process. In real usage the
// daemon (shipd) will host the server so it outlives any single command.
//
// Run with: sudo go test ./internal/engine/containerd/ -run SmokeDNSResolve -v
func TestSmokeDNSResolve(t *testing.T) {
	eng, err := New()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer eng.Close()

	ctx := context.Background()

	h, err := eng.Start(ctx, engine.Spec{
		Name:  "resolvetest",
		Image: "docker.io/library/nginx:alpine",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer eng.Stop(ctx, h, 5*time.Second)

	time.Sleep(2 * time.Second)

	// Query the DNS server over the wire, the way a container would.
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("resolvetest.", dns.TypeA)

	resp, _, err := c.Exchange(m, "10.88.0.1:53")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if len(resp.Answer) == 0 {
		t.Fatal("no answer for resolvetest")
	}

	t.Logf("DNS answered over the wire: %s", resp.Answer[0].String())
}
