# SlackCommandRelay

A simple Go web service which consumes Slack commands and publishes to Redis pub/sub

## Features

- Receives and parses Slack Slash Command requests
- Verifies Slack request signatures using HMAC SHA256
- Publishes all command payloads as JSON to a configurable Redis pub/sub channel
- Configurable log levels (DEBUG, INFO, WARN, ERROR)
- Configurable port via environment variable
- Configurable Redis connection via environment variables
- Docker and Docker Compose support for easy deployment

## Configuration

### Redis Channel Configuration

The service publishes all received Slack commands to a single Redis pub/sub channel. The channel name is configurable via the `REDIS_CHANNEL` environment variable.

**Environment Variables:**

- `REDIS_CHANNEL`: Redis pub/sub channel name (default: `slack-commands`)

**Example:**

```bash
# Use default channel (slack-commands)
./slack-command-relay

# Use custom channel
REDIS_CHANNEL=my-custom-channel ./slack-command-relay
```

### Log Level Configuration

Control the verbosity of logging with the `LOG_LEVEL` environment variable.

**Available Log Levels:**

- `DEBUG`: Most verbose, includes command payloads
- `INFO`: Standard operational messages (default)
- `WARN`: Warning messages only
- `ERROR`: Error messages only

**Environment Variables:**

- `LOG_LEVEL`: Sets the logging level (default: `INFO`)

**Note:** Command payloads are only logged when `LOG_LEVEL` is set to `DEBUG`. This prevents sensitive data from appearing in logs during normal operation.

**Example:**

```bash
# Use INFO level (default)
./slack-command-relay

# Use DEBUG level to see command payloads
LOG_LEVEL=DEBUG ./slack-command-relay

# Use WARN level for minimal logging
LOG_LEVEL=WARN ./slack-command-relay
```

### Port Configuration

The server port can be configured via the `PORT` environment variable. If not set, it defaults to `8080`.

```bash
# Run on default port 8080
./slack-command-relay

# Run on custom port
PORT=3000 ./slack-command-relay
```

### Redis Configuration

The service publishes received commands to Redis pub/sub. All commands are published to the configured channel as JSON payloads.

**Environment Variables:**

- `REDIS_HOST`: Redis server hostname (default: `localhost`)
- `REDIS_PORT`: Redis server port (default: `6379`)

**Note:** If the Redis connection fails, the application will log a warning and continue to work without Redis publishing. This ensures the service remains operational even if Redis is unavailable.

```bash
# Run with Redis configuration
REDIS_HOST=redis.example.com REDIS_PORT=6379 ./slack-command-relay

# Run with default Redis settings (connects to localhost:6379)
./slack-command-relay
```

### Slack Signing Secret

To enable Slack request signature verification:

1. Create a `.secret` file in the application directory
2. Add your Slack app's signing secret to this file (found in your Slack app's Basic Information page)
3. The application will automatically load this secret on startup

**Note:** If the `.secret` file is not found, the application will start but signature verification will be skipped (with a warning logged).

#### Setting up Slack Slash Commands

1. Create a Slack app at https://api.slack.com/apps
2. Navigate to "Slash Commands" in your app settings
3. Click "Create New Command"
4. Set the Request URL to your server's `/command` endpoint (e.g., `https://your-server.com/command`)
5. Configure the command name (e.g., `/mycommand`), description, and usage hint
6. Get your signing secret from "Basic Information" → "App Credentials" → "Signing Secret"

Example `.secret` file:
```
your-signing-secret-here
```

**Security:** The `.secret` file is excluded from version control via `.gitignore`.

## Building and Running

### Local Development

```bash
# Initialize Go modules (first time only)
go mod download

# Build the application
go build -o slack-command-relay

# Run the server
./slack-command-relay

# Run with custom Redis channel
REDIS_CHANNEL=my-commands ./slack-command-relay

# Run with custom port
PORT=3000 ./slack-command-relay

# Run with DEBUG logging
LOG_LEVEL=DEBUG ./slack-command-relay

# Run with Redis configuration
REDIS_HOST=redis.example.com REDIS_PORT=6379 ./slack-command-relay

# Run with all options
LOG_LEVEL=DEBUG REDIS_CHANNEL=my-commands REDIS_HOST=localhost PORT=8080 ./slack-command-relay
```

### Using Docker

