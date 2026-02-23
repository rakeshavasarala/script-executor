# DLT Network — GitOps Deployment Pipeline: Implementation Specification

> **Status:** Design Complete — Ready for Implementation  
> **Stack:** ArgoCD (Admin + Non-Admin), GitLab CI, Helm (Artifactory), Kubernetes  
> **Scope:** QA, Pre-prod, Production environments

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Constraints & Principles](#2-constraints--principles)
3. [Repository Structure](#3-repository-structure)
4. [Versioning Strategy](#4-versioning-strategy)
5. [ArgoCD Setup — Admin vs Non-Admin](#5-argocd-setup--admin-vs-non-admin)
6. [ProjectApplicationTemplate (PAT) Pattern](#6-projectapplicationtemplate-pat-pattern)
7. [App of Apps Hierarchy](#7-app-of-apps-hierarchy)
8. [ApplicationSet Definitions](#8-applicationset-definitions)
9. [Helm Values Layering](#9-helm-values-layering)
10. [Sync Wave Ordering](#10-sync-wave-ordering)
11. [Backup Strategy](#11-backup-strategy)
12. [Promotion Pipeline (GitLab CI)](#12-promotion-pipeline-gitlab-ci)
13. [Manifest Diff in MR Pipeline](#13-manifest-diff-in-mr-pipeline)
14. [Environment-Specific Sync Policies](#14-environment-specific-sync-policies)
15. [Adding a New Node](#15-adding-a-new-node)
16. [Scaling Reference](#16-scaling-reference)
17. [Implementation Checklist](#17-implementation-checklist)

---

## 1. Architecture Overview

### High-Level Flow

```
Build Pipeline
  └─► Image published to Artifactory
  └─► CI updates versions/{env}.yaml + argocd/{env}/*.yaml
  └─► Git push to main branch

GitOps Repo (GitLab)
  └─► ArgoCD clusters poll for changes (read-only from cluster side)
  └─► Admin ArgoCD: provisions namespaces via PAT controller
  └─► Non-Admin ArgoCD: deploys node components

Promotion
  QA (auto-sync) ──► Pre-prod (manual sync) ──► Prod (manual sync)
```

### Key Architectural Decisions

- **GitLab cannot reach pre-prod or prod clusters.** All cluster interactions are pull-based (ArgoCD polls Git).
- **Two ArgoCD instances per cluster:** Admin (cluster-scoped resources) and Non-Admin (application workloads).
- **Namespace provisioning** uses a custom `ProjectApplicationTemplate` (PAT) CR — not a Helm chart.
- **All Helm charts** are versioned and stored in Artifactory. No charts in the GitOps repo.
- **Per-env mono-repo:** QA, pre-prod, and prod manifests live in separate folders in a single GitLab repo.
- **Backup must complete** before any deployment proceeds, enforced via ArgoCD sync waves.

---

## 2. Constraints & Principles

| Constraint | Detail |
|---|---|
| GitLab → Cluster access | **None** for pre-prod and prod. Read-only GitOps model. |
| QA sync | Auto-sync with sync windows permitted |
| Pre-prod / Prod sync | Manual sync only — operator triggers via ArgoCD UI |
| Backup | Must run before every deployment — enforced at wave -2 |
| Namespace creation | Via PAT CR → custom controller → Admin ArgoCD |
| Chart storage | Artifactory only — no charts in GitOps repo |
| Promotion | QA → Pre-prod → Prod, version-gated, manual gates for pre-prod and prod |
| Node heterogeneity | Each env has a different set of nodes with different configs |

---

## 3. Repository Structure

```
gitops-repo/
├── versions/
│   ├── qa.yaml                          # Chart + image versions for QA
│   ├── preprod.yaml                     # Chart + image versions for Pre-prod
│   └── prod.yaml                        # Chart + image versions for Prod
│
├── envs/
│   ├── qa/
│   │   ├── common-node-values.yaml      # Shared values for ALL nodes in QA
│   │   ├── backup/
│   │   │   └── values.yaml
│   │   ├── shared-services/
│   │   │   └── values.yaml
│   │   └── nodes/
│   │       ├── node-alpha/
│   │       │   ├── values-namespace.yaml   # PAT CR values (node name/overrides)
│   │       │   ├── values-vault.yaml
│   │       │   ├── values-backend.yaml
│   │       │   └── values-frontend.yaml
│   │       ├── node-beta/
│   │       │   └── ... (same structure)
│   │       └── node-gamma/
│   │           └── ... (same structure)
│   ├── preprod/
│   │   ├── common-node-values.yaml
│   │   ├── backup/
│   │   ├── shared-services/
│   │   └── nodes/
│   │       ├── node-alpha/
│   │       └── node-delta/              # different node set from QA
│   └── prod/
│       ├── common-node-values.yaml
│       ├── backup/
│       ├── shared-services/
│       └── nodes/
│           ├── node-alpha/
│           └── node-epsilon/            # different node set from pre-prod
│
└── argocd/
    ├── qa/
    │   ├── app-root.yaml                # App of Apps — bootstrapped manually once
    │   ├── app-backup.yaml              # wave -2
    │   ├── app-shared-services.yaml     # wave 0
    │   ├── appset-namespace.yaml        # wave -1 (PAT CR generator)
    │   ├── appset-vault.yaml            # wave 1
    │   ├── appset-backend.yaml          # wave 2
    │   └── appset-frontend.yaml         # wave 3
    ├── preprod/
    │   └── ... (same structure, no auto-sync)
    └── prod/
        └── ... (same structure, no auto-sync)
```

> **Rule:** The `envs/` folders contain only Helm values files. No ArgoCD Application YAMLs live here. The `argocd/` folders contain only ArgoCD Application and ApplicationSet manifests. No Helm values live here.

---

## 4. Versioning Strategy

### `versions/{env}.yaml` — Source of Truth for All Versions

```yaml
# versions/qa.yaml
dlt-node:
  namespace:
    chartVersion: "1.0.0"          # thin wrapper chart for PAT CR
  vault:
    chartVersion: "1.2.0"
    imageTag: "v1.2.0"
  backend:
    chartVersion: "3.1.0"
    imageTag: "v2.1.0"
  frontend:
    chartVersion: "2.3.1"
    imageTag: "v1.5.0"

shared-services:
  chartVersion: "2.1.0"
  imageTag: "v1.8.3"

backup:
  chartVersion: "0.3.1"
  imageTag: "v1.2.0"
```

### How Versions Flow

```
versions/{env}.yaml           ← CI writes here on promotion
        │
        ├── read by CI        → patches targetRevision in argocd/{env}/appset-*.yaml
        └── read by ArgoCD    → passed as first values file layer to Helm
```

> **Important:** ArgoCD ApplicationSet `targetRevision` for the Helm chart **cannot** dynamically read from `versions.yaml` — the generator does not parse arbitrary YAML for template variables. Therefore the CI promotion job must patch **both** `versions/{env}.yaml` (source of truth for image tags) and the `targetRevision` field in `argocd/{env}/appset-*.yaml` (consumed by ArgoCD for chart version). Both happen in a single Git commit.

---

## 5. ArgoCD Setup — Admin vs Non-Admin

```
┌─────────────────────────────────────────────────────────────┐
│                        K8s Cluster                          │
│                                                             │
│  ┌─────────────────────┐    ┌────────────────────────────┐ │
│  │   Admin ArgoCD      │    │   Non-Admin ArgoCD         │ │
│  │                     │    │                            │ │
│  │  Manages:           │    │  Manages:                  │ │
│  │  - Namespaces       │◄───│  - PAT CRs (triggers admin)│ │
│  │  - RBAC             │    │  - Vault Applications      │ │
│  │  - Cluster roles    │    │  - Backend Applications    │ │
│  │                     │    │  - Frontend Applications   │ │
│  │  Source: GitLab     │    │  - Shared Services         │ │
│  │  (read-only pull)   │    │  - Backup Application      │ │
│  └─────────────────────┘    │                            │ │
│           ▲                 │  Source: GitLab            │ │
│           │                 │  (read-only pull)          │ │
│  ┌────────────────────┐     └────────────────────────────┘ │
│  │  PAT Controller    │              │                      │
│  │  (custom operator) │◄─────────────┘                     │
│  │                    │  Watches PAT CRs in non-admin      │
│  │  1. Reads PAT CR   │  Creates ArgoCD App in admin       │
│  │  2. Creates NS App │  Signals back when NS is ready     │
│  │  3. Signals ready  │                                    │
│  └────────────────────┘                                    │
└─────────────────────────────────────────────────────────────┘
```

### Namespace Provisioning Flow

```
1. Non-Admin ArgoCD syncs appset-namespace
        │
        ▼
2. PAT CR created in cluster (by thin wrapper Helm chart)
        │
        ▼
3. PAT Controller detects new PAT CR
        │
        ▼
4. Controller creates ArgoCD Application in Admin ArgoCD
        │
        ▼
5. Admin ArgoCD provisions Namespace + RBAC
        │
        ▼
6. Controller updates PAT CR status.phase = "Ready"
        │
        ▼
7. Non-Admin ArgoCD custom health check sees "Ready"
        │
        ▼
8. wave -1 marked Healthy ✓ → wave 1 (vault) begins
```

---

## 6. ProjectApplicationTemplate (PAT) Pattern

### PAT CR Health Check (Critical)

Register a custom health check in ArgoCD so that wave -1 only completes when the namespace is **actually provisioned**, not just when the CR is applied:

```yaml
# argocd-cm ConfigMap (in non-admin ArgoCD namespace)
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  resource.customizations.health.your.group_ProjectApplicationTemplate: |
    hs = {}
    if obj.status ~= nil then
      if obj.status.phase == "Ready" then
        hs.status = "Healthy"
        hs.message = "Namespace provisioned"
      elseif obj.status.phase == "Failed" then
        hs.status = "Degraded"
        hs.message = obj.status.message
      else
        hs.status = "Progressing"
        hs.message = "Waiting for namespace provisioning"
      end
    else
      hs.status = "Progressing"
      hs.message = "Waiting for status"
    end
    return hs
```

> Replace `your.group` with the actual API group of your PAT CRD.

### Thin Wrapper Helm Chart for PAT

Since the PAT CR is generated by your existing Helm library, create a thin wrapper chart that calls the library. This chart is published to Artifactory as `dlt-node-namespace`.

```
dlt-node-namespace/             # published to Artifactory
├── Chart.yaml
├── templates/
│   └── pat.yaml                # calls library helper
└── values.yaml                 # defaults only
```

```yaml
# Chart.yaml
apiVersion: v2
name: dlt-node-namespace
version: 1.0.0
dependencies:
- name: your-helm-library
  version: x.y.z
  repository: https://artifactory.example.com/helm
```

```yaml
# templates/pat.yaml
{{- include "your-helm-library.projectApplicationTemplate" . }}
```

This reuses the existing library with zero new library logic. Only the chart wrapper is new.

### PAT Values Files

```yaml
# envs/qa/common-node-values.yaml (PAT section)
projectApplicationTemplate:
  project: dlt-qa
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
  # default RBAC, quota templates for all nodes in QA
```

```yaml
# envs/qa/nodes/node-alpha/values-namespace.yaml
projectApplicationTemplate:
  name: dlt-node-alpha
  namespace: dlt-node-alpha
  # any node-specific quota or label overrides here
```

---

## 7. App of Apps Hierarchy

### Visual Hierarchy (QA — 3 nodes)

```
dlt-root-qa  (App of Apps — bootstrapped manually once)
│
├── app-backup-qa                    (wave -2)
├── app-shared-services-qa           (wave 0)
│
├── appset-namespace-qa              (wave -1) — ApplicationSet
│   ├── dlt-node-alpha-namespace-qa
│   ├── dlt-node-beta-namespace-qa
│   └── dlt-node-gamma-namespace-qa
│
├── appset-vault-qa                  (wave 1) — ApplicationSet
│   ├── dlt-node-alpha-vault-qa
│   ├── dlt-node-beta-vault-qa
│   └── dlt-node-gamma-vault-qa
│
├── appset-backend-qa                (wave 2) — ApplicationSet
│   ├── dlt-node-alpha-backend-qa
│   ├── dlt-node-beta-backend-qa
│   └── dlt-node-gamma-backend-qa
│
└── appset-frontend-qa               (wave 3) — ApplicationSet
    ├── dlt-node-alpha-frontend-qa
    ├── dlt-node-beta-frontend-qa
    └── dlt-node-gamma-frontend-qa
```

### ArgoCD Object Count

| Object Type | QA (3 nodes) | Pre-prod (2 nodes) | Prod (5 nodes) |
|---|---|---|---|
| Root App of Apps | 1 | 1 | 1 |
| Backup Application | 1 | 1 | 1 |
| Shared Services Application | 1 | 1 | 1 |
| ApplicationSets | 4 | 4 | 4 |
| Generated Applications | 12 | 8 | 20 |
| **Total** | **19** | **15** | **27** |

> **Note:** 4 ApplicationSets exist per **environment**, not per node. Adding a node adds 4 Applications (one per AppSet), with zero changes to any ArgoCD manifest.

### `app-root.yaml` (Bootstrap — Applied Once Per Cluster)

```yaml
# argocd/qa/app-root.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dlt-root-qa
  namespace: argocd
  finalizers:
  - resources-finalizer.argocd.argoproj.io
spec:
  project: dlt-qa
  source:
    repoURL: https://gitlab.example.com/org/gitops-repo.git
    targetRevision: main
    path: argocd/qa
    directory:
      recurse: false
      include: "*.yaml"
      exclude: "app-root.yaml"      # prevent self-reconciliation
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

Bootstrap command (run once per cluster by an operator with cluster access):

```bash
argocd app create dlt-root-qa \
  --repo https://gitlab.example.com/org/gitops-repo.git \
  --path argocd/qa \
  --dest-server https://kubernetes.default.svc \
  --dest-namespace argocd \
  --sync-policy automated \
  --self-heal \
  --auto-prune
```

---

## 8. ApplicationSet Definitions

All ApplicationSets use the **multi-source pattern**: Helm chart from Artifactory, values files from the GitOps repo. Values are layered in order: `versions → common-node → node-specific`.

### `app-backup.yaml` (wave -2)

```yaml
# argocd/qa/app-backup.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dlt-backup-qa
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
spec:
  project: dlt-qa
  sources:
  - repoURL: https://artifactory.example.com/helm
    chart: dlt-backup
    targetRevision: "0.3.1"            # CI patches this on promotion
    helm:
      valueFiles:
      - $values/envs/qa/backup/values.yaml
  - repoURL: https://gitlab.example.com/org/gitops-repo.git
    targetRevision: main
    ref: values
  destination:
    server: https://kubernetes.default.svc
    namespace: dlt-ops
  syncPolicy:
    automated:
      prune: false
      selfHeal: false
```

### `appset-namespace.yaml` (wave -1)

```yaml
# argocd/qa/appset-namespace.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dlt-nodes-namespace-qa
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: https://gitlab.example.com/org/gitops-repo.git
      revision: main
      directories:
      - path: envs/qa/nodes/*
  template:
    metadata:
      name: "dlt-{{path.basename}}-namespace-qa"
      annotations:
        argocd.argoproj.io/sync-wave: "-1"
    spec:
      project: dlt-qa
      sources:
      - repoURL: https://artifactory.example.com/helm
        chart: dlt-node-namespace
        targetRevision: "1.0.0"        # CI patches this on promotion
        helm:
          valueFiles:
          - $values/envs/qa/common-node-values.yaml
          - $values/envs/qa/nodes/{{path.basename}}/values-namespace.yaml
      - repoURL: https://gitlab.example.com/org/gitops-repo.git
        targetRevision: main
        ref: values
      destination:
        server: https://kubernetes.default.svc
        namespace: argocd              # PAT CR lives in argocd namespace
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

### `app-shared-services.yaml` (wave 0)

```yaml
# argocd/qa/app-shared-services.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: dlt-shared-services-qa
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "0"
spec:
  project: dlt-qa
  sources:
  - repoURL: https://artifactory.example.com/helm
    chart: dlt-shared-services
    targetRevision: "2.1.0"            # CI patches this on promotion
    helm:
      valueFiles:
      - $values/versions/qa.yaml
      - $values/envs/qa/shared-services/values.yaml
  - repoURL: https://gitlab.example.com/org/gitops-repo.git
    targetRevision: main
    ref: values
  destination:
    server: https://kubernetes.default.svc
    namespace: dlt-shared
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

### `appset-vault.yaml` (wave 1)

```yaml
# argocd/qa/appset-vault.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dlt-nodes-vault-qa
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: https://gitlab.example.com/org/gitops-repo.git
      revision: main
      directories:
      - path: envs/qa/nodes/*
  template:
    metadata:
      name: "dlt-{{path.basename}}-vault-qa"
      annotations:
        argocd.argoproj.io/sync-wave: "1"
    spec:
      project: dlt-qa
      sources:
      - repoURL: https://artifactory.example.com/helm
        chart: dlt-vault
        targetRevision: "1.2.0"        # CI patches this on promotion
        helm:
          valueFiles:
          - $values/versions/qa.yaml
          - $values/envs/qa/common-node-values.yaml
          - $values/envs/qa/nodes/{{path.basename}}/values-vault.yaml
      - repoURL: https://gitlab.example.com/org/gitops-repo.git
        targetRevision: main
        ref: values
      destination:
        server: https://kubernetes.default.svc
        namespace: "dlt-{{path.basename}}"
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=false        # namespace created by PAT — never auto-create
```

### `appset-backend.yaml` (wave 2)

```yaml
# argocd/qa/appset-backend.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dlt-nodes-backend-qa
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: https://gitlab.example.com/org/gitops-repo.git
      revision: main
      directories:
      - path: envs/qa/nodes/*
  template:
    metadata:
      name: "dlt-{{path.basename}}-backend-qa"
      annotations:
        argocd.argoproj.io/sync-wave: "2"
    spec:
      project: dlt-qa
      sources:
      - repoURL: https://artifactory.example.com/helm
        chart: dlt-backend
        targetRevision: "3.1.0"        # CI patches this on promotion
        helm:
          valueFiles:
          - $values/versions/qa.yaml
          - $values/envs/qa/common-node-values.yaml
          - $values/envs/qa/nodes/{{path.basename}}/values-backend.yaml
      - repoURL: https://gitlab.example.com/org/gitops-repo.git
        targetRevision: main
        ref: values
      destination:
        server: https://kubernetes.default.svc
        namespace: "dlt-{{path.basename}}"
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=false
```

### `appset-frontend.yaml` (wave 3)

```yaml
# argocd/qa/appset-frontend.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dlt-nodes-frontend-qa
  namespace: argocd
spec:
  generators:
  - git:
      repoURL: https://gitlab.example.com/org/gitops-repo.git
      revision: main
      directories:
      - path: envs/qa/nodes/*
  template:
    metadata:
      name: "dlt-{{path.basename}}-frontend-qa"
      annotations:
        argocd.argoproj.io/sync-wave: "3"
    spec:
      project: dlt-qa
      sources:
      - repoURL: https://artifactory.example.com/helm
        chart: dlt-frontend
        targetRevision: "2.3.1"        # CI patches this on promotion
        helm:
          valueFiles:
          - $values/versions/qa.yaml
          - $values/envs/qa/common-node-values.yaml
          - $values/envs/qa/nodes/{{path.basename}}/values-frontend.yaml
      - repoURL: https://gitlab.example.com/org/gitops-repo.git
        targetRevision: main
        ref: values
      destination:
        server: https://kubernetes.default.svc
        namespace: "dlt-{{path.basename}}"
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
        - CreateNamespace=false
```

> **Pre-prod and Prod:** All four AppSets are identical except `syncPolicy` has no `automated` block. Operator manually triggers sync in ArgoCD UI after reviewing the diff.

---

## 9. Helm Values Layering

### Three-Layer Merge (left to right, later files win)

```
Layer 1: versions/{env}.yaml              ← image tags, chart versions (CI-managed)
        +
Layer 2: envs/{env}/common-node-values.yaml   ← chain config, resources, env defaults
        +
Layer 3: envs/{env}/nodes/{node}/values-{component}.yaml  ← node-specific overrides
        │
        ▼
  Final rendered Helm values for this node+component
```

### What Belongs in Each Layer

**`versions/{env}.yaml` (Layer 1)**

Only versions — never touched by humans, only by CI:

```yaml
dlt-node:
  backend:
    imageTag: "v2.1.0"
  frontend:
    imageTag: "v1.5.0"
  vault:
    imageTag: "v1.2.0"
```

**`common-node-values.yaml` (Layer 2)**

All values that are identical across nodes in an env:

```yaml
# envs/qa/common-node-values.yaml

# Namespace / PAT defaults
projectApplicationTemplate:
  project: dlt-qa
  destination:
    server: https://kubernetes.default.svc

# Vault defaults for all nodes in QA
vault:
  server:
    ha:
      enabled: false
  injector:
    enabled: true

# Backend defaults for all nodes in QA
backend:
  chain:
    networkId: qa-net-001
    genesisConfigMap: qa-genesis
    consensusTimeout: 5s
  persistence:
    storageClass: fast-ssd
    size: 50Gi
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 2
      memory: 4Gi

# Frontend defaults for all nodes in QA
frontend:
  replicaCount: 1
  ingress:
    enabled: false
  env:
    LOG_LEVEL: debug
```

**`values-{component}.yaml` per node (Layer 3)**

Only the overrides — keep minimal:

```yaml
# envs/qa/nodes/node-alpha/values-backend.yaml
backend:
  node:
    id: alpha
    role: validator
    peers:
    - node-beta.dlt-node-beta.svc.cluster.local
    - node-gamma.dlt-node-gamma.svc.cluster.local

# envs/qa/nodes/node-alpha/values-frontend.yaml
frontend:
  env:
    NODE_ID: alpha
    BACKEND_URL: http://backend.dlt-node-alpha.svc.cluster.local

# envs/qa/nodes/node-alpha/values-vault.yaml
vault:
  server:
    extraEnv:
    - name: VAULT_CLUSTER_NAME
      value: node-alpha

# envs/qa/nodes/node-alpha/values-namespace.yaml
projectApplicationTemplate:
  name: dlt-node-alpha
  namespace: dlt-node-alpha
```

> **Note:** Each ApplicationSet passes the full `common-node-values.yaml` to every chart. Helm silently ignores top-level keys that don't exist in the chart's `values.yaml`. No need to split the common file per component.

---

## 10. Sync Wave Ordering

### Wave Sequence Per Deployment

```
t=0  wave -2   app-backup-qa
               └─► PreSync hook Job runs inside backup Helm chart
               └─► Backup completes successfully
               └─► Application marked Healthy ✓

t=1  wave -1   dlt-{node}-namespace-qa  (all nodes in parallel)
               └─► PAT CR applied via thin wrapper chart
               └─► PAT controller creates namespace in Admin ArgoCD
               └─► status.phase transitions to "Ready"
               └─► Custom health check: Healthy ✓

t=2  wave  0   app-shared-services-qa
               └─► Shared infrastructure (databases, message queues)
               └─► Application Healthy ✓

t=3  wave  1   dlt-{node}-vault-qa  (all nodes in parallel)
               └─► Namespace exists, Vault deploys
               └─► Healthy ✓

t=4  wave  2   dlt-{node}-backend-qa  (all nodes in parallel)
               └─► Vault ready, secrets available
               └─► Healthy ✓

t=5  wave  3   dlt-{node}-frontend-qa  (all nodes in parallel)
               └─► Backend ready
               └─► Healthy ✓
```

### Key Notes on Wave Behaviour

- All nodes progress through each wave **in parallel** — node-alpha, node-beta, node-gamma all deploy at wave 1 simultaneously.
- Waves gate **component ordering within a deployment**, not sequencing between nodes.
- If any Application in a wave fails, ArgoCD halts and does not proceed to the next wave.
- The backup Application at wave -2 uses `prune: false` and `selfHeal: false` intentionally — it should not auto-delete completed backup Jobs.

---

## 11. Backup Strategy

The backup Helm chart (`dlt-backup`) contains a PreSync hook Job internally. When the backup Application syncs at wave -2, the hook fires first.

```yaml
# Inside dlt-backup Helm chart (managed by backup chart team)
# charts/dlt-backup/templates/backup-hook.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: "{{ .Release.Name }}-backup"
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
spec:
  template:
    spec:
      containers:
      - name: backup
        image: "{{ .Values.backup.image.repository }}:{{ .Values.backup.imageTag }}"
        env:
        - name: ENV
          value: "{{ .Values.backup.env }}"
        - name: TARGET
          value: "{{ .Values.backup.target }}"
        command: ["./backup.sh"]
      restartPolicy: OnFailure
```

```yaml
# envs/qa/backup/values.yaml
backup:
  env: qa
  target: s3://company-backups/dlt/qa
  retention: 7d
  image:
    repository: artifactory.example.com/tools/backup
```

---

## 12. Promotion Pipeline (GitLab CI)

### Pipeline Stages

```
build          → publishes image to Artifactory
update-qa      → auto, on merge to main
promote-preprod → manual gate
promote-prod   → manual gate (reads from preprod versions)
```

### `.gitlab-ci.yml`

```yaml
stages:
- build
- update-qa
- promote-preprod
- promote-prod

variables:
  GITLAB_BOT_EMAIL: "ci-bot@example.com"
  GITLAB_BOT_NAME: "CI Bot"

# ── Shared promotion script ──────────────────────────────────────────────────
.promote-script: &promote-script
  image: artifactory.example.com/tools/ci-tools:latest   # yq, git, helm, curl
  before_script:
  - git config user.email "$GITLAB_BOT_EMAIL"
  - git config user.name "$GITLAB_BOT_NAME"
  - git fetch origin main
  - git checkout main
  script:
  - |
    ENV=$TARGET_ENV

    # Read source versions
    NAMESPACE_VER=$(yq e '.dlt-node.namespace.chartVersion' versions/${SOURCE_ENV}.yaml)
    VAULT_VER=$(yq e '.dlt-node.vault.chartVersion' versions/${SOURCE_ENV}.yaml)
    VAULT_IMG=$(yq e '.dlt-node.vault.imageTag' versions/${SOURCE_ENV}.yaml)
    BACKEND_VER=$(yq e '.dlt-node.backend.chartVersion' versions/${SOURCE_ENV}.yaml)
    BACKEND_IMG=$(yq e '.dlt-node.backend.imageTag' versions/${SOURCE_ENV}.yaml)
    FRONTEND_VER=$(yq e '.dlt-node.frontend.chartVersion' versions/${SOURCE_ENV}.yaml)
    FRONTEND_IMG=$(yq e '.dlt-node.frontend.imageTag' versions/${SOURCE_ENV}.yaml)
    SHARED_VER=$(yq e '.shared-services.chartVersion' versions/${SOURCE_ENV}.yaml)
    SHARED_IMG=$(yq e '.shared-services.imageTag' versions/${SOURCE_ENV}.yaml)
    BACKUP_VER=$(yq e '.backup.chartVersion' versions/${SOURCE_ENV}.yaml)
    BACKUP_IMG=$(yq e '.backup.imageTag' versions/${SOURCE_ENV}.yaml)

    # Update versions/{env}.yaml
    yq e ".dlt-node.namespace.chartVersion = \"${NAMESPACE_VER}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.vault.chartVersion = \"${VAULT_VER}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.vault.imageTag = \"${VAULT_IMG}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.backend.chartVersion = \"${BACKEND_VER}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.backend.imageTag = \"${BACKEND_IMG}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.frontend.chartVersion = \"${FRONTEND_VER}\"" -i versions/${ENV}.yaml
    yq e ".dlt-node.frontend.imageTag = \"${FRONTEND_IMG}\"" -i versions/${ENV}.yaml
    yq e ".shared-services.chartVersion = \"${SHARED_VER}\"" -i versions/${ENV}.yaml
    yq e ".shared-services.imageTag = \"${SHARED_IMG}\"" -i versions/${ENV}.yaml
    yq e ".backup.chartVersion = \"${BACKUP_VER}\"" -i versions/${ENV}.yaml
    yq e ".backup.imageTag = \"${BACKUP_IMG}\"" -i versions/${ENV}.yaml

    # Patch targetRevision in each ApplicationSet and Application
    yq e ".spec.template.spec.sources[0].targetRevision = \"${NAMESPACE_VER}\"" \
       -i argocd/${ENV}/appset-namespace.yaml
    yq e ".spec.template.spec.sources[0].targetRevision = \"${VAULT_VER}\"" \
       -i argocd/${ENV}/appset-vault.yaml
    yq e ".spec.template.spec.sources[0].targetRevision = \"${BACKEND_VER}\"" \
       -i argocd/${ENV}/appset-backend.yaml
    yq e ".spec.template.spec.sources[0].targetRevision = \"${FRONTEND_VER}\"" \
       -i argocd/${ENV}/appset-frontend.yaml
    yq e ".spec.sources[0].targetRevision = \"${SHARED_VER}\"" \
       -i argocd/${ENV}/app-shared-services.yaml
    yq e ".spec.sources[0].targetRevision = \"${BACKUP_VER}\"" \
       -i argocd/${ENV}/app-backup.yaml

    # Commit and push
    git add versions/${ENV}.yaml argocd/${ENV}/
    git diff --cached --quiet && echo "No changes to commit" && exit 0
    git commit -m "chore(${ENV}): promote versions from ${SOURCE_ENV} [skip ci]"
    git push origin main

# ── QA: auto on merge to main ────────────────────────────────────────────────
update-qa:
  <<: *promote-script
  stage: update-qa
  rules:
  - if: $CI_COMMIT_BRANCH == "main" && $CI_PIPELINE_SOURCE == "push"
  variables:
    TARGET_ENV: qa
    SOURCE_ENV: qa       # QA reads from build artifacts — see note below
  # Note: For QA, SOURCE_ENV versions are written by the build pipeline directly
  # into versions/qa.yaml before this job runs, so SOURCE_ENV=qa reads current.

# ── Pre-prod: manual gate ────────────────────────────────────────────────────
promote-preprod:
  <<: *promote-script
  stage: promote-preprod
  when: manual
  rules:
  - if: $CI_COMMIT_BRANCH == "main"
  variables:
    TARGET_ENV: preprod
    SOURCE_ENV: qa

# ── Prod: manual gate ────────────────────────────────────────────────────────
promote-prod:
  <<: *promote-script
  stage: promote-prod
  when: manual
  rules:
  - if: $CI_COMMIT_BRANCH == "main"
  variables:
    TARGET_ENV: prod
    SOURCE_ENV: preprod
```

### Promotion Version Flow

```
Build pipeline writes image tags
        │
        ▼
versions/qa.yaml  ──(manual CI gate)──► versions/preprod.yaml ──(manual CI gate)──► versions/prod.yaml
        │                                       │                                           │
        ▼                                       ▼                                           ▼
argocd/qa/*.yaml                     argocd/preprod/*.yaml                      argocd/prod/*.yaml
        │                                       │                                           │
        ▼                                       ▼                                           ▼
ArgoCD auto-syncs                    Operator manually syncs                  Operator manually syncs
(within sync windows)                via ArgoCD UI                           via ArgoCD UI
```

---

## 13. Manifest Diff in MR Pipeline

Reviewers see fully rendered Kubernetes manifests (not just values changes) before merging.

### Option A — Diff as MR Comment (No Cluster Access Required — works for all envs)

```yaml
# .gitlab-ci.yml (add to MR pipelines)
manifest-diff:
  stage: review
  rules:
  - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  image: artifactory.example.com/tools/ci-tools:latest
  script:
  - |
    helm repo add dlt https://artifactory.example.com/helm
    helm repo update

    mkdir -p /tmp/diff/proposed /tmp/diff/current
    FULL_DIFF=""

    for ENV in qa preprod prod; do
      for node_dir in envs/${ENV}/nodes/*/; do
        NODE=$(basename $node_dir)

        for COMPONENT in vault backend frontend; do
          CHART_VER=$(yq e ".dlt-node.${COMPONENT}.chartVersion" versions/${ENV}.yaml)

          # Proposed (MR branch)
          helm template dlt-${NODE}-${COMPONENT}-${ENV} dlt/dlt-${COMPONENT} \
            --version $CHART_VER \
            --values versions/${ENV}.yaml \
            --values envs/${ENV}/common-node-values.yaml \
            --values envs/${ENV}/nodes/${NODE}/values-${COMPONENT}.yaml \
            > /tmp/diff/proposed/${ENV}-${NODE}-${COMPONENT}.yaml 2>/dev/null || true

          # Current (main branch)
          git show origin/main:versions/${ENV}.yaml > /tmp/versions-main.yaml 2>/dev/null || echo "{}" > /tmp/versions-main.yaml
          CURRENT_VER=$(yq e ".dlt-node.${COMPONENT}.chartVersion" /tmp/versions-main.yaml)

          git show origin/main:envs/${ENV}/nodes/${NODE}/values-${COMPONENT}.yaml \
            > /tmp/values-main.yaml 2>/dev/null || echo "{}" > /tmp/values-main.yaml
          git show origin/main:envs/${ENV}/common-node-values.yaml \
            > /tmp/common-main.yaml 2>/dev/null || echo "{}" > /tmp/common-main.yaml

          helm template dlt-${NODE}-${COMPONENT}-${ENV} dlt/dlt-${COMPONENT} \
            --version $CURRENT_VER \
            --values /tmp/versions-main.yaml \
            --values /tmp/common-main.yaml \
            --values /tmp/values-main.yaml \
            > /tmp/diff/current/${ENV}-${NODE}-${COMPONENT}.yaml 2>/dev/null || echo "" > /tmp/diff/current/${ENV}-${NODE}-${COMPONENT}.yaml

          DIFF=$(diff -u \
            /tmp/diff/current/${ENV}-${NODE}-${COMPONENT}.yaml \
            /tmp/diff/proposed/${ENV}-${NODE}-${COMPONENT}.yaml || true)

          if [ -n "$DIFF" ]; then
            FULL_DIFF="${FULL_DIFF}\n### ${ENV} / ${NODE} / ${COMPONENT}\n\`\`\`diff\n${DIFF}\n\`\`\`\n"
          fi
        done
      done
    done

    if [ -z "$FULL_DIFF" ]; then
      BODY="✅ **Manifest Diff**: No changes to rendered Kubernetes manifests."
    else
      BODY="## 📋 Rendered Manifest Diff\n${FULL_DIFF}"
    fi

    curl -s --request POST \
      --header "PRIVATE-TOKEN: ${GITLAB_BOT_TOKEN}" \
      --header "Content-Type: application/json" \
      --data "{\"body\": \"$(echo -e $BODY | sed 's/"/\\"/g')\"}" \
      "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}/notes"
```

### Option B — Live Diff Against Cluster (QA Only — requires cluster access)

```yaml
manifest-diff-live-qa:
  stage: review
  rules:
  - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  image: artifactory.example.com/tools/ci-tools:latest
  script:
  - helm plugin install https://github.com/databus23/helm-diff
  - |
    for node_dir in envs/qa/nodes/*/; do
      NODE=$(basename $node_dir)
      for COMPONENT in vault backend frontend; do
        CHART_VER=$(yq e ".dlt-node.${COMPONENT}.chartVersion" versions/qa.yaml)
        echo "=== qa / ${NODE} / ${COMPONENT} ==="
        helm diff upgrade dlt-${NODE}-${COMPONENT}-qa dlt/dlt-${COMPONENT} \
          --version $CHART_VER \
          --values versions/qa.yaml \
          --values envs/qa/common-node-values.yaml \
          --values envs/qa/nodes/${NODE}/values-${COMPONENT}.yaml \
          --namespace dlt-${NODE} || true
      done
    done
```

> Use Option A for pre-prod and prod (no cluster access). Optionally add Option B for QA to see diff against live cluster state (also catches drift).

---

## 14. Environment-Specific Sync Policies

### QA — Auto-Sync with Sync Windows

```yaml
# argocd/qa/appset-backend.yaml (syncPolicy section)
syncPolicy:
  automated:
    prune: true
    selfHeal: true
  syncOptions:
  - CreateNamespace=false
```

Configure sync windows in ArgoCD to restrict auto-sync to business hours or approved windows:

```yaml
# argocd-appproject for dlt-qa
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: dlt-qa
  namespace: argocd
spec:
  syncWindows:
  - kind: allow
    schedule: "0 8 * * 1-5"    # weekdays 08:00
    duration: 8h
    applications: ["*"]
    namespaces: ["*"]
    clusters: ["*"]
  - kind: deny
    schedule: "0 0 * * *"      # deny all outside allowed window
    duration: 24h
    applications: ["*"]
```

### Pre-prod — Manual Sync, No Auto-Sync

```yaml
# argocd/preprod/appset-backend.yaml (syncPolicy section)
syncPolicy: {}                  # no automated block — operator triggers sync manually
```

Operator workflow for pre-prod/prod sync:
1. Promotion CI job merges version changes to main
2. ArgoCD detects drift (shows OutOfSync) but does NOT sync
3. Operator reviews the diff in ArgoCD UI
4. Operator clicks **Sync** — wave ordering is enforced automatically
5. Backup fires at wave -2 before anything deploys

### Prod — Manual Sync, Change-Window Gated

Same as pre-prod `syncPolicy: {}`, additionally configure a deny sync window so that even if someone clicks sync outside the change window, ArgoCD blocks it:

```yaml
# AppProject for dlt-prod
spec:
  syncWindows:
  - kind: allow
    schedule: "0 22 * * 3"     # Wednesday 22:00 — change window
    duration: 4h
    applications: ["*"]
  - kind: deny
    schedule: "0 0 * * *"
    duration: 24h
    applications: ["*"]
```

---

## 15. Adding a New Node

Adding a node to any environment requires **only Git changes** — no ArgoCD manifest changes needed.

### Steps

1. Create the node folder with four values files:

```bash
mkdir -p envs/qa/nodes/node-delta
```

2. Create `values-namespace.yaml`:

```yaml
projectApplicationTemplate:
  name: dlt-node-delta
  namespace: dlt-node-delta
```

3. Create `values-vault.yaml`:

```yaml
vault:
  server:
    extraEnv:
    - name: VAULT_CLUSTER_NAME
      value: node-delta
```

4. Create `values-backend.yaml`:

```yaml
backend:
  node:
    id: delta
    role: observer
    peers:
    - node-alpha.dlt-node-alpha.svc.cluster.local
```

5. Create `values-frontend.yaml`:

```yaml
frontend:
  env:
    NODE_ID: delta
    BACKEND_URL: http://backend.dlt-node-delta.svc.cluster.local
```

6. Commit and push / raise MR.

On the next Git poll, all four ApplicationSets detect the new folder and automatically generate four new Applications. No changes to `argocd/qa/*.yaml` are needed.

---

## 16. Scaling Reference

### Application Count Formula

```
Total ArgoCD objects per env =
    1 (root App of Apps)
  + 1 (backup Application)
  + 1 (shared-services Application)
  + 4 (ApplicationSets)
  + (N nodes × 4 components)

Example: 5 nodes = 1 + 1 + 1 + 4 + 20 = 27 objects
```

### File Count Per Node

```
4 values files per node (values-namespace, values-vault, values-backend, values-frontend)
```

### CI Promotion Patches Per Promotion Run

```
1  versions/{env}.yaml
6  argocd/{env}/*.yaml files (4 AppSets + shared-services + backup)
─────────────────────────
7  files changed in a single commit per promotion
```

---

## 17. Implementation Checklist

### Phase 1 — Repository Setup

- [ ] Create `versions/qa.yaml`, `versions/preprod.yaml`, `versions/prod.yaml` with initial values
- [ ] Create `envs/qa/common-node-values.yaml` with shared node defaults
- [ ] Create `envs/{env}/nodes/{node}/values-{component}.yaml` for each node and component
- [ ] Create `envs/{env}/backup/values.yaml` for each env
- [ ] Create `envs/{env}/shared-services/values.yaml` for each env

### Phase 2 — ArgoCD Manifests

- [ ] Create `argocd/qa/app-root.yaml` (App of Apps)
- [ ] Create `argocd/qa/app-backup.yaml` (wave -2)
- [ ] Create `argocd/qa/appset-namespace.yaml` (wave -1)
- [ ] Create `argocd/qa/app-shared-services.yaml` (wave 0)
- [ ] Create `argocd/qa/appset-vault.yaml` (wave 1)
- [ ] Create `argocd/qa/appset-backend.yaml` (wave 2)
- [ ] Create `argocd/qa/appset-frontend.yaml` (wave 3)
- [ ] Repeat above for `preprod` and `prod` (removing `automated` from `syncPolicy`)

### Phase 3 — PAT Integration

- [ ] Create thin wrapper Helm chart `dlt-node-namespace` using existing Helm library
- [ ] Publish `dlt-node-namespace` chart to Artifactory
- [ ] Register custom health check for PAT CR in `argocd-cm` ConfigMap
- [ ] Verify PAT controller signals `status.phase = "Ready"` correctly
- [ ] Test wave -1 blocks wave 1 until namespace is ready

### Phase 4 — CI Pipeline

- [ ] Implement promotion script with `yq` for all version/targetRevision patches
- [ ] Configure `GITLAB_BOT_TOKEN` as CI variable (with push access to main)
- [ ] Add `manifest-diff` job for MR pipelines
- [ ] Test QA auto-promotion on merge to main
- [ ] Test manual pre-prod promotion gate
- [ ] Test manual prod promotion gate

### Phase 5 — ArgoCD Projects & Sync Windows

- [ ] Create `AppProject` for each env (`dlt-qa`, `dlt-preprod`, `dlt-prod`)
- [ ] Configure sync windows for QA (business hours auto-sync)
- [ ] Configure deny windows for prod (outside change window)
- [ ] Bootstrap root App of Apps per cluster with `argocd app create`

### Phase 6 — Validation

- [ ] Deploy to QA end-to-end — verify wave ordering
- [ ] Verify backup completes before any node component deploys
- [ ] Verify namespace health check blocks wave 1 until ready
- [ ] Run promote-preprod — verify manual sync works in ArgoCD UI
- [ ] Run manifest diff in an MR — verify output is readable and accurate
- [ ] Add a new node to QA by creating a folder — verify ApplicationSets auto-discover it

---

*Document generated from architecture brainstorming session. All YAML examples use placeholder values — substitute actual repo URLs, chart names, image registries, and namespace conventions before implementation.*
