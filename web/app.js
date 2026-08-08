const $ = (id) => document.getElementById(id);

const home = $("home");
const results = $("results");
const bookPanel = $("book");
const catalogPanel = $("catalog");
const listsPanel = $("lists");
const usersPanel = $("users");
const readerEl = $("reader");
const grid = $("grid");
const empty = $("empty");
const form = $("search-form");
const qInput = $("q");
const resultsBack = $("results-back");
const resultsSub = $("results-sub");

let lastQuery = "";
let lastBooks = [];
let listContext = null; // { kind: 'author'|'series'|'genre', id, name }
let currentUser = null;
let currentBookId = null;
let currentShelfStatus = "";
let shelfTab = "reading";
let catalogTab = "authors";
let catalogLetter = "";
let catalogLoadSeq = 0;
let readerBookId = null;
let readerSaveTimer = null;
let restorePosition = 0;
let fontScale = Number(localStorage.getItem("libshelf_font") || "1");
let readMode = localStorage.getItem("libshelf_read_mode") === "pages" ? "pages" : "scroll";

async function api(url, opts = {}) {
  const res = await fetch(url, { ...opts, credentials: "same-origin" });
  if (res.status === 401) {
    location.href = "/login.html";
    throw new Error("unauthorized");
  }
  return res;
}

async function loadSession() {
  sessionStorage.removeItem("libshelf_login_as");
  const res = await fetch("/api/me?_=" + Date.now(), {
    credentials: "same-origin",
    cache: "no-store",
    headers: { "Cache-Control": "no-cache", Pragma: "no-cache" },
  });
  if (res.status === 401) {
    location.href = "/login.html";
    return false;
  }
  if (!res.ok) return true;
  const data = await res.json();
  if (data.auth && data.user) {
    currentUser = data.user;
    $("user-box").classList.remove("hidden");
    $("nav-lists").classList.remove("hidden");
    $("user-label").textContent = data.user.username;
    $("user-label").title = roleLabel(data.user.role);
    $("users-btn").classList.toggle("hidden", data.user.role !== "admin");
  }
  return true;
}

function roleLabel(role) {
  return role === "admin" ? "Администратор" : "Читатель";
}

