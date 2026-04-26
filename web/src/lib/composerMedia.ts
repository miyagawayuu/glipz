/** Max image attachments per post (server matches). */
export const MAX_COMPOSER_IMAGE_SLOTS = 4;

export const SAFE_MEDIA_ACCEPT =
  "image/avif,image/gif,image/jpeg,image/png,image/webp,video/mp4,video/quicktime,video/webm,audio/aac,audio/flac,audio/mp4,audio/mpeg,audio/ogg,audio/wav,audio/webm,audio/x-wav";

export const SAFE_PROFILE_IMAGE_ACCEPT = "image/avif,image/gif,image/jpeg,image/png,image/webp";

const SAFE_MEDIA_TYPES = new Set(SAFE_MEDIA_ACCEPT.split(","));
const SAFE_PROFILE_IMAGE_TYPES = new Set(SAFE_PROFILE_IMAGE_ACCEPT.split(","));

export function isSafeComposerMediaFile(file: File): boolean {
  return SAFE_MEDIA_TYPES.has(file.type);
}

export function isSafeProfileImageFile(file: File): boolean {
  return SAFE_PROFILE_IMAGE_TYPES.has(file.type);
}

export function inferPostMediaType(files: File[]): "none" | "image" | "video" | "audio" {
  if (!files.length) return "none";
  const t = files[0].type;
  if (t.startsWith("video/")) return "video";
  if (t.startsWith("audio/")) return "audio";
  return "image";
}

export function composerAttachmentLabel(files: File[]): string {
  const mt = inferPostMediaType(files);
  if (mt === "video" || mt === "audio") return "1/1";
  return `${files.length}/${MAX_COMPOSER_IMAGE_SLOTS}`;
}

/**
 * Merges newly picked files into the composer selection.
 * Video and audio each occupy the whole post; images allow up to MAX_COMPOSER_IMAGE_SLOTS.
 */
export function mergePickedComposerFiles(
  existing: File[],
  picked: File[],
): { files: File[]; replacedKind: boolean; partialImageDrop: boolean; excludedImages: number } {
  const valid = picked.filter(isSafeComposerMediaFile);
  if (!valid.length) {
    return { files: existing, replacedKind: false, partialImageDrop: false, excludedImages: 0 };
  }

  const firstVideo = valid.find((f) => f.type.startsWith("video/"));
  if (firstVideo) {
    const replacedKind = existing.length > 0 && !existing.every((f) => f.type.startsWith("video/"));
    return { files: [firstVideo], replacedKind, partialImageDrop: false, excludedImages: 0 };
  }

  const firstAudio = valid.find((f) => f.type.startsWith("audio/"));
  if (firstAudio) {
    const replacedKind = existing.length > 0 && !existing.every((f) => f.type.startsWith("audio/"));
    return { files: [firstAudio], replacedKind, partialImageDrop: false, excludedImages: 0 };
  }

  const images = valid.filter((f) => f.type.startsWith("image/"));
  const exWasVideoOrAudio = existing.some((f) => f.type.startsWith("video/") || f.type.startsWith("audio/"));
  const base = exWasVideoOrAudio ? [] : [...existing];
  const cap = Math.max(0, MAX_COMPOSER_IMAGE_SLOTS - base.length);
  const toAdd = images.slice(0, cap);
  const excludedImages = images.length - toAdd.length;
  return {
    files: [...base, ...toAdd],
    replacedKind: exWasVideoOrAudio,
    partialImageDrop: excludedImages > 0,
    excludedImages,
  };
}
