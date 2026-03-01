"""
approval_bot.py - Machine B (Approval Bot / @approvalbot)
==========================================================
Polls its own bot for GLAWMAIL_APPROVAL_REQUEST messages from Machine A.
Shows email previews with Approve/Decline buttons to the owner.
  Approve -> sends email via Gmail API, notifies Machine A
  Decline -> notifies Machine A
  Error   -> notifies Machine A

No public endpoint required. Telegram is the only transport.

First run:
  1. python setup.py --machine b
  2. python approval_bot.py    <- waits for pairing on first start
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

from pairing import (
    PAIR_CODE_TTL_SECONDS,
    PAIR_REQUEST_PREFIX,
    generate_pair_code,
    is_paired,
    is_trusted_sender,
    make_pair_confirm,
    parse_pair_message,
    record_pairing,
)

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

OWN_BOT_TOKEN        = os.environ["OWN_BOT_TOKEN"]
OWNER_CHAT_ID        = os.environ["OWNER_CHAT_ID"]
OPENCLAW_BOT_CHAT_ID = os.environ["OPENCLAW_BOT_CHAT_ID"]
WEBHOOK_SECRET       = os.environ["WEBHOOK_SECRET"]
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
    payload: dict = {
        "chat_id":    int(OWNER_CHAT_ID),
        "text":       text,
        "parse_mode": "HTML",
    }
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


def _send_to_peer(text: str):
    """Send a status message to Machine A's bot (@openclawbot)."""
    try:
        _tg("sendMessage", chat_id=int(OPENCLAW_BOT_CHAT_ID), text=text)
    except Exception as exc:
        logger.error("Failed to send message to @openclawbot: %s", exc)


def _send_to_openclaw(prefix: str, email_data: dict, extra: dict | None = None):
    """
    Send a signed status message to Machine A's bot.
    Format: <PREFIX>:<hmac_sig>:<json_payload>
    Used for GLAWMAIL_APPROVED, GLAWMAIL_DECLINE, and GLAWMAIL_ERROR events.
    """
    payload = {
        "callback_id": email_data.get("callback_id"),
        "to":          email_data.get("to"),
        "subject":     email_data.get("subject"),
        "metadata":    email_data.get("metadata", {}),
        **(extra or {}),
    }
    payload_str = json.dumps(payload)
    _send_to_peer(f"{prefix}:{_sign(payload_str)}:{payload_str}")
    logger.info("%s sent to @openclawbot for callback_id=%s", prefix, payload["callback_id"])


