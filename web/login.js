const form = document.getElementById("login-form");
const errorEl = document.getElementById("error");

async function alreadySignedIn() {
  try {
    const res = await fetch("/api/me", { credentials: "same-origin" });
    if (!res.ok) return null;
    const data = await res.json();
    if (data.auth && data.user) return data.user;
  } catch {
    /* ignore */
  }
  return null;
}

(async function boot() {
  // Already have a cookie session → go home. Switch user via «Выйти», then login.
  const user = await alreadySignedIn();
  if (user) {
    location.replace("/");
  }
})();

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  errorEl.classList.add("hidden");
  const username = document.getElementById("username").value.trim();
  const password = document.getElementById("password").value;
  const res = await fetch("/api/login", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    errorEl.textContent = "Неверный логин или пароль";
    errorEl.classList.remove("hidden");
    return;
  }
  const user = await res.json();
  if (user && user.username && user.username.toLowerCase() !== username.toLowerCase()) {
    errorEl.textContent = "Сессия не совпала с логином — попробуйте ещё раз";
    errorEl.classList.remove("hidden");
    return;
  }
  location.href = "/";
});
