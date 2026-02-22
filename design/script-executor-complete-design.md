# Script Executor - Complete Implementation Design

**Version:** 1.0  
**Date:** 2026-02-20  
**Status:** Ready for Implementation  

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Step Type Specification](#step-type-specification)
4. [Configuration](#configuration)
5. [Image Management](#image-management)
6. [Kubernetes Integration](#kubernetes-integration)
7. [Security](#security)
8. [Approval Workflow](#approval-workflow)
9. [Monitoring & Observability](#monitoring--observability)
10. [Implementation Guide](#implementation-guide)
11. [Examples](#examples)

---

## Overview

### Purpose

Execute shell, Python, and Ruby scripts in a secure, controlled environment with:
- User-provided container images (air-gap friendly)
- Full Kubernetes Job configuration support
- SRE-only access with RBAC
- Comprehensive audit trail
- Approval workflow for sensitive operations

### Key Features

✅ **Air-Gap Friendly** - Use images from internal registries  
✅ **Full K8s Control** - Node selectors, tolerations, affinity, etc.  
✅ **Environment Management** - Env vars from values, secrets, configmaps  
✅ **Security Hardened** - Sandboxed execution, non-root, read-only  
✅ **Approval Workflow** - Manual approval for dangerous operations  
✅ **Audit Trail** - Full logging of script executions  

---

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────┐
│  OpsControlRoom Core                                │
│  ┌───────────────────────────────────────────────┐  │
│  │ Orchestrator                                  │  │
│  └────────────┬──────────────────────────────────┘  │
└───────────────┼─────────────────────────────────────┘
                │ gRPC
                │
┌───────────────▼─────────────────────────────────────┐
│  Script Executor (gRPC Service)                     │
│  ┌──────────────────────────────────────────────┐   │
│  │ Service Layer                                │   │
│  │ - Execute()                                  │   │
│  │ - Describe()                                 │   │
│  │ - Health()                                   │   │
│  └────────────┬─────────────────────────────────┘   │
│               │                                      │
│  ┌────────────▼─────────────────────────────────┐   │
│  │ Execution Manager                            │   │
│  │ - Validate request                           │   │
│  │ - Resolve image                              │   │
│  │ - Check approval                             │   │
│  │ - Create K8s Job                             │   │
│  │ - Monitor execution                          │   │
│  │ - Collect results                            │   │
│  │ - Audit logging                              │   │
│  └────────────┬─────────────────────────────────┘   │
│               │                                      │
│  ┌────────────▼─────────────────────────────────┐   │
│  │ Supporting Components                        │   │
│  │ - Image Catalog                              │   │
│  │ - Approval Store                             │   │
│  │ - RBAC Checker                               │   │
│  │ - Script Validator                           │   │
│  │ - Audit Logger                               │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
                │
                │ K8s API
                │
┌───────────────▼─────────────────────────────────────┐
│  Kubernetes Cluster                                 │
│  ┌──────────────────────────────────────────────┐   │
│  │ Job: script-exec-{execution-id}              │   │
│  │   ├─ Pod: User-specified image               │   │
│  │   ├─ Node Selector: User-specified           │   │
│  │   ├─ Tolerations: User-specified             │   │
│  │   ├─ Env: From values/secrets/configmaps     │   │
│  │   └─ Volumes: Workspace, secrets, configmaps │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

---

## Step Type Specification

### `script.run`

Complete parameter specification with all Kubernetes configuration options.

#### Parameters

```yaml
# ============================================================================
# SCRIPT SOURCE (exactly ONE required)
# ============================================================================

inline_script: string
  # Inline script content
  # Example: |
  #   #!/bin/bash
  #   echo "Hello World"
  
script_path: string
  # Path to pre-registered script in ConfigMap
  # Example: "/scripts/check-db-lag.sh"
  # References: ConfigMap mounted at /scripts

# ============================================================================
# IMAGE CONFIGURATION
# ============================================================================

image: string
  # Full container image reference
  # Example: "harbor.company.internal/sre/aws-tools:v1.0.0"
  # Default: Uses default_image from config
  
image_ref: string
  # Reference to image in catalog
  # Example: "terraform"
  # Looks up full image path from catalog
  
image_pull_policy: string
  # Always | IfNotPresent | Never
  # Default: "IfNotPresent"
  
image_pull_secret: string
  # Name of imagePullSecret in executor namespace
  # Example: "harbor-credentials"
  # Default: Uses default_image_pull_secret from config

# ============================================================================
# INTERPRETER & EXECUTION
# ============================================================================

interpreter: string
  # Interpreter to use
  # Options: /bin/bash | /bin/sh | python3 | ruby
  # Default: "/bin/bash"
  
args: []string
  # Additional arguments passed to script
  # Example: ["--verbose", "--dry-run"]
  
working_dir: string
  # Working directory inside container
  # Default: "/workspace"
  
timeout: duration
  # Maximum execution time
  # Example: "10m", "1h"
  # Default: "5m"
  # Max: "30m" (configurable)

# ============================================================================
# ENVIRONMENT VARIABLES
# ============================================================================

env: map[string]string
  # Environment variables from literal values
  # Example:
  #   AWS_REGION: "us-east-1"
  #   LOG_LEVEL: "debug"

env_from_secret: map[string]SecretRef
  # Environment variables from secrets
  # Example:
  #   DB_PASSWORD:
  #     secret_name: "postgres-credentials"
  #     key: "password"
  #   API_TOKEN:
  #     secret_name: "api-tokens"
  #     key: "github-token"

env_from_configmap: map[string]ConfigMapRef
  # Environment variables from configmaps
  # Example:
  #   APP_CONFIG:
  #     configmap_name: "app-settings"
  #     key: "config.json"

secret_env_all: []string
  # Load ALL keys from secret as env vars
  # Example: ["database-credentials", "api-tokens"]
  # Each key in secret becomes env var

configmap_env_all: []string
  # Load ALL keys from configmap as env vars
  # Example: ["app-config", "feature-flags"]

# ============================================================================
# VOLUME MOUNTS
# ============================================================================

volumes_from_secret: []SecretVolume
  # Mount secrets as files
  # Example:
  #   - secret_name: "tls-certs"
  #     mount_path: "/etc/certs"
  #     optional: false
  #   - secret_name: "ssh-keys"
  #     mount_path: "/root/.ssh"
  #     items:
  #       - key: "id_rsa"
  #         path: "id_rsa"
  #         mode: 0600

volumes_from_configmap: []ConfigMapVolume
  # Mount configmaps as files
  # Example:
  #   - configmap_name: "nginx-config"
  #     mount_path: "/etc/nginx"
  #   - configmap_name: "application-properties"
  #     mount_path: "/config"
  #     items:
  #       - key: "app.properties"
  #         path: "application.properties"

# ============================================================================
# KUBERNETES SCHEDULING
# ============================================================================

node_selector: map[string]string
  # Node selector labels
  # Example:
  #   workload-type: "batch"
  #   disk-type: "ssd"

tolerations: []Toleration
  # Pod tolerations
  # Example:
  #   - key: "dedicated"
  #     operator: "Equal"
  #     value: "batch-jobs"
  #     effect: "NoSchedule"
  #   - key: "gpu"
  #     operator: "Exists"
  #     effect: "NoSchedule"

affinity: Affinity
  # Pod affinity/anti-affinity
  # Example:
  #   node_affinity:
  #     required:
  #       match_expressions:
  #         - key: "zone"
  #           operator: "In"
  #           values: ["us-east-1a", "us-east-1b"]
  #   pod_anti_affinity:
  #     preferred:
  #       - weight: 100
  #         match_expressions:
  #           - key: "app"
  #             operator: "In"
  #             values: ["database"]

priority_class_name: string
  # Pod priority class
  # Example: "high-priority"

# ============================================================================
# RESOURCE LIMITS
# ============================================================================

resources:
  requests:
    cpu: string       # Example: "100m"
    memory: string    # Example: "128Mi"
    
  limits:
    cpu: string       # Example: "1000m"
    memory: string    # Example: "1Gi"
    ephemeral_storage: string  # Example: "2Gi"

# ============================================================================
# SECURITY & APPROVAL
# ============================================================================

approval_required: bool
  # Require manual approval before execution
  # Default: false

approvers: []string
  # List of RBAC groups that can approve
  # Example: ["sre-leads", "platform-team"]

allowed_commands: []string
  # Whitelist of allowed commands (optional)
  # Example: ["kubectl", "aws", "terraform"]
  # If specified, only these commands allowed

blocked_commands: []string
  # Additional blocked commands beyond default
  # Example: ["rm", "dd"]
  # Merged with default blocked list

service_account: string
  # K8s service account to use
  # Default: "script-executor-runner"
  # Must exist in executor namespace

# ============================================================================
# ADVANCED OPTIONS
# ============================================================================

ttl_seconds_after_finished: int
  # Time to keep job after completion
  # Example: 3600 (1 hour)
  # Default: 300 (5 minutes)

backoff_limit: int
  # Number of retries on failure
  # Default: 0 (no retries)
  # Max: 3

stdin: string
  # Input data passed to script via stdin
  # Example: |
  #   {"key": "value"}

labels: map[string]string
  # Additional labels for Job/Pod
  # Example:
  #   team: "platform"
  #   cost-center: "engineering"

annotations: map[string]string
  # Additional annotations for Job/Pod
  # Example:
  #   description: "Database maintenance"
```

#### Output

```yaml
exit_code: int
  # Script exit code
  # 0 = success, non-zero = failure

stdout: string
  # Standard output from script

stderr: string
  # Standard error from script

duration_seconds: float
  # Execution time in seconds

script_hash: string
  # SHA256 hash of executed script

approved_by: string
  # User who approved (if approval required)

approved_at: timestamp
  # When approved (if approval required)

resource_usage:
  cpu_millis: int64
    # CPU used in millicores
  memory_mb: int64
    # Memory used in MB
  
job_name: string
  # Kubernetes Job name created

pod_name: string
  # Kubernetes Pod name created
```

---

## Configuration

### Complete Configuration File

```yaml
# config/script-executor.yaml

script_executor:
  # =========================================================================
  # GRPC SERVER
  # =========================================================================
  grpc:
    port: 50051
    max_message_size: 10485760  # 10MB
    
  # =========================================================================
  # KUBERNETES
  # =========================================================================
  kubernetes:
    namespace: "opscontrolroom-system"
    service_account: "script-executor-runner"
    
    # Job defaults
    job_defaults:
      ttl_seconds_after_finished: 300  # 5 minutes
      backoff_limit: 0                 # No retries
      active_deadline_seconds: 1800    # 30 minutes max
    
    # Resource defaults
    default_resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
        ephemeral_storage: "1Gi"
    
    # Maximum resource limits (hard cap)
    max_resources:
      limits:
        cpu: "4000m"           # 4 cores max
        memory: "8Gi"          # 8GB max
        ephemeral_storage: "20Gi"  # 20GB max
  
  # =========================================================================
  # IMAGE CONFIGURATION
  # =========================================================================
  image:
    # Default image if none specified
    default_image: "harbor.company.internal/sre/base:v1.0.0"
    default_image_pull_secret: "harbor-credentials"
    default_image_pull_policy: "IfNotPresent"
    
    # Image validation
    approved_images:
      - "harbor.company.internal/sre/*"
      - "harbor.company.internal/engineering/*"
      - "gcr.io/company-prod/*"
      - "hashicorp/terraform:1.7.*"
    
    blocked_images:
      - "docker.io/*"        # Public Docker Hub
      - "quay.io/*"         # Public Quay
      - "ghcr.io/*"         # GitHub Container Registry
    
    # Image catalog
    catalog:
      enabled: true
      configmap_name: "script-image-catalog"
      configmap_namespace: "opscontrolroom-system"
    
    # Image verification
    verify_image_access: true      # Check if image is pullable
    scan_images_on_approval: true  # Scan with image-scanner-executor
  
  # =========================================================================
  # SECURITY
  # =========================================================================
  security:
    # RBAC
    required_permission: "executors.use.script"
    
    # Command filtering
    blocked_commands:
      # Destructive
      - "rm"
      - "dd"
      - "mkfs"
      - "fdisk"
      - "mkswap"
      # Privilege escalation
      - "sudo"
      - "su"
      - "setuid"
      - "chmod +s"
      # System modification
      - "reboot"
      - "shutdown"
      - "init"
      - "systemctl"
      # Network attacks
      - "nmap"
      - "masscan"
      # Process manipulation
      - "kill -9"
      - "killall -9"
      - "pkill -9"
    
    # Script limits
    max_script_size: 524288      # 500KB
    max_script_lines: 1000
    
    # Execution limits
    default_timeout: "5m"
    max_timeout: "30m"
    
    # Pod security
    run_as_non_root: true
    run_as_user: 65534           # nobody
    fs_group: 65534
    read_only_root_filesystem: true
    allow_privilege_escalation: false
    drop_all_capabilities: true
  
  # =========================================================================
  # APPROVAL WORKFLOW
  # =========================================================================
  approval:
    enabled: true
    
    # Storage backend for approval requests
    storage:
      type: "configmap"          # configmap | postgres | redis
      configmap_name: "script-approvals"
      configmap_namespace: "opscontrolroom-system"
    
    # Approval timeout
    approval_timeout: "24h"
    
    # Default approvers (if not specified in step)
    default_approvers:
      - "sre-leads"
    
    # Auto-approve certain patterns
    auto_approve:
      enabled: false
      rules:
        - user_group: "sre-leads"
          script_pattern: "^#!/bin/bash\necho"  # Only echo commands
  
  # =========================================================================
  # AUDIT & LOGGING
  # =========================================================================
  audit:
    enabled: true
    
    # Audit log file
    log_file: "/var/log/ocr/script-audit.log"
    
    # SIEM integration
    siem:
      enabled: false
      endpoint: "https://siem.company.com/api/v1/events"
      token_secret:
        name: "siem-token"
        key: "token"
    
    # What to log
    log_script_content: true     # Log full script
    log_script_output: true      # Log stdout/stderr
    log_environment: false       # Don't log env vars (may contain secrets)
  
  # =========================================================================
  # MONITORING
  # =========================================================================
  monitoring:
    metrics:
      enabled: true
      port: 9090
      path: "/metrics"
    
    # Prometheus pushgateway (optional)
    pushgateway:
      enabled: false
      url: "http://pushgateway:9091"
```

---

## Image Management

### Image Catalog

**ConfigMap: `script-image-catalog`**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: script-image-catalog
  namespace: opscontrolroom-system
data:
  catalog.yaml: |
    # =====================================================================
    # GENERAL PURPOSE
    # =====================================================================
    base:
      image: "harbor.company.internal/sre/base:v1.0.0"
      pull_secret: "harbor-credentials"
      description: "Minimal tools: bash, python3, ruby, curl, jq, git"
      tools:
        - bash
        - python3
        - ruby
        - curl
        - jq
        - git
      approved_by: "john.doe@company.com"
      approved_at: "2026-02-15T10:00:00Z"
    
    # =====================================================================
    # CLOUD PROVIDERS
    # =====================================================================
    aws:
      image: "harbor.company.internal/sre/aws-tools:v1.0.0"
      pull_secret: "harbor-credentials"
      description: "AWS CLI, kubectl, helm, eksctl"
      tools:
        - bash
        - aws-cli
        - kubectl
        - helm
        - eksctl
        - jq
      approved_by: "jane.smith@company.com"
      approved_at: "2026-02-16T14:30:00Z"
    
    gcp:
      image: "harbor.company.internal/sre/gcp-tools:v1.0.0"
      pull_secret: "harbor-credentials"
      description: "gcloud SDK, kubectl, helm"
      tools:
        - bash
        - gcloud
        - kubectl
        - helm
        - jq
    
    azure:
      image: "harbor.company.internal/sre/azure-tools:v1.0.0"
      pull_secret: "harbor-credentials"
      description: "Azure CLI, kubectl, helm"
      tools:
        - bash
        - az
        - kubectl
        - helm
        - jq
    
    # =====================================================================
    # INFRASTRUCTURE AS CODE
    # =====================================================================
    terraform:
      image: "harbor.company.internal/sre/terraform:v1.7.0"
      pull_secret: "harbor-credentials"
      description: "Terraform 1.7.0, AWS CLI, kubectl"
      tools:
        - terraform
        - aws-cli
        - kubectl
        - jq
      approved_by: "infra-team@company.com"
      approved_at: "2026-02-18T09:00:00Z"
    
    terraform-1.6:
      image: "harbor.company.internal/sre/terraform:v1.6.0"
      pull_secret: "harbor-credentials"
      description: "Terraform 1.6.0 (for legacy projects)"
      tools:
        - terraform
        - aws-cli
        - kubectl
    
    ansible:
      image: "harbor.company.internal/sre/ansible:v2.15.0"
      pull_secret: "harbor-credentials"
      description: "Ansible 2.15, Python 3.11, cloud CLIs"
      tools:
        - ansible
        - ansible-playbook
        - python3
        - aws-cli
        - jq
    
    # =====================================================================
    # DATABASES
    # =====================================================================
    database:
      image: "harbor.company.internal/sre/database-tools:v2.0.0"
      pull_secret: "harbor-credentials"
      description: "PostgreSQL, MySQL, MongoDB, Redis clients"
      tools:
        - psql
        - pg_dump
        - mysql
        - mysqldump
        - mongosh
        - mongodump
        - redis-cli
        - jq
    
    postgres:
      image: "postgres:16-alpine"
      pull_secret: "dockerhub-credentials"
      description: "PostgreSQL 16 client tools only"
      tools:
        - psql
        - pg_dump
        - pg_restore
    
    # =====================================================================
    # MONITORING & OBSERVABILITY
    # =====================================================================
    prometheus:
      image: "harbor.company.internal/sre/prometheus-tools:v1.0.0"
      pull_secret: "harbor-credentials"
      description: "Prometheus, Grafana, and alerting tools"
      tools:
        - promtool
        - amtool
        - grafana-cli
        - jq
    
    # =====================================================================
    # CUSTOM COMPANY TOOLS
    # =====================================================================
    company-validator:
      image: "harbor.company.internal/engineering/validator:v3.2.1"
      pull_secret: "harbor-credentials"
      description: "Company's proprietary infrastructure validator"
      tools:
        - infrastructure-validator
        - compliance-checker
      approved_by: "platform-team@company.com"
      approved_at: "2026-02-19T16:45:00Z"
```

---

## Kubernetes Integration

### Job Template with All Configuration Options

```go
// internal/executor/job_builder.go

package executor

import (
    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/utils/ptr"
)

type JobBuilder struct {
    config *Config
}

func (b *JobBuilder) BuildJob(ctx ExecutionContext) (*batchv1.Job, error) {
    jobName := fmt.Sprintf("script-exec-%s", ctx.ExecutionID)
    
    job := &batchv1.Job{
        ObjectMeta: b.buildJobMetadata(jobName, ctx),
        Spec: batchv1.JobSpec{
            // Backoff limit
            BackoffLimit: ptr.Int32(ctx.BackoffLimit),
            
            // TTL after finished
            TTLSecondsAfterFinished: ptr.Int32(ctx.TTLSecondsAfterFinished),
            
            // Active deadline (max runtime)
            ActiveDeadlineSeconds: ptr.Int64(int64(ctx.Timeout.Seconds())),
            
            Template: corev1.PodTemplateSpec{
                ObjectMeta: b.buildPodMetadata(ctx),
                Spec: b.buildPodSpec(ctx),
            },
        },
    }
    
    return job, nil
}

func (b *JobBuilder) buildJobMetadata(jobName string, ctx ExecutionContext) metav1.ObjectMeta {
    labels := map[string]string{
        "executor":      "script",
        "execution-id":  ctx.ExecutionID,
        "runbook-id":    ctx.RunbookID,
        "user":          sanitizeLabel(ctx.User),
        "managed-by":    "opscontrolroom",
    }
    
    // Merge user-provided labels
    for k, v := range ctx.Labels {
        labels[k] = v
    }
    
    annotations := map[string]string{
        "script-hash":      ctx.ScriptHash,
        "execution-id":     ctx.ExecutionID,
        "runbook-id":       ctx.RunbookID,
        "user":             ctx.User,
        "image":            ctx.Image,
        "created-by":       "script-executor",
    }
    
    // Merge user-provided annotations
    for k, v := range ctx.Annotations {
        annotations[k] = v
    }
    
    return metav1.ObjectMeta{
        Name:        jobName,
        Namespace:   b.config.Namespace,
        Labels:      labels,
        Annotations: annotations,
    }
}

func (b *JobBuilder) buildPodMetadata(ctx ExecutionContext) metav1.ObjectMeta {
    labels := map[string]string{
        "executor":     "script",
        "execution-id": ctx.ExecutionID,
    }
    
    for k, v := range ctx.Labels {
        labels[k] = v
    }
    
    return metav1.ObjectMeta{
        Labels:      labels,
        Annotations: ctx.Annotations,
    }
}

func (b *JobBuilder) buildPodSpec(ctx ExecutionContext) corev1.PodSpec {
    spec := corev1.PodSpec{
        RestartPolicy: corev1.RestartPolicyNever,
        
        // Service account
        ServiceAccountName: ctx.ServiceAccount,
        
        // Image pull secrets
        ImagePullSecrets: []corev1.LocalObjectReference{
            {Name: ctx.ImagePullSecret},
        },
        
        // Security context (pod level)
        SecurityContext: &corev1.PodSecurityContext{
            RunAsNonRoot: ptr.Bool(b.config.Security.RunAsNonRoot),
            RunAsUser:    ptr.Int64(b.config.Security.RunAsUser),
            FSGroup:      ptr.Int64(b.config.Security.FSGroup),
            SeccompProfile: &corev1.SeccompProfile{
                Type: corev1.SeccompProfileTypeRuntimeDefault,
            },
        },
        
        // Node selector
        NodeSelector: ctx.NodeSelector,
        
        // Tolerations
        Tolerations: ctx.Tolerations,
        
        // Affinity
        Affinity: ctx.Affinity,
        
        // Priority class
        PriorityClassName: ctx.PriorityClassName,
        
        // Containers
        Containers: []corev1.Container{
            b.buildScriptContainer(ctx),
        },
        
        // Volumes
        Volumes: b.buildVolumes(ctx),
    }
    
    return spec
}

func (b *JobBuilder) buildScriptContainer(ctx ExecutionContext) corev1.Container {
    return corev1.Container{
        Name:            "script",
        Image:           ctx.Image,
        ImagePullPolicy: ctx.ImagePullPolicy,
        
        // Command
        Command: []string{ctx.Interpreter, "-c", ctx.Script},
        Args:    ctx.Args,
        
        // Working directory
        WorkingDir: ctx.WorkingDir,
        
        // Environment variables
        Env: b.buildEnvVars(ctx),
        
        // EnvFrom (secrets and configmaps)
        EnvFrom: b.buildEnvFrom(ctx),
        
        // Security context (container level)
        SecurityContext: &corev1.SecurityContext{
            AllowPrivilegeEscalation: ptr.Bool(false),
            ReadOnlyRootFilesystem:   ptr.Bool(b.config.Security.ReadOnlyRootFilesystem),
            RunAsNonRoot:             ptr.Bool(b.config.Security.RunAsNonRoot),
            RunAsUser:                ptr.Int64(b.config.Security.RunAsUser),
            Capabilities: &corev1.Capabilities{
                Drop: []corev1.Capability{"ALL"},
            },
        },
        
        // Resources
        Resources: ctx.Resources,
        
        // Volume mounts
        VolumeMounts: b.buildVolumeMounts(ctx),
        
        // Stdin
        Stdin: ctx.Stdin != "",
        StdinOnce: ctx.Stdin != "",
    }
}

func (b *JobBuilder) buildEnvVars(ctx ExecutionContext) []corev1.EnvVar {
    envVars := []corev1.EnvVar{}
    
    // 1. Literal environment variables
    for key, val := range ctx.Env {
        envVars = append(envVars, corev1.EnvVar{
            Name:  key,
            Value: val,
        })
    }
    
    // 2. Environment variables from secrets
    for envName, secretRef := range ctx.EnvFromSecret {
        envVars = append(envVars, corev1.EnvVar{
            Name: envName,
            ValueFrom: &corev1.EnvVarSource{
                SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{
                        Name: secretRef.SecretName,
                    },
                    Key:      secretRef.Key,
                    Optional: ptr.Bool(secretRef.Optional),
                },
            },
        })
    }
    
    // 3. Environment variables from configmaps
    for envName, cmRef := range ctx.EnvFromConfigMap {
        envVars = append(envVars, corev1.EnvVar{
            Name: envName,
            ValueFrom: &corev1.EnvVarSource{
                ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{
                        Name: cmRef.ConfigMapName,
                    },
                    Key:      cmRef.Key,
                    Optional: ptr.Bool(cmRef.Optional),
                },
            },
        })
    }
    
    return envVars
}

func (b *JobBuilder) buildEnvFrom(ctx ExecutionContext) []corev1.EnvFromSource {
    envFrom := []corev1.EnvFromSource{}
    
    // All keys from secrets
    for _, secretName := range ctx.SecretEnvAll {
        envFrom = append(envFrom, corev1.EnvFromSource{
            SecretRef: &corev1.SecretEnvSource{
                LocalObjectReference: corev1.LocalObjectReference{
                    Name: secretName,
                },
            },
        })
    }
    
    // All keys from configmaps
    for _, cmName := range ctx.ConfigMapEnvAll {
        envFrom = append(envFrom, corev1.EnvFromSource{
            ConfigMapRef: &corev1.ConfigMapEnvSource{
                LocalObjectReference: corev1.LocalObjectReference{
                    Name: cmName,
                },
            },
        })
    }
    
    return envFrom
}

func (b *JobBuilder) buildVolumes(ctx ExecutionContext) []corev1.Volume {
    volumes := []corev1.Volume{
        // Workspace (always present)
        {
            Name: "workspace",
            VolumeSource: corev1.VolumeSource{
                EmptyDir: &corev1.EmptyDirVolumeSource{
                    SizeLimit: ctx.Resources.Limits.StorageEphemeral(),
                },
            },
        },
        // Tmp (always present)
        {
            Name: "tmp",
            VolumeSource: corev1.VolumeSource{
                EmptyDir: &corev1.EmptyDirVolumeSource{
                    SizeLimit: resource.MustParse("100Mi"),
                },
            },
        },
    }
    
    // Volumes from secrets
    for i, secretVol := range ctx.VolumesFromSecret {
        volumeName := fmt.Sprintf("secret-%d", i)
        
        volume := corev1.Volume{
            Name: volumeName,
            VolumeSource: corev1.VolumeSource{
                Secret: &corev1.SecretVolumeSource{
                    SecretName:  secretVol.SecretName,
                    Optional:    ptr.Bool(secretVol.Optional),
                    DefaultMode: secretVol.DefaultMode,
                },
            },
        }
        
        // Specific items from secret
        if len(secretVol.Items) > 0 {
            volume.VolumeSource.Secret.Items = []corev1.KeyToPath{}
            for _, item := range secretVol.Items {
                volume.VolumeSource.Secret.Items = append(
                    volume.VolumeSource.Secret.Items,
                    corev1.KeyToPath{
                        Key:  item.Key,
                        Path: item.Path,
                        Mode: item.Mode,
                    },
                )
            }
        }
        
        volumes = append(volumes, volume)
    }
    
    // Volumes from configmaps
    for i, cmVol := range ctx.VolumesFromConfigMap {
        volumeName := fmt.Sprintf("configmap-%d", i)
        
        volume := corev1.Volume{
            Name: volumeName,
            VolumeSource: corev1.VolumeSource{
                ConfigMap: &corev1.ConfigMapVolumeSource{
                    LocalObjectReference: corev1.LocalObjectReference{
                        Name: cmVol.ConfigMapName,
                    },
                    Optional:    ptr.Bool(cmVol.Optional),
                    DefaultMode: cmVol.DefaultMode,
                },
            },
        }
        
        // Specific items from configmap
        if len(cmVol.Items) > 0 {
            volume.VolumeSource.ConfigMap.Items = []corev1.KeyToPath{}
            for _, item := range cmVol.Items {
                volume.VolumeSource.ConfigMap.Items = append(
                    volume.VolumeSource.ConfigMap.Items,
                    corev1.KeyToPath{
                        Key:  item.Key,
                        Path: item.Path,
                        Mode: item.Mode,
                    },
                )
            }
        }
        
        volumes = append(volumes, volume)
    }
    
    return volumes
}

func (b *JobBuilder) buildVolumeMounts(ctx ExecutionContext) []corev1.VolumeMount {
    mounts := []corev1.VolumeMount{
        // Workspace
        {
            Name:      "workspace",
            MountPath: "/workspace",
        },
        // Tmp
        {
            Name:      "tmp",
            MountPath: "/tmp",
        },
    }
    
    // Secret volumes
    for i, secretVol := range ctx.VolumesFromSecret {
        mounts = append(mounts, corev1.VolumeMount{
            Name:      fmt.Sprintf("secret-%d", i),
            MountPath: secretVol.MountPath,
            ReadOnly:  true,
            SubPath:   secretVol.SubPath,
        })
    }
    
    // ConfigMap volumes
    for i, cmVol := range ctx.VolumesFromConfigMap {
        mounts = append(mounts, corev1.VolumeMount{
            Name:      fmt.Sprintf("configmap-%d", i),
            MountPath: cmVol.MountPath,
            ReadOnly:  true,
            SubPath:   cmVol.SubPath,
        })
    }
    
    return mounts
}
```

---

## Security

### Command Validation

```go
// internal/security/script_validator.go

package security

import (
    "bufio"
    "fmt"
    "regexp"
    "strings"
)

type ScriptValidator struct {
    blockedCommands []string
    allowedCommands []string  // If set, only these allowed
}

func NewScriptValidator(blocked, allowed []string) *ScriptValidator {
    return &ScriptValidator{
        blockedCommands: blocked,
        allowedCommands: allowed,
    }
}

func (v *ScriptValidator) Validate(script string) error {
    // 1. Check script size
    if len(script) > 524288 { // 500KB
        return fmt.Errorf("script too large: %d bytes (max: 500KB)", len(script))
    }
    
    // 2. Check line count
    lines := strings.Split(script, "\n")
    if len(lines) > 1000 {
        return fmt.Errorf("script too long: %d lines (max: 1000)", len(lines))
    }
    
    // 3. Extract commands from script
    commands := v.extractCommands(script)
    
    // 4. Check against blocked commands
    for _, cmd := range commands {
        for _, blocked := range v.blockedCommands {
            if v.matchesCommand(cmd, blocked) {
                return fmt.Errorf("blocked command detected: %s", cmd)
            }
        }
    }
    
    // 5. If whitelist exists, check commands are allowed
    if len(v.allowedCommands) > 0 {
        for _, cmd := range commands {
            allowed := false
            for _, allowedCmd := range v.allowedCommands {
                if v.matchesCommand(cmd, allowedCmd) {
                    allowed = true
                    break
                }
            }
            if !allowed {
                return fmt.Errorf("command not in allowlist: %s", cmd)
            }
        }
    }
    
    return nil
}

func (v *ScriptValidator) extractCommands(script string) []string {
    commands := []string{}
    scanner := bufio.NewScanner(strings.NewReader(script))
    
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        
        // Skip comments and empty lines
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        
        // Extract command (first word)
        fields := strings.Fields(line)
        if len(fields) > 0 {
            cmd := fields[0]
            
            // Handle pipes and command chains
            cmd = strings.TrimPrefix(cmd, "|")
            cmd = strings.TrimPrefix(cmd, "&&")
            cmd = strings.TrimPrefix(cmd, "||")
            cmd = strings.TrimPrefix(cmd, ";")
            
            // Handle sudo/su prefixes
            if cmd == "sudo" || cmd == "su" {
                if len(fields) > 1 {
                    cmd = fields[1]
                }
            }
            
            commands = append(commands, cmd)
        }
    }
    
    return unique(commands)
}

func (v *ScriptValidator) matchesCommand(cmd, pattern string) bool {
    // Exact match
    if cmd == pattern {
        return true
    }
    
    // Wildcard match
    if strings.HasSuffix(pattern, "*") {
        prefix := strings.TrimSuffix(pattern, "*")
        return strings.HasPrefix(cmd, prefix)
    }
    
    // Regex match
    if strings.HasPrefix(pattern, "regex:") {
        regex := strings.TrimPrefix(pattern, "regex:")
        matched, _ := regexp.MatchString(regex, cmd)
        return matched
    }
    
    return false
}

func unique(slice []string) []string {
    keys := make(map[string]bool)
    list := []string{}
    
    for _, entry := range slice {
        if _, value := keys[entry]; !value {
            keys[entry] = true
            list = append(list, entry)
        }
    }
    
    return list
}
```

---

## Approval Workflow

### Approval Store Implementation

```go
// internal/approval/store.go

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

type ApprovalRequest struct {
    ID            string    `json:"id"`
    ExecutionID   string    `json:"execution_id"`
    StepName      string    `json:"step_name"`
    RunbookID     string    `json:"runbook_id"`
    User          string    `json:"user"`
    
    Script        string    `json:"script"`
    ScriptHash    string    `json:"script_hash"`
    
    Approvers     []string  `json:"approvers"`
    
    Status        string    `json:"status"`  // pending|approved|denied|expired
    ApprovedBy    string    `json:"approved_by,omitempty"`
    ApprovedAt    time.Time `json:"approved_at,omitempty"`
    DeniedBy      string    `json:"denied_by,omitempty"`
    DeniedAt      time.Time `json:"denied_at,omitempty"`
    DenialReason  string    `json:"denial_reason,omitempty"`
    
    CreatedAt     time.Time `json:"created_at"`
    ExpiresAt     time.Time `json:"expires_at"`
}

type ConfigMapApprovalStore struct {
    client        kubernetes.Interface
    namespace     string
    configMapName string
}

func NewConfigMapStore(client kubernetes.Interface, namespace, cmName string) *ConfigMapApprovalStore {
    return &ConfigMapApprovalStore{
        client:        client,
        namespace:     namespace,
        configMapName: cmName,
    }
}

func (s *ConfigMapApprovalStore) Create(ctx context.Context, req *ApprovalRequest) error {
    // Generate ID
    req.ID = generateID()
    req.Status = "pending"
    req.CreatedAt = time.Now()
    req.ExpiresAt = req.CreatedAt.Add(24 * time.Hour)
    
    // Serialize
    data, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("failed to serialize approval request: %w", err)
    }
    
    // Get or create ConfigMap
    cm, err := s.getOrCreateConfigMap(ctx)
    if err != nil {
        return err
    }
    
    // Add to ConfigMap
    if cm.Data == nil {
        cm.Data = make(map[string]string)
    }
    
    key := fmt.Sprintf("%s-%s", req.ExecutionID, req.StepName)
    cm.Data[key] = string(data)
    
    // Update ConfigMap
    _, err = s.client.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
    return err
}

func (s *ConfigMapApprovalStore) Get(ctx context.Context, executionID, stepName string) (*ApprovalRequest, error) {
    cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.configMapName, metav1.GetOptions{})
    if err != nil {
        return nil, err
    }
    
    key := fmt.Sprintf("%s-%s", executionID, stepName)
    data, ok := cm.Data[key]
    if !ok {
        return nil, fmt.Errorf("approval request not found")
    }
    
    var req ApprovalRequest
    if err := json.Unmarshal([]byte(data), &req); err != nil {
        return nil, err
    }
    
    // Check if expired
    if time.Now().After(req.ExpiresAt) && req.Status == "pending" {
        req.Status = "expired"
        s.Update(ctx, &req)
    }
    
    return &req, nil
}

func (s *ConfigMapApprovalStore) Approve(ctx context.Context, executionID, stepName, approver string) error {
    req, err := s.Get(ctx, executionID, stepName)
    if err != nil {
        return err
    }
    
    if req.Status != "pending" {
        return fmt.Errorf("cannot approve: status is %s", req.Status)
    }
    
    req.Status = "approved"
    req.ApprovedBy = approver
    req.ApprovedAt = time.Now()
    
    return s.Update(ctx, req)
}

func (s *ConfigMapApprovalStore) Deny(ctx context.Context, executionID, stepName, denier, reason string) error {
    req, err := s.Get(ctx, executionID, stepName)
    if err != nil {
        return err
    }
    
    if req.Status != "pending" {
        return fmt.Errorf("cannot deny: status is %s", req.Status)
    }
    
    req.Status = "denied"
    req.DeniedBy = denier
    req.DeniedAt = time.Now()
    req.DenialReason = reason
    
    return s.Update(ctx, req)
}

func (s *ConfigMapApprovalStore) Update(ctx context.Context, req *ApprovalRequest) error {
    cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.configMapName, metav1.GetOptions{})
    if err != nil {
        return err
    }
    
    data, err := json.Marshal(req)
    if err != nil {
        return err
    }
    
    key := fmt.Sprintf("%s-%s", req.ExecutionID, req.StepName)
    cm.Data[key] = string(data)
    
    _, err = s.client.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
    return err
}

func (s *ConfigMapApprovalStore) getOrCreateConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
    cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.configMapName, metav1.GetOptions{})
    if err == nil {
        return cm, nil
    }
    
    // Create if doesn't exist
    cm = &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      s.configMapName,
            Namespace: s.namespace,
        },
        Data: make(map[string]string),
    }
    
    return s.client.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
}
```

### Approval API

```go
// internal/api/approval_handler.go

