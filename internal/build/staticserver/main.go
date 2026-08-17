// Command staticserver is a minimal static file server that FastShip bakes
// into images for static apps (React, Vue, plain frontends). It serves a
// directory of files over HTTP, with single-page-app fallback so client-
// side routes work.
//
// It reads two env vars FastShip sets:
//
//	STATIC_DIR  — the directory of files to serve (default /static)
//	PORT        — the port to listen on (default 3000)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dir := os.Getenv("STATIC_DIR")
	if dir == "" {
		dir = "/static"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fs := http.FileServer(http.Dir(dir))

	// Serve files, with SPA fallback: if a path doesn't map to a real file,
	// serve index.html so client-side routing (React Router, Vue Router)
	// works. Without this, refreshing on /about would 404.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, r.URL.Path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Not a real file — serve index.html (SPA fallback).
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})

	log.Printf("serving %s on :%s", dir, port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}
