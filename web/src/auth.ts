export const ACCESS = "glipz_access_token";
const MFA = "glipz_mfa_token";
const COOKIE_SESSION = "glipz_cookie_session";
const CSRF_COOKIE = "glipz_csrf";
export const COOKIE_AUTH_TOKEN = "__cookie_auth__";

function migrateLegacyAccessToken(): string | null {
  const legacy = localStorage.getItem(ACCESS);
  if (!legacy) return null;
  sessionStorage.setItem(ACCESS, legacy);
  localStorage.removeItem(ACCESS);
  return legacy;
}

function hasCSRFCookie(): boolean {
  if (typeof document === "undefined") return false;
  return document.cookie.split(";").some((part) => part.trim().startsWith(`${CSRF_COOKIE}=`));
}

export function getAccessToken(): string | null {
  return sessionStorage.getItem(ACCESS) ?? migrateLegacyAccessToken() ?? (hasCSRFCookie() || sessionStorage.getItem(COOKIE_SESSION) ? COOKIE_AUTH_TOKEN : null);
}

export function setAccessToken(token = COOKIE_AUTH_TOKEN) {
  if (token && token !== COOKIE_AUTH_TOKEN) {
    sessionStorage.setItem(ACCESS, token);
  } else {
    sessionStorage.setItem(COOKIE_SESSION, "1");
  }
  localStorage.removeItem(ACCESS);
}

export function clearTokens() {
  localStorage.removeItem(ACCESS);
  sessionStorage.removeItem(ACCESS);
  sessionStorage.removeItem(COOKIE_SESSION);
  localStorage.removeItem(MFA);
  sessionStorage.removeItem(MFA);
}

export function getMfaToken(): string | null {
  return sessionStorage.getItem(MFA);
}

export function setMfaToken(token: string) {
  sessionStorage.setItem(MFA, token || "1");
}