package api

import (
    "encoding/json"
    "net/http"
)

type ApprovalHandler struct {
    store approval.Store
}

// GET /api/v1/approvals/pending
func (h *ApprovalHandler) ListPendingApprovals(w http.ResponseWriter, r *http.Request) {
    // Get user from context
    user := r.Context().Value("user").(string)
    
    // Get pending approvals where user is an approver
    approvals, err := h.store.ListPendingForApprover(r.Context(), user)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(approvals)
}

// POST /api/v1/approvals/{execution_id}/{step_name}/approve
func (h *ApprovalHandler) ApproveRequest(w http.ResponseWriter, r *http.Request) {
    executionID := mux.Vars(r)["execution_id"]
    stepName := mux.Vars(r)["step_name"]
    user := r.Context().Value("user").(string)
    
    // Check if user is authorized approver
    req, err := h.store.Get(r.Context(), executionID, stepName)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    
    if !contains(req.Approvers, user) {
        http.Error(w, "not authorized to approve", http.StatusForbidden)
        return
    }
    
    // Approve
    if err := h.store.Approve(r.Context(), executionID, stepName, user); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "approved",
    })
}

// POST /api/v1/approvals/{execution_id}/{step_name}/deny
func (h *ApprovalHandler) DenyRequest(w http.ResponseWriter, r *http.Request) {
    executionID := mux.Vars(r)["execution_id"]
    stepName := mux.Vars(r)["step_name"]
    user := r.Context().Value("user").(string)
    
    var body struct {
        Reason string `json:"reason"`
    }
    json.NewDecoder(r.Body).Decode(&body)
    
    // Check authorization
    req, err := h.store.Get(r.Context(), executionID, stepName)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    
    if !contains(req.Approvers, user) {
        http.Error(w, "not authorized to deny", http.StatusForbidden)
        return
    }
    
    // Deny
    if err := h.store.Deny(r.Context(), executionID, stepName, user, body.Reason); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "denied",
    })
}
```

---

## Monitoring & Observability

### Prometheus Metrics

```go
// internal/metrics/metrics.go

