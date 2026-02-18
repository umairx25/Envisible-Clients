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

async function buildAuthHeaders() {
  await ensureSession();
  const session = await loadSession();
  return {
    Authorization: `Bearer ${session.access_token}`,
    "Content-Type": "application/json",
  };
}

async function fetchJSON(url, options, errorAction, errorContext) {
  let response;
  try {
    response = await fetch(url, { timeout: 10000, ...options });
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
      `Failed to ${errorAction} (${response.status || "HTTP error"}): ${
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
      `API returned an empty, non-JSON response when ${errorContext || errorAction}.`
    );
  }

  try {
    return JSON.parse(bodyText);
  } catch {
    const raw = bodyText.trim();
    if (raw) {
      return { raw };
    }
    throw new Error(
      `API returned an empty, non-JSON response when ${errorContext || errorAction}.`
    );
  }
}

async function get(projectId, secretName) {
  if (!projectId || !secretName) {
    throw new Error("Both project id and secret name are required.");
  }

  const headers = await buildAuthHeaders();

  const url = `${BASE_URL}/v1/projects/${encodeURIComponent(
    projectId
  )}/secrets/${encodeURIComponent(secretName)}`;

  const payload = await fetchJSON(
    url,
    { headers },
    "fetch secret",
    "fetching secret"
  );
  if (
    payload &&
    typeof payload === "object" &&
    Object.prototype.hasOwnProperty.call(payload, "value")
  ) {
    return payload.value;
  }
  return payload;
}

async function getMany(projectId, secretNames) {
  if (!projectId || !Array.isArray(secretNames)) {
    throw new Error(
      "Project id and an array of secret names are required."
    );
  }
  const names = secretNames
    .map((name) => (typeof name === "string" ? name.trim() : ""))
    .filter(Boolean);
  if (names.length === 0) {
    throw new Error("At least one secret name is required.");
  }

  const headers = await buildAuthHeaders();
  const url = `${BASE_URL}/v1/projects/${encodeURIComponent(
    projectId
  )}/secrets/batch`;

  const payload = await fetchJSON(
    url,
    {
      method: "POST",
      headers,
      body: JSON.stringify({ names }),
    },
    "fetch secrets",
    "fetching secrets"
  );

  if (
    payload &&
    typeof payload === "object" &&
    Object.prototype.hasOwnProperty.call(payload, "secrets")
  ) {
    return payload.secrets;
  }
  return payload;
}

async function getAll(projectId) {
  if (!projectId) {
    throw new Error("Project id is required.");
  }

  const headers = await buildAuthHeaders();
  const url = `${BASE_URL}/v1/projects/${encodeURIComponent(
    projectId
  )}/secrets/all`;

  const payload = await fetchJSON(
    url,
    { headers },
    "fetch secrets",
    "fetching secrets"
  );
  if (
    payload &&
    typeof payload === "object" &&
    Object.prototype.hasOwnProperty.call(payload, "secrets")
  ) {
    return payload.secrets;
  }
  return payload;
}

module.exports = {
  get,
  getMany,
  getAll,
  logout,
};
