Great catch! Yes, it should support that. Let me update the design to include that capability.

---

# Script Source Options - Enhancement

## Current Gap

The design shows `inline_script` and mentions `script_path` but doesn't fully specify how to reference scripts from ConfigMaps or Secrets.

---

## Enhanced Script Source Specification

### All Script Source Options

```yaml
# ============================================================================
# SCRIPT SOURCE (exactly ONE required)
# ============================================================================

# Option 1: Inline script
inline_script: string
  # Script content directly in runbook
  # Example: |
  #   #!/bin/bash
  #   echo "Hello World"

# Option 2: Script from ConfigMap
script_from_configmap:
  configmap_name: string (required)
  key: string (required)
  namespace: string (optional)  # Default: executor namespace
  # Example:
  #   configmap_name: "maintenance-scripts"
  #   key: "check-db-lag.sh"

# Option 3: Script from Secret
script_from_secret:
  secret_name: string (required)
  key: string (required)
  namespace: string (optional)  # Default: executor namespace
  # Example:
  #   secret_name: "sensitive-scripts"
  #   key: "cleanup-prod.sh"

# Option 4: Pre-registered script path (mounted via ConfigMap)
script_path: string
  # Path to script in pre-mounted ConfigMap
  # Requires ConfigMap mounted at /scripts
  # Example: "/scripts/check-db-lag.sh"
```

---

## Implementation

### Updated Job Builder

```go
// internal/executor/script_loader.go

package executor

import (
    "context"
    "fmt"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ScriptSource struct {
    Type   string  // inline|configmap|secret|path
    
    // For inline
    Content string
    
    // For configmap/secret
    Name      string
    Key       string
    Namespace string
    
    // For path
    Path string
}

func (e *ScriptExecutor) LoadScript(ctx context.Context, params Parameters) (string, ScriptSource, error) {
    // Check which source is provided
    
    // 1. Inline script
    if inline := params.GetString("inline_script"); inline != "" {
        return inline, ScriptSource{
            Type:    "inline",
            Content: inline,
        }, nil
    }
    
    // 2. Script from ConfigMap
    if cmRef := params.GetMap("script_from_configmap"); len(cmRef) > 0 {
        cmName := cmRef["configmap_name"].(string)
        key := cmRef["key"].(string)
        namespace := e.getStringOrDefault(cmRef, "namespace", e.config.Namespace)
        
        script, err := e.loadScriptFromConfigMap(ctx, namespace, cmName, key)
        if err != nil {
            return "", ScriptSource{}, fmt.Errorf("failed to load script from configmap: %w", err)
        }
        
        return script, ScriptSource{
            Type:      "configmap",
            Name:      cmName,
            Key:       key,
            Namespace: namespace,
        }, nil
    }
    
    // 3. Script from Secret
    if secretRef := params.GetMap("script_from_secret"); len(secretRef) > 0 {
        secretName := secretRef["secret_name"].(string)
        key := secretRef["key"].(string)
        namespace := e.getStringOrDefault(secretRef, "namespace", e.config.Namespace)
        
        script, err := e.loadScriptFromSecret(ctx, namespace, secretName, key)
        if err != nil {
            return "", ScriptSource{}, fmt.Errorf("failed to load script from secret: %w", err)
        }
        
        return script, ScriptSource{
            Type:      "secret",
            Name:      secretName,
            Key:       key,
            Namespace: namespace,
        }, nil
    }
    
    // 4. Pre-registered script path
    if path := params.GetString("script_path"); path != "" {
        // Note: Script will be loaded at runtime from mounted ConfigMap
        // We just validate the path exists in the registry
        if !e.scriptRegistry.Exists(path) {
            return "", ScriptSource{}, fmt.Errorf("script not found in registry: %s", path)
        }
        
        // For path-based scripts, we return empty content
        // The actual script is mounted in the pod
        return "", ScriptSource{
            Type: "path",
            Path: path,
        }, nil
    }
    
    return "", ScriptSource{}, fmt.Errorf("no script source provided")
}

func (e *ScriptExecutor) loadScriptFromConfigMap(ctx context.Context, namespace, name, key string) (string, error) {
    cm, err := e.k8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
    if err != nil {
        return "", err
    }
    
    script, ok := cm.Data[key]
    if !ok {
        return "", fmt.Errorf("key '%s' not found in configmap '%s'", key, name)
    }
    
    return script, nil
}

func (e *ScriptExecutor) loadScriptFromSecret(ctx context.Context, namespace, name, key string) (string, error) {
    secret, err := e.k8sClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
    if err != nil {
        return "", err
    }
    
    scriptBytes, ok := secret.Data[key]
    if !ok {
        return "", fmt.Errorf("key '%s' not found in secret '%s'", key, name)
    }
    
    return string(scriptBytes), nil
}
```

