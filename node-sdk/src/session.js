const fs = require("fs");
const path = require("path");
const os = require("os");
const crypto = require("crypto");

const chalk = require("chalk");
const fetch = require("node-fetch");

const SESSION_PATH = path.join(os.homedir(), ".envis", "session.json");
const REQUIRED_SESSION_KEYS = ["access_token", "refresh_token"];
const WAIT_TIME = 120; // seconds
const POLL_DELAY_SECONDS = 5;

function printc(text, color = "white") {
  const colorFn = chalk[color];
  if (typeof colorFn === "function") {
    console.log(colorFn(text));
    return;
  }
  console.log(text);
}

function getEnvUrl(varName, fallback) {
  const raw = process.env[varName];
  if (!raw) {
    return fallback;
  }
  return raw.replace(/\/+$/, "");
}

const BASE_URL = getEnvUrl(
  "ENVIS_API_URL",
  "https://api.envisible.dev"
);

const FRONTEND_URL = getEnvUrl(
  "ENVIS_DASH_URL",
  "https://envisible.dev"
);

async function pathExists(targetPath) {
  try {
    await fs.promises.access(targetPath, fs.constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

function shouldRefresh(session) {
  const expiresAt = session?.expires_at;
  if (!expiresAt) {
    return false;
  }
  const expiryTs = Number(expiresAt);
  if (!Number.isFinite(expiryTs)) {
    return false;
  }
  const nowSeconds = Math.floor(Date.now() / 1000);
  return nowSeconds >= expiryTs - 60;
}

async function invalidateSessionCache() {
  try {
    await fs.promises.unlink(SESSION_PATH);
  } catch (error) {
    if (error && error.code === "ENOENT") {
      return;
    }
    throw new Error(
      `Failed to delete invalid session cache: ${error?.message || error}`
    );
  }
}

function validateSessionPayload(session) {
  if (typeof session !== "object" || session === null) {
    throw new Error("Session payload must be an object.");
  }
  const missing = REQUIRED_SESSION_KEYS.filter(
    (key) => !session[key]
  );
  if (missing.length) {
    throw new Error(
      `Session payload missing required field(s): ${missing.join(", ")}.`
    );
  }
}

async function writeSession(sessionInfo) {
  validateSessionPayload(sessionInfo);

  let payload;
  try {
    payload = JSON.stringify(sessionInfo, null, 2);
  } catch (error) {
    throw new Error("Session payload is not JSON serializable.");
  }

  try {
    await fs.promises.mkdir(path.dirname(SESSION_PATH), {
      recursive: true,
    });
    await fs.promises.writeFile(SESSION_PATH, payload, {
      encoding: "utf-8",
    });
    await fs.promises.chmod(SESSION_PATH, 0o600);
  } catch (error) {
    throw new Error(`Failed to write session cache: ${error.message}`);
  }
}

async function refreshSession(session) {
  const refreshToken = session?.refresh_token;
  if (!refreshToken) {
    throw new Error(
      "Session expired and no refresh token is available. Re-run `envis login`."
    );
  }

  const url = `${BASE_URL}/v1/auth/refresh`;
  let response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
      timeout: 10000,
    });
  } catch (error) {
    throw new Error(
      `Failed to reach Envisible API for session refresh: ${error.message}`
    );
  }

  if (!response.ok) {
    let detail = "";
    try {
      detail = await response.text();
    } catch (error) {
      detail = error.message;
    }
    throw new Error(
      `Failed to refresh session (${response.status || "HTTP error"}): ${detail}`
    );
  }

  let newSession;
  try {
    newSession = await response.json();
  } catch (error) {
    throw new Error("Refresh endpoint returned invalid JSON.");
  }

  await writeSession(newSession);
  return newSession;
}

async function loadSession() {
  if (!(await pathExists(SESSION_PATH))) {
    const deviceCode = crypto.randomUUID();
    throw new Error(
      `Not authenticated. Visit ${FRONTEND_URL}/auth?device_code=${deviceCode} to link this device.`
    );
  }

  let raw;
  try {
    raw = await fs.promises.readFile(SESSION_PATH, {
      encoding: "utf-8",
    });
  } catch (error) {
    throw new Error("Unable to read session file. Re-run `envis login`.");
  }

  let session;
  try {
    session = JSON.parse(raw);
  } catch (error) {
    throw new Error("Session file is corrupt. Re-run `envis login`.");
  }

  let hydrated = session;
  const needsRefresh =
    shouldRefresh(session) ||
    (!session.access_token && session.refresh_token);

  if (needsRefresh) {
    hydrated = await refreshSession(session);
  }

  try {
    validateSessionPayload(hydrated);
  } catch (error) {
    await invalidateSessionCache();
    throw new Error(
      `Session cache invalid even after attempting refresh: ${error.message}`
    );
  }

  return hydrated;
}

async function ensureSession() {
  if (await pathExists(SESSION_PATH)) {
    return;
  }

  const { waitForAuth } = require("./auth");
  let openBrowser;
  try {
    openBrowser = require("open");
  } catch {
    openBrowser = null;
  }

  const deviceCode = crypto.randomUUID();
  const authUrl = `${FRONTEND_URL}/auth?device_code=${deviceCode}`;

  printc("No cached session detected.", "red");

  if (openBrowser) {
    try {
      await openBrowser(authUrl, { wait: false });
      console.log("\nTrying to open:");
      printc(`        ${authUrl}`, "blue");
    } catch {
      printc(
        `Could not open browser automatically – open ${authUrl}`,
        "red"
      );
    }
  } else {
    printc(
      `Open the following URL in your browser to link this device:\n${authUrl}`,
      "blue"
    );
  }

  console.log("\nWaiting for the session to be approved...");
  await waitForAuth(deviceCode);
  printc("\nDevice approved and session saved locally.", "green");
}

module.exports = {
  BASE_URL,
  FRONTEND_URL,
  WAIT_TIME,
  POLL_DELAY_SECONDS,
  SESSION_PATH,
  REQUIRED_SESSION_KEYS,
  printc,
  loadSession,
  writeSession,
  ensureSession,
  refreshSession,
  invalidateSessionCache,
};
