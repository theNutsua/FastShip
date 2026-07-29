// Package main is the entry point for shipd, the FastShip daemon.
//
// shipd is the long-running process that hosts everything which must
// outlive a single CLI command: the container engine, the DNS server,
// network state. The fastship CLI is a thin client that sends requests
// to this daemon over a local Unix socket.
//
// This split is why the DNS server can stay alive — it lives here, in a
// process that keeps running, not in the short-lived CLI.
package main

import (
	"log"

	"github.com/theNutsua/FastShip/internal/daemon"
)

func main() {
	// Run blocks, serving requests until the daemon is shut down.
	if err := daemon.Run(); err != nil {
		log.Fatalf("shipd: %v", err)
	}
}
