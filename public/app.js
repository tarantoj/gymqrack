if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js");
}

// Apple Watch: the viewport width/height media query normally matches the
// watch, but newer watchOS builds may render at a wider virtual viewport, so
// also flag it via the UA string. When set, the page shows only the QR code.
if (/Watch/.test(navigator.userAgent)) {
  document.documentElement.classList.add("watch");
}

let wakeLock = null;

// Keep the screen on while the QR is shown, release it otherwise.
async function syncWakeLock() {
  const showQr = document.getElementById("qrView") !== null;
  if (!showQr) {
    if (wakeLock) {
      wakeLock.release();
      wakeLock = null;
    }
    return;
  }
  if (!("wakeLock" in navigator) || document.hidden || wakeLock) return;
  try {
    wakeLock = await navigator.wakeLock.request("screen");
    wakeLock.addEventListener("release", () => {
      wakeLock = null;
    });
  } catch {}
}

// Returning to the foreground: re-acquire the wake lock and force an
// immediate QR refresh (the 60s poll may have gone stale while suspended).
function onVisibilityChange() {
  if (document.hidden) return;
  syncWakeLock();
  const el = document.getElementById("qrView");
  if (el && window.htmx) {
    htmx.trigger(el, "tabvisible");
  }
}

document.addEventListener("visibilitychange", onVisibilityChange);
document.addEventListener("htmx:afterSwap", () => syncWakeLock());
syncWakeLock();