```bash
# Build the Docker image
docker build -t slack-command-relay .

# Run the container
docker run -p 8080:8080 slack-command-relay

# Run with custom log level
docker run -p 8080:8080 -e LOG_LEVEL=DEBUG slack-command-relay

# Run with custom port
docker run -p 3000:8080 -e PORT=8080 slack-command-relay

# Run with secret file
docker run -p 8080:8080 -v $(pwd)/.secret:/app/.secret:ro slack-command-relay

# Run with Redis configuration (connecting to Redis on host machine)
docker run -p 8080:8080 -e REDIS_HOST=host.docker.internal -e REDIS_PORT=6379 -e REDIS_CHANNEL=my-commands slack-command-relay
```

### Using Docker Compose

The easiest way to run the application:

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose down
```

To use custom configuration with Docker Compose, you can set environment variables:

```bash
# Custom port
PORT=3000 docker-compose up -d

# Custom log level
LOG_LEVEL=DEBUG docker-compose up -d

# Custom Redis channel
REDIS_CHANNEL=my-commands docker-compose up -d

# Redis configuration
REDIS_HOST=192.168.1.100 REDIS_PORT=6379 docker-compose up -d
```

The docker-compose configuration automatically mounts the `.secret` file if it exists.

## API Endpoints

### POST /command

Accepts Slack Slash Command requests and publishes command payloads to Redis as JSON.

**Headers:**
- `X-Slack-Request-Timestamp`: Unix timestamp when the request was sent - **required**
- `X-Slack-Signature`: HMAC signature for request verification (verified if signing secret is configured)

**Request Body (URL-encoded form data):**

Slack sends command data as URL-encoded form data with the following fields:
- `token`: Verification token (deprecated, use signature verification instead)
- `team_id`: Unique identifier for the workspace
- `team_domain`: Domain name of the workspace
- `channel_id`: ID of the channel where the command was issued
- `channel_name`: Name of the channel
- `user_id`: ID of the user who issued the command
- `user_name`: Username of the user
- `command`: The command that was typed (e.g., `/mycommand`)
- `text`: The text following the command
- `response_url`: URL to send delayed responses
- `trigger_id`: ID to trigger modals
- `api_app_id`: ID of the app

**Published JSON Payload:**

The service converts the URL-encoded form data to JSON before publishing to Redis:

```json
{
  "token": "gIkuvaNzQIHg97ATvDxqgjtO",
  "team_id": "T0001",
  "team_domain": "example",
  "channel_id": "C2147483705",
  "channel_name": "test",
  "user_id": "U2147483697",
  "user_name": "Steve",
  "command": "/weather",
  "text": "94070",
  "response_url": "https://hooks.slack.com/commands/1234/5678",
  "trigger_id": "13345224609.738474920.8088930838d88f008e0",
  "api_app_id": "A123456"
}
```

**Response:**
- `200 OK`: Command received and processed successfully
- `401 Unauthorized`: Invalid request signature
- `405 Method Not Allowed`: Non-POST request
- `400 Bad Request`: Invalid form data or request body error

## Testing

### Manual Testing with curl

```bash
# Test with a sample command (without signature verification)
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "X-Slack-Request-Timestamp: $(date +%s)" \
  -d "token=test&team_id=T123&team_domain=example&channel_id=C123&channel_name=general&user_id=U123&user_name=testuser&command=/test&text=hello world&response_url=https://example.com&trigger_id=123.456&api_app_id=A123"
```

**Note:** Without a `.secret` file, signature verification is skipped for testing purposes.

### Testing Redis Integration

If you have Redis running locally, you can subscribe to the channel and see commands being published:

```bash
# Subscribe to the default commands channel
redis-cli
127.0.0.1:6379> SUBSCRIBE slack-commands

# In another terminal, send a test command to the service
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "X-Slack-Request-Timestamp: $(date +%s)" \
  -d "command=/test&text=hello&user_name=testuser&user_id=U123&team_id=T123&channel_id=C123"
```

## Development

The project follows standard Go conventions:
- Use `gofmt` for code formatting
- Explicit error handling
- Standard library packages preferred

## Architecture

This service is conceptually similar to [SlackRelay](https://github.com/its-the-vibe/SlackRelay) and follows the same project structure and layout, adapted for Slack Slash Commands instead of Slack Events API.

**Key Differences from SlackRelay:**
- Accepts Slack Slash Commands instead of Events API callbacks
- Parses URL-encoded form data instead of JSON
- Uses `/command` endpoint instead of `/slack`
- No event type configuration needed - publishes all commands to a single channel
- Converts command data to JSON before publishing to Redis
- No URL verification challenge handling (not needed for commands)

## License

MIT
