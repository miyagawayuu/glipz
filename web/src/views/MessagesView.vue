<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import Icon from "../components/Icon.vue";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { dmReceivedTick, latestDMEvent, unreadDMCount, refreshUnreadDMCount } from "../dmHub";
import { formatDateTime } from "../i18n";
import { api, apiBase } from "../lib/api";
import {
  assertDMIdentityMatchesServer,
  createDMThread,
  inviteDMPeer,
  fetchDMIdentity,
  fetchDMThread,
  listDMMessages,
  listDMThreads,
  saveDMIdentity,
  sendDMMessage,
  uploadDMFile,
  type DMAttachment,
  type DMIdentityResponse,
  type DMMessage,
  type DMThread,
} from "../lib/dm";
import {
  createLocalDMIdentity,
  decryptPrivateJwkFromPassphrase,
  decryptDMAttachmentToBlob,
  decryptDMAttachmentBytesToBlob,
  decryptDMText,
  encryptDMAttachment,
  encryptDMText,
  encryptPrivateJwkForPassphrase,
  type DMLocalIdentity,
} from "../lib/dmCrypto";
import {
  clearRememberedUnlockedIdentity,
  getRememberedUnlockedIdentity,
  rememberUnlockedIdentity,
} from "../lib/dmUnlockMemory";

type ResolvedDMMessage = DMMessage & {
  decrypted_text: string;
  decrypt_error: boolean;
};

type FederationDMThread = {
  thread_id: string;
  remote_acct: string;
  state: string;
  updated_at: string;
  created_at: string;
};

type FederationDMMessage = {
  message_id: string;
  thread_id: string;
  sender_acct: string;
  sender_payload?: { iv: string; data: string } | null;
  recipient_payload: { iv: string; data: string };
  attachments: any[];
  sent_at?: string | null;
  created_at: string;
};

type DMTimelineItem =
  {
    kind: "message";
    id: string;
    created_at: string;
    sent_by_me: boolean;
    message: ResolvedDMMessage;
  };

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const isDesktop = ref(false);
let desktopMediaQuery: MediaQueryList | null = null;

const threadId = computed(() => String(route.params.threadId ?? "").trim());
const handleQuery = computed(() => String(route.query.with ?? "").replace(/^@/, "").trim());

const meHandle = ref("");
const remoteIdentity = ref<DMIdentityResponse | null>(null);
const identity = ref<DMLocalIdentity | null>(null);
const identityBusy = ref(false);
const passphrase = ref("");
const passphraseConfirm = ref("");
const identityError = ref("");
const threadSearchQuery = ref("");
const threads = ref<DMThread[]>([]);
const activeThread = ref<DMThread | null>(null);
const messages = ref<ResolvedDMMessage[]>([]);
const loading = ref(true);
const loadingMessages = ref(false);
const threadBusy = ref(false);
const sendBusy = ref(false);
const error = ref("");
const fedThreads = ref<FederationDMThread[]>([]);
const fedActiveThreadId = ref("");
const fedActiveRemoteAcct = ref("");
const fedMessages = ref<
  {
    id: string;
    created_at: string;
    sent_by_me: boolean;
    text: string;
    decrypt_error: boolean;
    attachments: any[];
  }[]
>([]);
const fedLoading = ref(false);
const fedError = ref("");
const fedComposerText = ref("");
const fedSendBusy = ref(false);
const fedPendingFiles = ref<File[]>([]);
const fedFileInput = ref<HTMLInputElement | null>(null);
const notice = ref("");
let threadListInFlight: Promise<void> | null = null;
let threadRefreshTimer: ReturnType<typeof setTimeout> | null = null;

function triggerFedFilePicker() {
  fedFileInput.value?.click();
}

function onPickFedFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  input.value = "";
  if (!files.length) return;
  fedPendingFiles.value = [...fedPendingFiles.value, ...files].slice(0, 8);
}

function removeFedPendingFile(index: number) {
  fedPendingFiles.value = fedPendingFiles.value.filter((_, i) => i !== index);
}

const fedActiveThread = computed(() =>
  fedActiveThreadId.value ? fedThreads.value.find((x) => x.thread_id === fedActiveThreadId.value) ?? null : null,
);
const fedCanSend = computed(() => fedActiveThread.value?.state === "accepted");
const composerText = ref("");
const pendingFiles = ref<File[]>([]);
const attachmentBusyKey = ref("");
const fileInput = ref<HTMLInputElement | null>(null);

function stringsTrim(s: unknown): string {
  return String(s ?? "").trim();
}

const activeThreadTitle = computed(() =>
  activeThread.value ? activeThread.value.peer_display_name || `@${activeThread.value.peer_handle}` : "",
);
const identityConfigured = computed(() => remoteIdentity.value?.configured === true);
const identityUnlocked = computed(() => !!identity.value);
const showThreadListPane = computed(() =>
  isDesktop.value || (!threadId.value && identityUnlocked.value),
);
const showMessagePane = computed(() =>
  isDesktop.value || !!threadId.value || !identityUnlocked.value || loading.value,
);
const showOverviewHeader = computed(() =>
  isDesktop.value,
);
const filteredThreads = computed(() => {
  const q = threadSearchQuery.value.trim().toLowerCase();
  if (!q) return threads.value;
  return threads.value.filter((thread) => {
    const name = (thread.peer_display_name ?? "").toLowerCase();
    const handle = (thread.peer_handle ?? "").toLowerCase();
    return name.includes(q) || handle.includes(q);
  });
});
const timelineItems = computed<DMTimelineItem[]>(() => {
  const items: DMTimelineItem[] = [
    ...messages.value.map((message) => ({
      kind: "message" as const,
      id: `message:${message.id}`,
      created_at: message.created_at,
      sent_by_me: message.sent_by_me,
      message,
    })),
  ];
  items.sort((a, b) => {
    const timeDiff = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
    if (timeDiff !== 0) return timeDiff;
    return a.id.localeCompare(b.id);
  });
  return items;
});

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let size = value;
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx += 1;
  }
  return `${size >= 10 || idx === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[idx]}`;
}

