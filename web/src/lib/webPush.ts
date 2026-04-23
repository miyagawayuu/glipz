import { api } from "./api";
import { getAccessToken } from "../auth";

export type WebPushConfig = {
  available: boolean;
  vapid_public_key?: string;
  subscription_count: number;
};

type WebPushSubscriptionJSON = {
  endpoint: string;
  keys?: {
    p256dh?: string;
    auth?: string;
  };
};

let serviceWorkerRegistrationPromise: Promise<ServiceWorkerRegistration | null> | null = null;

export function isWebPushSupported(): boolean {
  return typeof window !== "undefined"
    && "serviceWorker" in navigator
    && "PushManager" in window
    && "Notification" in window;
}

export function currentNotificationPermission(): NotificationPermission | "unsupported" {
  if (!isWebPushSupported()) return "unsupported";
  return Notification.permission;
}

export async function registerPushServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!isWebPushSupported()) return null;
  if (!serviceWorkerRegistrationPromise) {
    serviceWorkerRegistrationPromise = navigator.serviceWorker.register("/sw.js").catch(() => null);
  }
  return serviceWorkerRegistrationPromise;
}

export async function fetchWebPushConfig(): Promise<WebPushConfig> {
  const token = getAccessToken();
  if (!token) {
    throw new Error("unauthorized");
  }
  return api<WebPushConfig>("/api/v1/me/web-push", {
    method: "GET",
    token,
  });
}

export async function getCurrentPushSubscription(): Promise<PushSubscription | null> {
  const registration = await registerPushServiceWorker();
  if (!registration) return null;
  return registration.pushManager.getSubscription();
}

export async function syncExistingPushSubscription(): Promise<boolean> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const existing = await getCurrentPushSubscription();
  if (!existing) return false;
  await sendSubscriptionToServer(token, existing);
  return true;
}

export async function enableWebPush(vapidPublicKey: string): Promise<void> {
  if (!isWebPushSupported()) {
    throw new Error("web_push_unsupported");
  }
  const token = getAccessToken();
  if (!token) {
    throw new Error("unauthorized");
  }
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    throw new Error("notification_permission_denied");
  }
  const registration = await registerPushServiceWorker();
  if (!registration) {
    throw new Error("service_worker_registration_failed");
  }
  let subscription = await registration.pushManager.getSubscription();
  if (!subscription) {
    subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKey),
    });
  }
  await sendSubscriptionToServer(token, subscription);
}

export async function disableWebPush(): Promise<void> {
  const token = getAccessToken();
  if (!token) {
    throw new Error("unauthorized");
  }
  const subscription = await getCurrentPushSubscription();
  if (!subscription) return;
  await api("/api/v1/me/web-push/unsubscribe", {
    method: "POST",
    token,
    json: { endpoint: subscription.endpoint },
  });
  await subscription.unsubscribe().catch(() => undefined);
}

async function sendSubscriptionToServer(token: string, subscription: PushSubscription): Promise<void> {
  const data = subscription.toJSON() as WebPushSubscriptionJSON;
  if (!data.endpoint || !data.keys?.p256dh || !data.keys?.auth) {
    throw new Error("invalid_subscription");
  }
  await api("/api/v1/me/web-push/subscription", {
    method: "PUT",
    token,
    json: {
      endpoint: data.endpoint,
      keys: {
        p256dh: data.keys.p256dh,
        auth: data.keys.auth,
      },
    },
  });
}

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padded = `${base64String}${"=".repeat((4 - (base64String.length % 4)) % 4)}`;
  const base64 = padded.replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(base64);
  const output = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i += 1) {
    output[i] = raw.charCodeAt(i);
  }
  return output;
}
