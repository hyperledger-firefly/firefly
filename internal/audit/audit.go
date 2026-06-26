package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/log"
)

const (
	maxLogSize    = 100 * 1024 * 1024 // 100MB
	maxLogAge     = 30 * 24 * time.Hour // 30 days
	maxLogBackups = 10
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"eventType"`
	UserID      string                 `json:"userId"`
	Action      string                 `json:"action"`
	Resource    string                 `json:"resource"`
	IPAddress   string                 `json:"ipAddress"`
	UserAgent   string                 `json:"userAgent"`
	Details     map[string]interface{} `json:"details"`
	Status      string                 `json:"status"`
	Error       string                 `json:"error,omitempty"`
}

// AuditLogger handles security audit logging
type AuditLogger struct {
	mu       sync.RWMutex
	logFile  *os.File
	basePath string
	size     int64
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(basePath string) (*AuditLogger, error) {
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %v", err)
	}

	// Create a new log file with timestamp
	timestamp := time.Now().Format("2006-01-02")
	logPath := filepath.Join(basePath, fmt.Sprintf("audit-%s.log", timestamp))
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit log file: %v", err)
	}

	// Get initial file size
	info, err := logFile.Stat()
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("failed to get log file info: %v", err)
	}

	return &AuditLogger{
		logFile:  logFile,
		basePath: basePath,
		size:     info.Size(),
	}, nil
}

// LogEvent logs a security audit event
func (al *AuditLogger) LogEvent(ctx context.Context, event *AuditEvent) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %v", err)
	}

	// Add newline for log file
	data = append(data, '\n')

	// Check if we need to rotate the log
	if al.size+int64(len(data)) > maxLogSize {
		if err := al.rotateLog(); err != nil {
			return fmt.Errorf("failed to rotate log: %v", err)
		}
	}

	// Write to log file
	n, err := al.logFile.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write audit event: %v", err)
	}

	// Update size
	al.size += int64(n)

	// Also log to standard logger for monitoring
	log.L(ctx).Infof("AUDIT: %s - %s - %s - %s - %s", 
		event.EventType,
		event.UserID,
		event.Action,
		event.Resource,
		event.Status)

	return nil
}

// Close closes the audit logger
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.logFile != nil {
		return al.logFile.Close()
	}
	return nil
}

// RotateLog rotates the audit log file
func (al *AuditLogger) rotateLog() error {
	// Close current file
	if al.logFile != nil {
		al.logFile.Close()
	}

	// Create new file with current timestamp
	timestamp := time.Now().Format("2006-01-02")
	logPath := filepath.Join(al.basePath, fmt.Sprintf("audit-%s.log", timestamp))
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to create new audit log file: %v", err)
	}

	al.logFile = logFile
	al.size = 0

	// Clean up old log files
	if err := al.cleanupOldLogs(); err != nil {
		return fmt.Errorf("failed to cleanup old logs: %v", err)
	}

	return nil
}

// cleanupOldLogs removes old log files
func (al *AuditLogger) cleanupOldLogs() error {
	entries, err := os.ReadDir(al.basePath)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if file is an audit log
		if !strings.HasPrefix(entry.Name(), "audit-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		// Parse timestamp from filename
		timestamp, err := time.Parse("2006-01-02", strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "audit-"), ".log"))
		if err != nil {
			continue
		}

		// Check if file is too old
		if now.Sub(timestamp) > maxLogAge {
			path := filepath.Join(al.basePath, entry.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove old log file %s: %v", path, err)
			}
		}
	}

	return nil
} 