"""Rate limiting, input validation, and CORS helpers."""

from __future__ import annotations

import re
import time
import unicodedata
from collections import defaultdict

from aiohttp import web as aiohttp_web

# Matrix room ID pattern: !<localpart>:<server>
_ROOM_ID_RE = re.compile(r"^![A-Za-z0-9._=\-/]+:[A-Za-z0-9.\-]+(:[0-9]+)?$")


class RateLimiter:
    """Sliding-window per-key rate limiter (in-memory).

    Tracks timestamps of recent requests per key (typically an IP address).
    Not persistent across restarts, which is acceptable for short windows.
    """

    def __init__(self, max_per_minute: int = 5) -> None:
        self.max_per_minute = max_per_minute
        self._windows: dict[str, list[float]] = defaultdict(list)

    def check(self, key: str) -> bool:
        """Return True if the request is allowed, False if rate-limited.

        If allowed, the current timestamp is recorded.
        """
        now = time.time()
        window = [t for t in self._windows[key] if now - t < 60]
        self._windows[key] = window
        if len(window) >= self.max_per_minute:
            return False
        self._windows[key].append(now)
        return True

    def cleanup(self) -> None:
        """Remove stale keys to prevent unbounded memory growth."""
        now = time.time()
        empty_keys = [
            k for k, v in self._windows.items()
            if not any(now - t < 60 for t in v)
        ]
        for k in empty_keys:
            del self._windows[k]


def validate_room_id(room_id: str) -> str | None:
    """Validate a Matrix room ID.

    Returns an error message string if invalid, or None if valid.
    """
    if not room_id:
        return "room_id is required"
    if not _ROOM_ID_RE.match(room_id):
        return "Invalid room_id format"
    return None


def validate_display_name(name: str | None) -> tuple[str | None, str | None]:
    """Validate and sanitize a guest display name.

    Returns (sanitized_name, error_message).  If error_message is not None
    the name was rejected.
    """
    if not name:
        return None, "display_name is required"

    # Strip control characters (Cc category in Unicode)
    sanitized = "".join(
        ch for ch in name if unicodedata.category(ch) != "Cc"
    ).strip()

    if not sanitized:
        return None, "display_name must contain visible characters"
    if len(sanitized) > 50:
        return None, "display_name must be 50 characters or fewer"

    return sanitized, None


def add_cors_headers(
    response: aiohttp_web.Response,
    allowed_origins: str,
) -> aiohttp_web.Response:
    """Add CORS headers to *response* in-place and return it."""
    response.headers["Access-Control-Allow-Origin"] = allowed_origins
    response.headers["Access-Control-Allow-Methods"] = "POST, GET, OPTIONS"
    response.headers["Access-Control-Allow-Headers"] = "Content-Type"
    return response


def cors_preflight(allowed_origins: str) -> aiohttp_web.Response:
    """Return a 204 response for CORS OPTIONS preflight."""
    return aiohttp_web.Response(
        status=204,
        headers={
            "Access-Control-Allow-Origin": allowed_origins,
            "Access-Control-Allow-Methods": "POST, GET, OPTIONS",
            "Access-Control-Allow-Headers": "Content-Type",
            "Access-Control-Max-Age": "86400",
        },
    )
