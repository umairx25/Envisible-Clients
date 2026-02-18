const { get, getMany, getAll, logout } = require("./main");
const {
  ensureSession,
  loadSession,
  refreshSession,
  SESSION_PATH,
  BASE_URL,
  FRONTEND_URL,
} = require("./session");

const envis = {
  get,
  getMany,
  getAll,
  logout,
  ensureSession,
  loadSession,
  refreshSession,
  SESSION_PATH,
  BASE_URL,
  FRONTEND_URL,
};

module.exports = envis;
module.exports.default = envis;
