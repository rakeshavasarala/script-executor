package execution

import (
	"time"

	"github.com/rakeshavasarala/script-executor/internal/script"
	corev1 "k8s.io/api/core/v1"
)

// Context holds all data needed to build and run a script execution Job.
type Context struct {
	ExecutionID string
	RunbookID   string
	User        string
	StepName    string

	// Script
	Script       string
	ScriptSource *script.Source
	ScriptHash   string

	// Image
	Image           string
	ImagePullPolicy corev1.PullPolicy
	ImagePullSecret string

	// Execution
	Interpreter string
	Args        []string
	WorkingDir  string
	Timeout     time.Duration
	Stdin       string

	// Environment
	Env               map[string]string
	EnvFromSecret     map[string]SecretKeyRef
	EnvFromConfigMap  map[string]ConfigMapKeyRef
	SecretEnvAll      []string
	ConfigMapEnvAll   []string

	// Volumes
	VolumesFromSecret   []SecretVolume
	VolumesFromConfigMap []ConfigMapVolume

	// Scheduling
	NodeSelector      map[string]string
	Tolerations       []corev1.Toleration
	Affinity          *corev1.Affinity
	PriorityClassName string

	// Resources
	Resources corev1.ResourceRequirements

	// Job settings
	ServiceAccount          string
	TTLSecondsAfterFinished int32
	BackoffLimit            int32
	Labels                  map[string]string
	Annotations             map[string]string
}

// SecretKeyRef references a key in a Secret.
type SecretKeyRef struct {
	SecretName string
	Key        string
	Optional   bool
}

// ConfigMapKeyRef references a key in a ConfigMap.
type ConfigMapKeyRef struct {
	ConfigMapName string
	Key           string
	Optional      bool
}

// SecretVolume mounts a Secret as files.
type SecretVolume struct {
	SecretName string
	MountPath  string
	Optional   bool
	Items      []KeyToPath
}

// ConfigMapVolume mounts a ConfigMap as files.
type ConfigMapVolume struct {
	ConfigMapName string
	MountPath     string
	Optional      bool
	Items         []KeyToPath
}

// KeyToPath maps a key to a path.
type KeyToPath struct {
	Key  string
	Path string
	Mode *int32
}
