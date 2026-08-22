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
const advForm = $("adv-search-form");
const advPanel = $("adv-search");
const advToggle = $("search-adv-toggle");
const qInput = $("q");
const advTitle = $("adv-title");
const advAuthor = $("adv-author");
const advYearFrom = $("adv-year-from");
const advYearTo = $("adv-year-to");
const advAddedFrom = $("adv-added-from");
const advAddedTo = $("adv-added-to");
const resultsBack = $("results-back");
const resultsSub = $("results-sub");

const PAGE_SIZE = 60;
const AUTHOR_CHIP_PREVIEW = 24;
let genreSort = "popular";

function normalizeGenreSort(sort) {
  const s = String(sort || "").toLowerCase();
  if (s === "new" || s === "added" || s === "date") return "new";
  if (s === "title" || s === "alpha" || s === "name") return "title";
  return "popular";
}

/** @typedef {{ q: string, title: string, author: string, yearFrom: string, yearTo: string, addedFrom: string, addedTo: string }} SearchParams */

/** @type {SearchParams} */
let lastSearch = emptySearch();
let lastBooks = [];
let listContext = null; // { kind: 'author'|'series'|'genre'|'lists', id, name, series?, status? }
/** Where ← on a named list should go (book page / catalog / search / author). */
/** @type {{ type: 'book', id: number } | { type: 'author', id: number } | { type: 'catalog', tab: string, letter: string } | { type: 'search' } | null} */
let listReturn = null;
/** @type {{ mode: 'search'|'author'|'series'|'genre', key: string|number|SearchParams, page: number, total: number, limit: number } | null} */
let resultsPager = null;
let currentUser = null;
let currentBookId = null;
let currentShelfStatus = "";
let shelfTab = "reading";
let catalogTab = "authors";
let catalogLetter = "";
let catalogLoadSeq = 0;
let catalogQuery = "";
let catalogFilterTimer = null;
/** @type {Array<{code: string, name: string, books: number}> | null} */
let catalogGenresCache = null;
const catalogFilter = $("catalog-filter");
let readerBookId = null;
let readerSaveTimer = null;
let restorePosition = 0;
let fontScale = Number(localStorage.getItem("libshelf_font") || "1");
/** @type {"pages-h"|"pages-v"|"scroll"} */
let readMode = (() => {
  const raw = localStorage.getItem("libshelf_read_mode");
  if (raw === "pages-h" || raw === "pages-v" || raw === "scroll") return raw;
  // Legacy: "pages" was vertical page flips.
  if (raw === "pages") return "pages-v";
  return "scroll";
})();
let textAlign = localStorage.getItem("libshelf_align") === "left" ? "left" : "justify";
const READER_FONTS = [
  {
    id: "sans",
    label: "Без засечек",
    family: '"Segoe UI", "Helvetica Neue", Arial, sans-serif',
  },
  {
    id: "serif",
    label: "С засечками",
    family: 'Georgia, "Iowan Old Style", "Palatino Linotype", Palatino, "Times New Roman", serif',
  },
];
let readerFont =
  READER_FONTS.some((f) => f.id === localStorage.getItem("libshelf_reader_font"))
    ? localStorage.getItem("libshelf_reader_font")
    : "sans";
let readerPageIndex = 0;
/** Page start offsets (px): X for pages-h, Y for pages-v. */
let readerPageOffsets = [0];
let readerPageStride = 0;
/** Last trusted 0..1 place in the book; survives collapsed layout / iOS scroll jumps. */
let lastGoodReaderPos = 0;
let lastKnownContentH = 0;
let pinnedReaderPos = null;
let pinReaderTimer = 0;
let restorePlaceTries = 0;
/** @type {Array<{id: string, title: string}>} */
let readerChapters = [];
let tocTick = 0;

function pageModeActive() {
  return (
    document.body.classList.contains("reading-mode") &&
    (readMode === "pages-h" || readMode === "pages-v")
  );
}

function pageFlipHorizontal() {
  return readMode === "pages-h";
}

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

