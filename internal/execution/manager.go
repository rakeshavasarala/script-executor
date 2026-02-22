package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rakeshavasarala/script-executor/internal/approval"
	"github.com/rakeshavasarala/script-executor/internal/audit"
	"github.com/rakeshavasarala/script-executor/internal/config"
	"github.com/rakeshavasarala/script-executor/internal/image"
	"github.com/rakeshavasarala/script-executor/internal/script"
	"github.com/rakeshavasarala/script-executor/internal/security"
	executorv1 "github.com/rakeshavasarala/script-executor/gen/go/proto/executor/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Manager orchestrates script execution.
type Manager struct {
	config    *config.Config
	client    kubernetes.Interface
	loader    *script.Loader
	catalog   *image.Catalog
	resolver  *image.Resolver
	validator *image.Validator
	scriptVal *security.ScriptValidator
	approval  *approval.Checker
	jobBuilder *JobBuilder
	monitor   *Monitor
	auditLog  *audit.Logger
}

// NewManager creates an execution manager.
func NewManager(cfg *config.Config) (*Manager, error) {
	var k8sConfig *rest.Config
	var err error
	if cfg.ScriptExecutor.Kubernetes.Namespace == "" {
		cfg.ScriptExecutor.Kubernetes.Namespace = "default"
	}
	k8sConfig, err = rest.InClusterConfig()
	if err != nil {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		k8sConfig, err = kubeConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("k8s config: %w", err)
		}
	}
	client, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	namespace := cfg.ScriptExecutor.Kubernetes.Namespace
	loader, err := script.NewLoader(k8sConfig, namespace)
	if err != nil {
		return nil, err
	}

	imgCatalog := image.NewCatalog(client, namespace, cfg.ScriptExecutor.Image.Catalog.ConfigMapName)
	resolver := image.NewResolver(imgCatalog, image.ResolverDefaults{
		Image:      cfg.ScriptExecutor.Image.DefaultImage,
		PullSecret: cfg.ScriptExecutor.Image.DefaultImagePullSecret,
		PullPolicy: cfg.ScriptExecutor.Image.DefaultImagePullPolicy,
	})
	imgValidator := image.NewValidator(
		cfg.ScriptExecutor.Image.ApprovedImages,
		cfg.ScriptExecutor.Image.BlockedImages,
	)
	scriptValidator := security.NewScriptValidator(
		cfg.ScriptExecutor.Security.BlockedCommands,
		nil,
		cfg.ScriptExecutor.Security.MaxScriptSize,
		cfg.ScriptExecutor.Security.MaxScriptLines,
	)

	var approvalChecker *approval.Checker
	if cfg.ScriptExecutor.Approval.Enabled {
		store := approval.NewConfigMapStore(client, namespace, cfg.ScriptExecutor.Approval.Storage.ConfigMapName)
		approvalChecker = approval.NewChecker(store, cfg.ScriptExecutor.Approval.DefaultApprovers)
	}

	var auditLogger *audit.Logger
	if cfg.ScriptExecutor.Audit.Enabled {
		auditLogger, _ = audit.NewLogger(cfg.ScriptExecutor.Audit.LogFile, cfg.ScriptExecutor.Audit)
	}

	mgr := &Manager{
		config:    cfg,
		client:    client,
		loader:    loader,
		catalog:   imgCatalog,
		resolver:  resolver,
		validator: imgValidator,
		scriptVal: scriptValidator,
		approval:  approvalChecker,
		jobBuilder: NewJobBuilder(cfg),
		monitor:   NewMonitor(client, namespace),
		auditLog:  auditLogger,
	}
	return mgr, nil
}

// ApprovalChecker returns the approval checker for HTTP API wiring.
func (m *Manager) ApprovalChecker() *approval.Checker {
	return m.approval
}

