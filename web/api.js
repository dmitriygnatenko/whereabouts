/* =========================================================================
   API CLIENT
   Talks to the real Go backend (see the repo root). If the frontend is
   served from a different host than the backend, change API_BASE below.
   ========================================================================= */
const API_BASE = '/api';

// Called on a 401 from any request — e.g. the session expired in another tab. The app subscribes to
// this in mounted() (see app.js).
let unauthorizedHandler = null;
// onUnauthorized registers fn as the handler invoked whenever a request comes back with a 401.
function onUnauthorized(fn) { unauthorizedHandler = fn; }

// handleResponse parses a fetch Response into JSON, firing the unauthorizedHandler on a 401 and
// throwing a localized Error for any other non-OK status.
async function handleResponse(res) {
  if (res.status === 204) return null;
  let data = null;
  try { data = await res.json(); } catch (e) { /* empty response body */ }
  if (res.status === 401 && unauthorizedHandler) {
    unauthorizedHandler();
  }
  if (!res.ok) {
    const raw = (data && data.error) ? data.error : translate(apiLocale, 'Request error ({status})', { status: res.status });
    throw new Error(localizeServerMessage(apiLocale, raw));
  }
  return data;
}

// apiFetch issues a same-origin request against the backend (sending the httpOnly session cookie)
// and returns its parsed JSON body.
async function apiFetch(path, options) {
  let res;
  try {
    res = await fetch(API_BASE + path, {
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin', // send the httpOnly session cookie
      ...options,
    });
  } catch (e) {
    throw new Error(translate(apiLocale, 'Backend unavailable. Check that the Go server is running.'));
  }
  return handleResponse(res);
}

const api = {
  listItems() {
    return apiFetch('/items');
  },
  createItem(payload) {
    return apiFetch('/items', { method: 'POST', body: JSON.stringify(payload) });
  },
  updateItem(id, payload) {
    return apiFetch('/items/' + encodeURIComponent(id), { method: 'PUT', body: JSON.stringify(payload) });
  },
  deleteItem(id) {
    return apiFetch('/items/' + encodeURIComponent(id), { method: 'DELETE' });
  },
  listLocations() {
    return apiFetch('/locations');
  },
  createLocation(payload) {
    return apiFetch('/locations', { method: 'POST', body: JSON.stringify(payload) });
  },
  updateLocation(id, payload) {
    return apiFetch('/locations/' + encodeURIComponent(id), { method: 'PUT', body: JSON.stringify(payload) });
  },
  deleteLocation(id) {
    return apiFetch('/locations/' + encodeURIComponent(id), { method: 'DELETE' });
  },
};

/* =========================================================================
   AUTH
   Talks to the real backend: /api/auth/login, /api/auth/logout, /api/auth/me.
   The session is an httpOnly cookie set by the server; the client never
   reads or stores it directly, just relies on credentials: 'same-origin'.
   ========================================================================= */
const authApi = {
  async login({ username, password, language }) {
    return apiFetch('/auth/login', { method: 'POST', body: JSON.stringify({ username, password, language }) });
  },
  async logout() {
    return apiFetch('/auth/logout', { method: 'POST' });
  },
  async me() {
    return apiFetch('/auth/me');
  },
};

/* =========================================================================
   USER
   Profile settings for the signed-in user: /api/user/language,
   /api/user/location-filter-depth, /api/user/username, /api/user/password.
   ========================================================================= */
const userApi = {
  async updateLanguage(language) {
    return apiFetch('/user/language', { method: 'PATCH', body: JSON.stringify({ language }) });
  },
  async updateLocationFilterDepth(locationFilterDepth) {
    return apiFetch('/user/location-filter-depth', { method: 'PATCH', body: JSON.stringify({ locationFilterDepth }) });
  },
  async updateUsername({ username, currentPassword }) {
    return apiFetch('/user/username', { method: 'PATCH', body: JSON.stringify({ username, currentPassword }) });
  },
  async changePassword({ currentPassword, newPassword }) {
    return apiFetch('/user/password', { method: 'PATCH', body: JSON.stringify({ currentPassword, newPassword }) });
  },
};