function formatLastSeen(iso) {
  if (!iso) return "никогда";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "никогда";
  const diff = Date.now() - t;
  if (diff < 60_000) return "только что";
  if (diff < 60 * 60_000) {
    const m = Math.floor(diff / 60_000);
    return m + " мин назад";
  }
  if (diff < 24 * 60 * 60_000) {
    const h = Math.floor(diff / (60 * 60_000));
    return h + " ч назад";
  }
  if (diff < 7 * 24 * 60 * 60_000) {
    const d = Math.floor(diff / (24 * 60 * 60_000));
    if (d === 1) return "вчера";
    return d + " дн назад";
  }
  try {
    return new Date(t).toLocaleString("ru-RU", {
      day: "numeric",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
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

function formatRate(n) {
  const v = Number(n);
  if (!v || v <= 0) return "";
  return Number.isInteger(v) ? String(v) : v.toFixed(1).replace(".", ",");
}

function formatFantLab(b) {
  if (!b || !b.fantlabVoters || !b.fantlabRate) return "";
  return formatRate(b.fantlabRate);
}

function fantlabTitle(b) {
  if (!b || !b.fantlabVoters) return "";
  return "FantLab, " + formatNum(b.fantlabVoters) + " оценок";
}

function formatProgress(p) {
  const v = Number(p);
  if (!v || v <= 0.01) return "";
  return Math.min(100, Math.round(v * 100)) + "%";
}

function formatSize(bytes) {
  if (!bytes) return "";
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(0) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

function show(panel) {
  const reading = panel === "reader";
  document.documentElement.classList.remove("boot-reader");
  document.body.classList.toggle("reading-mode", reading);
  readerEl.classList.toggle("hidden", !reading);
  $("site-header").classList.toggle("hidden", reading);
  $("site-main").classList.toggle("hidden", reading);
  if (!reading) {
    document.body.classList.remove("reader-pages", "reader-chrome-hidden");
    lockPageScroll(false);
  }

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

function coverSeed(title, authors) {
  const s = String(title || "") + "\0" + String(authors || "");
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 33 + s.charCodeAt(i)) >>> 0;
  return h;
}

function escapeXml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function wrapCoverTitle(title, maxChars, maxLines) {
  const words = String(title || "").replace(/\s+/g, " ").trim().split(" ").filter(Boolean);
  if (!words.length) return ["Книга"];
  const lines = [];
  let i = 0;
  while (i < words.length && lines.length < maxLines) {
    const last = lines.length === maxLines - 1;
    if (last) {
      let rest = words.slice(i).join(" ");
      if (rest.length > maxChars) rest = rest.slice(0, Math.max(1, maxChars - 1)) + "…";
      lines.push(rest);
      break;
    }
    let line = words[i++];
    while (i < words.length && (line + " " + words[i]).length <= maxChars) {
      line += " " + words[i++];
    }
    if (line.length > maxChars) line = line.slice(0, Math.max(1, maxChars - 1)) + "…";
    lines.push(line);
  }
  return lines;
}

function coverTitleLayout(title) {
  const text = String(title || "").replace(/\s+/g, " ").trim();
  const tries = [
    { size: 22, chars: 11, lines: 5 },
    { size: 18, chars: 14, lines: 6 },
    { size: 15, chars: 17, lines: 7 },
    { size: 13, chars: 20, lines: 8 },
  ];
  for (const t of tries) {
    const lines = wrapCoverTitle(text, t.chars, t.lines);
    const shown = lines.join(" ").replace(/…$/, "");
    const fits = shown.length >= text.length || t === tries[tries.length - 1];
    if (fits) return { lines, size: t.size };
  }
  return { lines: wrapCoverTitle(text, 20, 8), size: 13 };
}

function placeholderCover(title, authors) {
  const dark = document.documentElement.getAttribute("data-theme") === "dark";
  const palettes = dark
    ? ["#3d4f4a", "#3a4558", "#4a3f4a", "#4a4536", "#3d4a58", "#4a3d3d"]
    : ["#c9d4c8", "#c5cdd8", "#d4c8d0", "#d6d0c0", "#c8d0d8", "#d4c8c4"];
  const bands = dark
    ? ["#2a3532", "#283040", "#352c35", "#353024", "#2a3440", "#352a2a"]
    : ["#a8b6a6", "#a4aebc", "#b6a8b0", "#b8b09c", "#a8b0bc", "#b6a8a4"];
  const ink = dark ? "#e8efe9" : "#2a2e2c";
  const i = coverSeed(title, authors) % palettes.length;
  const { lines, size } = coverTitleLayout(title);
  const lineH = size * 1.28;
  const startY = 150 - (lines.length * lineH) / 2 + size * 0.82;
  const texts = lines.map((line, idx) => {
    const y = (startY + idx * lineH).toFixed(1);
    return `<text x="107" y="${y}" text-anchor="middle" fill="${ink}" fill-opacity="0.92" font-family="Georgia, 'Times New Roman', serif" font-size="${size}" font-weight="600">${escapeXml(line)}</text>`;
  }).join("");
  return "data:image/svg+xml," + encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300" viewBox="0 0 200 300">
      <rect width="200" height="300" fill="${palettes[i]}"/>
      <rect x="0" y="0" width="14" height="300" fill="${bands[i]}"/>
      <rect x="28" y="36" width="148" height="3" fill="${ink}" fill-opacity="0.18"/>
      <rect x="28" y="261" width="148" height="3" fill="${ink}" fill-opacity="0.18"/>
      ${texts}
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

function editionWord(n) {
  const abs = Math.abs(n) % 100;
  const d = abs % 10;
  if (abs > 10 && abs < 20) return "изданий";
  if (d === 1) return "издание";
  if (d >= 2 && d <= 4) return "издания";
  return "изданий";
}

function worksLabel(n) {
  const abs = Math.abs(n) % 100;
  const d = abs % 10;
  if (abs > 10 && abs < 20) return `${formatNum(n)} произведений`;
  if (d === 1) return `${formatNum(n)} произведение`;
  if (d >= 2 && d <= 4) return `${formatNum(n)} произведения`;
  return `${formatNum(n)} произведений`;
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
    const rateText = formatFantLab(b);
    const progText = formatProgress(b.progress);
    if (rateText || progText) {
      const row = document.createElement("p");
      row.className = "card-rate";
      if (rateText) {
        const star = document.createElement("span");
        star.textContent = "★ " + rateText;
        star.title = fantlabTitle(b);
        row.appendChild(star);
      }
      if (progText) {
        if (rateText) row.appendChild(document.createTextNode(" · "));
        const pr = document.createElement("span");
        pr.className = "card-progress";
        pr.textContent = progText;
        pr.title = "Прочитано";
        row.appendChild(pr);
      }
      el.querySelector(".meta").appendChild(row);
    }
    if (b.editionCount > 1) {
      const badge = document.createElement("span");
      badge.className = "edition-badge";
      badge.textContent = `${b.editionCount} ${editionWord(b.editionCount)}`;
      el.appendChild(badge);
    }
    const img = el.querySelector("img");
    img.onerror = () => { img.src = placeholderCover(b.title, b.authors); };
    el.addEventListener("click", () => openBook(b.id));
    el.addEventListener("keydown", (e) => {
      if (e.key === "Enter") openBook(b.id);
    });
    target.appendChild(el);
  }
}

function pageCount(total, limit) {
  return Math.max(1, Math.ceil((total || 0) / (limit || PAGE_SIZE)));
}

function syncResultsPager() {
  const nav = $("results-pager");
  const prev = $("pager-prev");
  const next = $("pager-next");
  const status = $("pager-status");
  if (!resultsPager || resultsPager.total <= resultsPager.limit) {
    nav.classList.add("hidden");
    return;
  }
  const { page, total, limit } = resultsPager;
  const pages = pageCount(total, limit);
  const from = (page - 1) * limit + 1;
  const to = Math.min(page * limit, total);
  status.textContent = `${from}–${to} из ${formatNum(total)} · стр. ${page}/${pages}`;
  prev.disabled = page <= 1;
  next.disabled = page >= pages;
  nav.classList.remove("hidden");
}

function emptySearch() {
  return { q: "", title: "", author: "", yearFrom: "", yearTo: "", addedFrom: "", addedTo: "" };
}

function readSearchFromForm() {
  return {
    q: (qInput.value || "").trim(),
    title: (advTitle.value || "").trim(),
    author: (advAuthor.value || "").trim(),
    yearFrom: (advYearFrom.value || "").trim(),
    yearTo: (advYearTo.value || "").trim(),
    addedFrom: (advAddedFrom.value || "").trim(),
    addedTo: (advAddedTo.value || "").trim(),
  };
}

function writeSearchToForm(s) {
  const p = s || emptySearch();
  qInput.value = p.q || "";
  advTitle.value = p.title || "";
  advAuthor.value = p.author || "";
  advYearFrom.value = p.yearFrom || "";
  advYearTo.value = p.yearTo || "";
  advAddedFrom.value = p.addedFrom || "";
  advAddedTo.value = p.addedTo || "";
}

function searchHasFilters(s) {
  return !!(s.q || s.title || s.author || s.yearFrom || s.yearTo || s.addedFrom || s.addedTo);
}

function searchIsAdvanced(s) {
  return !!(s.title || s.author || s.yearFrom || s.yearTo || s.addedFrom || s.addedTo);
}

function yearRangeLabel(from, to, prefix) {
  if (from && to && from === to) return prefix + from;
  if (from || to) return prefix + (from || "…") + "–" + (to || "…");
  return "";
}

function searchLabel(s) {
  const bits = [];
  if (s.q) bits.push(`«${s.q}»`);
  if (s.title) bits.push(`назв. «${s.title}»`);
  if (s.author) bits.push(`авт. «${s.author}»`);
  const pub = yearRangeLabel(s.yearFrom, s.yearTo, "изд. ");
  if (pub) bits.push(pub);
  const added = yearRangeLabel(s.addedFrom, s.addedTo, "доб. ");
  if (added) bits.push(added);
  return bits.length ? bits.join(" · ") : "";
}

function appendYearParams(qs, from, to, exactKey, fromKey, toKey) {
  if (from && to && from === to) {
    qs.set(exactKey, from);
    return;
  }
  if (from) qs.set(fromKey, from);
  if (to) qs.set(toKey, to);
}

function searchAPIURL(s, page) {
  const qs = new URLSearchParams();
  if (s.q) qs.set("q", s.q);
  if (s.title) qs.set("title", s.title);
  if (s.author) qs.set("author", s.author);
  appendYearParams(qs, s.yearFrom, s.yearTo, "year", "year_from", "year_to");
  appendYearParams(qs, s.addedFrom, s.addedTo, "added", "added_from", "added_to");
  qs.set("limit", String(PAGE_SIZE));
  qs.set("offset", String((Math.max(1, page || 1) - 1) * PAGE_SIZE));
  return "/api/search?" + qs.toString();
}

function searchPageURL(s, page) {
  const qs = new URLSearchParams();
  if (s.q) qs.set("q", s.q);
  if (s.title) qs.set("title", s.title);
  // "au" — текст автора в поиске; "author" занят id страницы автора.
  if (s.author) qs.set("au", s.author);
  appendYearParams(qs, s.yearFrom, s.yearTo, "year", "year_from", "year_to");
  appendYearParams(qs, s.addedFrom, s.addedTo, "added", "added_from", "added_to");
  if (page > 1) qs.set("p", String(page));
  const str = qs.toString();
  return str ? "/?" + str : "/";
}

function yearRangeFromParams(params, exactKey, fromKey, toKey) {
  const exact = (params.get(exactKey) || "").trim();
  let from = (params.get(fromKey) || "").trim();
  let to = (params.get(toKey) || "").trim();
  if (exact && !from && !to) {
    from = exact;
    to = exact;
  }
  return { from, to };
}

function searchFromURLParams(params) {
  const year = yearRangeFromParams(params, "year", "year_from", "year_to");
  const added = yearRangeFromParams(params, "added", "added_from", "added_to");
  return {
    q: (params.get("q") || "").trim(),
    title: (params.get("title") || "").trim(),
    author: (params.get("au") || "").trim(),
    yearFrom: year.from,
    yearTo: year.to,
    addedFrom: added.from,
    addedTo: added.to,
  };
}

function setAdvOpen(open) {
  advPanel.classList.toggle("hidden", !open);
  advToggle.setAttribute("aria-expanded", open ? "true" : "false");
}

/** @type {{ id: number, name: string, books?: number }[]} */
let searchAuthorsAll = [];
let searchAuthorsExpanded = false;

function appendAuthorChip(list, a) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "author-series-chip";
  const name = document.createElement("span");
  name.textContent = a.name || "Автор";
  btn.appendChild(name);
  if (a.books) {
    const count = document.createElement("span");
    count.className = "author-series-count";
    count.textContent = formatNum(a.books);
    btn.appendChild(count);
  }
  btn.addEventListener("click", () => {
    listReturn = { type: "search" };
    openAuthor(a.id);
  });
  list.appendChild(btn);
}

function paintSearchAuthors() {
  const box = $("search-authors");
  const list = $("search-authors-list");
  const label = box.querySelector(".author-series-label");
  list.innerHTML = "";
  const all = searchAuthorsAll;
  if (!all.length) {
    box.classList.add("hidden");
    return;
  }
  if (label) {
    label.textContent = all.length > AUTHOR_CHIP_PREVIEW
      ? `Авторы · ${formatNum(all.length)}`
      : "Авторы";
  }
  const hidden = !searchAuthorsExpanded && all.length > AUTHOR_CHIP_PREVIEW;
  const visible = hidden ? all.slice(0, AUTHOR_CHIP_PREVIEW) : all;
  for (const a of visible) appendAuthorChip(list, a);
  if (all.length > AUTHOR_CHIP_PREVIEW) {
    const more = document.createElement("button");
    more.type = "button";
    more.className = "author-series-chip author-series-more";
    more.setAttribute("aria-expanded", searchAuthorsExpanded ? "true" : "false");
    if (searchAuthorsExpanded) {
      more.textContent = "Свернуть";
    } else {
      const rest = all.length - AUTHOR_CHIP_PREVIEW;
      const name = document.createElement("span");
      name.textContent = "…";
      const count = document.createElement("span");
      count.className = "author-series-count";
      count.textContent = formatNum(rest);
      more.appendChild(name);
      more.appendChild(count);
      more.title = `Показать ещё ${formatNum(rest)}`;
      more.setAttribute("aria-label", `Показать ещё ${formatNum(rest)} авторов`);
    }
    more.addEventListener("click", () => {
      searchAuthorsExpanded = !searchAuthorsExpanded;
      paintSearchAuthors();
    });
    list.appendChild(more);
  }
  box.classList.remove("hidden");
}

function renderSearchAuthors(authors) {
  searchAuthorsAll = authors || [];
  searchAuthorsExpanded = false;
  paintSearchAuthors();
}

function renderResults(search, books, total, page, authors) {
  listContext = null;
  lastSearch = search || emptySearch();
  lastBooks = books || [];
  resultsPager = {
    mode: "search",
    key: lastSearch,
    page: page || 1,
    total: total || lastBooks.length,
    limit: PAGE_SIZE,
  };
  resultsBack.classList.add("hidden");
  resultsSub.classList.remove("hidden");
  $("author-series").classList.add("hidden");
  $("list-sort").classList.add("hidden");
  if (page <= 1) {
    renderSearchAuthors(authors);
  } else {
    $("search-authors").classList.add("hidden");
  }
  const label = searchLabel(lastSearch);
  $("results-title").textContent = label ? `Поиск: ${label}` : "Результаты";
  resultsSub.textContent = total ? worksLabel(total) : "";
  if (!total && !(authors && authors.length)) resultsSub.classList.add("hidden");
  renderBookGrid(lastBooks);
  syncResultsPager();
  show("results");
}

function renderAuthorSeries(series, authorId) {
  const box = $("author-series");
  const list = $("author-series-list");
  list.innerHTML = "";
  const items = series || [];
  if (!items.length) {
    box.classList.add("hidden");
    return;
  }
  for (const s of items) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "author-series-chip";
    const name = document.createElement("span");
    name.textContent = s.title || s.name || "Серия";
    btn.appendChild(name);
    if (s.books) {
      const count = document.createElement("span");
      count.className = "author-series-count";
      count.textContent = formatNum(s.books);
      btn.appendChild(count);
    }
    btn.addEventListener("click", () => {
      listReturn = { type: "author", id: authorId };
      openSeries(s.id);
    });
    list.appendChild(btn);
  }
  box.classList.remove("hidden");
}

function paintListSort() {
  const box = $("list-sort");
  if (listContext?.kind !== "genre") {
    box.classList.add("hidden");
    return;
  }
  box.classList.remove("hidden");
  box.querySelectorAll("[data-sort]").forEach((btn) => {
    btn.classList.toggle("is-active", btn.getAttribute("data-sort") === genreSort);
  });
}

function renderNamedList(kind, data, page) {
  if (kind === "genre" && data.sort) genreSort = normalizeGenreSort(data.sort);
  listContext = { kind, id: data.id, name: data.name, series: data.series || null };
  lastBooks = data.books || [];
  resultsPager = {
    mode: kind,
    key: kind === "genre" ? data.id : data.id,
    page: page || 1,
    total: data.total || lastBooks.length,
    limit: PAGE_SIZE,
    sort: kind === "genre" ? genreSort : "",
  };
  resultsBack.classList.remove("hidden");
  resultsSub.classList.remove("hidden");
  const label = kind === "author" ? "Автор" : kind === "series" ? "Серия" : "Жанр";
  $("results-title").textContent = `${label}: ${data.name}`;
  resultsSub.textContent = worksLabel(data.total || lastBooks.length);
  $("search-authors").classList.add("hidden");
  if (kind === "author") {
    renderAuthorSeries(data.series, data.id);
  } else {
    $("author-series").classList.add("hidden");
  }
  paintListSort();
  renderBookGrid(lastBooks);
  syncResultsPager();
  show("results");
}

async function doSearch(search, page) {
  const s =
    typeof search === "string"
      ? { ...emptySearch(), q: (search || "").trim() }
      : {
          q: (search?.q || "").trim(),
          title: (search?.title || "").trim(),
          author: (search?.author || "").trim(),
          yearFrom: String(search?.yearFrom || "").trim(),
          yearTo: String(search?.yearTo || "").trim(),
          addedFrom: String(search?.addedFrom || "").trim(),
          addedTo: String(search?.addedTo || "").trim(),
        };
  page = Math.max(1, page || 1);
  writeSearchToForm(s);
  if (searchIsAdvanced(s)) setAdvOpen(true);
  if (!searchHasFilters(s)) {
    listContext = null;
    lastSearch = emptySearch();
    resultsPager = null;
    syncResultsPager();
    show("home");
    history.replaceState(null, "", "/");
    loadContinue();
    return;
  }
  history.replaceState(null, "", searchPageURL(s, page));
  const res = await api(searchAPIURL(s, page));
  if (!res.ok) {
    alert("Ошибка поиска: " + (await res.text()));
    return;
  }
  const data = await res.json();
  const total = data.total || 0;
  const pages = pageCount(total, PAGE_SIZE);
  if (page > pages && total > 0) {
    return doSearch(s, pages);
  }
  renderResults(s, data.books || [], total, page, page <= 1 ? data.authors || [] : []);
  window.scrollTo(0, 0);
}

async function openAuthor(id, page) {
  page = Math.max(1, page || 1);
  const offset = (page - 1) * PAGE_SIZE;
  const res = await api("/api/author/" + id + "?limit=" + PAGE_SIZE + "&offset=" + offset);
  if (!res.ok) {
    alert("Автор недоступен");
    return;
  }
  const data = await res.json();
  const pages = pageCount(data.total, PAGE_SIZE);
  if (page > pages && data.total > 0) {
    return openAuthor(id, pages);
  }
  history.pushState({ author: id, p: page }, "", resultsURLFrom("author", id, page));
  renderNamedList("author", data, page);
  window.scrollTo(0, 0);
}

async function openSeries(id, page) {
  page = Math.max(1, page || 1);
  const offset = (page - 1) * PAGE_SIZE;
  const res = await api("/api/series/" + id + "?limit=" + PAGE_SIZE + "&offset=" + offset);
  if (!res.ok) {
    alert("Серия недоступна");
    return;
  }
  const data = await res.json();
  const pages = pageCount(data.total, PAGE_SIZE);
  if (page > pages && data.total > 0) {
    return openSeries(id, pages);
  }
  history.pushState({ series: id, p: page }, "", resultsURLFrom("series", id, page));
  renderNamedList("series", data, page);
  window.scrollTo(0, 0);
}

function resultsURLFrom(mode, key, page, sort) {
  const p = page > 1 ? "&p=" + page : "";
  if (mode === "search") return searchPageURL(typeof key === "string" ? { ...emptySearch(), q: key } : key, page);
  if (mode === "author") return "/?author=" + key + p;
  if (mode === "series") return "/?series=" + key + p;
  if (mode === "genre") {
    const s = normalizeGenreSort(sort || genreSort);
    const sortQS = s !== "popular" ? "&sort=" + encodeURIComponent(s) : "";
    return "/?genre=" + encodeURIComponent(key) + sortQS + p;
  }
  return "/";
}

async function goResultsPage(delta) {
  if (!resultsPager) return;
  const page = resultsPager.page + delta;
  const pages = pageCount(resultsPager.total, resultsPager.limit);
  if (page < 1 || page > pages) return;
  const { mode, key } = resultsPager;
  if (mode === "search") return doSearch(key, page);
  if (mode === "author") return openAuthor(key, page);
  if (mode === "series") return openSeries(key, page);
  if (mode === "genre") return openGenre(key, page, resultsPager.sort);
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

function formatEditionRow(ed) {
  let main;
  if (ed.translators && ed.translators.length) {
    main = "Перевод: " + ed.translators.join(", ");
  } else if (ed.publisher || ed.pubYear || ed.year) {
    main = [ed.publisher, ed.pubYear || (ed.year ? String(ed.year) : "")]
      .filter(Boolean)
      .join(", ");
  } else {
    main = "Издание";
  }
  const subBits = [];
  if (ed.translators && ed.translators.length) {
    if (ed.publisher) subBits.push(ed.publisher);
    if (ed.city) subBits.push(ed.city);
    if (ed.pubYear) subBits.push(ed.pubYear);
    else if (ed.year) subBits.push(String(ed.year));
  } else if (ed.city) {
    subBits.push(ed.city);
  }
  if (ed.size) subBits.push(formatSize(ed.size));
  if (ed.series) {
    subBits.push(ed.seriesNum ? `${ed.series} — ${ed.seriesNum}` : ed.series);
  }
  return { main, sub: subBits.join(" · ") };
}

function renderEditions(b) {
  const box = $("book-editions");
  const list = $("book-editions-list");
  list.innerHTML = "";
  const editions = b.editions || [];
  if (editions.length <= 1) {
    box.classList.add("hidden");
    return;
  }
  for (const ed of editions) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "edition-row" + (ed.current || ed.id === b.id ? " is-current" : "");
    const { main, sub } = formatEditionRow(ed);
    btn.innerHTML = `<span class="edition-main"></span><span class="edition-sub"></span>`;
    const isCurrent = !!(ed.current || ed.id === b.id);
    btn.querySelector(".edition-main").textContent = main + (isCurrent ? " · текущее" : "");
    btn.querySelector(".edition-sub").textContent = sub;
    if (isCurrent) {
      btn.setAttribute("aria-current", "true");
    } else {
      btn.addEventListener("click", () => openBook(ed.id));
    }
    li.appendChild(btn);
    list.appendChild(li);
  }
  box.classList.remove("hidden");
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
  if (listContext?.kind === "lists") {
    qs.set("lists", listContext.status || shelfTab);
  } else if (searchHasFilters(lastSearch)) {
    const searchQS = new URLSearchParams(searchPageURL(lastSearch, 1).slice(2));
    searchQS.forEach((v, k) => {
      if (k !== "p") qs.set(k, v);
    });
  }
  if (listContext?.kind === "author") qs.set("author", listContext.id);
  if (listContext?.kind === "series") qs.set("series", listContext.id);
  history.pushState({ book: id }, "", "/?" + qs.toString());
  $("back").textContent = listContext?.kind === "lists" ? "К списку" : "К результатам";

  $("book-title").textContent = b.title;

  const authorsEl = $("book-authors");
  authorsEl.innerHTML = "";
  const authors = b.authorList || [];
  if (authors.length === 0 && b.authors) {
    authorsEl.textContent = b.authors;
  } else {
    authors.forEach((a, i) => {
      if (i) authorsEl.appendChild(document.createTextNode(", "));
      authorsEl.appendChild(
        linkButton(a.name, () => {
          listReturn = { type: "book", id: currentBookId };
          openAuthor(a.id);
        })
      );
    });
  }

  const seriesEl = $("book-series");
  seriesEl.innerHTML = "";
  if (b.series && b.seriesId) {
    seriesEl.appendChild(document.createTextNode("Серия: "));
    const label = b.seriesNum ? `${b.series} — ${b.seriesNum}` : b.series;
    seriesEl.appendChild(
      linkButton(label, () => {
        listReturn = { type: "book", id: currentBookId };
        openSeries(b.seriesId);
      })
    );
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
  const rateText = formatFantLab(b);
  if (rateText) bits.push("★ " + rateText);
  $("book-info").textContent = bits.join(" · ");
  $("book-info").title = fantlabTitle(b);

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
  cover.onerror = () => { cover.src = placeholderCover(b.title, b.authors); };
  $("book-download").href = b.downloadUrl;
  updateReadButton(b.progress || 0);
  renderEditions(b);

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
  if (pos != null) restoreReaderPositionSoon(pos);
}

function readerFontDef() {
  return READER_FONTS.find((f) => f.id === readerFont) || READER_FONTS[0];
}

function applyReaderFont(opts = {}) {
  const keepPosition = opts.keepPosition !== false;
  const pos = keepPosition && readerBookId ? readerPosition() : null;
  const def = readerFontDef();
  const el = $("reader-content");
  if (el) el.style.fontFamily = def.family;
  document.body.dataset.readerFont = def.id;
  localStorage.setItem("libshelf_reader_font", def.id);
  const btn = $("reader-face-btn");
  if (btn) {
    btn.title = "Шрифт: " + def.label;
    btn.setAttribute("aria-label", btn.title);
    btn.classList.toggle("is-serif", def.id === "serif");
  }
  if (pos != null) restoreReaderPositionSoon(pos);
}

function cycleReaderFont() {
  const i = READER_FONTS.findIndex((f) => f.id === readerFont);
  readerFont = READER_FONTS[(i + 1) % READER_FONTS.length].id;
  applyReaderFont();
}

function readerContentEl() {
  return $("reader-content");
}

function readerViewportEl() {
  return document.querySelector(".reader-viewport");
}

function notePopEl() {
  return $("note-pop");
}

function notePopOpen() {
  const el = notePopEl();
  return !!(el && !el.classList.contains("hidden"));
}

function hideNotePop() {
  const el = notePopEl();
  if (!el) return;
  el.classList.add("hidden");
  el.hidden = true;
  const body = $("note-pop-body");
  if (body) body.innerHTML = "";
}

function showNotePop(noteEl) {
  const pop = notePopEl();
  const body = $("note-pop-body");
  const title = $("note-pop-title");
  if (!pop || !body || !noteEl) return;
  const clone = noteEl.cloneNode(true);
  const label = clone.querySelector(".fb2-note-label");
  const labelText = label ? label.textContent.trim() : "";
  if (label) label.remove();
  title.textContent = labelText ? "Примечание " + labelText : "Примечание";
  body.innerHTML = "";
  while (clone.firstChild) body.appendChild(clone.firstChild);
  pop.classList.remove("hidden");
  pop.hidden = false;
}

function readerNoteTarget(anchor) {
  if (!anchor) return null;
  const href = anchor.getAttribute("href") || "";
  if (!href.startsWith("#") || href.length < 2) return null;
  let id = href.slice(1);
  try {
    id = decodeURIComponent(id);
  } catch {
    /* keep raw id */
  }
  if (!id) return null;
  const note = document.getElementById(id);
  if (!note) return null;
  if (note.classList.contains("fb2-note") || note.closest(".fb2-notes")) return note;
  return null;
}

function tocPopOpen() {
  const el = $("toc-pop");
  return !!(el && !el.classList.contains("hidden"));
}

function hideTocPop() {
  const el = $("toc-pop");
  if (!el) return;
  el.classList.add("hidden");
  el.hidden = true;
}

function renderTocList() {
  const list = $("toc-pop-list");
  if (!list) return;
  list.innerHTML = "";
  readerChapters.forEach((ch, i) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "toc-item";
    btn.dataset.chapterId = ch.id;
    btn.textContent = (ch.title || "").trim() || ("Глава " + (i + 1));
    btn.addEventListener("click", () => jumpToChapter(ch.id));
    list.appendChild(btn);
  });
}

function chapterOffsetY(id) {
  const content = readerContentEl();
  const el = document.getElementById(id);
  if (!content || !el || !content.contains(el)) return null;
  return el.getBoundingClientRect().top - content.getBoundingClientRect().top;
}

function currentChapterIndex() {
  if (!readerChapters.length) return -1;
  const content = readerContentEl();
  if (!content) return 0;
  let y = 0;
  if (pageModeActive()) {
    y = (readerPageOffsets[readerPageIndex] || 0) + 12;
  } else {
    const vp = scrollViewportEl();
    y = vp ? vp.scrollTop + 12 : 0;
  }
  let best = 0;
  for (let i = 0; i < readerChapters.length; i++) {
    const top = chapterOffsetY(readerChapters[i].id);
    if (top == null) continue;
    if (top <= y) best = i;
  }
  return best;
}

function updateTocBar() {
  const bar = $("reader-toc-bar");
  if (!bar) return;
  const show = !!(readerBookId && readerChapters.length >= 2);
  bar.classList.toggle("hidden", !show);
  if (!show) return;
  const idx = Math.max(0, currentChapterIndex());
  const ch = readerChapters[idx];
  const title = $("reader-toc-title");
  if (title) title.textContent = (ch && ch.title) || "";
  const pctEl = $("reader-toc-pct");
  if (pctEl) {
    const pct = Math.round(readerPosition() * 100);
    pctEl.textContent = pct > 0 ? pct + "%" : "";
  }
  document.querySelectorAll("#toc-pop-list .toc-item").forEach((el, i) => {
    el.classList.toggle("is-current", i === idx);
  });
}

function scheduleTocUpdate() {
  if (tocTick) return;
  tocTick = requestAnimationFrame(() => {
    tocTick = 0;
    updateTocBar();
  });
}

function showTocPop() {
  if (readerChapters.length < 2) return;
  updateTocBar();
  const pop = $("toc-pop");
  if (!pop) return;
  pop.classList.remove("hidden");
  pop.hidden = false;
  pop.querySelector(".toc-item.is-current")?.scrollIntoView({ block: "nearest" });
}

function jumpToChapter(id) {
  hideTocPop();
  if (pageModeActive()) {
    rebuildReaderPages();
    const y = chapterOffsetY(id);
    if (y == null) return;
    let best = 0;
    for (let i = 0; i < readerPageOffsets.length; i++) {
      if (readerPageOffsets[i] <= y + 1) best = i;
      else break;
    }
    readerPageIndex = best;
    applyPageTransform(false);
  } else {
    const y = chapterOffsetY(id);
    if (y == null) return;
    const vp = scrollViewportEl();
    if (vp) vp.scrollTop = Math.max(0, y - 6);
  }
  lastGoodReaderPos = readerPosition();
  scheduleSaveProgress();
  updateTocBar();
}

function onReaderLinkClick(e) {
  const el = eventElement(e.target);
  const a = el?.closest?.("a");
  if (!a || !readerContentEl()?.contains(a)) return;
  if (a.classList.contains("fb2-ext")) return;
  const note = readerNoteTarget(a);
  if (!note) {
    if ((a.getAttribute("href") || "").startsWith("#")) e.preventDefault();
    return;
  }
  e.preventDefault();
  e.stopPropagation();
  showNotePop(note);
}

/** Visible size of the page viewport (layout can be taller than the screen with browser chrome). */
function pageViewportBox() {
  const vp = readerViewportEl();
  let h = vp ? vp.clientHeight : 0;
  let w = vp ? vp.clientWidth : 0;

  const vv = window.visualViewport;
  if (vv && vp) {
    const rect = vp.getBoundingClientRect();
    const visTop = Math.max(rect.top, vv.offsetTop);
    const visBottom = Math.min(rect.bottom, vv.offsetTop + vv.height);
    const visLeft = Math.max(rect.left, vv.offsetLeft);
    const visRight = Math.min(rect.right, vv.offsetLeft + vv.width);
    const visibleH = visBottom - visTop;
    const visibleW = visRight - visLeft;
    if (visibleH >= 80) h = h > 0 ? Math.min(h, visibleH) : visibleH;
    if (visibleW >= 80) w = w > 0 ? Math.min(w, visibleW) : visibleW;
  } else if (vv) {
    if (vv.height >= 80) h = h > 0 ? Math.min(h, vv.height) : vv.height;
    if (vv.width >= 80) w = w > 0 ? Math.min(w, vv.width) : vv.width;
  }

  if (h < 80 || w < 80) {
    const chromeOn = !document.body.classList.contains("reader-chrome-hidden");
    const bar = document.querySelector(".reader-bar");
    const toc = $("reader-toc-bar");
    const barH = chromeOn && bar ? bar.offsetHeight : 0;
    const tocH = chromeOn && toc && !toc.classList.contains("hidden") ? toc.offsetHeight : 0;
    const fallbackH = (vv && vv.height >= 80 ? vv.height : window.innerHeight) - barH - tocH;
    const fallbackW = vv && vv.width >= 80 ? vv.width : window.innerWidth;
    if (h < 80) h = fallbackH;
    if (w < 80) w = fallbackW;
  }
  return { h: Math.max(80, h - 2), w: Math.max(80, w) };
}

function pageViewportHeight() {
  return pageViewportBox().h;
}

function clearPageLayoutStyles(el) {
  if (!el) return;
  el.style.height = "";
  el.style.width = "";
  el.style.maxWidth = "";
  el.style.columnWidth = "";
  el.style.columnGap = "";
  el.style.columnFill = "";
  el.style.boxSizing = "";
  el.style.opacity = "";
}

/** Bottoms of line/box fragments, relative to the content element top. */
function collectLineBottoms(root) {
  const rootRect = root.getBoundingClientRect();
  const bottoms = [];
  const add = (y) => {
    if (y > 0.5) bottoms.push(y);
  };

  root.querySelectorAll("p, h1, h2, h3, h4, li, blockquote, .chapter, .fb2-img, img, pre, hr").forEach((el) => {
    const r = el.getBoundingClientRect();
    if (r.height < 1) return;
    add(r.bottom - rootRect.top);
  });

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let node;
  while ((node = walker.nextNode())) {
    if (!/\S/.test(node.nodeValue || "")) continue;
    const range = document.createRange();
    try {
      range.selectNodeContents(node);
      const rects = range.getClientRects();
      for (let i = 0; i < rects.length; i++) {
        const r = rects[i];
        if (r.height < 1 || r.width < 1) continue;
        add(r.bottom - rootRect.top);
      }
    } catch {
      /* ignore detached nodes */
    }
  }

  bottoms.sort((a, b) => a - b);
  const out = [];
  for (const y of bottoms) {
    if (!out.length || y - out[out.length - 1] > 0.75) out.push(y);
  }
  return out;
}

/**
  Build page offsets (line-snapped translateY windows).
  Both "вбок" and "вниз" use this — CSS multicol on a whole book is too slow on phones.
  "Вбок" only changes the swipe axis / flip animation.
 */
function rebuildReaderPages() {
  const content = readerContentEl();
  if (!content) {
    readerPageOffsets = [0];
    readerPageStride = 0;
    return;
  }

  clearPageLayoutStyles(content);
  content.style.transition = "none";
  content.style.transform = "none";
  content.style.clipPath = "none";

  const H = pageViewportHeight();
  const total = content.scrollHeight;
  if (H < 80 || total <= H + 1) {
    readerPageOffsets = [0];
    readerPageStride = H;
    return;
  }

  const bottoms = collectLineBottoms(content);
  const offsets = [0];
  let pageTop = 0;
  const eps = 1;

  while (pageTop + H < total - eps) {
    const limit = pageTop + H - 4;
    let best = -1;
    for (let i = 0; i < bottoms.length; i++) {
      const b = bottoms[i];
      if (b <= pageTop + eps) continue;
      if (b > limit) break;
      best = b;
    }

    let nextTop = best > pageTop + eps ? best : pageTop + H;
    if (nextTop <= pageTop + 1) nextTop = pageTop + H;
    if (nextTop >= total - 4) break;

    offsets.push(nextTop);
    pageTop = nextTop;
    if (offsets.length > 20000) break;
  }

  readerPageOffsets = offsets;
  readerPageStride = H;
  if (readerPageIndex > offsets.length - 1) readerPageIndex = offsets.length - 1;
}

function maxReaderPageIndex() {
  return Math.max(0, readerPageOffsets.length - 1);
}

/** Tap / buttons / keys — snappier. Drag completion keeps a softer settle. */
const PAGE_SLIDE_TAP_MS = 250;
const PAGE_SLIDE_DRAG_MS = 420;
const PAGE_EASE = "cubic-bezier(0.22, 0.61, 0.36, 1)";
let pageFlipBusy = false;
let pageFlipTimer = 0;

function clearPageTransform(el) {
  if (!el) return;
  el.style.transform = "";
  el.style.clipPath = "";
  el.style.transition = "";
  el.style.opacity = "";
}

function pageWindowMetrics() {
  const el = readerContentEl();
  if (!el) return null;
  const off = readerPageOffsets[readerPageIndex] || 0;
  const total = el.scrollHeight;
  const next = readerPageOffsets[readerPageIndex + 1];
  const bottom = next != null ? next : total;
  return {
    el,
    off,
    total,
    bottom,
    clip: `inset(${Math.max(0, off)}px 0 ${Math.max(0, total - bottom)}px 0)`,
  };
}

/** Snap content to the current page window (no animation). */
function settlePageTransform() {
  const m = pageWindowMetrics();
  if (!m) return;
  m.el.style.transition = "none";
  m.el.style.transform = `translate3d(0, ${-m.off}px, 0)`;
  m.el.style.clipPath = m.clip;
  m.el.style.opacity = "1";
}

function applyPageTransform(smooth, flipDir = 0) {
  if (!pageModeActive()) {
    clearPageTransform(readerContentEl());
    clearPageLayoutStyles(readerContentEl());
    return;
  }
  if (smooth && flipDir) {
    animatePageFlip(flipDir, 0);
    return;
  }
  settlePageTransform();
}

/**
 * Slide the current page off-screen, then snap to the neighbour page.
 * fromSlide: continue a finger-drag (px along the flip axis).
 */
function animatePageFlip(dir, fromSlide = 0) {
  if (!pageModeActive() || pageFlipBusy) return false;
  if (readerPageOffsets.length <= 1) rebuildReaderPages();
  const nextIdx = readerPageIndex + dir;
  if (nextIdx < 0 || nextIdx > maxReaderPageIndex()) {
    animatePageSlideBack(fromSlide);
    return false;
  }

  const m = pageWindowMetrics();
  if (!m) return false;
  pageFlipBusy = true;
  const horizontal = pageFlipHorizontal();
  const span = horizontal ? pageViewportBox().w : pageViewportBox().h;
  const toSlide = dir > 0 ? -span : span;
  const ms = Math.abs(fromSlide) > 8 ? PAGE_SLIDE_DRAG_MS : PAGE_SLIDE_TAP_MS;

  m.el.style.clipPath = m.clip;
  m.el.style.opacity = "1";
  m.el.style.transition = "none";
  if (horizontal) {
    m.el.style.transform = `translate3d(${fromSlide}px, ${-m.off}px, 0)`;
  } else {
    m.el.style.transform = `translate3d(0, ${-m.off + fromSlide}px, 0)`;
  }
  void m.el.offsetWidth;

  let finished = false;
  const finish = () => {
    if (finished) return;
    finished = true;
    m.el.removeEventListener("transitionend", onEnd);
    if (pageFlipTimer) {
      clearTimeout(pageFlipTimer);
      pageFlipTimer = 0;
    }
    readerPageIndex = nextIdx;
    settlePageTransform();
    pageFlipBusy = false;
    scheduleSaveProgress();
    scheduleTocUpdate();
  };
  const onEnd = (ev) => {
    if (ev && ev.target !== m.el) return;
    if (ev && ev.propertyName && ev.propertyName !== "transform") return;
    finish();
  };
  m.el.addEventListener("transitionend", onEnd);
  pageFlipTimer = setTimeout(finish, ms + 100);

  requestAnimationFrame(() => {
    m.el.style.transition = `transform ${ms}ms ${PAGE_EASE}`;
    if (horizontal) {
      m.el.style.transform = `translate3d(${toSlide}px, ${-m.off}px, 0)`;
    } else {
      m.el.style.transform = `translate3d(0, ${-m.off + toSlide}px, 0)`;
    }
  });
  return true;
}

/** Spring back after an unfinished drag. */
function animatePageSlideBack(fromSlide = 0) {
  const m = pageWindowMetrics();
  if (!m) return;
  const horizontal = pageFlipHorizontal();
  m.el.style.clipPath = m.clip;
  m.el.style.transition = "none";
  if (horizontal) {
    m.el.style.transform = `translate3d(${fromSlide}px, ${-m.off}px, 0)`;
  } else {
    m.el.style.transform = `translate3d(0, ${-m.off + fromSlide}px, 0)`;
  }
  void m.el.offsetWidth;
  m.el.style.transition = `transform ${Math.round(PAGE_SLIDE_TAP_MS * 0.85)}ms ${PAGE_EASE}`;
  m.el.style.transform = `translate3d(0, ${-m.off}px, 0)`;
}

function applyPageDragSlide(slide) {
  const m = pageWindowMetrics();
  if (!m || pageFlipBusy) return;
  const horizontal = pageFlipHorizontal();
  m.el.style.transition = "none";
  m.el.style.clipPath = m.clip;
  if (horizontal) {
    m.el.style.transform = `translate3d(${slide}px, ${-m.off}px, 0)`;
  } else {
    m.el.style.transform = `translate3d(0, ${-m.off + slide}px, 0)`;
  }
}

/** Rubber-band when dragging past the first/last page. */
function resistPageDrag(slide) {
  const atStart = readerPageIndex <= 0;
  const atEnd = readerPageIndex >= maxReaderPageIndex();
  // slide < 0 → toward next; slide > 0 → toward previous
  if (slide < 0 && atEnd) return slide * 0.22;
  if (slide > 0 && atStart) return slide * 0.22;
  return slide;
}

function lockPageScroll(on) {
  document.documentElement.classList.toggle("reader-pages-lock", !!on);
  if (on) window.scrollTo(0, 0);
}

function applyReadMode() {
  const paging = pageModeActive();
  // Only while reading — otherwise page-mode CSS leaks onto the catalog.
  document.body.classList.toggle("reader-pages", paging);
  document.body.classList.toggle("reader-pages-h", paging && readMode === "pages-h");
  document.body.classList.toggle("reader-pages-v", paging && readMode === "pages-v");
  if (!paging) document.body.classList.remove("reader-chrome-hidden");
  lockPageScroll(paging);
  const btn = $("reader-mode-btn");
  if (btn) {
    const pagesH = btn.querySelector(".reader-ico-pages-h");
    const pagesV = btn.querySelector(".reader-ico-pages-v");
    const scrollIco = btn.querySelector(".reader-ico-scroll");
    pagesH?.classList.toggle("hidden", readMode !== "pages-h");
    pagesV?.classList.toggle("hidden", readMode !== "pages-v");
    scrollIco?.classList.toggle("hidden", readMode !== "scroll");
    // Title describes the next mode on click.
    const titles = {
      "pages-h": "Листать вниз",
      "pages-v": "Сплошной текст",
      scroll: "Листать вбок",
    };
    btn.title = titles[readMode] || "Режим чтения";
    btn.setAttribute("aria-label", btn.title);
  }
  localStorage.setItem("libshelf_read_mode", readMode);
  const el = readerContentEl();
  if (el && !paging) {
    clearPageTransform(el);
    clearPageLayoutStyles(el);
  } else if (el) {
    clearPageLayoutStyles(el);
  }
}

function applyTextAlign() {
  const justify = textAlign === "justify";
  document.body.classList.toggle("reader-align-justify", justify);
  const btn = $("reader-align-btn");
  if (btn) {
    btn.querySelector(".reader-ico-justify")?.classList.toggle("hidden", !justify);
    btn.querySelector(".reader-ico-left")?.classList.toggle("hidden", justify);
    btn.title = justify ? "По левому краю" : "По ширине";
    btn.setAttribute("aria-label", btn.title);
  }
  localStorage.setItem("libshelf_align", textAlign);
}

function setTextAlign(mode) {
  textAlign = mode === "left" ? "left" : "justify";
  const pos = readerBookId ? readerPosition() : null;
  applyTextAlign();
  if (pos != null) restoreReaderPositionSoon(pos);
}

function clampReaderPos(pos) {
  const p = Number(pos);
  if (!Number.isFinite(p)) return 0;
  return Math.min(1, Math.max(0, p));
}

function pinReaderPlace() {
  if (!readerBookId) return;
  pinnedReaderPos = readerPosition();
  if (pinReaderTimer) clearTimeout(pinReaderTimer);
  pinReaderTimer = setTimeout(() => {
    pinnedReaderPos = null;
    pinReaderTimer = 0;
  }, 600);
}

function scrollViewportEl() {
  return readerViewportEl();
}

function pageLayoutCollapsed(total, viewH) {
  if (viewH < 80 || total < 40) return true;
  if (lastKnownContentH > viewH * 2 && total <= viewH + 8) return true;
  return false;
}

function readReaderPosition() {
  if (pageModeActive()) {
    const el = readerContentEl();
    const total = el ? el.scrollHeight : 0;
    const viewH = pageViewportHeight();
    if (pageLayoutCollapsed(total, viewH)) return null;
    lastKnownContentH = total;
    const maxScroll = Math.max(0, total - viewH);
    if (maxScroll <= 0) return 0;
    const y = readerPageOffsets[readerPageIndex] || 0;
    return clampReaderPos(y / maxScroll);
  }
  const vp = scrollViewportEl();
  if (!vp) return null;
  const total = vp.scrollHeight;
  const viewH = vp.clientHeight;
  if (pageLayoutCollapsed(total, viewH)) return null;
  lastKnownContentH = total;
  const max = total - viewH;
  if (max <= 0) return 0;
  return clampReaderPos(vp.scrollTop / max);
}

function readerPosition() {
  const raw = readReaderPosition();
  if (raw == null) return pinnedReaderPos ?? lastGoodReaderPos;
  if (raw < 0.001 && pinnedReaderPos != null && pinnedReaderPos > 0.001) {
    return pinnedReaderPos;
  }
  lastGoodReaderPos = raw;
  return raw;
}

function restoreReaderPositionSoon(pos) {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => restoreReaderPosition(pos));
  });
}