// NewManagerWithClient creates a manager with an existing K8s client (for testing).
func NewManagerWithClient(cfg *config.Config, client kubernetes.Interface) *Manager {
	namespace := cfg.ScriptExecutor.Kubernetes.Namespace
	loader := script.NewLoaderWithClient(client, namespace)
	imgCatalog := image.NewCatalog(client, namespace, cfg.ScriptExecutor.Image.Catalog.ConfigMapName)
	resolver := image.NewResolver(imgCatalog, image.ResolverDefaults{
		Image:      cfg.ScriptExecutor.Image.DefaultImage,
		PullSecret: cfg.ScriptExecutor.Image.DefaultImagePullSecret,
		PullPolicy: cfg.ScriptExecutor.Image.DefaultImagePullPolicy,
	})
	imgValidator := image.NewValidator(
		cfg.ScriptExecutor.Image.ApprovedImages,
		cfg.ScriptExecutor.Image.BlockedImages,
	)
	scriptValidator := security.NewScriptValidator(
		cfg.ScriptExecutor.Security.BlockedCommands,
		nil,
		cfg.ScriptExecutor.Security.MaxScriptSize,
		cfg.ScriptExecutor.Security.MaxScriptLines,
	)

	var approvalChecker *approval.Checker
	if cfg.ScriptExecutor.Approval.Enabled {
		store := approval.NewConfigMapStore(client, namespace, cfg.ScriptExecutor.Approval.Storage.ConfigMapName)
		approvalChecker = approval.NewChecker(store, cfg.ScriptExecutor.Approval.DefaultApprovers)
	}

	var auditLogger *audit.Logger
	if cfg.ScriptExecutor.Audit.Enabled {
		auditLogger, _ = audit.NewLogger(cfg.ScriptExecutor.Audit.LogFile, cfg.ScriptExecutor.Audit)
	}

	return &Manager{
		config:    cfg,
		client:    client,
		loader:    loader,
		catalog:   imgCatalog,
		resolver:  resolver,
		validator: imgValidator,
		scriptVal: scriptValidator,
		approval:  approvalChecker,
		jobBuilder: NewJobBuilder(cfg),
		monitor:   NewMonitor(client, namespace),
		auditLog:  auditLogger,
	}
}