package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Execution metrics
    ExecutionsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "script_executor_executions_total",
            Help: "Total number of script executions",
        },
        []string{"interpreter", "status", "image_ref"},
    )
    
    ExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "script_executor_execution_duration_seconds",
            Help:    "Script execution duration",
            Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~1h
        },
        []string{"interpreter", "image_ref"},
    )
    
    ExecutionErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "script_executor_execution_errors_total",
            Help: "Total script execution errors",
        },
        []string{"error_type", "interpreter"},
    )
    
    // Resource usage
    CPUUsageMillis = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "script_executor_cpu_usage_millis",
            Help:    "CPU usage in millicores",
            Buckets: prometheus.ExponentialBuckets(10, 2, 12), // 10m to ~40 cores
        },
        []string{"image_ref"},
    )
    
    MemoryUsageMB = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "script_executor_memory_usage_mb",
            Help:    "Memory usage in MB",
            Buckets: prometheus.ExponentialBuckets(16, 2, 12), // 16MB to 64GB
        },
        []string{"image_ref"},
    )
    
    // Approval workflow
    ApprovalsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "script_executor_approvals_total",
            Help: "Total approval requests",
        },
        []string{"status"}, // pending|approved|denied|expired
    )
    
    ApprovalDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "script_executor_approval_duration_seconds",
            Help:    "Time to approval",
            Buckets: prometheus.ExponentialBuckets(60, 2, 12), // 1min to ~68 hours
        },
    )
    
    // Image usage
    ImagePulls = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "script_executor_image_pulls_total",
            Help: "Total image pulls",
        },
        []string{"image", "status"},
    )
    
    ImagePullDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "script_executor_image_pull_duration_seconds",
            Help:    "Image pull duration",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~17min
        },
        []string{"image"},
    )
)
```

### Structured Logging

```go
// internal/logging/logger.go

