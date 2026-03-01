"""
pairing.py - Glawmail bot pairing protocol
===========================================
Handles the one-time pairing handshake that locks each bot to its
expected counterpart before any operational messages are processed.

How it works:

  Machine A (ai_app.py / @openclawbot):
    1. On startup, sends a GLAWMAIL_PAIR_REQUEST:<code> message to Machine B's bot
    2. Waits for a GLAWMAIL_PAIR_CONFIRM:<code> message back from Machine B's bot
    3. Once confirmed, locks the peer bot ID and writes it to .pairing

  Machine B (approval_bot.py / @approvalbot):
    1. On startup, waits to receive a GLAWMAIL_PAIR_REQUEST:<code> from Machine A
    2. Displays the code to the owner in Telegram for manual confirmation
    3. Owner types /confirm in the approval bot chat
    4. Sends GLAWMAIL_PAIR_CONFIRM:<code> back to Machine A's bot
    5. Locks the peer bot ID and writes it to .pairing

After pairing:
  - Both bots persist the verified peer bot ID in .pairing (mode 600)
  - Any message from any sender other than the locked peer is silently dropped
  - Re-pairing requires deleting .pairing and restarting both bots

Security properties:
  - Codes are 6-digit random values, valid for PAIR_CODE_TTL_SECONDS only
  - The owner must physically confirm the code in Telegram (human in the loop)
  - Peer bot IDs are verified against the confirmed code before locking
  - .pairing file is mode 600 and excluded from version control
"""

import json
import logging
import os
import random
import time
from pathlib import Path

logger = logging.getLogger(__name__)

PAIR_CODE_TTL_SECONDS = 300   # Code expires after 5 minutes
PAIRING_FILE          = ".pairing"
PAIR_REQUEST_PREFIX   = "GLAWMAIL_PAIR_REQUEST"
PAIR_CONFIRM_PREFIX   = "GLAWMAIL_PAIR_CONFIRM"


# ── Persistence ───────────────────────────────────────────────────────────────
def load_pairing(path: str = PAIRING_FILE) -> dict:
    p = Path(path)
    if not p.exists():
        return {}
    try:
        return json.loads(p.read_text())
    except (json.JSONDecodeError, OSError):
        return {}


def save_pairing(data: dict, path: str = PAIRING_FILE):
    p = Path(path)
    p.write_text(json.dumps(data))
    p.chmod(0o600)


def is_paired(path: str = PAIRING_FILE) -> bool:
    data = load_pairing(path)
    return bool(data.get("peer_bot_id") and data.get("paired_at"))


def get_peer_bot_id(path: str = PAIRING_FILE) -> str | None:
    return load_pairing(path).get("peer_bot_id")


def record_pairing(peer_bot_id: str, path: str = PAIRING_FILE):
    save_pairing({
        "peer_bot_id": str(peer_bot_id),
        "paired_at":   time.time(),
    }, path)
    logger.info("Pairing recorded - peer_bot_id=%s", peer_bot_id)


# ── Code generation ───────────────────────────────────────────────────────────
def generate_pair_code() -> str:
    """6-digit numeric code, zero-padded."""
    return f"{random.SystemRandom().randint(0, 999999):06d}"


def make_pair_request(code: str) -> str:
    return f"{PAIR_REQUEST_PREFIX}:{code}"


def make_pair_confirm(code: str) -> str:
    return f"{PAIR_CONFIRM_PREFIX}:{code}"


def parse_pair_message(text: str) -> tuple[str, str] | None:
    """
    Returns (prefix, code) if the message is a valid pairing message,
    otherwise None.
    """
    for prefix in (PAIR_REQUEST_PREFIX, PAIR_CONFIRM_PREFIX):
        if text.startswith(f"{prefix}:"):
            _, _, code = text.partition(":")
            if code.isdigit() and len(code) == 6:
                return prefix, code
    return None


# ── Sender verification ───────────────────────────────────────────────────────
def is_trusted_sender(sender_id: str, path: str = PAIRING_FILE) -> bool:
    """
    Returns True only if:
      - Pairing is complete, AND
      - sender_id matches the locked peer bot ID exactly
    """
    peer = get_peer_bot_id(path)
    if not peer:
        return False
    return str(sender_id) == str(peer)
