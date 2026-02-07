const fs = require("fs");
const { URL } = require("url");

const fetch = require("node-fetch");

const {
  ensureSession,
  loadSession,
  printc,
  SESSION_PATH,
  BASE_URL,
} = require("./session");

async function logout() {
  try {
    await fs.promises.access(SESSION_PATH, fs.constants.F_OK);
  } catch {
    throw new Error(
      "Already logged out. Please authenticate again."
    );
  }

  try {
    await fs.promises.unlink(SESSION_PATH);
    printc("Successfully logged out!", "green");
  } catch (error) {
    throw new Error(`Error logging out: ${error.message}`);
  }
}

async function get(projectId, secretName) {
  if (!projectId || !secretName) {
    throw new Error("Both project id and secret name are required.");
  }

  await ensureSession();
  const session = await loadSession();

  const headers = {
    Authorization: `Bearer ${session.access_token}`,
    "Content-Type": "application/json",
  };

  const url = `${BASE_URL}/v1/projects/${encodeURIComponent(
    projectId
  )}/secrets/${encodeURIComponent(secretName)}`;

  let response;
  try {
    response = await fetch(url, { headers, timeout: 10000 });
  } catch (error) {
    throw new Error(`Failed to reach Envault API: ${error.message}`);
  }

  if (!response.ok) {
    let detail;
    try {
      detail = (await response.text()).trim();
    } catch (error) {
      detail = error.message;
    }
    throw new Error(
      `Failed to fetch secret (${response.status || "HTTP error"}): ${
        detail || "No response body provided."
      }`
    );
  }

  let bodyText = "";
  try {
    bodyText = await response.text();
  } catch {
    bodyText = "";
  }

  if (!bodyText) {
    throw new Error(
      "API returned an empty, non-JSON response when fetching secret."
    );
  }

  try {
    const payload = JSON.parse(bodyText);
    if (
      payload &&
      typeof payload === "object" &&
      Object.prototype.hasOwnProperty.call(payload, "value")
    ) {
      return payload.value;
    }
    return payload;
  } catch {
    const raw = bodyText.trim();
    if (raw) {
      return { raw };
    }
    throw new Error(
      "API returned an empty, non-JSON response when fetching secret."
    );
  }
}

module.exports = {
  get,
  logout,
};