function triggerFilePicker() {
  fileInput.value?.click();
}

function syncDesktopLayout() {
  if (typeof window === "undefined") return;
  isDesktop.value = window.matchMedia("(min-width: 1024px)").matches;
}

function onPickFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  input.value = "";
  if (!files.length) return;
  pendingFiles.value = [...pendingFiles.value, ...files].slice(0, 8);
}

function removePendingFile(index: number) {
  pendingFiles.value = pendingFiles.value.filter((_, i) => i !== index);
}

async function loadIdentity() {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const me = await api<{ handle: string }>("/api/v1/me", { method: "GET", token });
  meHandle.value = me.handle;
  try {
    remoteIdentity.value = await fetchDMIdentity();
  } catch {
    // Network / transient failures should not force a relock.
    // Keep the current identity + remembered cache as-is and let the UI continue.
    return;
  }
  if (!remoteIdentity.value?.configured) {
    clearRememberedUnlockedIdentity();
    identity.value = null;
    return;
  }
  identity.value = getRememberedUnlockedIdentity(remoteIdentity.value);
}

async function loadThreadsOnly() {
  if (threadListInFlight) return threadListInFlight;
  threadListInFlight = (async () => {
    threads.value = await listDMThreads();
  })();
  try {
    await threadListInFlight;
  } finally {
    threadListInFlight = null;
  }
}

function scheduleThreadListRefresh(delayMs = 1000) {
  if (threadRefreshTimer) clearTimeout(threadRefreshTimer);
  threadRefreshTimer = setTimeout(() => {
    threadRefreshTimer = null;
    if (!identityUnlocked.value) return;
    if (threadId.value) {
      void loadActiveThreadAndMessages();
    } else {
      void loadThreadsOnly();
    }
  }, delayMs);
}

async function resolveThreadFromQuery() {
  if (!handleQuery.value || threadId.value) return;
  threadBusy.value = true;
  try {
    const thread = await createDMThread(handleQuery.value);
    await loadThreadsOnly();
    await router.replace({ path: `/messages/${thread.id}` });
  } catch (e: unknown) {
    const code = e instanceof Error ? e.message : "";
    if (code === "peer_identity_required") {
      try {
        const st = await inviteDMPeer(handleQuery.value);
        if (st === "peer_ready") {
          const thread = await createDMThread(handleQuery.value);
          await loadThreadsOnly();
          await router.replace({ path: `/messages/${thread.id}` });
          return;
        }
        notice.value =
          st === "invited_auto" ? t("views.messages.peerDmInviteSentAuto") : t("views.messages.peerDmInviteSent");
      } catch {
        error.value = t("views.messages.errors.inviteDmFailed");
      }
      await router.replace({ path: "/messages" });
      return;
    }
    error.value = code || t("views.messages.errors.loadMessagesFailed");
    await router.replace({ path: "/messages" });
  } finally {
    threadBusy.value = false;
  }
}

async function decryptMessages(rows: DMMessage[], thread: DMThread): Promise<ResolvedDMMessage[]> {
  if (!identity.value) return [];
  const peerKey = thread.peer_public_jwk;
  return Promise.all(
    rows.map(async (row) => {
      try {
        const publicJwk = row.sent_by_me ? identity.value!.publicJwk : peerKey;
        if (!publicJwk) throw new Error("peer_key_missing");
        const decrypted = await decryptDMText(row.ciphertext, identity.value!, publicJwk);
        return { ...row, decrypted_text: decrypted, decrypt_error: false };
      } catch {
        return { ...row, decrypted_text: t("views.messages.decryptFailed"), decrypt_error: true };
      }
    }),
  );
}

async function loadActiveThreadAndMessages() {
  if (!threadId.value) {
    activeThread.value = null;
    messages.value = [];
    await refreshUnreadDMCount();
    return;
  }
  loadingMessages.value = true;
  error.value = "";
  try {
    const thread = await fetchDMThread(threadId.value);
    activeThread.value = thread;
    const rows = await listDMMessages(thread.id);
    messages.value = await decryptMessages(rows.reverse(), thread);
    await refreshUnreadDMCount();
    await loadThreadsOnly();
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.messages.errors.loadMessagesFailed");
  } finally {
    loadingMessages.value = false;
  }
}

