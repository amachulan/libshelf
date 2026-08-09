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

const PAGE_SIZE = 60;

let lastQuery = "";
let lastBooks = [];
let listContext = null; // { kind: 'author'|'series'|'genre', id, name }
/** @type {{ mode: 'search'|'author'|'series'|'genre', key: string|number, page: number, total: number, limit: number } | null} */
let resultsPager = null;
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
let textAlign = localStorage.getItem("libshelf_align") === "left" ? "left" : "justify";
let readerPageIndex = 0;
/** Y offsets (px into #reader-content) where each page starts — snapped to line bottoms. */
let readerPageOffsets = [0];

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
    if (b.editionCount > 1) {
      const badge = document.createElement("span");
      badge.className = "edition-badge";
      badge.textContent = `${b.editionCount} ${editionWord(b.editionCount)}`;
      el.appendChild(badge);
    }
    const img = el.querySelector("img");
    img.onerror = () => { img.src = placeholderCover(); };
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

function renderResults(query, books, total, page) {
  listContext = null;
  lastQuery = query;
  lastBooks = books || [];
  resultsPager = {
    mode: "search",
    key: query,
    page: page || 1,
    total: total || lastBooks.length,
    limit: PAGE_SIZE,
  };
  resultsBack.classList.add("hidden");
  resultsSub.classList.remove("hidden");
  $("results-title").textContent = query ? `Поиск: «${query}»` : "Результаты";
  resultsSub.textContent = total ? worksLabel(total) : "";
  if (!total) resultsSub.classList.add("hidden");
  renderBookGrid(lastBooks);
  syncResultsPager();
  show("results");
}

function renderNamedList(kind, data, page) {
  listContext = { kind, id: data.id, name: data.name };
  lastBooks = data.books || [];
  lastQuery = "";
  resultsPager = {
    mode: kind,
    key: kind === "genre" ? data.id : data.id,
    page: page || 1,
    total: data.total || lastBooks.length,
    limit: PAGE_SIZE,
  };
  resultsBack.classList.remove("hidden");
  resultsSub.classList.remove("hidden");
  const label = kind === "author" ? "Автор" : kind === "series" ? "Серия" : "Жанр";
  $("results-title").textContent = `${label}: ${data.name}`;
  resultsSub.textContent = worksLabel(data.total || lastBooks.length);
  renderBookGrid(lastBooks);
  syncResultsPager();
  show("results");
}

