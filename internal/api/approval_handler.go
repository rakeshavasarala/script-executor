package api

import (
	"encoding/json"
	"net/http"

	"github.com/rakeshavasarala/script-executor/internal/approval"
)

// ApprovalHandler handles approval HTTP endpoints.
type ApprovalHandler struct {
	checker *approval.Checker
}

// NewApprovalHandler creates an approval handler.
func NewApprovalHandler(checker *approval.Checker) *ApprovalHandler {
	return &ApprovalHandler{checker: checker}
}

// ListPending returns pending approvals for the current user.
func (h *ApprovalHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "approval not enabled", http.StatusServiceUnavailable)
		return
	}
	// TODO: Get user from auth context; for now return empty
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

// Approve approves an execution.
func (h *ApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "approval not enabled", http.StatusServiceUnavailable)
		return
	}
	executionID := r.PathValue("execution_id")
	stepName := r.PathValue("step_name")
	if executionID == "" || stepName == "" {
		http.Error(w, "missing execution_id or step_name", http.StatusBadRequest)
		return
	}
	// TODO: Get user from auth context; for now use header
	user := r.Header.Get("X-User")
	if user == "" {
		user = "anonymous"
	}
	if err := h.checker.Approve(r.Context(), executionID, stepName, user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

// Deny denies an execution.
func (h *ApprovalHandler) Deny(w http.ResponseWriter, r *http.Request) {
	if h.checker == nil {
		http.Error(w, "approval not enabled", http.StatusServiceUnavailable)
		return
	}
	executionID := r.PathValue("execution_id")
	stepName := r.PathValue("step_name")
	if executionID == "" || stepName == "" {
		http.Error(w, "missing execution_id or step_name", http.StatusBadRequest)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	user := r.Header.Get("X-User")
	if user == "" {
		user = "anonymous"
	}
	if err := h.checker.Deny(r.Context(), executionID, stepName, user, body.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "denied"})
}
