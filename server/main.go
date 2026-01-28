package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Version is set at build time
var Version = "dev"

// serverAPIKey is the optional authentication key for the server itself
var serverAPIKey = os.Getenv("DROIDRUN_SERVER_KEY")

// Valid providers for LLM backends
var validProviders = map[string]bool{
	"Google":      true,
	"GoogleGenAI": true,
	"Anthropic":   true,
	"OpenAI":      true,
	"DeepSeek":    true,
	"Ollama":      true,
}

func main() {
	port := "8000"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	workerPath := "./worker.py"
	if len(os.Args) > 2 {
		workerPath = os.Args[2]
	}

	q := NewQueue(workerPath)
	go q.Run()

	api := NewAPI(q)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      api,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown handling
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Server shutting down...")

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Could not gracefully shutdown: %v", err)
		}
		close(done)
	}()

	log.Printf("DroidRun server v%s starting on :%s", Version, port)
	log.Printf("Worker: %s", workerPath)
	if serverAPIKey != "" {
		log.Printf("Server authentication: enabled")
	} else {
		log.Printf("Server authentication: disabled (set DROIDRUN_SERVER_KEY to enable)")
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	<-done
	log.Println("Server stopped")
}

// --- HTTP API (easy to replace) ---

type API struct {
	queue *Queue
	mux   *http.ServeMux
}

func NewAPI(q *Queue) *API {
	a := &API{queue: q, mux: http.NewServeMux()}
	a.mux.HandleFunc("/run", a.handleRun)
	a.mux.HandleFunc("/task/", a.handleTask)
	a.mux.HandleFunc("/queue", a.handleQueue)
	a.mux.HandleFunc("/health", a.handleHealth)
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add request ID for tracing
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateRequestID()
	}
	w.Header().Set("X-Request-ID", requestID)

	// Server authentication (skip for health check)
	if serverAPIKey != "" && r.URL.Path != "/health" {
		if r.Header.Get("X-Server-Key") != serverAPIKey {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	a.mux.ServeHTTP(w, r)
}

// ErrorResponse represents a JSON error response
type ErrorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(ErrorResponse{
		Error:     msg,
		RequestID: w.Header().Get("X-Request-ID"),
	}); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"version":      Version,
		"queue_size":   a.queue.Size(),
		"current_task": a.queue.Current(),
	}); err != nil {
		log.Printf("Failed to encode health response: %v", err)
	}
}

func (a *API) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get API key from header (preferred) or body (fallback for backwards compatibility)
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = req.APIKey
	}
	req.APIKey = "" // Clear from request struct (don't store)

	// Validation
	if err := validateRequest(&req, apiKey); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	task := a.queue.Submit(req, apiKey)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"task_id":  task.ID,
		"status":   task.Status,
		"position": a.queue.Position(task.ID),
	}); err != nil {
		log.Printf("Failed to encode run response: %v", err)
	}
}

func validateRequest(req *TaskRequest, apiKey string) error {
	// Goal is required
	req.Goal = strings.TrimSpace(req.Goal)
	if req.Goal == "" {
		return fmt.Errorf("goal is required")
	}

	// Provider validation
	if req.Provider == "" {
		req.Provider = "Google" // default
	}
	if !validProviders[req.Provider] {
		return fmt.Errorf("invalid provider: %s (valid: Google, Anthropic, OpenAI, DeepSeek, Ollama)", req.Provider)
	}

	// Model defaults
	if req.Model == "" {
		switch req.Provider {
		case "Google", "GoogleGenAI":
			req.Model = "gemini-2.0-flash"
		case "Anthropic":
			req.Model = "claude-sonnet-4-20250514"
		case "OpenAI":
			req.Model = "gpt-4o"
		case "DeepSeek":
			req.Model = "deepseek-chat"
		case "Ollama":
			req.Model = "llama3.2"
		}
	}

	// MaxSteps clamping (1-100)
	if req.MaxSteps <= 0 {
		req.MaxSteps = 30
	} else if req.MaxSteps > 100 {
		req.MaxSteps = 100
	}

	// API key required (except for Ollama which runs locally)
	if apiKey == "" && req.Provider != "Ollama" {
		return fmt.Errorf("API key required (use X-API-Key header)")
	}

	// App package validation (if provided)
	if req.App != "" {
		// Android package names: letters, digits, underscores, dots
		matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`, req.App)
		if !matched {
			return fmt.Errorf("invalid app package name: %s", req.App)
		}
	}

	return nil
}

func (a *API) handleTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/task/"):]
	if id == "" {
		writeError(w, "task ID required", http.StatusBadRequest)
		return
	}

	if r.Method == "DELETE" {
		if a.queue.Cancel(id) {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"}); err != nil {
				log.Printf("Failed to encode cancel response: %v", err)
			}
		} else {
			writeError(w, "cannot cancel (task not found or already completed)", http.StatusBadRequest)
		}
		return
	}

	if r.Method != "GET" {
		writeError(w, "GET or DELETE only", http.StatusMethodNotAllowed)
		return
	}

	task := a.queue.Get(id)
	if task == nil {
		writeError(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(task); err != nil {
		log.Printf("Failed to encode task response: %v", err)
	}
}

func (a *API) handleQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		count := a.queue.Clear()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"cleared": count}); err != nil {
			log.Printf("Failed to encode clear response: %v", err)
		}
		return
	}

	if r.Method != "GET" {
		writeError(w, "GET or DELETE only", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"queue_size":   a.queue.Size(),
		"current_task": a.queue.Current(),
		"tasks":        a.queue.All(),
	}); err != nil {
		log.Printf("Failed to encode queue response: %v", err)
	}
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		log.Printf("Failed to generate request ID: %v", err)
	}
	return hex.EncodeToString(b)
}

// --- Public interface for custom APIs ---

func (a *API) Submit(req TaskRequest, apiKey string) *Task {
	return a.queue.Submit(req, apiKey)
}

func (a *API) GetTask(id string) *Task {
	return a.queue.Get(id)
}

func (a *API) QueueSize() int {
	return a.queue.Size()
}

func (a *API) QueueStatus() map[string]any {
	return map[string]any{
		"queue_size":   a.queue.Size(),
		"current_task": a.queue.Current(),
		"tasks":        a.queue.All(),
	}
}
