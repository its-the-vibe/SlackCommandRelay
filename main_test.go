package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// computeSignature builds a valid Slack v0 HMAC-SHA256 signature for tests.
func computeSignature(secret []byte, timestamp, body string) string {
	base := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// --- parseLogLevel ---

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"DEBUG", DEBUG},
		{"debug", DEBUG},
		{"INFO", INFO},
		{"info", INFO},
		{"WARN", WARN},
		{"warn", WARN},
		{"ERROR", ERROR},
		{"error", ERROR},
		{"", INFO},
		{"UNKNOWN", INFO},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// --- absInt64 ---

func TestAbsInt64(t *testing.T) {
	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-1, 1},
		{100, 100},
	}
	for _, tt := range tests {
		got := absInt64(tt.input)
		if got != tt.expected {
			t.Errorf("absInt64(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// --- verifySlackSignature ---

func TestVerifySlackSignature_NoSecret(t *testing.T) {
	// Empty secret bypasses verification
	if !verifySlackSignature(nil, []byte("body"), "", "") {
		t.Error("expected true when no secret is configured")
	}
}

func TestVerifySlackSignature_MissingHeaders(t *testing.T) {
	secret := []byte("test-secret")
	ts := fmt.Sprintf("%d", time.Now().Unix())

	if verifySlackSignature(secret, []byte("body"), ts, "") {
		t.Error("expected false when signature header is missing")
	}
	if verifySlackSignature(secret, []byte("body"), "", "v0=abc") {
		t.Error("expected false when timestamp header is missing")
	}
}

func TestVerifySlackSignature_InvalidTimestamp(t *testing.T) {
	secret := []byte("test-secret")
	if verifySlackSignature(secret, []byte("body"), "not-a-number", "v0=abc") {
		t.Error("expected false for non-numeric timestamp")
	}
}

func TestVerifySlackSignature_ReplayAttack(t *testing.T) {
	secret := []byte("test-secret")
	body := []byte("command=%2Ftest")
	// Timestamp more than 5 minutes in the past
	oldTs := fmt.Sprintf("%d", time.Now().Unix()-400)
	sig := computeSignature(secret, oldTs, string(body))

	if verifySlackSignature(secret, body, oldTs, sig) {
		t.Error("expected false for stale timestamp (replay attack)")
	}
}

func TestVerifySlackSignature_InvalidPrefix(t *testing.T) {
	secret := []byte("test-secret")
	ts := fmt.Sprintf("%d", time.Now().Unix())
	body := []byte("command=%2Ftest")
	if verifySlackSignature(secret, body, ts, "bad=abc123") {
		t.Error("expected false for signature without v0= prefix")
	}
}

func TestVerifySlackSignature_WrongSignature(t *testing.T) {
	secret := []byte("test-secret")
	ts := fmt.Sprintf("%d", time.Now().Unix())
	body := []byte("command=%2Ftest")
	if verifySlackSignature(secret, body, ts, "v0=deadbeef") {
		t.Error("expected false for incorrect HMAC signature")
	}
}

func TestVerifySlackSignature_Valid(t *testing.T) {
	secret := []byte("test-secret")
	ts := fmt.Sprintf("%d", time.Now().Unix())
	body := []byte("command=%2Ftest&text=hello")
	sig := computeSignature(secret, ts, string(body))

	if !verifySlackSignature(secret, body, ts, sig) {
		t.Error("expected true for a valid HMAC signature")
	}
}

// saveAndRestoreGlobals saves the current values of package-level test globals
// and registers a cleanup function to restore them after the test completes.
// This prevents test pollution when tests modify global state.
func saveAndRestoreGlobals(t *testing.T) {
	t.Helper()
	origSecret := signingSecret
	origClient := redisClient
	t.Cleanup(func() {
		signingSecret = origSecret
		redisClient = origClient
	})
}

// --- slackCommandHandler ---

func TestSlackCommandHandler_MethodNotAllowed(t *testing.T) {
	saveAndRestoreGlobals(t)
	req := httptest.NewRequest(http.MethodGet, "/command", nil)
	w := httptest.NewRecorder()

	signingSecret = nil // no secret
	slackCommandHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestSlackCommandHandler_NoSecretAcceptsRequest(t *testing.T) {
	saveAndRestoreGlobals(t)
	body := "command=%2Ftest&text=hello&user_name=alice&user_id=U1&team_id=T1&channel_id=C1"
	req := httptest.NewRequest(http.MethodPost, "/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	w := httptest.NewRecorder()

	signingSecret = nil // skip verification
	redisClient = nil   // no Redis
	slackCommandHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSlackCommandHandler_InvalidSignatureReturns401(t *testing.T) {
	saveAndRestoreGlobals(t)
	body := "command=%2Ftest&text=hello"
	req := httptest.NewRequest(http.MethodPost, "/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Slack-Signature", "v0=badhash")
	w := httptest.NewRecorder()

	signingSecret = []byte("real-secret")
	redisClient = nil
	slackCommandHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSlackCommandHandler_ValidSignature(t *testing.T) {
	saveAndRestoreGlobals(t)
	secret := []byte("my-signing-secret")
	bodyStr := "command=%2Ftest&text=hello&user_name=bob&user_id=U2&team_id=T2&channel_id=C2"
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := computeSignature(secret, ts, bodyStr)

	req := httptest.NewRequest(http.MethodPost, "/command", strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	w := httptest.NewRecorder()

	signingSecret = secret
	redisClient = nil
	slackCommandHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
