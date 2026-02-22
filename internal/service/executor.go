package service

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	executorv1 "github.com/rakeshavasarala/script-executor/gen/go/proto/executor/v1"
	"github.com/rakeshavasarala/script-executor/internal/execution"
)

// ScriptExecutor implements the Executor gRPC service.
type ScriptExecutor struct {
	executorv1.UnimplementedExecutorServer
	manager *execution.Manager
}

// NewScriptExecutor creates a new ScriptExecutor service.
func NewScriptExecutor(manager *execution.Manager) *ScriptExecutor {
	return &ScriptExecutor{manager: manager}
}

// Execute handles script.run step execution.
func (s *ScriptExecutor) Execute(ctx context.Context, req *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	if req.StepType != "script.run" {
		return nil, status.Errorf(codes.Unimplemented, "unknown step type: %s", req.StepType)
	}
	return s.manager.Execute(ctx, req)
}

// ExecuteStream streams progress for script.run execution.
func (s *ScriptExecutor) ExecuteStream(req *executorv1.ExecuteRequest, stream grpc.ServerStreamingServer[executorv1.ExecuteProgress]) error {
	if req.StepType != "script.run" {
		return status.Errorf(codes.Unimplemented, "step type %s does not support streaming", req.StepType)
	}

	// Send starting
	stream.Send(&executorv1.ExecuteProgress{
		Stage:           executorv1.ExecuteProgress_STAGE_STARTING,
		PercentComplete: 0,
		Message:         "Validating and preparing script execution...",
		Timestamp:       timestamppb.Now(),
	})

	// Execute (same as Execute but we stream progress)
	startTime := time.Now()
	resp, err := s.manager.Execute(stream.Context(), req)
	if err != nil {
		return err
	}

	// Send running
	stream.Send(&executorv1.ExecuteProgress{
		Stage:           executorv1.ExecuteProgress_STAGE_RUNNING,
		PercentComplete: 50,
		Message:         "Script execution in progress...",
		Timestamp:       timestamppb.Now(),
	})

	// The manager already waits for completion, so we just send the result
	stream.Send(&executorv1.ExecuteProgress{
		Stage:           executorv1.ExecuteProgress_STAGE_DONE,
		PercentComplete: 100,
		Message:         fmt.Sprintf("Execution completed with status %s", resp.Status),
		Timestamp:       timestamppb.Now(),
		Result:          resp,
	})

	_ = startTime
	return nil
}

// Describe returns executor capabilities.
func (s *ScriptExecutor) Describe(ctx context.Context, req *executorv1.DescribeRequest) (*executorv1.DescribeResponse, error) {
	return &executorv1.DescribeResponse{
		Name:    "script",
		Version: "1.0.0",
		StepTypes: []*executorv1.StepTypeCapability{
			{
				Type:               "script.run",
				SupportsStreaming:  true,
				TypicalDuration:    durationpb.New(5 * time.Minute),
				Description:        "Execute shell, Python, or Ruby scripts in a secure Kubernetes Job",
				RequiredParameters: []string{},
				OptionalParameters: []string{
					"inline_script", "script_from_configmap", "script_from_secret", "script_path", "script_id",
					"image", "image_ref", "interpreter", "args", "env", "timeout",
					"env_from_secret", "env_from_configmap", "secret_env_all", "configmap_env_all",
					"volumes_from_secret", "volumes_from_configmap", "node_selector", "resources",
					"approval_required", "approvers",
				},
			},
		},
	}, nil
}

// Health returns health status.
func (s *ScriptExecutor) Health(ctx context.Context, req *executorv1.HealthRequest) (*executorv1.HealthResponse, error) {
	return &executorv1.HealthResponse{
		Status:    executorv1.HealthResponse_STATUS_SERVING,
		Timestamp: timestamppb.Now(),
		Details: map[string]string{
			"version": "1.0.0",
		},
	}, nil
}
