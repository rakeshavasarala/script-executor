package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the script executor.
type Config struct {
	ScriptExecutor ScriptExecutorConfig `yaml:"script_executor"`
}

// ScriptExecutorConfig is the root config section.
type ScriptExecutorConfig struct {
	GRPC        GRPCConfig        `yaml:"grpc"`
	Kubernetes  KubernetesConfig  `yaml:"kubernetes"`
	Image       ImageConfig       `yaml:"image"`
	Security    SecurityConfig    `yaml:"security"`
	Approval    ApprovalConfig    `yaml:"approval"`
	Audit       AuditConfig       `yaml:"audit"`
	Monitoring  MonitoringConfig  `yaml:"monitoring"`
}

// GRPCConfig holds gRPC server settings.
type GRPCConfig struct {
	Port             int `yaml:"port"`
	MaxMessageSize   int `yaml:"max_message_size"`
}

// KubernetesConfig holds K8s-related settings.
type KubernetesConfig struct {
	Namespace      string              `yaml:"namespace"`
	ServiceAccount string              `yaml:"service_account"`
	JobDefaults    JobDefaultsConfig   `yaml:"job_defaults"`
	DefaultResources ResourceConfig    `yaml:"default_resources"`
	MaxResources   ResourceConfig      `yaml:"max_resources"`
}

// JobDefaultsConfig holds Job spec defaults.
type JobDefaultsConfig struct {
	TTLSecondsAfterFinished int `yaml:"ttl_seconds_after_finished"`
	BackoffLimit           int `yaml:"backoff_limit"`
	ActiveDeadlineSeconds  int `yaml:"active_deadline_seconds"`
}

// ResourceConfig holds CPU/memory resource settings.
type ResourceConfig struct {
	Requests ResourceRequests `yaml:"requests"`
	Limits   ResourceLimits   `yaml:"limits"`
}

// ResourceRequests holds request values.
type ResourceRequests struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

// ResourceLimits holds limit values.
type ResourceLimits struct {
	CPU              string `yaml:"cpu"`
	Memory           string `yaml:"memory"`
	EphemeralStorage string `yaml:"ephemeral_storage"`
}

// ImageConfig holds image catalog and validation settings.
type ImageConfig struct {
	DefaultImage         string           `yaml:"default_image"`
	DefaultImagePullSecret string         `yaml:"default_image_pull_secret"`
	DefaultImagePullPolicy string         `yaml:"default_image_pull_policy"`
	ApprovedImages       []string         `yaml:"approved_images"`
	BlockedImages        []string         `yaml:"blocked_images"`
	Catalog              ImageCatalogConfig `yaml:"catalog"`
	VerifyImageAccess    bool             `yaml:"verify_image_access"`
	ScanImagesOnApproval bool             `yaml:"scan_images_on_approval"`
}

// ImageCatalogConfig holds catalog ConfigMap settings.
type ImageCatalogConfig struct {
	Enabled   bool   `yaml:"enabled"`
	ConfigMapName      string `yaml:"configmap_name"`
	ConfigMapNamespace string `yaml:"configmap_namespace"`
}

// SecurityConfig holds security and validation settings.
type SecurityConfig struct {
	RequiredPermission string   `yaml:"required_permission"`
	BlockedCommands    []string `yaml:"blocked_commands"`
	MaxScriptSize     int      `yaml:"max_script_size"`
	MaxScriptLines    int      `yaml:"max_script_lines"`
	DefaultTimeout    string   `yaml:"default_timeout"`
	MaxTimeout        string   `yaml:"max_timeout"`
	RunAsNonRoot      bool     `yaml:"run_as_non_root"`
	RunAsUser         int64    `yaml:"run_as_user"`
	FSGroup           int64    `yaml:"fs_group"`
	ReadOnlyRootFilesystem bool `yaml:"read_only_root_filesystem"`
	AllowPrivilegeEscalation bool `yaml:"allow_privilege_escalation"`
	DropAllCapabilities bool   `yaml:"drop_all_capabilities"`
}

// ApprovalConfig holds approval workflow settings.
type ApprovalConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	Storage        ApprovalStorageConfig `yaml:"storage"`
	ApprovalTimeout string              `yaml:"approval_timeout"`
	DefaultApprovers []string           `yaml:"default_approvers"`
	AutoApprove    AutoApproveConfig    `yaml:"auto_approve"`
}

// ApprovalStorageConfig holds approval storage backend settings.
type ApprovalStorageConfig struct {
	Type               string `yaml:"type"`
	ConfigMapName      string `yaml:"configmap_name"`
	ConfigMapNamespace string `yaml:"configmap_namespace"`
}

// AutoApproveConfig holds auto-approve rules.
type AutoApproveConfig struct {
	Enabled bool                   `yaml:"enabled"`
	Rules   []AutoApproveRule      `yaml:"rules"`
}