async function loadStats() {
  try {
    const res = await api("/api/stats");
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
  const reading = panel === "reader";
  document.body.classList.toggle("reading-mode", reading);
  readerEl.classList.toggle("hidden", !reading);
  $("site-header").classList.toggle("hidden", reading);
  $("site-main").classList.toggle("hidden", reading);

  home.classList.toggle("hidden", panel !== "home");
  results.classList.toggle("hidden", panel !== "results");
  bookPanel.classList.toggle("hidden", panel !== "book");
  catalogPanel.classList.toggle("hidden", panel !== "catalog");
  listsPanel.classList.toggle("hidden", panel !== "lists");
  usersPanel.classList.toggle("hidden", panel !== "users");

  $("nav-home").classList.toggle("is-active", panel === "home" || panel === "results" || panel === "book");
  $("nav-catalog").classList.toggle("is-active", panel === "catalog");
  $("nav-lists").classList.toggle("is-active", panel === "lists");
  $("users-btn").classList.toggle("is-active", panel === "users");
}

function coverSrc(url, id) {
  return `${url}?v=${id}`;
}

function placeholderCover() {
  const dark = document.documentElement.getAttribute("data-theme") === "dark";
  const bg = dark ? "#2a3338" : "#e7dfd0";
  const fg = dark ? "#9aa8a3" : "#8a8070";
  return "data:image/svg+xml," + encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300">
      <rect width="100%" height="100%" fill="${bg}"/>
      <text x="50%" y="50%" text-anchor="middle" fill="${fg}" font-family="sans-serif" font-size="18">нет обложки</text>
    </svg>`
  );
}

function shortAuthors(authors, maxNames = 2) {
  const list = (authors || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (list.length === 0) return "";
  if (list.length <= maxNames) return list.join(", ");
  const rest = list.length - maxNames;
  return `${list.slice(0, maxNames).join(", ")} и ещё ${rest}`;
}

function renderBookGrid(books, target = grid, emptyEl = empty) {
  target.innerHTML = "";
  if (emptyEl) emptyEl.classList.toggle("hidden", books.length > 0);
  for (const b of books) {
    const el = document.createElement("article");
    el.className = "card";
    el.tabIndex = 0;
    el.innerHTML = `
      <img src="${coverSrc(b.coverUrl, b.id)}" alt="" loading="lazy">
      <div class="meta">
        <p class="title"></p>
        <p class="authors"></p>
      </div>`;
    const titleEl = el.querySelector(".title");
    const authorsEl = el.querySelector(".authors");
    titleEl.textContent = b.title;
    titleEl.title = b.title || "";
    authorsEl.textContent = shortAuthors(b.authors);
    authorsEl.title = b.authors || "";
    const img = el.querySelector("img");
    img.onerror = () => { img.src = placeholderCover(); };
    el.addEventListener("click", () => openBook(b.id));
    el.addEventListener("keydown", (e) => {
      if (e.key === "Enter") openBook(b.id);
    });
    target.appendChild(el);
  }
}

function renderResults(query, books) {
  listContext = null;
  lastQuery = query;
  lastBooks = books || [];
  resultsBack.classList.add("hidden");
  resultsSub.classList.add("hidden");
  $("results-title").textContent = query ? `Поиск: «${query}»` : "Результаты";
  renderBookGrid(lastBooks);
  show("results");
}

function renderNamedList(kind, data) {
  listContext = { kind, id: data.id, name: data.name };
  lastBooks = data.books || [];
  resultsBack.classList.remove("hidden");
  resultsSub.classList.remove("hidden");
  const label = kind === "author" ? "Автор" : "Серия";
  $("results-title").textContent = `${label}: ${data.name}`;
  resultsSub.textContent = `${formatNum(data.total)} книг`;
  renderBookGrid(lastBooks);
  show("results");
}

async function doSearch(query) {
  query = (query || "").trim();
  if (!query) {
    listContext = null;
    show("home");
    history.replaceState(null, "", "/");
    loadContinue();
    return;
  }
  history.replaceState(null, "", "/?q=" + encodeURIComponent(query));
  const res = await api("/api/search?q=" + encodeURIComponent(query) + "&limit=60");
  if (!res.ok) {
    alert("Ошибка поиска: " + (await res.text()));
    return;
  }
  const data = await res.json();
  renderResults(query, data.books || []);
}

async function openAuthor(id) {
  const res = await api("/api/author/" + id + "?limit=100");
  if (!res.ok) {
    alert("Автор недоступен");
    return;
  }
  const data = await res.json();
  history.pushState({ author: id }, "", "/?author=" + id);
  renderNamedList("author", data);
}

async function openSeries(id) {
  const res = await api("/api/series/" + id + "?limit=100");
  if (!res.ok) {
    alert("Серия недоступна");
    return;
  }
  const data = await res.json();
  history.pushState({ series: id }, "", "/?series=" + id);
  renderNamedList("series", data);
}

function linkButton(text, onClick) {
  const a = document.createElement("button");
  a.type = "button";
  a.className = "inline-link";
  a.textContent = text;
  a.addEventListener("click", onClick);
  return a;
}

function syncShelfPills(status) {
  currentShelfStatus = status || "";
  document.querySelectorAll("#shelf-controls .shelf-pill").forEach((btn) => {
    const st = btn.getAttribute("data-status");
    btn.classList.toggle("is-active", st === currentShelfStatus && st !== "");
  });
}

async function setShelfStatus(status) {
  if (!currentUser || !currentBookId) return;
  const body = status === "" ? { status: null } : { status };
  const res = await api("/api/shelf/" + currentBookId, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    alert(await res.text());
    return;
  }
  const data = await res.json();
  syncShelfPills(data.status || "");
  updateReadButton(data.progress || 0);
}

function updateReadButton(progress) {
  const btn = $("book-read");
  if (progress > 0.01 && progress < 0.99) {
    btn.textContent = `Продолжить · ${Math.round(progress * 100)}%`;
  } else {
    btn.textContent = "Читать";
  }
}

async function openBook(id) {
  const res = await api("/api/book/" + id);
  if (!res.ok) {
    alert("Книга недоступна");
    return;
  }
  const b = await res.json();
  currentBookId = b.id;
  const qs = new URLSearchParams();
  qs.set("book", id);
  if (lastQuery) qs.set("q", lastQuery);
  if (listContext?.kind === "author") qs.set("author", listContext.id);
  if (listContext?.kind === "series") qs.set("series", listContext.id);
  history.pushState({ book: id }, "", "/?" + qs.toString());

  $("book-title").textContent = b.title;

  const authorsEl = $("book-authors");
  authorsEl.innerHTML = "";
  const authors = b.authorList || [];
  if (authors.length === 0 && b.authors) {
    authorsEl.textContent = b.authors;
  } else {
    authors.forEach((a, i) => {
      if (i) authorsEl.appendChild(document.createTextNode(", "));
      authorsEl.appendChild(linkButton(a.name, () => openAuthor(a.id)));
    });
  }

  const seriesEl = $("book-series");
  seriesEl.innerHTML = "";
  if (b.series && b.seriesId) {
    seriesEl.appendChild(document.createTextNode("Серия: "));
    const label = b.seriesNum ? `${b.series} — ${b.seriesNum}` : b.series;
    seriesEl.appendChild(linkButton(label, () => openSeries(b.seriesId)));
  } else if (b.series) {
    seriesEl.textContent = b.seriesNum ? `${b.series} — ${b.seriesNum}` : b.series;
  }

  const translatorsEl = $("book-translators");
  if (b.translators && b.translators.length) {
    translatorsEl.textContent = "Перевод: " + b.translators.join(", ");
    translatorsEl.classList.remove("hidden");
  } else {
    translatorsEl.textContent = "";
    translatorsEl.classList.add("hidden");
  }

  const editionEl = $("book-edition");
  const editionBits = [];
  if (b.publisher) editionBits.push(b.publisher);
  if (b.city) editionBits.push(b.city);
  if (b.pubYear) editionBits.push(b.pubYear);
  else if (b.year && (b.publisher || b.city)) editionBits.push(String(b.year));
  if (b.isbn) editionBits.push("ISBN " + b.isbn);
  if (editionBits.length) {
    editionEl.textContent = editionBits.join(", ");
    editionEl.classList.remove("hidden");
  } else {
    editionEl.textContent = "";
    editionEl.classList.add("hidden");
  }

  const bits = [];
  if (b.year && !b.pubYear && !b.publisher && !b.city) bits.push(String(b.year));
  if (b.ext) bits.push(b.ext.toUpperCase());
  if (b.size) bits.push(formatSize(b.size));
  $("book-info").textContent = bits.join(" · ");

  const genres = (b.genreList || []).map((g) => g.name || g.code).filter(Boolean);
  $("book-genres").textContent = genres.length ? genres.join(" · ") : "";

  const ann = $("book-annotation");
  if (b.annotation) {
    ann.innerHTML = b.annotation;
    ann.classList.remove("hidden");
  } else {
    ann.innerHTML = "";
    ann.classList.add("hidden");
  }

  const cover = $("book-cover");
  cover.src = coverSrc(b.coverUrl, b.id);
  cover.onerror = () => { cover.src = placeholderCover(); };
  $("book-download").href = b.downloadUrl;
  updateReadButton(b.progress || 0);

  const shelf = $("shelf-controls");
  if (currentUser) {
    shelf.classList.remove("hidden");
    syncShelfPills(b.shelfStatus || "");
  } else {
    shelf.classList.add("hidden");
  }

  show("book");
}

function applyFontScale(opts = {}) {
  const keepPosition = opts.keepPosition !== false;
  const pos = keepPosition && readerBookId ? readerPosition() : null;
  $("reader-content").style.fontSize = `${fontScale}rem`;
  localStorage.setItem("libshelf_font", String(fontScale));
  if (pos != null) {
    requestAnimationFrame(() => restoreReaderPosition(pos));
  }
}

function readerContentEl() {
  return $("reader-content");
}

function syncPageMetrics() {
  const el = readerContentEl();
  if (!el) return;
  if (readMode !== "pages") {
    el.style.columnWidth = "";
    return;
  }
  const style = getComputedStyle(el);
  const pad = (parseFloat(style.paddingLeft) || 0) + (parseFloat(style.paddingRight) || 0);
  // One page = content box width; viewport clips any subpixel leftovers.
  const col = Math.max(120, Math.floor(el.clientWidth - pad));
  el.style.columnWidth = col + "px";
}

function pageStride() {
  const el = readerContentEl();
  if (!el) return 1;
  return Math.max(1, parseFloat(el.style.columnWidth) || el.clientWidth);
}

function applyReadMode() {
  document.body.classList.toggle("reader-pages", readMode === "pages");
  const btn = $("reader-mode-btn");
  if (btn) {
    btn.textContent = readMode === "pages" ? "Лента" : "Страницы";
    btn.title = readMode === "pages" ? "Сплошной текст" : "Листать страницами";
  }
  localStorage.setItem("libshelf_read_mode", readMode);
  syncPageMetrics();
}

function readerPosition() {
  if (readMode === "pages") {
    const el = readerContentEl();
    if (!el) return 0;
    const max = el.scrollWidth - el.clientWidth;
    if (max <= 0) return 0;
    return Math.min(1, Math.max(0, el.scrollLeft / max));
  }
  const el = document.documentElement;
  const max = el.scrollHeight - el.clientHeight;
  if (max <= 0) return 0;
  return Math.min(1, Math.max(0, el.scrollTop / max));
}

function restoreReaderPosition(pos) {
  const p = Math.min(1, Math.max(0, Number(pos) || 0));
  if (readMode === "pages") {
    const el = readerContentEl();
    if (!el) return;
    syncPageMetrics();
    const max = el.scrollWidth - el.clientWidth;
    if (max <= 0) {
      el.scrollLeft = 0;
      return;
    }
    const stride = pageStride();
    el.scrollLeft = Math.round((p * max) / stride) * stride;
    return;
  }
  const el = document.documentElement;
  const max = el.scrollHeight - el.clientHeight;
  el.scrollTop = max > 0 ? p * max : 0;
}

function scrollToReaderTarget(target) {
  if (!target) return;
  if (readMode === "pages") {
    const el = readerContentEl();
    syncPageMetrics();
    const cRect = el.getBoundingClientRect();
    const tRect = target.getBoundingClientRect();
    const delta = tRect.left - cRect.left + el.scrollLeft;
    const stride = pageStride();
    el.scrollTo({ left: Math.floor(delta / stride) * stride, behavior: "smooth" });
    return;
  }
  target.scrollIntoView({ behavior: "smooth", block: "start" });
}

function flipReaderPage(dir) {
  if (readMode !== "pages") return;
  const el = readerContentEl();
  syncPageMetrics();
  el.scrollBy({ left: dir * pageStride(), behavior: "smooth" });
  scheduleSaveProgress();
}

function setReadMode(mode) {
  const next = mode === "pages" ? "pages" : "scroll";
  const pos = readerBookId ? readerPosition() : restorePosition;
  readMode = next;
  applyReadMode();
  if (readerBookId) {
    requestAnimationFrame(() => {
      restoreReaderPosition(pos);
      scheduleSaveProgress();
    });
  }
}

function saveReaderProgress() {
  if (!currentUser || !readerBookId) return;
  const position = readerPosition();
  api("/api/book/" + readerBookId + "/progress", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ position }),
  }).catch(() => {});
}

function scheduleSaveProgress() {
  if (readerSaveTimer) clearTimeout(readerSaveTimer);
  readerSaveTimer = setTimeout(saveReaderProgress, 800);
}

async function openReader(id) {
  const res = await api("/api/book/" + id + "/read");
  if (!res.ok) {
    alert("Не удалось открыть книгу");
    return;
  }
  const data = await res.json();
  readerBookId = id;
  currentBookId = id;
  restorePosition = data.position || 0;
  history.pushState({ read: id }, "", "/?read=" + id);
  $("reader-title").textContent = data.title || "";
  $("reader-content").innerHTML = data.html || "";
  applyFontScale({ keepPosition: false });
  applyReadMode();

  const toc = $("reader-toc");
  toc.innerHTML = "";
  toc.classList.add("hidden");
  const chapters = data.chapters || [];
  if (chapters.length) {
    const ul = document.createElement("ul");
    for (const ch of chapters) {
      const li = document.createElement("li");
      const a = document.createElement("button");
      a.type = "button";
      a.className = "toc-link";
      a.textContent = ch.title;
      a.addEventListener("click", () => {
        scrollToReaderTarget(document.getElementById(ch.id));
        toc.classList.add("hidden");
        scheduleSaveProgress();
      });
      li.appendChild(a);
      ul.appendChild(li);
    }
    toc.appendChild(ul);
  }
  $("reader-toc-btn").classList.toggle("hidden", chapters.length === 0);

  show("reader");
  requestAnimationFrame(() => {
    requestAnimationFrame(() => restoreReaderPosition(restorePosition));
  });
}

function closeReader() {
  saveReaderProgress();
  readerBookId = null;
  $("reader-toc").classList.add("hidden");
  document.body.classList.remove("reader-pages");
  if (currentBookId) {
    openBook(currentBookId);
  } else {
    history.replaceState(null, "", "/");
    show("home");
    loadContinue();
  }
}

async function loadContinue() {
  const block = $("continue-block");
  if (!currentUser) {
    block.classList.add("hidden");
    return;
  }
  try {
    const res = await api("/api/shelf/continue?limit=6");
    if (!res.ok) {
      block.classList.add("hidden");
      return;
    }
    const data = await res.json();
    const books = (data.items || []).map((it) => it.book).filter(Boolean);
    if (!books.length) {
      block.classList.add("hidden");
      return;
    }
    block.classList.remove("hidden");
    renderBookGrid(books, $("continue-grid"), null);
  } catch {
    block.classList.add("hidden");
  }
}

function setCatalogLettersIdle(idle) {
  const strip = $("catalog-letters");
  strip.classList.toggle("is-idle", idle);
  strip.setAttribute("aria-hidden", idle ? "true" : "false");
}

function renderCatalogLetters(letters) {
  const strip = $("catalog-letters");
  strip.innerHTML = "";
  if (!letters.length) {
    setCatalogLettersIdle(true);
    return;
  }
  setCatalogLettersIdle(false);
  for (const l of letters) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "letter-btn" + (l.letter === catalogLetter ? " is-active" : "");
    btn.textContent = l.letter;
    btn.title = formatNum(l.count);
    btn.addEventListener("click", () => openCatalog(catalogTab, l.letter));
    strip.appendChild(btn);
  }
}

function renderCatalogRows(items, kind) {
  const list = $("catalog-list");
  const emptyEl = $("catalog-empty");
  list.innerHTML = "";
  emptyEl.classList.toggle("hidden", items.length > 0);
  for (const it of items) {
    const row = document.createElement("button");
    row.type = "button";
    row.className = "catalog-row";
    const name = it.name || it.title;
    row.innerHTML = `<span class="catalog-name"></span><span class="catalog-count"></span>`;
    row.querySelector(".catalog-name").textContent = name;
    row.querySelector(".catalog-count").textContent = formatNum(it.books);
    row.addEventListener("click", () => {
      if (kind === "authors") openAuthor(it.id);
      else if (kind === "series") openSeries(it.id);
      else if (kind === "genres") openGenre(it.code);
    });
    list.appendChild(row);
  }
}

async function openGenre(code) {
  const res = await api("/api/catalog/genres/" + encodeURIComponent(code) + "?limit=100");
  if (!res.ok) {
    alert("Жанр недоступен");
    return;
  }
  const data = await res.json();
  history.pushState({ genre: code }, "", "/?genre=" + encodeURIComponent(code));
  listContext = { kind: "genre", id: code, name: data.name };
  lastBooks = data.books || [];
  resultsBack.classList.remove("hidden");
  resultsSub.classList.remove("hidden");
  $("results-title").textContent = `Жанр: ${data.name}`;
  resultsSub.textContent = `${formatNum(data.total)} книг`;
  renderBookGrid(lastBooks);
  show("results");
}

function setCatalogLoading(on) {
  const loading = $("catalog-loading");
  loading.classList.toggle("hidden", !on);
  loading.setAttribute("aria-busy", on ? "true" : "false");
  if (on) $("catalog-empty").classList.add("hidden");
}

async function openCatalog(tab, letter) {
  catalogTab = tab || catalogTab || "authors";
  catalogLetter = letter || "";
  const seq = ++catalogLoadSeq;
  document.querySelectorAll("#catalog-tabs .shelf-pill").forEach((btn) => {
    btn.classList.toggle("is-active", btn.getAttribute("data-cat") === catalogTab);
  });

  const emptyEl = $("catalog-empty");
  emptyEl.classList.add("hidden");
  if (catalogTab === "genres") setCatalogLettersIdle(true);
  setCatalogLoading(true);
  show("catalog");

  try {
    if (catalogTab === "genres") {
      history.pushState({ catalog: "genres" }, "", "/?catalog=genres");
      const res = await api("/api/catalog/genres");
      if (seq !== catalogLoadSeq) return;
      if (!res.ok) {
        alert("Не удалось загрузить жанры");
        return;
      }
      const data = await res.json();
      if (seq !== catalogLoadSeq) return;
      $("catalog-list").innerHTML = "";
      const genresList = data.genres || [];
      emptyEl.classList.toggle("hidden", genresList.length > 0);
      renderCatalogRows(genresList.map((g) => ({
        code: g.code,
        name: g.name || g.code,
        books: g.books,
      })), "genres");
      return;
    }

    let url = "/api/catalog/" + catalogTab + "?limit=150";
    if (catalogLetter) url += "&letter=" + encodeURIComponent(catalogLetter);
    const res = await api(url);
    if (seq !== catalogLoadSeq) return;
    if (!res.ok) {
      alert("Не удалось загрузить каталог");
      return;
    }
    const data = await res.json();
    if (seq !== catalogLoadSeq) return;
    const letters = data.letters || [];
    catalogLetter = data.letter || catalogLetter || (letters[0] && letters[0].letter) || "";
    const qs = new URLSearchParams();
    qs.set("catalog", catalogTab);
    if (catalogLetter) qs.set("letter", catalogLetter);
    history.pushState({ catalog: catalogTab, letter: catalogLetter }, "", "/?" + qs.toString());

    renderCatalogLetters(letters);
    $("catalog-list").innerHTML = "";
    if (catalogTab === "authors") {
      renderCatalogRows(data.authors || [], "authors");
    } else {
      renderCatalogRows((data.series || []).map((s) => ({
        id: s.id,
        name: s.title,
        books: s.books,
      })), "series");
    }
  } finally {
    if (seq === catalogLoadSeq) setCatalogLoading(false);
  }
}

async function openLists(status) {
  if (!currentUser) return;
  shelfTab = status || shelfTab || "reading";
  history.pushState({ lists: shelfTab }, "", "/?lists=" + shelfTab);
  document.querySelectorAll("#shelf-tabs .shelf-pill").forEach((btn) => {
    btn.classList.toggle("is-active", btn.getAttribute("data-status") === shelfTab);
  });
  const res = await api("/api/shelf?status=" + encodeURIComponent(shelfTab) + "&limit=100");
  if (!res.ok) {
    alert("Не удалось загрузить списки");
    return;
  }
  const data = await res.json();
  const books = (data.items || []).map((it) => it.book).filter(Boolean);
  renderBookGrid(books, $("lists-grid"), $("lists-empty"));
  show("lists");
}

function goBackFromBook() {
  if (listContext) {
    const { kind, id } = listContext;
    if (kind === "author") {
      history.replaceState({ author: id }, "", "/?author=" + id);
      openAuthor(id);
      return;
    }
    if (kind === "series") {
      history.replaceState({ series: id }, "", "/?series=" + id);
      openSeries(id);
      return;
    }
    if (kind === "genre") {
      history.replaceState({ genre: id }, "", "/?genre=" + encodeURIComponent(id));
      openGenre(id);
      return;
    }
  }
  if (lastQuery) {
    history.replaceState(null, "", "/?q=" + encodeURIComponent(lastQuery));
    renderResults(lastQuery, lastBooks);
  } else {
    history.replaceState(null, "", "/");
    show("home");
    loadContinue();
  }
}

form.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(qInput.value);
});

$("back").addEventListener("click", goBackFromBook);
$("book-read").addEventListener("click", () => {
  if (currentBookId) openReader(currentBookId);
});

document.querySelectorAll("#shelf-controls .shelf-pill").forEach((btn) => {
  btn.addEventListener("click", () => setShelfStatus(btn.getAttribute("data-status") || ""));
});

document.querySelectorAll("#shelf-tabs .shelf-pill").forEach((btn) => {
  btn.addEventListener("click", () => openLists(btn.getAttribute("data-status")));
});

resultsBack.addEventListener("click", () => {
  if (lastQuery) {
    history.replaceState(null, "", "/?q=" + encodeURIComponent(lastQuery));
    doSearch(lastQuery);
    return;
  }
  if (listContext?.kind === "genre") {
    openCatalog("genres", "");
    return;
  }
  if (listContext?.kind === "author") {
    openCatalog("authors", catalogLetter || "");
    return;
  }
  if (listContext?.kind === "series") {
    openCatalog("series", catalogLetter || "");
    return;
  }
  history.replaceState(null, "", "/");
  listContext = null;
  show("home");
  loadContinue();
});

window.addEventListener("popstate", () => {
  bootFromURL();
});

window.addEventListener("scroll", () => {
  if (!document.body.classList.contains("reading-mode")) return;
  if (readMode === "pages") return;
  scheduleSaveProgress();
}, { passive: true });

$("reader-content").addEventListener("scroll", () => {
  if (!document.body.classList.contains("reading-mode")) return;
  if (readMode !== "pages") return;
  scheduleSaveProgress();
}, { passive: true });

$("reader-back").addEventListener("click", closeReader);
$("reader-toc-btn").addEventListener("click", () => {
  $("reader-toc").classList.toggle("hidden");
});
$("reader-mode-btn").addEventListener("click", () => {
  setReadMode(readMode === "pages" ? "scroll" : "pages");
});
$("reader-page-prev").addEventListener("click", () => flipReaderPage(-1));
$("reader-page-next").addEventListener("click", () => flipReaderPage(1));
$("reader-font-up").addEventListener("click", () => {
  fontScale = Math.min(1.6, Math.round((fontScale + 0.1) * 10) / 10);
  applyFontScale();
});
$("reader-font-down").addEventListener("click", () => {
  fontScale = Math.max(0.85, Math.round((fontScale - 0.1) * 10) / 10);
  applyFontScale();
});

window.addEventListener("keydown", (e) => {
  if (!document.body.classList.contains("reading-mode") || readMode !== "pages") return;
  if (e.key === "ArrowRight" || e.key === "PageDown" || e.key === " ") {
    e.preventDefault();
    flipReaderPage(1);
  } else if (e.key === "ArrowLeft" || e.key === "PageUp") {
    e.preventDefault();
    flipReaderPage(-1);
  }
});

window.addEventListener("resize", () => {
  if (!document.body.classList.contains("reading-mode") || readMode !== "pages" || !readerBookId) return;
  const pos = readerPosition();
  requestAnimationFrame(() => {
    syncPageMetrics();
    restoreReaderPosition(pos);
  });
});

async function bootFromURL() {
  const params = new URLSearchParams(location.search);
  const read = params.get("read");
  const book = params.get("book");
  const author = params.get("author");
  const series = params.get("series");
  const genre = params.get("genre");
  const catalog = params.get("catalog");
  const letter = params.get("letter") || "";
  const lists = params.get("lists");
  const q = params.get("q") || "";
  qInput.value = q;

  if (read) {
    await openReader(read);
    return;
  }
  if (lists && currentUser) {
    await openLists(lists);
    return;
  }
  if (catalog) {
    await openCatalog(catalog, letter);
    return;
  }
  if (genre && !book) {
    await openGenre(genre);
    return;
  }
  if (author && !book) {
    await openAuthor(author);
    return;
  }
  if (series && !book) {
    await openSeries(series);
    return;
  }
  if (book) {
    if (author) listContext = { kind: "author", id: Number(author) };
    if (series) listContext = { kind: "series", id: Number(series) };
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
  await loadContinue();
}

async function loadUsers() {
  // Bust caches: some proxies/browsers reuse an older GET /api/users.
  const res = await api("/api/users?_=" + Date.now(), {
    cache: "no-store",
    headers: { "Cache-Control": "no-cache", Pragma: "no-cache" },
  });
  if (!res.ok) {
    alert((await res.text()) || "Нет доступа");
    return;
  }
  const data = await res.json();
  const body = $("users-body");
  body.innerHTML = "";
  const users = data.users || [];
  for (const u of users) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td></td><td></td><td class="actions"></td>`;
    tr.children[0].textContent = u.username;
    tr.children[1].textContent = roleLabel(u.role);
    if (!currentUser || u.id !== currentUser.id) {
      const del = document.createElement("button");
      del.type = "button";
      del.className = "text-btn";
      del.textContent = "Удалить";
      del.addEventListener("click", async () => {
        if (!confirm(`Удалить ${u.username}?`)) return;
        const r = await api("/api/users/" + u.id, { method: "DELETE" });
        if (!r.ok) {
          alert(await r.text());
          return;
        }
        loadUsers();
      });
      tr.children[2].appendChild(del);
    }
    body.appendChild(tr);
  }
  show("users");
  return users;
}

