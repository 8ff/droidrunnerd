# DroidRun Task Queue Server

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml/badge.svg)](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml)

A production-ready HTTP task queue server for [droidrun](https://github.com/droidrun/droidrun) - LLM-powered Android automation.

## Quick Start

```bash
# Build the server
cd server && go build -o droidrun-server

# Start the server
./droidrun-server 8000 ./worker.py

# Submit a task (using header-based API key)
curl -X POST http://localhost:8000/run \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $GOOGLE_API_KEY" \
  -d '{"goal":"open settings","provider":"Google"}'
```

## Architecture

```
[Clients] --HTTP--> [Go Server] --stdin/stdout--> [Python Worker] --ADB--> [Phone]
```

- **Go Server**: HTTP API with task queue (one task at a time, graceful shutdown)
- **Python Worker**: Runs droidrun agent, returns JSON result
- **Container**: Both server and worker packaged together

## Installation

### Docker Compose (Easiest)

```bash
# 1. Enable USB Debugging on your Android phone:
#    Settings > Developer Options > USB Debugging

# 2. Connect phone via USB

# 3. Create .env with your server key
echo "DROIDRUN_SERVER_KEY=your-secret-key" > .env

# 4. Start (will show setup instructions)
docker compose up

# 5. When prompted on phone, tap "Allow" for USB debugging

# 6. Check it's working
curl http://localhost:8000/health
```

The container will print helpful instructions if no device is detected.

### Manual Container Setup

```bash
# Build
docker build -t droidrun .

# Run with USB passthrough
docker run -d --name droidrun \
  --privileged \
  --network=host \
  -v /dev/bus/usb:/dev/bus/usb \
  -v ~/.android:/root/.android \
  -e DROIDRUN_SERVER_KEY="your-secret-key" \
  droidrun

# View startup logs (includes device status)
docker logs droidrun
```

**Required flags:**
- `--privileged` - USB device access
- `-v /dev/bus/usb:/dev/bus/usb` - USB device passthrough
- `-v ~/.android:/root/.android` - Persist ADB authorization

### Server Setup (Debian/Ubuntu - Manual)

```bash
# As root
./install.sh

# Connect phone, enable USB debugging, authorize
adb devices
```

### Build from Source

```bash
# Server
cd server && go build -o droidrun-server -ldflags "-X main.Version=0.2.0"

# Client
cd client && go build -o droidrun-client -ldflags "-X main.Version=0.2.0"
```

## Client Usage

### Go Client

```bash
# Simple task
./droidrun-client -key $GOOGLE_API_KEY "open settings"

# With task file
./droidrun-client -task tasks/whatsapp-reply.toml -server http://10.0.0.65:8000

# Quiet mode for scripting
./droidrun-client -quiet -key $GOOGLE_API_KEY "check battery" | jq .success

# Show version
./droidrun-client -version
```

### curl

```bash
# Submit task
curl -X POST http://localhost:8000/run \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $GOOGLE_API_KEY" \
  -d '{"goal":"open settings","provider":"Google"}'

# Check status
curl http://localhost:8000/task/TASK_ID

# Health check
curl http://localhost:8000/health

# View queue
curl http://localhost:8000/queue
```

## API Reference

### POST /run

Submit a task to the queue.

**Headers:**
- `Content-Type: application/json`
- `X-API-Key: <your-api-key>` (required except for Ollama)

**Request Body:**
```json
{
  "goal": "open settings and enable dark mode",
  "app": "com.android.settings",
  "provider": "Google",
  "model": "gemini-2.0-flash",
  "reasoning": true,
  "vision": false,
  "max_steps": 30
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `goal` | string | Yes | - | What you want the agent to do |
| `app` | string | No | - | Android package to launch first |
| `provider` | string | No | `Google` | LLM provider (see below) |
| `model` | string | No | varies | Model name for the provider |
| `reasoning` | bool | No | `false` | Enable reasoning mode |
| `vision` | bool | No | `false` | Enable vision mode |
| `max_steps` | int | No | `30` | Max steps (clamped 1-100) |

**Providers:**
| Provider | Default Model | Env Var |
|----------|---------------|---------|
| `Google` | `gemini-2.0-flash` | `GOOGLE_API_KEY` |
| `Anthropic` | `claude-sonnet-4-20250514` | `ANTHROPIC_API_KEY` |
| `OpenAI` | `gpt-4o` | `OPENAI_API_KEY` |
| `DeepSeek` | `deepseek-chat` | `DEEPSEEK_API_KEY` |
| `Ollama` | `llama3.2` | (none needed) |

**Response:**
```json
{
  "task_id": "a1b2c3d4",
  "status": "queued",
  "position": 0
}
```

### GET /task/{id}

Get task status and result.

**Response:**
```json
{
  "id": "a1b2c3d4",
  "request": {
    "goal": "open settings",
    "provider": "Google",
    "model": "gemini-2.0-flash",
    "reasoning": false,
    "vision": false,
    "max_steps": 30
  },
  "status": "completed",
  "success": true,
  "result": "Successfully opened settings",
  "steps": [...],
  "created_at": "2025-01-27T10:00:00Z",
  "started_at": "2025-01-27T10:00:01Z",
  "finished_at": "2025-01-27T10:00:15Z"
}
```

**Status values:** `queued`, `running`, `completed`, `failed`, `cancelled`

### DELETE /task/{id}

Cancel a queued or running task.

### GET /queue

View all tasks in the queue.

### DELETE /queue

Clear all tasks from the queue.

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "version": "0.2.0",
  "queue_size": 0,
  "current_task": null
}
```

## Error Handling

All errors return JSON:

```json
{
  "error": "goal is required",
  "request_id": "abc123def456"
}
```

Common errors:
- `400` - Invalid request (missing goal, invalid provider, etc.)
- `404` - Task not found
- `405` - Method not allowed

## Security

### Server Authentication

Protect your server from unauthorized access by setting `DROIDRUN_SERVER_KEY`:

```bash
export DROIDRUN_SERVER_KEY="your-secret-key"
./droidrun-server
```

When enabled, all requests (except `/health`) must include the `X-Server-Key` header:

```bash
curl -X POST http://localhost:8000/run \
  -H "X-Server-Key: your-secret-key" \
  -H "X-API-Key: $GOOGLE_API_KEY" \
  -d '{"goal":"open settings"}'
```

**Note:** Without `DROIDRUN_SERVER_KEY` set, the server runs without authentication. Always enable this in production.

### LLM API Key Handling

- LLM API keys are sent via the `X-API-Key` header (recommended)
- Keys are **never** stored in task objects or logs
- Keys are passed to the worker via stdin only
- Task JSON responses do not include API keys

### Request Tracing

All responses include an `X-Request-ID` header for debugging. You can also send your own request ID:

```bash
curl -H "X-Request-ID: my-trace-123" http://localhost:8000/health
```

### Best Practices

1. Always use environment variables for API keys
2. Run the server behind a reverse proxy with TLS in production
3. Use the health endpoint for monitoring
4. Monitor the `X-Request-ID` for debugging

## Task Files

Define reusable automation tasks in TOML:

```toml
# tasks/whatsapp-reply.toml
[task]
name = "whatsapp-reply"
description = "Reply to unread WhatsApp messages"

[task.goal]
prompt = "Open WhatsApp and reply to any unread messages with 'I'll get back to you soon'"
app = "com.whatsapp"

[task.model]
provider = "Google"
model = "gemini-2.0-flash"

[task.options]
reasoning = true
vision = false
max_steps = 30
```

## Files

```
install.sh          # Server setup script
Containerfile       # Container: Go server + Python worker + ADB
worker.py           # Python droidrun wrapper
server/             # Go HTTP server source
client/             # Go CLI client
tasks/              # Task definition files (TOML)
```

## Development

```bash
# Run tests
cd server && go test -v ./...

# Run with race detector
cd server && go test -race ./...

# Build with version
go build -ldflags "-X main.Version=$(git describe --tags)" -o droidrun-server
```

## Related Projects

- [droidrun](https://github.com/droidrun/droidrun) - The core LLM-powered Android automation framework

## License

MIT License - see [LICENSE](LICENSE)
