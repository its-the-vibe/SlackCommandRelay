package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

const (
	// slackTimestampToleranceSeconds is the maximum age of a Slack request timestamp
	// Slack recommends rejecting requests older than 5 minutes to prevent replay attacks
	slackTimestampToleranceSeconds = 300
)

// SlackCommand represents a parsed Slack command request
type SlackCommand struct {
	Token          string `json:"token"`
	TeamID         string `json:"team_id"`
	TeamDomain     string `json:"team_domain"`
	ChannelID      string `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name"`
	Command        string `json:"command"`
	Text           string `json:"text"`
	ResponseURL    string `json:"response_url"`
	TriggerID      string `json:"trigger_id"`
	APIAppID       string `json:"api_app_id"`
	EnterpriseID   string `json:"enterprise_id,omitempty"`
	EnterpriseName string `json:"enterprise_name,omitempty"`
}

var signingSecret []byte
var redisClient *redis.Client
var currentLogLevel LogLevel = INFO
var redisChannel string

// parseLogLevel converts a string to LogLevel
func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// logDebug logs a message at DEBUG level
func logDebug(format string, v ...interface{}) {
	if currentLogLevel <= DEBUG {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// logInfo logs a message at INFO level
func logInfo(format string, v ...interface{}) {
	if currentLogLevel <= INFO {
		log.Printf("[INFO] "+format, v...)
	}
}

// logWarn logs a message at WARN level
func logWarn(format string, v ...interface{}) {
	if currentLogLevel <= WARN {
		log.Printf("[WARN] "+format, v...)
	}
}

// logError logs a message at ERROR level
func logError(format string, v ...interface{}) {
	if currentLogLevel <= ERROR {
		log.Printf("[ERROR] "+format, v...)
	}
}

func verifySlackSignature(body []byte, timestamp string, signature string) bool {
	if len(signingSecret) == 0 {
		// No secret configured, skip verification
		return true
	}

	if signature == "" || timestamp == "" {
		return false
	}

	// Check timestamp to prevent replay attacks (should be within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	if absInt64(now-ts) > slackTimestampToleranceSeconds {
		logWarn("Request timestamp too old or too far in the future")
		return false
	}

	// Slack sends signature as "v0=<hash>"
	if !strings.HasPrefix(signature, "v0=") {
		return false
	}

	signatureHash := strings.TrimPrefix(signature, "v0=")

	// Compute expected signature: v0:<timestamp>:<body>
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write([]byte(baseString))
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signatureHash), []byte(expectedSignature))
}

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func slackCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	// Verify Slack request signature
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	if !verifySlackSignature(body, timestamp, signature) {
		logWarn("Invalid Slack signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse URL-encoded form data from Slack command
	values, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	// Convert to SlackCommand struct
	command := SlackCommand{
		Token:          values.Get("token"),
		TeamID:         values.Get("team_id"),
		TeamDomain:     values.Get("team_domain"),
		ChannelID:      values.Get("channel_id"),
		ChannelName:    values.Get("channel_name"),
		UserID:         values.Get("user_id"),
		UserName:       values.Get("user_name"),
		Command:        values.Get("command"),
		Text:           values.Get("text"),
		ResponseURL:    values.Get("response_url"),
		TriggerID:      values.Get("trigger_id"),
		APIAppID:       values.Get("api_app_id"),
		EnterpriseID:   values.Get("enterprise_id"),
		EnterpriseName: values.Get("enterprise_name"),
	}

	logInfo("Received Slack command: %s from user %s", command.Command, command.UserName)

	// Only log payload at DEBUG level
	if currentLogLevel <= DEBUG {
		jsonOutput, err := json.MarshalIndent(command, "", "  ")
		if err != nil {
			logError("Error formatting JSON: %v", err)
			logDebug("Raw payload: %s", string(body))
		} else {
			logDebug("Slack command payload:\n%s", string(jsonOutput))
		}
	}

	// Publish to Redis if client is configured
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Convert command to JSON for publishing
		jsonPayload, err := json.Marshal(command)
		if err != nil {
			logError("Error marshaling command to JSON: %v", err)
		} else {
			err = redisClient.Publish(ctx, redisChannel, jsonPayload).Err()
			if err != nil {
				logError("Error publishing to Redis channel '%s': %v", redisChannel, err)
				// Don't fail the request if Redis publish fails
			} else {
				logInfo("Published command to Redis channel: %s", redisChannel)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(fmt.Sprintf("Slash command `%s` received 🎉", command.Command))); err != nil {
		logError("Error writing response: %v", err)
	}
}

func main() {
	// Set log level from environment variable
	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" {
		logLevelStr = "INFO"
	}
	currentLogLevel = parseLogLevel(logLevelStr)
	logInfo("Log level set to: %s", strings.ToUpper(logLevelStr))

	// Get Redis channel name from environment variable
	redisChannel = os.Getenv("REDIS_CHANNEL")
	if redisChannel == "" {
		redisChannel = "slack-commands"
	}
	logInfo("Redis channel set to: %s", redisChannel)

	// Load Slack signing secret from .secret file
	secretData, err := os.ReadFile(".secret")
	if err != nil {
		logWarn(".secret file not found. Slack signature verification will be skipped.")
		logWarn("To enable verification, create a .secret file with your Slack signing secret.")
	} else {
		signingSecret = []byte(strings.TrimSpace(string(secretData)))
		logInfo("Slack signing secret loaded. Signature verification enabled.")
	}

	// Configure Redis connection
	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")

	// Set defaults
	if redisHost == "" {
		redisHost = "localhost"
	}
	if redisPort == "" {
		redisPort = "6379"
	}

	// Initialize Redis client
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)
	redisClient = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Test Redis connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = redisClient.Ping(ctx).Result()
	if err != nil {
		logWarn("Could not connect to Redis at %s: %v", redisAddr, err)
		logWarn("Redis publishing will be disabled. Service will continue to work without Redis.")
		redisClient = nil
	} else {
		logInfo("Connected to Redis at %s", redisAddr)
	}

	http.HandleFunc("/command", slackCommandHandler)

	// Get port from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Ensure port has colon prefix
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	logInfo("Starting Slack command server on port %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