### Updated Job Command Builder

```go
func (b *JobBuilder) buildScriptCommand(ctx ExecutionContext) ([]string, error) {
    interpreter := ctx.Interpreter
    
    switch ctx.ScriptSource.Type {
    case "inline":
        // Script content passed directly
        return []string{interpreter, "-c", ctx.Script}, nil
    
    case "configmap":
        // Script loaded from ConfigMap at executor startup
        // Already available in ctx.Script
        return []string{interpreter, "-c", ctx.Script}, nil
    
    case "secret":
        // Script loaded from Secret at executor startup
        // Already available in ctx.Script
        return []string{interpreter, "-c", ctx.Script}, nil
    
    case "path":
        // Script mounted in pod at /scripts
        // Execute directly from mounted path
        return []string{interpreter, ctx.ScriptSource.Path}, nil
    
    default:
        return nil, fmt.Errorf("unknown script source type: %s", ctx.ScriptSource.Type)
    }
}
```

---

## Examples

### Example 1: Script from ConfigMap

**ConfigMap:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: maintenance-scripts
  namespace: opscontrolroom-system
data:
  check-db-lag.sh: |
    #!/bin/bash
    set -euo pipefail
    
    THRESHOLD=${1:-5000}
    LAG=$(psql -h $DB_HOST -t -c \
      "SELECT pg_last_wal_receive_lsn() - pg_last_wal_replay_lsn()" | bc)
    
    if [ $LAG -gt $THRESHOLD ]; then
      echo "CRITICAL: Replication lag is ${LAG}ms"
      exit 1
    fi
    echo "OK: Replication lag is ${LAG}ms"
  
  cleanup-temp-files.sh: |
    #!/bin/bash
    find /tmp -type f -mtime +7 -delete
    echo "Cleanup complete"
  
  backup-database.sh: |
    #!/bin/bash
    pg_dump -h $DB_HOST -U $DB_USER $DB_NAME | \
      gzip > /backups/backup-$(date +%Y%m%d-%H%M%S).sql.gz
```

**Runbook:**
```yaml
id: ops.check-db-lag
name: "Check Database Replication Lag"

inputs:
  threshold_ms:
    type: int
    default: 5000

steps:
  - name: check-lag
    type: script.run
    with:
      image_ref: "postgres"
      
      # Load script from ConfigMap
      script_from_configmap:
        configmap_name: "maintenance-scripts"
        key: "check-db-lag.sh"
      
      args:
        - "{{ .Inputs.threshold_ms }}"
      
      env:
        DB_HOST: "postgres.database.svc.cluster.local"
      
      timeout: "30s"
```

---

### Example 2: Script from Secret (Sensitive Scripts)

**Secret:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sensitive-scripts
  namespace: opscontrolroom-system
type: Opaque
stringData:
  delete-prod-backups.sh: |
    #!/bin/bash
    set -euo pipefail
    
    # Dangerous operation - requires approval
    BUCKET="s3://prod-backups"
    CUTOFF_DATE=$(date -d "90 days ago" +%Y-%m-%d)
    
    echo "Deleting backups older than $CUTOFF_DATE from $BUCKET"
    
    aws s3 ls $BUCKET/ | while read -r line; do
      DATE=$(echo $line | awk '{print $1}')
      FILE=$(echo $line | awk '{print $4}')
      
      if [[ "$DATE" < "$CUTOFF_DATE" ]]; then
        echo "Deleting: $FILE"
        aws s3 rm "$BUCKET/$FILE"
      fi
    done
  
  rotate-api-keys.sh: |
    #!/bin/bash
    # Script contains sensitive logic
    # Stored in Secret to prevent exposure
    ...
```

**Runbook:**
```yaml
id: ops.delete-old-backups
name: "Delete Old Production Backups"

steps:
  - name: delete-backups
    type: script.run
    with:
      image_ref: "aws"
      
      # Load script from Secret
      script_from_secret:
        secret_name: "sensitive-scripts"
        key: "delete-prod-backups.sh"
      
      secret_env_all:
        - "aws-credentials"
      
      timeout: "30m"
      
      # Require approval for sensitive operation
      approval_required: true
      approvers:
        - "sre-leads"
```

