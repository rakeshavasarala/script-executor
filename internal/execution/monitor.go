package execution

import (
	"context"
	"fmt"
	"io"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// Monitor watches a Job until completion.
type Monitor struct {
	client    kubernetes.Interface
	namespace string
}

// NewMonitor creates a Job monitor.
func NewMonitor(client kubernetes.Interface, namespace string) *Monitor {
	return &Monitor{client: client, namespace: namespace}
}

// Result holds the execution result.
type Result struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	Duration   time.Duration
	JobName    string
	PodName    string
	Succeeded  bool
}

// Wait waits for the Job to complete and returns the result.
func (m *Monitor) Wait(ctx context.Context, job *batchv1.Job, timeout time.Duration) (*Result, error) {
	watcher, err := m.client.BatchV1().Jobs(m.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", job.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("watch job: %w", err)
	}
	defer watcher.Stop()

	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("job %s timed out after %v", job.Name, timeout)
			}
		}

		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				// Channel closed, re-fetch job status
				job, err = m.client.BatchV1().Jobs(m.namespace).Get(ctx, job.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				return m.collectResult(ctx, job)
			}
			if event.Type == watch.Deleted {
				return nil, fmt.Errorf("job %s was deleted", job.Name)
			}
			j, ok := event.Object.(*batchv1.Job)
			if !ok {
				continue
			}
			if j.Name != job.Name {
				continue
			}
			if isJobComplete(j) || isJobFailed(j) {
				return m.collectResult(ctx, j)
			}
		case <-time.After(2 * time.Second):
			// Re-fetch in case we missed events
			j, err := m.client.BatchV1().Jobs(m.namespace).Get(ctx, job.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			if isJobComplete(j) || isJobFailed(j) {
				return m.collectResult(ctx, j)
			}
		}
	}
}

func isJobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return job.Status.Succeeded > 0
}

func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return job.Status.Failed > 0
}

func (m *Monitor) collectResult(ctx context.Context, job *batchv1.Job) (*Result, error) {
	res := &Result{
		JobName:   job.Name,
		Succeeded: job.Status.Succeeded > 0,
	}

	// Get pod
	pods, err := m.client.CoreV1().Pods(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
	})
	if err != nil || len(pods.Items) == 0 {
		return res, nil
	}
	pod := &pods.Items[0]
	res.PodName = pod.Name

	// Duration from pod
	if pod.Status.StartTime != nil {
		endTime := metav1.Now()
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil && cs.State.Terminated.FinishedAt.After(endTime.Time) {
				endTime = cs.State.Terminated.FinishedAt
			}
		}
		res.Duration = endTime.Sub(pod.Status.StartTime.Time)
	}

	// Exit code and logs
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil {
			res.ExitCode = int(cs.State.Terminated.ExitCode)
			break
		}
	}

	// Fetch logs (stdout and stderr combined by default)
	req := m.client.CoreV1().Pods(m.namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "script",
	})
	logStream, err := req.Stream(ctx)
	if err == nil {
		defer logStream.Close()
		buf, _ := io.ReadAll(logStream)
		if len(buf) > 0 {
			res.Stdout = string(buf)
		}
	}

	return res, nil
}
