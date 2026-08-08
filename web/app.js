const $ = (id) => document.getElementById(id);

const home = $("home");
const results = $("results");
const bookPanel = $("book");
const grid = $("grid");
const empty = $("empty");
const form = $("search-form");
const qInput = $("q");
const resultsBack = $("results-back");
const resultsSub = $("results-sub");

let lastQuery = "";
let lastBooks = [];
let listContext = null; // { kind: 'author'|'series', id, name }

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

function renderBookGrid(books) {
  grid.innerHTML = "";
  empty.classList.toggle("hidden", books.length > 0);
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
    grid.appendChild(el);
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

async function openAuthor(id) {
  const res = await fetch("/api/author/" + id + "?limit=100");
  if (!res.ok) {
    alert("Автор недоступен");
    return;
  }
  const data = await res.json();
  history.pushState({ author: id }, "", "/?author=" + id);
  renderNamedList("author", data);
}

async function openSeries(id) {
  const res = await fetch("/api/series/" + id + "?limit=100");
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

async function openBook(id) {
  const res = await fetch("/api/book/" + id);
  if (!res.ok) {
    alert("Книга недоступна");
    return;
  }
  const b = await res.json();
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

  const bits = [];
  if (b.year) bits.push(String(b.year));
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
  show("book");
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
  }
  if (lastQuery) {
    history.replaceState(null, "", "/?q=" + encodeURIComponent(lastQuery));
    renderResults(lastQuery, lastBooks);
  } else {
    history.replaceState(null, "", "/");
    show("home");
  }
}

form.addEventListener("submit", (e) => {
  e.preventDefault();
  doSearch(qInput.value);
});

$("back").addEventListener("click", goBackFromBook);

resultsBack.addEventListener("click", () => {
  if (lastQuery) {
    history.replaceState(null, "", "/?q=" + encodeURIComponent(lastQuery));
    doSearch(lastQuery);
  } else {
    history.replaceState(null, "", "/");
    listContext = null;
    show("home");
  }
});

window.addEventListener("popstate", () => {
  bootFromURL();
});

async function bootFromURL() {
  const params = new URLSearchParams(location.search);
  const book = params.get("book");
  const author = params.get("author");
  const series = params.get("series");
  const q = params.get("q") || "";
  qInput.value = q;

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
}

loadStats();
bootFromURL();
