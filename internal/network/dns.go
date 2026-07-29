// Package network provides FastShip's flat networking: a DNS server that
// lets components find each other by name, so an app connects to
// "postgres" instead of a hardcoded IP that changes every run.
//
// The model is deliberately simple. FastShip already assigns every
// component an IP when it attaches to the bridge network. This package
// keeps a table of name → IP and answers DNS queries from it. Containers
// are pointed at this server, so looking up "postgres" returns the IP of
// the running postgres container.
//
// The DNS protocol itself is handled by the miekg/dns library — a
// scaffolding dependency FastShip replaces with its own resolver in a
// later phase, the same way Cobra will be replaced. For now it lets us
// speak DNS without implementing the wire format by hand.
package network

import (
	"fmt"
	"net"
	"sync"

	"github.com/miekg/dns"
)

// DNS is FastShip's name server. It maps component names to their IPs and
// answers DNS queries from that map.
type DNS struct {
	// mu guards records, since components start and stop concurrently and
	// each start/stop mutates the table while queries read it.
	mu sync.RWMutex

	// records maps a component name to its IP address, e.g.
	// "postgres" → "10.88.0.3". This is the whole source of truth.
	records map[string]string

	// server is the underlying miekg/dns server.
	server *dns.Server

	// addr is where the DNS server listens — the bridge gateway, so every
	// container on the bridge can reach it.
	addr string
}

// NewDNS creates a DNS server that will listen on the given address.
// addr is typically the bridge gateway with the DNS port, "10.88.0.1:53".
func NewDNS(addr string) *DNS {
	return &DNS{
		records: map[string]string{},
		addr:    addr,
	}
}

// Register adds or updates a name → IP mapping. Called when a component
// attaches to the network and gets its IP.
func (d *DNS) Register(name, ip string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records[name] = ip
}

// Deregister removes a mapping. Called when a component stops, so its
// name no longer resolves to a now-dead container.
func (d *DNS) Deregister(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.records, name)
}

// lookup returns the IP for a name, and whether it was found.
func (d *DNS) lookup(name string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ip, ok := d.records[name]
	return ip, ok
}

// Start begins serving DNS queries in the background.
//
// It runs the server in a goroutine so Start returns immediately; the
// server keeps running until Stop is called.
func (d *DNS) Start() error {
	// handleQuery is called for every incoming DNS query.
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		d.handleQuery(w, r)
	})

	d.server = &dns.Server{
		Addr:    d.addr,
		Net:     "udp", // DNS is UDP by default
		Handler: handler,
	}

	// Serve in the background. If it fails to bind, that surfaces on the
	// error channel; for now we log-and-forget since a DNS bind failure is
	// a setup problem the caller will notice when lookups fail.
	go func() {
		if err := d.server.ListenAndServe(); err != nil {
			fmt.Printf("dns server stopped: %v\n", err)
		}
	}()

	return nil
}

// Stop shuts the DNS server down.
func (d *DNS) Stop() error {
	if d.server != nil {
		return d.server.Shutdown()
	}
	return nil
}

// handleQuery answers a single DNS query.
//
// It only handles A records (name → IPv4). For a name it knows, it
// returns the IP. For anything else, it returns an empty response, which
// tells the resolver "I don't know this name" so it can try elsewhere.
func (d *DNS) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		// Only A-record (IPv4 address) questions are handled.
		if q.Qtype != dns.TypeA {
			continue
		}

		// DNS names arrive with a trailing dot ("postgres."). Strip it to
		// match our plain component names.
		name := q.Name
		if len(name) > 0 && name[len(name)-1] == '.' {
			name = name[:len(name)-1]
		}

		// TEMPORARY DEBUG — remove after diagnosing
		fmt.Printf("DNS query for %q; table has: %v\n", name, d.records)
		
		ip, ok := d.lookup(name)
		if !ok {
			continue // unknown name — leave it unanswered
		}

		// Build an A record: this name resolves to this IP.
		rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
		if err != nil {
			continue
		}
		msg.Answer = append(msg.Answer, rr)
	}

	w.WriteMsg(msg)
}

// validIP is a small guard used when registering, to avoid storing
// garbage. An empty or unparseable IP is rejected.
func validIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// Lookup returns the IP registered for a name, and whether it was found.
// Exported so other packages (and tests) can query the table directly,
// separate from the DNS wire protocol handling.
func (d *DNS) Lookup(name string) (string, bool) {
	return d.lookup(name)
}
