package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os/exec"
	"sync"
	"time"
)

// TaskRequest represents an incoming task request.
// Note: APIKey is accepted but never stored or included in JSON output.
type TaskRequest struct {
	Goal      string `json:"goal"`
	App       string `json:"app,omitempty"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Reasoning bool   `json:"reasoning"`
	Vision    bool   `json:"vision"`
	MaxSteps  int    `json:"max_steps"`
	APIKey    string `json:"api_key,omitempty"` // Only used for backwards-compat parsing, never stored
}

// TaskRequestSafe is the sanitized version without sensitive fields.
// This is what gets stored and returned in API responses.
type TaskRequestSafe struct {
	Goal      string `json:"goal"`
	App       string `json:"app,omitempty"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Reasoning bool   `json:"reasoning"`
	Vision    bool   `json:"vision"`
	MaxSteps  int    `json:"max_steps"`
}

type Task struct {
	ID         string          `json:"id"`
	Request    TaskRequestSafe `json:"request"`
	Status     string          `json:"status"` // queued, running, completed, failed, cancelled
	Success    bool            `json:"success,omitempty"`
	Result     string          `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Logs       string          `json:"logs,omitempty"`
	Steps      any             `json:"steps,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  time.Time       `json:"started_at,omitempty"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`

	// apiKey is stored internally but never serialized to JSON
	apiKey string
}

type Queue struct {
	mu           sync.RWMutex
	tasks        map[string]*Task
	pending      chan string
	pendingOrder []string // Track order of pending tasks for Position()
	current      string
	currentCmd   *exec.Cmd
	workerPath   string
}

func NewQueue(workerPath string) *Queue {
	return &Queue{
		tasks:      make(map[string]*Task),
		pending:    make(chan string, 100),
		workerPath: workerPath,
	}
}

func (q *Queue) Submit(req TaskRequest, apiKey string) *Task {
	// Apply defaults
	if req.Provider == "" {
		req.Provider = "Google"
	}
	if req.Model == "" {
		req.Model = "gemini-2.0-flash"
	}
	if req.MaxSteps == 0 {
		req.MaxSteps = 30
	}

	id := randomID()
	task := &Task{
		ID: id,
		Request: TaskRequestSafe{
			Goal:      req.Goal,
			App:       req.App,
			Provider:  req.Provider,
			Model:     req.Model,
			Reasoning: req.Reasoning,
			Vision:    req.Vision,
			MaxSteps:  req.MaxSteps,
		},
		Status:    "queued",
		CreatedAt: time.Now(),
		apiKey:    apiKey, // Store internally, not in JSON
	}

	q.mu.Lock()
	q.tasks[id] = task
	q.pendingOrder = append(q.pendingOrder, id)
	q.mu.Unlock()

	q.pending <- id
	return task
}

func (q *Queue) Get(id string) *Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tasks[id]
}

func (q *Queue) All() map[string]*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	cp := make(map[string]*Task)
	for k, v := range q.tasks {
		cp[k] = v
	}
	return cp
}

func (q *Queue) Size() int {
	return len(q.pending)
}

func (q *Queue) Current() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.current
}

func (q *Queue) Position(id string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// If currently running, position is 0
	if q.current == id {
		return 0
	}

	// Find position in pending order
	for i, taskID := range q.pendingOrder {
		if taskID == id {
			return i + 1 // 1-based position (0 means running)
		}
	}

	return -1 // Not found in queue
}

func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task := q.tasks[id]
	if task == nil {
		return false
	}

	// If running, kill the process
	if task.Status == "running" && q.currentCmd != nil && q.current == id {
		if err := q.currentCmd.Process.Kill(); err != nil {
			log.Printf("[%s] Failed to kill process: %v", id, err)
		}
	}

	// If queued or running, mark as cancelled
	if task.Status == "queued" || task.Status == "running" {
		task.Status = "cancelled"
		task.FinishedAt = time.Now()
		q.removePendingOrder(id)
		return true
	}
	return false
}

func (q *Queue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Kill current task if running
	if q.currentCmd != nil {
		if err := q.currentCmd.Process.Kill(); err != nil {
			log.Printf("Failed to kill current process: %v", err)
		}
	}

	count := len(q.tasks)
	q.tasks = make(map[string]*Task)
	q.current = ""
	q.pendingOrder = nil

	// Drain pending queue
	for len(q.pending) > 0 {
		<-q.pending
	}

	return count
}

func (q *Queue) Run() {
	for id := range q.pending {
		q.process(id)
	}
}

func (q *Queue) process(id string) {
	q.mu.Lock()
	task := q.tasks[id]
	if task == nil {
		q.mu.Unlock()
		return
	}
	task.Status = "running"
	task.StartedAt = time.Now()
	q.current = id
	q.removePendingOrder(id)
	apiKey := task.apiKey // Get the stored API key
	q.mu.Unlock()

	log.Printf("[%s] Starting task: %s", id, truncate(task.Request.Goal, 50))

	// Build input for worker - include API key here (passed via stdin, not stored)
	input, _ := json.Marshal(map[string]any{
		"goal":      task.Request.Goal,
		"app":       task.Request.App,
		"provider":  task.Request.Provider,
		"model":     task.Request.Model,
		"reasoning": task.Request.Reasoning,
		"vision":    task.Request.Vision,
		"max_steps": task.Request.MaxSteps,
		"api_key":   apiKey,
	})

	// Run worker
	cmd := exec.Command("python3", q.workerPath)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	q.mu.Lock()
	q.currentCmd = cmd
	q.mu.Unlock()

	err := cmd.Run()
	output := stdout.Bytes()

	q.mu.Lock()
	q.currentCmd = nil
	task.FinishedAt = time.Now()
	task.Logs = stderr.String()
	q.current = ""

	// Check if cancelled while running
	if task.Status == "cancelled" {
		log.Printf("[%s] Cancelled", id)
		q.mu.Unlock()
		return
	}

	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		if stderr.Len() > 0 {
			task.Error = stderr.String()
		}
		log.Printf("[%s] Failed: %s", id, task.Error)
	} else {
		var result struct {
			OK      bool   `json:"ok"`
			Success bool   `json:"success"`
			Reason  string `json:"reason"`
			Error   string `json:"error"`
			Steps   any    `json:"steps"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			task.Status = "failed"
			task.Error = "invalid worker output: " + string(output)
		} else if !result.OK {
			task.Status = "failed"
			task.Error = result.Error
		} else {
			task.Status = "completed"
			task.Success = result.Success
			task.Result = result.Reason
			task.Steps = result.Steps
		}
		log.Printf("[%s] Completed: success=%v", id, task.Success)
	}
	q.mu.Unlock()
}

// removePendingOrder removes an id from pendingOrder slice.
// Must be called with mu held.
func (q *Queue) removePendingOrder(id string) {
	for i, taskID := range q.pendingOrder {
		if taskID == id {
			q.pendingOrder = append(q.pendingOrder[:i], q.pendingOrder[i+1:]...)
			return
		}
	}
}

func randomID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		log.Printf("Failed to generate random ID: %v", err)
	}
	return hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
