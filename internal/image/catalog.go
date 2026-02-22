package image

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

// CatalogEntry describes an image in the catalog.
type CatalogEntry struct {
	Image       string   `yaml:"image"`
	PullSecret  string   `yaml:"pull_secret"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	ApprovedBy  string   `yaml:"approved_by"`
	ApprovedAt  string   `yaml:"approved_at"`
}

// Catalog provides image lookup by reference name.
type Catalog struct {
	client    kubernetes.Interface
	namespace string
	name      string
	entries   map[string]CatalogEntry
}

// NewCatalog creates an image catalog.
func NewCatalog(client kubernetes.Interface, namespace, configMapName string) *Catalog {
	return &Catalog{
		client:    client,
		namespace: namespace,
		name:      configMapName,
		entries:   make(map[string]CatalogEntry),
	}
}

// Load fetches and parses the catalog from the ConfigMap.
func (c *Catalog) Load(ctx context.Context) error {
	cm, err := c.client.CoreV1().ConfigMaps(c.namespace).Get(ctx, c.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get image catalog configmap: %w", err)
	}

	yamlContent, ok := cm.Data["catalog.yaml"]
	if !ok {
		return fmt.Errorf("catalog.yaml not found in configmap %s", c.name)
	}

	var data map[string]CatalogEntry
	if err := yaml.Unmarshal([]byte(yamlContent), &data); err != nil {
		return fmt.Errorf("parse catalog.yaml: %w", err)
	}

	c.entries = data
	return nil
}

// Get returns the catalog entry for an image reference (e.g. "terraform", "aws").
func (c *Catalog) Get(ref string) (CatalogEntry, bool) {
	entry, ok := c.entries[ref]
	return entry, ok
}

// ResolvedImage holds the full image reference and pull settings.
type ResolvedImage struct {
	Image           string
	PullSecret      string
	PullPolicy      string
}
