package config

import (
	"strings"
	"testing"

	"github.com/Madh93/karakeepbot/internal/secret"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"valid HTTPS URL", "https://example.com", true},
		{"valid HTTPS URL with path", "https://example.com/webhook", true},
		{"valid HTTPS URL with port", "https://example.com:8080", true},
		{"invalid HTTP URL", "http://example.com", false},
		{"invalid URL without scheme", "example.com", false},
		{"invalid empty URL", "", false},
		{"invalid URL with spaces", "https://example .com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsValidListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{"valid with port and colon prefix", ":8080", true},
		{"valid with host and port", "0.0.0.0:8080", true},
		{"valid with localhost and port", "localhost:3000", true},
		{"valid with IP and port", "127.0.0.1:8080", true},
		{"invalid without port", "localhost", false},
		{"invalid empty string", "", false},
		{"invalid just number", "8080", false},
		{"invalid with extra colon", ":::8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidListenAddr(tt.addr)
			if result != tt.expected {
				t.Errorf("isValidListenAddr(%q) = %v, want %v", tt.addr, result, tt.expected)
			}
		})
	}
}

func TestTelegramConfig_Validate_WebhookMode(t *testing.T) {
	tests := []struct {
		name      string
		config    TelegramConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid webhook mode with all fields",
			config: TelegramConfig{
				Token:              secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:          []int64{123456789},
				Mode:               ModeWebhook,
				WebhookURL:         "https://example.com",
				WebhookPath:        "/telegram-webhook",
				WebhookSecretToken: "secret-token",
				ListenAddr:         ":8080",
			},
			expectErr: false,
		},
		{
			name: "webhook mode missing webhook_url",
			config: TelegramConfig{
				Token:      secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:  []int64{123456789},
				Mode:       ModeWebhook,
				ListenAddr: ":8080",
			},
			expectErr: true,
			errMsg:    "webhook_url is required when using webhook mode",
		},
		{
			name: "webhook mode missing listen_addr",
			config: TelegramConfig{
				Token:      secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:  []int64{123456789},
				Mode:       ModeWebhook,
				WebhookURL: "https://example.com",
			},
			expectErr: true,
			errMsg:    "listen_addr is required when using webhook mode",
		},
		{
			name: "webhook mode with invalid webhook_url",
			config: TelegramConfig{
				Token:      secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:  []int64{123456789},
				Mode:       ModeWebhook,
				WebhookURL: "http://example.com", // HTTP instead of HTTPS
				ListenAddr: ":8080",
			},
			expectErr: true,
			errMsg:    "webhook_url must be a valid HTTPS URL",
		},
		{
			name: "webhook mode with invalid listen_addr",
			config: TelegramConfig{
				Token:      secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:  []int64{123456789},
				Mode:       ModeWebhook,
				WebhookURL: "https://example.com",
				ListenAddr: "invalid",
			},
			expectErr: true,
			errMsg:    "listen_addr must be in format host:port or :port",
		},
		{
			name: "invalid mode",
			config: TelegramConfig{
				Token:     secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist: []int64{123456789},
				Mode:      "invalid_mode",
			},
			expectErr: true,
			errMsg:    "invalid Telegram mode",
		},
		{
			name: "auto-detect webhook mode with all fields",
			config: TelegramConfig{
				Token:              secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist:          []int64{123456789},
				WebhookURL:         "https://example.com",
				WebhookSecretToken: "secret-token",
				ListenAddr:         ":8080",
			},
			expectErr: false,
		},
		{
			name: "long polling mode (default) - no webhook validation",
			config: TelegramConfig{
				Token:     secret.String("123456789:ABCdefGhIjKlmNOPQRstUVWXYZ123456789"),
				Allowlist: []int64{123456789},
				Mode:      ModeLongPolling,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr && err == nil {
				t.Errorf("expected error containing %q, but got nil", tt.errMsg)
			}
			if tt.expectErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTelegramConfig_ModeConstants(t *testing.T) {
	if ModeWebhook != "webhook" {
		t.Errorf("ModeWebhook = %q, want %q", ModeWebhook, "webhook")
	}
	if ModeLongPolling != "long_polling" {
		t.Errorf("ModeLongPolling = %q, want %q", ModeLongPolling, "long_polling")
	}
}
