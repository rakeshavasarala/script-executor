package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Status represents approval status.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
	StatusExpired  Status = "expired"
)

// Request represents an approval request.
type Request struct {
	ID          string    `json:"id"`
	ExecutionID string    `json:"execution_id"`
	StepName    string    `json:"step_name"`
	RunbookID   string    `json:"runbook_id"`
	User        string    `json:"user"`
	Script      string    `json:"script"`
	ScriptHash  string    `json:"script_hash"`
	Approvers   []string  `json:"approvers"`
	Status      Status    `json:"status"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	ApprovedAt  time.Time `json:"approved_at,omitempty"`
	DeniedBy    string    `json:"denied_by,omitempty"`
	DeniedAt    time.Time `json:"denied_at,omitempty"`
	DenialReason string   `json:"denial_reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Store provides approval persistence.
type Store interface {
	Create(ctx context.Context, req *Request) error
	Get(ctx context.Context, executionID, stepName string) (*Request, error)
	Update(ctx context.Context, req *Request) error
}

// ConfigMapStore implements Store using a ConfigMap.
type ConfigMapStore struct {
	client    kubernetes.Interface
	namespace string
	name      string
}

// NewConfigMapStore creates a ConfigMap-backed store.
func NewConfigMapStore(client kubernetes.Interface, namespace, name string) *ConfigMapStore {
	return &ConfigMapStore{
		client:    client,
		namespace: namespace,
		name:      name,
	}
}

// Create stores an approval request.
func (s *ConfigMapStore) Create(ctx context.Context, req *Request) error {
	req.ID = fmt.Sprintf("%s-%s-%d", req.ExecutionID, req.StepName, time.Now().UnixNano())
	req.Status = StatusPending
	req.CreatedAt = time.Now()
	req.ExpiresAt = req.CreatedAt.Add(24 * time.Hour)

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	cm, err := s.getOrCreate(ctx)
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	key := fmt.Sprintf("%s-%s", req.ExecutionID, req.StepName)
	cm.Data[key] = string(data)

	_, err = s.client.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// Get retrieves an approval request.
func (s *ConfigMapStore) Get(ctx context.Context, executionID, stepName string) (*Request, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s-%s", executionID, stepName)
	data, ok := cm.Data[key]
	if !ok {
		return nil, fmt.Errorf("approval request not found")
	}
	var req Request
	if err := json.Unmarshal([]byte(data), &req); err != nil {
		return nil, err
	}
	if req.Status == StatusPending && time.Now().After(req.ExpiresAt) {
		req.Status = StatusExpired
		s.Update(ctx, &req)
	}
	return &req, nil
}

// Update updates an approval request.
func (s *ConfigMapStore) Update(ctx context.Context, req *Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s-%s", req.ExecutionID, req.StepName)
	cm.Data[key] = string(data)
	_, err = s.client.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

func (s *ConfigMapStore) getOrCreate(ctx context.Context) (*corev1.ConfigMap, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err == nil {
		return cm, nil
	}
	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: s.name, Namespace: s.namespace},
		Data:       make(map[string]string),
	}
	return s.client.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
}
