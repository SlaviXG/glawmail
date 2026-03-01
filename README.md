# GlawMail - Gmail Sender Bot

Human-in-the-loop email sending via Telegram. Forward a properly formatted message to the bot, and it sends the email via Gmail.

## How It Works

```
┌─────────────────────┐                    ┌─────────────────────┐
│  AI / Machine A     │                    │     Machine B       │
│  (Email Generator)  │                    │   (Gmail Sender)    │
├─────────────────────┤                    ├─────────────────────┤
│                     │                    │                     │
│  Generates email    │                    │  Receives message   │
│  Shows preview      │    Owner forwards  │  Validates HMAC     │
│  Creates GLAWMAIL   ├───────────────────►│  Sends via Gmail    │
│  message            │                    │  Reports success    │
│                     │                    │                     │
└─────────────────────┘                    └─────────────────────┘
        (optional)                              (required)
```

**Machine A is optional.** Any AI with the right prompt/skill can generate `GLAWMAIL_SEND` messages directly.

## Message Format

```
GLAWMAIL_SEND:<hmac>:<json>
```

Where `<json>` contains:
```json
{
  "to": "recipient@example.com",
  "subject": "Email subject",
  "body": "Email body text",
  "html": false,
  "metadata": { "optional": "data" }
}
```

The HMAC is SHA256 of the JSON payload, using the shared `WEBHOOK_SECRET`.

## Quick Start

### Prerequisites

- Go 1.22+
- A Telegram bot (create via [@BotFather](https://t.me/BotFather))
- Gmail API credentials

### Installation

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go mod download
```

### Setup Gmail Sender (Machine B)

```bash
go run ./setup --machine b
```

This will prompt for:
1. Your bot token
2. Your Telegram chat ID
3. A shared HMAC secret (generate or paste)
4. Gmail OAuth authorization

### Run

```bash
go run ./cmd/machine_b
```

### Test

If you also set up Machine A (optional):

```bash
go run ./setup --machine a
go run ./cmd/machine_a test
```

This shows an email preview and generates a `GLAWMAIL_SEND` message. Forward it to Machine B's bot to send.

## Building

```bash
# Build for current platform
go build -o glawmail-sender ./cmd/machine_b
go build -o glawmail-preview ./cmd/machine_a
go build -o glawmail-setup ./setup

# Cross-compile for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o glawmail-sender-pi ./cmd/machine_b
```

## AI Integration

Any AI can generate emails for this system. Example prompt/skill:

```
When the user asks you to send an email, format it as:

GLAWMAIL_SEND:<hmac>:<json>

Where:
- <hmac> is HMAC-SHA256 of the JSON using secret "YOUR_WEBHOOK_SECRET"
- <json> is {"to":"...", "subject":"...", "body":"..."}

Show the user a preview first, then provide the GLAWMAIL_SEND message
for them to forward to their Gmail bot.
```

## Security

- **HMAC-SHA256 signed** - Messages without valid signatures are rejected
- **Human approval** - Owner must manually forward each message
- **No public endpoints** - Bot polls Telegram, no webhooks needed
- **Isolated credentials** - Gmail credentials only on Machine B

## Project Structure

```
glawmail/
├── cmd/
│   ├── machine_a/main.go    # Email preview bot (optional)
│   └── machine_b/main.go    # Gmail sender bot
├── internal/
│   ├── color/               # Terminal colors
│   ├── config/              # .env loading
│   ├── gmail/               # Gmail OAuth + send
│   ├── hmac/                # Message signing
│   └── telegram/            # Bot API helpers
├── setup/main.go            # Interactive setup wizard
├── go.mod
├── README.md
└── LICENSE
```

## Files

| File | Purpose |
|------|---------|
| `.env` | Bot token, chat ID, HMAC secret, Gmail paths |
| `token.json` | Gmail OAuth token - **never commit** |
| `credentials.json` | Google OAuth client - **never commit** |

## License

MIT License - see [LICENSE](LICENSE)
