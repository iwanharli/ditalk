package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/rs/cors"

	"ditalk/backend/internal/ai"
	"ditalk/backend/internal/config"
	"ditalk/backend/internal/crypto"
	"ditalk/backend/internal/queue"
	"ditalk/backend/internal/storage"
)

type Server struct {
	cfg      config.Config
	db       *storage.DB
	queue    *queue.Client
	ai       *ai.Client
	cipher   *crypto.Cipher
	log      *log.Logger
	validate *validator.Validate
}

// NewServer wires dependencies. cipher may be nil when no encryption key is
// configured; endpoints that persist chat content refuse to run in that case
// rather than storing plaintext.
func NewServer(
	cfg config.Config,
	db *storage.DB,
	q *queue.Client,
	aiClient *ai.Client,
	cipher *crypto.Cipher,
	logger *log.Logger,
) *Server {
	return &Server{
		cfg:      cfg,
		db:       db,
		queue:    q,
		ai:       aiClient,
		cipher:   cipher,
		log:      logger,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// Routes follow the API contract in doc bab 22.1. Handlers not yet implemented
// return 501 so the contract is visible before the logic exists.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	mux.HandleFunc("POST /auth/login", notImplemented)
	mux.HandleFunc("POST /wa/pair", notImplemented)
	mux.HandleFunc("GET /wa/status", notImplemented)
	mux.HandleFunc("DELETE /wa/session", notImplemented)

	// Import Export Chat is the first ingestion path: no unofficial API, and the
	// dataset stays under the user's control (doc bab 30, Keputusan 1).
	mux.HandleFunc("POST /imports/preview", s.handleImportPreview)
	mux.HandleFunc("POST /imports", s.handleImport)

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

	// Internal ingestion endpoint used by the Node/Baileys connector.
	mux.HandleFunc("POST /internal/events", notImplemented)

	c := cors.New(cors.Options{
		AllowedOrigins:   s.cfg.AllowOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	return c.Handler(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{"status": "ok", "database": "ok"}
	if err := s.db.Pool.Ping(r.Context()); err != nil {
		status["status"] = "degraded"
		status["database"] = "unreachable"
	}
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not_implemented"})
}
