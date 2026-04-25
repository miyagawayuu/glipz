import { apiBase } from "./api";
import { translate } from "../i18n";

export type DmStreamPayload = {
  v: number;
  kind:
    | "message"
    | "federation_dm_invite"
    | "federation_dm_accept"
    | "federation_dm_reject"
    | "federation_dm_message";
  thread_id: string;
  message_id?: string;
  sender_handle: string;
  sender_display_name: string;
  sender_badges?: string[];
  created_at: string;
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

export function dmToastMessage(p: DmStreamPayload): string {
  const name = p.sender_display_name || `@${p.sender_handle}`;
  if (p.kind === "federation_dm_invite") {
    return translate("notifications.dmInvite", { name });
  }
  return translate("notifications.dmMessage", { name });
}

export function connectDMStream(opts: {
  token: string;
  onPayload: (p: DmStreamPayload) => void;
  onError?: (e: unknown) => void;
}): () => void {
  const ac = new AbortController();
  const url = `${apiBase()}/api/v1/dm/stream`;

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
            const p = JSON.parse(line) as DmStreamPayload;
            if (p?.thread_id && (p.kind !== "message" || p?.message_id)) opts.onPayload(p);
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
