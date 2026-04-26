export type VideoEmbed = {
  url: string;
  provider: "youtube" | "vimeo" | "dailymotion" | "loom" | "nicovideo" | "streamable" | "wistia" | "bilibili" | "tiktok" | "steam";
  embedKind: "iframe";
  embedUrl: string;
  title: string;
  layout?: "video" | "steam";
};

export function extractVideoEmbeds(urls: readonly string[]): VideoEmbed[] {
  const out: VideoEmbed[] = [];
  const seen = new Set<string>();
  for (const url of urls) {
    const embed = parseVideoEmbed(url);
    if (!embed) continue;
    if (seen.has(embed.embedUrl)) continue;
    seen.add(embed.embedUrl);
    out.push(embed);
  }
  return out;
}

function parseVideoEmbed(raw: string): VideoEmbed | null {
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    const host = url.hostname.toLowerCase().replace(/^www\./, "");

    if (host === "youtu.be" || host === "youtube.com" || host === "m.youtube.com" || host === "music.youtube.com") {
      return parseYouTubeEmbed(url, raw);
    }
    if (host === "vimeo.com" || host === "player.vimeo.com") {
      return parseVimeoEmbed(url, raw);
    }
    if (host === "dailymotion.com" || host === "dai.ly") {
      return parseDailymotionEmbed(url, raw);
    }
    if (host === "loom.com" || host === "www.loom.com") {
      return parseLoomEmbed(url, raw);
    }
    if (host === "nicovideo.jp" || host === "sp.nicovideo.jp" || host === "nico.ms") {
      return parseNicovideoEmbed(url, raw);
    }
    if (host === "streamable.com") {
      return parseStreamableEmbed(url, raw);
    }
    if (host === "wistia.com" || host.endsWith(".wistia.com") || host === "wi.st") {
      return parseWistiaEmbed(url, raw);
    }
    if (host === "bilibili.com" || host.endsWith(".bilibili.com")) {
      return parseBilibiliEmbed(url, raw);
    }
    if (host === "tiktok.com" || host.endsWith(".tiktok.com")) {
      return parseTikTokEmbed(url, raw);
    }
    if (host === "store.steampowered.com") {
      return parseSteamEmbed(url, raw);
    }
    return null;
  } catch {
    return null;
  }
}

function parseYouTubeEmbed(url: URL, raw: string): VideoEmbed | null {
  const host = url.hostname.toLowerCase().replace(/^www\./, "");
  const parts = url.pathname.split("/").filter(Boolean);
  let videoId = "";
  if (host === "youtu.be") {
    videoId = parts[0] ?? "";
  } else if (parts[0] === "watch") {
    videoId = url.searchParams.get("v") ?? "";
  } else if (parts[0] === "shorts" || parts[0] === "embed" || parts[0] === "live") {
    videoId = parts[1] ?? "";
  }
  if (!/^[A-Za-z0-9_-]{6,}$/.test(videoId)) return null;
  const start = parseYouTubeStart(url);
  const embedUrl = new URL(`https://www.youtube-nocookie.com/embed/${videoId}`);
  if (start > 0) embedUrl.searchParams.set("start", String(start));
  return {
    url: raw,
    provider: "youtube",
    embedKind: "iframe",
    embedUrl: embedUrl.toString(),
    title: "YouTube video player",
  };
}

function parseYouTubeStart(url: URL): number {
  const raw = url.searchParams.get("t") ?? url.searchParams.get("start") ?? "";
  if (!raw) return 0;
  if (/^\d+$/.test(raw)) return Number(raw);
  const match = raw.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/i);
  if (!match) return 0;
  const hours = Number(match[1] ?? 0);
  const minutes = Number(match[2] ?? 0);
  const seconds = Number(match[3] ?? 0);
  return (hours * 3600) + (minutes * 60) + seconds;
}

function parseVimeoEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  const id = parts.find((part) => /^\d+$/.test(part)) ?? "";
  if (!id) return null;
  return {
    url: raw,
    provider: "vimeo",
    embedKind: "iframe",
    embedUrl: `https://player.vimeo.com/video/${id}`,
    title: "Vimeo video player",
  };
}