---

### Example 3: Pre-Registered Script Path

**Pre-Mounted Scripts ConfigMap:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: approved-scripts
  namespace: opscontrolroom-system
data:
  check-db-lag.sh: |
    #!/bin/bash
    # Pre-approved script
    # Mounted at /scripts in all script executor pods
    ...
  
  health-check.sh: |
    #!/bin/bash
    ...
```

**Script Executor Deployment (mounts ConfigMap):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: script-executor
spec:
  template:
    spec:
      containers:
      - name: executor
        volumeMounts:
        - name: approved-scripts
          mountPath: /scripts
          readOnly: true
      
      volumes:
      - name: approved-scripts
        configMap:
          name: approved-scripts
```

**Runbook:**
```yaml
id: ops.health-check
name: "Run Health Check"

steps:
  - name: check-health
    type: script.run
    with:
      image_ref: "base"
      
      # Reference pre-mounted script
      script_path: "/scripts/health-check.sh"
      
      timeout: "1m"
```

---

### Example 4: Script Registry Pattern

**ConfigMap Registry:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: script-registry
  namespace: opscontrolroom-system
data:
  registry.yaml: |
    scripts:
      check-db-lag:
        configmap: "maintenance-scripts"
        key: "check-db-lag.sh"
        description: "Check PostgreSQL replication lag"
        approved_by: "dba-team@company.com"
        approved_at: "2026-02-20T10:00:00Z"
      
      cleanup-temp:
        configmap: "maintenance-scripts"
        key: "cleanup-temp-files.sh"
        description: "Clean up temporary files"
        approved_by: "sre-team@company.com"
        approved_at: "2026-02-19T15:30:00Z"
      
      delete-backups:
        secret: "sensitive-scripts"
        key: "delete-prod-backups.sh"
        description: "Delete old production backups"
        requires_approval: true
        approved_by: "sre-leads@company.com"
        approved_at: "2026-02-18T09:00:00Z"
```

**Enhanced Script Loading with Registry:**

```go
// Load script by registry ID
func (e *ScriptExecutor) LoadScriptByID(ctx context.Context, scriptID string) (string, ScriptSource, error) {
    // Get registry
    registry, err := e.loadScriptRegistry(ctx)
    if err != nil {
        return "", ScriptSource{}, err
    }
    
    // Find script
    scriptInfo, ok := registry.Scripts[scriptID]
    if !ok {
        return "", ScriptSource{}, fmt.Errorf("script not found in registry: %s", scriptID)
    }
    
    // Load based on source
    if scriptInfo.ConfigMap != "" {
        return e.loadScriptFromConfigMap(ctx, e.config.Namespace, scriptInfo.ConfigMap, scriptInfo.Key)
    }
    
    if scriptInfo.Secret != "" {
        return e.loadScriptFromSecret(ctx, e.config.Namespace, scriptInfo.Secret, scriptInfo.Key)
    }
    
    return "", ScriptSource{}, fmt.Errorf("invalid script registry entry")
}
```

**Runbook with Registry Reference:**
```yaml
id: ops.db-maintenance
name: "Database Maintenance"

steps:
  - name: check-lag
    type: script.run
    with:
      image_ref: "postgres"
      
      # Reference by registry ID
      script_id: "check-db-lag"
      
      env:
        DB_HOST: "postgres.database.svc.cluster.local"
```

---

## Updated Parameter Specification

```yaml
# ============================================================================
# SCRIPT SOURCE (exactly ONE required)
# ============================================================================

inline_script: string
  # Inline script content
  
script_from_configmap:
  configmap_name: string
  key: string
  namespace: string (optional)
  
script_from_secret:
  secret_name: string
  key: string
  namespace: string (optional)

script_path: string
  # Pre-mounted script path
  
script_id: string
  # Reference to script in registry
  # Looks up in script-registry ConfigMap