async function doSearch(query, page) {
  query = (query || "").trim();
  page = Math.max(1, page || 1);
  if (!query) {
    listContext = null;
    resultsPager = null;
    syncResultsPager();
    show("home");
    history.replaceState(null, "", "/");
    loadContinue();
    return;
  }
  const offset = (page - 1) * PAGE_SIZE;
  const url =
    "/api/search?q=" +
    encodeURIComponent(query) +
    "&limit=" +
    PAGE_SIZE +
    "&offset=" +
    offset;
  history.replaceState(null, "", "/?q=" + encodeURIComponent(query) + (page > 1 ? "&p=" + page : ""));
  const res = await api(url);
  if (!res.ok) {
    alert("Ошибка поиска: " + (await res.text()));
    return;
  }
  const data = await res.json();
  const total = data.total || 0;
  const pages = pageCount(total, PAGE_SIZE);
  if (page > pages && total > 0) {
    return doSearch(query, pages);
  }
  renderResults(query, data.books || [], total, page);
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

function resultsURLFrom(mode, key, page) {
  const p = page > 1 ? "&p=" + page : "";
  if (mode === "search") return "/?q=" + encodeURIComponent(key) + p;
  if (mode === "author") return "/?author=" + key + p;
  if (mode === "series") return "/?series=" + key + p;
  if (mode === "genre") return "/?genre=" + encodeURIComponent(key) + p;
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
  if (mode === "genre") return openGenre(key, page);
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
  if (pos != null) {
    requestAnimationFrame(() => restoreReaderPosition(pos));
  }
}

function readerContentEl() {
  return $("reader-content");
}

function readerViewportEl() {
  return document.querySelector(".reader-viewport");
}

function pageModeActive() {
  return document.body.classList.contains("reading-mode") && readMode === "pages";
}

function pageViewportHeight() {
  const vp = readerViewportEl();
  const h = vp ? vp.clientHeight : 0;
  if (h >= 80) return h;
  const bar = document.querySelector(".reader-bar");
  const barH = bar && !document.body.classList.contains("reader-chrome-hidden")
    ? bar.offsetHeight
    : 0;
  return Math.max(80, window.innerHeight - barH);
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

function rebuildReaderPages() {
  const content = readerContentEl();
  if (!content) {
    readerPageOffsets = [0];
    return;
  }

  // Measure without transform/clip so line boxes are stable.
  content.style.transition = "none";
  content.style.transform = "none";
  content.style.clipPath = "none";

  const H = pageViewportHeight();
  const total = content.scrollHeight;
  if (H < 80 || total <= H + 1) {
    readerPageOffsets = [0];
    return;
  }

  const bottoms = collectLineBottoms(content);
  const offsets = [0];
  let pageTop = 0;
  const eps = 1;

  while (pageTop + H < total - eps) {
    const limit = pageTop + H - eps;
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
}

function maxReaderPageIndex() {
  return Math.max(0, readerPageOffsets.length - 1);
}

function clearPageTransform(el) {
  if (!el) return;
  el.style.transform = "";
  el.style.clipPath = "";
  el.style.transition = "";
}

function applyPageTransform(smooth) {
  const el = readerContentEl();
  if (!el) return;
  if (!pageModeActive()) {
    clearPageTransform(el);
    return;
  }
  const total = el.scrollHeight;
  const top = readerPageOffsets[readerPageIndex] || 0;
  const next = readerPageOffsets[readerPageIndex + 1];
  // Clip to the line-snapped page end so a sliced line is never shown at the bottom.
  const bottom = next != null ? next : total;
  el.style.transition = smooth ? "transform 0.22s ease" : "none";
  el.style.transform = `translate3d(0, ${-top}px, 0)`;
  el.style.clipPath = `inset(${Math.max(0, top)}px 0 ${Math.max(0, total - bottom)}px 0)`;
}

function lockPageScroll(on) {
  document.documentElement.classList.toggle("reader-pages-lock", !!on);
  if (on) window.scrollTo(0, 0);
}

function applyReadMode() {
  const paging = readMode === "pages" && document.body.classList.contains("reading-mode");
  // Only while reading — otherwise page-mode CSS leaks onto the catalog.
  document.body.classList.toggle("reader-pages", paging);
  if (!paging) document.body.classList.remove("reader-chrome-hidden");
  lockPageScroll(paging);
  const btn = $("reader-mode-btn");
  if (btn) {
    const pagesIco = btn.querySelector(".reader-ico-pages");
    const scrollIco = btn.querySelector(".reader-ico-scroll");
    pagesIco?.classList.toggle("hidden", readMode !== "pages");
    scrollIco?.classList.toggle("hidden", readMode === "pages");
    btn.title = readMode === "pages" ? "Сплошной текст" : "Листать страницами";
    btn.setAttribute("aria-label", btn.title);
  }
  localStorage.setItem("libshelf_read_mode", readMode);
  const el = readerContentEl();
  if (el) {
    el.style.columnWidth = "";
    if (!paging) clearPageTransform(el);
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
  const pos = readerBookId && pageModeActive() ? readerPosition() : null;
  applyTextAlign();
  if (pos != null) {
    requestAnimationFrame(() => restoreReaderPosition(pos));
  }
}

function readerPosition() {
  if (readMode === "pages") {
    const el = readerContentEl();
    const total = el ? el.scrollHeight : 0;
    const maxScroll = Math.max(0, total - pageViewportHeight());
    if (maxScroll <= 0) return 0;
    const y = readerPageOffsets[readerPageIndex] || 0;
    return Math.min(1, Math.max(0, y / maxScroll));
  }
  const el = document.documentElement;
  const max = el.scrollHeight - el.clientHeight;
  if (max <= 0) return 0;
  return Math.min(1, Math.max(0, el.scrollTop / max));
}

function restoreReaderPosition(pos) {
  const p = Math.min(1, Math.max(0, Number(pos) || 0));
  if (readMode === "pages") {
    rebuildReaderPages();
    const el = readerContentEl();
    const total = el ? el.scrollHeight : 0;
    const maxScroll = Math.max(0, total - pageViewportHeight());
    const targetY = p * maxScroll;
    let best = 0;
    for (let i = 0; i < readerPageOffsets.length; i++) {
      if (readerPageOffsets[i] <= targetY + 1) best = i;
      else break;
    }
    readerPageIndex = best;
    applyPageTransform(false);
    return;
  }
  const el = document.documentElement;
  const max = el.scrollHeight - el.clientHeight;
  el.scrollTop = max > 0 ? p * max : 0;
}

function flipReaderPage(dir) {
  if (readMode !== "pages") return;
  if (readerPageOffsets.length <= 1) rebuildReaderPages();
  const next = Math.min(maxReaderPageIndex(), Math.max(0, readerPageIndex + dir));
  if (next === readerPageIndex) return;
  readerPageIndex = next;
  applyPageTransform(true);
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
  currentBookId = id;
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
  $("reader-title").textContent = data.title || "";
  $("reader-content").innerHTML = data.html || "";
  applyFontScale({ keepPosition: false });

  applyReadMode();
  requestAnimationFrame(() => {
    requestAnimationFrame(() => restoreReaderPosition(restorePosition));
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

function preserveReaderPageAfterResize() {
  if (!pageModeActive() || !readerBookId) return;
  const y = readerPageOffsets[readerPageIndex] || 0;
  requestAnimationFrame(() => {
    rebuildReaderPages();
    let best = 0;
    for (let i = 0; i < readerPageOffsets.length; i++) {
      if (readerPageOffsets[i] <= y + 1) best = i;
      else break;
    }
    readerPageIndex = best;
    applyPageTransform(false);
  });
}

async function toggleReaderFullscreen() {
  if (!fullscreenSupported()) return;
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
  document.body.classList.remove("reader-pages", "reader-chrome-hidden");
  lockPageScroll(false);
  clearPageTransform(readerContentEl());
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

async function openGenre(code, page) {
  page = Math.max(1, page || 1);
  const offset = (page - 1) * PAGE_SIZE;
  const res = await api(
    "/api/catalog/genres/" + encodeURIComponent(code) + "?limit=" + PAGE_SIZE + "&offset=" + offset
  );
  if (!res.ok) {
    alert("Жанр недоступен");
    return;
  }
  const data = await res.json();
  data.id = code;
  const pages = pageCount(data.total, PAGE_SIZE);
  if (page > pages && data.total > 0) {
    return openGenre(code, pages);
  }
  history.pushState({ genre: code, p: page }, "", resultsURLFrom("genre", code, page));
  renderNamedList("genre", data, page);
  window.scrollTo(0, 0);
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
    const { kind, id, name } = listContext;
    const page = resultsPager && resultsPager.mode === kind ? resultsPager.page : 1;
    const total = resultsPager ? resultsPager.total : lastBooks.length;
    history.replaceState({ [kind]: id, p: page }, "", resultsURLFrom(kind, id, page));
    renderNamedList(kind, { id, name, books: lastBooks, total }, page);
    return;
  }
  if (lastQuery) {
    const page = resultsPager && resultsPager.mode === "search" ? resultsPager.page : 1;
    history.replaceState(null, "", resultsURLFrom("search", lastQuery, page));
    renderResults(lastQuery, lastBooks, resultsPager?.total, page);
  } else {
    resultsPager = null;
    syncResultsPager();
    history.replaceState(null, "", "/");
    show("home");
    loadContinue();
  }
}

form.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(qInput.value, 1);
});

$("pager-prev").addEventListener("click", () => goResultsPage(-1));
$("pager-next").addEventListener("click", () => goResultsPage(1));

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
  if (readMode === "pages") {
    // Kill any native window scroll that sneaks through on desktop trackpads.
    if (window.scrollY || window.scrollX) window.scrollTo(0, 0);
    return;
  }
  scheduleSaveProgress();
}, { passive: true });

// Trackpads fire many tiny wheel events — coalesce into one page flip.
let wheelAcc = 0;
let wheelLocked = false;
let wheelResetTimer = 0;
window.addEventListener("wheel", (e) => {
  if (!pageModeActive()) return;
  e.preventDefault();
  if (wheelLocked) return;
  wheelAcc += e.deltaY;
  if (wheelResetTimer) clearTimeout(wheelResetTimer);
  wheelResetTimer = setTimeout(() => { wheelAcc = 0; }, 180);
  if (Math.abs(wheelAcc) < 48) return;
  const dir = wheelAcc > 0 ? 1 : -1;
  wheelAcc = 0;
  wheelLocked = true;
  flipReaderPage(dir);
  setTimeout(() => { wheelLocked = false; }, 220);
}, { passive: false, capture: true });

let readerTouchStartY = 0;
let readerTouchStartX = 0;
function touchOnReaderChrome(target) {
  return !!(target && target.closest && target.closest(".reader-bar, .reader-page-nav"));
}
function pageTouchMoveBlock(e) {
  if (!pageModeActive()) return;
  if (touchOnReaderChrome(e.target)) return;
  e.preventDefault();
}
document.addEventListener("touchmove", pageTouchMoveBlock, { passive: false, capture: true });
readerEl.addEventListener("touchstart", (e) => {
  if (!pageModeActive()) return;
  readerTouchStartX = e.changedTouches[0]?.clientX || 0;
  readerTouchStartY = e.changedTouches[0]?.clientY || 0;
}, { passive: true, capture: true });
readerEl.addEventListener("touchend", (e) => {
  if (!pageModeActive()) return;
  if (touchOnReaderChrome(e.target)) return;
  const x = e.changedTouches[0]?.clientX || 0;
  const y = e.changedTouches[0]?.clientY || 0;
  const dx = x - readerTouchStartX;
  const dy = readerTouchStartY - y;
  if (Math.abs(dy) < 28 && Math.abs(dx) < 28) {
    const pos = readerPosition();
    document.body.classList.toggle("reader-chrome-hidden");
    requestAnimationFrame(() => restoreReaderPosition(pos));
    return;
  }
  if (Math.abs(dy) < 40 || Math.abs(dy) < Math.abs(dx)) return;
  flipReaderPage(dy > 0 ? 1 : -1);
}, { passive: true, capture: true });

$("reader-back").addEventListener("click", closeReader);
$("reader-mode-btn").addEventListener("click", () => {
  setReadMode(readMode === "pages" ? "scroll" : "pages");
});
$("reader-align-btn").addEventListener("click", () => {
  setTextAlign(textAlign === "justify" ? "left" : "justify");
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
$("reader-fs-btn").addEventListener("click", () => {
  toggleReaderFullscreen();
});
function onFullscreenChange() {
  syncFullscreenButton();
  preserveReaderPageAfterResize();
}
document.addEventListener("fullscreenchange", onFullscreenChange);
document.addEventListener("webkitfullscreenchange", onFullscreenChange);
syncFullscreenButton();

window.addEventListener("keydown", (e) => {
  if (!pageModeActive()) return;
  if (e.key === "ArrowDown" || e.key === "ArrowRight" || e.key === "PageDown" || e.key === " ") {
    e.preventDefault();
    flipReaderPage(1);
  } else if (e.key === "ArrowUp" || e.key === "ArrowLeft" || e.key === "PageUp") {
    e.preventDefault();
    flipReaderPage(-1);
  }
});

window.addEventListener("resize", () => {
  if (!document.body.classList.contains("reading-mode") || readMode !== "pages" || !readerBookId) return;
  const pos = readerPosition();
  requestAnimationFrame(() => restoreReaderPosition(pos));
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
  const page = Math.max(1, parseInt(params.get("p") || "1", 10) || 1);
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
    await openGenre(genre, page);
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
    if (author) listContext = { kind: "author", id: Number(author) };
    if (series) listContext = { kind: "series", id: Number(series) };
    if (q && lastBooks.length === 0) {
      await doSearch(q, page);
    }
    await openBook(book);
    return;
  }
  if (q) {
    await doSearch(q, page);
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
  const params = new URLSearchParams(location.search);
  const readId = params.get("read");
  // Switch to reader chrome before network calls so refresh never flashes home.
  if (readId) show("reader");
  if (!(await loadSession())) return;
  if (!readId) await loadStats();
  applyFontScale();
  applyTextAlign();
  applyReadMode();
  if (params.get("users") === "1" && currentUser?.role === "admin") {
    await loadUsers();
    return;
  }
  await bootFromURL();
})();