async function bootstrap() {
  loading.value = true;
  error.value = "";
  try {
    await loadIdentity();
    if (identity.value) {
      await continueAfterIdentityReady();
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.messages.errors.bootstrapFailed");
  } finally {
    loading.value = false;
  }
}

async function continueAfterIdentityReady() {
  await loadThreadsOnly();
  await loadFederationThreadsOnly();
  await resolveThreadFromQuery();
  await resolveFederationThreadFromQuery();
  await loadActiveThreadAndMessages();
  await loadActiveFederationThreadAndMessages();
}

async function loadFederationThreadsOnly() {
  const token = getAccessToken();
  if (!token || !identity.value) return;
  try {
    const res = await api<{ items: FederationDMThread[] }>("/api/v1/federation/dm/threads", { method: "GET", token });
    fedThreads.value = res.items ?? [];
  } catch {
    fedThreads.value = [];
  }
}

async function resolveFederationThreadFromQuery() {
  const q = String(route.query.fed_thread ?? "").trim();
  if (!q) return;
  const row = (fedThreads.value ?? []).find((x) => x.thread_id === q);
  fedActiveThreadId.value = q;
  fedActiveRemoteAcct.value = row?.remote_acct ?? "";
}

async function loadActiveFederationThreadAndMessages() {
  const token = getAccessToken();
  if (!token || !identity.value || !fedActiveThreadId.value) return;
  fedLoading.value = true;
  fedError.value = "";
  try {
    const threadId = fedActiveThreadId.value;
    const msgs = await api<{ items: FederationDMMessage[] }>(`/api/v1/federation/dm/threads/${encodeURIComponent(threadId)}/messages`, {
      method: "GET",
      token,
    });
    const remoteAcct = fedActiveRemoteAcct.value || (fedThreads.value.find((x) => x.thread_id === threadId)?.remote_acct ?? "");
    fedActiveRemoteAcct.value = remoteAcct;
    let peerPublicJwk: JsonWebKey | null = null;
    if (remoteAcct) {
      const keyDoc = await api<any>(`/api/v1/federation/dm/keys?acct=${encodeURIComponent(remoteAcct)}`, { method: "GET", token });
      peerPublicJwk = (keyDoc?.public_jwk ?? null) as JsonWebKey | null;
    }
    const rows = (msgs.items ?? []).slice().reverse();
    const existingTextByID = new Map<string, string>(fedMessages.value.map((x) => [x.id, x.text]));
    fedMessages.value = await Promise.all(
      rows.map(async (m) => {
        const sentByMe = remoteAcct ? String(m.sender_acct).toLowerCase() !== remoteAcct.toLowerCase() : false;
        if (sentByMe) {
          if (m.sender_payload && stringsTrim(m.sender_payload.iv) && stringsTrim(m.sender_payload.data)) {
            try {
              const text = await decryptDMText(m.sender_payload, identity.value!, identity.value!.publicJwk);
              return { id: m.message_id, created_at: m.created_at, sent_by_me: true, text, decrypt_error: false, attachments: m.attachments ?? [] };
            } catch {
              // Fall through to existing cache / placeholder.
            }
          }
          const existing = existingTextByID.get(m.message_id);
          return {
            id: m.message_id,
            created_at: m.created_at,
            sent_by_me: true,
            text: existing && existing !== "(送信済み)" ? existing : "(送信済み)",
            decrypt_error: false,
            attachments: m.attachments ?? [],
          };
        }
        if (!peerPublicJwk) {
          return { id: m.message_id, created_at: m.created_at, sent_by_me: false, text: "(鍵がありません)", decrypt_error: true, attachments: m.attachments ?? [] };
        }
        try {
          const text = await decryptDMText(m.recipient_payload, identity.value!, peerPublicJwk);
          return { id: m.message_id, created_at: m.created_at, sent_by_me: false, text, decrypt_error: false, attachments: m.attachments ?? [] };
        } catch {
          // Keep the message visible even if decryption fails.
          return { id: m.message_id, created_at: m.created_at, sent_by_me: false, text: "(復号できません)", decrypt_error: true, attachments: m.attachments ?? [] };
        }
      }),
    );
  } catch (e: unknown) {
    fedError.value = e instanceof Error ? e.message : "連合DMの読み込みに失敗しました";
    fedMessages.value = [];
  } finally {
    fedLoading.value = false;
  }
}

async function sendFederationMessage() {
  if (fedSendBusy.value || !identity.value) return;
  const token = getAccessToken();
  if (!token) return;
  const text = fedComposerText.value;
  if (!text.trim() && fedPendingFiles.value.length === 0) return;
  if (!fedActiveThreadId.value || !fedActiveRemoteAcct.value) return;
  if (!fedCanSend.value) return;

  fedSendBusy.value = true;
  fedError.value = "";
  try {
    const keyDoc = await api<any>(`/api/v1/federation/dm/keys?acct=${encodeURIComponent(fedActiveRemoteAcct.value)}`, {
      method: "GET",
      token,
    });
    const peerPublicJwk = (keyDoc?.public_jwk ?? null) as JsonWebKey | null;
    if (!peerPublicJwk) throw new Error("peer_key_missing");
    const encryptedAttachments: any[] = [];
    for (const file of fedPendingFiles.value) {
      const encrypted = await encryptDMAttachment(file, identity.value, peerPublicJwk);
      const uploaded = await uploadDMFile(token, encrypted.encryptedFile);
      encryptedAttachments.push({
        public_url: uploaded.public_url,
        file_name: encrypted.meta.file_name,
        content_type: encrypted.meta.content_type,
        size_bytes: encrypted.meta.size_bytes,
        encrypted_bytes: encrypted.meta.encrypted_bytes,
        file_iv: encrypted.meta.file_iv,
        sender_key_box: encrypted.meta.sender_key_box,
        recipient_key_box: encrypted.meta.recipient_key_box,
      });
    }
    const sealed = await encryptDMText(text, identity.value, peerPublicJwk);
    const messageId = (crypto as any).randomUUID ? (crypto as any).randomUUID() : String(Date.now());
    const sentAt = new Date().toISOString();
    // Optimistic UI append (sender cannot decrypt recipient_payload by design).
    fedMessages.value = [
      ...fedMessages.value,
      { id: messageId, created_at: sentAt, sent_by_me: true, text, decrypt_error: false, attachments: encryptedAttachments },
    ];
    await api("/api/v1/federation/dm/message", {
      method: "POST",
      token,
      json: {
        thread_id: fedActiveThreadId.value,
        message_id: messageId,
        to_acct: fedActiveRemoteAcct.value,
        sender_payload: sealed.senderPayload,
        recipient_payload: sealed.recipientPayload,
        sent_at: sentAt,
        attachments: encryptedAttachments,
      },
    });
    fedComposerText.value = "";
    fedPendingFiles.value = [];
    await loadFederationThreadsOnly();
  } catch (e: unknown) {
    fedError.value = e instanceof Error ? e.message : "送信に失敗しました";
  } finally {
    fedSendBusy.value = false;
  }
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]!);
  return btoa(binary);
}