```

---

## Benefits of Each Approach

### 1. Inline Scripts
**Use When:**
- Quick prototyping
- Simple, one-off operations
- Script is tightly coupled to runbook

**Pros:**
- ✅ Self-contained runbook
- ✅ Easy to version with runbook
- ✅ No external dependencies

**Cons:**
- ❌ Script not reusable
- ❌ Runbook becomes verbose

---

### 2. Scripts from ConfigMaps
**Use When:**
- Shared scripts across runbooks
- Non-sensitive scripts
- Version-controlled scripts

**Pros:**
- ✅ Reusable across runbooks
- ✅ Centralized management
- ✅ Easy to update (kubectl edit)
- ✅ Can view with kubectl

**Cons:**
- ❌ Extra hop to load
- ❌ Not suitable for sensitive scripts

**Example Structure:**
```
ConfigMaps:
- maintenance-scripts (database maintenance)
- monitoring-scripts (health checks, metrics)
- deployment-scripts (rollout, rollback)
- backup-scripts (backup, restore)
```

---

### 3. Scripts from Secrets
**Use When:**
- Sensitive operations
- Scripts contain credentials
- Need to restrict visibility

**Pros:**
- ✅ Encrypted at rest
- ✅ RBAC-controlled access
- ✅ Audit trail
- ✅ Not visible in runbook YAML

**Cons:**
- ❌ Harder to inspect (base64)
- ❌ Extra permissions needed

**Example Structure:**
```
Secrets:
- production-scripts (dangerous operations)
- api-management-scripts (API key rotation)
- security-scripts (security audits)
```

---

### 4. Pre-Registered Scripts (script_path)
**Use When:**
- Scripts require approval process
- Stable, rarely-changing scripts
- Want to version with executor image

**Pros:**
- ✅ Pre-approved scripts only
- ✅ Fast (no K8s API call)
- ✅ Immutable (can't be changed at runtime)

**Cons:**
- ❌ Requires executor restart to update
- ❌ Less flexible

---

### 5. Script Registry
**Use When:**
- Large number of scripts
- Want metadata (approvals, descriptions)
- Need catalog/discovery

**Pros:**
- ✅ Central catalog
- ✅ Metadata tracking
- ✅ Approval history
- ✅ Easy discovery

**Cons:**
- ❌ Extra abstraction layer
- ❌ Registry must be maintained

---

## Security Considerations

### Script Validation

All scripts, regardless of source, go through validation:

```go
func (e *ScriptExecutor) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // 1. Load script from any source
    script, source, err := e.LoadScript(ctx, req.Parameters)
    if err != nil {
        return errorResponse(err)
    }
    
    // 2. Validate script (same for all sources)
    if err := e.validator.Validate(script); err != nil {
        return errorResponse(fmt.Errorf("script validation failed: %w", err))
    }
    
    // 3. Hash script for audit
    scriptHash := sha256Hash(script)
    
    // 4. Audit log includes source
    e.auditLog(AuditRecord{
        ExecutionID:   req.Context.ExecutionId,
        ScriptHash:    scriptHash,
        ScriptSource:  source.Type,
        ScriptRef:     source.Name,  // ConfigMap or Secret name
        // ...
    })
    
    // ... continue execution
}
```

### RBAC for Script Sources

```yaml
# script-executor ServiceAccount needs permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: script-executor
  namespace: opscontrolroom-system
rules:
# ConfigMaps (for scripts)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
  # Optionally restrict to specific names:
  # resourceNames: ["maintenance-scripts", "monitoring-scripts"]

# Secrets (for sensitive scripts)
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
  # Restrict to specific secrets:
  resourceNames: ["sensitive-scripts", "production-scripts"]
```

---

## Audit Trail

Scripts from all sources are logged:

```json
{
  "event": "script_execution",
  "execution_id": "exec-abc123",
  "user": "john.doe@company.com",
  "script_hash": "sha256:abc123...",
  "script_source": {
    "type": "configmap",
    "configmap_name": "maintenance-scripts",
    "key": "check-db-lag.sh",
    "namespace": "opscontrolroom-system"
  },
  "script_content": "#!/bin/bash\n...",  // Full script for audit
  "timestamp": "2026-02-20T10:30:00Z"
}
```

---

## Migration Path

**Phase 1: Inline Scripts (Week 1)**
- Start with inline scripts
- Easy to get started
- Self-contained runbooks

**Phase 2: ConfigMaps (Week 2-3)**
- Move common scripts to ConfigMaps
- Share across runbooks
- Centralized management

**Phase 3: Secrets (Week 4)**
- Move sensitive scripts to Secrets
- Enhanced security
- Audit trail

**Phase 4: Registry (Optional)**
- Add script registry for discovery
- Metadata and approval tracking
- Catalog management

---

This enhancement gives you **maximum flexibility** while maintaining **security and auditability**! You can now:

1. Prototype with inline scripts
2. Share via ConfigMaps
3. Secure via Secrets
4. Catalog via Registry

All while maintaining the same validation, approval, and audit controls. 🎯