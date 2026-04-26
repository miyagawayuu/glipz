import { apiBase } from "./api";

export type DMSealedBox = {
  iv: string;
  data: string;
};

export type DMEncryptedPrivateKeyBackup = {
  kdf: "PBKDF2";
  hash: "SHA-256";
  iterations: number;
  salt: string;
  iv: string;
  data: string;
};

export type DMLocalIdentity = {
  algorithm: "ECDH-P256";
  publicJwk: JsonWebKey;
  privateJwk: JsonWebKey;
};

export type DMEncryptedAttachmentDraft = {
  encryptedFile: File;
  meta: {
    object_key: string;
    public_url: string;
    file_name: string;
    content_type: string;
    size_bytes: number;
    encrypted_bytes: number;
    file_iv: string;
    sender_key_box: DMSealedBox;
    recipient_key_box: DMSealedBox;
  };
};

const DM_PRIVATE_KEY_KDF_ITERATIONS = 250000;
const DM_PRIVATE_KEY_KDF_MIN_ITERATIONS = 250000;

function bytesToBase64(bytes: ArrayBuffer | Uint8Array): string {
  const arr = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = "";
  for (let i = 0; i < arr.length; i += 1) binary += String.fromCharCode(arr[i]!);
  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) out[i] = binary.charCodeAt(i);
  return out;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

async function importPublicKey(jwk: JsonWebKey): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "jwk",
    jwk,
    { name: "ECDH", namedCurve: "P-256" },
    true,
    [],
  );
}

async function importPrivateKey(jwk: JsonWebKey): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "jwk",
    jwk,
    { name: "ECDH", namedCurve: "P-256" },
    true,
    ["deriveKey"],
  );
}

async function derivePairKey(privateJwk: JsonWebKey, publicJwk: JsonWebKey): Promise<CryptoKey> {
  const privateKey = await importPrivateKey(privateJwk);
  const publicKey = await importPublicKey(publicJwk);
  return crypto.subtle.deriveKey(
    { name: "ECDH", public: publicKey },
    privateKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

async function encryptBytesWithPublicKey(
  privateJwk: JsonWebKey,
  publicJwk: JsonWebKey,
  data: Uint8Array,
): Promise<DMSealedBox> {
  const key = await derivePairKey(privateJwk, publicJwk);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encrypted = await crypto.subtle.encrypt({ name: "AES-GCM", iv: toArrayBuffer(iv) }, key, toArrayBuffer(data));
  return {
    iv: bytesToBase64(iv),
    data: bytesToBase64(encrypted),
  };
}

async function decryptBytesWithPublicKey(
  privateJwk: JsonWebKey,
  publicJwk: JsonWebKey,
  box: DMSealedBox,
): Promise<Uint8Array> {
  const key = await derivePairKey(privateJwk, publicJwk);
  const decrypted = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: toArrayBuffer(base64ToBytes(box.iv)) },
    key,
    toArrayBuffer(base64ToBytes(box.data)),
  );
  return new Uint8Array(decrypted);
}

async function importPassphraseBaseKey(passphrase: string): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(passphrase),
    { name: "PBKDF2" },
    false,
    ["deriveKey"],
  );
}

export async function createLocalDMIdentity(): Promise<DMLocalIdentity> {
  const pair = await crypto.subtle.generateKey(
    { name: "ECDH", namedCurve: "P-256" },
    true,
    ["deriveKey"],
  );
  const publicJwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const privateJwk = await crypto.subtle.exportKey("jwk", pair.privateKey);
  return {
    algorithm: "ECDH-P256",
    publicJwk,
    privateJwk,
  };
}

export function samePublicJwk(a: JsonWebKey | null | undefined, b: JsonWebKey | null | undefined): boolean {
  if (!a || !b) return false;
  return JSON.stringify(a) === JSON.stringify(b);
}

