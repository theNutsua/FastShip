// Package cli — client.go is how the CLI talks to the shipd daemon.
//
// The CLI does not run containers itself anymore. It sends requests to the
// daemon over the same Unix socket curl used, and the daemon does the
// work. This file is the plumbing for that conversation.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"
)

const socketPath = "/run/fastship/shipd.sock"

// newClient builds an HTTP client that talks over the Unix socket instead
// of a network address. The DialContext override is what redirects every
// request to the socket file.
func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// post sends a JSON request to the daemon and decodes the JSON reply.
//
// The host in the URL ("localhost") is ignored — the transport always
// dials the socket — but net/http requires a syntactically valid URL.
func post(path string, reqBody any, respBody any) error {
	// Make sure the daemon is running before we try to talk to it.
	if err := ensureDaemon(); err != nil {
		return err
	}

	client := newClient()

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := client.Post(
		"http://localhost"+path,
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		// The most common failure: the daemon is not running.
		return fmt.Errorf(
			"could not reach the fastship daemon — is it running?\n"+
				"start it with: sudo shipd\n\n(%w)", err)
	}
	defer resp.Body.Close()

	// The daemon signals errors with a non-2xx status and an {"error": ...}
	// body. Surface that message to the user.
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("%s", e.Error)
	}

	if respBody != nil {
		return json.NewDecoder(resp.Body).Decode(respBody)
	}
	return nil
}

// ensureDaemon makes sure the daemon is running before we send it a
// request. If it is not reachable, it starts shipd in the background and
// waits for it to come up. This is what makes the daemon invisible — the
// user never starts it by hand.
func ensureDaemon() error {
	// Already up? Nothing to do.
	if daemonHealthy() {
		return nil
	}

	// Not running — start it. shipd is expected to be on PATH alongside
	// fastship; we launch it detached so it keeps running after this CLI
	// command exits.
	fmt.Println("→ starting fastship daemon...")

	cmd := exec.Command("/vagrant/FastShip/shipd")
	// Detach: the daemon must outlive this CLI process, so we do not wait
	// on it and we let it run in its own session.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the daemon: %w", err)
	}

	// Wait for it to become reachable, polling /health. Give it a few
	// seconds — starting the engine and binding the socket takes a moment.
	for i := 0; i < 30; i++ {
		if daemonHealthy() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf( // daemon is down, tell the user
		"the fastship daemon is not running\n" +
			"start it with: sudo shipd &")
}

// daemonHealthy reports whether the daemon answers a health check.
func daemonHealthy() bool {
	client := newClient()
	resp, err := client.Get("http://localhost/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
