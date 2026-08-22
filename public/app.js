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

// Show/hide the password so members can check what they typed. Uses event
// delegation because htmx re-renders the login form on every failed attempt.
function onTogglePasswordClick(e) {
  const btn = e.target.closest(".toggle-pass");
  if (!btn) return;
  const input = btn.closest(".pass")?.querySelector("input");
  if (!input) return;
  const show = input.type === "password";
  input.type = show ? "text" : "password";
  btn.textContent = show ? "Hide" : "Show";
  btn.setAttribute("aria-pressed", String(show));
}
document.addEventListener("click", onTogglePasswordClick);

// After a failed login htmx replaces the form, so focus is lost. Return it to
// the email field, unless the form rendered without an error.
document.addEventListener("htmx:afterSwap", () => {
  const view = document.getElementById("loginView");
  if (!view) return;
  const email = view.querySelector("#email");
  if (email && view.querySelector(".error")?.textContent.trim()) {
    email.focus();
  }
});

// Keep ARIA state in sync with the browser's :user-invalid styling so screen
// readers and visuals announce validation at the same moment.
function setAriaInvalid(target) {
  if (!(target instanceof HTMLInputElement)) return;
  if (target.matches(":user-invalid")) {
    target.setAttribute("aria-invalid", "true");
  } else {
    target.removeAttribute("aria-invalid");
  }
}
document.addEventListener("blur", (e) => setAriaInvalid(e.target), true);
document.addEventListener("focus", (e) => setAriaInvalid(e.target), true);
document.addEventListener("input", (e) => {
  if (e.target instanceof HTMLInputElement && e.target.hasAttribute("aria-invalid")) {
    setAriaInvalid(e.target);
  }
});
