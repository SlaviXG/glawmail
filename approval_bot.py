"""
approval_bot.py - Machine B (Approval Bot + Gmail Sender)
==========================================================
Polls its own bot (@approvalbot) for APPROVAL_REQUEST messages from Machine A.
Shows email previews with ✅ / ❌ buttons to the owner.
  ✅ → sends email via Gmail API
  ❌ → sends a signed DECLINE message back to Machine A's bot (@openclawbot)

No public endpoint required. No direct networking to Machine A.
Telegram is the only transport between machines.

Run setup.py --machine b first, then:
    python approval_bot.py
"""

import base64
import hashlib
import hmac
import json
import logging
import os
import sys
import time
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from pathlib import Path

import requests
from dotenv import load_dotenv
from google.auth.transport.requests import Request
from google.oauth2.credentials import Credentials
from googleapiclient.discovery import build

# ── Load config ───────────────────────────────────────────────────────────────
_env_path = Path(__file__).parent / ".env"
if not _env_path.exists():
    print("ERROR: .env not found. Run: python setup.py --machine b")
    sys.exit(1)
load_dotenv(_env_path)

_REQUIRED = [
    "OWN_BOT_TOKEN",
    "OWNER_CHAT_ID",
    "OPENCLAW_BOT_CHAT_ID",
    "WEBHOOK_SECRET",
    "GMAIL_FROM",
    "GMAIL_TOKEN_FILE",
]
_missing = [k for k in _REQUIRED if not os.getenv(k)]
if _missing:
    print(f"ERROR: Missing env vars: {', '.join(_missing)}")
    print("Run: python setup.py --machine b")
    sys.exit(1)

OWN_BOT_TOKEN        = os.environ["OWN_BOT_TOKEN"]        # @approvalbot token
OWNER_CHAT_ID        = os.environ["OWNER_CHAT_ID"]        # Your personal chat ID
OPENCLAW_BOT_CHAT_ID = os.environ["OPENCLAW_BOT_CHAT_ID"] # Machine A's bot user ID
WEBHOOK_SECRET       = os.environ["WEBHOOK_SECRET"]        # Shared HMAC secret
GMAIL_FROM           = os.environ["GMAIL_FROM"]
GMAIL_TOKEN_FILE     = os.environ["GMAIL_TOKEN_FILE"]

OWN_API      = f"https://api.telegram.org/bot{OWN_BOT_TOKEN}"
GMAIL_SCOPES = ["https://www.googleapis.com/auth/gmail.send"]

# ── Logging ───────────────────────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
)
logger = logging.getLogger(__name__)

# In-memory store: callback_id to email payload
# Use Redis for persistence across restarts
pending: dict[str, dict] = {}


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


def _send_to_owner(text: str, reply_markup: dict | None = None):
    """Show an email preview (or status update) to the owner."""
    payload: dict = {"chat_id": int(OWNER_CHAT_ID), "text": text, "parse_mode": "HTML"}
    if reply_markup:
        payload["reply_markup"] = reply_markup
    _tg("sendMessage", **payload)


def _edit_owner_message(message_id: int, text: str):
    _tg("editMessageText",
        chat_id=int(OWNER_CHAT_ID),
        message_id=message_id,
        text=text,
        parse_mode="HTML",
        reply_markup={"inline_keyboard": []})


def _send_to_openclaw(prefix: str, email_data: dict, extra: dict | None = None):
    """
    Send a signed status message to Machine A's bot (@openclawbot).
    Format: <PREFIX>:<hmac_sig>:<json_payload>

    Used for APPROVED, DECLINE, and ERROR events.
    Failures here are logged but never re-raised - the caller should not crash
    because of a notification delivery problem.
    """
    payload = {
        "callback_id": email_data.get("callback_id"),
        "to":          email_data.get("to"),
        "subject":     email_data.get("subject"),
        "metadata":    email_data.get("metadata", {}),
        **(extra or {}),
    }
    payload_str = json.dumps(payload)
    message = f"{prefix}:{_sign(payload_str)}:{payload_str}"
    try:
        _tg("sendMessage", chat_id=int(OPENCLAW_BOT_CHAT_ID), text=message)
        logger.info("%s sent to @openclawbot for callback_id=%s", prefix, payload["callback_id"])
    except Exception as exc:
        logger.error("Failed to send %s to @openclawbot for callback_id=%s: %s",
                     prefix, payload["callback_id"], exc)


def _keyboard(callback_id: str) -> dict:
    return {"inline_keyboard": [[
        {"text": "✅ Send Email", "callback_data": f"approve:{callback_id}"},
        {"text": "❌ Decline",    "callback_data": f"decline:{callback_id}"},
    ]]}


# ── Gmail helpers ─────────────────────────────────────────────────────────────
def _gmail_service():
    token_path = Path(GMAIL_TOKEN_FILE)
    if not token_path.exists():
        raise FileNotFoundError(
            f"Gmail token not found at {GMAIL_TOKEN_FILE}. "
            "Run: python setup.py --machine b"
        )
    creds = Credentials.from_authorized_user_file(str(token_path), GMAIL_SCOPES)
    if creds.expired and creds.refresh_token:
        creds.refresh(Request())
        token_path.write_text(creds.to_json())
        token_path.chmod(0o600)
        logger.info("Gmail token refreshed")
    return build("gmail", "v1", credentials=creds)


def _send_gmail(to: str, subject: str, body: str, html: bool) -> str:
    msg = MIMEMultipart("alternative")
    msg["From"]    = GMAIL_FROM
    msg["To"]      = to
    msg["Subject"] = subject
    msg.attach(MIMEText(body, "html" if html else "plain"))
    raw    = base64.urlsafe_b64encode(msg.as_bytes()).decode()
    result = _gmail_service().users().messages().send(
        userId="me", body={"raw": raw}
    ).execute()
    return result["id"]