function restoreReaderPosition(pos, opts = {}) {
  if (!readerBookId) return;
  const p = clampReaderPos(pos);
  lastGoodReaderPos = p;
  const relayout = opts.relayout !== false;

  if (pageModeActive()) {
    const el = readerContentEl();
    const total = el ? el.scrollHeight : 0;
    const viewH = pageViewportHeight();
    if (pageLayoutCollapsed(total, viewH)) {
      if (restorePlaceTries++ < 12) {
        requestAnimationFrame(() => restoreReaderPosition(p, opts));
      } else {
        restorePlaceTries = 0;
      }
      return;
    }
    restorePlaceTries = 0;
    if (relayout) rebuildReaderPages();
    const laidOut = el ? el.scrollHeight : 0;
    const maxScroll = Math.max(0, laidOut - pageViewportHeight());
    const targetY = p * maxScroll;
    let best = 0;
    for (let i = 0; i < readerPageOffsets.length; i++) {
      if (readerPageOffsets[i] <= targetY + 1) best = i;
      else break;
    }
    readerPageIndex = best;
    applyPageTransform(false);
    scheduleTocUpdate();
    return;
  }

  const vp = scrollViewportEl();
  if (!vp || vp.clientHeight < 40) {
    if (restorePlaceTries++ < 12) {
      requestAnimationFrame(() => restoreReaderPosition(p, opts));
    } else {
      restorePlaceTries = 0;
    }
    return;
  }
  restorePlaceTries = 0;
  const max = vp.scrollHeight - vp.clientHeight;
  vp.scrollTop = max > 0 ? p * max : 0;
  scheduleTocUpdate();
}

