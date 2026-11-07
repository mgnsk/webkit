// Service worker: network only, with an offline fallback page for
// navigations. Data is never cached so pages are never stale.
const config = /*CONFIG*/{};

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(config.cache).then((cache) => cache.addAll(config.precache)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((key) => key !== config.cache).map((key) => caches.delete(key))),
    ),
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.mode === "navigate") {
    if (config.offlinePage) {
      event.respondWith(fetch(request).catch(() => caches.match(config.offlinePage, { cacheName: config.cache })));
    }
    return;
  }
  // Precached files, e.g. the offline page's stylesheet, while offline.
  const url = new URL(request.url);
  if (request.method === "GET" && url.origin === self.location.origin && config.precache.includes(url.pathname + url.search)) {
    event.respondWith(
      fetch(request).catch(async () => (await caches.match(request, { cacheName: config.cache })) || Response.error()),
    );
  }
});

// Show a push notification. iOS revokes the subscription of apps that
// receive pushes without showing a notification, so one is always shown.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data.text() };
  }
  event.waitUntil(
    self.registration.showNotification(data.title || config.title, {
      body: data.body || "",
      tag: data.tag || undefined,
      icon: config.icon || undefined,
      badge: config.icon || undefined,
      data: { url: data.url || "/" },
    }),
  );
});

// Open the notification's page. An open app window is told to navigate
// itself: WindowClient.navigate() is unreliable in the iOS Home Screen app
// and openWindow() does nothing there when the app is already open.
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL(event.notification.data?.url || "/", self.location.origin);
  if (url.origin !== self.location.origin) {
    return;
  }
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const client =
        windows.find((c) => c.focused) || windows.find((c) => c.visibilityState === "visible") || windows[0];
      if (!client) {
        return self.clients.openWindow(url.href);
      }
      client.postMessage({ type: "navigate", url: url.pathname + url.search });
      return client.focus().catch(() => {});
    })(),
  );
});