function canonicalPublicJwk(jwk: JsonWebKey): string {
  return JSON.stringify({
    crv: jwk.crv || "",
    kty: jwk.kty || "",
    x: jwk.x || "",
    y: jwk.y || "",
  });
}

export async function publicJwkFingerprint(jwk: JsonWebKey): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(canonicalPublicJwk(jwk)));
  return bytesToBase64(digest).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export async function encryptPrivateJwkForPassphrase(
  privateJwk: JsonWebKey,
  passphrase: string,
): Promise<DMEncryptedPrivateKeyBackup> {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const baseKey = await importPassphraseBaseKey(passphrase);
  const key = await crypto.subtle.deriveKey(
    {
      name: "PBKDF2",
      salt,
      iterations: DM_PRIVATE_KEY_KDF_ITERATIONS,
      hash: "SHA-256",
    },
    baseKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt"],
  );
  const encrypted = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv },
    key,
    new TextEncoder().encode(JSON.stringify(privateJwk)),
  );
  return {
    kdf: "PBKDF2",
    hash: "SHA-256",
    iterations: DM_PRIVATE_KEY_KDF_ITERATIONS,
    salt: bytesToBase64(salt),
    iv: bytesToBase64(iv),
    data: bytesToBase64(encrypted),
  };
}

export async function decryptPrivateJwkFromPassphrase(
  backup: DMEncryptedPrivateKeyBackup,
  passphrase: string,
): Promise<JsonWebKey> {
  if (!backup?.salt || !backup?.iv || !backup?.data) {
    throw new Error("invalid_backup");
  }
  if (backup.kdf !== "PBKDF2" || backup.hash !== "SHA-256") {
    throw new Error("unsupported_backup_kdf");
  }
  const iterations = Number(backup.iterations);
  if (!Number.isFinite(iterations) || iterations < DM_PRIVATE_KEY_KDF_MIN_ITERATIONS) {
    throw new Error("weak_backup_kdf");
  }
  const baseKey = await importPassphraseBaseKey(passphrase);
  const key = await crypto.subtle.deriveKey(
    {
      name: "PBKDF2",
      salt: toArrayBuffer(base64ToBytes(backup.salt)),
      iterations,
      hash: "SHA-256",
    },
    baseKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["decrypt"],
  );
  const decrypted = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: toArrayBuffer(base64ToBytes(backup.iv)) },
    key,
    toArrayBuffer(base64ToBytes(backup.data)),
  );
  return JSON.parse(new TextDecoder().decode(decrypted)) as JsonWebKey;
}

export async function encryptDMText(
  text: string,
  selfIdentity: DMLocalIdentity,
  peerPublicJwk: JsonWebKey,
): Promise<{ senderPayload: DMSealedBox; recipientPayload: DMSealedBox }> {
  const bytes = new TextEncoder().encode(text);
  return {
    senderPayload: await encryptBytesWithPublicKey(selfIdentity.privateJwk, selfIdentity.publicJwk, bytes),
    recipientPayload: await encryptBytesWithPublicKey(selfIdentity.privateJwk, peerPublicJwk, bytes),
  };
}

export async function decryptDMText(
  ciphertext: DMSealedBox,
  selfIdentity: DMLocalIdentity,
  publicJwk: JsonWebKey,
): Promise<string> {
  const bytes = await decryptBytesWithPublicKey(selfIdentity.privateJwk, publicJwk, ciphertext);
  return new TextDecoder().decode(bytes);
}

