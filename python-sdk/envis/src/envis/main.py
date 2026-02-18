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

def _build_headers() -> dict:
    ci_token = os.getenv("ENVIS_CI_TOKEN")
    headers = {
        "Content-Type": "application/json",
    }
    if ci_token:
        headers["X-CI-Token"] = ci_token
        return headers

    ensure_session()
    session = load_session()
    headers["Authorization"] = f"Bearer {session['access_token']}"
    if _is_local_base_url(BASE_URL):
        user_id = _extract_user_id(session)
        if user_id:
            headers["X-User-Id"] = user_id
    return headers

def _parse_json_response(resp: requests.Response, error_context: str) -> dict:
    try:
        return resp.json()
    except ValueError:
        text = resp.text.strip()
        if text:
            return {"raw": text}
        raise RuntimeError(f"API returned an empty, non-JSON response when {error_context}.")

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
    headers = _build_headers()

    url = f"{BASE_URL}/v1/projects/{project_id}/secrets/{secret_name}"
    try:
        resp = requests.get(url, headers=headers, timeout=10)
        resp.raise_for_status()
    except requests.HTTPError as exc:
        detail = exc.response.text if exc.response is not None else str(exc)
        raise RuntimeError(f"Failed to fetch secret ({exc.response.status_code if exc.response else 'HTTP error'}): {detail}") from exc
    except requests.RequestException as exc:
        raise RuntimeError(f"Failed to reach Envault API: {exc}") from exc

    res = _parse_json_response(resp, "fetching secret")
    if isinstance(res, dict) and "value" in res:
        return res["value"]
    return res

def get_many(project_id: str, secret_names: list[str]) -> list[dict]:
    """
    Fetch multiple secrets in a single request using the batch endpoint.
    """
    if not isinstance(secret_names, list):
        raise RuntimeError("secret_names must be a list of strings.")

    names = [name.strip() for name in secret_names if isinstance(name, str) and name.strip()]
    if not names:
        raise RuntimeError("secret_names must include at least one non-empty name.")

    headers = _build_headers()
    url = f"{BASE_URL}/v1/projects/{project_id}/secrets/batch"

    try:
        resp = requests.post(url, headers=headers, json={"names": names}, timeout=10)
        resp.raise_for_status()
    except requests.HTTPError as exc:
        detail = exc.response.text if exc.response is not None else str(exc)
        raise RuntimeError(f"Failed to fetch secrets ({exc.response.status_code if exc.response else 'HTTP error'}): {detail}") from exc
    except requests.RequestException as exc:
        raise RuntimeError(f"Failed to reach Envault API: {exc}") from exc

    res = _parse_json_response(resp, "fetching secrets")
    if isinstance(res, dict) and "secrets" in res:
        return res["secrets"]
    return res

def get_all(project_id: str) -> list[dict]:
    """
    Fetch all secrets for a project in a single request.
    """
    headers = _build_headers()
    url = f"{BASE_URL}/v1/projects/{project_id}/secrets/all"

    try:
        resp = requests.get(url, headers=headers, timeout=10)
        resp.raise_for_status()
    except requests.HTTPError as exc:
        detail = exc.response.text if exc.response is not None else str(exc)
        raise RuntimeError(f"Failed to fetch secrets ({exc.response.status_code if exc.response else 'HTTP error'}): {detail}") from exc
    except requests.RequestException as exc:
        raise RuntimeError(f"Failed to reach Envault API: {exc}") from exc

    res = _parse_json_response(resp, "fetching secrets")
    if isinstance(res, dict) and "secrets" in res:
        return res["secrets"]
    return res
