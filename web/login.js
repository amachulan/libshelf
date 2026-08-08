const form = document.getElementById("login-form");
const errorEl = document.getElementById("error");
const userInput = document.getElementById("username");
const passInput = document.getElementById("password");

async function fetchMe() {
  const res = await fetch("/api/me?_=" + Date.now(), {
    credentials: "same-origin",
    cache: "no-store",
    headers: { "Cache-Control": "no-cache", Pragma: "no-cache" },
  });
  if (res.status === 401) return null;
  if (!res.ok) return null;
  const data = await res.json();
  return data.auth && data.user ? data.user : null;
}

// Clean up diagnostic redirect flag from older builds.
if (location.search) {
  history.replaceState(null, "", "/login.html");
}

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  errorEl.classList.add("hidden");

  // Read at submit time (after autofill), then freeze so the manager can't swap fields mid-request.
  const username = userInput.value.trim();
  const password = passInput.value;
  userInput.readOnly = true;
  passInput.readOnly = true;

  try {
    // Drop any leftover admin/reader cookie before switching accounts.
    await fetch("/api/logout", { method: "POST", credentials: "same-origin", cache: "no-store" });
    const res = await fetch("/api/login", {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) {
      errorEl.textContent = "Неверный логин или пароль";
      errorEl.classList.remove("hidden");
      return;
    }
    const user = await res.json();
    if (!user || !user.username || user.username.toLowerCase() !== username.toLowerCase()) {
      errorEl.textContent = "Сервер вернул другого пользователя — попробуйте ещё раз";
      errorEl.classList.remove("hidden");
      await fetch("/api/logout", { method: "POST", credentials: "same-origin", cache: "no-store" });
      return;
    }
    // Confirm the browser actually attached the new session (not a stale cached /api/me).
    const me = await fetchMe();
    if (!me || String(me.username).toLowerCase() !== username.toLowerCase()) {
      errorEl.textContent = me
        ? `Сессия осталась как «${me.username}». Очистите cookies для сайта и войдите снова.`
        : "Сессия не установилась — очистите cookies для сайта и войдите снова.";
      errorEl.classList.remove("hidden");
      await fetch("/api/logout", { method: "POST", credentials: "same-origin", cache: "no-store" });
      return;
    }
    location.replace("/");
  } finally {
    userInput.readOnly = false;
    passInput.readOnly = false;
  }
});