function flipReaderPage(dir, fromSlide = 0) {
  if (!pageModeActive()) return;
  animatePageFlip(dir, fromSlide);
}

function setReadMode(mode) {
  const allowed = { "pages-h": 1, "pages-v": 1, scroll: 1 };
  const next = allowed[mode] ? mode : "scroll";
  const pos = readerBookId ? readerPosition() : restorePosition;
  readMode = next;
  applyReadMode();
  if (readerBookId) {
    restoreReaderPositionSoon(pos);
    scheduleSaveProgress();
  }
}

function cycleReadMode() {
  const order = ["pages-h", "pages-v", "scroll"];
  const i = order.indexOf(readMode);
  setReadMode(order[(i + 1) % order.length]);
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
  currentBookId = id;
  hideNotePop();
  hideTocPop();
  show("reader");
  applyReadMode();
  history.pushState({ read: id }, "", "/?read=" + id);

  const res = await api("/api/book/" + id + "/read");
  if (!res.ok) {
    alert("Не удалось открыть книгу");
    document.documentElement.classList.remove("boot-reader");
    if (currentBookId) {
      openBook(currentBookId);
    } else {
      show("home");
    }
    return;
  }
  const data = await res.json();
  readerBookId = id;
  restorePosition = data.position || 0;
  lastGoodReaderPos = restorePosition;
  lastKnownContentH = 0;
  pinnedReaderPos = null;
  $("reader-title").textContent = data.title || "";
  $("reader-content").innerHTML = data.html || "";
  readerChapters = Array.isArray(data.chapters) ? data.chapters : [];
  hideTocPop();
  renderTocList();
  updateTocBar();
  applyReaderFont({ keepPosition: false });
  applyFontScale({ keepPosition: false });

  applyReadMode();
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      restoreReaderPosition(restorePosition);
      updateTocBar();
    });
  });
}

