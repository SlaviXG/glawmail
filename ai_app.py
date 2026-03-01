"""
ai_app.py - Machine A (OpenClaw AI Bot)
========================================
Generates emails, sends approval requests to Machine B's bot via Telegram,
and listens for decline callbacks on its own bot's incoming messages.

No public endpoint required. Telegram is the only transport.

Run setup.py --machine a first, then:
    python ai_app.py
"""

import hashlib
import hmac
import json
import logging
import os
import sys
import time
import uuid
from pathlib import Path
from threading import Thread

import requests
from dotenv import load_dotenv

# ── Load config ───────────────────────────────────────────────────────────────
_env_path = Path(__file__).parent / ".env"
if not _env_path.exists():
    print("ERROR: .env not found. Run: python setup.py --machine a")
    sys.exit(1)
load_dotenv(_env_path)

_REQUIRED = ["OWN_BOT_TOKEN", "OWNER_CHAT_ID", "APPROVAL_BOT_CHAT_ID", "WEBHOOK_SECRET"]
_missing  = [k for k in _REQUIRED if not os.getenv(k)]
if _missing:
    print(f"ERROR: Missing env vars: {', '.join(_missing)}")
    print("Run: python setup.py --machine a")
    sys.exit(1)

OWN_BOT_TOKEN        = os.environ["OWN_BOT_TOKEN"]        # @openclawbot token
OWNER_CHAT_ID        = os.environ["OWNER_CHAT_ID"]        # Your personal chat ID
APPROVAL_BOT_CHAT_ID = os.environ["APPROVAL_BOT_CHAT_ID"] # Machine B's bot user ID
WEBHOOK_SECRET       = os.environ["WEBHOOK_SECRET"]        # Shared HMAC secret

OWN_API = f"https://api.telegram.org/bot{OWN_BOT_TOKEN}"

# ── Logging ───────────────────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
)
logger = logging.getLogger(__name__)


# ── HMAC helpers ──────────────────────────────────────────────────────────────
def _sign(payload: str) -> str:
    return "sha256=" + hmac.new(
        WEBHOOK_SECRET.encode(), payload.encode(), hashlib.sha256
    ).hexdigest()


def _verify(payload: str, signature: str) -> bool:
    return hmac.compare_digest(signature, _sign(payload))


# ── Telegram helpers ──────────────────────────────────────────────────────────
def _tg(method: str, **kwargs) -> dict:
    r = requests.post(f"{OWN_API}/{method}", json=kwargs, timeout=10)
    r.raise_for_status()
    return r.json()


def _send_to_approval_bot(text: str):
    """Send a message from @openclawbot to @approvalbot via Telegram."""
    _tg("sendMessage", chat_id=int(APPROVAL_BOT_CHAT_ID), text=text)


# ── Public API: request email approval ───────────────────────────────────────
def request_email_approval(
    to: str,
    subject: str,
    body: str,
    html: bool = False,
    metadata: dict | None = None,
) -> str:
    """
    Send an email approval request to Machine B via Telegram.
    Returns the callback_id so you can correlate approval and decline events.

    Machine B will either:
      - Send an GLAWMAIL_APPROVED:<json> message back to this bot after Gmail sends successfully
      - Send a GLAWMAIL_DECLINE:<json> message back to this bot if the owner rejects it
    """
    callback_id = str(uuid.uuid4())
    payload = {
        "callback_id": callback_id,
        "to":          to,
        "subject":     subject,
        "body":        body,
        "html":        html,
        "metadata":    metadata or {},
    }
    payload_str = json.dumps(payload)
    # Prefix so Machine B can identify approval request messages
    # Format: GLAWMAIL_APPROVAL_REQUEST:<hmac_sig>:<json_payload>
    message = f"GLAWMAIL_APPROVAL_REQUEST:{_sign(payload_str)}:{payload_str}"
    _send_to_approval_bot(message)
    logger.info("Approval request sent for %s (callback_id=%s)", to, callback_id)
    return callback_id


# ── Approval, decline, and error handlers ────────────────────────────────────
def handle_approved_email(callback_id: str, to: str, subject: str, gmail_id: str, metadata: dict):
    """
    Called when Machine B confirms the email was sent via Gmail.
    Replace the body with your actual business logic.
    """
    logger.info("GLAWMAIL_APPROVED - id=%s | to=%s | subject=%s | gmail_id=%s | metadata=%s",
                callback_id, to, subject, gmail_id, metadata)
    # e.g. db.mark_sent(callback_id, gmail_id)
    # e.g. crm.log_outreach(to, subject)
    # e.g. agent.notify_sent(callback_id)


def handle_declined_email(callback_id: str, to: str, subject: str, metadata: dict):
    """
    Called when the owner taps Decline in Telegram.
    Replace the body with your actual business logic.
    """
    logger.info("GLAWMAIL_DECLINED - id=%s | to=%s | subject=%s | metadata=%s",
                callback_id, to, subject, metadata)
    # e.g. db.mark_declined(callback_id)
    # e.g. agent.notify_declined(callback_id)
    # e.g. lead_manager.do_not_contact(to)


