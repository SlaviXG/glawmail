# GlawMail

Human-in-the-loop Gmail approval bot for Telegram. Secure AI email sending with complete separation between AI and Gmail access.

For maximum security, run GlawMail on a separate machine (e.g. Raspberry Pi) from your AI. This ensures complete isolation between AI and Gmail credentials.

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

---

## For Humans

### Setup

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go run ./setup
```

The setup wizard guides you through:
1. Creating a Telegram bot via @BotFather
2. Connecting your Gmail via OAuth
3. Optional global `glawmail` command

### Install as Service

#### Linux / Raspberry Pi

```bash
chmod +x glawmail.sh
./glawmail.sh install
glawmail up
```

#### Windows (run as Administrator)

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

---

## For AI Agents

See [SKILL.md](SKILL.md) for the full skill specification.

### Format

When asked to send an email, output this format:

```
GLAWMAIL
To: recipient@example.com
Subject: Email subject
Body:
Email body text here...
```

### Rules

- Start with `GLAWMAIL` on its own line
- `To:` followed by email address
- `Subject:` followed by subject line
- `Body:` followed by the message
- You cannot send emails directly - user forwards to approve
- For multiple emails, send each as a separate message

### Example

User: "Email john@example.com about the project update"

```
GLAWMAIL
To: john@example.com
Subject: Project Update
Body:
Hi John,

Here is the latest update on the project.

Best regards
```

---

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