$("logout-btn").addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
  location.href = "/login.html";
});

$("nav-home").addEventListener("click", (e) => {
  e.preventDefault();
  history.pushState(null, "", "/");
  listContext = null;
  lastQuery = "";
  qInput.value = "";
  show("home");
  loadContinue();
});

$("nav-catalog").addEventListener("click", () => openCatalog(catalogTab, catalogLetter));

document.querySelectorAll("#catalog-tabs .shelf-pill").forEach((btn) => {
  btn.addEventListener("click", () => openCatalog(btn.getAttribute("data-cat"), ""));
});

$("nav-lists").addEventListener("click", () => openLists(shelfTab));

$("users-btn").addEventListener("click", () => {
  history.pushState({ users: true }, "", "/?users=1");
  loadUsers();
});

$("users-back").addEventListener("click", () => {
  history.replaceState(null, "", "/");
  show("home");
  loadContinue();
});

$("user-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const err = $("users-error");
  const ok = $("users-ok");
  const btn = $("user-add-btn");
  err.classList.add("hidden");
  ok.classList.add("hidden");
  const username = $("new-username").value.trim();
  const password = $("new-password").value;
  const role = $("new-role").value;
  if (!username || !password) {
    err.textContent = "Укажите логин и пароль";
    err.classList.remove("hidden");
    return;
  }
  btn.disabled = true;
  try {
    // Confirm the cookie session is still admin before creating.
    const meRes = await api("/api/me");
    if (!meRes.ok) {
      err.textContent = (await meRes.text()) || "Не удалось проверить сессию";
      err.classList.remove("hidden");
      return;
    }
    const me = await meRes.json();
    if (!me.auth || !me.user || String(me.user.role).toLowerCase() !== "admin") {
      err.textContent = me.user
        ? `Нет прав админа (сейчас ${me.user.username}, роль ${me.user.role}). Выйдите и войдите как admin.`
        : "Сессия потеряна — войдите как admin снова";
      err.classList.remove("hidden");
      return;
    }
    const res = await api("/api/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password, role }),
    });
    if (!res.ok) {
      const detail = (await res.text()).trim() || res.statusText;
      err.textContent = `${detail} (HTTP ${res.status}, как ${me.user.username})`;
      err.classList.remove("hidden");
      // 409: user exists in DB — refresh list so it isn't hidden behind a stale cache.
      if (res.status === 409) await loadUsers();
      return;
    }
    const created = await res.json();
    $("new-username").value = "";
    $("new-password").value = "";
    $("new-role").value = "reader";
    ok.textContent = `Создан: ${created.username} (${roleLabel(created.role)})`;
    ok.classList.remove("hidden");
    await loadUsers();
  } finally {
    btn.disabled = false;
  }
});

(async function boot() {
  if (!(await loadSession())) return;
  await loadStats();
  applyFontScale();
  const params = new URLSearchParams(location.search);
  if (params.get("users") === "1" && currentUser?.role === "admin") {
    await loadUsers();
    return;
  }
  await bootFromURL();
})();