package logging

import (
    "os"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

func InitLogger(config LogConfig) {
    // Console output
    zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
    
    if config.Pretty {
        log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
    }
    
    // Set level
    level, err := zerolog.ParseLevel(config.Level)
    if err != nil {
        level = zerolog.InfoLevel
    }
    zerolog.SetGlobalLevel(level)
}

// Audit logger (separate from main logger)
func NewAuditLogger(path string) (zerolog.Logger, error) {
    file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return zerolog.Logger{}, err
    }
    
    return zerolog.New(file).With().
        Timestamp().
        Str("audit_type", "script_execution").
        Logger(), nil
}

// Log execution events
func LogExecutionStart(executionID, user, runbookID, image string) {
    log.Info().
        Str("event", "execution_start").
        Str("execution_id", executionID).
        Str("user", user).
        Str("runbook_id", runbookID).
        Str("image", image).
        Msg("Script execution started")
}

func LogExecutionComplete(executionID string, duration float64, exitCode int, status string) {
    log.Info().
        Str("event", "execution_complete").
        Str("execution_id", executionID).
        Float64("duration_seconds", duration).
        Int("exit_code", exitCode).
        Str("status", status).
        Msg("Script execution completed")
}

func LogApprovalRequest(executionID, stepName, user string, approvers []string) {
    log.Info().
        Str("event", "approval_requested").
        Str("execution_id", executionID).
        Str("step_name", stepName).
        Str("user", user).
        Strs("approvers", approvers).
        Msg("Approval request created")
}

