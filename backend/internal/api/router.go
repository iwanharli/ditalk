package api

import (
	"encoding/json"
	"net/http"
)

// NewRouter wires up the top-level HTTP routes for the ditalk backend.
// Endpoints follow the contract in docs/.../22 DESAIN API DAN EVENT INTERNAL.
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)

	mux.HandleFunc("POST /auth/login", notImplemented)
	mux.HandleFunc("POST /wa/pair", notImplemented)
	mux.HandleFunc("GET /wa/status", notImplemented)
	mux.HandleFunc("DELETE /wa/session", notImplemented)

	mux.HandleFunc("POST /conversations/sync", notImplemented)
	mux.HandleFunc("GET /conversations/{id}/messages", notImplemented)
	mux.HandleFunc("DELETE /conversations/{id}", notImplemented)

	mux.HandleFunc("POST /analysis/run", notImplemented)
	mux.HandleFunc("GET /analysis/trends", notImplemented)
	mux.HandleFunc("PATCH /analyses/{id}/correction", notImplemented)

	mux.HandleFunc("POST /search", notImplemented)

	mux.HandleFunc("GET /memories/candidates", notImplemented)
	mux.HandleFunc("POST /memories/{id}/confirm", notImplemented)

	mux.HandleFunc("GET /people/{id}/profile", notImplemented)
	mux.HandleFunc("GET /relationships/{id}/insights", notImplemented)

	mux.HandleFunc("POST /journal", notImplemented)
	mux.HandleFunc("PATCH /commitments/{id}", notImplemented)
	mux.HandleFunc("POST /exports", notImplemented)

	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not_implemented"})
}
