(function () {
  const themeKey = "libshelf_theme";

  function currentTheme() {
    return document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
  }

  function applyTheme(theme) {
    const next = theme === "dark" ? "dark" : "light";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem(themeKey, next);
    let meta = document.querySelector('meta[name="theme-color"]');
    if (!meta) {
      meta = document.createElement("meta");
      meta.name = "theme-color";
      document.head.appendChild(meta);
    }
    meta.content = next === "dark" ? "#12161a" : "#f3efe6";
    document.querySelectorAll("[data-theme-toggle]").forEach((btn) => {
      btn.textContent = next === "dark" ? "☀" : "☾";
      btn.title = next === "dark" ? "Светлая тема" : "Тёмная тема";
      btn.setAttribute("aria-label", btn.title);
    });
  }

  function initTheme() {
    const saved = localStorage.getItem(themeKey);
    if (saved === "light" || saved === "dark") {
      applyTheme(saved);
    } else {
      applyTheme(
        window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
      );
    }
    document.querySelectorAll("[data-theme-toggle]").forEach((btn) => {
      btn.addEventListener("click", () => {
        applyTheme(currentTheme() === "light" ? "dark" : "light");
      });
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initTheme);
  } else {
    initTheme();
  }
})();
