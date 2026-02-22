package script

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// ApprovedScriptsConfigMap is the fixed ConfigMap name for script_path.
	ApprovedScriptsConfigMap = "approved-scripts"
	// ScriptRegistryConfigMap is the ConfigMap for script_id lookup.
	ScriptRegistryConfigMap = "script-registry"
)

// SourceType identifies where the script came from.
type SourceType string

const (
	SourceInline     SourceType = "inline"
	SourceConfigMap  SourceType = "configmap"
	SourceSecret     SourceType = "secret"
	SourcePath       SourceType = "path"
	SourceRegistry   SourceType = "registry"
)

// Source describes the script source for audit/logging.
type Source struct {
	Type      SourceType
	Content   string   // For inline/configmap/secret: the script content
	Path      string   // For path: the path (e.g. /scripts/foo.sh)
	Name      string   // ConfigMap or Secret name
	Key       string   // Key within ConfigMap/Secret
	Namespace string   // K8s namespace
}

// Loader loads scripts from various sources.
type Loader struct {
	client    kubernetes.Interface
	namespace string
	registry  *Registry
}

// NewLoader creates a script loader.
func NewLoader(cfg *rest.Config, namespace string) (*Loader, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}
	return &Loader{
		client:    client,
		namespace: namespace,
		registry:  NewRegistry(client, namespace),
	}, nil
}

// NewLoaderWithClient creates a loader with an existing K8s client.
func NewLoaderWithClient(client kubernetes.Interface, namespace string) *Loader {
	return &Loader{
		client:    client,
		namespace: namespace,
		registry:  NewRegistryWithClient(client, namespace),
	}
}

func getString(m *structpb.Struct, key, def string) string {
	if m == nil || m.Fields == nil {
		return def
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return def
	}
	s := f.GetStringValue()
	if s == "" {
		return def
	}
	return s
}

func getMap(m *structpb.Struct, key string) *structpb.Struct {
	if m == nil || m.Fields == nil {
		return nil
	}
	f, ok := m.Fields[key]
	if !ok || f == nil {
		return nil
	}
	return f.GetStructValue()
}

// LoadScript loads script content from parameters. Exactly one source must be provided.
func (l *Loader) LoadScript(ctx context.Context, params *structpb.Struct) (string, *Source, error) {
	// 1. Inline script
	if inline := getString(params, "inline_script", ""); inline != "" {
		return inline, &Source{Type: SourceInline, Content: inline}, nil
	}

	// 2. Script from ConfigMap
	if cmRef := getMap(params, "script_from_configmap"); cmRef != nil && len(cmRef.Fields) > 0 {
		cmName := getString(cmRef, "configmap_name", "")
		key := getString(cmRef, "key", "")
		if cmName == "" || key == "" {
			return "", nil, fmt.Errorf("script_from_configmap requires configmap_name and key")
		}
		ns := getString(cmRef, "namespace", l.namespace)
		content, err := l.loadFromConfigMap(ctx, ns, cmName, key)
		if err != nil {
			return "", nil, fmt.Errorf("load from configmap: %w", err)
		}
		return content, &Source{
			Type:      SourceConfigMap,
			Content:   content,
			Name:      cmName,
			Key:       key,
			Namespace: ns,
		}, nil
	}

	// 3. Script from Secret
	if secretRef := getMap(params, "script_from_secret"); secretRef != nil && len(secretRef.Fields) > 0 {
		secretName := getString(secretRef, "secret_name", "")
		key := getString(secretRef, "key", "")
		if secretName == "" || key == "" {
			return "", nil, fmt.Errorf("script_from_secret requires secret_name and key")
		}
		ns := getString(secretRef, "namespace", l.namespace)
		content, err := l.loadFromSecret(ctx, ns, secretName, key)
		if err != nil {
			return "", nil, fmt.Errorf("load from secret: %w", err)
		}
		return content, &Source{
			Type:      SourceSecret,
			Content:   content,
			Name:      secretName,
			Key:       key,
			Namespace: ns,
		}, nil
	}

	// 4. Script path (pre-mounted)
	if path := getString(params, "script_path", ""); path != "" {
		exists, err := l.scriptPathExists(ctx, path)
		if err != nil {
			return "", nil, fmt.Errorf("validate script path: %w", err)
		}
		if !exists {
			return "", nil, fmt.Errorf("script not found in approved-scripts: %s", path)
		}
		// Empty content - script runs from mounted path
		return "", &Source{Type: SourcePath, Path: path}, nil
	}

	// 5. Script by registry ID
	if scriptID := getString(params, "script_id", ""); scriptID != "" {
		content, source, err := l.registry.LoadByID(ctx, scriptID)
		if err != nil {
			return "", nil, fmt.Errorf("load by script_id: %w", err)
		}
		return content, source, nil
	}

	return "", nil, fmt.Errorf("no script source provided (inline_script, script_from_configmap, script_from_secret, script_path, or script_id)")
}

func (l *Loader) loadFromConfigMap(ctx context.Context, namespace, name, key string) (string, error) {
	cm, err := l.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	content, ok := cm.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in configmap %s/%s", key, namespace, name)
	}
	return content, nil
}

func (l *Loader) loadFromSecret(ctx context.Context, namespace, name, key string) (string, error) {
	secret, err := l.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	contentBytes, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", key, namespace, name)
	}
	return string(contentBytes), nil
}

// scriptPathExists checks if the path exists in the approved-scripts ConfigMap.
// Path should be like /scripts/foo.sh - we validate the key (filename) exists.
func (l *Loader) scriptPathExists(ctx context.Context, path string) (bool, error) {
	// Path format: /scripts/filename.sh
	// ConfigMap keys are the filenames
	if len(path) < 9 || path[:9] != "/scripts/" {
		return false, fmt.Errorf("script_path must start with /scripts/")
	}
	key := path[9:]
	if key == "" {
		return false, fmt.Errorf("script_path must include filename after /scripts/")
	}

	cm, err := l.client.CoreV1().ConfigMaps(l.namespace).Get(ctx, ApprovedScriptsConfigMap, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	_, ok := cm.Data[key]
	return ok, nil
}