def _keyboard(callback_id: str) -> dict:
    return {"inline_keyboard": [[
        {"text": "Send Email", "callback_data": f"approve:{callback_id}"},
        {"text": "Decline",    "callback_data": f"decline:{callback_id}"},
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


# ── Pairing ───────────────────────────────────────────────────────────────────
def _run_pairing(offset: int) -> tuple[bool, int]:
    """
    Waits for a GLAWMAIL_PAIR_REQUEST from Machine A's bot, then:
      1. Displays the pairing code to the owner in Telegram
      2. Waits for the owner to type /confirm
      3. Sends GLAWMAIL_PAIR_CONFIRM:<code> back to Machine A
      4. Locks the peer bot ID to .pairing

    Returns (success, new_offset).
    """
    deadline = time.time() + PAIR_CODE_TTL_SECONDS
    pending_code: str | None = None
    pending_sender: str | None = None

    logger.info("Waiting for GLAWMAIL_PAIR_REQUEST from @openclawbot...")
    _send_to_owner(
        "Glawmail approval bot started.\n\n"
        "Waiting for Machine A to initiate pairing...\n"
        "Make sure ai_app.py is running on Machine A."
    )

    while time.time() < deadline:
        try:
            resp = requests.get(
                f"{OWN_API}/getUpdates",
                params={
                    "offset":          offset,
                    "timeout":         10,
                    "allowed_updates": ["message"],
                },
                timeout=15,
            )
            resp.raise_for_status()
            for update in resp.json().get("result", []):
                offset    = update["update_id"] + 1
                msg       = update.get("message", {})
                sender_id = str(msg.get("from", {}).get("id", ""))
                text      = msg.get("text", "")

                # Step 1 - receive PAIR_REQUEST from Machine A's bot
                if pending_code is None:
                    parsed = parse_pair_message(text)
                    if not parsed:
                        continue
                    prefix, code = parsed
                    if prefix != PAIR_REQUEST_PREFIX:
                        continue
                    # Only accept from the configured peer bot
                    if sender_id != str(OPENCLAW_BOT_CHAT_ID):
                        logger.warning(
                            "Pairing request from unexpected sender %s - ignoring", sender_id
                        )
                        continue
                    pending_code   = code
                    pending_sender = sender_id
                    logger.info("Received GLAWMAIL_PAIR_REQUEST - code=%s", code)
                    _send_to_owner(
                        f"Pairing request received from Machine A.\n\n"
                        f"Code: <b>{code}</b>\n\n"
                        f"Type /confirm to approve this pairing, "
                        f"or /cancel to reject it."
                    )

                # Step 2 - wait for owner to confirm
                elif text.strip() == "/confirm":
                    if str(msg.get("from", {}).get("id", "")) != str(OWNER_CHAT_ID):
                        continue
                    # Send confirm back to Machine A's bot
                    _send_to_peer(make_pair_confirm(pending_code))
                    record_pairing(pending_sender)
                    _send_to_owner(
                        "Pairing complete. "
                        "Machine B is now linked to Machine A."
                    )
                    logger.info(
                        "Pairing complete - peer locked to bot ID %s", pending_sender
                    )
                    return True, offset

                elif text.strip() == "/cancel":
                    if str(msg.get("from", {}).get("id", "")) != str(OWNER_CHAT_ID):
                        continue
                    pending_code   = None
                    pending_sender = None
                    _send_to_owner("Pairing cancelled. Waiting for a new request...")
                    logger.info("Owner cancelled pairing")

        except requests.exceptions.Timeout:
            pass
        except Exception as exc:
            logger.error("Pairing poll error: %s", exc)
            time.sleep(2)

    logger.error("Pairing timed out.")
    _send_to_owner("Pairing timed out. Restart both bots to try again.")
    return False, offset


# ── Process incoming GLAWMAIL_APPROVAL_REQUEST from Machine A ─────────────────
def _process_approval_request(text: str):
    """
    Parse and verify a GLAWMAIL_APPROVAL_REQUEST message.
    Format: GLAWMAIL_APPROVAL_REQUEST:<hmac_sig>:<json_payload>
    """
    try:
        prefix, sig, payload_str = text.split(":", 2)
        if prefix != "GLAWMAIL_APPROVAL_REQUEST":
            return
        if not _verify(payload_str, sig):
            logger.warning("GLAWMAIL_APPROVAL_REQUEST failed HMAC verification - ignoring")
            return
        data        = json.loads(payload_str)
        callback_id = data.get("callback_id")
        to          = data.get("to", "")
        subject     = data.get("subject", "(no subject)")
        body        = data.get("body", "")

        if not callback_id or not to:
            logger.warning("GLAWMAIL_APPROVAL_REQUEST missing callback_id or to - ignoring")
            return

        pending[callback_id] = data
        preview = (
            f"Email Approval Request\n\n"
            f"<b>To:</b> {to}\n"
            f"<b>Subject:</b> {subject}\n\n"
            f"<b>Body:</b>\n{body}\n\n"
            f"<i>Tap Send to approve, Decline to reject and notify the AI.</i>"
        )
        try:
            _send_to_owner(preview, _keyboard(callback_id))
        except Exception as exc:
            pending.pop(callback_id, None)
            _send_to_openclaw("GLAWMAIL_ERROR", data, extra={
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
    from_id = str(callback_query["from"]["id"])
    if from_id != str(OWNER_CHAT_ID):
        logger.warning("Ignoring callback from non-owner %s", from_id)
        return

    message_id    = callback_query["message"]["message_id"]
    callback_data = callback_query.get("data", "")
    action, _, callback_id = callback_data.partition(":")

    _tg("answerCallbackQuery", callback_query_id=callback_query["id"])

    email_data = pending.pop(callback_id, None)
    if not email_data:
        _edit_owner_message(message_id, "Email not found - already processed?")
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
                f"Sent to <b>{email_data['to']}</b>\n<i>Gmail ID: {gmail_id}</i>"
            )
            _send_to_openclaw("GLAWMAIL_APPROVED", email_data, extra={"gmail_id": gmail_id})
            logger.info("Email %s sent to %s", callback_id, email_data["to"])
        except Exception as exc:
            error_msg = str(exc)
            pending[callback_id] = email_data
            _edit_owner_message(
                message_id,
                f"Send failed: {error_msg}\n\n<i>Email restored - tap Send to retry.</i>"
            )
            _tg("editMessageReplyMarkup",
                chat_id=int(OWNER_CHAT_ID),
                message_id=message_id,
                reply_markup=_keyboard(callback_id))
            _send_to_openclaw("GLAWMAIL_ERROR", email_data, extra={
                "stage":  "gmail_send",
                "reason": error_msg,
            })
            logger.error("Gmail error for %s: %s", callback_id, exc)

    elif action == "decline":
        _edit_owner_message(
            message_id,
            f"Declined - email to <b>{email_data['to']}</b> discarded.\n"
            f"<i>Notifying AI...</i>"
        )
        _send_to_openclaw("GLAWMAIL_DECLINE", email_data, extra={"reason": "declined_by_owner"})
        logger.info("Email %s declined", callback_id)


# ── Main loop ─────────────────────────────────────────────────────────────────
def run():
    """
    Handles pairing on first run, then long-polls for:
      - Messages from @openclawbot (GLAWMAIL_APPROVAL_REQUEST)
      - callback_query events (button presses from the owner)

    Only messages from the locked peer bot ID are processed.
    All other senders are silently dropped.
    """
    offset = 0

    if not is_paired():
        logger.info("Not yet paired - waiting for pairing handshake from Machine A...")
        success, offset = _run_pairing(offset)
        if not success:
            logger.error("Pairing failed. Exiting.")
            sys.exit(1)

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

                    # Only process GLAWMAIL messages from the verified peer bot
                    if not is_trusted_sender(sender_id):
                        if text.startswith("GLAWMAIL_"):
                            logger.warning(
                                "Dropped GLAWMAIL message from untrusted sender %s", sender_id
                            )
                        continue

                    if text.startswith("GLAWMAIL_APPROVAL_REQUEST:"):
                        _process_approval_request(text)

                elif "callback_query" in update:
                    _process_callback_query(update["callback_query"])

        except requests.exceptions.Timeout:
            pass
        except Exception as exc:
            logger.error("Polling error: %s - retrying in 5s", exc)
            time.sleep(5)


if __name__ == "__main__":
    run()
