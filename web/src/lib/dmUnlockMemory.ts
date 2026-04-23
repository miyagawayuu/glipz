import { samePublicJwk, type DMLocalIdentity } from "./dmCrypto";

const MEMORY_UNLOCK_TTL_MS = 30 * 60 * 1000;

type RemoteIdentityLike = {
  configured?: boolean;
  public_jwk?: JsonWebKey;
};

type CachedIdentity = {
  identity: DMLocalIdentity;
  expiresAt: number;
};

let cachedIdentity: CachedIdentity | null = null;

export function rememberUnlockedIdentity(identity: DMLocalIdentity, ttlMs = MEMORY_UNLOCK_TTL_MS) {
  cachedIdentity = {
    identity,
    expiresAt: Date.now() + Math.max(1, ttlMs),
  };
}

export function getRememberedUnlockedIdentity(remote?: RemoteIdentityLike | null): DMLocalIdentity | null {
  if (!cachedIdentity) return null;
  if (Date.now() > cachedIdentity.expiresAt) {
    cachedIdentity = null;
    return null;
  }
  if (remote?.configured && remote.public_jwk && !samePublicJwk(cachedIdentity.identity.publicJwk, remote.public_jwk)) {
    cachedIdentity = null;
    return null;
  }
  cachedIdentity.expiresAt = Date.now() + MEMORY_UNLOCK_TTL_MS;
  return cachedIdentity.identity;
}

export function clearRememberedUnlockedIdentity() {
  cachedIdentity = null;
}

export function hasRememberedUnlockedIdentity(): boolean {
  return getRememberedUnlockedIdentity() !== null;
}