function parseDailymotionEmbed(url: URL, raw: string): VideoEmbed | null {
  const host = url.hostname.toLowerCase().replace(/^www\./, "");
  const parts = url.pathname.split("/").filter(Boolean);
  const id = host === "dai.ly"
    ? (parts[0] ?? "")
    : parts[0] === "video"
      ? (parts[1] ?? "")
      : "";
  if (!/^[A-Za-z0-9]+$/.test(id)) return null;
  return {
    url: raw,
    provider: "dailymotion",
    embedKind: "iframe",
    embedUrl: `https://www.dailymotion.com/embed/video/${id}`,
    title: "Dailymotion video player",
  };
}

function parseLoomEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts[0] !== "share" && parts[0] !== "embed") return null;
  const id = parts[1] ?? "";
  if (!/^[A-Za-z0-9]+$/.test(id)) return null;
  return {
    url: raw,
    provider: "loom",
    embedKind: "iframe",
    embedUrl: `https://www.loom.com/embed/${id}`,
    title: "Loom video player",
  };
}

function parseNicovideoEmbed(url: URL, raw: string): VideoEmbed | null {
  void url;
  void raw;
  return null;
}

function parseStreamableEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  const id = parts[0] === "e" ? (parts[1] ?? "") : (parts[0] ?? "");
  if (!/^[a-z0-9]+$/i.test(id)) return null;
  return {
    url: raw,
    provider: "streamable",
    embedKind: "iframe",
    embedUrl: `https://streamable.com/e/${id}`,
    title: "Streamable video player",
  };
}

function parseWistiaEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  let mediaId = "";
  if (parts[0] === "medias") {
    mediaId = parts[1] ?? "";
  } else if (parts[0] === "embed" && parts[1] === "iframe") {
    mediaId = parts[2] ?? "";
  }
  if (!/^[a-z0-9]+$/i.test(mediaId)) return null;
  return {
    url: raw,
    provider: "wistia",
    embedKind: "iframe",
    embedUrl: `https://fast.wistia.net/embed/iframe/${mediaId}`,
    title: "Wistia video player",
  };
}

function parseBilibiliEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts[0] !== "video") return null;
  const id = parts[1] ?? "";
  const page = Math.max(1, Number(url.searchParams.get("p") ?? "1") || 1);
  if (/^BV[0-9A-Za-z]+$/i.test(id)) {
    return {
      url: raw,
      provider: "bilibili",
      embedKind: "iframe",
      embedUrl: `https://player.bilibili.com/player.html?bvid=${encodeURIComponent(id)}&page=${page}`,
      title: "Bilibili video player",
    };
  }
  if (/^av\d+$/i.test(id)) {
    return {
      url: raw,
      provider: "bilibili",
      embedKind: "iframe",
      embedUrl: `https://player.bilibili.com/player.html?aid=${encodeURIComponent(id.slice(2))}&page=${page}`,
      title: "Bilibili video player",
    };
  }
  return null;
}

function parseTikTokEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  const videoIndex = parts.findIndex((part) => part === "video");
  const id = videoIndex >= 0 ? (parts[videoIndex + 1] ?? "") : "";
  if (!/^\d{6,}$/.test(id)) return null;
  return {
    url: raw,
    provider: "tiktok",
    embedKind: "iframe",
    embedUrl: `https://www.tiktok.com/embed/v2/${id}`,
    title: "TikTok video player",
  };
}

function parseSteamEmbed(url: URL, raw: string): VideoEmbed | null {
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts[0] !== "app") return null;
  const appId = parts[1] ?? "";
  if (!/^\d+$/.test(appId)) return null;
  return {
    url: raw,
    provider: "steam",
    embedKind: "iframe",
    embedUrl: `https://store.steampowered.com/widget/${appId}/`,
    title: "Steam store widget",
    layout: "steam",
  };
}
