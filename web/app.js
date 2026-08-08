const $ = (id) => document.getElementById(id);

const home = $("home");
const results = $("results");
const bookPanel = $("book");
const usersPanel = $("users");
const grid = $("grid");
const empty = $("empty");
const form = $("search-form");
const qInput = $("q");
const resultsBack = $("results-back");
const resultsSub = $("results-sub");

let lastQuery = "";
let lastBooks = [];
let listContext = null; // { kind: 'author'|'series', id, name }
let currentUser = null;

async function api(url, opts) {
  const res = await fetch(url, opts);
  if (res.status === 401) {
    location.href = "/login.html";
    throw new Error("unauthorized");
  }
  return res;
}

async function loadSession() {
  const res = await fetch("/api/me");
  if (res.status === 401) {
    location.href = "/login.html";
    return false;
  }
  if (!res.ok) return true;
  const data = await res.json();
  if (data.auth && data.user) {
    currentUser = data.user;
    $("user-box").classList.remove("hidden");
    $("user-label").textContent = `${data.user.username} · ${roleLabel(data.user.role)}`;
    $("users-btn").classList.toggle("hidden", data.user.role !== "admin");
  }
  return true;
}

function roleLabel(role) {
  return role === "admin" ? "админ" : "читатель";
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
  home.classList.toggle("hidden", panel !== "home");
  results.classList.toggle("hidden", panel !== "results");
  bookPanel.classList.toggle("hidden", panel !== "book");
  usersPanel.classList.toggle("hidden", panel !== "users");
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

async function openBook(id) {
  const res = await api("/api/book/" + id);
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

async function loadUsers() {
  const res = await api("/api/users");
  if (!res.ok) {
    alert("Нет доступа");
    return;
  }
  const data = await res.json();
  const body = $("users-body");
  body.innerHTML = "";
  for (const u of data.users || []) {
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
}

$("logout-btn").addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST" });
  location.href = "/login.html";
});

$("users-btn").addEventListener("click", () => {
  history.pushState({ users: true }, "", "/?users=1");
  loadUsers();
});

$("users-back").addEventListener("click", () => {
  history.replaceState(null, "", "/");
  show("home");
});

$("user-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const err = $("users-error");
  err.classList.add("hidden");
  const res = await api("/api/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      username: $("new-username").value.trim(),
      password: $("new-password").value,
      role: $("new-role").value,
    }),
  });
  if (!res.ok) {
    err.textContent = await res.text();
    err.classList.remove("hidden");
    return;
  }
  $("new-username").value = "";
  $("new-password").value = "";
  loadUsers();
});

(async function boot() {
  if (!(await loadSession())) return;
  await loadStats();
  const params = new URLSearchParams(location.search);
  if (params.get("users") === "1" && currentUser?.role === "admin") {
    await loadUsers();
    return;
  }
  await bootFromURL();
})();
