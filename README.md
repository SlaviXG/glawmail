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
```

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

Both automatically enable auto-start on system boot.

### Commands

| Command | Description |
|---------|-------------|
| `up` | Start the bot |
| `down` | Stop the bot |
| `status` | Check if running |
| `install` | Build + enable auto-start |
| `uninstall` | Stop + disable auto-start |

## AI Skill

Any AI can generate emails:

```
GLAWMAIL
To: recipient@example.com
Subject: Subject here
Body:
Your message here...
```

## License

MIT
