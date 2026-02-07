const fetch = require("node-fetch");

const {
  BASE_URL,
  WAIT_TIME,
  POLL_DELAY_SECONDS,
  writeSession,
} = require("./session");

const SECOND = 1000;

function sleep(seconds) {
  return new Promise((resolve) => setTimeout(resolve, seconds * SECOND));
}

function parseRetryHeader(retryHeader) {
  if (!retryHeader) {
    return POLL_DELAY_SECONDS;
  }
  const parsed = Number(retryHeader);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return POLL_DELAY_SECONDS;
  }
  return Math.max(1, Math.floor(parsed));
}

async function waitForAuth(deviceId) {

  const url = `${BASE_URL}/v1/auth/${deviceId}`;
  const deadline = Date.now() + WAIT_TIME * SECOND;

  while (Date.now() < deadline) {
    let response;
    try {
      response = await fetch(url, { timeout: 10000 });
    } catch {
      await sleep(POLL_DELAY_SECONDS);
      continue;
    }

    let bodyText = "";
    try {
      bodyText = await response.text();
    } catch {
      bodyText = "";
    }

    let payload = null;
    if (bodyText) {
      try {
        payload = JSON.parse(bodyText);
      } catch (error) {
        if (response.status === 202) {
          await sleep(parseRetryHeader(response.headers.get("retry-after")));
          continue;
        }
        const preview =
          bodyText.trim().slice(0, 200) || "<empty body>";
        throw new Error(
          `Auth endpoint returned invalid JSON (status ${response.status}): ${preview}`
        );
      }
    }

    if (
      response.status === 200 &&
      payload &&
      typeof payload === "object" &&
      payload.is_auth
    ) {
      const sessionBlob = payload.content;
      if (!sessionBlob) {
        throw new Error(
          "Auth endpoint returned an empty session payload."
        );
      }

      let session;
      if (
        typeof sessionBlob === "string" ||
        sessionBlob instanceof String
      ) {
        try {
          session = JSON.parse(sessionBlob);
        } catch (error) {
          throw new Error(
            "Auth endpoint returned malformed session payload."
          );
        }
      } else if (typeof sessionBlob === "object") {
        session = sessionBlob;
      } else {
        throw new Error(
          "Auth endpoint returned an unsupported session payload."
        );
      }

      await writeSession(session);
      return session;
    }

    if (response.status === 202) {
      await sleep(parseRetryHeader(response.headers.get("retry-after")));
      continue;
    }

    const detail =
      payload && typeof payload === "object"
        ? payload.detail || bodyText || "<empty body>"
        : bodyText || "<empty body>";

    throw new Error(
      `Auth endpoint failed (${response.status}): ${detail}`
    );
  }

  throw new Error("Timed out waiting for device approval. Please try again.");
}

module.exports = {
  waitForAuth,
};
