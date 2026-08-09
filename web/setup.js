(function () {
  const form = document.getElementById("setup-form");
  const err = document.getElementById("error");
  const status = document.getElementById("status");
  const creds = document.getElementById("credentials");
  const submitBtn = document.getElementById("submit-btn");
  const inpx = document.getElementById("inpx");
  const libraryDir = document.getElementById("library_dir");
  const dataDir = document.getElementById("data_dir");
  const auth = document.getElementById("auth");
  const langAll = document.getElementById("lang-all");

  function showError(msg) {
    err.textContent = msg || "Ошибка";
    err.classList.remove("hidden");
  }
  function hideError() {
    err.classList.add("hidden");
    err.textContent = "";
  }
  function setBusy(on, text) {
    submitBtn.disabled = on;
    form.querySelectorAll("input, select, button").forEach((el) => {
      if (el === submitBtn) return;
      el.disabled = on;
    });
    if (text) {
      status.textContent = text;
      status.classList.remove("hidden");
    } else if (!on) {
      status.classList.add("hidden");
    }
  }

  function selectedLanguages() {
    if (langAll.checked) return ["*"];
    return Array.from(form.querySelectorAll('input[name="lang"]:checked'))
      .map((el) => el.value)
      .filter((v) => v !== "*");
  }

  function applyLanguages(list) {
    const set = new Set((list || []).map((x) => String(x).toLowerCase()));
    const all = set.has("*") || set.has("all") || set.size === 0 && list && list.length === 0;
    langAll.checked = all || set.has("*");
    form.querySelectorAll('input[name="lang"]').forEach((el) => {
      if (el.value === "*") return;
      el.checked = langAll.checked ? false : set.has(el.value);
      el.disabled = langAll.checked;
    });
    if (!langAll.checked && selectedLanguages().length === 0) {
      const ru = form.querySelector('input[name="lang"][value="ru"]');
      if (ru) ru.checked = true;
    }
  }

  langAll.addEventListener("change", () => {
    form.querySelectorAll('input[name="lang"]').forEach((el) => {
      if (el.value === "*") return;
      el.disabled = langAll.checked;
      if (langAll.checked) el.checked = false;
    });
    if (!langAll.checked) {
      const ru = form.querySelector('input[name="lang"][value="ru"]');
      if (ru) ru.checked = true;
    }
  });

  async function browse(kind, input) {
    hideError();
    try {
      const res = await fetch("/api/setup/browse", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        showError(data.error || "Не удалось открыть диалог");
        return;
      }
      if (data.canceled || !data.path) return;
      input.value = data.path;
    } catch {
      showError("Нет связи с LibShelf");
    }
  }

  async function pollUntilDone() {
    for (;;) {
      const res = await fetch("/api/setup", { cache: "no-store" });
      const data = await res.json();
      if (data.phase === "importing") {
        status.textContent = data.message || "Импорт…";
        status.classList.remove("hidden");
        await new Promise((r) => setTimeout(r, 1000));
        continue;
      }
      if (data.phase === "error") {
        setBusy(false);
        showError(data.message || "Ошибка импорта");
        return;
      }
      if (data.phase === "ready") {
        status.textContent = `Готово: ${data.books || 0} книг.`;
        status.classList.remove("hidden");
        if (data.adminUser && data.adminPassword) {
          creds.innerHTML =
            `<strong>Админ создан</strong><br>Логин: <code>${data.adminUser}</code><br>` +
            `Пароль: <code>${data.adminPassword}</code><br>` +
            `<span class="muted">Сохраните пароль — он больше не покажется.</span>`;
          creds.classList.remove("hidden");
          submitBtn.textContent = "Открыть библиотеку";
          submitBtn.disabled = false;
          submitBtn.onclick = (e) => {
            e.preventDefault();
            location.href = "/";
          };
          return;
        }
        location.href = "/";
        return;
      }
      await new Promise((r) => setTimeout(r, 800));
    }
  }

  document.getElementById("browse-inpx").addEventListener("click", () => browse("inpx", inpx));
  document.getElementById("browse-library").addEventListener("click", () => browse("library", libraryDir));
  document.getElementById("browse-data").addEventListener("click", () => browse("data", dataDir));

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    hideError();
    creds.classList.add("hidden");
    const languages = selectedLanguages();
    if (!languages.length) {
      showError("Выберите хотя бы один язык или «Все языки»");
      return;
    }
    setBusy(true, "Запуск импорта…");
    try {
      const res = await fetch("/api/setup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          inpx: inpx.value.trim(),
          library_dir: libraryDir.value.trim(),
          data_dir: dataDir.value.trim(),
          auth: auth.value,
          languages,
        }),
      });
      const raw = await res.text();
      let data = {};
      try { data = JSON.parse(raw); } catch { /* plain text error */ }
      if (!res.ok) {
        setBusy(false);
        showError((data && data.error) || raw || "Ошибка");
        return;
      }
      await pollUntilDone();
    } catch {
      setBusy(false);
      showError("Нет связи с LibShelf");
    }
  });

  (async function boot() {
    try {
      const res = await fetch("/api/setup", { cache: "no-store" });
      if (!res.ok) {
        location.href = "/";
        return;
      }
      const data = await res.json();
      if (data.phase === "ready") {
        location.href = "/";
        return;
      }
      if (data.phase === "importing") {
        setBusy(true, data.message || "Импорт…");
        await pollUntilDone();
        return;
      }
      const d = data.defaults || {};
      if (d.inpx) inpx.value = d.inpx;
      if (d.library_dir) libraryDir.value = d.library_dir;
      if (d.data_dir) dataDir.value = d.data_dir;
      if (d.auth) auth.value = d.auth;
      if (d.languages) applyLanguages(d.languages);
      if (data.canBrowse) {
        ["browse-inpx", "browse-library", "browse-data"].forEach((id) => {
          document.getElementById(id).hidden = false;
        });
      }
    } catch {
      showError("Не удалось загрузить настройки");
    }
  })();
})();
