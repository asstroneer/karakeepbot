package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/Madh93/karakeepbot/internal/secret"
	"github.com/Madh93/karakeepbot/internal/validation"
)

// Telegram bot modes.
const (
	ModeWebhook     = "webhook"
	ModeLongPolling = "long_polling"
)

// TelegramConfig represents a configuration for Telegram.
type TelegramConfig struct {
	Token     secret.String `koanf:"token"`     // Telegram bot token.
	Allowlist []int64       `koanf:"allowlist"` // Allowed chat IDs for the bot to interact with.
	Threads   []int         `koanf:"threads"`   // Allowed thread IDs (a.k.a topics) for the bot to interact with.

	// Mode: "webhook" or "long_polling"
	Mode string `koanf:"mode"` // Bot mode: webhook or long_polling (default)

	// WebhookURL is the external URL where the bot will receive webhook updates.
	// Example: https://example.com/bot
	WebhookURL string `koanf:"webhook_url"`

	// WebhookPath is the path suffix for the webhook endpoint.
	// Default: "/telegram-webhook"
	WebhookPath string `koanf:"webhook_path"`

	// WebhookSecretToken is the secret token for validating Telegram webhook requests.
	// Example: mysecrettoken
	WebhookSecretToken string `koanf:"webhook_secret_token"`

	// ListenAddr is the address where the HTTP server will listen for incoming updates.
	// Example: 127.0.0.1:3000
	ListenAddr string `koanf:"listen_addr"`
}

// Validate checks if the Telegram configuration is valid.
func (c TelegramConfig) Validate() error {
	if err := validation.ValidateTelegramToken(c.Token); err != nil {
		return err
	}

	if len(c.Allowlist) == 1 && c.Allowlist[0] == -1 {
		return fmt.Errorf("invalid Telegram Allowlist (-1). Please configure it with your actual chat ID or an empty list to allow all users (not recommended)")
	}

	// Validate mode
	if c.Mode != "" && c.Mode != ModeWebhook && c.Mode != ModeLongPolling {
		return fmt.Errorf("invalid Telegram mode (%q). Must be either %q or %q", c.Mode, ModeWebhook, ModeLongPolling)
	}

	// Determine if webhook mode is enabled
	isWebhookMode := c.Mode == ModeWebhook || (c.Mode == "" && c.WebhookURL != "" && c.ListenAddr != "")

	if isWebhookMode {
		// Validate WebhookURL
		if c.WebhookURL == "" {
			return fmt.Errorf("webhook_url is required when using webhook mode")
		}
		if !isValidURL(c.WebhookURL) {
			return fmt.Errorf("webhook_url must be a valid HTTPS URL")
		}

		// Validate ListenAddr
		if c.ListenAddr == "" {
			return fmt.Errorf("listen_addr is required when using webhook mode")
		}
		if !isValidListenAddr(c.ListenAddr) {
			return fmt.Errorf("listen_addr must be in format host:port or :port")
		}

		// Warn if WebhookSecretToken is empty (not an error)
		if c.WebhookSecretToken == "" {
			fmt.Println("WARNING: webhook_secret_token is not set. It is recommended to set a secret token for security.")
		}
	}

	return nil
}

// isValidURL checks if the given string is a valid HTTPS URL.
func isValidURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return u.Scheme == "https"
}

// isValidListenAddr checks if the given string is a valid listen address (host:port or :port).
func isValidListenAddr(s string) bool {
	_, _, err := net.SplitHostPort(s)
	return err == nil && strings.Contains(s, ":")
}
