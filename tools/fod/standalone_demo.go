package main

// The synthetic preview serves only static assets and a mode flag. It never
// opens a node connection, loads a key, or implements a transaction endpoint.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func standaloneDemoHandler(origin string) http.Handler {
	assets := map[string]string{"/": "standalone_app.html", "/app.js": "standalone_app.js", "/app.css": "standalone_app.css"}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		if r.Host != strings.TrimPrefix(origin, "http://") ||
			(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "local origin refused", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "read-only preview", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/api/mode" {
			writeStandaloneJSON(w, struct {
				Preview bool `json:"preview"`
			}{Preview: true})
			return
		}
		file, ok := assets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		raw, err := standaloneAssets.ReadFile(file)
		if err != nil {
			http.Error(w, "preview asset unavailable", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case "/app.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case "/app.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		_, _ = w.Write(raw)
	})
}

func runStandaloneDemo() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	origin := "http://" + listener.Addr().String()
	fmt.Println("Flips on Demand synthetic preview:", origin)
	fmt.Println("No node, key, private file, or transaction is connected.")
	if err := openStandaloneBrowser(origin); err != nil {
		fmt.Println("Open the preview URL above in your browser.")
	}
	server := &http.Server{Handler: standaloneDemoHandler(origin), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
