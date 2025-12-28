# Copilot Instructions for SlackCommandRelay

## Project Overview

SlackCommandRelay is a Go web service that receives Slack Slash Command requests, verifies their authenticity, and publishes them to Redis pub/sub. This project is conceptually similar to [SlackRelay](https://github.com/its-the-vibe/SlackRelay) but handles Slash Commands instead of Events API callbacks.

## Architecture

- **HTTP Server**: Listens on configurable port (default 8080) for POST requests to `/command`
- **Request Verification**: HMAC SHA256 signature verification using Slack signing secret
- **Data Flow**: URL-encoded form data → JSON → Redis pub/sub
- **Single Endpoint**: `/command` handles all Slack command types
- **Redis Integration**: Optional pub/sub publishing to configurable channel

## Coding Standards

### Go Conventions
- Follow standard Go formatting with `gofmt`
- Use explicit error handling - never ignore errors
- Prefer standard library packages over third-party dependencies
- Use structured logging with log level prefixes (`[DEBUG]`, `[INFO]`, `[WARN]`, `[ERROR]`)
- Keep functions focused and maintainable

### Code Style
- Use descriptive variable names
- Add comments only when necessary to explain complex logic
- Follow existing patterns in the codebase
- No unnecessary abstractions - keep it simple

### Security Best Practices
- Always verify Slack signatures when signing secret is configured
- Validate timestamps to prevent replay attacks (5-minute tolerance window)
- Never log sensitive data at INFO level or above - only at DEBUG level
- Handle errors gracefully without exposing internal details
- Use constant-time comparison for HMAC verification

## Build and Test

### Building
```bash
# Download dependencies
go mod download

# Build the application
go build -o slack-command-relay

# Build Docker image
docker build -t slack-command-relay .
```

### Running
```bash
# Run locally
./slack-command-relay

# Run with Docker
docker run -p 8080:8080 slack-command-relay

# Run with Docker Compose
docker-compose up -d
```

### Testing
- No automated test suite currently exists
- Manual testing can be done with curl:
```bash
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "X-Slack-Request-Timestamp: $(date +%s)" \
  -d "command=/test&text=hello&user_name=testuser&user_id=U123&team_id=T123&channel_id=C123"
```

## Configuration

### Environment Variables
- `PORT`: Server port (default: `8080`)
- `LOG_LEVEL`: Logging verbosity - `DEBUG`, `INFO`, `WARN`, `ERROR` (default: `INFO`)
- `REDIS_HOST`: Redis server hostname (default: `localhost`)
- `REDIS_PORT`: Redis server port (default: `6379`)
- `REDIS_PASSWORD`: Redis password (optional)
- `REDIS_CHANNEL`: Redis pub/sub channel name (default: `slack-commands`)

### Secret Management
- Slack signing secret stored in `.secret` file (git-ignored)
- File should contain only the signing secret, trimmed of whitespace
- Application starts with warning if `.secret` file is missing

## Logging Guidelines

### Log Levels
- **DEBUG**: Verbose output including full command payloads (may contain sensitive data)
- **INFO**: Standard operational messages (default)
- **WARN**: Warning conditions (e.g., missing secret file, Redis connection failures)
- **ERROR**: Error conditions that don't stop the service

### When to Log What
- Use `logInfo` for normal operations (command received, Redis publish success)
- Use `logWarn` for degraded operation (Redis unavailable, missing secret)
- Use `logError` for failures (parsing errors, Redis publish failures)
- Use `logDebug` ONLY for sensitive data (command payloads, detailed debugging)

## Key Dependencies

- `github.com/redis/go-redis/v9`: Redis client
- Go 1.25.5
- Standard library: `crypto/hmac`, `encoding/json`, `net/http`

## Common Tasks

### Adding New Environment Variables
1. Define a constant or variable at package level if needed
2. Read from `os.Getenv()` in `main()` with appropriate defaults
3. Log the configuration value at INFO level during startup
4. Update README.md configuration section

### Modifying Request Handling
- All Slack command handling is in `slackCommandHandler()`
- Maintain URL-encoded form parsing → JSON conversion flow
- Preserve signature verification before processing
- Keep Redis publishing optional and non-blocking

### Error Handling Pattern
- Read errors: return 400 Bad Request
- Signature verification failures: return 401 Unauthorized
- Method not allowed: return 405
- Don't fail requests if Redis publishing fails - log error and continue

## Important Notes

- Service should remain operational even if Redis is unavailable
- Signature verification is skipped if `.secret` file doesn't exist (with warning)
- All commands are published to a single Redis channel (no filtering by command type)
- Response to Slack should be immediate (200 OK) - don't wait for downstream processing
- Timestamp tolerance is 5 minutes (300 seconds) as per Slack recommendations