function fullscreenElement() {
  return document.fullscreenElement || document.webkitFullscreenElement || null;
}

function fullscreenSupported() {
  const el = document.documentElement;
  return !!(el.requestFullscreen || el.webkitRequestFullscreen);
}

function exitReaderFullscreen() {
  const exit = document.exitFullscreen || document.webkitExitFullscreen;
  if (fullscreenElement() && exit) {
    return Promise.resolve(exit.call(document)).catch(() => {});
  }
  return Promise.resolve();
}

function syncFullscreenButton() {
  const btn = $("reader-fs-btn");
  if (!btn) return;
  if (!fullscreenSupported()) {
    btn.classList.add("hidden");
    return;
  }
  btn.classList.remove("hidden");
  const on = !!fullscreenElement();
  btn.textContent = on ? "↙" : "⛶";
  btn.title = on ? "Свернуть экран" : "На весь экран";
  btn.setAttribute("aria-label", btn.title);
  btn.setAttribute("aria-pressed", on ? "true" : "false");
}

function preserveReaderPlaceAfterResize() {
  if (!readerBookId) return;
  const remembered = pinnedReaderPos ?? lastGoodReaderPos;
  const raw = readReaderPosition();
  const pos =
    raw == null || (raw < 0.001 && remembered > 0.001) ? remembered : raw;
  restoreReaderPositionSoon(pos);
}

