package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/rakeshavasarala/script-executor/internal/api"
	"github.com/rakeshavasarala/script-executor/internal/config"
	"github.com/rakeshavasarala/script-executor/internal/execution"
	executorv1 "github.com/rakeshavasarala/script-executor/gen/go/proto/executor/v1"
	"github.com/rakeshavasarala/script-executor/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create execution manager
	manager, err := execution.NewManager(cfg)
	if err != nil {
		log.Fatalf("Failed to create execution manager: %v", err)
	}

	// Create gRPC server
	grpcPort := cfg.ScriptExecutor.GRPC.Port
	if grpcPort == 0 {
		grpcPort = 50051
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(cfg.ScriptExecutor.GRPC.MaxMessageSize),
		grpc.MaxSendMsgSize(cfg.ScriptExecutor.GRPC.MaxMessageSize),
	)

	executorService := service.NewScriptExecutor(manager)
	executorv1.RegisterExecutorServer(grpcServer, executorService)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	// Start gRPC server
	go func() {
		log.Printf("gRPC server listening on :%d", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Start HTTP server for approval API
	var httpServer *api.Server
	if cfg.ScriptExecutor.Approval.Enabled {
		approvalHandler := api.NewApprovalHandler(manager.ApprovalChecker())
		httpServer = api.NewServer(8080, approvalHandler)
		go func() {
			log.Printf("HTTP server listening on :8080")
			httpServer.Start()
		}()
	}

	// Start metrics server
	metricsPort := cfg.ScriptExecutor.Monitoring.Metrics.Port
	if metricsPort == 0 {
		metricsPort = 9090
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", metricsPort),
		Handler: metricsMux,
	}
	go func() {
		log.Printf("Metrics server listening on :%d", metricsPort)
		metricsServer.ListenAndServe()
	}()

	log.Println("Script Executor started")
	log.Println("  - script.run (supports streaming)")
	log.Println("Test with: grpcurl -plaintext localhost:50051 list")

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	grpcServer.GracefulStop()
	metricsServer.Shutdown(ctx)
	if httpServer != nil {
		httpServer.Shutdown(ctx)
	}

	log.Println("Server stopped")
}
