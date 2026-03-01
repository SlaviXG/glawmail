# GlawMail - Gmail Sender Bot

Human-in-the-loop email sending via Telegram. Forward an email in GLAWMAIL format to the bot, and it sends via Gmail.

## Format

```
GLAWMAIL
To: recipient@example.com
Subject: Email subject
Body:
Email body text here...
```

## Quick Start

### Prerequisites

- Go 1.22+
- A Telegram bot (create via [@BotFather](https://t.me/BotFather))
- Gmail API credentials

### Setup

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go run ./setup --machine b
```

This prompts for:
1. Your bot token
2. Your Telegram chat ID
3. Gmail OAuth authorization

### Run

```bash
go run ./cmd/machine_b
```

### Use

Forward any message starting with `GLAWMAIL` in the format above. The bot parses it and sends via Gmail.

## AI Integration

Any AI can generate emails for this system. Example prompt:

```
When sending an email, format it as:

GLAWMAIL
To: recipient@example.com
Subject: Subject here
Body:
Your message here...

The user will forward this to their Gmail bot.
```

## Building

```bash
# Build for current platform
go build -o glawmail-sender ./cmd/machine_b

# Cross-compile for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o glawmail-sender-pi ./cmd/machine_b
```

## Project Structure

```
glawmail/
├── cmd/
│   ├── machine_a/    # Email preview bot (optional)
│   └── machine_b/    # Gmail sender bot
├── internal/
│   ├── color/        # Terminal colors
│   ├── config/       # .env loading
│   ├── gmail/        # Gmail OAuth + send
│   └── telegram/     # Bot API helpers
├── setup/            # Setup wizard
└── README.md
```

## License

MIT License - see [LICENSE](LICENSE)