async function toggleReaderFullscreen() {
  if (!fullscreenSupported()) return;
  pinReaderPlace();
  try {
    if (fullscreenElement()) {
      await exitReaderFullscreen();
    } else {
      const el = document.documentElement;
      const req = el.requestFullscreen || el.webkitRequestFullscreen;
      await req.call(el);
    }
  } catch {
    /* user denied or browser blocked */
  }
  syncFullscreenButton();
}

function closeReader() {
  saveReaderProgress();
  exitReaderFullscreen();
  readerBookId = null;
  readerPageIndex = 0;
  readerPageOffsets = [0];
  lastGoodReaderPos = 0;
  lastKnownContentH = 0;
  pinnedReaderPos = null;
  pageFlipBusy = false;
  if (pageFlipTimer) {
    clearTimeout(pageFlipTimer);
    pageFlipTimer = 0;
  }
  endPageTouch();
  hideNotePop();
  hideTocPop();
  readerChapters = [];
  updateTocBar();
  document.body.classList.remove(
    "reader-pages",
    "reader-pages-h",
    "reader-pages-v",
    "reader-chrome-hidden"
  );
  lockPageScroll(false);
  clearPageTransform(readerContentEl());
  clearPageLayoutStyles(readerContentEl());
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
    btn.className = "letter-btn" + (!catalogQuery && l.letter === catalogLetter ? " is-active" : "");
    btn.textContent = l.letter;
    btn.title = formatNum(l.count);
    btn.addEventListener("click", () => {
      catalogQuery = "";
      catalogFilter.value = "";
      openCatalog(catalogTab, l.letter);
    });
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
      listReturn = { type: "catalog", tab: kind, letter: catalogLetter || "" };
      if (kind === "authors") openAuthor(it.id);
      else if (kind === "series") openSeries(it.id);
      else if (kind === "genres") openGenre(it.code);
    });
    list.appendChild(row);
  }
}

async function openGenre(code, page, sort) {
  page = Math.max(1, page || 1);
  if (sort) genreSort = normalizeGenreSort(sort);
  const offset = (page - 1) * PAGE_SIZE;
  const res = await api(
    "/api/catalog/genres/" + encodeURIComponent(code) +
      "?limit=" + PAGE_SIZE + "&offset=" + offset + "&sort=" + encodeURIComponent(genreSort)
  );
  if (!res.ok) {
    alert("Жанр недоступен");
    return;
  }
  const data = await res.json();
  data.id = code;
  data.sort = data.sort || genreSort;
  const pages = pageCount(data.total, PAGE_SIZE);
  if (page > pages && data.total > 0) {
    return openGenre(code, pages, genreSort);
  }
  history.pushState({ genre: code, p: page, sort: genreSort }, "", resultsURLFrom("genre", code, page, genreSort));
  renderNamedList("genre", data, page);
  window.scrollTo(0, 0);
}

