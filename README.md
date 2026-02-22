# Script Executor

gRPC executor service for executing shell, Python, and Ruby scripts in Kubernetes Jobs. Part of OpsControlRoom.

## Features

- **Script sources**: Inline, ConfigMap, Secret, path, or registry
- **Full K8s control**: Node selectors, tolerations, affinity, resources
- **Image catalog**: Pre-approved images from ConfigMap
- **Approval workflow**: Manual approval for sensitive operations
- **Security**: Command filtering, non-root execution, read-only filesystem

## Quick Start

### Prerequisites

- Go 1.25+
- Kubernetes cluster (or kubeconfig for local dev)
- [buf](https://buf.build) for proto generation

### Build

```bash
make generate  # Regenerate proto (if modified)
make build
```

### Run Locally

```bash
# With kubeconfig for your cluster
export KUBECONFIG=~/.kube/config
export KUBERNETES_NAMESPACE=opscontrolroom-system
./bin/script-executor
```

### Test with grpcurl

```bash
# Describe capabilities
grpcurl -plaintext localhost:50051 executor.v1.Executor/Describe

# Health check
grpcurl -plaintext localhost:50051 executor.v1.Executor/Health

# Execute inline script
grpcurl -plaintext -d '{
  "step_type": "script.run",
  "parameters": {
    "inline_script": "#!/bin/bash\necho hello",
    "image": "alpine:latest"
  },
  "context": {
    "execution_id": "test-123",
    "user": "test@example.com"
  }
}' localhost:50051 executor.v1.Executor/Execute
```

## Configuration

Configuration is loaded from `CONFIG_PATH` (default: env vars). See [design/script-executor-complete-design.md](design/script-executor-complete-design.md) for full config reference.

Key env vars:
- `GRPC_PORT` - gRPC port (default: 50051)
- `KUBERNETES_NAMESPACE` - Namespace for Jobs (default: opscontrolroom-system)
- `DEFAULT_IMAGE` - Default container image

## Deployment

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
```

Build and push the image:
```bash
docker build -t your-registry/script-executor:latest .
docker push your-registry/script-executor:latest
```

Update `deploy/k8s/deployment.yaml` with your image.

## Step Type: script.run

See the design documents for full parameter specification:
- [script-executor-complete-design.md](design/script-executor-complete-design.md)
- [loading-scripts-from-cm-and-secrets.md](design/loading-scripts-from-cm-and-secrets.md)
