/* Spacemolt KB site search.
 *
 * The whole site is static HTML on GitHub Pages, so search is a prebuilt index
 * (kb/search-index.json, ~1.4 MB raw / ~200 KB gzipped for 15,649 pages) filtered
 * in the browser. The index is fetched lazily on first focus, so no page pays
 * for it until someone actually searches.
 *
 * The index is title-only. That finds a page when you roughly know its name; it
 * cannot find a ship by a module it mounts. Adding per-page keywords later is a
 * change to the index builder and the score() weights, not to this file's shape.
 *
 * kbnav.Header stamps data-root on the input because pages live at varying
 * depths (kb/index.html, kb/items/index.html, kb/ships/Combat/x.html) and every
 * URL here — the index fetch and each result link — has to be relative to it.
 */
(function () {
  "use strict";

  var MAX_RESULTS = 30;      // dropdown only; the results page shows everything
  var DEBOUNCE_MS = 120;
  var MIN_QUERY = 2;

  var input, box, root, page;
  var index = null;          // [{path, title, section, catalog}] once loaded
  var loading = null;        // in-flight fetch, so double-focus does not refetch
  var rows = [], active = -1, timer = null;

  function norm(s) { return s.toLowerCase().replace(/[‐-―]/g, "-"); }

  /* Flatten the section-keyed index into one array, tagging catalog membership
     so ranking can float reference pages above bulk records. */
  function flatten(doc) {
    var catalog = {}, out = [];
    (doc.catalog || []).forEach(function (s) { catalog[s] = true; });
    Object.keys(doc.s || {}).forEach(function (section) {
      doc.s[section].forEach(function (e) {
        out.push({
          path: e[0], title: e[1], lc: norm(e[1]),
          section: section || "", catalog: !!catalog[section]
        });
      });
    });
    return out;
  }

  function load() {
    if (index) return Promise.resolve(index);
    if (loading) return loading;
    loading = fetch(root + "search-index.json")
      .then(function (r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (doc) { index = flatten(doc); return index; })
      .catch(function (err) {
        loading = null;
        throw err;
      });
    return loading;
  }

  /* Lower is better. Match quality dominates; catalog membership breaks ties
     between equally good matches, and a shorter title wins after that so
     "Railgun II" outranks "Railgun II Blueprint Fragment". */
  function score(entry, q) {
    var i = entry.lc.indexOf(q);
    if (i < 0) return -1;
    var rank;
    if (i === 0) rank = 0;                                  // title starts with it
    else if (/[\s\-_(/]/.test(entry.lc.charAt(i - 1))) rank = 1;  // a word starts with it
    else rank = 2;                                          // somewhere inside a word
    return rank * 1000 + (entry.catalog ? 0 : 400) + Math.min(entry.title.length, 90);
  }

  function search(q, limit) {
    q = norm(q).trim();
    if (q.length < MIN_QUERY || !index) return [];
    var hits = [];
    for (var i = 0; i < index.length; i++) {
      var s = score(index[i], q);
      if (s >= 0) hits.push([s, index[i]]);
    }
    hits.sort(function (a, b) { return a[0] - b[0] || (a[1].title < b[1].title ? -1 : 1); });
    if (limit) hits = hits.slice(0, limit);
    return hits.map(function (h) { return h[1]; });
  }

  function esc(s) {
    return String(s).replace(/[&<>"]/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c];
    });
  }

  /* Bold the matched run so it is obvious why a row came back. */
  function mark(title, q) {
    var i = norm(title).indexOf(norm(q));
    if (i < 0) return esc(title);
    return esc(title.slice(0, i)) + "<b>" + esc(title.slice(i, i + q.length)) +
      "</b>" + esc(title.slice(i + q.length));
  }

  function label(section) { return section ? section.replace(/-/g, " ") : "site"; }

  function render(q) {
    rows = search(q, MAX_RESULTS + 1);
    var more = rows.length > MAX_RESULTS;
    if (more) rows = rows.slice(0, MAX_RESULTS);
    active = -1;
    if (!rows.length) {
      box.innerHTML = norm(q).trim().length < MIN_QUERY ? "" :
        '<div class="kbs-empty">No page titled like that</div>';
      box.hidden = !box.innerHTML;
      return;
    }
    var html = rows.map(function (e, i) {
      return '<a class="kbs-row" role="option" id="kbs-o' + i + '" href="' +
        esc(root + e.path) + '"><span class="kbs-t">' + mark(e.title, q) +
        '</span><span class="kbs-s">' + esc(label(e.section)) + "</span></a>";
    }).join("");
    if (more) {
      html += '<a class="kbs-row kbs-all" href="' + esc(root + "search.html?q=" +
        encodeURIComponent(q)) + '">Show all results for &ldquo;' + esc(q) + "&rdquo;</a>";
    }
    box.innerHTML = html;
    box.hidden = false;
  }

  function move(delta) {
    var items = box.querySelectorAll(".kbs-row");
    if (!items.length) return;
    if (active >= 0) items[active].classList.remove("on");
    active = (active + delta + items.length) % items.length;
    items[active].classList.add("on");
    items[active].scrollIntoView({ block: "nearest" });
    input.setAttribute("aria-activedescendant", items[active].id || "");
  }

  function close() { box.hidden = true; active = -1; }

  function onInput() {
    clearTimeout(timer);
    var q = input.value;
    timer = setTimeout(function () {
      load().then(function () { render(q); })
        .catch(function () {
          box.innerHTML = '<div class="kbs-empty">Search index unavailable</div>';
          box.hidden = false;
        });
    }, DEBOUNCE_MS);
  }

  function wire() {
    input.addEventListener("focus", function () { load().catch(function () {}); });
    input.addEventListener("input", onInput);
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown") { e.preventDefault(); move(1); }
      else if (e.key === "ArrowUp") { e.preventDefault(); move(-1); }
      else if (e.key === "Escape") { close(); input.blur(); }
      else if (e.key === "Enter") {
        var items = box.querySelectorAll(".kbs-row");
        if (active >= 0 && items[active]) { e.preventDefault(); items[active].click(); }
        else if (input.value.trim()) {
          e.preventDefault();
          location.href = root + "search.html?q=" + encodeURIComponent(input.value.trim());
        }
      }
    });
    document.addEventListener("click", function (e) {
      if (!box.contains(e.target) && e.target !== input) close();
    });
    // "/" focuses search from anywhere, the way most doc sites behave.
    document.addEventListener("keydown", function (e) {
      if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey) return;
      var t = e.target.tagName;
      if (t === "INPUT" || t === "TEXTAREA" || t === "SELECT" || e.target.isContentEditable) return;
      e.preventDefault();
      input.focus();
      input.select();
    });
  }

  /* The dedicated results page: same index, same ranking, no result cap. */
  function renderPage() {
    var q = new URLSearchParams(location.search).get("q") || "";
    var head = document.getElementById("kbs-page-q");
    var list = document.getElementById("kbs-page-results");
    if (input) input.value = q;
    if (!q.trim()) { head.textContent = "Type a query to search the knowledge base."; return; }
    head.textContent = "Searching…";
    load().then(function () {
      var hits = search(q, 0);
      head.textContent = hits.length
        ? hits.length.toLocaleString() + " page" + (hits.length === 1 ? "" : "s") +
          " matching “" + q + "”"
        : "Nothing titled like “" + q + "”";
      list.innerHTML = hits.map(function (e) {
        return '<a class="kbs-hit" href="' + esc(root + e.path) + '"><span class="kbs-t">' +
          mark(e.title, q) + '</span><span class="kbs-s">' + esc(label(e.section)) + "</span></a>";
      }).join("");
    }).catch(function () { head.textContent = "Search index unavailable."; });
  }

  function init() {
    input = document.getElementById("kb-search");
    box = document.getElementById("kb-search-results");
    page = document.getElementById("kbs-page-results");
    // data-root is on whichever element the generator emitted.
    var host = input || document.querySelector("[data-kb-root]");
    root = (host && host.getAttribute("data-kb-root")) || "";
    if (input && box) wire();
    if (page) renderPage();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