func LogApprovalGranted(executionID, stepName, approver string) {
    log.Info().
        Str("event", "approval_granted").
        Str("execution_id", executionID).
        Str("step_name", stepName).
        Str("approved_by", approver).
        Msg("Approval granted")
}
```

---

## Implementation Guide

### Repository Structure

```
script-executor/
├── cmd/
│   └── script-executor/
│       └── main.go
├── internal/
│   ├── service/
│   │   ├── executor.go
│   │   ├── executor_test.go
│   │   └── health.go
│   ├── execution/
│   │   ├── manager.go
│   │   ├── job_builder.go
│   │   ├── monitor.go
│   │   └── result_collector.go
│   ├── image/
│   │   ├── catalog.go
│   │   ├── validator.go
│   │   └── resolver.go
│   ├── approval/
│   │   ├── store.go
│   │   └── checker.go
│   ├── security/
│   │   ├── rbac.go
│   │   └── script_validator.go
│   ├── config/
│   │   └── config.go
│   ├── metrics/
│   │   └── metrics.go
│   └── api/
│       ├── approval_handler.go
│       └── server.go
├── deploy/
│   └── k8s/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── rbac.yaml
│       ├── configmap.yaml
│       └── secrets.yaml.template
├── Dockerfile
├── Makefile
├── go.mod
└── README.md
```

---

## Examples

### Example 1: Simple Script with Environment Variables

```yaml
id: ops.check-api-status
name: "Check API Status"

