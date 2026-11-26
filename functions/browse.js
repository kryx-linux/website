// Cloudflare Pages Function: zero-JS server-side package & port filter/pagination at /browse.
// Works natively in lynx, w3m, links, curl (?format=txt), and graphical browsers.

import {
  PORT_COLLECTIONS,
  SITE_URL,
  escapeHTML,
  renderPage,
} from "./_shared.js";

let cachedCatalog = null;

async function loadCatalog(context) {
  if (cachedCatalog) return cachedCatalog;
  const assetURL = new URL("/search-index.json", context.request.url);
  const resp = await context.env.ASSETS.fetch(assetURL);
  if (!resp.ok) {
    throw new Error(`failed to load search-index.json (${resp.status})`);
  }
  const entries = await resp.json();
  const byName = new Map();

  for (const e of entries) {
    if (e.k !== "port") continue;
    if (byName.has(e.n)) continue;
    byName.set(e.n, {
      name: e.n || "",
      version: e.v || "",
      collection: e.c || "",
      description: e.d || "",
      prebuilt: false,
      url: e.u || "",
      portURL: e.u || "",
    });
  }
  for (const e of entries) {
    if (e.k !== "package") continue;
    const cur = byName.get(e.n);
    if (cur) {
      cur.prebuilt = true;
      cur.url = e.u || cur.url;
      if (e.v) cur.version = e.v;
      if (e.c && !cur.collection) cur.collection = e.c;
      if (e.d && !cur.description) cur.description = e.d;
    } else {
      byName.set(e.n, {
        name: e.n || "",
        version: e.v || "",
        collection: e.c || "",
        description: e.d || "",
        prebuilt: true,
        url: e.u || "",
        portURL: "",
      });
    }
  }
  cachedCatalog = Array.from(byName.values());
  return cachedCatalog;
}

function buildBrowseURL({ col, kind, sortBy, q, per, page, format }) {
  const sp = new URLSearchParams();
  if (col) sp.set("col", col);
  if (kind !== "all") sp.set("kind", kind);
  if (sortBy !== "name") sp.set("sort", sortBy);
  if (q) sp.set("q", q);
  if (per !== 50) sp.set("per", String(per));
  if (page > 1) sp.set("page", String(page));
  if (format) sp.set("format", format);
  const qs = sp.toString();
  return qs ? `/browse?${qs}` : "/browse";
}

