# DroidRun Task Queue Server

[![CI](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml/badge.svg)](https://github.com/8ff/droidrunnerd/actions/workflows/ci.yml)

HTTP task queue for [droidrun](https://github.com/droidrun/droidrun) - LLM-powered Android automation.

## Prerequisites

### Phone Setup
1. Enable Developer Options: **Settings > About Phone > Tap "Build Number" 7 times**
2. Enable USB Debugging: **Settings > Developer Options > USB Debugging**
3. Connect phone via USB
4. When prompted, tap **"Allow"** to authorize your computer

### Get an LLM API Key
Get a key from one of the supported providers:
- [Google AI Studio](https://aistudio.google.com/apikey) (recommended)
- [Anthropic](https://console.anthropic.com/)
- [OpenAI](https://platform.openai.com/api-keys)

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

## Usage

### CLI Client (Recommended)

```bash
# Download the client (or build: cd client && go build -o droidrun-client)
# Releases: https://github.com/8ff/droidrunnerd/releases

# Set credentials
export DROIDRUN_SERVER_KEY="change-me"
export GOOGLE_API_KEY="your-api-key"

# Run a task
./droidrun-client -server http://localhost:8000 -key $GOOGLE_API_KEY "open settings"

# Run a predefined task
./droidrun-client -server http://localhost:8000 -task tasks/whatsapp-reply.toml
```

### curl

```bash
# Submit a task
curl -X POST http://localhost:8000/run \
  -H "Content-Type: application/json" \
  -H "X-Server-Key: $DROIDRUN_SERVER_KEY" \
  -H "X-API-Key: $GOOGLE_API_KEY" \
  -d '{"goal":"open WhatsApp and send hello to Mom"}'

# Check task status
curl -H "X-Server-Key: $DROIDRUN_SERVER_KEY" http://localhost:8000/task/TASK_ID

# Cancel a task
curl -X DELETE -H "X-Server-Key: $DROIDRUN_SERVER_KEY" http://localhost:8000/task/TASK_ID
```

## API Reference

All endpoints (except `/health`) require `X-Server-Key` header.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/run` | POST | Submit a task |
| `/task/{id}` | GET | Get task status |
| `/task/{id}` | DELETE | Cancel task |
| `/health` | GET | Health check (no auth) |

**Task request fields:**
| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `goal` | Yes | - | What you want the agent to do |
| `provider` | No | `Google` | `Google`, `Anthropic`, `OpenAI`, `DeepSeek`, `Ollama` |
| `model` | No | auto | Model name (e.g., `gemini-2.0-flash`) |
| `max_steps` | No | `30` | Max steps (1-100) |

**Task status values:** `queued`, `running`, `completed`, `failed`, `cancelled`

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

## License

MIT
