from uuid import uuid4
from pathlib import Path
from typing import Any
import json
import os
import sys
import time
import webbrowser

import requests
from termcolor import colored

SESSION_PATH = Path.home() / ".envis" / "session.json"
REQUIRED_SESSION_KEYS = ("access_token", "refresh_token")

def printc(text: str, color: str = "white"):
    print(colored(text, color))

def _get_env_url(var_name: str, default: str) -> str:
    """
    Read a URL override from environment variables with a safe fallback.
    Trailing slashes are stripped so joins don't produce double slashes.
    """
    value = os.getenv(var_name)
    if not value:
        return default
    return value.rstrip("/")

BASE_URL = _get_env_url("ENVIS_API_URL", "https://api.envisible.dev")
FRONTEND_URL = _get_env_url("ENVIS_DASH_URL", "https://envisible.dev")
WAIT_TIME = 120
POLL_DELAY_SECONDS = 5

def _should_refresh(session: dict[str, Any]) -> bool:
    """
    Determine whether the cached session is within the refresh window.
    """
    expires_at = session.get("expires_at")
    if expires_at is None:
        return False

    try:
        expiry_ts = int(expires_at)
    except (TypeError, ValueError):
        return False

    # Refresh one minute before expiry to avoid race conditions.
    return time.time() >= (expiry_ts - 60)

def _invalidate_session_cache() -> None:
    """
    Remove the cached session so a fresh login can be initiated.
    """
    try:
        SESSION_PATH.unlink()
    except FileNotFoundError:
        return
    except OSError as exc:
        raise RuntimeError(f"Failed to delete invalid session cache: {exc}") from exc

def _validate_session_payload(session: dict[str, Any]) -> None:
    """
    Ensure the cached payload contains the bare minimum fields we rely on.
    """
    if not isinstance(session, dict):
        raise RuntimeError("Session payload must be a dictionary.")

    missing = [key for key in REQUIRED_SESSION_KEYS if not session.get(key)]
    if missing:
        raise RuntimeError(
            f"Session payload missing required field(s): {', '.join(missing)}."
        )

def refresh_session(session: dict[str, Any]) -> dict[str, Any]:
    """
    Call the backend refresh endpoint to obtain a fresh Supabase session.
    """
    refresh_token = session.get("refresh_token")
    
    if not refresh_token:
        raise RuntimeError("Session expired and no refresh token is available. Re-run `envis login`.")

    url = f"{BASE_URL}/v1/auth/refresh"
    payload = {"refresh_token": refresh_token}

    try:
        response = requests.post(url, json=payload, timeout=10)
        response.raise_for_status()
    except requests.HTTPError as exc:
        detail = exc.response.text if exc.response is not None else str(exc)
        raise RuntimeError(f"Failed to refresh session ({exc.response.status_code if exc.response else 'HTTP error'}): {detail}") from exc
    except requests.RequestException as exc:
        raise RuntimeError(f"Failed to reach Envisible API for session refresh: {exc}") from exc

    try:
        new_session = response.json()
    except ValueError as exc:
        raise RuntimeError("Refresh endpoint returned invalid JSON.") from exc

    write_session(new_session)
    return new_session


def load_session() -> dict:
    """
    Load the cached session tokens that the CLI stored after login.
    """

    if not SESSION_PATH.exists():
        device_code = uuid4()
        raise RuntimeError(
            f"Not authenticated. Visit {FRONTEND_URL}/auth?device_code={device_code} to link this device."
        )

    try:
        session = json.loads(SESSION_PATH.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise RuntimeError("Session file is corrupt. Re-run `envis login`.") from exc

    needs_refresh = _should_refresh(session) or (
        not session.get("access_token") and session.get("refresh_token")
    )

    if needs_refresh:
        session = refresh_session(session)

    try:
        _validate_session_payload(session)
    except RuntimeError as exc:
        _invalidate_session_cache()
        raise RuntimeError(
            f"Session cache invalid even after attempting refresh: {exc}"
        ) from exc

    return session


def write_session(session_info: dict[str, Any]) -> None:
    """
    Persist the Supabase session locally so future calls can reuse it.
    """
    _validate_session_payload(session_info)

    try:
        payload = json.dumps(session_info, indent=2)
    except (TypeError, ValueError) as exc:
        raise RuntimeError("Session payload is not JSON serializable.") from exc

    try:
        SESSION_PATH.parent.mkdir(parents=True, exist_ok=True)
        SESSION_PATH.write_text(payload, encoding="utf-8")
        os.chmod(SESSION_PATH, 0o600)
    except OSError as exc:
        raise RuntimeError(f"Failed to write session cache: {exc}") from exc

def _is_headless() -> bool:
    """
    Return True when interactive auth should not be attempted.
    """
    if os.getenv("ENVIS_CI_TOKEN"):
        return True
    try:
        return not (sys.stdin.isatty() and sys.stdout.isatty())
    except Exception:
        return True

def ensure_session() -> None:

    from .auth import wait_for_auth

    if SESSION_PATH.exists():
        return

    if _is_headless():
        raise RuntimeError(
            "No cached session and headless environment detected. "
            "Set ENVIS_CI_TOKEN or run `envis login` on a machine with a browser "
        )

    device_code = str(uuid4())
    auth_url = f"{FRONTEND_URL}/auth?device_code={device_code}"

    printc("No cached session detected.", "red")

    try:
        webbrowser.open(auth_url, new=2)
        print(f"\nTrying to open:")
        printc(f"        {auth_url}", "blue")
    except Exception:
        printc("Could not open browser automatically – open \n {auth_url}", "red")

    print("\nWaiting for the session to be approved...")
    wait_for_auth(device_code)
    printc("\nDevice approved and session saved locally.", "green")

    
