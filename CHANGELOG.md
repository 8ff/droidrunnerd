# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2025-01-28

### Added
- **Server authentication**: Optional `DROIDRUN_SERVER_KEY` env var to protect the server
- **Security**: LLM API key handling via `X-API-Key` header (keys no longer stored in task objects)
- **Health endpoint**: `GET /health` returns server status, version, and queue info
- **Input validation**: Goal required, provider validation, max_steps clamping (1-100)
- **JSON error responses**: All errors now return structured `{"error": "message"}` JSON
- **Request tracing**: `X-Request-ID` header on all responses for debugging
- **Graceful shutdown**: Server handles SIGINT/SIGTERM cleanly
- **Client improvements**: `--version` flag, `--quiet` mode for scripting
- **Test suite**: Comprehensive tests for API endpoints and queue operations
- **CI/CD**: GitHub Actions workflow for testing, building, and linting
- **Documentation**: Enhanced README with API reference and security section

### Changed
- API key is now sent via `X-API-Key` header instead of JSON body (backwards compatible)
- Task JSON output no longer includes API key field
- Default models updated for each provider

### Security
- API keys are never persisted or logged
- API keys passed to worker via stdin only
- Input validation prevents malformed requests

## [0.1.0] - 2025-01-20

### Added
- Initial release
- HTTP API server with task queue
- Go CLI client with TOML task file support
- Python worker wrapper for droidrun
- Container support (Podman/Docker)
- Support for multiple LLM providers (Google, Anthropic, OpenAI, DeepSeek, Ollama)
