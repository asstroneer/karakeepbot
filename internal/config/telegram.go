package config

import (
	"fmt"

	"github.com/Madh93/karakeepbot/internal/secret"
	"github.com/Madh93/karakeepbot/internal/validation"
)

// TelegramConfig represents a configuration for Telegram.
type TelegramConfig struct {
	Token     secret.String `koanf:"token"`     // Telegram bot token.
	Allowlist []int64       `koanf:"allowlist"` // Allowed chat IDs for the bot to interact with.
	Threads   []int         `koanf:"threads"`   // Allowed thread IDs (a.k.a topics) for the bot to interact with.

	// TODO add validation for this fields
	WebHookUrl         string `koanf:"webhook_url"`          // Set webhook url which will be reached for updates. Example: https://example.com/bot
	WebHookSecretToken string `koanf:"webhook_secret_token"` // Secret token for webhook for validate telegram webhook requests. Example: mysecrettoken
	ListenAddr         string `koanf:"listen_addr"`          // Listen address of server to listen for incoming updates. Example: 127.0.0.1:3000
}

// Validate checks if the Telegram configuration is valid.
func (c TelegramConfig) Validate() error {
	if err := validation.ValidateTelegramToken(c.Token); err != nil {
		return err
	}

	if len(c.Allowlist) == 1 && c.Allowlist[0] == -1 {
		return fmt.Errorf("invalid Telegram Allowlist (-1). Please configure it with your actual chat ID or an empty list to allow all users (not recommended)")
	}

	return nil
}
