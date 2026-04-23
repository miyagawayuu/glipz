import { apiBase } from "./api";
import { fullHandleAt } from "./feedDisplay";
import { translate } from "../i18n";

export type NotifyPayload = {
  v: number;
  id: string;
  kind: "reply" | "like" | "repost" | "follow" | "dm_invite" | "dm_message";
  actor_handle: string;
  actor_display_name: string;
  subject_post_id?: string | null;
  actor_post_id?: string | null;
  subject_author_handle?: string | null;
  created_at: string;
  read_at?: string | null;
};

function consumeSseBuffer(buf: string, onDataLine: (line: string) => void): string {
  for (;;) {
    const sep = buf.indexOf("\n\n");
    if (sep < 0) return buf;
    const block = buf.slice(0, sep);
    buf = buf.slice(sep + 2);
    for (const line of block.split("\n")) {
      if (line.startsWith("data:")) {
        onDataLine(line.slice(5).trimStart());
      }
    }
  }
}

export function notifyToastMessage(p: NotifyPayload): string {
  const name = p.actor_display_name?.trim() || fullHandleAt(p.actor_handle);
  switch (p.kind) {
    case "reply":
      return translate("notifications.reply", { name });
    case "like":
      return translate("notifications.like", { name });
    case "repost":
      return translate("notifications.repost", { name });
    case "follow":
      return translate("notifications.follow", { name });
    case "dm_invite":
      return translate("notifications.dmInvite", { name });
    case "dm_message":
      return translate("notifications.dmMessage", { name });
    default:
      return translate("notifications.generic");
  }
}

export function connectNotifyStream(opts: {
  token: string;
  onPayload: (p: NotifyPayload) => void;
  onError?: (e: unknown) => void;
}): () => void {
  const ac = new AbortController();
  const url = `${apiBase()}/api/v1/notifications/stream`;

  void (async () => {
    try {
      const res = await fetch(url, {
        method: "GET",
        headers: { Accept: "text/event-stream", Authorization: `Bearer ${opts.token}` },
        signal: ac.signal,
      });
      if (!res.ok || !res.body) {
        opts.onError?.(new Error(String(res.status)));
        return;
      }
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = "";
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        buf = consumeSseBuffer(buf, (line) => {
          if (!line || line.startsWith(":")) return;
          try {
            const p = JSON.parse(line) as NotifyPayload;
            if (p?.kind && p?.id) opts.onPayload(p);
          } catch {
            /* ignore */
          }
        });
      }
    } catch (e: unknown) {
      if ((e as Error)?.name === "AbortError") return;
      opts.onError?.(e);
    }
  })();

  return () => ac.abort();
}
