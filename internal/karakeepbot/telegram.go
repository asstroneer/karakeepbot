package karakeepbot

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Madh93/karakeepbot/internal/config"
	"github.com/Madh93/karakeepbot/internal/logging"
	"github.com/Madh93/karakeepbot/internal/secret"
	tgbotapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const ModeWebhook = "webhook"
const ModeLongPolling = "long_polling"

// Bot is an alias for tgbotapi.Bot.
type Bot = tgbotapi.Bot

// Telegram embeds the Telegram bot API client to add high level functionality.
type Telegram struct {
	*Bot
	token         secret.String
	mode          string
	listenAddr    string
	webhookURL    string
	webhookPath   string
	webhookSecret string
	logger        *logging.Logger
}

// createTelegram initializes the Telegram Bot API client.
func createTelegram(logger *logging.Logger, config *config.TelegramConfig) *Telegram {
	logger.Debug(fmt.Sprintf("Initializing Telegram Bot API using %s token", config.Token))

	var opts []tgbotapi.Option
	mode := config.Mode
	if mode == "" {
		// Auto-detect webhook mode if all webhook fields are set
		if config.WebhookURL != "" && config.WebhookSecretToken != "" && config.ListenAddr != "" {
			mode = ModeWebhook
		} else {
			mode = ModeLongPolling
		}
	}
	if mode == ModeWebhook {
		logger.Debug(fmt.Sprintf("Telegram Bot init in webhook mode with url: %s and %s", config.WebhookURL, config.WebhookSecretToken))
		opts = []tgbotapi.Option{
			tgbotapi.WithWebhookSecretToken(config.WebhookSecretToken),
			tgbotapi.WithServerURL(config.WebhookURL),
		}
	}

	telegramBot, err := tgbotapi.New(config.Token.Value(), opts...)
	if err != nil {
		logger.Fatal("Error creating Telegram Bot API.", "error", err)
	}

	logger.Info(fmt.Sprintf("Telegram Bot API initialised in %s mode", mode))
	return &Telegram{
		Bot:           telegramBot,
		token:         config.Token,
		mode:          mode,
		listenAddr:    config.ListenAddr,
		webhookURL:    config.WebhookURL,
		webhookPath:   config.WebhookPath,
		webhookSecret: config.WebhookSecretToken,
		logger:        logger,
	}
}

// SendNewMessage sends a new message to the user's chat.
func (t Telegram) SendNewMessage(ctx context.Context, msg *TelegramMessage) error {
	params := &tgbotapi.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            msg.Text,
	}

	if _, err := t.SendMessage(ctx, params); err != nil {
		return err
	}

	return nil
}

func (t *Telegram) GetMode() string {
	return t.mode
}

// RegisterWebhook registers the webhook with Telegram API.
func (t *Telegram) RegisterWebhook(ctx context.Context) error {
	// Build full webhook URL (base URL + path)
	fullURL := t.webhookURL
	if t.webhookPath != "" {
		fullURL = strings.TrimSuffix(t.webhookURL, "/") + "/" + strings.TrimPrefix(t.webhookPath, "/")
	}

	// Call Telegram setWebhook API
	params := &tgbotapi.SetWebhookParams{
		URL:         fullURL,
		SecretToken: t.webhookSecret,
	}

	result, err := t.SetWebhook(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	if !result {
		return fmt.Errorf("webhook registration returned false")
	}

	t.logger.Info("Webhook registered successfully", "url", fullURL)
	return nil
}

// DeleteWebhook deletes the webhook from Telegram API.
func (t *Telegram) DeleteWebhook(ctx context.Context) error {
	t.logger.Info("Deleting webhook from Telegram...")

	result, err := t.Bot.DeleteWebhook(ctx, &tgbotapi.DeleteWebhookParams{
		DropPendingUpdates: false,
	})
	if err != nil {
		t.logger.Error("Failed to delete webhook", "error", err)
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	if !result {
		t.logger.Warn("Webhook deletion returned false")
	} else {
		t.logger.Info("Webhook deleted successfully")
	}

	return nil
}

// healthHandler handles health check endpoints.
type healthHandler struct {
	ready atomic.Bool
}

// ServeHTTP handles the /health endpoint - always returns 200.
func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// readyHandler handles the /ready endpoint - returns 200 if ready, 503 if not.
func (h *healthHandler) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.ready.Load() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not ready"}`))
	}
}

// StartAndServeWebhook starts the webhook server with graceful shutdown.
func (t *Telegram) StartAndServeWebhook(ctx context.Context) error {
	health := &healthHandler{}

	// Register webhook with Telegram
	if err := t.RegisterWebhook(ctx); err != nil {
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	// Mark as ready
	health.ready.Store(true)

	// Determine webhook path
	webhookPath := t.webhookPath
	if webhookPath == "" {
		webhookPath = "/telegram-webhook"
	}

	// Create HTTP server mux
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health.ServeHTTP)
	mux.HandleFunc("/ready", health.readyHandler)
	mux.Handle(webhookPath, t.WebhookHandler())

	// Create HTTP server
	server := &http.Server{
		Addr:    t.listenAddr,
		Handler: mux,
	}

	// Channel for server errors
	errCh := make(chan error, 1)

	// Start server in goroutine
	go func() {
		t.logger.Info("Starting webhook HTTP server", "addr", t.listenAddr, "path", webhookPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		t.logger.Info("Shutting down webhook server...")

		// Stop accepting new traffic
		health.ready.Store(false)

		// Graceful shutdown with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			t.logger.Error("Error shutting down server", "error", err)
		}

		// Delete webhook from Telegram (ignore errors, just log)
		if err := t.DeleteWebhook(context.Background()); err != nil {
			t.logger.Error("Error deleting webhook", "error", err)
		}

		return nil

	case err := <-errCh:
		return fmt.Errorf("webhook server error: %w", err)
	}
}

// StartAndServerWebHook is kept for backward compatibility - calls StartAndServeWebhook.
func (t *Telegram) StartAndServerWebHook(ctx context.Context) error {
	go t.StartWebhook(ctx)
	return http.ListenAndServe(t.listenAddr, t.WebhookHandler())
}

// SendPhotoWithCaption sends a photo with a caption.
func (t *Telegram) SendPhotoWithCaption(ctx context.Context, msg *TelegramMessage, photoID string, caption string) error {
	params := &tgbotapi.SendPhotoParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Photo:           &models.InputFileString{Data: photoID},
		Caption:         caption,
	}

	if _, err := t.SendPhoto(ctx, params); err != nil {
		return err
	}

	return nil
}

// SendReply sends a reply to a specific message.
func (t Telegram) SendReply(ctx context.Context, msg *TelegramMessage, text string) error {
	params := &tgbotapi.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		Text:            text,
	}

	if _, err := t.SendMessage(ctx, params); err != nil {
		return err
	}

	return nil
}

// DeleteOriginalMessage deletes the original message from the user's chat.
func (t Telegram) DeleteOriginalMessage(ctx context.Context, msg *TelegramMessage) error {
	params := &tgbotapi.DeleteMessageParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
	}

	if _, err := t.DeleteMessage(ctx, params); err != nil {
		return err
	}

	return nil
}

// GetFileURL returns the download URL for a given file ID.
func (t Telegram) GetFileURL(ctx context.Context, fileID string) (string, error) {
	file, err := t.GetFile(ctx, &tgbotapi.GetFileParams{FileID: fileID})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", t.token.Value(), file.FilePath), nil
}