function setCatalogLoading(on) {
  const loading = $("catalog-loading");
  loading.classList.toggle("hidden", !on);
  loading.setAttribute("aria-busy", on ? "true" : "false");
  if (on) $("catalog-empty").classList.add("hidden");
}

function catalogFilterPlaceholder() {
  if (catalogTab === "series") return "Начать вводить название серии…";
  if (catalogTab === "genres") return "Фильтр жанров…";
  return "Начать вводить фамилию или имя…";
}

function syncCatalogFilterUI() {
  catalogFilter.placeholder = catalogFilterPlaceholder();
  catalogFilter.value = catalogQuery;
  $("catalog-letters").classList.toggle("is-filtered", !!catalogQuery && catalogTab !== "genres");
}

function renderFilteredGenres(query) {
  const q = (query || "").trim().toLowerCase();
  const all = catalogGenresCache || [];
  const items = !q
    ? all
    : all.filter((g) => (g.name || "").toLowerCase().includes(q) || (g.code || "").toLowerCase().includes(q));
  const emptyEl = $("catalog-empty");
  emptyEl.classList.toggle("hidden", items.length > 0);
  emptyEl.textContent = items.length ? "Ничего нет" : "Ничего не нашлось";
  renderCatalogRows(items, "genres");
}

async function openCatalog(tab, letter) {
  const tabChanged = !!(tab && tab !== catalogTab);
  catalogTab = tab || catalogTab || "authors";
  if (letter) {
    catalogLetter = letter;
  } else if (tabChanged) {
    catalogLetter = "";
  }
  const seq = ++catalogLoadSeq;
  document.querySelectorAll("#catalog-tabs .shelf-pill").forEach((btn) => {
    btn.classList.toggle("is-active", btn.getAttribute("data-cat") === catalogTab);
  });
  syncCatalogFilterUI();

  const emptyEl = $("catalog-empty");
  emptyEl.textContent = "Ничего нет";
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
      catalogGenresCache = (data.genres || []).map((g) => ({
        code: g.code,
        name: g.name || g.code,
        books: g.books,
      }));
      renderFilteredGenres(catalogQuery);
      return;
    }

    let url = "/api/catalog/" + catalogTab + "?limit=150";
    if (catalogQuery) {
      url = "/api/catalog/" + catalogTab + "?q=" + encodeURIComponent(catalogQuery) + "&limit=50";
    } else if (catalogLetter) {
      url += "&letter=" + encodeURIComponent(catalogLetter);
    }
    const res = await api(url);
    if (seq !== catalogLoadSeq) return;
    if (!res.ok) {
      alert("Не удалось загрузить каталог");
      return;
    }
    const data = await res.json();
    if (seq !== catalogLoadSeq) return;
    const letters = data.letters || [];
    if (!catalogQuery) {
      catalogLetter = data.letter || catalogLetter || (letters[0] && letters[0].letter) || "";
    }
    const qs = new URLSearchParams();
    qs.set("catalog", catalogTab);
    if (catalogQuery) qs.set("cq", catalogQuery);
    else if (catalogLetter) qs.set("letter", catalogLetter);
    history.pushState({ catalog: catalogTab, letter: catalogLetter, cq: catalogQuery }, "", "/?" + qs.toString());

    renderCatalogLetters(letters);
    syncCatalogFilterUI();
    $("catalog-list").innerHTML = "";
    const rows =
      catalogTab === "authors"
        ? data.authors || []
        : (data.series || []).map((s) => ({
            id: s.id,
            name: s.title,
            books: s.books,
          }));
    emptyEl.classList.toggle("hidden", rows.length > 0);
    emptyEl.textContent = rows.length ? "Ничего нет" : "Ничего не нашлось";
    renderCatalogRows(rows, catalogTab);
  } finally {
    if (seq === catalogLoadSeq) setCatalogLoading(false);
  }
}

function scheduleCatalogFilter() {
  clearTimeout(catalogFilterTimer);
  catalogFilterTimer = setTimeout(() => {
    catalogQuery = (catalogFilter.value || "").trim();
    if (catalogTab === "genres") {
      syncCatalogFilterUI();
      if (catalogGenresCache) {
        renderFilteredGenres(catalogQuery);
        return;
      }
    }
    openCatalog(catalogTab);
  }, 280);
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
  listContext = { kind: "lists", status: shelfTab };
  renderBookGrid(books, $("lists-grid"), $("lists-empty"));
  show("lists");
}

function goBackFromBook() {
  if (listContext?.kind === "lists") {
    openLists(listContext.status || shelfTab);
    return;
  }
  if (listContext) {
    const { kind, id, name, series } = listContext;
    const page = resultsPager && resultsPager.mode === kind ? resultsPager.page : 1;
    const total = resultsPager ? resultsPager.total : lastBooks.length;
    history.replaceState({ [kind]: id, p: page, sort: genreSort }, "", resultsURLFrom(kind, id, page, genreSort));
    renderNamedList(kind, { id, name, books: lastBooks, total, series, sort: genreSort }, page);
    return;
  }
  if (searchHasFilters(lastSearch)) {
    const page = resultsPager && resultsPager.mode === "search" ? resultsPager.page : 1;
    doSearch(lastSearch, page);
  } else {
    listReturn = null;
    resultsPager = null;
    syncResultsPager();
    history.replaceState(null, "", "/");
    show("home");
    loadContinue();
  }
}

function goBackFromNamedList() {
  const ret = listReturn;
  listReturn = null;
  if (ret?.type === "book") {
    openBook(ret.id);
    return;
  }
  if (ret?.type === "author") {
    openAuthor(ret.id);
    return;
  }
  if (ret?.type === "search" || searchHasFilters(lastSearch)) {
    const page = resultsPager && resultsPager.mode === "search" ? resultsPager.page : 1;
    doSearch(lastSearch, page);
    return;
  }
  if (ret?.type === "catalog") {
    openCatalog(ret.tab, ret.letter || "");
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
}

form.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(readSearchFromForm(), 1);
});

advForm.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(readSearchFromForm(), 1);
});

advToggle.addEventListener("click", () => {
  const open = advPanel.classList.contains("hidden");
  setAdvOpen(open);
  if (open) advAuthor.focus();
});

$("adv-search-clear").addEventListener("click", () => {
  writeSearchToForm(emptySearch());
  qInput.focus();
});

$("pager-prev").addEventListener("click", () => goResultsPage(-1));
$("pager-next").addEventListener("click", () => goResultsPage(1));

document.querySelectorAll("#list-sort [data-sort]").forEach((btn) => {
  btn.addEventListener("click", () => {
    if (listContext?.kind !== "genre") return;
    const next = normalizeGenreSort(btn.getAttribute("data-sort"));
    if (next === genreSort) return;
    openGenre(listContext.id, 1, next);
  });
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
  goBackFromNamedList();
});

window.addEventListener("popstate", () => {
  bootFromURL();
});

window.addEventListener("scroll", () => {
  if (!document.body.classList.contains("reading-mode")) return;
  if (pageModeActive()) {
    // Kill any native window scroll that sneaks through on desktop trackpads.
    if (window.scrollY || window.scrollX) window.scrollTo(0, 0);
  }
}, { passive: true });

readerViewportEl()?.addEventListener("scroll", () => {
  if (!readerBookId || pageModeActive()) return;
  scheduleSaveProgress();
  scheduleTocUpdate();
}, { passive: true });

// Trackpads fire many tiny wheel events — coalesce into one page flip.
let wheelAcc = 0;
let wheelLocked = false;
let wheelResetTimer = 0;
window.addEventListener("wheel", (e) => {
  if (!pageModeActive()) return;
  if (notePopOpen() || tocPopOpen()) return;
  e.preventDefault();
  if (wheelLocked) return;
  // Prefer horizontal delta; fall back to vertical (mouse wheel).
  const delta = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
  wheelAcc += delta;
  if (wheelResetTimer) clearTimeout(wheelResetTimer);
  wheelResetTimer = setTimeout(() => { wheelAcc = 0; }, 180);
  if (Math.abs(wheelAcc) < 48) return;
  const dir = wheelAcc > 0 ? 1 : -1;
  wheelAcc = 0;
  wheelLocked = true;
  flipReaderPage(dir);
  setTimeout(() => { wheelLocked = false; }, PAGE_SLIDE_TAP_MS + 40);
}, { passive: false, capture: true });

/** @type {null | { x: number, y: number, t: number, axis: ""|"h"|"v", sliding: boolean, lastX: number, lastY: number, lastT: number }} */
let pageTouch = null;

function eventElement(target) {
  if (!target) return null;
  return target.nodeType === 1 ? target : target.parentElement;
}

function touchOnReaderChrome(target) {
  const el = eventElement(target);
  return !!(
    el &&
    el.closest &&
    el.closest(".reader-bar, .reader-page-nav, .reader-toc-bar, .note-pop, a.fb2-note-ref, a.fb2-ref, a.fb2-ext")
  );
}

// Block native document pan in page mode (Android rubber-band / scroll steal).
document.addEventListener("touchmove", (e) => {
  if (!pageModeActive()) return;
  if (touchOnReaderChrome(e.target)) return;
  e.preventDefault();
}, { passive: false, capture: true });

function endPageTouch() {
  pageTouch = null;
}

function pageTouchPrimary(e) {
  return e.changedTouches?.[0] || e.touches?.[0] || null;
}

readerEl.addEventListener("touchstart", (e) => {
  if (!pageModeActive() || pageFlipBusy) return;
  if (touchOnReaderChrome(e.target)) {
    endPageTouch();
    return;
  }
  const t = pageTouchPrimary(e);
  if (!t) return;
  pageTouch = {
    x: t.clientX,
    y: t.clientY,
    t: performance.now(),
    axis: "",
    sliding: false,
    lastX: t.clientX,
    lastY: t.clientY,
    lastT: performance.now(),
  };
}, { passive: true, capture: true });