export async function onRequestGet(context) {
  let catalog;
  try {
    catalog = await loadCatalog(context);
  } catch {
    return new Response("Package catalog temporarily unavailable.", {
      status: 500,
      headers: { "content-type": "text/plain; charset=utf-8" },
    });
  }

  const reqURL = new URL(context.request.url);
  const sp = reqURL.searchParams;
  const col = (sp.get("col") || "").trim();
  const rawKind = (sp.get("kind") || "").trim().toLowerCase();
  const kind =
    rawKind === "prebuilt" || rawKind === "source" ? rawKind : "all";
  const rawSort = (sp.get("sort") || "").trim().toLowerCase();
  const sortBy =
    rawSort === "col" || rawSort === "version" ? rawSort : "name";
  const q = (sp.get("q") || "").trim();
  const format = (sp.get("format") || "").trim().toLowerCase();

  let per = Number.parseInt(sp.get("per") || "50", 10);
  if (!Number.isFinite(per) || per < 10 || per > 200) per = 50;
  let page = Number.parseInt(sp.get("page") || "1", 10);
  if (!Number.isFinite(page) || page < 1) page = 1;

  const tokens = q ? q.toLowerCase().split(/\s+/).filter(Boolean) : [];
  const filtered = catalog.filter((it) => {
    if (col && it.collection !== col) return false;
    if (kind === "prebuilt" && !it.prebuilt) return false;
    if (kind === "source" && it.prebuilt) return false;
    if (tokens.length > 0) {
      const hay = `${it.name} ${it.collection} ${it.description}`.toLowerCase();
      for (const tok of tokens) {
        if (!hay.includes(tok)) return false;
      }
    }
    return true;
  });

  filtered.sort((a, b) => {
    if (sortBy === "col" && a.collection !== b.collection) {
      return a.collection < b.collection ? -1 : 1;
    }
    if (sortBy === "version" && a.version !== b.version) {
      return a.version > b.version ? -1 : 1;
    }
    return a.name < b.name ? -1 : a.name > b.name ? 1 : 0;
  });

  if (format === "txt") {
    const lines = [
      `# KryX Linux browse: col=${JSON.stringify(col)} kind=${kind} sort=${sortBy} q=${JSON.stringify(q)} (${filtered.length} item(s))`,
    ];
    for (const it of filtered) {
      const st = (it.prebuilt ? "prebuilt" : "source").padEnd(8);
      const c = (it.collection || "-").padEnd(10);
      const n = it.name.padEnd(26);
      const v = (it.version || "-").padEnd(16);
      lines.push(`${st} ${c} ${n} ${v} ${SITE_URL}${it.url}`);
    }
    return new Response(lines.join("\n") + "\n", {
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "public, max-age=60",
      },
    });
  }

  const total = filtered.length;
  const totalPages = Math.max(1, Math.ceil(total / per));
  if (page > totalPages) page = totalPages;
  const start = (page - 1) * per;
  const slice = filtered.slice(start, start + per);

  const colOpts = [
    `<option value="">All collections</option>`,
    ...PORT_COLLECTIONS.map((c) => {
      const sel = c === col ? " selected" : "";
      return `<option value="${escapeHTML(c)}"${sel}>${escapeHTML(c)}</option>`;
    }),
  ].join("\n    ");

  const kindOpts = [
    ["all", "All (binary + source)"],
    ["prebuilt", "Prebuilt binary only"],
    ["source", "Source port only"],
  ]
    .map(([v, l]) => `<option value="${v}"${v === kind ? " selected" : ""}>${l}</option>`)
    .join("\n    ");

  const sortOpts = [
    ["name", "Sort: Name"],
    ["col", "Sort: Collection"],
    ["version", "Sort: Version"],
  ]
    .map(([v, l]) => `<option value="${v}"${v === sortBy ? " selected" : ""}>${l}</option>`)
    .join("\n    ");

  const txtURL = buildBrowseURL({ col, kind, sortBy, q, per, page: 1, format: "txt" });

  let body = `<h1>Browse Packages &amp; Ports</h1>
<p class="meta-note">Zero-JavaScript server-side filter across all prebuilt binary packages and source ports. Works natively in <code>lynx</code>, <code>w3m</code>, <code>links</code>, and <code>curl</code>.</p>
<form class="filter-bar" action="/browse" method="get" role="search">
  <label class="sr-only" for="b-q">Filter query</label>
  <input id="b-q" type="search" name="q" value="${escapeHTML(q)}" placeholder="Filter by name or description...">
  <label class="sr-only" for="b-col">Collection</label>
  <select id="b-col" name="col">
    ${colOpts}
  </select>
  <label class="sr-only" for="b-kind">Availability</label>
  <select id="b-kind" name="kind">
    ${kindOpts}
  </select>
  <label class="sr-only" for="b-sort">Sort order</label>
  <select id="b-sort" name="sort">
    ${sortOpts}
  </select>
  <button type="submit">Filter</button>
</form>
<p class="meta-note">${total} matching item(s) &middot; page ${page} of ${totalPages} &middot; <a href="${escapeHTML(txtURL)}">Plain text (curl)</a> &middot; <a href="/browse">Reset</a></p>
`;

  if (total === 0) {
    body += `<p>No packages or ports matched the active filter.</p>\n`;
  } else {
    const rows = slice
      .map((it) => {
        const st = it.prebuilt
          ? `<span class="badge-ok">prebuilt</span>`
          : `<span class="kind-tag">source</span>`;
        const colCell = it.collection
          ? `<a href="/browse?col=${encodeURIComponent(it.collection)}">${escapeHTML(it.collection)}</a>`
          : "&mdash;";
        const ver = it.version || "—";
        return `<tr><td><a href="${escapeHTML(it.url)}">${escapeHTML(it.name)}</a></td><td><code>${escapeHTML(ver)}</code></td><td>${colCell}</td><td>${st}</td><td>${escapeHTML(it.description)}</td><td><a href="/deps?pkg=${encodeURIComponent(it.name)}">deps</a></td></tr>`;
      })
      .join("\n");
    body += `<div class="table-wrap">
<table>
<thead>
<tr><th>Name</th><th>Version</th><th>Collection</th><th>Status</th><th>Description</th><th>Graph</th></tr>
</thead>
<tbody>
${rows}
</tbody>
</table>
</div>
`;

    if (totalPages > 1) {
      const prevHTML =
        page > 1
          ? `<a class="btn" href="${escapeHTML(buildBrowseURL({ col, kind, sortBy, q, per, page: page - 1 }))}">&larr; Previous</a>`
          : `<span class="page-disabled">&larr; Previous</span>`;
      const nextHTML =
        page < totalPages
          ? `<a class="btn" href="${escapeHTML(buildBrowseURL({ col, kind, sortBy, q, per, page: page + 1 }))}">Next &rarr;</a>`
          : `<span class="page-disabled">Next &rarr;</span>`;
      body += `<nav class="pagination" aria-label="Pagination">
  ${prevHTML}
  <span class="page-info">Page ${page} of ${totalPages}</span>
  ${nextHTML}
</nav>
`;
    }
  }

  const html = renderPage({
    title: "Browse Packages & Ports",
    description:
      "Filter and paginate KryX Linux binary packages and source ports without JavaScript.",
    canonical: "/browse",
    activeNav: "packages",
    bodyHTML: body,
    nextPath: reqURL.pathname + reqURL.search,
  });

  return new Response(html, {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "public, max-age=60",
    },
  });
}
