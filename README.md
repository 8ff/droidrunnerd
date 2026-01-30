# DroidRun Task Queue Server

[![CI](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml/badge.svg)](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml)

API server for [droidrun](https://github.com/droidrun/droidrun) - automate your Android phone using LLMs (Claude, ChatGPT, Gemini, DeepSeek, Ollama). Just describe what you want done.

## Prerequisites

### Phone Setup
1. Enable Developer Options: **Settings > About Phone > Tap "Build Number" 7 times**
2. Enable USB Debugging: **Settings > Developer Options > USB Debugging**
3. Connect phone via USB
4. When prompted, tap **"Allow"** to authorize your computer

### Get an API Key
Get a key from one of the supported providers:
- [Google AI Studio](https://aistudio.google.com/apikey) - Gemini (recommended)
- [Anthropic](https://console.anthropic.com/) - Claude
- [OpenAI](https://platform.openai.com/api-keys) - ChatGPT / GPT-4
- [DeepSeek](https://platform.deepseek.com/) - DeepSeek
- [Ollama](https://ollama.ai/) - Local models (no API key needed)

## Quick Start

```bash
# 1. Start the server
docker run -d --name droidrun \
  --privileged \
  --network=host \
  -v /dev/bus/usb:/dev/bus/usb \
  -v ~/.android:/root/.android \
  -e DROIDRUN_SERVER_KEY="change-me" \
  ghcr.io/8ff/droidrunnerd:latest

# 2. Verify it's running
curl http://localhost:8000/health
```

> **Mac users:** Replace `--network=host` with `-p 8000:8000`. See [Wireless ADB](#wireless-adb-mac--no-usb) below.

## Wireless ADB (Mac / No USB)

Docker on Mac doesn't support USB passthrough, so you'll need to connect to your phone over WiFi instead.

### Phone Setup (Android 11+)

1. Go to **Settings > Developer Options > Wireless debugging**
2. Enable **Wireless debugging**
3. Tap **Pair device with pairing code** - note the pairing code and IP:port

### Start the Server

```bash
docker run -d --name droidrun \
  --privileged \
  -p 8000:8000 \
  -v ~/.android:/root/.android \
  -e DROIDRUN_SERVER_KEY="change-me" \
  ghcr.io/8ff/droidrunnerd:latest
```

### Connect via ADB

```bash
# Pair (one-time) - use IP:port and code from Wireless debugging screen
docker exec -it droidrun adb pair 192.168.1.100:37123
# Enter pairing code when prompted

# Connect (use the main IP:port shown under "Wireless debugging", not the pairing port)
docker exec droidrun adb connect 192.168.1.100:5555

# Verify connection
docker exec droidrun adb devices
```

> **Note:** The pairing port (e.g., 37123) and connect port (e.g., 5555) are different. Check the Wireless debugging screen for both.

## Usage

### CLI Client (Recommended)

```bash
# Download the client (or build: cd client && go build -o droidrun-client)
# Releases: https://github.com/8ff/droidrunnerd/releases

# Set credentials
export DROIDRUN_SERVER_KEY="change-me"
export LLM_API_KEY="your-api-key"

# Run a task (defaults to Google/Gemini)
./droidrun-client -server http://localhost:8000 -key $LLM_API_KEY "open settings"

# Specify provider and model
./droidrun-client -server http://localhost:8000 -key $LLM_API_KEY \
  -provider Anthropic -model claude-sonnet-4-20250514 "open settings"

# Run a predefined task
./droidrun-client -server http://localhost:8000 -task tasks/whatsapp-reply.toml

# Discover deep links for an app
./droidrun-client -server http://localhost:8000 -deeplinks com.instagram.android

# Run a task with a deep link (opens specific screen before the agent starts)
./droidrun-client -server http://localhost:8000 -key $LLM_API_KEY \
  -app com.instagram.android -deeplink "instagram://mainfeed" "like the first post"
```

### curl

```bash
# Submit a task (defaults to Google/Gemini)
curl -X POST http://localhost:8000/run \
  -H "Content-Type: application/json" \
  -H "X-Server-Key: $DROIDRUN_SERVER_KEY" \
  -H "X-API-Key: $LLM_API_KEY" \
  -d '{"goal":"open WhatsApp and send hello to Mom"}'

# With specific provider/model
curl -X POST http://localhost:8000/run \
  -H "Content-Type: application/json" \
  -H "X-Server-Key: $DROIDRUN_SERVER_KEY" \
  -H "X-API-Key: $LLM_API_KEY" \
  -d '{"goal":"open WhatsApp", "provider":"Anthropic", "model":"claude-sonnet-4-20250514"}'

# Check task status
curl -H "X-Server-Key: $DROIDRUN_SERVER_KEY" http://localhost:8000/task/TASK_ID

# Cancel a task
curl -X DELETE -H "X-Server-Key: $DROIDRUN_SERVER_KEY" http://localhost:8000/task/TASK_ID
```

### Task Files

Task files are TOML configs for reusable tasks. Example with a deep link:

```toml
[task]
name = "instagram-feed"
description = "Like the first post on Instagram feed"

[task.goal]
app = "com.instagram.android"
deeplink = "instagram://mainfeed"
prompt = """
1. You are on the Instagram main feed.
2. Like the first post you see.
3. Return to home desktop when done.
"""

[task.model]
provider = "Google"
model = "gemini-flash-latest"

[task.options]
reasoning = true
vision = false
max_steps = 15
```

Run with: `./droidrun-client -task tasks/instagram-feed.toml -server http://localhost:8000`

Use `-deeplinks` to discover available deep links for an app before writing task files.

## API Reference

**Base URL:** `http://localhost:8000`

**Authentication:** All endpoints except `/health` require the `X-Server-Key` header.

---

### POST /run

Submit a new task to the queue.

**Headers:**
```
Content-Type: application/json
X-Server-Key: your-server-key
X-API-Key: your-llm-api-key
```

**Request:**
```json
{
  "goal": "open WhatsApp and check unread messages",
  "provider": "Google",
  "model": "gemini-2.0-flash",
  "max_steps": 30
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `goal` | string | Yes | - | What you want the agent to do |
| `app` | string | No | - | Android package to launch (e.g. `com.whatsapp`) |
| `deeplink` | string | No | - | Deep link URI to open (e.g. `instagram://mainfeed`) |
| `provider` | string | No | `Google` | LLM provider (see below) |
| `model` | string | No | auto | Model name |
| `max_steps` | int | No | `30` | Maximum steps (1-100) |

If both `app` and `deeplink` are set, the app is launched first, then the deep link is opened. If only `deeplink` is set, it opens directly (which implicitly opens the app).

**Providers:**
| Provider | Default Model |
|----------|---------------|
| `Google` | `gemini-2.0-flash` |
| `Anthropic` | `claude-sonnet-4-20250514` |
| `OpenAI` | `gpt-4o` |
| `DeepSeek` | `deepseek-chat` |
| `Ollama` | `llama3.2` |

**Response:** `200 OK`
```json
{
  "task_id": "a1b2c3d4",
  "status": "queued",
  "position": 0
}
```

---

### GET /task/{id}

Get task status and result.

**Headers:**
```
X-Server-Key: your-server-key
```

**Response:** `200 OK`
```json
{
  "id": "a1b2c3d4",
  "status": "completed",
  "success": true,
  "result": "Found 3 unread messages in WhatsApp",
  "error": "",
  "logs": "...",
  "steps": [...],
  "request": {
    "goal": "open WhatsApp and check unread messages",
    "provider": "Google",
    "model": "gemini-2.0-flash",
    "max_steps": 30
  },
  "created_at": "2025-01-28T10:00:00Z",
  "started_at": "2025-01-28T10:00:01Z",
  "finished_at": "2025-01-28T10:00:15Z"
}
```

| Field | Description |
|-------|-------------|
| `status` | `queued`, `running`, `completed`, `failed`, `cancelled` |
| `success` | Whether the goal was achieved |
| `result` | Agent's final answer/summary |
| `error` | Error message if failed |
| `logs` | Execution logs |
| `steps` | Array of steps taken |

---

### DELETE /task/{id}

Cancel a queued or running task.

**Headers:**
```
X-Server-Key: your-server-key
```

**Response:** `200 OK`
```json
{
  "status": "cancelled"
}
```

---

### GET /deeplinks

Discover available deep links for an installed app. Runs `adb shell dumpsys package` and parses intent filters for non-http/https URI schemes.

**Headers:**
```
X-Server-Key: your-server-key
```

**Query Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `app` | Yes | Android package name (e.g. `com.instagram.android`) |

**Response:** `200 OK`
```json
{
  "app": "com.instagram.android",
  "deeplinks": [
    "instagram://camera",
    "instagram://mainfeed",
    "instagram://reels_home"
  ]
}
```

---

### GET /health

Health check. No authentication required.

**Response:** `200 OK`
```json
{
  "status": "ok",
  "version": "1.0.0",
  "queue_size": 0,
  "current_task": ""
}
```

---

### Errors

All errors return JSON:

```json
{
  "error": "error message",
  "request_id": "abc123"
}
```

| Code | Description |
|------|-------------|
| `400` | Bad request (invalid JSON, missing goal, etc.) |
| `401` | Unauthorized (missing or invalid `X-Server-Key`) |
| `404` | Task not found |
| `405` | Method not allowed |

## Build from Source

```bash
# Build container
docker build -t droidrun .

# Or build binaries directly
cd server && go build -o droidrun-server
cd client && go build -o droidrun-client
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DROIDRUN_SERVER_KEY` | **Required.** Server authentication key |
| `GOOGLE_API_KEY` | Google AI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |

## Troubleshooting

**No device detected:**
```bash
# Check ADB sees your phone
docker exec droidrun adb devices
# Should show your device, not "unauthorized"
```

**"unauthorized" device:**
- Check phone screen for USB debugging prompt
- Tap "Allow" and check "Always allow"

**Container won't start:**
- Ensure `DROIDRUN_SERVER_KEY` is set (required)

## Credits

This project is a wrapper around [droidrun](https://github.com/droidrun/droidrun) - the actual Android automation magic happens there. Check them out!

## License

MIT