# ── Process incoming APPROVAL_REQUEST from Machine A ─────────────────────────
def _process_approval_request(text: str):
    """
    Parse and verify an APPROVAL_REQUEST message from @openclawbot.
    Format: APPROVAL_REQUEST:<hmac_sig>:<json_payload>
    """
    try:
        prefix, sig, payload_str = text.split(":", 2)
        if prefix != "APPROVAL_REQUEST":
            return
        if not _verify(payload_str, sig):
            logger.warning("APPROVAL_REQUEST failed HMAC verification - ignoring")
            return
        data        = json.loads(payload_str)
        callback_id = data.get("callback_id")
        to          = data.get("to", "")
        subject     = data.get("subject", "(no subject)")
        body        = data.get("body", "")

        if not callback_id or not to:
            logger.warning("APPROVAL_REQUEST missing callback_id or to - ignoring")
            return

        pending[callback_id] = data
        preview = (
            f"📧 <b>Email Approval Request</b>\n\n"
            f"<b>To:</b> {to}\n"
            f"<b>Subject:</b> {subject}\n\n"
            f"<b>Body:</b>\n{body}\n\n"
            f"<i>Tap Send to approve, Decline to reject and notify the AI.</i>"
        )
        try:
            _send_to_owner(preview, _keyboard(callback_id))
        except Exception as exc:
            # Could not deliver the preview to the owner via Telegram
            pending.pop(callback_id, None)
            _send_to_openclaw("ERROR", data, extra={
                "stage":  "telegram_preview",
                "reason": str(exc),
            })
            logger.error("Failed to send preview to owner for %s: %s", callback_id, exc)
            return

        logger.info("Approval request queued: %s to %s", callback_id, to)
    except (ValueError, json.JSONDecodeError) as exc:
        logger.debug("Ignoring non-approval message: %s", exc)


# ── Process button presses (callback_query) ───────────────────────────────────
def _process_callback_query(callback_query: dict):
    # Owner-only enforcement
    from_id = str(callback_query["from"]["id"])
    if from_id != OWNER_CHAT_ID:
        logger.warning("Ignoring callback from non-owner %s", from_id)
        return

    message_id    = callback_query["message"]["message_id"]
    callback_data = callback_query.get("data", "")
    action, _, callback_id = callback_data.partition(":")

    # Ack the button press
    _tg("answerCallbackQuery", callback_query_id=callback_query["id"])

    email_data = pending.pop(callback_id, None)
    if not email_data:
        _edit_owner_message(message_id, "⚠️ Email not found - already processed?")
        return

    if action == "approve":
        try:
            gmail_id = _send_gmail(
                to      = email_data["to"],
                subject = email_data.get("subject", ""),
                body    = email_data.get("body", ""),
                html    = email_data.get("html", False),
            )
            _edit_owner_message(
                message_id,
                f"✅ Sent to <b>{email_data['to']}</b>\n<i>Gmail ID: {gmail_id}</i>"
            )
            _send_to_openclaw("APPROVED", email_data, extra={"gmail_id": gmail_id})
            logger.info("Email %s sent to %s", callback_id, email_data["to"])
        except Exception as exc:
            error_msg = str(exc)
            # Restore pending so the owner can retry from Telegram
            pending[callback_id] = email_data
            _edit_owner_message(
                message_id,
                f"❌ Send failed: {error_msg}\n\n<i>Email restored - tap Send to retry.</i>"
            )
            _tg("editMessageReplyMarkup",
                chat_id=int(OWNER_CHAT_ID),
                message_id=message_id,
                reply_markup=_keyboard(callback_id))
            # Notify Machine A so it is not left waiting
            _send_to_openclaw("ERROR", email_data, extra={
                "stage":   "gmail_send",
                "reason":  error_msg,
            })
            logger.error("Gmail error for %s: %s", callback_id, exc)

    elif action == "decline":
        _edit_owner_message(
            message_id,
            f"🚫 <b>Declined</b> - email to <b>{email_data['to']}</b> discarded.\n"
            f"<i>Notifying AI...</i>"
        )
        _send_to_openclaw("DECLINE", email_data, extra={"reason": "declined_by_owner"})


# ── Main polling loop ─────────────────────────────────────────────────────────
def run():
    """
    Long-poll the Telegram Bot API for:
      - Messages from @openclawbot (APPROVAL_REQUEST)
      - callback_query events (button presses from the owner)
    """
    offset = 0
    logger.info("Approval bot running - polling Telegram...")
    while True:
        try:
            resp = requests.get(
                f"{OWN_API}/getUpdates",
                params={
                    "offset":          offset,
                    "timeout":         30,
                    "allowed_updates": ["message", "callback_query"],
                },
                timeout=40,
            )
            resp.raise_for_status()
            updates = resp.json().get("result", [])

            for update in updates:
                offset = update["update_id"] + 1

                if "message" in update:
                    msg       = update["message"]
                    sender_id = str(msg.get("from", {}).get("id", ""))
                    text      = msg.get("text", "")
                    # Only process messages from Machine A's bot
                    if sender_id == OPENCLAW_BOT_CHAT_ID and text.startswith("APPROVAL_REQUEST:"):
                        _process_approval_request(text)

                elif "callback_query" in update:
                    _process_callback_query(update["callback_query"])

        except requests.exceptions.Timeout:
            pass  # Normal for long-polling
        except Exception as exc:
            logger.error("Polling error: %s - retrying in 5s", exc)
            time.sleep(5)


if __name__ == "__main__":
    run()
