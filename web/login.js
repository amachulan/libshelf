const form = document.getElementById("login-form");
const errorEl = document.getElementById("error");
const userInput = document.getElementById("username");
const passInput = document.getElementById("password");

if (new URLSearchParams(location.search).get("switch") === "1") {
  errorEl.textContent = "Предыдущая сессия сброшена — войдите снова";
  errorEl.classList.remove("hidden");
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
    await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
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
      await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
      return;
    }
    sessionStorage.setItem("libshelf_login_as", user.username);
    location.replace("/");
  } finally {
    userInput.readOnly = false;
    passInput.readOnly = false;
  }
});