steps:
  - name: check-status
    type: script.run
    with:
      image_ref: "base"
      
      inline_script: |
        #!/bin/bash
        set -euo pipefail
        
        echo "Checking API at $API_URL"
        
        STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/health")
        
        if [ "$STATUS" = "200" ]; then
          echo "✓ API is healthy"
          exit 0
        else
          echo "✗ API returned status $STATUS"
          exit 1
        fi
      
      env:
        API_URL: "https://api.company.com"
      
      timeout: "30s"
```

---

### Example 2: Database Maintenance with Secrets

```yaml
id: ops.db-maintenance
name: "Database Maintenance"

steps:
  - name: vacuum-database
    type: script.run
    with:
      image_ref: "postgres"
      
      inline_script: |
        #!/bin/bash
        set -euo pipefail
        
        echo "Running VACUUM ANALYZE on database..."
        
        psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "VACUUM ANALYZE;"
        
        echo "✓ VACUUM complete"
      
      # Environment from secret
      env_from_secret:
        DB_PASSWORD:
          secret_name: "postgres-credentials"
          key: "password"
      
      env:
        DB_HOST: "postgres.database.svc.cluster.local"
        DB_USER: "admin"
        DB_NAME: "production"
      
      timeout: "10m"
```

---

### Example 3: Terraform with Node Selector

```yaml
id: infra.terraform-apply
name: "Apply Terraform Changes"