def handle_error_email(callback_id: str, to: str, subject: str,
                       stage: str, reason: str, metadata: dict):
    """
    Called when Machine B hits an error processing an approval request.

    `stage` indicates where the failure occurred:
      - "gmail_send"       the Gmail API call failed after the owner approved
      - "telegram_preview" the bot could not deliver the preview to the owner

    The email has NOT been sent in either case. Machine A should decide whether
    to retry, alert a human, or mark the job as failed.

    Replace the body with your actual business logic.
    """
    logger.error("ERROR - id=%s | to=%s | stage=%s | reason=%s | metadata=%s",
                 callback_id, to, stage, reason, metadata)
    # e.g. db.mark_error(callback_id, stage, reason)
    # e.g. agent.notify_error(callback_id, stage, reason)
    # e.g. retry_queue.enqueue(callback_id)


def _process_status_message(text: str):
    """
    Parse and verify an GLAWMAIL_APPROVED, GLAWMAIL_DECLINE, or GLAWMAIL_ERROR message from Machine B's bot.
    Format: <PREFIX>:<hmac_sig>:<json_payload>
    """
    try:
        prefix, sig, payload_str = text.split(":", 2)
        if prefix not in ("GLAWMAIL_APPROVED", "GLAWMAIL_DECLINE", "GLAWMAIL_ERROR"):
            return
        if not _verify(payload_str, sig):
            logger.warning("%s message failed HMAC verification - ignoring", prefix)
            return
        data = json.loads(payload_str)
        if prefix == "GLAWMAIL_APPROVED":
            handle_approved_email(
                callback_id = data.get("callback_id", ""),
                to          = data.get("to", ""),
                subject     = data.get("subject", ""),
                gmail_id    = data.get("gmail_id", ""),
                metadata    = data.get("metadata", {}),
            )
        elif prefix == "GLAWMAIL_DECLINE":
            handle_declined_email(
                callback_id = data.get("callback_id", ""),
                to          = data.get("to", ""),
                subject     = data.get("subject", ""),
                metadata    = data.get("metadata", {}),
            )
        elif prefix == "GLAWMAIL_ERROR":
            handle_error_email(
                callback_id = data.get("callback_id", ""),
                to          = data.get("to", ""),
                subject     = data.get("subject", ""),
                stage       = data.get("stage", "unknown"),
                reason      = data.get("reason", ""),
                metadata    = data.get("metadata", {}),
            )
    except (ValueError, json.JSONDecodeError) as exc:
        logger.debug("Ignoring non-status message: %s", exc)


# ── Update poller ─────────────────────────────────────────────────────────────
def _poll_updates():
    """
    Long-polls the Telegram Bot API for incoming messages to @openclawbot.
    Filters for GLAWMAIL_APPROVED, GLAWMAIL_DECLINE, and GLAWMAIL_ERROR messages from Machine B's bot only.
    Runs in a background thread.
    """
    offset = 0
    logger.info("Polling for Telegram updates on @openclawbot...")
    while True:
        try:
            resp = requests.get(
                f"{OWN_API}/getUpdates",
                params={"offset": offset, "timeout": 30, "allowed_updates": ["message"]},
                timeout=40,
            )
            resp.raise_for_status()
            updates = resp.json().get("result", [])
            for update in updates:
                offset = update["update_id"] + 1
                msg = update.get("message", {})
                # Only process messages from Machine B's bot
                sender_id = str(msg.get("from", {}).get("id", ""))
                if sender_id != APPROVAL_BOT_CHAT_ID:
                    continue
                text = msg.get("text", "")
                if (text.startswith("GLAWMAIL_APPROVED:") or
                        text.startswith("GLAWMAIL_DECLINE:") or
                        text.startswith("GLAWMAIL_ERROR:")):
                    _process_status_message(text)
        except requests.exceptions.Timeout:
            pass  # Normal for long-polling
        except Exception as exc:
            logger.error("Polling error: %s - retrying in 5s", exc)
            time.sleep(5)


# ── Start ─────────────────────────────────────────────────────────────────────
def start():
    """Start the background update poller. Call this from your AI app."""
    t = Thread(target=_poll_updates, daemon=True)
    t.start()
    return t


# ── Example / smoke test ──────────────────────────────────────────────────────
if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "test":
        print("Sending test approval request to Machine B...")
        callback_id = request_email_approval(
            to       = "james@spotship.com",
            subject  = "Quick question about Spot Ship",
            body     = (
                "Hi James,\n\n"
                "I saw you're building Spot Ship - congrats on the recent raise!\n\n"
                "Curious: how much time do you spend manually reconciling Stripe payouts?\n\n"
                "Happy to chat for 20 min.\n\nSlava"
            ),
            metadata = {"lead_id": "lead_001", "campaign": "outreach_jan_2026"},
        )
        print(f"Sent. callback_id={callback_id}")
        print("Now polling for decline callbacks - press Ctrl+C to stop.")
        _poll_updates()  # Block in foreground for the test
    else:
        print("Usage: python ai_app.py test")
        print("Import start() and request_email_approval() into your AI app.")
