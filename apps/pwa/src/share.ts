// Web Share Target support. When Satark is installed on Android it shows
// up in the share menu. The service worker receives the POST to /share,
// parks the payload in the "satark-share" cache and redirects to
// /?shared=1, where the page picks it up and runs the check.

export const SHARE_CACHE = "satark-share";
export const SHARE_TEXT_KEY = "/__share/text";
export const SHARE_FILE_KEY = "/__share/file";

/**
 * Combines the share fields into one input. Apps are inconsistent: some
 * put the link in `url`, others append it to `text`, and some repeat it.
 */
export function combineShared(title?: string | null, text?: string | null, url?: string | null): string {
  const parts: string[] = [];
  for (const p of [title, text, url]) {
    const s = (p ?? "").trim();
    if (s && !parts.some((x) => x.includes(s))) parts.push(s);
  }
  return parts.join("\n");
}

export interface SharedPayload {
  text?: string;
  file?: Blob;
}

/** Reads and clears a pending share. Returns null when there is none. */
export async function takeShared(cacheStorage: CacheStorage = caches): Promise<SharedPayload | null> {
  const cache = await cacheStorage.open(SHARE_CACHE);
  const [t, f] = await Promise.all([cache.match(SHARE_TEXT_KEY), cache.match(SHARE_FILE_KEY)]);
  if (!t && !f) return null;
  const payload: SharedPayload = {};
  if (t) payload.text = await t.text();
  if (f) payload.file = await f.blob();
  await Promise.all([cache.delete(SHARE_TEXT_KEY), cache.delete(SHARE_FILE_KEY)]);
  return payload;
}