steps:
  - name: terraform-apply
    type: script.run
    with:
      image_ref: "terraform"
      
      inline_script: |
        #!/bin/bash
        set -euo pipefail
        
        cd /workspace/terraform
        
        terraform init
        terraform plan -out=plan.tfplan
        terraform apply -auto-approve plan.tfplan
      
      # Mount Terraform code from ConfigMap
      volumes_from_configmap:
        - configmap_name: "terraform-code"
          mount_path: "/workspace/terraform"
      
      # Mount AWS credentials from Secret
      volumes_from_secret:
        - secret_name: "aws-credentials"
          mount_path: "/root/.aws"
          items:
            - key: "credentials"
              path: "credentials"
              mode: 0600
      
      # Run on specific nodes
      node_selector:
        workload-type: "terraform"
        disk-type: "ssd"
      
      # Resources
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
        limits:
          cpu: "2000m"
          memory: "2Gi"
      
      timeout: "15m"
      
      # Require approval
      approval_required: true
      approvers:
        - "sre-leads"
        - "platform-team"
```

---

### Example 4: Multi-Cloud with Tolerations

```yaml
id: ops.multi-cloud-backup
name: "Multi-Cloud Backup Check"

steps:
  # AWS backup check
  - name: check-aws-backups
    type: script.run
    with:
      image_ref: "aws"
      
      inline_script: |
        #!/bin/bash
        aws s3 ls s3://prod-backups/ --recursive | tail -10
      
      secret_env_all:
        - "aws-credentials"
      
      node_selector:
        cloud-provider: "aws"
      
      tolerations:
        - key: "dedicated"
          operator: "Equal"
          value: "batch-jobs"
          effect: "NoSchedule"
  
  # GCP backup check
  - name: check-gcp-backups
    type: script.run
    with:
      image_ref: "gcp"
      
      inline_script: |
        #!/bin/bash
        gsutil ls gs://prod-backups/ | tail -10
      
      secret_env_all:
        - "gcp-credentials"
      
      node_selector:
        cloud-provider: "gcp"
      
      tolerations:
        - key: "dedicated"
          operator: "Equal"
          value: "batch-jobs"
          effect: "NoSchedule"
```

---

### Example 5: Complex Configuration with Affinity

```yaml
id: ops.data-processing
name: "Data Processing Job"

