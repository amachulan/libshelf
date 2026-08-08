const $ = (id) => document.getElementById(id);

const home = $("home");
const results = $("results");
const bookPanel = $("book");
const grid = $("grid");
const empty = $("empty");
const form = $("search-form");
const qInput = $("q");

let lastQuery = "";
let lastBooks = [];

async function loadStats() {
  try {
    const res = await fetch("/api/stats");
    const data = await res.json();
    $("stats").textContent = `${formatNum(data.books)} книг (ru)`;
  } catch {
    $("stats").textContent = "";
  }
}

function formatNum(n) {
  return new Intl.NumberFormat("ru-RU").format(n || 0);
}

function formatSize(bytes) {
  if (!bytes) return "";
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(0) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

function show(panel) {
  home.classList.toggle("hidden", panel !== "home");
  results.classList.toggle("hidden", panel !== "results");
  bookPanel.classList.toggle("hidden", panel !== "book");
}

function coverSrc(url, id) {
  return `${url}?v=${id}`;
}

function placeholderCover() {
  return "data:image/svg+xml," + encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300">
      <rect width="100%" height="100%" fill="#e7dfd0"/>
      <text x="50%" y="50%" text-anchor="middle" fill="#8a8070" font-family="sans-serif" font-size="18">нет обложки</text>
    </svg>`
  );
}

function renderResults(query, books) {
  lastQuery = query;
  lastBooks = books || [];
  $("results-title").textContent = query ? `Поиск: «${query}»` : "Результаты";
  grid.innerHTML = "";
  empty.classList.toggle("hidden", lastBooks.length > 0);
  for (const b of lastBooks) {
    const el = document.createElement("article");
    el.className = "card";
    el.tabIndex = 0;
    el.innerHTML = `
      <img src="${coverSrc(b.coverUrl, b.id)}" alt="" loading="lazy">
      <div class="meta">
        <p class="title"></p>
        <p class="authors"></p>
      </div>`;
    el.querySelector(".title").textContent = b.title;
    el.querySelector(".authors").textContent = b.authors || "";
    const img = el.querySelector("img");
    img.onerror = () => { img.src = placeholderCover(); };
    el.addEventListener("click", () => openBook(b.id));
    el.addEventListener("keydown", (e) => {
      if (e.key === "Enter") openBook(b.id);
    });
    grid.appendChild(el);
  }
  show("results");
}

async function doSearch(query) {
  query = (query || "").trim();
  if (!query) {
    show("home");
    history.replaceState(null, "", "/");
    return;
  }
  history.replaceState(null, "", "/?q=" + encodeURIComponent(query));
  const res = await fetch("/api/search?q=" + encodeURIComponent(query) + "&limit=60");
  if (!res.ok) {
    alert("Ошибка поиска: " + (await res.text()));
    return;
  }
  const data = await res.json();
  renderResults(query, data.books || []);
}

async function openBook(id) {
  const res = await fetch("/api/book/" + id);
  if (!res.ok) {
    alert("Книга недоступна");
    return;
  }
  const b = await res.json();
  history.pushState({ book: id }, "", "/?book=" + id + (lastQuery ? "&q=" + encodeURIComponent(lastQuery) : ""));
  $("book-title").textContent = b.title;
  $("book-authors").textContent = b.authors || "";
  $("book-series").textContent = b.series
    ? (b.seriesNum ? `${b.series} — ${b.seriesNum}` : b.series)
    : "";
  const bits = [];
  if (b.year) bits.push(String(b.year));
  if (b.ext) bits.push(b.ext.toUpperCase());
  if (b.size) bits.push(formatSize(b.size));
  $("book-info").textContent = bits.join(" · ");
  $("book-genres").textContent = (b.genres || []).join(", ");
  const cover = $("book-cover");
  cover.src = coverSrc(b.coverUrl, b.id);
  cover.onerror = () => { cover.src = placeholderCover(); };
  $("book-download").href = b.downloadUrl;
  show("book");
}

form.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(qInput.value);
});

$("back").addEventListener("click", () => {
  if (lastQuery) {
    history.replaceState(null, "", "/?q=" + encodeURIComponent(lastQuery));
    renderResults(lastQuery, lastBooks);
  } else {
    history.replaceState(null, "", "/");
    show("home");
  }
});

window.addEventListener("popstate", () => {
  bootFromURL();
});

async function bootFromURL() {
  const params = new URLSearchParams(location.search);
  const book = params.get("book");
  const q = params.get("q") || "";
  qInput.value = q;
  if (book) {
    if (q && lastBooks.length === 0) {
      await doSearch(q);
    }
    await openBook(book);
    return;
  }
  if (q) {
    await doSearch(q);
    return;
  }
  show("home");
}

loadStats();
bootFromURL();