export async function encryptDMAttachment(
  file: File,
  selfIdentity: DMLocalIdentity,
  peerPublicJwk: JsonWebKey,
): Promise<DMEncryptedAttachmentDraft> {
  const fileKey = await crypto.subtle.generateKey(
    { name: "AES-GCM", length: 256 },
    true,
    ["encrypt", "decrypt"],
  );
  const rawKey = new Uint8Array(await crypto.subtle.exportKey("raw", fileKey));
  const fileIv = crypto.getRandomValues(new Uint8Array(12));
  const fileBytes = await file.arrayBuffer();
  const encrypted = await crypto.subtle.encrypt({ name: "AES-GCM", iv: fileIv }, fileKey, fileBytes);
  const encryptedFile = new File([encrypted], `${file.name}.glipz`, { type: "application/octet-stream" });
  return {
    encryptedFile,
    meta: {
      object_key: "",
      public_url: "",
      file_name: file.name,
      content_type: file.type || "application/octet-stream",
      size_bytes: file.size,
      encrypted_bytes: encrypted.byteLength,
      file_iv: bytesToBase64(fileIv),
      sender_key_box: await encryptBytesWithPublicKey(selfIdentity.privateJwk, selfIdentity.publicJwk, rawKey),
      recipient_key_box: await encryptBytesWithPublicKey(selfIdentity.privateJwk, peerPublicJwk, rawKey),
    },
  };
}

export async function decryptDMAttachmentToBlob(
  attachment: {
    public_url: string;
    file_name: string;
    content_type: string;
    file_iv: string;
    sender_key_box: DMSealedBox;
    recipient_key_box: DMSealedBox;
  },
  selfIdentity: DMLocalIdentity,
  peerPublicJwk: JsonWebKey,
  sentByMe: boolean,
): Promise<Blob> {
  if (!isAllowedDMAttachmentURL(attachment.public_url)) {
    throw new Error("attachment_url_not_allowed");
  }
  const res = await fetch(attachment.public_url, { method: "GET" });
  if (!res.ok) throw new Error("attachment_fetch_failed");
  const encryptedFile = await res.arrayBuffer();
  return decryptDMAttachmentBytesToBlob(attachment, encryptedFile, selfIdentity, peerPublicJwk, sentByMe);
}

export function isAllowedDMAttachmentURL(raw: string): boolean {
  if (typeof window === "undefined") return false;
  try {
    const u = new URL(raw, window.location.origin);
    const allowedOrigins = new Set([window.location.origin]);
    const base = apiBase();
    if (base) {
      allowedOrigins.add(new URL(base, window.location.origin).origin);
    }
    const extraBaseURLs = String(import.meta.env.VITE_ALLOWED_DM_ATTACHMENT_BASE_URLS || "")
      .split(",")
      .map((baseURL) => baseURL.trim())
      .filter(Boolean);
    for (const baseURL of extraBaseURLs) {
      const base = new URL(baseURL);
      const basePath = base.pathname.endsWith("/") ? base.pathname : `${base.pathname}/`;
      if (basePath === "/") continue;
      if (base.protocol === "https:" && u.origin === base.origin && u.pathname.startsWith(basePath)) return true;
    }
    return allowedOrigins.has(u.origin) && u.pathname.startsWith("/api/v1/media/object/");
  } catch {
    return false;
  }
}

export async function decryptDMAttachmentBytesToBlob(
  attachment: {
    file_name: string;
    content_type: string;
    file_iv: string;
    sender_key_box: DMSealedBox;
    recipient_key_box: DMSealedBox;
  },
  encryptedFile: ArrayBuffer,
  selfIdentity: DMLocalIdentity,
  peerPublicJwk: JsonWebKey,
  sentByMe: boolean,
): Promise<Blob> {
  const box = sentByMe ? attachment.sender_key_box : attachment.recipient_key_box;
  const keyPublicJwk = sentByMe ? selfIdentity.publicJwk : peerPublicJwk;
  const rawKey = await decryptBytesWithPublicKey(selfIdentity.privateJwk, keyPublicJwk, box);
  const key = await crypto.subtle.importKey("raw", toArrayBuffer(rawKey), { name: "AES-GCM" }, false, ["decrypt"]);
  const decrypted = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: toArrayBuffer(base64ToBytes(attachment.file_iv)) },
    key,
    encryptedFile,
  );
  return new Blob([decrypted], { type: attachment.content_type || "application/octet-stream" });
}