// AutoApproveRule defines when to auto-approve.
type AutoApproveRule struct {
	UserGroup     string `yaml:"user_group"`
	ScriptPattern string `yaml:"script_pattern"`
}

// AuditConfig holds audit logging settings.
type AuditConfig struct {
	Enabled          bool         `yaml:"enabled"`
	LogFile          string       `yaml:"log_file"`
	SIEM             SIEMConfig  `yaml:"siem"`
	LogScriptContent bool         `yaml:"log_script_content"`
	LogScriptOutput  bool         `yaml:"log_script_output"`
	LogEnvironment   bool         `yaml:"log_environment"`
}

// SIEMConfig holds SIEM integration settings.
type SIEMConfig struct {
	Enabled    bool              `yaml:"enabled"`
	Endpoint   string            `yaml:"endpoint"`
	TokenSecret SecretRef        `yaml:"token_secret"`
}

// SecretRef references a K8s secret key.
type SecretRef struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

// MonitoringConfig holds metrics and observability settings.
type MonitoringConfig struct {
	Metrics    MetricsConfig    `yaml:"metrics"`
	Pushgateway PushgatewayConfig `yaml:"pushgateway"`
}

// MetricsConfig holds Prometheus metrics settings.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// PushgatewayConfig holds pushgateway settings.
type PushgatewayConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// Load reads configuration from file and environment.
func Load() (*Config, error) {
	cfg := defaultConfig()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		var fileConfig Config
		if err := yaml.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}

		mergeConfig(cfg, &fileConfig)
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		ScriptExecutor: ScriptExecutorConfig{
			GRPC: GRPCConfig{
				Port:           50051,
				MaxMessageSize: 10 * 1024 * 1024, // 10MB
			},
			Kubernetes: KubernetesConfig{
				Namespace:      getEnvOrDefault("KUBERNETES_NAMESPACE", "opscontrolroom-system"),
				ServiceAccount: "script-executor-runner",
				JobDefaults: JobDefaultsConfig{
					TTLSecondsAfterFinished: 300,
					BackoffLimit:            0,
					ActiveDeadlineSeconds:   1800,
				},
				DefaultResources: ResourceConfig{
					Requests: ResourceRequests{CPU: "100m", Memory: "64Mi"},
					Limits:   ResourceLimits{CPU: "500m", Memory: "256Mi", EphemeralStorage: "1Gi"},
				},
				MaxResources: ResourceConfig{
					Limits: ResourceLimits{CPU: "4000m", Memory: "8Gi", EphemeralStorage: "20Gi"},
				},
			},
			Image: ImageConfig{
				DefaultImage:          getEnvOrDefault("DEFAULT_IMAGE", "alpine:latest"),
				DefaultImagePullSecret: getEnvOrDefault("DEFAULT_IMAGE_PULL_SECRET", ""),
				DefaultImagePullPolicy: "IfNotPresent",
				ApprovedImages:        []string{},
				BlockedImages:         []string{},
				Catalog: ImageCatalogConfig{
					Enabled:   true,
					ConfigMapName:      "script-image-catalog",
					ConfigMapNamespace: "opscontrolroom-system",
				},
				VerifyImageAccess:    false,
				ScanImagesOnApproval: false,
			},
			Security: SecurityConfig{
				RequiredPermission: "executors.use.script",
				BlockedCommands: []string{
					"rm", "dd", "mkfs", "fdisk", "mkswap",
					"sudo", "su", "setuid",
					"reboot", "shutdown", "init", "systemctl",
					"nmap", "masscan",
					"kill", "killall", "pkill",
				},
				MaxScriptSize:     524288, // 500KB
				MaxScriptLines:    1000,
				DefaultTimeout:    "5m",
				MaxTimeout:        "30m",
				RunAsNonRoot:      true,
				RunAsUser:         65534,
				FSGroup:           65534,
				ReadOnlyRootFilesystem: true,
				AllowPrivilegeEscalation: false,
				DropAllCapabilities: true,
			},
			Approval: ApprovalConfig{
				Enabled: true,
				Storage: ApprovalStorageConfig{
					Type:               "configmap",
					ConfigMapName:      "script-approvals",
					ConfigMapNamespace: "opscontrolroom-system",
				},
				ApprovalTimeout:  "24h",
				DefaultApprovers: []string{"sre-leads"},
			},
			Audit: AuditConfig{
				Enabled:          true,
				LogFile:          "/var/log/ocr/script-audit.log",
				LogScriptContent: true,
				LogScriptOutput:  true,
				LogEnvironment:   false,
			},
			Monitoring: MonitoringConfig{
				Metrics: MetricsConfig{
					Enabled: true,
					Port:    9090,
					Path:    "/metrics",
				},
				Pushgateway: PushgatewayConfig{Enabled: false},
			},
		},
	}
}

