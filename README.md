# GlawMail - Email Approval Bot

Human-in-the-loop email sending via Telegram. Two bots, two machines, zero shared
networking - Telegram is the only transport between them.

---

## Architecture

```
Machine A - OpenClaw AI Bot (@openclawbot)
┌─────────────────────────────────────────────┐
│  ai_app.py                                  │
│                                             │
│  1. Generates email                         │
│  2. Sends GLAWMAIL_APPROVAL_REQUEST to @approvalbot  │──────────────────────────────►
│  5. Polls own bot for GLAWMAIL_APPROVED/GLAWMAIL_DECLINE messages  │◄─────────────────────────
│  6. Calls handle_approved_email()               │
│     or handle_declined_email()                  │
└─────────────────────────────────────────────┘
         │ via Telegram API only                        │ via Telegram API only
         ▼                                              │
    Telegram servers ──────────────────────────────────►
                                                        │
                             Machine B - Approval Bot (@approvalbot)
                             ┌──────────────────────────────────────┐
                             │  approval_bot.py                     │
                             │                                      │
                             │  3. Receives GLAWMAIL_APPROVAL_REQUEST        │
                             │  4. Shows preview + ✅/❌ to owner   │
                             │     ✅ → sends via Gmail API         │
                             │     ❌ → sends GLAWMAIL_DECLINE to @openclaw  │
                             └──────────────────────────────────────┘
```

**What's shared between machines:**
- Only the `WEBHOOK_SECRET` - used to HMAC-sign the JSON payloads sent via Telegram messages so neither machine can be spoofed

**What stays isolated:**
- Machine A has @openclawbot token only
- Machine B has @approvalbot token + Gmail credentials only
- No open ports, no public endpoints, no firewall rules needed on either machine

---

## Pairing

On the first run of each bot, a one-time pairing handshake locks each bot to its
counterpart. No operational messages are processed until pairing is complete.

```
Machine A starts                 Machine B starts
     |                                |
     | GLAWMAIL_PAIR_REQUEST:123456   |
     |------------------------------>>|
     |                                | Shows code 123456 to owner
     |                                | Owner types /confirm
     |                                |
     | GLAWMAIL_PAIR_CONFIRM:123456   |
     |<<------------------------------|
     |                                |
  Writes .pairing               Writes .pairing
  (peer bot ID locked)          (peer bot ID locked)
     |                                |
  Operational                   Operational
```

After pairing:
- Both bots store the verified peer bot ID in `.pairing` (mode 600)
- Any GLAWMAIL message from any other sender is silently dropped and logged
- To re-pair, delete `.pairing` on both machines and restart both bots

---

### Machine A to Machine B (approval request)
Sent as a Telegram message from @openclawbot to @approvalbot:
```
GLAWMAIL_APPROVAL_REQUEST:<sha256_hmac>:<json_payload>
```
```json
{
  "callback_id": "550e8400-...",
  "to":          "james@spotship.com",
  "subject":     "Quick question",
  "body":        "Hi James...",
  "html":        false,
  "metadata":    { "lead_id": "lead_001" }
}
```

### Machine B to Machine A (approved - email sent)
Sent as a Telegram message from @approvalbot to @openclawbot:
```
GLAWMAIL_APPROVED:<sha256_hmac>:<json_payload>
```
```json
{
  "callback_id": "550e8400-...",
  "to":          "james@spotship.com",
  "subject":     "Quick question",
  "gmail_id":    "18f3a2b1c4d5e6f7",
  "metadata":    { "lead_id": "lead_001" }
}
```

### Machine B to Machine A (declined - email discarded)
Sent as a Telegram message from @approvalbot to @openclawbot:
```
GLAWMAIL_DECLINE:<sha256_hmac>:<json_payload>
```
```json
{
  "callback_id": "550e8400-...",
  "to":          "james@spotship.com",
  "subject":     "Quick question",
  "reason":      "declined_by_owner",
  "metadata":    { "lead_id": "lead_001" }
}
```

### Machine B to Machine A (error - email not sent)
Sent as a Telegram message from @approvalbot to @openclawbot when something goes
wrong on Machine B, regardless of whether the owner approved or not:
```
GLAWMAIL_ERROR:<sha256_hmac>:<json_payload>
```
```json
{
  "callback_id": "550e8400-...",
  "to":          "james@spotship.com",
  "subject":     "Quick question",
  "stage":       "gmail_send",
  "reason":      "Token has been expired or revoked.",
  "metadata":    { "lead_id": "lead_001" }
}
```

Possible values for `stage`:
- `gmail_send` - the owner approved but the Gmail API call failed
- `telegram_preview` - the preview could not be delivered to the owner

The email has NOT been sent in either case.

All messages verified with HMAC-SHA256 before processing.

---

## Setup

### Prerequisites
```bash
pip install -r requirements.txt
```

### Step 1 - Create two Telegram bots
Message [@BotFather](https://t.me/botfather):
1. `/newbot` → name it something like **OpenClaw** → note the token
2. `/newbot` → name it something like **Email Approvals** → note the token

### Step 2 - Machine B: Gmail credentials
1. [Google Cloud Console](https://console.cloud.google.com/) → enable Gmail API
2. OAuth consent screen → add your Gmail as test user
3. Credentials → OAuth 2.0 Client ID (Desktop) → download as `credentials.json`

### Step 3 - Machine B: run setup
```bash
python setup.py --machine b
```
Note the **approval bot's numeric ID** printed during setup - you'll need it for Machine A.

### Step 4 - Machine A: run setup
```bash
python setup.py --machine a
```
You'll need:
- The WEBHOOK_SECRET generated here → copy to Machine B
- @approvalbot's numeric ID from Step 3

### Step 5 - Start both services

**Machine B:**
```bash
python approval_bot.py
```

**Machine A:**
```bash
# Integrate into your AI app:
from ai_app import start, request_email_approval
start()  # begins background polling for GLAWMAIL_DECLINE callbacks

# When your AI wants to send an email:
callback_id = request_email_approval(
    to       = "james@spotship.com",
    subject  = "Quick question about Spot Ship",
    body     = "Hi James...",
    metadata = {"lead_id": "lead_001"},
)

# Smoke test:
python ai_app.py test
```

---

## Files

| File | Machine | Purpose |
|---|---|---|
| `setup.py` | Both | Interactive first-run setup wizard |
| `pairing.py` | Both | Pairing protocol shared by both bots |
| `ai_app.py` | A | Sends approval requests, polls for status callbacks |
| `approval_bot.py` | B | Telegram UI, Gmail sender, status relay |
| `requirements.txt` | Both | Python dependencies |
| `.env` | Both | Generated by setup.py - **never commit** |
| `.pairing` | Both | Generated on first run - **never commit** |
| `token.json` | B only | Gmail OAuth token - **never commit** |
| `credentials.json` | B only | Google OAuth client - **never commit** |

---

## Production Hardening
- Replace `pending` dict in `approval_bot.py` with **Redis** for persistence across restarts
- Run both scripts under **systemd** or in **Docker**
- File permissions: `chmod 600 .env token.json credentials.json`
- Rotate `WEBHOOK_SECRET` periodically by re-running `setup.py`
