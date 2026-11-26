// Cloudflare Pages Function: server-side zero-JS search at /search.
// Lives outside public/ so zero bytes of client-side JavaScript are ever served.
// Works natively with HTML <form method="get" action="/search"> in lynx, w3m,
// links, curl (?format=txt), and all graphical browsers.

import { SITE_URL, VALID_SCOPES, escapeHTML, renderPage } from "./_shared.js";

let cachedIndex = null;

function normalizeScope(raw) {
  switch (String(raw || "").trim().toLowerCase()) {
    case "packages":
    case "package":
    case "pkg":
      return "packages";
    case "ports":
    case "port":
      return "ports";
    case "docs":
    case "doc":
    case "handbook":
      return "docs";
    case "man":
    case "manpages":
      return "man";
    case "news":
      return "news";
    default:
      return "all";
  }
}

function matchesScope(kind, scope) {
  if (scope === "all") return true;
  if (scope === "packages") return kind === "package";
  if (scope === "ports") return kind === "port";
  if (scope === "docs") return kind === "doc";
  if (scope === "man") return kind === "man";
  if (scope === "news") return kind === "news";
  return true;
}

function searchEntries(entries, query, scope) {
  const tokens = query
    .toLowerCase()
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  if (tokens.length === 0) return [];
  const fullQuery = tokens.join(" ");

  const matches = [];
  for (const e of entries) {
    if (!matchesScope(e.k, scope)) continue;
    const nameL = (e.n || "").toLowerCase();
    const colL = (e.c || "").toLowerCase();
    const descL = (e.d || "").toLowerCase();
    const kindL = (e.k || "").toLowerCase();

    let ok = true;
    let score = 0;
    for (const tok of tokens) {
      if (nameL === tok) score += 1000;
      else if (nameL.startsWith(tok)) score += 500;
      else if (nameL.includes(tok)) score += 250;
      else if (colL && colL === tok) score += 140;
      else if (colL && colL.includes(tok)) score += 100;
      else if (descL.includes(tok)) score += 60;
      else if (kindL === tok) score += 40;
      else {
        ok = false;
        break;
      }
    }
    if (!ok) continue;

    if (nameL === fullQuery) score += 600;
    else if (nameL.startsWith(fullQuery)) score += 200;

    if (e.k === "package") score += 25;
    else if (e.k === "port") score += 15;
    else if (e.k === "doc") score += 12;
    else if (e.k === "man") score += 10;

    matches.push({ e, score });
  }

  matches.sort((a, b) => {
    if (b.score !== a.score) return b.score - a.score;
    const na = a.e.n || "";
    const nb = b.e.n || "";
    if (na !== nb) return na < nb ? -1 : 1;
    return (a.e.k || "") < (b.e.k || "") ? -1 : 1;
  });
  return matches;
}

function renderSearchForm(query, scope) {
  const opts = VALID_SCOPES.map(([val, label]) => {
    const sel = val === scope ? " selected" : "";
    return `    <option value="${escapeHTML(val)}"${sel}>${escapeHTML(label)}</option>`;
  }).join("\n");

  return `<form class="search-form" action="/search" method="get" role="search">
  <label class="sr-only" for="q">Search query</label>
  <input id="q" type="search" name="q" value="${escapeHTML(query)}" placeholder="Search packages, ports, docs, man pages...">
  <label class="sr-only" for="scope">Scope</label>
  <select id="scope" name="scope">
${opts}
  </select>
  <button type="submit">Search</button>
</form>`;
}

async function loadIndex(context) {
  if (cachedIndex) return cachedIndex;
  const assetURL = new URL("/search-index.json", context.request.url);
  const resp = await context.env.ASSETS.fetch(assetURL);
  if (!resp.ok) {
    throw new Error(`failed to load search-index.json (${resp.status})`);
  }
  cachedIndex = await resp.json();
  return cachedIndex;
}

export async function onRequestGet(context) {
  const reqURL = new URL(context.request.url);
  const q = (reqURL.searchParams.get("q") || "").trim();
  const scope = normalizeScope(reqURL.searchParams.get("scope"));
  const format = (reqURL.searchParams.get("format") || "").trim().toLowerCase();

  if (!q && format !== "txt") {
    const staticURL = new URL("/search/index.html", context.request.url);
    return context.env.ASSETS.fetch(staticURL);
  }

  let entries;
  try {
    entries = await loadIndex(context);
  } catch {
    return new Response("Search index temporarily unavailable.", {
      status: 500,
      headers: { "content-type": "text/plain; charset=utf-8" },
    });
  }

  const allMatches = searchEntries(entries, q, scope);
  const total = allMatches.length;
  const shown = allMatches.slice(0, 150);

  if (format === "txt") {
    const lines = [
      `# KryX Linux search: q=${JSON.stringify(q)} scope=${scope} (${total} match(es))`,
    ];
    for (const { e } of shown) {
      const col = (e.c || "-").padEnd(10);
      const kind = (e.k || "-").padEnd(8);
      const name = (e.n || "-").padEnd(24);
      const ver = (e.v || "-").padEnd(14);
      lines.push(`${kind} ${col} ${name} ${ver} ${SITE_URL}${e.u}`);
    }
    return new Response(lines.join("\n") + "\n", {
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "public, max-age=60",
      },
    });
  }

  let body = `<h1>Search KryX Linux</h1>\n${renderSearchForm(q, scope)}\n`;
  if (total === 0) {
    body += `<p>No results found for <code>${escapeHTML(q)}</code> in scope <code>${escapeHTML(scope)}</code>.</p>\n`;
    body += `<p class="meta-note">Try fewer keywords, switch scope to <em>All</em>, or browse <a href="/browse">/browse</a>, <a href="/packages/">/packages/</a>, and <a href="/ports/">/ports/</a>.</p>\n`;
  } else {
    if (total > shown.length) {
      body += `<p class="meta-note">Showing top ${shown.length} of ${total} matches for <code>${escapeHTML(q)}</code> (scope: <code>${escapeHTML(scope)}</code>).</p>\n`;
    } else {
      body += `<p class="meta-note">${total} match(es) for <code>${escapeHTML(q)}</code> (scope: <code>${escapeHTML(scope)}</code>).</p>\n`;
    }
    const rows = shown
      .map(({ e }) => {
        const colCell = e.c
          ? `<a href="/ports/${escapeHTML(e.c)}/">${escapeHTML(e.c)}</a>`
          : "&mdash;";
        const verCell = e.v ? `<code>${escapeHTML(e.v)}</code>` : "&mdash;";
        return `<tr><td><span class="kind-tag">${escapeHTML(e.k)}</span></td><td><a href="${escapeHTML(e.u)}">${escapeHTML(e.n)}</a></td><td>${verCell}</td><td>${colCell}</td><td>${escapeHTML(e.d || "")}</td></tr>`;
      })
      .join("\n");
    body += `<div class="table-wrap">\n<table>\n<thead>\n<tr><th>Type</th><th>Name</th><th>Version</th><th>Collection</th><th>Description</th></tr>\n</thead>\n<tbody>\n${rows}\n</tbody>\n</table>\n</div>\n`;
  }

  const html = renderPage({
    title: `Search: ${q}`,
    description: "Zero-JavaScript server-side search results across KryX Linux.",
    canonical: "/search/",
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
