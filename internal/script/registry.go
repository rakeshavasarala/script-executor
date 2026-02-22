package script

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

// RegistryEntry describes a script in the registry.
type RegistryEntry struct {
	ConfigMap string `yaml:"configmap"`
	Secret    string `yaml:"secret"`
	Key       string `yaml:"key"`
}

// RegistryData is the parsed registry.yaml structure.
type RegistryData struct {
	Scripts map[string]RegistryEntry `yaml:"scripts"`
}

// Registry provides script_id lookup.
type Registry struct {
	client    kubernetes.Interface
	namespace string
}

// NewRegistry creates a registry.
func NewRegistry(client kubernetes.Interface, namespace string) *Registry {
	return &Registry{client: client, namespace: namespace}
}

// NewRegistryWithClient is an alias for NewRegistry.
func NewRegistryWithClient(client kubernetes.Interface, namespace string) *Registry {
	return NewRegistry(client, namespace)
}

// LoadByID loads a script by its registry ID.
func (r *Registry) LoadByID(ctx context.Context, scriptID string) (string, *Source, error) {
	entry, err := r.getEntry(ctx, scriptID)
	if err != nil {
		return "", nil, err
	}

	if entry.ConfigMap != "" {
		content, err := r.loadFromConfigMap(ctx, r.namespace, entry.ConfigMap, entry.Key)
		if err != nil {
			return "", nil, err
		}
		return content, &Source{
			Type:      SourceRegistry,
			Content:   content,
			Name:      entry.ConfigMap,
			Key:       entry.Key,
			Namespace: r.namespace,
		}, nil
	}

	if entry.Secret != "" {
		content, err := r.loadFromSecret(ctx, r.namespace, entry.Secret, entry.Key)
		if err != nil {
			return "", nil, err
		}
		return content, &Source{
			Type:      SourceRegistry,
			Content:   content,
			Name:      entry.Secret,
			Key:       entry.Key,
			Namespace: r.namespace,
		}, nil
	}

	return "", nil, fmt.Errorf("registry entry %q has neither configmap nor secret", scriptID)
}

func (r *Registry) getEntry(ctx context.Context, scriptID string) (*RegistryEntry, error) {
	cm, err := r.client.CoreV1().ConfigMaps(r.namespace).Get(ctx, ScriptRegistryConfigMap, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get script-registry configmap: %w", err)
	}

	yamlContent, ok := cm.Data["registry.yaml"]
	if !ok {
		return nil, fmt.Errorf("registry.yaml not found in script-registry configmap")
	}

	var data RegistryData
	if err := yaml.Unmarshal([]byte(yamlContent), &data); err != nil {
		return nil, fmt.Errorf("parse registry.yaml: %w", err)
	}

	entry, ok := data.Scripts[scriptID]
	if !ok {
		return nil, fmt.Errorf("script not found in registry: %s", scriptID)
	}

	if (entry.ConfigMap == "" && entry.Secret == "") || entry.Key == "" {
		return nil, fmt.Errorf("invalid registry entry for %s: must have configmap or secret and key", scriptID)
	}

	if entry.ConfigMap != "" && entry.Secret != "" {
		return nil, fmt.Errorf("registry entry %s has both configmap and secret", scriptID)
	}

	return &entry, nil
}

func (r *Registry) loadFromConfigMap(ctx context.Context, namespace, name, key string) (string, error) {
	cm, err := r.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	content, ok := cm.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in configmap %s/%s", key, namespace, name)
	}
	return content, nil
}

func (r *Registry) loadFromSecret(ctx context.Context, namespace, name, key string) (string, error) {
	secret, err := r.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	contentBytes, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", key, namespace, name)
	}
	return string(contentBytes), nil
}