async function acceptFederationThread() {
  if (fedSendBusy.value || !identity.value || !fedActiveThreadId.value || !fedActiveRemoteAcct.value) return;
  const token = getAccessToken();
  if (!token) return;
  fedSendBusy.value = true;
  fedError.value = "";
  try {
    const keyDoc = await api<any>(`/api/v1/federation/dm/keys?acct=${encodeURIComponent(fedActiveRemoteAcct.value)}`, {
      method: "GET",
      token,
    });
    const peerPublicJwk = (keyDoc?.public_jwk ?? null) as JsonWebKey | null;
    if (!peerPublicJwk) throw new Error("peer_key_missing");

    // Generate a thread key (not yet used for message payloads in this phase).
    const threadKey = crypto.getRandomValues(new Uint8Array(32));
    localStorage.setItem(`fed_dm_thread_key:${fedActiveThreadId.value}`, bytesToBase64(threadKey));

    const sealed = await encryptDMText(JSON.stringify({ thread_key: bytesToBase64(threadKey) }), identity.value, peerPublicJwk);
    await api("/api/v1/federation/dm/accept", {
      method: "POST",
      token,
      json: {
        thread_id: fedActiveThreadId.value,
        to_acct: fedActiveRemoteAcct.value,
        key_box_for_inviter: sealed.recipientPayload,
      },
    });
    await loadFederationThreadsOnly();
  } catch (e: unknown) {
    fedError.value = e instanceof Error ? e.message : "承認に失敗しました";
  } finally {
    fedSendBusy.value = false;
  }
}

async function rejectFederationThread() {
  if (fedSendBusy.value || !fedActiveThreadId.value || !fedActiveRemoteAcct.value) return;
  const token = getAccessToken();
  if (!token) return;
  fedSendBusy.value = true;
  fedError.value = "";
  try {
    await api("/api/v1/federation/dm/reject", {
      method: "POST",
      token,
      json: {
        thread_id: fedActiveThreadId.value,
        to_acct: fedActiveRemoteAcct.value,
      },
    });
    await loadFederationThreadsOnly();
    fedActiveThreadId.value = "";
    fedActiveRemoteAcct.value = "";
    fedMessages.value = [];
  } catch (e: unknown) {
    fedError.value = e instanceof Error ? e.message : "拒否に失敗しました";
  } finally {
    fedSendBusy.value = false;
  }
}

async function unlockIdentity() {
  if (!remoteIdentity.value?.configured || !remoteIdentity.value.public_jwk || !remoteIdentity.value.encrypted_private_jwk) {
    throw new Error("identity_not_configured");
  }
  const privateJwk = await decryptPrivateJwkFromPassphrase(
    remoteIdentity.value.encrypted_private_jwk,
    passphrase.value,
  );
  const next: DMLocalIdentity = {
    algorithm: "ECDH-P256",
    publicJwk: remoteIdentity.value.public_jwk,
    privateJwk,
  };
  assertDMIdentityMatchesServer(next, remoteIdentity.value);
  identity.value = next;
  rememberUnlockedIdentity(next);
}

async function createIdentity() {
  const next = await createLocalDMIdentity();
  const encryptedPrivateJwk = await encryptPrivateJwkForPassphrase(next.privateJwk, passphrase.value);
  await saveDMIdentity(next, encryptedPrivateJwk);
  remoteIdentity.value = await fetchDMIdentity();
  identity.value = next;
  rememberUnlockedIdentity(next);
}

async function submitIdentityForm() {
  if (identityBusy.value) return;
  identityBusy.value = true;
  identityError.value = "";
  error.value = "";
  try {
    const normalized = passphrase.value.trim();
    if (normalized.length < 8) {
      throw new Error("passphrase_too_short");
    }
    if (!identityConfigured.value && normalized !== passphraseConfirm.value.trim()) {
      throw new Error("passphrase_mismatch");
    }
    passphrase.value = normalized;
    if (identityConfigured.value) {
      await unlockIdentity();
    } else {
      await createIdentity();
    }
    passphrase.value = "";
    passphraseConfirm.value = "";
    await continueAfterIdentityReady();
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : t("views.messages.errors.identityInitFailed");
    identityError.value =
      message === "passphrase_too_short"
        ? t("views.messages.errors.passphraseTooShort")
        : message === "passphrase_mismatch"
          ? t("views.messages.errors.passphraseMismatch")
          : message === "OperationError"
            ? t("views.messages.errors.passphraseWrong")
            : message === "identity_mismatch"
              ? t("views.messages.errors.identityMismatch")
              : message;
    if (message === "OperationError" || message === "identity_mismatch") {
      clearRememberedUnlockedIdentity();
      identity.value = null;
    }
  } finally {
    identityBusy.value = false;
  }
}

async function openThread(thread: DMThread) {
  if (thread.id === threadId.value) return;
  await router.push({ path: `/messages/${thread.id}` });
}

async function sendCurrentMessage() {
  if (sendBusy.value || !activeThread.value || !identity.value) return;
  const token = getAccessToken();
  if (!token) return;
  const text = composerText.value;
  if (!text.trim() && pendingFiles.value.length === 0) return;
  if (!activeThread.value.peer_public_jwk) {
    error.value = t("views.messages.errors.peerKeyMissing");
    return;
  }
  sendBusy.value = true;
  error.value = "";
  try {
    const encryptedAttachments: DMAttachment[] = [];
    for (const file of pendingFiles.value) {
      const encrypted = await encryptDMAttachment(file, identity.value, activeThread.value.peer_public_jwk);
      const uploaded = await uploadDMFile(token, encrypted.encryptedFile);
      encryptedAttachments.push({
        ...encrypted.meta,
        object_key: uploaded.object_key,
        public_url: uploaded.public_url,
      });
    }
    const encryptedText = await encryptDMText(text, identity.value, activeThread.value.peer_public_jwk);
    await sendDMMessage(activeThread.value.id, {
      sender_payload: encryptedText.senderPayload,
      recipient_payload: encryptedText.recipientPayload,
      attachments: encryptedAttachments,
    });
    composerText.value = "";
    pendingFiles.value = [];
    await loadActiveThreadAndMessages();
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.messages.errors.sendFailed");
  } finally {
    sendBusy.value = false;
  }
}