// Execute runs a script.run step.
func (m *Manager) Execute(ctx context.Context, req *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	startTime := time.Now()
	params := req.Parameters
	if params == nil {
		params = &structpb.Struct{Fields: make(map[string]*structpb.Value)}
	}

	execCtx := req.Context
	if execCtx == nil {
		execCtx = &executorv1.ExecutionContext{}
	}
	executionID := execCtx.ExecutionId
	if executionID == "" {
		executionID = fmt.Sprintf("exec-%d", time.Now().UnixNano())
	}
	user := execCtx.User
	runbookID := execCtx.RunbookId

	// 1. Load script
	scriptContent, source, err := m.loader.LoadScript(ctx, params)
	if err != nil {
		return errorResponse(err, startTime), nil
	}

	// For script_path, we don't have content - validation is skipped for path
	if source.Type != script.SourcePath && scriptContent != "" {
		// 2. Validate script
		if err := m.scriptVal.Validate(scriptContent); err != nil {
			return errorResponse(fmt.Errorf("script validation: %w", err), startTime), nil
		}
	}

	// 3. Script hash (for inline/configmap/secret)
	scriptHash := ""
	if scriptContent != "" {
		h := sha256.Sum256([]byte(scriptContent))
		scriptHash = hex.EncodeToString(h[:])
	}

	// 4. Resolve image
	imageStr := getString(params, "image", "")
	imageRef := getString(params, "image_ref", "")
	imagePullPolicy := getString(params, "image_pull_policy", "")
	imagePullSecret := getString(params, "image_pull_secret", "")

	resolved, err := m.resolver.Resolve(ctx, imageStr, imageRef, imagePullPolicy, imagePullSecret)
	if err != nil {
		return errorResponse(fmt.Errorf("resolve image: %w", err), startTime), nil
	}

	// 5. Validate image
	if err := m.validator.Validate(resolved.Image); err != nil {
		return errorResponse(fmt.Errorf("image validation: %w", err), startTime), nil
	}

	// 6. Check approval
	approvalRequired := getBool(params, "approval_required")
	if approvalRequired && m.approval != nil {
		approvers := getStringSlice(params, "approvers")
		if len(approvers) == 0 {
			approvers = m.config.ScriptExecutor.Approval.DefaultApprovers
		}
		stepName := getString(params, "step_name", "default")
		status, err := m.approval.Check(ctx, executionID, stepName, scriptContent, scriptHash, approvers, user)
		if err != nil {
			return errorResponse(err, startTime), nil
		}
		if status == approval.StatusPending {
			// Create approval request and return PENDING
			if err := m.approval.CreateRequest(ctx, executionID, stepName, runbookID, user, scriptContent, scriptHash, approvers); err != nil {
				return errorResponse(err, startTime), nil
			}
			return &executorv1.ExecuteResponse{
				Status:   executorv1.ExecuteResponse_STATUS_PENDING,
				Error:    "Awaiting approval",
				Duration: durationpbOf(time.Since(startTime)),
			}, nil
		}
		if status == approval.StatusDenied {
			return errorResponse(fmt.Errorf("execution was denied"), startTime), nil
		}
	}

	// 7. Build execution context
	execContext, err := BuildContext(
		params, execCtx,
		scriptContent, source, scriptHash,
		resolved.Image, string(resolved.PullPolicy), resolved.PullSecret,
		m.config,
	)
	if err != nil {
		return errorResponse(err, startTime), nil
	}
	execContext.ExecutionID = executionID
	execContext.RunbookID = runbookID
	execContext.User = user

	// 8. Build and create Job
	job, err := m.jobBuilder.Build(execContext)
	if err != nil {
		return errorResponse(fmt.Errorf("build job: %w", err), startTime), nil
	}

	created, err := m.client.BatchV1().Jobs(m.config.ScriptExecutor.Kubernetes.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return errorResponse(fmt.Errorf("create job: %w", err), startTime), nil
	}

	// 9. Wait for completion
	timeout := execContext.Timeout
	if req.Timeout != nil && req.Timeout.AsDuration() > 0 {
		timeout = req.Timeout.AsDuration()
	}
	result, err := m.monitor.Wait(ctx, created, timeout+30*time.Second)
	if err != nil {
		return errorResponse(fmt.Errorf("wait for job: %w", err), startTime), nil
	}

	// 10. Audit log
	if m.auditLog != nil {
		m.auditLog.LogExecution(executionID, user, runbookID, scriptHash, source, result.Succeeded, result.Duration, result.ExitCode)
	}

	// 11. Build response
	output := buildOutput(execContext, result)
	resp := &executorv1.ExecuteResponse{
		Status:   executorv1.ExecuteResponse_STATUS_SUCCEEDED,
		Output:   output,
		Duration: durationpbOf(result.Duration),
	}
	if !result.Succeeded {
		resp.Status = executorv1.ExecuteResponse_STATUS_FAILED
		resp.Error = fmt.Sprintf("exit code %d", result.ExitCode)
	}

	return resp, nil
}

func buildOutput(ctx *Context, result *Result) *structpb.Struct {
	fields := map[string]interface{}{
		"exit_code":        float64(result.ExitCode),
		"stdout":           result.Stdout,
		"stderr":           result.Stderr,
		"duration_seconds": result.Duration.Seconds(),
		"script_hash":      ctx.ScriptHash,
		"job_name":         result.JobName,
		"pod_name":         result.PodName,
	}
	out, _ := structpb.NewStruct(fields)
	return out
}

func errorResponse(err error, startTime time.Time) *executorv1.ExecuteResponse {
	return &executorv1.ExecuteResponse{
		Status:   executorv1.ExecuteResponse_STATUS_FAILED,
		Error:    err.Error(),
		Duration: durationpbOf(time.Since(startTime)),
	}
}

func durationpbOf(d time.Duration) *durationpb.Duration {
	return durationpb.New(d)
}
