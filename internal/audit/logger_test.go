package audit

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLog_ToZap(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	logger := New(zapLogger, nil)

	event := Event{
		Timestamp: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		UserID:    "user-123",
		OrgID:     "org-456",
		Action:    "token.create",
		Resource:  "tok-789",
		IP:        "192.168.1.100",
		UserAgent: "Mozilla/5.0",
		Status:    "success",
		Details:   map[string]string{"scope": "inference"},
	}

	logger.Log(context.Background(), event)

	if recorded.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", recorded.Len())
	}

	entry := recorded.All()[0]
	if entry.Message != "audit" {
		t.Errorf("expected message 'audit', got '%s'", entry.Message)
	}

	// Verify all expected fields are present
	fieldMap := make(map[string]interface{})
	for _, f := range entry.Context {
		fieldMap[f.Key] = f
	}

	expectedFields := []string{
		"audit.action",
		"audit.status",
		"audit.user_id",
		"audit.org_id",
		"audit.resource",
		"audit.ip",
		"audit.user_agent",
		"audit.timestamp",
		"audit.details",
	}

	for _, name := range expectedFields {
		if _, ok := fieldMap[name]; !ok {
			t.Errorf("missing expected field '%s'", name)
		}
	}

	// Verify field values
	for _, f := range entry.Context {
		switch f.Key {
		case "audit.action":
			if f.String != "token.create" {
				t.Errorf("expected audit.action 'token.create', got '%s'", f.String)
			}
		case "audit.status":
			if f.String != "success" {
				t.Errorf("expected audit.status 'success', got '%s'", f.String)
			}
		case "audit.user_id":
			if f.String != "user-123" {
				t.Errorf("expected audit.user_id 'user-123', got '%s'", f.String)
			}
		case "audit.org_id":
			if f.String != "org-456" {
				t.Errorf("expected audit.org_id 'org-456', got '%s'", f.String)
			}
		}
	}
}

func TestLog_NilDB(t *testing.T) {
	// Ensure logging with nil DB doesn't panic
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	logger := New(zapLogger, nil)

	event := Event{
		UserID: "user-1",
		OrgID:  "org-1",
		Action: "login",
		Status: "success",
	}

	// Should not panic
	logger.Log(context.Background(), event)

	if recorded.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", recorded.Len())
	}
}

func TestLog_DefaultTimestamp(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	logger := New(zapLogger, nil)

	before := time.Now().UTC()
	event := Event{
		Action: "test.action",
		Status: "success",
	}
	logger.Log(context.Background(), event)
	after := time.Now().UTC()

	if recorded.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", recorded.Len())
	}

	// Find the timestamp field
	entry := recorded.All()[0]
	for _, f := range entry.Context {
		if f.Key == "audit.timestamp" {
			ts := time.Unix(0, f.Integer)
			if ts.Before(before) || ts.After(after) {
				t.Errorf("timestamp %v outside expected range [%v, %v]", ts, before, after)
			}
			return
		}
	}
	t.Error("audit.timestamp field not found")
}

func TestLog_NilDetails(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	logger := New(zapLogger, nil)

	event := Event{
		Action:  "logout",
		Status:  "success",
		Details: nil,
	}

	logger.Log(context.Background(), event)

	if recorded.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", recorded.Len())
	}

	// Verify that audit.details is not present when nil
	entry := recorded.All()[0]
	for _, f := range entry.Context {
		if f.Key == "audit.details" {
			t.Error("audit.details should not be present when Details is nil")
		}
	}
}

func TestNew(t *testing.T) {
	zapLogger := zap.NewNop()
	logger := New(zapLogger, nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.zap != zapLogger {
		t.Error("expected zap logger to be set")
	}
	if logger.db != nil {
		t.Error("expected db to be nil")
	}
}
