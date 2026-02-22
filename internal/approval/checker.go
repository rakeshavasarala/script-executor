package approval

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Checker handles approval flow.
type Checker struct {
	store          Store
	defaultApprovers []string
}

// NewChecker creates an approval checker.
func NewChecker(store Store, defaultApprovers []string) *Checker {
	return &Checker{
		store:           store,
		defaultApprovers: defaultApprovers,
	}
}

// CreateRequest creates a new approval request.
func (c *Checker) CreateRequest(ctx context.Context, executionID, stepName, runbookID, user, script, scriptHash string, approvers []string) error {
	if len(approvers) == 0 {
		approvers = c.defaultApprovers
	}
	req := &Request{
		ExecutionID: executionID,
		StepName:    stepName,
		RunbookID:   runbookID,
		User:        user,
		Script:      script,
		ScriptHash:  scriptHash,
		Approvers:   approvers,
	}
	return c.store.Create(ctx, req)
}

// Check returns the approval status for an execution.
func (c *Checker) Check(ctx context.Context, executionID, stepName, script, scriptHash string, approvers []string, user string) (Status, error) {
	req, err := c.store.Get(ctx, executionID, stepName)
	if err != nil {
		// Not found - not yet approved, proceed (or create pending)
		return StatusPending, nil
	}

	switch req.Status {
	case StatusApproved:
		return StatusApproved, nil
	case StatusDenied:
		return StatusDenied, nil
	case StatusExpired:
		return StatusPending, nil
	case StatusPending:
		return StatusPending, nil
	}
	return StatusPending, nil
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// Approve approves an execution.
func (c *Checker) Approve(ctx context.Context, executionID, stepName, approver string) error {
	req, err := c.store.Get(ctx, executionID, stepName)
	if err != nil {
		return err
	}
	if req.Status != StatusPending {
		return fmt.Errorf("cannot approve: status is %s", req.Status)
	}
	if !contains(req.Approvers, approver) {
		return fmt.Errorf("user %s is not authorized to approve", approver)
	}
	req.Status = StatusApproved
	req.ApprovedBy = approver
	req.ApprovedAt = time.Now()
	return c.store.Update(ctx, req)
}

// Deny denies an execution.
func (c *Checker) Deny(ctx context.Context, executionID, stepName, denier, reason string) error {
	req, err := c.store.Get(ctx, executionID, stepName)
	if err != nil {
		return err
	}
	if req.Status != StatusPending {
		return fmt.Errorf("cannot deny: status is %s", req.Status)
	}
	if !contains(req.Approvers, denier) {
		return fmt.Errorf("user %s is not authorized to deny", denier)
	}
	req.Status = StatusDenied
	req.DeniedBy = denier
	req.DeniedAt = time.Now()
	req.DenialReason = reason
	return c.store.Update(ctx, req)
}
