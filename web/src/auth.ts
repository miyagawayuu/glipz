export const ACCESS = "glipz_access_token";
const MFA = "glipz_mfa_token";

function migrateLegacyAccessToken(): string | null {
  const legacy = sessionStorage.getItem(ACCESS);
  if (!legacy) return null;
  localStorage.setItem(ACCESS, legacy);
  sessionStorage.removeItem(ACCESS);
  return legacy;
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS) ?? migrateLegacyAccessToken();
}

export function setAccessToken(token: string) {
  localStorage.setItem(ACCESS, token);
  sessionStorage.removeItem(ACCESS);
}

export function clearTokens() {
  localStorage.removeItem(ACCESS);
  sessionStorage.removeItem(ACCESS);
  localStorage.removeItem(MFA);
  sessionStorage.removeItem(MFA);
}

export function getMfaToken(): string | null {
  return sessionStorage.getItem(MFA);
}

export function setMfaToken(token: string) {
  sessionStorage.setItem(MFA, token);
}