async function saveAttachment(message: ResolvedDMMessage, attachment: DMAttachment, index: number) {
  if (!identity.value || !activeThread.value?.peer_public_jwk) return;
  attachmentBusyKey.value = `${message.id}:${index}`;
  try {
    const blob = await decryptDMAttachmentToBlob(
      attachment,
      identity.value,
      activeThread.value.peer_public_jwk,
      message.sent_by_me,
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = attachment.file_name || "attachment.bin";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 60000);
  } catch {
    error.value = t("views.messages.errors.attachmentDecryptFailed");
  } finally {
    attachmentBusyKey.value = "";
  }
}

async function saveFederationAttachment(message: { id: string; sent_by_me: boolean }, attachment: any, index: number) {
  if (!identity.value || !fedActiveRemoteAcct.value || !fedActiveThreadId.value) return;
  attachmentBusyKey.value = `${message.id}:fed:${index}`;
  try {
    const token = getAccessToken();
    if (!token) return;
    const keyDoc = await api<any>(`/api/v1/federation/dm/keys?acct=${encodeURIComponent(fedActiveRemoteAcct.value)}`, {
      method: "GET",
      token,
    });
    const peerPublicJwk = (keyDoc?.public_jwk ?? null) as JsonWebKey | null;
    if (!peerPublicJwk) throw new Error("peer_key_missing");

    const proxyURL = `${apiBase()}/api/v1/federation/dm/attachment?acct=${encodeURIComponent(fedActiveRemoteAcct.value)}&url=${encodeURIComponent(String(attachment.public_url || ""))}`;
    const res = await fetch(proxyURL, { method: "GET", headers: { Authorization: `Bearer ${token}` } });
    if (!res.ok) throw new Error("attachment_fetch_failed");
    const encryptedFile = await res.arrayBuffer();
    const blob = await decryptDMAttachmentBytesToBlob(attachment, encryptedFile, identity.value, peerPublicJwk, message.sent_by_me);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = String(attachment.file_name || "attachment.bin");
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 60000);
  } catch {
    fedError.value = "添付ファイルの復号に失敗しました";
  } finally {
    attachmentBusyKey.value = "";
  }
}

async function syncFederationDMStateFromEvent() {
  const event = latestDMEvent.value;
  if (!event) return;
  if (
    event.kind !== "federation_dm_invite"
    && event.kind !== "federation_dm_accept"
    && event.kind !== "federation_dm_reject"
    && event.kind !== "federation_dm_message"
  ) {
    return;
  }
  if (!identityUnlocked.value) return;
  // Always refresh threads to update state (invited -> accepted, etc).
  await loadFederationThreadsOnly();
  if (fedActiveThreadId.value && event.thread_id === fedActiveThreadId.value) {
    await loadActiveFederationThreadAndMessages();
  }
}

watch(
  () => threadId.value,
  async () => {
    if (!loading.value && identityUnlocked.value) {
      await loadActiveThreadAndMessages();
    }
  },
);

watch(
  () => String(route.query.fed_thread ?? "").trim(),
  async (next) => {
    if (!identityUnlocked.value || loading.value) return;
    if (!next) {
      fedActiveThreadId.value = "";
      fedActiveRemoteAcct.value = "";
      fedMessages.value = [];
      return;
    }
    // When navigating between federated DM threads via query params,
    // resolve the active thread and load messages without requiring a full reload.
    fedActiveThreadId.value = next;
    const row = (fedThreads.value ?? []).find((x) => x.thread_id === next);
    fedActiveRemoteAcct.value = row?.remote_acct ?? fedActiveRemoteAcct.value;
    await loadActiveFederationThreadAndMessages();
  },
  { immediate: true },
);

watch(unreadDMCount, () => {
  if (!identityUnlocked.value) return;
  scheduleThreadListRefresh();
});

watch(dmReceivedTick, () => {
  if (identityUnlocked.value) {
    void syncFederationDMStateFromEvent();
  }
});

onMounted(() => {
  syncDesktopLayout();
  if (typeof window !== "undefined") {
    desktopMediaQuery = window.matchMedia("(min-width: 1024px)");
    desktopMediaQuery.addEventListener("change", syncDesktopLayout);
  }
  void bootstrap();
});

onBeforeUnmount(() => {
  if (threadRefreshTimer) {
    clearTimeout(threadRefreshTimer);
    threadRefreshTimer = null;
  }
  desktopMediaQuery?.removeEventListener("change", syncDesktopLayout);
  desktopMediaQuery = null;
});
</script>

