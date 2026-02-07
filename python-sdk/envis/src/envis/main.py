from urllib.parse import urlparse
import os
import requests
from dotenv import load_dotenv, find_dotenv
from .session import load_session, ensure_session, printc, SESSION_PATH, BASE_URL

# Load .env from the current working directory (or its parents) instead of the package location.
load_dotenv(find_dotenv(usecwd=True))

"""
Helpers
"""

def _extract_user_id(session: dict) -> str | None:
    """
    Return the Supabase user ID from the cached session, if present.
    """
    user = session.get("user")
    if isinstance(user, dict):
        user_id = user.get("id")
        if isinstance(user_id, str) and user_id.strip():
            return user_id
    return None


def _is_local_base_url(base_url: str) -> bool:
    """
    Determine if the API URL is targeting a local dev server.
    """
    parsed = urlparse(base_url)
    host = parsed.hostname or ""
    return host in {"localhost", "127.0.0.1", "0.0.0.0"}

"""
Main functions
"""

def logout() -> None:
    """
    Remove the cached session so the next call forces re-authentication.
    """
    if not SESSION_PATH.exists():
        raise RuntimeError("Already logged out. Please authenticate again.")

    try:
        SESSION_PATH.unlink()
        printc("Successfully logged out!", "green")
    except OSError as exc:
        raise RuntimeError(f"Error loggin out") from exc
    

def get(project_id: str, secret_name: str) -> dict:
    """
    Fetch a secret value, enforcing that the caller has an authenticated session.
    """
    ci_token = os.getenv("ENVIS_CI_TOKEN")
    if not ci_token:
        ensure_session()
        session = load_session()
    headers = {
        "Content-Type": "application/json",
    }
    if ci_token:
        headers["X-CI-Token"] = ci_token
    else:
        headers["Authorization"] = f"Bearer {session['access_token']}"
        if _is_local_base_url(BASE_URL):
            user_id = _extract_user_id(session)
            if user_id:
                headers["X-User-Id"] = user_id

    url = f"{BASE_URL}/v1/projects/{project_id}/secrets/{secret_name}"
    try:
        resp = requests.get(url, headers=headers, timeout=10)
        resp.raise_for_status()
    except requests.HTTPError as exc:
        detail = exc.response.text if exc.response is not None else str(exc)
        raise RuntimeError(f"Failed to fetch secret ({exc.response.status_code if exc.response else 'HTTP error'}): {detail}") from exc
    except requests.RequestException as exc:
        raise RuntimeError(f"Failed to reach Envault API: {exc}") from exc

    try:
        res= resp.json()
        return res["value"]
    except ValueError:
        text = resp.text.strip()
        if text:
            return {"raw": text}
        raise RuntimeError("API returned an empty, non-JSON response when fetching secret.")
