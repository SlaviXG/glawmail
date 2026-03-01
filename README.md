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

## Install as Service (Linux/Pi)

```bash
# Build and install
./glawmail.sh install

# Manage
glawmail up       # start
glawmail down     # stop
glawmail status   # check status
glawmail logs     # view logs
```

Add to your shell profile for the `glawmail` command:
```bash
echo 'alias glawmail="~/glawmail/glawmail.sh"' >> ~/.bashrc
source ~/.bashrc
```

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
