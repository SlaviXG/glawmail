# GlawMail

Gmail sender bot for Telegram. Forward an email in GLAWMAIL format to the bot, it sends via Gmail.

## Format

```
GLAWMAIL
To: recipient@example.com
Subject: Email subject
Body:
Email body text here...
```

## Setup

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go run ./setup
go run ./cmd/glawmail
```

## AI Skill

Any AI can generate emails. Example prompt:

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
go build -o glawmail ./cmd/glawmail

# Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o glawmail-pi ./cmd/glawmail
```

## License

MIT
