package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/rakeshavasarala/script-executor/internal/config"
	"github.com/rakeshavasarala/script-executor/internal/script"
)

// Logger writes audit logs.
type Logger struct {
	file   *os.File
	config config.AuditConfig
	mu     sync.Mutex
}

// NewLogger creates an audit logger.
func NewLogger(path string, cfg config.AuditConfig) (*Logger, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: file, config: cfg}, nil
}

// LogExecution logs a script execution.
func (l *Logger) LogExecution(executionID, user, runbookID, scriptHash string, source *script.Source, succeeded bool, duration time.Duration, exitCode int) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	evt := map[string]interface{}{
		"event":         "script_execution",
		"execution_id":  executionID,
		"user":          user,
		"runbook_id":    runbookID,
		"script_hash":   scriptHash,
		"succeeded":     succeeded,
		"duration_sec":  duration.Seconds(),
		"exit_code":     exitCode,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	if source != nil {
		evt["script_source"] = string(source.Type)
		if source.Name != "" {
			evt["script_ref"] = source.Name + "/" + source.Key
		}
	}
	data, _ := json.Marshal(evt)
	l.file.Write(append(data, '\n'))
}
