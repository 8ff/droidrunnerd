package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestQueueSubmit(t *testing.T) {
	q := NewQueue("./worker.py")

	req := TaskRequest{
		Goal:     "test goal",
		Provider: "Google",
		Model:    "gemini-2.0-flash",
		MaxSteps: 10,
	}

	task := q.Submit(req, "test-api-key")

	if task.ID == "" {
		t.Error("expected task ID to be set")
	}

	if task.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", task.Status)
	}

	if task.Request.Goal != "test goal" {
		t.Errorf("expected goal 'test goal', got %q", task.Request.Goal)
	}

	if task.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestQueueSubmitDefaults(t *testing.T) {
	q := NewQueue("./worker.py")

	req := TaskRequest{
		Goal: "test",
	}

	task := q.Submit(req, "key")

	if task.Request.Provider != "Google" {
		t.Errorf("expected default provider 'Google', got %q", task.Request.Provider)
	}

	if task.Request.Model != "gemini-2.0-flash" {
		t.Errorf("expected default model 'gemini-2.0-flash', got %q", task.Request.Model)
	}

	if task.Request.MaxSteps != 30 {
		t.Errorf("expected default MaxSteps 30, got %d", task.Request.MaxSteps)
	}
}

func TestQueueGet(t *testing.T) {
	q := NewQueue("./worker.py")

	req := TaskRequest{Goal: "test"}
	task := q.Submit(req, "key")

	got := q.Get(task.ID)
	if got == nil {
		t.Fatal("expected to find task")
	}

	if got.ID != task.ID {
		t.Errorf("expected ID %q, got %q", task.ID, got.ID)
	}
}

func TestQueueGetNotFound(t *testing.T) {
	q := NewQueue("./worker.py")

	got := q.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent task")
	}
}

func TestQueueAll(t *testing.T) {
	q := NewQueue("./worker.py")

	q.Submit(TaskRequest{Goal: "test1"}, "key1")
	q.Submit(TaskRequest{Goal: "test2"}, "key2")
	q.Submit(TaskRequest{Goal: "test3"}, "key3")

	all := q.All()
	if len(all) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(all))
	}
}

func TestQueueSize(t *testing.T) {
	q := NewQueue("./worker.py")

	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}

	q.Submit(TaskRequest{Goal: "test"}, "key")
	// Size reflects pending channel
	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}
}

func TestQueueCancelQueued(t *testing.T) {
	q := NewQueue("./worker.py")

	task := q.Submit(TaskRequest{Goal: "test"}, "key")

	if !q.Cancel(task.ID) {
		t.Error("expected Cancel to succeed")
	}

	got := q.Get(task.ID)
	if got.Status != "cancelled" {
		t.Errorf("expected status 'cancelled', got %q", got.Status)
	}

	if got.FinishedAt.IsZero() {
		t.Error("expected FinishedAt to be set")
	}
}

func TestQueueCancelNotFound(t *testing.T) {
	q := NewQueue("./worker.py")

	if q.Cancel("nonexistent") {
		t.Error("expected Cancel to fail for nonexistent task")
	}
}

func TestQueueClear(t *testing.T) {
	q := NewQueue("./worker.py")

	q.Submit(TaskRequest{Goal: "test1"}, "key1")
	q.Submit(TaskRequest{Goal: "test2"}, "key2")

	count := q.Clear()
	if count != 2 {
		t.Errorf("expected Clear to return 2, got %d", count)
	}

	if len(q.All()) != 0 {
		t.Error("expected queue to be empty after Clear")
	}
}

func TestQueueCurrent(t *testing.T) {
	q := NewQueue("./worker.py")

	if q.Current() != "" {
		t.Error("expected Current to be empty initially")
	}
}

func TestQueuePosition(t *testing.T) {
	q := NewQueue("./worker.py")

	task := q.Submit(TaskRequest{Goal: "test"}, "key")
	pos := q.Position(task.ID)

	// Position returns Size(), so with 1 pending task it should be 1
	if pos != 1 {
		t.Errorf("expected position 1, got %d", pos)
	}
}

func TestTaskJSONDoesNotIncludeAPIKey(t *testing.T) {
	q := NewQueue("./worker.py")

	task := q.Submit(TaskRequest{
		Goal:     "test",
		Provider: "Google",
	}, "super-secret-api-key")

	// Marshal the task to JSON
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}

	// Check that the API key is not in the JSON
	jsonStr := string(data)
	if contains(jsonStr, "super-secret-api-key") {
		t.Error("API key should not be present in task JSON")
	}
	if contains(jsonStr, "api_key") {
		t.Error("api_key field should not be present in task JSON")
	}
}

func TestTaskRequestSafeFields(t *testing.T) {
	q := NewQueue("./worker.py")

	task := q.Submit(TaskRequest{
		Goal:      "test goal",
		App:       "com.test.app",
		Provider:  "Anthropic",
		Model:     "claude-3",
		Reasoning: true,
		Vision:    true,
		MaxSteps:  50,
	}, "api-key")

	// Verify the safe request struct has all expected fields
	if task.Request.Goal != "test goal" {
		t.Errorf("Goal mismatch")
	}
	if task.Request.App != "com.test.app" {
		t.Errorf("App mismatch")
	}
	if task.Request.Provider != "Anthropic" {
		t.Errorf("Provider mismatch")
	}
	if task.Request.Model != "claude-3" {
		t.Errorf("Model mismatch")
	}
	if !task.Request.Reasoning {
		t.Errorf("Reasoning should be true")
	}
	if !task.Request.Vision {
		t.Errorf("Vision should be true")
	}
	if task.Request.MaxSteps != 50 {
		t.Errorf("MaxSteps mismatch")
	}
}

func TestRandomID(t *testing.T) {
	// Test that IDs are unique
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := randomID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true

		// IDs should be 8 hex chars
		if len(id) != 8 {
			t.Errorf("expected ID length 8, got %d", len(id))
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		n        int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"exactly", 7, "exactly"},
		{"exactly8", 7, "exactly..."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.expected)
		}
	}
}

func TestTaskTimestamps(t *testing.T) {
	q := NewQueue("./worker.py")

	before := time.Now()
	task := q.Submit(TaskRequest{Goal: "test"}, "key")
	after := time.Now()

	if task.CreatedAt.Before(before) || task.CreatedAt.After(after) {
		t.Error("CreatedAt should be between before and after")
	}

	// StartedAt and FinishedAt should be zero initially
	if !task.StartedAt.IsZero() {
		t.Error("StartedAt should be zero for queued task")
	}
	if !task.FinishedAt.IsZero() {
		t.Error("FinishedAt should be zero for queued task")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
