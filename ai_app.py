"""
ai_app.py - Machine A (OpenClaw AI Bot / @openclawbot)
=======================================================
Generates emails, sends approval requests to Machine B's bot via Telegram,
and listens for status callbacks on its own bot's incoming messages.

No public endpoint required. Telegram is the only transport.

First run:
  1. python setup.py --machine a
  2. python ai_app.py          <- initiates pairing on first start
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
from threading import Event, Thread

import requests
from dotenv import load_dotenv

from pairing import (
    PAIR_CODE_TTL_SECONDS,
    PAIR_CONFIRM_PREFIX,
    generate_pair_code,
    is_paired,
    is_trusted_sender,
    make_pair_request,
    parse_pair_message,
    record_pairing,
)

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

OWN_BOT_TOKEN        = os.environ["OWN_BOT_TOKEN"]
OWNER_CHAT_ID        = os.environ["OWNER_CHAT_ID"]
APPROVAL_BOT_CHAT_ID = os.environ["APPROVAL_BOT_CHAT_ID"]
WEBHOOK_SECRET       = os.environ["WEBHOOK_SECRET"]

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


def _send_to_peer(text: str):
    """Send a message to Machine B's bot (@approvalbot)."""
    _tg("sendMessage", chat_id=int(APPROVAL_BOT_CHAT_ID), text=text)


def _notify_owner(text: str):
    """Send a plain status message to the owner's personal chat."""
    try:
        _tg("sendMessage", chat_id=int(OWNER_CHAT_ID), text=text)
    except Exception as exc:
        logger.warning("Could not notify owner: %s", exc)


