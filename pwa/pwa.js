// Client script of the pwa package: registers the service worker, follows
// notification taps and exposes the push API. See the package documentation.
(() => {
  const webkit = (window.webkit = window.webkit || {});

  const hasServiceWorker = "serviceWorker" in navigator;
  if (hasServiceWorker) {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  }

  const pwa = (webkit.pwa = {
    // Running as the installed app rather than in a browser tab.
    standalone: window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true,
    // About to navigate to the page of a tapped notification.
    navigating: false,
  });

  // Tapping a notification while the app is open: go to its page. Assigning
  // the current URL reloads it, so the view is fresh either way.
  if (hasServiceWorker) {
    navigator.serviceWorker.addEventListener("message", (event) => {
      const data = event.data;
      if (
        !data ||
        data.type !== "navigate" ||
        typeof data.url !== "string" ||
        !data.url.startsWith("/") ||
        data.url.startsWith("//")
      ) {
        return;
      }
      pwa.navigating = true;
      window.location.assign(data.url);
    });
    navigator.serviceWorker.startMessages();
  }

  const supported = hasServiceWorker && "PushManager" in window && "Notification" in window;

  function post(path, sub) {
    return fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sub),
    }).then((resp) => {
      if (!resp.ok) {
        throw new Error(`${path}: ${resp.status}`);
      }
    });
  }

  function base64UrlToBytes(s) {
    const b64 = (s + "=".repeat((4 - (s.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/");
    return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
  }

  async function fetchKey() {
    const resp = await fetch("/push/key", { credentials: "same-origin" });
    if (!resp.ok) {
      throw new Error(`/push/key: ${resp.status}`);
    }
    return (await resp.text()).trim();
  }

  let devicePromise;

  // device resolves to the push device once the service worker is ready and
  // the VAPID key is known, or to null without push support.
  function device() {
    if (!supported) {
      return Promise.resolve(null);
    }
    devicePromise =
      devicePromise ||
      Promise.all([navigator.serviceWorker.ready, fetchKey()]).then(([reg, key]) => ({
        subscribed: async () => (await reg.pushManager.getSubscription()) !== null,

        // subscribe calls pushManager.subscribe before any await, so that
        // iOS shows the permission prompt in a tap handler.
        subscribe: async () => {
          const sub = await reg.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey: base64UrlToBytes(key),
          });
          try {
            await post("/push/subscribe", sub);
          } catch (err) {
            await sub.unsubscribe();
            throw err;
          }
        },

        unsubscribe: async () => {
          const sub = await reg.pushManager.getSubscription();
          if (sub) {
            await post("/push/unsubscribe", sub).catch(() => {});
            await sub.unsubscribe();
          }
        },
      }));
    devicePromise.catch(() => {
      devicePromise = undefined;
    });
    return devicePromise;
  }

  async function currentSubscription() {
    if (!supported) {
      return null;
    }
    const reg = await navigator.serviceWorker.ready;
    return reg.pushManager.getSubscription();
  }

  const syncedKey = "webkit-push-synced";

  webkit.push = {
    supported,
    device,

    // sync stores this device's subscription for the signed-in user once per
    // browser session.
    async sync() {
      if (sessionStorage.getItem(syncedKey)) {
        return;
      }
      const sub = await currentSubscription();
      if (sub && Notification.permission === "granted") {
        await post("/push/subscribe", sub);
      }
      sessionStorage.setItem(syncedKey, "1");
    },

    // unsubscribe stops notifications for this device, e.g. before signing
    // out. serviceWorker.ready never resolves without a service worker, so it
    // gives up after 3 seconds.
    async unsubscribe() {
      sessionStorage.removeItem(syncedKey);
      const timeout = new Promise((resolve) => setTimeout(resolve, 3000, null));
      const sub = await Promise.race([currentSubscription(), timeout]);
      if (sub) {
        await post("/push/unsubscribe", sub);
        await sub.unsubscribe();
      }
    },
  };
})();
