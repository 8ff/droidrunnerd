# Build stage - compile Go server from source
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY server/ ./server/
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o droidrun-server .

# Runtime stage
FROM python:3.12-slim

# Install ADB and utilities
RUN apt-get update && apt-get install -y \
    android-tools-adb \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install droidrun + LLM packages
RUN pip install --no-cache-dir 'droidrun[google,anthropic,openai,deepseek,ollama]' llama-index-llms-gemini

# Copy server binary, worker script, and entrypoint
COPY --from=builder /build/server/droidrun-server /usr/local/bin/droidrun-server
COPY worker.py /app/worker.py
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

WORKDIR /app

# Configuration via environment variables
ENV PORT=8000
# Set DROIDRUN_SERVER_KEY for authentication (recommended)

EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:${PORT}/health || exit 1

# Use entrypoint script for user-friendly startup
ENTRYPOINT ["/app/entrypoint.sh"]
