package karakeepbot

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Madh93/karakeepbot/internal/config"
	"github.com/Madh93/karakeepbot/internal/logging"
)

// mockTelegramServer creates a mock Telegram API server for testing.
type mockTelegramServer struct {
	*httptest.Server
	mu                 sync.Mutex
	setWebhookCalls    int
	deleteWebhookCalls int
	webhookURL         string
}

func newMockTelegramServer() *mockTelegramServer {
	m := &mockTelegramServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handleRequest))
	return m
}

func (m *mockTelegramServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// The bot library calls /bot{token}/method
	// Extract the method from the path (e.g., /bot1234567890:getMe -> getMe)
	path := r.URL.Path
	var method string
	if len(path) > 0 {
		// Find the last slash and get everything after it
		lastSlash := -1
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				lastSlash = i
				break
			}
		}
		if lastSlash >= 0 && lastSlash < len(path)-1 {
			method = path[lastSlash+1:]
		}
	}

	switch method {
	case "getMe":
		// Return mock bot info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"id":                          1234567890,
				"is_bot":                      true,
				"first_name":                  "TestBot",
				"username":                    "testbot",
				"can_join_groups":             true,
				"can_read_all_group_messages": false,
				"supports_inline_queries":     false,
			},
		})

	case "setWebhook":
		m.setWebhookCalls++
		// Return success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          true,
			"result":      true,
			"description": "Webhook was set",
		})

	case "deleteWebhook":
		m.deleteWebhookCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          true,
			"result":      true,
			"description": "Webhook was deleted",
		})

	case "getWebhookInfo":
		// Return mock webhook info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"url":                    m.webhookURL,
				"has_custom_certificate": false,
				"pending_update_count":   0,
			},
		})

	default:
		http.NotFound(w, r)
	}
}

func (m *mockTelegramServer) getSetWebhookCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setWebhookCalls
}

func (m *mockTelegramServer) getDeleteWebhookCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteWebhookCalls
}

// getFreePort finds a free port on localhost.
func getFreePort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

// createTestLogger creates a logger for testing that outputs to os.Stderr.
func createTestLogger() *logging.Logger {
	loggingConfig := &config.LoggingConfig{
		Level:   "error", // Use error level to minimize output during tests
		Format:  "text",
		Output:  "stderr",
		Colored: false,
	}
	return logging.New(loggingConfig)
}