readerEl.addEventListener("touchmove", (e) => {
  if (!pageModeActive() || !pageTouch || pageFlipBusy) return;
  if (touchOnReaderChrome(e.target)) return;
  const t = pageTouchPrimary(e);
  if (!t) return;
  const dx = t.clientX - pageTouch.x;
  const dy = t.clientY - pageTouch.y;
  if (!pageTouch.axis) {
    if (Math.abs(dx) < 10 && Math.abs(dy) < 10) return;
    if (pageFlipHorizontal()) {
      pageTouch.axis = Math.abs(dx) >= Math.abs(dy) ? "h" : "v";
    } else {
      pageTouch.axis = Math.abs(dy) >= Math.abs(dx) ? "v" : "h";
    }
  }
  // Finger-follow only on the page-flip axis (Yandex-style drag).
  const follow = pageFlipHorizontal() ? pageTouch.axis === "h" : pageTouch.axis === "v";
  if (!follow) return;
  e.preventDefault();
  pageTouch.sliding = true;
  pageTouch.lastX = t.clientX;
  pageTouch.lastY = t.clientY;
  pageTouch.lastT = performance.now();
  const raw = pageFlipHorizontal() ? dx : dy;
  applyPageDragSlide(resistPageDrag(raw));
}, { passive: false, capture: true });

readerEl.addEventListener("touchend", (e) => {
  if (!pageModeActive()) return;
  if (touchOnReaderChrome(e.target)) {
    endPageTouch();
    return;
  }
  const t = pageTouchPrimary(e);
  const touch = pageTouch;
  endPageTouch();
  if (!t || !touch || pageFlipBusy) return;

  const dx = t.clientX - touch.x;
  const dy = t.clientY - touch.y;
  const dt = Math.max(1, performance.now() - touch.t);
  const span = pageFlipHorizontal() ? pageViewportBox().w : pageViewportBox().h;
  const slide = pageFlipHorizontal() ? dx : dy;
  const vel = slide / dt; // px/ms

  // Tap: side zones turn the page; center toggles chrome.
  if (!touch.sliding && Math.abs(dx) < 28 && Math.abs(dy) < 28) {
    const xRatio = t.clientX / Math.max(1, window.innerWidth);
    if (xRatio >= 0.72) {
      flipReaderPage(1, 0);
      return;
    }
    if (xRatio <= 0.28) {
      flipReaderPage(-1, 0);
      return;
    }
    const pos = readerPosition();
    document.body.classList.toggle("reader-chrome-hidden");
    restoreReaderPositionSoon(pos);
    return;
  }

  const follow = pageFlipHorizontal() ? touch.axis === "h" : touch.axis === "v";
  if (!follow) return;

  const commit =
    Math.abs(slide) > span * 0.18 ||
    (Math.abs(slide) > 36 && Math.abs(vel) > 0.45);
  if (!commit) {
    animatePageSlideBack(resistPageDrag(slide));
    return;
  }
  // Drag left / up → next page.
  const dir = slide < 0 ? 1 : -1;
  flipReaderPage(dir, resistPageDrag(slide));
}, { passive: false, capture: true });

readerEl.addEventListener("touchcancel", () => {
  if (pageTouch?.sliding) animatePageSlideBack(0);
  endPageTouch();
}, { passive: true, capture: true });

$("reader-back").addEventListener("click", closeReader);
$("reader-content").addEventListener("click", onReaderLinkClick);
$("note-pop-close")?.addEventListener("click", hideNotePop);
$("note-pop-dismiss")?.addEventListener("click", hideNotePop);
$("reader-toc-btn")?.addEventListener("click", showTocPop);
$("reader-toc-now")?.addEventListener("click", showTocPop);
$("toc-pop-close")?.addEventListener("click", hideTocPop);
$("toc-pop-dismiss")?.addEventListener("click", hideTocPop);
document.querySelector(".reader-bar")?.addEventListener("pointerdown", () => {
  pinReaderPlace();
}, { capture: true });
$("reader-toc-bar")?.addEventListener("pointerdown", () => {
  pinReaderPlace();
}, { capture: true });
$("reader-mode-btn").addEventListener("click", () => {
  cycleReadMode();
});
$("reader-align-btn").addEventListener("click", () => {
  setTextAlign(textAlign === "justify" ? "left" : "justify");
});
$("reader-page-prev").addEventListener("click", () => flipReaderPage(-1));
$("reader-page-next").addEventListener("click", () => flipReaderPage(1));
$("reader-face-btn").addEventListener("click", () => {
  cycleReaderFont();
});
$("reader-font-up").addEventListener("click", () => {
  fontScale = Math.min(1.6, Math.round((fontScale + 0.1) * 10) / 10);
  applyFontScale();
});
$("reader-font-down").addEventListener("click", () => {
  fontScale = Math.max(0.85, Math.round((fontScale - 0.1) * 10) / 10);
  applyFontScale();
});
$("reader-fs-btn").addEventListener("click", () => {
  toggleReaderFullscreen();
});
function onFullscreenChange() {
  syncFullscreenButton();
  preserveReaderPlaceAfterResize();
}
document.addEventListener("fullscreenchange", onFullscreenChange);
document.addEventListener("webkitfullscreenchange", onFullscreenChange);
syncFullscreenButton();

window.addEventListener("keydown", (e) => {
  if (notePopOpen() || tocPopOpen()) {
    if (e.key === "Escape") {
      e.preventDefault();
      hideNotePop();
      hideTocPop();
    }
    return;
  }
  if (!pageModeActive()) return;
  if (e.key === "ArrowDown" || e.key === "ArrowRight" || e.key === "PageDown" || e.key === " ") {
    e.preventDefault();
    flipReaderPage(1);
  } else if (e.key === "ArrowUp" || e.key === "ArrowLeft" || e.key === "PageUp") {
    e.preventDefault();
    flipReaderPage(-1);
  }
});

let readerViewportTimer = null;
let lastPageViewportH = 0;

function onReaderViewportChange() {
  if (!pageModeActive() || !readerBookId) return;
  clearTimeout(readerViewportTimer);
  readerViewportTimer = setTimeout(() => {
    const h = pageViewportHeight();
    if (lastPageViewportH > 0 && Math.abs(h - lastPageViewportH) < 3) return;
    lastPageViewportH = h;
    preserveReaderPlaceAfterResize();
  }, 80);
}

window.addEventListener("resize", onReaderViewportChange);
if (window.visualViewport) {
  window.visualViewport.addEventListener("resize", onReaderViewportChange);
  window.visualViewport.addEventListener("scroll", onReaderViewportChange);
}

async function bootFromURL() {
  const params = new URLSearchParams(location.search);
  const read = params.get("read");
  const book = params.get("book");
  const author = params.get("author");
  const series = params.get("series");
  const genre = params.get("genre");
  const catalog = params.get("catalog");
  const letter = params.get("letter") || "";
  const cq = (params.get("cq") || "").trim();
  const lists = params.get("lists");
  const search = searchFromURLParams(params);
  const page = Math.max(1, parseInt(params.get("p") || "1", 10) || 1);
  writeSearchToForm(search);
  if (searchIsAdvanced(search)) setAdvOpen(true);

  if (read) {
    await openReader(read);
    return;
  }
  if (lists && currentUser && !book && !read) {
    await openLists(lists);
    return;
  }
  if (catalog) {
    catalogQuery = cq;
    catalogFilter.value = cq;
    await openCatalog(catalog, letter);
    return;
  }
  if (genre && !book) {
    await openGenre(genre, page, params.get("sort"));
    return;
  }
  if (author && !book) {
    await openAuthor(author, page);
    return;
  }
  if (series && !book) {
    await openSeries(series, page);
    return;
  }
  if (book) {
    if (lists) listContext = { kind: "lists", status: lists };
    if (author) listContext = { kind: "author", id: Number(author) };
    if (series) listContext = { kind: "series", id: Number(series) };
    if (!lists && searchHasFilters(search) && lastBooks.length === 0) {
      await doSearch(search, page);
    }
    await openBook(book);
    return;
  }
  if (searchHasFilters(search)) {
    await doSearch(search, page);
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
    tr.innerHTML = `<td></td><td></td><td class="users-seen"></td><td class="actions"></td>`;
    tr.children[0].textContent = u.username;
    tr.children[1].textContent = roleLabel(u.role);
    tr.children[2].textContent = formatLastSeen(u.lastSeenAt);
    if (u.lastSeenAt) {
      try {
        tr.children[2].title = new Date(u.lastSeenAt).toLocaleString("ru-RU");
      } catch {
        tr.children[2].title = u.lastSeenAt;
      }
    }
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
      tr.children[3].appendChild(del);
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
  listReturn = null;
  lastSearch = emptySearch();
  writeSearchToForm(lastSearch);
  setAdvOpen(false);
  resultsPager = null;
  syncResultsPager();
  show("home");
  loadContinue();
});

$("nav-catalog").addEventListener("click", () => openCatalog(catalogTab, catalogLetter));

document.querySelectorAll("#catalog-tabs .shelf-pill").forEach((btn) => {
  btn.addEventListener("click", () => {
    catalogQuery = "";
    catalogFilter.value = "";
    openCatalog(btn.getAttribute("data-cat"), "");
  });
});

catalogFilter.addEventListener("input", scheduleCatalogFilter);
catalogFilter.addEventListener("search", scheduleCatalogFilter);

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
  const params = new URLSearchParams(location.search);
  const readId = params.get("read");
  // Switch to reader chrome before network calls so refresh never flashes home.
  if (readId) show("reader");
  if (!(await loadSession())) return;
  if (!readId) await loadStats();
  applyReaderFont({ keepPosition: false });
  applyFontScale({ keepPosition: false });
  applyTextAlign();
  applyReadMode();
  if (params.get("users") === "1" && currentUser?.role === "admin") {
    await loadUsers();
    return;
  }
  await bootFromURL();
})();