<template>
  <section class="flex h-full min-h-0 w-full flex-col overflow-hidden bg-neutral-50">
    <div v-if="showOverviewHeader" class="border-b border-neutral-200 bg-white px-5 py-4">
      <h1 class="text-lg font-semibold text-neutral-900">{{ $t("views.messages.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-500">
        {{ $t("views.messages.subtitle") }}
      </p>
    </div>

    <div class="flex min-h-0 flex-1 overflow-hidden flex-col lg:flex-row">
      <aside
        class="min-h-0 w-full flex-col border-b border-neutral-200 bg-white lg:flex lg:min-h-0 lg:w-80 lg:border-b-0 lg:border-r"
        :class="showThreadListPane ? 'flex' : 'hidden'"
      >
        <div class="px-4 py-3">
          <label class="sr-only" for="dm-thread-search">{{ $t("views.messages.searchSr") }}</label>
          <input
            id="dm-thread-search"
            v-model="threadSearchQuery"
            type="search"
            :placeholder="$t('views.messages.searchPlaceholder')"
            autocomplete="off"
            class="w-full rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
        </div>
        <div v-if="loading" class="px-4 pb-4 text-sm text-neutral-500">{{ $t("views.messages.loadingShort") }}</div>
        <div v-else-if="!identityUnlocked" class="px-4 pb-4 text-sm text-neutral-500">
          {{ $t("views.messages.unlockThreadsHint") }}
        </div>
        <div v-else-if="!threads.length" class="px-4 pb-4 text-sm text-neutral-500">
          {{ $t("views.messages.noThreads") }}
        </div>
        <div v-else-if="!filteredThreads.length" class="px-4 pb-4 text-sm text-neutral-500">
          {{ $t("views.messages.noThreadMatch") }}
        </div>
        <ul v-else class="min-h-0 flex-1 overflow-y-auto">
          <li v-for="thread in filteredThreads" :key="thread.id">
            <button
              type="button"
              class="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-lime-50"
              :class="thread.id === threadId ? 'bg-lime-50' : 'bg-white'"
              @click="openThread(thread)"
            >
              <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-lime-500 text-sm font-semibold text-white">
                {{ (thread.peer_display_name || thread.peer_handle).slice(0, 1).toUpperCase() }}
              </span>
              <span class="min-w-0 flex-1">
                <span class="flex flex-wrap items-center gap-2">
                  <span class="truncate text-sm font-semibold text-neutral-900">{{ thread.peer_display_name }}</span>
                  <UserBadges :badges="thread.peer_badges" size="xs" />
                  <span v-if="thread.unread_count > 0" class="inline-flex h-5 min-w-[1.25rem] items-center justify-center whitespace-nowrap rounded-full bg-red-500 px-1.5 text-[11px] font-bold leading-none text-white">
                    {{ thread.unread_count > 99 ? "99+" : thread.unread_count }}
                  </span>
                </span>
                <span class="block truncate text-xs text-neutral-500">@{{ thread.peer_handle }}</span>
                <span class="mt-1 block text-xs text-neutral-400">
                  {{ thread.last_message_at ? formatDateTime(thread.last_message_at, { dateStyle: "short", timeStyle: "short" }) : $t("views.messages.messagePending") }}
                </span>
              </span>
            </button>
          </li>
        </ul>

        <div v-if="identityUnlocked" class="border-t border-neutral-200">
          <div class="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-neutral-500">連合DM</div>
          <div v-if="!fedThreads.length" class="px-4 pb-4 text-sm text-neutral-500">連合DMはまだありません。</div>
          <ul v-else class="max-h-56 overflow-y-auto pb-3">
            <li v-for="t in fedThreads" :key="t.thread_id">
              <button
                type="button"
                class="flex w-full items-start gap-3 px-4 py-2 text-left hover:bg-lime-50"
                :class="t.thread_id === fedActiveThreadId ? 'bg-lime-50' : 'bg-white'"
                @click="router.push({ path: '/messages', query: { fed_thread: t.thread_id } })"
              >
                <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-neutral-900 text-xs font-semibold text-white">F</span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-semibold text-neutral-900">{{ t.remote_acct }}</span>
                  <span class="block truncate text-xs text-neutral-500">{{ t.state }}</span>
                </span>
              </button>
            </li>
          </ul>
        </div>
      </aside>

      <div
        class="min-h-0 flex-1 flex-col overflow-hidden"
        :class="showMessagePane ? 'flex' : 'hidden lg:flex'"
      >
        <div v-if="error" class="border-b border-red-200 bg-red-50 px-5 py-3 text-sm text-red-700">
          {{ error }}
        </div>

        <template v-if="loading">
          <div class="flex flex-1 items-center justify-center px-6 py-10 text-sm text-neutral-500">
            {{ $t("views.messages.loadingShort") }}
          </div>
        </template>

        <template v-else-if="!identityUnlocked">
          <div class="flex flex-1 items-center justify-center px-6 py-10">
            <div class="w-full max-w-lg rounded-3xl border border-neutral-200 bg-white p-6 shadow-sm">
              <h2 class="text-lg font-semibold text-neutral-900">
                {{ identityConfigured ? $t("views.messages.identityUnlockTitle") : $t("views.messages.identityCreateTitle") }}
              </h2>
              <p class="mt-2 text-sm leading-6 text-neutral-600">
                {{
                  identityConfigured
                    ? $t("views.messages.identityUnlockBody")
                    : $t("views.messages.identityCreateBody")
                }}
              </p>
              <div v-if="identityError" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                {{ identityError }}
              </div>
              <form class="mt-5 space-y-4" @submit.prevent="submitIdentityForm">
                <label class="block">
                  <span class="mb-1 block text-sm font-medium text-neutral-800">{{ $t("views.messages.passphraseLabel") }}</span>
                  <input
                    v-model="passphrase"
                    type="password"
                    autocomplete="current-password"
                    class="w-full rounded-2xl border border-neutral-200 px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                    :placeholder="identityConfigured ? $t('views.messages.passphrasePlaceholderUnlock') : $t('views.messages.passphrasePlaceholderNew')"
                  />
                </label>
                <label v-if="!identityConfigured" class="block">
                  <span class="mb-1 block text-sm font-medium text-neutral-800">{{ $t("views.messages.passphraseConfirmLabel") }}</span>
                  <input
                    v-model="passphraseConfirm"
                    type="password"
                    autocomplete="new-password"
                    class="w-full rounded-2xl border border-neutral-200 px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                    :placeholder="$t('views.messages.passphraseConfirmPlaceholder')"
                  />
                </label>
                <button
                  type="submit"
                  class="w-full rounded-2xl bg-lime-600 px-4 py-3 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
                  :disabled="identityBusy"
                >
                  {{
                    identityBusy
                      ? identityConfigured
                        ? $t("views.messages.identityBusyUnlocking")
                        : $t("views.messages.identityBusyCreating")
                      : identityConfigured
                        ? $t("views.messages.identitySubmitUnlock")
                        : $t("views.messages.identitySubmitCreate")
                  }}
                </button>
              </form>
            </div>
          </div>
        </template>

        <template v-else-if="fedActiveThreadId">
          <div class="border-b border-neutral-200 bg-white px-5 py-4">
            <p class="text-sm font-semibold text-neutral-900">連合DM</p>
            <p class="mt-1 text-sm text-neutral-500">{{ fedActiveRemoteAcct || fedActiveThreadId }}</p>
          </div>
          <div v-if="fedError" class="border-b border-red-200 bg-red-50 px-5 py-3 text-sm text-red-700">{{ fedError }}</div>
          <div v-if="fedLoading" class="flex flex-1 items-center justify-center px-6 py-10 text-sm text-neutral-500">読み込み中…</div>
          <div v-else class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
            <div v-if="!fedMessages.length" class="text-sm text-neutral-500">まだメッセージがありません。</div>
            <div v-else class="space-y-3">
              <div
                v-for="m in fedMessages"
                :key="m.id"
                class="max-w-[42rem] rounded-2xl border px-4 py-3 text-sm leading-6"
                :class="m.sent_by_me ? 'ml-auto border-neutral-200 bg-neutral-900 text-white' : 'mr-auto border-neutral-200 bg-white text-neutral-900'"
              >
                <p class="whitespace-pre-wrap">{{ m.text }}</p>
                <p v-if="m.decrypt_error" class="mt-2 text-xs opacity-70">復号エラー</p>
                <div v-if="m.attachments?.length" class="mt-3 space-y-2">
                  <div
                    v-for="(a, index) in m.attachments"
                    :key="`${m.id}:${index}`"
                    class="flex items-center justify-between gap-2 rounded-xl border border-neutral-200/30 bg-white/10 px-3 py-2 text-xs"
                    :class="m.sent_by_me ? 'border-white/15' : 'border-neutral-200 bg-neutral-50/80 text-neutral-800'"
                  >
                    <span class="min-w-0 flex-1 truncate">{{ a.file_name || a.public_url || 'attachment' }}</span>
                    <button
                      type="button"
                      class="shrink-0 rounded-full bg-white/15 px-3 py-1.5 font-semibold text-white hover:bg-white/20 disabled:opacity-50"
                      :class="m.sent_by_me ? '' : 'bg-neutral-900 text-white hover:bg-neutral-800'"
                      :disabled="attachmentBusyKey === `${m.id}:fed:${index}`"
                      @click="saveFederationAttachment(m, a, index)"
                    >
                      {{ attachmentBusyKey === `${m.id}:fed:${index}` ? $t("views.messages.decrypting") : $t("views.messages.saveAttachment") }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="border-t border-neutral-200 bg-white px-5 py-4">
            <div v-if="!fedCanSend" class="space-y-3">
              <p class="text-sm text-neutral-500">承認待ちのため送信できません。</p>
              <div v-if="fedActiveThread?.state === 'invited_inbound'" class="flex flex-wrap gap-2">
                <button
                  type="button"
                  class="rounded-2xl bg-lime-600 px-5 py-3 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
                  :disabled="fedSendBusy"
                  @click="acceptFederationThread"
                >
                  承認
                </button>
                <button
                  type="button"
                  class="rounded-2xl border border-red-200 bg-red-50 px-5 py-3 text-sm font-semibold text-red-800 hover:bg-red-100 disabled:opacity-50"
                  :disabled="fedSendBusy"
                  @click="rejectFederationThread"
                >
                  拒否
                </button>
              </div>
            </div>
            <form v-else class="flex items-end gap-3" @submit.prevent="sendFederationMessage">
              <label class="sr-only" for="fed-dm-input">連合DM</label>
              <button
                type="button"
                class="shrink-0 rounded-2xl border border-neutral-200 bg-white px-3 py-3 text-sm font-semibold text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
                :disabled="fedSendBusy"
                @click="triggerFedFilePicker"
                title="ファイルを添付"
              >
                ＋
              </button>
              <textarea
                id="fed-dm-input"
                v-model="fedComposerText"
                rows="2"
                placeholder="メッセージを入力…"
                class="min-h-[2.75rem] w-full resize-none rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              />
              <button
                type="submit"
                class="shrink-0 rounded-2xl bg-neutral-900 px-5 py-3 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
                :disabled="fedSendBusy || (!fedComposerText.trim() && !fedPendingFiles.length)"
              >
                送信
              </button>
              <input ref="fedFileInput" type="file" multiple class="hidden" @change="onPickFedFiles" />
            </form>
            <div v-if="fedPendingFiles.length" class="mt-3 flex flex-wrap gap-2">
              <div
                v-for="(file, index) in fedPendingFiles"
                :key="`${file.name}:${index}`"
                class="flex items-center gap-2 rounded-full border border-neutral-200 bg-neutral-50 px-3 py-1.5 text-xs text-neutral-800"
              >
                <span class="max-w-[14rem] truncate">{{ file.name }}</span>
                <button
                  type="button"
                  class="rounded-full bg-neutral-200 px-2 py-0.5 text-[11px] font-semibold text-neutral-700 hover:bg-neutral-300"
                  @click="removeFedPendingFile(index)"
                >
                  ×
                </button>
              </div>
            </div>
          </div>
        </template>

        <template v-else-if="activeThread">
          <header class="flex flex-wrap items-center justify-between gap-3 border-b border-neutral-200 bg-white px-5 py-4">
            <div class="flex items-center gap-3">
              <button
                type="button"
                class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100 lg:hidden"
                :aria-label="$t('views.messages.backToThreadsAria')"
                @click="router.push('/messages')"
              >
                <Icon name="back" class="h-5 w-5" stroke-width="2" />
              </button>
              <div>
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="text-base font-semibold text-neutral-900">{{ activeThreadTitle }}</h2>
                  <UserBadges :badges="activeThread.peer_badges" size="xs" />
                </div>
                <p class="text-sm text-neutral-500">@{{ activeThread.peer_handle }}</p>
              </div>
            </div>
          </header>

          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <div v-if="loadingMessages" class="px-5 py-4 text-sm text-neutral-500">{{ $t("views.messages.loadingMessages") }}</div>
            <div v-else class="relative flex min-h-0 flex-1 flex-col">
              <div v-if="notice" class="border-b border-neutral-200 bg-white px-5 py-3 text-sm text-neutral-600">
                {{ notice }}
              </div>
              <div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 py-4">
              <div v-if="!timelineItems.length" class="mx-auto mt-10 max-w-md rounded-2xl border border-dashed border-neutral-200 bg-white px-6 py-5 text-center text-sm text-neutral-500">
                {{ $t("views.messages.emptyThread") }}
              </div>
              <div v-else class="space-y-3">
                <div
                  v-for="item in timelineItems"
                  :key="item.id"
                  class="flex"
                  :class="item.sent_by_me ? 'justify-end' : 'justify-start'"
                >
                  <div
                    class="max-w-[min(100%,42rem)] rounded-2xl px-4 py-3 shadow-sm"
                    :class="item.sent_by_me ? 'bg-lime-600 text-white' : 'border border-neutral-200 bg-white text-neutral-900'"
                  >
                    <p class="mb-1 text-[11px]" :class="item.sent_by_me ? 'text-lime-100' : 'text-neutral-400'">
                      {{ item.message.sender_display_name }} · {{ formatDateTime(item.message.created_at, { dateStyle: "short", timeStyle: "short" }) }}
                    </p>
                    <UserBadges :badges="item.message.sender_badges" size="xs" />
                    <p class="whitespace-pre-wrap break-words text-sm leading-6">{{ item.message.decrypted_text }}</p>
                    <div v-if="item.kind === 'message' && item.message.attachments.length" class="mt-3 space-y-2">
                      <div
                        v-for="(attachment, index) in item.message.attachments"
                        :key="`${item.message.id}:${index}`"
                        class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2"
                        :class="item.sent_by_me ? 'border-lime-300/50 bg-lime-500/20' : 'border-neutral-200 bg-neutral-50'"
                      >
                        <div class="min-w-0">
                          <p class="truncate text-sm font-medium">{{ attachment.file_name }}</p>
                          <p class="text-xs opacity-80">{{ attachment.content_type }} · {{ formatBytes(attachment.size_bytes) }}</p>
                        </div>
                        <button
                          type="button"
                          class="shrink-0 rounded-full border px-3 py-1 text-xs font-semibold"
                          :class="item.sent_by_me ? 'border-white/40 text-white hover:bg-white/10' : 'border-neutral-200 text-neutral-700 hover:bg-white'"
                          :disabled="attachmentBusyKey === `${item.message.id}:${index}`"
                          @click="saveAttachment(item.message, attachment, index)"
                        >
                          {{ attachmentBusyKey === `${item.message.id}:${index}` ? $t("views.messages.decrypting") : $t("views.messages.saveAttachment") }}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              </div>
            </div>

            <div class="border-t border-neutral-200 bg-white px-4 py-4">
              <div v-if="pendingFiles.length" class="mb-3 flex flex-wrap gap-2">
                <span
                  v-for="(file, index) in pendingFiles"
                  :key="`${file.name}:${file.size}:${index}`"
                  class="inline-flex items-center gap-2 rounded-full border border-neutral-200 bg-neutral-50 px-3 py-1 text-xs text-neutral-700"
                >
                  <span class="max-w-[12rem] truncate">{{ file.name }}</span>
                  <button type="button" class="text-neutral-500 hover:text-neutral-800" @click="removePendingFile(index)">×</button>
                </span>
              </div>
              <div class="flex flex-col gap-3">
                <textarea
                  v-model="composerText"
                  rows="4"
                  class="w-full rounded-2xl border border-neutral-200 px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                  :placeholder="$t('views.messages.composerPlaceholder')"
                />
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <div class="flex items-center gap-2">
                    <input ref="fileInput" type="file" multiple class="hidden" @change="onPickFiles" />
                    <button
                      type="button"
                      class="rounded-full border border-neutral-200 bg-white px-3 py-1.5 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
                      @click="triggerFilePicker"
                    >
                      {{ $t("views.messages.addFile") }}
                    </button>
                    <span class="text-xs text-neutral-500">{{ $t("views.messages.fileTypesHint") }}</span>
                  </div>
                  <button
                    type="button"
                    class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
                    :disabled="sendBusy || threadBusy"
                    @click="sendCurrentMessage"
                  >
                    {{ sendBusy ? $t("views.messages.sending") : $t("views.messages.send") }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </template>

        <div v-else class="flex flex-1 items-center justify-center px-6 text-center text-sm text-neutral-500">
          <div>
            <p class="text-base font-medium text-neutral-700">{{ $t("views.messages.selectConversationTitle") }}</p>
            <p class="mt-2">{{ $t("views.messages.selectConversationHint") }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
