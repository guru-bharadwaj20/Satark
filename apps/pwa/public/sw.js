// Service worker: caches the app shell so Satark opens offline.
// API calls are never cached (results must be fresh and may contain user text).
const CACHE = "satark-shell-v1";
const SHELL = ["/", "/index.html", "/manifest.webmanifest", "/icon.svg"];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))),
  );
  self.clients.claim();
});

// Web Share Target: keep in sync with src/share.ts.
const SHARE_CACHE = "satark-share";
const SHARE_TEXT_KEY = "/__share/text";
const SHARE_FILE_KEY = "/__share/file";

async function handleShare(request) {
  try {
    const form = await request.formData();
    const parts = [];
    for (const k of ["title", "text", "url"]) {
      const v = (form.get(k) || "").toString().trim();
      if (v && !parts.some((p) => p.includes(v))) parts.push(v);
    }
    const cache = await caches.open(SHARE_CACHE);
    await cache.delete(SHARE_TEXT_KEY);
    await cache.delete(SHARE_FILE_KEY);
    if (parts.length) await cache.put(SHARE_TEXT_KEY, new Response(parts.join("\n").slice(0, 5000)));
    const file = form.get("image");
    if (file && typeof file !== "string" && file.size > 0 && file.size <= 10 * 1024 * 1024) {
      await cache.put(SHARE_FILE_KEY, new Response(file, { headers: { "Content-Type": file.type } }));
    }
  } catch (_) {
    // Fall through: the page will simply open without a shared item.
  }
  return Response.redirect("/?shared=1", 303);
}

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method === "POST" && url.origin === self.location.origin && url.pathname === "/share") {
    event.respondWith(handleShare(event.request));
    return;
  }
  if (event.request.method !== "GET" || url.origin !== self.location.origin || url.pathname.startsWith("/api/")) {
    return;
  }
  // Network first, falling back to the cached shell.
  event.respondWith(
    fetch(event.request)
      .then((res) => {
        if (res.ok) {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(event.request, copy));
        }
        return res;
      })
      .catch(() => caches.match(event.request).then((r) => r || caches.match("/index.html"))),
  );
});
