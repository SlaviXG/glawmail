# GlawMail - Email Approval Bot

Human-in-the-loop email approval system. An AI bot generates emails on Machine A, sends them to Machine B for human approval via Telegram, and Machine B sends approved emails via Gmail.

**Zero shared networking** - Telegram is the only transport between machines.

## Architecture

```
┌─────────────────────┐                    ┌─────────────────────┐
│     Machine A       │                    │     Machine B       │
│  (AI Bot Server)    │                    │  (Approval Server)  │
├─────────────────────┤                    ├─────────────────────┤
│                     │   Telegram API     │                     │
│  AI generates email ├───────────────────►│  Shows preview to   │
│                     │  APPROVAL_REQUEST  │  owner in Telegram  │
│                     │                    │                     │
│                     │◄───────────────────┤  Owner taps         │
│  Receives callback  │  APPROVED/DECLINE  │  ✅ Send / ❌ Decline │
│                     │                    │                     │
│                     │                    │  If approved:       │
│                     │                    │  → Gmail API send   │
└─────────────────────┘                    └─────────────────────┘
```

## Security

- **HMAC-SHA256 signed messages** - All operational messages are cryptographically signed
- **One-time pairing** - 6-digit code handshake locks machines together on first run
- **Isolated credentials** - Machine A has no Gmail access; Machine B has no AI access
- **No public endpoints** - Both bots poll Telegram, no webhooks needed

## Quick Start

### Prerequisites

- Go 1.22+
- Two Telegram bots (create via [@BotFather](https://t.me/BotFather))
- Gmail API credentials (for Machine B)

### Installation

```bash
git clone https://github.com/SlaviXG/glawmail
cd glawmail
go mod download
```

### Setup Machine A (AI Bot)

```bash
go run ./setup --machine a
```

This will prompt for:
1. Your AI bot token (@openclawbot)
2. Your Telegram chat ID
3. Machine B's bot ID
4. Generate a shared HMAC secret

### Setup Machine B (Approval Bot)

```bash
go run ./setup --machine b
```

This will prompt for:
1. Your approval bot token (@approvalbot)
2. Your Telegram chat ID
3. Machine A's bot ID
4. The shared HMAC secret from Machine A
5. Gmail OAuth authorization

### Run

**Machine A:**
```bash
go run ./cmd/machine_a
```

**Machine B:**
```bash
go run ./cmd/machine_b
```

On first run, both machines will perform a pairing handshake. Machine A sends a 6-digit code to Machine B, you confirm it in Telegram, and they're locked together.

### Test

```bash
go run ./cmd/machine_a test
```

Sends a test approval request to Machine B.

## Building

```bash
# Build for current platform
go build -o glawmail-a ./cmd/machine_a
go build -o glawmail-b ./cmd/machine_b
go build -o glawmail-setup ./setup

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o glawmail-a-linux ./cmd/machine_a
GOOS=darwin GOARCH=arm64 go build -o glawmail-a-mac ./cmd/machine_a
GOOS=windows GOARCH=amd64 go build -o glawmail-a.exe ./cmd/machine_a
```

## Project Structure

```
glawmail/
├── cmd/
│   ├── machine_a/main.go    # AI bot bridge
│   └── machine_b/main.go    # Approval bot + Gmail sender
├── internal/
│   ├── config/              # .env loading
│   ├── gmail/               # Gmail OAuth + send
│   ├── hmac/                # Message signing
│   ├── pairing/             # Pairing protocol
│   └── telegram/            # Bot API helpers
├── setup/main.go            # Interactive setup wizard
├── go.mod
├── README.md
└── LICENSE
```

## Pairing Protocol

On first run, a one-time pairing handshake locks each bot to its counterpart:

```
Machine A starts                 Machine B starts
     │                                │
     │ GLAWMAIL_PAIR_REQUEST:123456   │
     │───────────────────────────────►│
     │                                │ Shows code to owner
     │                                │ Owner types /confirm
     │                                │
     │ GLAWMAIL_PAIR_CONFIRM:123456   │
     │◄───────────────────────────────│
     │                                │
  Writes .pairing               Writes .pairing
  (peer bot ID locked)          (peer bot ID locked)
```

After pairing:
- Both bots store the verified peer bot ID in `.pairing` (mode 600)
- Any GLAWMAIL message from any other sender is silently dropped
- To re-pair, delete `.pairing` on both machines and restart

## Message Protocol

All messages use the format: `PREFIX:HMAC_SIGNATURE:JSON_PAYLOAD`

| Message | Direction | Purpose |
|---------|-----------|---------|
| `GLAWMAIL_PAIR_REQUEST:<code>` | A → B | Initiate pairing |
| `GLAWMAIL_PAIR_CONFIRM:<code>` | B → A | Confirm pairing |
| `GLAWMAIL_APPROVAL_REQUEST` | A → B | Request email approval |
| `GLAWMAIL_APPROVED` | B → A | Email sent successfully |
| `GLAWMAIL_DECLINE` | B → A | Owner declined the email |
| `GLAWMAIL_ERROR` | B → A | Error during processing |

### Example: Approval Request (A → B)
```
GLAWMAIL_APPROVAL_REQUEST:sha256=abc123...:{json}
```
```json
{
  "callback_id": "550e8400-...",
  "to": "james@spotship.com",
  "subject": "Quick question",
  "body": "Hi James...",
  "html": false,
  "metadata": { "lead_id": "lead_001" }
}
```

### Example: Approved (B → A)
```json
{
  "callback_id": "550e8400-...",
  "to": "james@spotship.com",
  "subject": "Quick question",
  "gmail_id": "18f3a2b1c4d5e6f7",
  "metadata": { "lead_id": "lead_001" }
}
```

## Files

| File | Machine | Purpose |
|------|---------|---------|
| `cmd/machine_a/main.go` | A | Sends approval requests, polls for callbacks |
| `cmd/machine_b/main.go` | B | Telegram UI, Gmail sender, status relay |
| `setup/main.go` | Both | Interactive first-run setup wizard |
| `.env` | Both | Generated by setup - **never commit** |
| `.pairing` | Both | Generated on first run - **never commit** |
| `token.json` | B only | Gmail OAuth token - **never commit** |
| `credentials.json` | B only | Google OAuth client - **never commit** |

## Production

- Replace in-memory `pending` map with Redis for persistence
- Run as systemd services or in Docker
- File permissions: `chmod 600 .env token.json credentials.json`
- Rotate `WEBHOOK_SECRET` periodically by re-running setup

## License

MIT License - see [LICENSE](LICENSE)
