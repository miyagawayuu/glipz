import { uploadMediaFile } from "./api";

/** Uploads an image or video and returns the public URL to embed in note content. */
export async function uploadNoteMedia(file: File, token: string): Promise<string> {
  const up = await uploadMediaFile(token, file);
  return up.public_url;
}
