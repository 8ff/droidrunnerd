package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

// Version is set at build time
var Version = "dev"

// Task file structs
type TaskFile struct {
	Task TaskConfig `toml:"task"`
}

type TaskConfig struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	Goal        GoalConfig  `toml:"goal"`
	Model       ModelConfig `toml:"model"`
	Options     Options     `toml:"options"`
}

type GoalConfig struct {
	Prompt   string `toml:"prompt"`
	App      string `toml:"app"`      // package name to launch first
	Deeplink string `toml:"deeplink"` // deep link URI to open (e.g. instagram://mainfeed)
}

type ModelConfig struct {
	Provider string `toml:"provider"`
	Model    string `toml:"model"`
}

type Options struct {
	Reasoning bool `toml:"reasoning"`
	Vision    bool `toml:"vision"`
	MaxSteps  int  `toml:"max_steps"`
}

// API structs
type TaskRequest struct {
	Goal      string `json:"goal"`
	App       string `json:"app,omitempty"`
	Deeplink  string `json:"deeplink,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Reasoning bool   `json:"reasoning"`
	Vision    bool   `json:"vision"`
	MaxSteps  int    `json:"max_steps,omitempty"`
}

type SubmitResponse struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`
	Position int    `json:"position"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type TaskStatus struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Success    bool   `json:"success"`
	Result     string `json:"result"`
	Error      string `json:"error"`
	Logs       string `json:"logs"`
	Steps      any    `json:"steps"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

func main() {
	server := flag.String("server", "http://localhost:8000", "Server URL")
	provider := flag.String("provider", "", "LLM provider (overrides task file)")
	model := flag.String("model", "", "Model name (overrides task file)")
	reasoning := flag.Bool("reasoning", true, "Use reasoning mode")
	vision := flag.Bool("vision", false, "Use vision mode")
	maxSteps := flag.Int("steps", 30, "Max steps")
	apiKey := flag.String("key", "", "API key (or set env var based on provider)")
	taskFile := flag.String("task", "", "Task file (TOML)")
	appPkg := flag.String("app", "", "App package to launch first (e.g. com.whatsapp)")
	deeplink := flag.String("deeplink", "", "Deep link URI to open (e.g. instagram://mainfeed)")
	deeplinksApp := flag.String("deeplinks", "", "Discover deep links for an app package (e.g. com.instagram.android)")
	clearTasks := flag.Bool("clear", false, "Clear all tasks from server queue")
	quiet := flag.Bool("quiet", false, "Quiet mode - minimal output for scripting")
	showVersion := flag.Bool("version", false, "Show version and exit")
	serverKey := flag.String("server-key", "", "Server authentication key (or DROIDRUN_SERVER_KEY env)")
	flag.Parse()

	// Get server key from flag or env
	srvKey := *serverKey
	if srvKey == "" {
		srvKey = os.Getenv("DROIDRUN_SERVER_KEY")
	}

	// Handle -version flag
	if *showVersion {
		fmt.Printf("droidrun-client version %s\n", Version)
		os.Exit(0)
	}

	// Handle -clear flag
	if *clearTasks {
		req, _ := http.NewRequest("DELETE", *server+"/queue", nil)
		if srvKey != "" {
			req.Header.Set("X-Server-Key", srvKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = resp.Body.Close() }()
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}
		if !*quiet {
			fmt.Printf("Cleared %v tasks\n", result["cleared"])
		}
		os.Exit(0)
	}

	// Handle -deeplinks flag: discover deep links for an app
	if *deeplinksApp != "" {
		dlReq, _ := http.NewRequest("GET", *server+"/deeplinks?app="+*deeplinksApp, nil)
		if srvKey != "" {
			dlReq.Header.Set("X-Server-Key", srvKey)
		}
		dlResp, err := http.DefaultClient.Do(dlReq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = dlResp.Body.Close() }()

		if dlResp.StatusCode != http.StatusOK {
			var errResp ErrorResponse
			bodyBytes, _ := io.ReadAll(dlResp.Body)
			if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", errResp.Error)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", string(bodyBytes))
			}
			os.Exit(1)
		}

		var dlResult struct {
			App       string   `json:"app"`
			Deeplinks []string `json:"deeplinks"`
		}
		if err := json.NewDecoder(dlResp.Body).Decode(&dlResult); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Deep links for %s:\n", dlResult.App)
		if len(dlResult.Deeplinks) == 0 {
			fmt.Println("  (none found)")
		}
		for _, dl := range dlResult.Deeplinks {
			fmt.Printf("  %s\n", dl)
		}
		os.Exit(0)
	}

	var goal, prov, mod, app, dl string
	var reason, vis bool
	var steps int

	if *taskFile != "" {
		// Load from task file
		var tf TaskFile
		if _, err := toml.DecodeFile(*taskFile, &tf); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading task file: %v\n", err)
			os.Exit(1)
		}

		goal = tf.Task.Goal.Prompt
		app = tf.Task.Goal.App
		dl = tf.Task.Goal.Deeplink
		prov = tf.Task.Model.Provider
		mod = tf.Task.Model.Model
		reason = tf.Task.Options.Reasoning
		vis = tf.Task.Options.Vision
		steps = tf.Task.Options.MaxSteps

		if steps == 0 {
			steps = 30
		}

		if !*quiet {
			fmt.Printf("Task:    %s\n", tf.Task.Name)
			fmt.Printf("Desc:    %s\n", tf.Task.Description)
		}
	} else {
		// Use command line args
		if flag.NArg() < 1 {
			fmt.Println("Usage: droidrun-client [flags] \"goal\"")
			fmt.Println("       droidrun-client -task <file.toml> [flags]")
			fmt.Println("\nFlags:")
			flag.PrintDefaults()
			fmt.Println("\nExamples:")
			fmt.Println("  droidrun-client -key $GOOGLE_API_KEY \"open settings\"")
			fmt.Println("  droidrun-client -task tasks/whatsapp-reply.toml -server http://10.0.0.65:8000")
			os.Exit(1)
		}

		goal = flag.Arg(0)
		prov = "Google"
		mod = "gemini-2.0-flash"
		reason = *reasoning
		vis = *vision
		steps = *maxSteps
	}

	// Command line flags override task file values
	if *provider != "" {
		prov = *provider
	}
	if *model != "" {
		mod = *model
	}
	if *appPkg != "" {
		app = *appPkg
	}
	if *deeplink != "" {
		dl = *deeplink
	}

	// Get API key from flag or env
	key := *apiKey
	if key == "" {
		switch prov {
		case "Google", "GoogleGenAI":
			key = os.Getenv("GOOGLE_API_KEY")
		case "Anthropic":
			key = os.Getenv("ANTHROPIC_API_KEY")
		case "OpenAI":
			key = os.Getenv("OPENAI_API_KEY")
		case "DeepSeek":
			key = os.Getenv("DEEPSEEK_API_KEY")
		case "Ollama":
			// Ollama doesn't need an API key
		}
	}

	if key == "" && prov != "Ollama" {
		fmt.Fprintln(os.Stderr, "Error: API key required (-key flag or env var)")
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Server:  %s\n", *server)
		fmt.Printf("Model:   %s/%s\n", prov, mod)
		if app != "" {
			fmt.Printf("App:     %s\n", app)
		}
		if dl != "" {
			fmt.Printf("Link:    %s\n", dl)
		}
		fmt.Printf("Goal:    %s\n\n", truncate(goal, 60))
	}

	// Submit task (without API key in body)
	req := TaskRequest{
		Goal:      goal,
		App:       app,
		Deeplink:  dl,
		Provider:  prov,
		Model:     mod,
		Reasoning: reason,
		Vision:    vis,
		MaxSteps:  steps,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", *server+"/run", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", key) // Send LLM API key via header
	if srvKey != "" {
		httpReq.Header.Set("X-Server-Key", srvKey) // Server authentication
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		bodyBytes, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", errResp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", string(bodyBytes))
		}
		os.Exit(1)
	}

	var submitResp SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
		os.Exit(1)
	}

	if submitResp.TaskID == "" {
		fmt.Fprintln(os.Stderr, "Error: no task ID received")
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("Task:    %s (position: %d)\n", submitResp.TaskID, submitResp.Position)
		fmt.Println("Waiting...")
	}

	// Handle Ctrl+C to cancel task
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if !*quiet {
			fmt.Println("\nCancelling task...")
		}
		cancelReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/task/%s", *server, submitResp.TaskID), nil)
		if srvKey != "" {
			cancelReq.Header.Set("X-Server-Key", srvKey)
		}
		_, _ = http.DefaultClient.Do(cancelReq) // Best effort cancel before exit
		os.Exit(130)
	}()

	// Poll for result
	for {
		pollReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/task/%s", *server, submitResp.TaskID), nil)
		if srvKey != "" {
			pollReq.Header.Set("X-Server-Key", srvKey)
		}
		resp, err := http.DefaultClient.Do(pollReq)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		var status TaskStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			_ = resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		_ = resp.Body.Close()

		switch status.Status {
		case "queued":
			if !*quiet {
				fmt.Print(".")
			}
		case "running":
			if !*quiet {
				fmt.Print("\r[running]   ")
			}
		case "completed":
			if !*quiet {
				fmt.Print("\r            \r")
				fmt.Println("=== COMPLETED ===")
				fmt.Printf("Success: %v\n\n", status.Success)
				if status.Logs != "" {
					fmt.Println("=== LOGS ===")
					fmt.Printf("%s\n", status.Logs)
				}
				if status.Steps != nil {
					fmt.Println("=== STEPS ===")
					stepsJSON, _ := json.MarshalIndent(status.Steps, "", "  ")
					fmt.Printf("%s\n\n", stepsJSON)
				}
				fmt.Printf("Result:\n%s\n", status.Result)
			} else {
				// Quiet mode: output JSON
				output, _ := json.Marshal(map[string]any{
					"success": status.Success,
					"result":  status.Result,
				})
				fmt.Println(string(output))
			}
			if status.Success {
				os.Exit(0)
			}
			os.Exit(1)
		case "failed":
			if !*quiet {
				fmt.Print("\r            \r")
				fmt.Println("=== FAILED ===")
				fmt.Printf("Error: %s\n", status.Error)
			} else {
				output, _ := json.Marshal(map[string]any{
					"success": false,
					"error":   status.Error,
				})
				fmt.Println(string(output))
			}
			os.Exit(1)
		case "cancelled":
			if !*quiet {
				fmt.Print("\r            \r")
				fmt.Println("=== CANCELLED ===")
			}
			os.Exit(130)
		}

		time.Sleep(2 * time.Second)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