// TestHealthHandler tests the health handler endpoints.
func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		ready          bool
		expectedBody   string
	}{
		{
			name:           "health endpoint returns 200",
			path:           "/health",
			expectedStatus: http.StatusOK,
			ready:          false,
			expectedBody:   `{"status":"ok"}`,
		},
		{
			name:           "ready endpoint returns 200 when ready",
			path:           "/ready",
			expectedStatus: http.StatusOK,
			ready:          true,
			expectedBody:   `{"status":"ready"}`,
		},
		{
			name:           "ready endpoint returns 503 when not ready",
			path:           "/ready",
			expectedStatus: http.StatusServiceUnavailable,
			ready:          false,
			expectedBody:   `{"status":"not ready"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &healthHandler{}
			handler.ready.Store(tt.ready)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			if tt.path == "/health" {
				handler.ServeHTTP(w, req)
			} else {
				handler.readyHandler(w, req)
			}

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestWebhookRegistration tests the webhook registration process.
func TestWebhookRegistration(t *testing.T) {
	// Skip if no network available
	if !hasNetwork() {
		t.Skip("Skipping test: no network available")
	}

	// Create mock Telegram server
	mockServer := newMockTelegramServer()
	defer mockServer.Close()

	// Create a logger
	logger := createTestLogger()

	// Get a free port for the test
	addr, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	// Create TelegramConfig
	telegramConfig := config.TelegramConfig{
		Token:              "1234567890:mock_token",
		Allowlist:          []int64{123456789},
		Mode:               ModeWebhook,
		WebhookURL:         mockServer.URL,
		WebhookPath:        "/telegram-webhook",
		WebhookSecretToken: "test-secret",
		ListenAddr:         addr,
	}

	// Create Telegram instance with webhook configuration
	telegram := createTelegram(logger, &telegramConfig)

	// Override the webhook URL to use our mock server
	telegram.webhookURL = mockServer.URL

	// Test webhook registration
	ctx := context.Background()
	err = telegram.RegisterWebhook(ctx)

	if err != nil {
		t.Errorf("RegisterWebhook failed: %v", err)
	}

	// Verify webhook was registered
	if mockServer.getSetWebhookCalls() != 1 {
		t.Errorf("expected 1 setWebhook call, got %d", mockServer.getSetWebhookCalls())
	}
}

// TestWebhookDeletion tests the webhook deletion process.
func TestWebhookDeletion(t *testing.T) {
	// Skip if no network available
	if !hasNetwork() {
		t.Skip("Skipping test: no network available")
	}

	// Create mock Telegram server
	mockServer := newMockTelegramServer()
	defer mockServer.Close()

	logger := createTestLogger()

	addr, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	telegramConfig := config.TelegramConfig{
		Token:              "1234567890:mock_token",
		Allowlist:          []int64{123456789},
		Mode:               ModeWebhook,
		WebhookURL:         mockServer.URL,
		WebhookPath:        "/telegram-webhook",
		WebhookSecretToken: "test-secret",
		ListenAddr:         addr,
	}

	telegram := createTelegram(logger, &telegramConfig)
	telegram.webhookURL = mockServer.URL

	// Test webhook deletion
	ctx := context.Background()
	err = telegram.DeleteWebhook(ctx)

	if err != nil {
		t.Errorf("DeleteWebhook failed: %v", err)
	}

	// Verify webhook was deleted
	if mockServer.getDeleteWebhookCalls() != 1 {
		t.Errorf("expected 1 deleteWebhook call, got %d", mockServer.getDeleteWebhookCalls())
	}
}

// TestGracefulShutdown tests the graceful shutdown flow.
func TestGracefulShutdown(t *testing.T) {
	// Skip if no network available
	if !hasNetwork() {
		t.Skip("Skipping test: no network available")
	}

	// Create mock Telegram server
	mockServer := newMockTelegramServer()
	defer mockServer.Close()

	logger := createTestLogger()

	addr, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	telegramConfig := config.TelegramConfig{
		Token:              "1234567890:mock_token",
		Allowlist:          []int64{123456789},
		Mode:               ModeWebhook,
		WebhookURL:         mockServer.URL,
		WebhookPath:        "/test-webhook",
		WebhookSecretToken: "test-secret",
		ListenAddr:         addr,
	}

	telegram := createTelegram(logger, &telegramConfig)
	telegram.webhookURL = mockServer.URL

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start webhook server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- telegram.StartAndServeWebhook(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to shutdown
	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Server returned error (expected on clean shutdown): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shutdown within timeout")
	}

	// Verify webhook was deleted on shutdown
	if mockServer.getDeleteWebhookCalls() != 1 {
		t.Errorf("expected 1 deleteWebhook call on shutdown, got %d", mockServer.getDeleteWebhookCalls())
	}
}

// TestHealthEndpointsHTTP tests the /health and /ready endpoints via HTTP.
func TestHealthEndpointsHTTP(t *testing.T) {
	// Skip if no network available
	if !hasNetwork() {
		t.Skip("Skipping test: no network available")
	}

	// Create mock Telegram server
	mockServer := newMockTelegramServer()
	defer mockServer.Close()

	logger := createTestLogger()

	addr, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	telegramConfig := config.TelegramConfig{
		Token:              "1234567890:mock_token",
		Allowlist:          []int64{123456789},
		Mode:               ModeWebhook,
		WebhookURL:         mockServer.URL,
		WebhookPath:        "/test-webhook",
		WebhookSecretToken: "test-secret",
		ListenAddr:         addr,
	}

	telegram := createTelegram(logger, &telegramConfig)
	telegram.webhookURL = mockServer.URL

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start webhook server in goroutine
	go func() {
		telegram.StartAndServeWebhook(ctx)
	}()

	// Give server time to start and register webhook
	time.Sleep(200 * time.Millisecond)

	// Test /health endpoint
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("Failed to GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected /health to return 200, got %d", resp.StatusCode)
	}

	// Test /ready endpoint (should be ready after webhook registration)
	resp, err = http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("Failed to GET /ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected /ready to return 200 when ready, got %d", resp.StatusCode)
	}
}

// hasNetwork checks if network is available for testing.
func hasNetwork() bool {
	// Try to connect to a known host
	conn, err := net.Dial("tcp", "8.8.8.8:53")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Ensure we implement the Logger interface - compile-time check
var _ *logging.Logger
