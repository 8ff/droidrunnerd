package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}

	// Should have X-Request-ID header
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}
}

func TestHealthEndpointWrongMethod(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("POST", "/health", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestRunEndpointValidation(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	tests := []struct {
		name       string
		body       string
		apiKey     string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing goal",
			body:       `{"provider":"Google"}`,
			apiKey:     "test-key",
			wantStatus: http.StatusBadRequest,
			wantError:  "goal is required",
		},
		{
			name:       "empty goal",
			body:       `{"goal":"   ","provider":"Google"}`,
			apiKey:     "test-key",
			wantStatus: http.StatusBadRequest,
			wantError:  "goal is required",
		},
		{
			name:       "invalid provider",
			body:       `{"goal":"test","provider":"InvalidProvider"}`,
			apiKey:     "test-key",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid provider",
		},
		{
			name:       "missing API key for non-Ollama",
			body:       `{"goal":"test","provider":"Google"}`,
			apiKey:     "",
			wantStatus: http.StatusBadRequest,
			wantError:  "API key required",
		},
		{
			name:       "Ollama without API key is OK",
			body:       `{"goal":"test","provider":"Ollama"}`,
			apiKey:     "",
			wantStatus: http.StatusOK,
			wantError:  "",
		},
		{
			name:       "valid request with header key",
			body:       `{"goal":"test","provider":"Google"}`,
			apiKey:     "test-key",
			wantStatus: http.StatusOK,
			wantError:  "",
		},
		{
			name:       "invalid app package",
			body:       `{"goal":"test","provider":"Ollama","app":"invalid"}`,
			apiKey:     "",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid app package",
		},
		{
			name:       "valid app package",
			body:       `{"goal":"test","provider":"Ollama","app":"com.whatsapp"}`,
			apiKey:     "",
			wantStatus: http.StatusOK,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/run", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantError != "" {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if !strings.Contains(resp.Error, tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, resp.Error)
				}
			}
		})
	}
}

func TestRunEndpointWrongMethod(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("GET", "/run", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestRunEndpointInvalidJSON(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("POST", "/run", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "invalid JSON") {
		t.Errorf("expected 'invalid JSON' error, got %q", resp.Error)
	}
}

func TestTaskEndpointNotFound(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("GET", "/task/nonexistent", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestQueueEndpoint(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	req := httptest.NewRequest("GET", "/queue", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["queue_size"]; !ok {
		t.Error("expected queue_size in response")
	}
}

func TestRequestIDPropagation(t *testing.T) {
	q := NewQueue("./worker.py")
	api := NewAPI(q)

	// Test that provided X-Request-ID is echoed back
	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "test-request-123")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "test-request-123" {
		t.Errorf("expected X-Request-ID 'test-request-123', got %q", got)
	}
}

func TestMaxStepsClamping(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 30},    // default
		{-5, 30},   // negative becomes default
		{1, 1},     // min valid
		{50, 50},   // mid-range
		{100, 100}, // max valid
		{200, 100}, // clamped to max
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			req := &TaskRequest{
				Goal:     "test",
				Provider: "Ollama",
				MaxSteps: tt.input,
			}
			err := validateRequest(req, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.MaxSteps != tt.expected {
				t.Errorf("MaxSteps: expected %d, got %d", tt.expected, req.MaxSteps)
			}
		})
	}
}

func TestServerAuthentication(t *testing.T) {
	// Save and restore original serverAPIKey
	origKey := serverAPIKey
	defer func() { serverAPIKey = origKey }()

	q := NewQueue("./worker.py")
	api := NewAPI(q)

	// Test with auth enabled
	serverAPIKey = "test-server-key"

	// Health endpoint should work without auth
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("health should work without auth, got %d", w.Code)
	}

	// Other endpoints should require auth
	req = httptest.NewRequest("GET", "/queue", nil)
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without server key, got %d", w.Code)
	}

	// With wrong key
	req = httptest.NewRequest("GET", "/queue", nil)
	req.Header.Set("X-Server-Key", "wrong-key")
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong key, got %d", w.Code)
	}

	// With correct key
	req = httptest.NewRequest("GET", "/queue", nil)
	req.Header.Set("X-Server-Key", "test-server-key")
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with correct key, got %d", w.Code)
	}

	// With auth disabled
	serverAPIKey = ""
	req = httptest.NewRequest("GET", "/queue", nil)
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with auth disabled, got %d", w.Code)
	}
}

func TestModelDefaults(t *testing.T) {
	tests := []struct {
		provider      string
		expectedModel string
	}{
		{"Google", "gemini-2.0-flash"},
		{"GoogleGenAI", "gemini-2.0-flash"},
		{"Anthropic", "claude-sonnet-4-20250514"},
		{"OpenAI", "gpt-4o"},
		{"DeepSeek", "deepseek-chat"},
		{"Ollama", "llama3.2"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			req := &TaskRequest{
				Goal:     "test",
				Provider: tt.provider,
			}
			apiKey := "test-key"
			if tt.provider == "Ollama" {
				apiKey = ""
			}
			err := validateRequest(req, apiKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.Model != tt.expectedModel {
				t.Errorf("expected model %q, got %q", tt.expectedModel, req.Model)
			}
		})
	}
}
