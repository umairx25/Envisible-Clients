from typing import Any
import time
import requests
import json
from uuid import UUID
from .session import write_session, BASE_URL, WAIT_TIME, POLL_DELAY_SECONDS

def wait_for_auth(device_id: str) -> dict[str, Any]:
    """
    Poll the backend until the device session is ready or the wait time expires.
    """
    try:
        UUID(device_id)
    except ValueError as exc:
        raise RuntimeError("Device id must be a valid UUID.") from exc

    url = f"{BASE_URL}/v1/auth/{device_id}"

    deadline = time.monotonic() + WAIT_TIME
    
    while time.monotonic() < deadline:
        try:
            response = requests.get(url, timeout=10)
        except requests.RequestException:
            time.sleep(POLL_DELAY_SECONDS)
            continue

        try:
            payload = response.json()
        except ValueError as exc:
            if response.status_code == 202:
                retry_header = response.headers.get("Retry-After")
                try:
                    delay = max(1, int(retry_header)) if retry_header else POLL_DELAY_SECONDS
                except ValueError:
                    delay = POLL_DELAY_SECONDS
                time.sleep(delay)
                continue

            body_preview = response.text.strip()[:200] or "<empty body>"
            raise RuntimeError(
                f"Auth endpoint returned invalid JSON (status {response.status_code}): {body_preview}"
            ) from exc

        if response.status_code == 200 and payload.get("is_auth"):
            session_blob = payload.get("content")
            if not session_blob:
                raise RuntimeError("Auth endpoint returned an empty session payload.")
            if not isinstance(session_blob, (str, bytes, bytearray, dict)):
                raise RuntimeError("Auth endpoint returned an unsupported session payload.")

            session = (
                json.loads(session_blob)
                if isinstance(session_blob, (str, bytes, bytearray))
                else session_blob
            )

            write_session(session)
            return session

        if response.status_code == 202:
            retry_header = response.headers.get("Retry-After")
            try:
                delay = max(1, int(retry_header)) if retry_header else POLL_DELAY_SECONDS
            except ValueError:
                delay = POLL_DELAY_SECONDS
            time.sleep(delay)
            continue

        detail = payload.get("detail") if isinstance(payload, dict) else response.text
        status_code = response.status_code
        raise RuntimeError(f"Auth endpoint failed ({status_code}): {detail}")
