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

# Add alias
echo 'alias glawmail="~/glawmail/glawmail.sh"' >> ~/.bashrc
source ~/.bashrc

# Manage
glawmail up       # start
glawmail down     # stop
glawmail status   # check status
glawmail logs     # view logs
```

### Windows

```cmd
glawmail.bat build
glawmail.bat install

:: Manage
glawmail.bat up       :: start
glawmail.bat down     :: stop
glawmail.bat status   :: check status
```

For auto-start on Windows login:
```cmd
schtasks /create /tn "GlawMail" /tr "C:\path\to\glawmail.exe" /sc onlogon /rl highest
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