func mergeConfig(dst, src *Config) {
	if src.ScriptExecutor.GRPC.Port != 0 {
		dst.ScriptExecutor.GRPC.Port = src.ScriptExecutor.GRPC.Port
	}
	if src.ScriptExecutor.GRPC.MaxMessageSize != 0 {
		dst.ScriptExecutor.GRPC.MaxMessageSize = src.ScriptExecutor.GRPC.MaxMessageSize
	}
	if src.ScriptExecutor.Kubernetes.Namespace != "" {
		dst.ScriptExecutor.Kubernetes.Namespace = src.ScriptExecutor.Kubernetes.Namespace
	}
	if src.ScriptExecutor.Kubernetes.ServiceAccount != "" {
		dst.ScriptExecutor.Kubernetes.ServiceAccount = src.ScriptExecutor.Kubernetes.ServiceAccount
	}
	if src.ScriptExecutor.Kubernetes.JobDefaults.TTLSecondsAfterFinished != 0 {
		dst.ScriptExecutor.Kubernetes.JobDefaults.TTLSecondsAfterFinished = src.ScriptExecutor.Kubernetes.JobDefaults.TTLSecondsAfterFinished
	}
	if src.ScriptExecutor.Kubernetes.JobDefaults.BackoffLimit != 0 || src.ScriptExecutor.Kubernetes.JobDefaults.ActiveDeadlineSeconds != 0 {
		dst.ScriptExecutor.Kubernetes.JobDefaults.BackoffLimit = src.ScriptExecutor.Kubernetes.JobDefaults.BackoffLimit
		dst.ScriptExecutor.Kubernetes.JobDefaults.ActiveDeadlineSeconds = src.ScriptExecutor.Kubernetes.JobDefaults.ActiveDeadlineSeconds
	}
	if src.ScriptExecutor.Image.DefaultImage != "" {
		dst.ScriptExecutor.Image.DefaultImage = src.ScriptExecutor.Image.DefaultImage
	}
	if src.ScriptExecutor.Image.DefaultImagePullSecret != "" {
		dst.ScriptExecutor.Image.DefaultImagePullSecret = src.ScriptExecutor.Image.DefaultImagePullSecret
	}
	if len(src.ScriptExecutor.Image.ApprovedImages) > 0 {
		dst.ScriptExecutor.Image.ApprovedImages = src.ScriptExecutor.Image.ApprovedImages
	}
	if len(src.ScriptExecutor.Image.BlockedImages) > 0 {
		dst.ScriptExecutor.Image.BlockedImages = src.ScriptExecutor.Image.BlockedImages
	}
	if src.ScriptExecutor.Security.MaxScriptSize != 0 {
		dst.ScriptExecutor.Security.MaxScriptSize = src.ScriptExecutor.Security.MaxScriptSize
	}
	if src.ScriptExecutor.Security.MaxScriptLines != 0 {
		dst.ScriptExecutor.Security.MaxScriptLines = src.ScriptExecutor.Security.MaxScriptLines
	}
	if src.ScriptExecutor.Approval.Storage.ConfigMapName != "" {
		dst.ScriptExecutor.Approval.Storage.ConfigMapName = src.ScriptExecutor.Approval.Storage.ConfigMapName
	}
	if src.ScriptExecutor.Approval.Storage.ConfigMapNamespace != "" {
		dst.ScriptExecutor.Approval.Storage.ConfigMapNamespace = src.ScriptExecutor.Approval.Storage.ConfigMapNamespace
	}
	if src.ScriptExecutor.Audit.LogFile != "" {
		dst.ScriptExecutor.Audit.LogFile = src.ScriptExecutor.Audit.LogFile
	}
	if src.ScriptExecutor.Monitoring.Metrics.Port != 0 {
		dst.ScriptExecutor.Monitoring.Metrics.Port = src.ScriptExecutor.Monitoring.Metrics.Port
	}
}

func applyEnvOverrides(cfg *Config) {
	if port := os.Getenv("GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.ScriptExecutor.GRPC.Port = p
		}
	}
	if ns := os.Getenv("KUBERNETES_NAMESPACE"); ns != "" {
		cfg.ScriptExecutor.Kubernetes.Namespace = ns
	}
	if img := os.Getenv("DEFAULT_IMAGE"); img != "" {
		cfg.ScriptExecutor.Image.DefaultImage = img
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// DefaultTimeout returns the default execution timeout as a duration.
func (c *Config) DefaultTimeout() time.Duration {
	d, err := time.ParseDuration(c.ScriptExecutor.Security.DefaultTimeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// MaxTimeout returns the maximum allowed timeout.
func (c *Config) MaxTimeout() time.Duration {
	d, err := time.ParseDuration(c.ScriptExecutor.Security.MaxTimeout)
	if err != nil {
		return 30 * time.Minute
	}
	return d
}
