# CLAUDE.md

## Project Overview

**karakeepbot** is a Telegram bot written in Go that allows users to save bookmarks (text, URLs, images) directly to [Karakeep](https://karakeep.app/) (a self-hostable bookmark manager). It supports AI-generated tags displayed as Telegram hashtags.

## Build & Run Commands

All tasks are managed via [Taskfile](https://taskfile.dev/):

```sh
task build              # Build binary to bin/karakeepbot
task run                # Run the installed binary
task install            # Install binary to $GOPATH/bin
task test               # Run all tests: go test ./...
task lint               # Run golangci-lint
task clean              # Remove bin/ and Go cache
task update-dependencies # go get -u all && go mod tidy
task build:docker       # Build Docker image
```

Direct Go commands:
```sh
go test ./...           # Run tests
go build -trimpath -ldflags "-s -w" -o bin/karakeepbot .
```

## Project Structure

```
karakeepbot/
├── main.go                    # Entry point: loads config, initializes bot
├── config.default.toml        # Default configuration (TOML + env overrides)
├── Dockerfile                 # Multi-stage Docker build (golang → scratch)
├── Taskfile.yml               # Task automation
├── go.mod / go.sum            # Go 1.24 module
├── kodata/                    # Ko build data (symlink to config)
├── docs/                      # Documentation assets
└── internal/
    ├── config/                # Config loading & validation (koanf)
    ├── karakeepbot/           # Core bot logic & message handling
    ├── fileprocessor/         # File download, validation, temp storage
    ├── filevalidator/         # Image format validation (JPEG, PNG, WebP)
    ├── validation/            # Input validators (tokens, URLs)
    ├── logging/               # Structured logging (slog + tint for colors)
    ├── secret/                # Secret string masking (shows first 4 chars)
    └── version/               # Version info injected via ldflags
```

## Key Architecture

### Bot Message Flow
1. Validate chat ID (and optional thread ID) against allowlist — reject unauthorized chats
2. Parse message type: photo → `AssetBookmark`, URL → `LinkBookmark`, text → `TextBookmark`
3. Create bookmark via Karakeep API
4. Poll for AI-generated tags at configurable interval
5. Reply with hashtag-formatted tags, delete original message

### Telegram Modes
- **long_polling** (default): periodic polling for updates
- **webhook**: HTTP server listens for updates; requires `webhook_url`, `webhook_path`, optional `webhook_secret_token`, `listen_addr`

### Bookmark Types (`internal/karakeepbot/`)
- `BookmarkType` — interface with `Create(ctx, karakeep)`, `String()`, `Attrs()`, `AttrsWithError(err)`
- `LinkBookmark` — URL bookmark
- `TextBookmark` — plain text bookmark
- `AssetBookmark` — image bookmark with title and note fields (Karakeep ≥ 0.27.1)

### Configuration (`internal/config/`)
- Loaded from `config.default.toml` then overridden by env vars prefixed `KARAKEEPBOT_`
- Validated at startup (fail-fast): token formats, URL, non-empty allowlist
- Key sections: `[telegram]`, `[karakeep]`, `[logging]`, `[fileprocessor]`

### Secret Handling (`internal/secret/`)
- `secret.String` type — never logs full value; only first 4 chars + `****`

## Configuration Reference

| Section | Key | Default | Notes |
|---|---|---|---|
| telegram | token | — | Required. BotFather token |
| telegram | allowlist | [-1] | Required. Must set real chat IDs |
| telegram | threads | [] | Optional topic/thread filter |
| telegram | mode | long_polling | or `webhook` |
| telegram | webhook_url | — | Required in webhook mode |
| telegram | listen_addr | :8080 | Webhook listen address |
| karakeep | url | http://localhost:3000 | Karakeep base URL |
| karakeep | token | — | Required. API key (`ak1_*` or `ak2_*`) |
| karakeep | interval | 5s | Tag polling interval |
| logging | level | info | debug/info/warn/error/panic |
| logging | format | text | text or json |
| fileprocessor | maxsize | 10485760 | 10 MB max file size |
| fileprocessor | mimetypes | image/jpeg,png,webp | Allowed MIME types |

## Testing Conventions

- Standard `testing` package, table-driven tests
- Test files: `*_test.go` alongside source files
- Run all: `task test` or `go test ./...`
- No external test framework — pure stdlib

## Code Conventions

- **Error handling**: wrap with `fmt.Errorf("context: %w", err)`; custom error types in `fileprocessor` (`ErrValidationFailed`, `ErrDownloadFailed`, `ErrProcessingFailed`)
- **Logging**: structured slog with key-value pairs; use `msg.Attrs()` / `msg.AttrsWithError(err)` on messages
- **Dependency injection**: all major structs accept logger + config in `New()` constructors
- **Secrets**: always use `secret.String` for tokens, never plain `string`
- **Security**: allowlist is mandatory; file size/MIME validation enforced; webhook secret token supported

## Key Dependencies

| Package | Purpose |
|---|---|
| `github.com/Madh93/go-karakeep` | Generated Karakeep API client |
| `github.com/go-telegram/bot` | Telegram Bot API wrapper |
| `github.com/knadh/koanf` | Config management (TOML + env) |
| `github.com/lmittmann/tint` | Colored slog text output |

## Docker

Multi-stage build using `golang:1.24` → `scratch`. Built with `CGO_ENABLED=0`.
Config file (`config.default.toml`) and CA certs are included in the image.
Use `ko` for production image builds (`task build:docker`).