steps:
  - name: process-data
    type: script.run
    with:
      image: "harbor.company.internal/data/processor:v2.1.0"
      image_pull_secret: "harbor-credentials"
      
      inline_script: |
        #!/usr/bin/env python3
        import os
        import json
        
        # Load config
        with open('/config/processing.json') as f:
            config = json.load(f)
        
        # Process data
        print(f"Processing {config['dataset']}...")
        
        # Simulate processing
        import time
        time.sleep(60)
        
        print("✓ Processing complete")
      
      # Config from ConfigMap
      volumes_from_configmap:
        - configmap_name: "data-processing-config"
          mount_path: "/config"
      
      # Data from PVC (requires custom volume support - future enhancement)
      # volumes_from_pvc:
      #   - pvc_name: "data-storage"
      #     mount_path: "/data"
      
      # Environment from multiple sources
      env:
        PROCESSOR_MODE: "batch"
        LOG_LEVEL: "info"
      
      env_from_secret:
        API_KEY:
          secret_name: "api-credentials"
          key: "key"
      
      configmap_env_all:
        - "feature-flags"
      
      # Advanced scheduling
      node_selector:
        node-type: "compute-optimized"
        gpu: "true"
      
      tolerations:
        - key: "gpu"
          operator: "Exists"
          effect: "NoSchedule"
        - key: "high-memory"
          operator: "Equal"
          value: "true"
          effect: "NoSchedule"
      
      affinity:
        node_affinity:
          required:
            match_expressions:
              - key: "zone"
                operator: "In"
                values: ["us-east-1a", "us-east-1b"]
        
        pod_anti_affinity:
          preferred:
            - weight: 100
              pod_affinity_term:
                label_selector:
                  match_expressions:
                    - key: "app"
                      operator: "In"
                      values: ["database"]
                topology_key: "kubernetes.io/hostname"
      
      priority_class_name: "high-priority-batch"
      
      resources:
        requests:
          cpu: "2000m"
          memory: "4Gi"
        limits:
          cpu: "8000m"
          memory: "16Gi"
          ephemeral_storage: "50Gi"
      
      timeout: "2h"
      
      labels:
        app: "data-processor"
        team: "data-engineering"
        cost-center: "analytics"
      
      annotations:
        description: "Nightly data processing job"
        owner: "data-team@company.com"
```

---

### Example 6: Approval Workflow

```yaml
id: ops.delete-old-backups
name: "Delete Old Backups"

steps:
  - name: delete-backups
    type: script.run
    
    # REQUIRES APPROVAL
    approval_required: true
    approvers:
      - "sre-leads"
      - "backup-admins"
    
    with:
      image_ref: "aws"
      
      inline_script: |
        #!/bin/bash
        set -euo pipefail
        
        DAYS_OLD=90
        BUCKET="s3://prod-backups"
        CUTOFF_DATE=$(date -d "$DAYS_OLD days ago" +%Y-%m-%d)
        
        echo "Deleting backups older than $CUTOFF_DATE from $BUCKET"
        
        COUNT=0
        aws s3 ls $BUCKET/ | while read -r line; do
          DATE=$(echo $line | awk '{print $1}')
          FILE=$(echo $line | awk '{print $4}')
          
          if [[ "$DATE" < "$CUTOFF_DATE" ]]; then
            echo "Deleting: $FILE (date: $DATE)"
            aws s3 rm "$BUCKET/$FILE"
            COUNT=$((COUNT + 1))
          fi
        done
        
        echo "Deleted $COUNT backup files"
      
      secret_env_all:
        - "aws-credentials"
      
      timeout: "30m"
  
  - name: report-deletion
    type: report.render
    with:
      sections:
        - id: deletion_results
          name: "Backup Deletion Results"
          checks:
            - id: deleted_count
              name: "Files Deleted"
              findings:
                - type: INFO
                  message: "{{ .Steps.delete_backups.output.stdout }}"
```

**Approval Flow:**

1. User triggers runbook
2. Step reaches `delete-backups` with `approval_required: true`
3. Script executor creates approval request
4. Returns STATUS_PENDING to orchestrator
5. Orchestrator pauses execution
6. SRE lead reviews script in UI
7. SRE lead approves via API: `POST /api/v1/approvals/{exec-id}/delete-backups/approve`
8. Orchestrator resumes execution
9. Script executes
10. Audit log records approver

---

## Deployment

### Complete Deployment Manifests

```yaml
# deploy/k8s/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: opscontrolroom-system

---
# deploy/k8s/rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: script-executor
  namespace: opscontrolroom-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: script-executor-runner
  namespace: opscontrolroom-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: script-executor
  namespace: opscontrolroom-system
rules:
# Jobs
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "watch", "delete"]
# Pods
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
# Pod logs
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
# ConfigMaps (for catalog and approvals)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "create", "update"]
# Secrets (for reading credentials)
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: script-executor
  namespace: opscontrolroom-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: script-executor
subjects:
- kind: ServiceAccount
  name: script-executor
  namespace: opscontrolroom-system

---
# deploy/k8s/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: script-executor-config
  namespace: opscontrolroom-system
data:
  config.yaml: |
    # Full config from earlier section
    script_executor:
      grpc:
        port: 50051
      # ... (rest of config)

---
# deploy/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: script-executor
  namespace: opscontrolroom-system
  labels:
    app: script-executor
spec:
  replicas: 2
  selector:
    matchLabels:
      app: script-executor
  template:
    metadata:
      labels:
        app: script-executor
    spec:
      serviceAccountName: script-executor
      
      containers:
      - name: executor
        image: your-registry/script-executor:latest
        imagePullPolicy: Always
        
        ports:
        - containerPort: 50051
          name: grpc
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        - containerPort: 8080
          name: http
          protocol: TCP
        
        env:
        - name: CONFIG_PATH
          value: "/etc/script-executor/config.yaml"
        
        volumeMounts:
        - name: config
          mountPath: /etc/script-executor
        
        resources:
          requests:
            cpu: "200m"
            memory: "256Mi"
          limits:
            cpu: "1000m"
            memory: "1Gi"
        
        livenessProbe:
          exec:
            command:
            - /bin/sh
            - -c
            - "nc -z localhost 50051"
          initialDelaySeconds: 10
          periodSeconds: 10
        
        readinessProbe:
          exec:
            command:
            - /bin/sh
            - -c
            - "nc -z localhost 50051"
          initialDelaySeconds: 5
          periodSeconds: 5
      
      volumes:
      - name: config
        configMap:
          name: script-executor-config

---
# deploy/k8s/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: script-executor
  namespace: opscontrolroom-system
  labels:
    app: script-executor
spec:
  type: ClusterIP
  ports:
  - port: 50051
    targetPort: 50051
    protocol: TCP
    name: grpc
  - port: 9090
    targetPort: 9090
    protocol: TCP
    name: metrics
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: script-executor
```

---

## Success Criteria

**Week 1-2: Core Implementation**
- ✅ gRPC service with Execute, Describe, Health
- ✅ Image resolution (catalog + explicit)
- ✅ K8s Job creation with full config support
- ✅ Basic execution and result collection
- ✅ Unit tests pass

**Week 3: Security & Approval**
- ✅ RBAC enforcement (SRE-only)
- ✅ Script validation (command filtering)
- ✅ Approval workflow implemented
- ✅ Audit logging working

**Week 4: Production Ready**
- ✅ All configuration options working
- ✅ Metrics exported
- ✅ Integration tests pass
- ✅ Documentation complete
- ✅ Deployed to dev cluster

---

**This design is comprehensive and ready for implementation!**
