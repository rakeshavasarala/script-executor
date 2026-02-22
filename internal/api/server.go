package api

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Server runs the HTTP API server.
type Server struct {
	server *http.Server
}

// NewServer creates an HTTP server for the approval API.
func NewServer(port int, handler *ApprovalHandler) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/approvals/pending", handler.ListPending)
	mux.HandleFunc("POST /api/v1/approvals/{execution_id}/{step_name}/approve", handler.Approve)
	mux.HandleFunc("POST /api/v1/approvals/{execution_id}/{step_name}/deny", handler.Deny)

	return &Server{
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
