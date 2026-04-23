self.addEventListener("install", (event) => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  event.waitUntil((async () => {
    let payload = {};
    try {
      payload = event.data ? event.data.json() : {};
    } catch {
      payload = {};
    }

    const clientsList = await self.clients.matchAll({
      type: "window",
      includeUncontrolled: true,
    });
    const hasVisibleClient = clientsList.some((client) => client.visibilityState === "visible");
    if (hasVisibleClient) {
      return;
    }

    const title = typeof payload.title === "string" && payload.title.trim() ? payload.title : "Glipz";
    const body = typeof payload.body === "string" ? payload.body : "";
    const url = typeof payload.url === "string" && payload.url.trim() ? payload.url : "/";
    await self.registration.showNotification(title, {
      body,
      icon: typeof payload.icon === "string" && payload.icon ? payload.icon : "/icon.svg",
      badge: typeof payload.badge === "string" && payload.badge ? payload.badge : "/badge.svg",
      tag: typeof payload.tag === "string" ? payload.tag : undefined,
      requireInteraction: payload.require_interaction === true,
      data: {
        url,
        kind: typeof payload.kind === "string" ? payload.kind : "",
        threadId: typeof payload.thread_id === "string" ? payload.thread_id : "",
        notificationId: typeof payload.notification_id === "string" ? payload.notification_id : "",
      },
    });
  })());
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil((async () => {
    const rawUrl = event.notification?.data?.url || "/";
    const targetUrl = new URL(rawUrl, self.location.origin).href;
    const clientsList = await self.clients.matchAll({
      type: "window",
      includeUncontrolled: true,
    });
    for (const client of clientsList) {
      if (!("focus" in client)) continue;
      await client.focus();
      if ("navigate" in client && client.url !== targetUrl) {
        await client.navigate(targetUrl);
      }
      return;
    }
    if (self.clients.openWindow) {
      await self.clients.openWindow(targetUrl);
    }
  })());
});
