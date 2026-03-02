# GlawMail

Human-in-the-loop Gmail approval bot for Telegram. Secure AI email sending with complete separation between AI and Gmail access.

## Why GlawMail?

AI agents need to send emails, but giving AI direct Gmail access is risky. GlawMail solves this:

- **AI has no Gmail access** - Your AI assistant generates email requests, but cannot send anything
- **Gmail bot has no AI access** - The sending bot only processes human-forwarded messages
- **You approve every email** - Forward to send, ignore to skip

This architecture ensures AI can never send unauthorized emails, even if compromised.

## How It Works

```
[AI Assistant] --> generates GLAWMAIL message
      |
      v
[You on Telegram] --> review the email
      |
      v (forward to approve)
[GlawMail Bot] --> sends via Gmail API
```

The human-in-the-loop design means you see and approve every email before it goes out.

## GLAWMAIL Format

Any AI can generate this format:

```
GLAWMAIL
To: recipient@example.com
Subject: Email subject
Body:
Email body text here...
```

Works with ChatGPT, Claude, Gemini, local LLMs, or any AI assistant.

## Setup

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go run ./setup
```

The setup wizard guides you through:
1. Creating a Telegram bot via @BotFather
2. Connecting your Gmail via OAuth
3. Optional global `glawmail` command

## Install as Service

### Linux / Raspberry Pi

```bash
chmod +x glawmail.sh
./glawmail.sh install
glawmail up
```

### Windows (run as Administrator)

```cmd
glawmail.bat install
glawmail.bat up
```

Auto-starts on system boot.

### Commands

| Command | Description |
|---------|-------------|
| `up` | Start the bot |
| `down` | Stop the bot |
| `status` | Check if running |
| `install` | Build and enable auto-start |
| `uninstall` | Stop and disable auto-start |

## Use Cases

- **AI email assistants** - Let AI draft emails, you approve and send
- **Outreach automation** - AI generates personalized emails, human reviews before sending
- **Agentic workflows** - Safe email capability for autonomous agents
- **Email pipelines** - Batch process AI-generated emails with human oversight

## Security

- No public endpoints required
- Gmail OAuth tokens stored locally
- Bot only responds to your Telegram chat ID
- AI and Gmail completely isolated from each other

## License

MIT