# ── Pairing ───────────────────────────────────────────────────────────────────
def _run_pairing() -> bool:
    """
    Initiates the pairing handshake with Machine B.

    Steps:
      1. Generate a 6-digit code and send GLAWMAIL_PAIR_REQUEST:<code> to Machine B
      2. Notify the owner so they know to confirm on the approval bot
      3. Poll for a GLAWMAIL_PAIR_CONFIRM:<code> reply from Machine B's bot
      4. Verify the code matches and the sender is the expected peer bot
      5. Lock the peer bot ID to .pairing

    Returns True if pairing succeeded, False if it timed out.
    """
    code     = generate_pair_code()
    deadline = time.time() + PAIR_CODE_TTL_SECONDS

    logger.info("Starting pairing - sending GLAWMAIL_PAIR_REQUEST to @approvalbot...")
    try:
        _send_to_peer(make_pair_request(code))
    except Exception as exc:
        logger.error("Could not reach @approvalbot to initiate pairing: %s", exc)
        return False

    _notify_owner(
        f"Glawmail pairing started.\n\n"
        f"The approval bot (@approvalbot) will show you a code and ask you to "
        f"type /confirm to complete pairing.\n\n"
        f"This request expires in {PAIR_CODE_TTL_SECONDS // 60} minutes."
    )

    offset = 0
    logger.info("Waiting for GLAWMAIL_PAIR_CONFIRM from @approvalbot...")

    while time.time() < deadline:
        try:
            resp = requests.get(
                f"{OWN_API}/getUpdates",
                params={"offset": offset, "timeout": 10, "allowed_updates": ["message"]},
                timeout=15,
            )
            resp.raise_for_status()
            for update in resp.json().get("result", []):
                offset = update["update_id"] + 1
                msg       = update.get("message", {})
                sender_id = str(msg.get("from", {}).get("id", ""))
                text      = msg.get("text", "")

                parsed = parse_pair_message(text)
                if not parsed:
                    continue
                prefix, recv_code = parsed

                if prefix != PAIR_CONFIRM_PREFIX:
                    continue

                if sender_id != str(APPROVAL_BOT_CHAT_ID):
                    logger.warning(
                        "Pairing confirm from unexpected sender %s - ignoring", sender_id
                    )
                    continue

                if recv_code != code:
                    logger.warning("Pairing confirm code mismatch - ignoring")
                    continue

                record_pairing(sender_id)
                _notify_owner(
                    "Glawmail pairing complete. "
                    "Machine A and Machine B are now linked."
                )
                logger.info("Pairing complete - peer locked to bot ID %s", sender_id)
                return True

        except requests.exceptions.Timeout:
            pass
        except Exception as exc:
            logger.error("Pairing poll error: %s", exc)
            time.sleep(2)

    logger.error("Pairing timed out. Restart both bots to try again.")
    _notify_owner("Glawmail pairing timed out. Restart both bots to try again.")
    return False


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

    Raises RuntimeError if called before pairing is complete.

    Machine B will send back one of:
      - GLAWMAIL_APPROVED:<json>  after Gmail sends successfully
      - GLAWMAIL_DECLINE:<json>   if the owner rejects it
      - GLAWMAIL_ERROR:<json>     if something goes wrong on Machine B
    """
    if not is_paired():
        raise RuntimeError(
            "Glawmail is not paired. Call start() and complete pairing first."
        )

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
    message = f"GLAWMAIL_APPROVAL_REQUEST:{_sign(payload_str)}:{payload_str}"
    _send_to_peer(message)
    logger.info("Approval request sent for %s (callback_id=%s)", to, callback_id)
    return callback_id


# ── Status message handlers ───────────────────────────────────────────────────
def handle_approved_email(callback_id: str, to: str, subject: str,
                          gmail_id: str, metadata: dict):
    """
    Called when Machine B confirms the email was sent via Gmail.
    Replace with your actual business logic.
    """
    logger.info("GLAWMAIL_APPROVED - id=%s | to=%s | subject=%s | gmail_id=%s | metadata=%s",
                callback_id, to, subject, gmail_id, metadata)
    # e.g. db.mark_sent(callback_id, gmail_id)
    # e.g. crm.log_outreach(to, subject)


def handle_declined_email(callback_id: str, to: str, subject: str, metadata: dict):
    """
    Called when the owner taps Decline in Telegram.
    Replace with your actual business logic.
    """
    logger.info("GLAWMAIL_DECLINED - id=%s | to=%s | subject=%s | metadata=%s",
                callback_id, to, subject, metadata)
    # e.g. db.mark_declined(callback_id)
    # e.g. lead_manager.do_not_contact(to)


def handle_error_email(callback_id: str, to: str, subject: str,
                       stage: str, reason: str, metadata: dict):
    """
    Called when Machine B hits an error processing an approval request.

    stage values:
      - "gmail_send"       the Gmail API call failed after the owner approved
      - "telegram_preview" the preview could not be delivered to the owner

    The email has NOT been sent in either case.
    Replace with your actual business logic.
    """
    logger.error("ERROR - id=%s | to=%s | stage=%s | reason=%s | metadata=%s",
                 callback_id, to, stage, reason, metadata)
    # e.g. db.mark_error(callback_id, stage, reason)
    # e.g. retry_queue.enqueue(callback_id)


def _process_status_message(text: str):
    """
    Parse and verify a GLAWMAIL_APPROVED, GLAWMAIL_DECLINE, or GLAWMAIL_ERROR
    message from Machine B's bot.
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
def _poll_updates(ready: Event | None = None):
    """
    Long-polls the Telegram Bot API for incoming messages to @openclawbot.
    Only processes messages from the locked peer bot ID (.pairing file).
    Any message from any other sender is silently dropped.
    Runs in a background thread.
    """
    offset = 0
    logger.info("Polling for Telegram updates on @openclawbot...")
    if ready:
        ready.set()

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
                msg       = update.get("message", {})
                sender_id = str(msg.get("from", {}).get("id", ""))
                text      = msg.get("text", "")

                if not is_trusted_sender(sender_id):
                    if text.startswith("GLAWMAIL_"):
                        logger.warning(
                            "Dropped GLAWMAIL message from untrusted sender %s", sender_id
                        )
                    continue

                if (text.startswith("GLAWMAIL_APPROVED:") or
                        text.startswith("GLAWMAIL_DECLINE:") or
                        text.startswith("GLAWMAIL_ERROR:")):
                    _process_status_message(text)

        except requests.exceptions.Timeout:
            pass
        except Exception as exc:
            logger.error("Polling error: %s - retrying in 5s", exc)
            time.sleep(5)


# ── Start ─────────────────────────────────────────────────────────────────────
def start() -> Thread:
    """
    Ensures pairing is complete, then starts the background update poller.
    Call this once from your AI app before using request_email_approval().
    Blocks until pairing is confirmed if this is the first run.
    """
    if not is_paired():
        logger.info("Not yet paired - starting pairing handshake...")
        if not _run_pairing():
            logger.error("Pairing failed. Exiting.")
            sys.exit(1)

    ready = Event()
    t = Thread(target=_poll_updates, args=(ready,), daemon=True)
    t.start()
    ready.wait()
    logger.info("Glawmail ready.")
    return t


# ── Smoke test ────────────────────────────────────────────────────────────────
if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "test":
        start()
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
        print("Polling for callbacks - press Ctrl+C to stop.")
        while True:
            time.sleep(1)
    else:
        print("Usage: python ai_app.py test")
        print("Or import start() and request_email_approval() into your AI app.")
