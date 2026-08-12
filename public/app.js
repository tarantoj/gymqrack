if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js");
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
