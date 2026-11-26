// Cloudflare Pages Function: zero-JS Dependency Explorer at /deps.
// Supports HTML, ?mode=why (reverse deps), ?include_make=1, ?format=txt, and ?format=dot.

import { SITE_URL, escapeHTML, renderPage } from "./_shared.js";

let cachedGraph = null;

async function loadGraph(context) {
  if (cachedGraph) return cachedGraph;
  const assetURL = new URL("/deps-graph.json", context.request.url);
  const resp = await context.env.ASSETS.fetch(assetURL);
  if (!resp.ok) {
    throw new Error(`failed to load deps-graph.json (${resp.status})`);
  }
  cachedGraph = await resp.json();
  return cachedGraph;
}

function renderDepsForm(pkg, mode, includeMake) {
  const treeSel = mode !== "why" ? " selected" : "";
  const whySel = mode === "why" ? " selected" : "";
  const chk = includeMake ? " checked" : "";
  return `<form class="search-form" action="/deps" method="get" role="search">
  <label class="sr-only" for="pkg">Package or port name</label>
  <input id="pkg" type="search" name="pkg" value="${escapeHTML(pkg)}" placeholder="Enter package or port name (e.g. wlroots, grub2, dbus)...">
  <label class="sr-only" for="mode">Mode</label>
  <select id="mode" name="mode">
    <option value="tree"${treeSel}>Dependencies (tree)</option>
    <option value="why"${whySel}>Required By (why)</option>
  </select>
  <label class="check-inline"><input type="checkbox" name="include_make" value="1"${chk}> include make deps</label>
  <button type="submit">Inspect</button>
</form>`;
}

function buildDepTreeLines(graph, root, includeMake, maxDepth = 8, maxLines = 200) {
  const lines = [];
  const visited = new Set();
  let truncated = false;

  function walk(name, prefix, isLast, isRoot, isMake, depth, stack) {
    if (lines.length >= maxLines) {
      truncated = true;
      return;
    }
    const node = Object.prototype.hasOwnProperty.call(graph, name)
      ? graph[name]
      : null;
    let branch = "";
    let childPrefix = prefix;
    if (!isRoot) {
      if (isLast) {
        branch = `${prefix}\`-- `;
        childPrefix = `${prefix}    `;
      } else {
        branch = `${prefix}|-- `;
        childPrefix = `${prefix}|   `;
      }
    }

    if (stack.has(name)) {
      lines.push({ prefix: branch, name, node, cycle: true, repeat: false, isMake });
      return;
    }
    if (visited.has(name) && !isRoot) {
      lines.push({ prefix: branch, name, node, cycle: false, repeat: true, isMake });
      return;
    }
    visited.add(name);
    lines.push({ prefix: branch, name, node, cycle: false, repeat: false, isMake });

    if (!node || depth >= maxDepth) return;

    const children = [];
    for (const d of node.d || []) children.push({ name: d, isMake: false });
    if (includeMake) {
      for (const m of node.m || []) children.push({ name: m, isMake: true });
    }
    if (children.length === 0) return;

    stack.add(name);
    for (let i = 0; i < children.length; i++) {
      walk(
        children[i].name,
        childPrefix,
        i === children.length - 1,
        false,
        children[i].isMake,
        depth + 1,
        stack
      );
    }
    stack.delete(name);
  }

  walk(root, "", true, true, false, 0, new Set());
  return { lines, total: visited.size, truncated };
}

function findDirectReverseDeps(graph, target, includeMake) {
  const out = [];
  for (const [name, node] of Object.entries(graph)) {
    if (name === target || !node) continue;
    if (Array.isArray(node.d) && node.d.includes(target)) {
      out.push({ name, node, isMake: false });
    } else if (includeMake && Array.isArray(node.m) && node.m.includes(target)) {
      out.push({ name, node, isMake: true });
    }
  }
  out.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  return out;
}

function renderDepDOT(graph, root, includeMake, maxNodes = 120) {
  const lines = [
    "digraph kryx_deps {",
    "  rankdir=LR;",
    '  node [shape=box, fontname="monospace", fontsize=10];',
  ];
  const seen = new Set([root]);
  const queue = [root];

  while (queue.length > 0 && seen.size <= maxNodes) {
    const cur = queue.shift();
    const node = Object.prototype.hasOwnProperty.call(graph, cur)
      ? graph[cur]
      : null;
    if (!node) continue;
    for (const d of node.d || []) {
      lines.push(`  ${JSON.stringify(cur)} -> ${JSON.stringify(d)};`);
      if (!seen.has(d) && seen.size < maxNodes) {
        seen.add(d);
        queue.push(d);
      }
    }
    if (includeMake) {
      for (const m of node.m || []) {
        lines.push(
          `  ${JSON.stringify(cur)} -> ${JSON.stringify(m)} [style=dashed, label="make"];`
        );
        if (!seen.has(m) && seen.size < maxNodes) {
          seen.add(m);
          queue.push(m);
        }
      }
    }
  }
  lines.push("}");
  return lines.join("\n") + "\n";
}

export async function onRequestGet(context) {
  const reqURL = new URL(context.request.url);
  const sp = reqURL.searchParams;
  const pkg = (sp.get("pkg") || "").trim();
  const mode = (sp.get("mode") || "").trim().toLowerCase() === "why" ? "why" : "tree";
  const rawMake = (sp.get("include_make") || "").trim().toLowerCase();
  const includeMake = rawMake === "1" || rawMake === "true";
  const format = (sp.get("format") || "").trim().toLowerCase();

  if (!pkg && format !== "txt" && format !== "dot") {
    const staticURL = new URL("/deps/index.html", context.request.url);
    return context.env.ASSETS.fetch(staticURL);
  }

  let graph;
  try {
    graph = await loadGraph(context);
  } catch {
    return new Response("Dependency graph temporarily unavailable.", {
      status: 500,
      headers: { "content-type": "text/plain; charset=utf-8" },
    });
  }

  const exists = Object.prototype.hasOwnProperty.call(graph, pkg);
  const node = exists ? graph[pkg] : null;

  if (format === "dot") {
    const out = exists
      ? renderDepDOT(graph, pkg, includeMake, 120)
      : `// package ${JSON.stringify(pkg)} not found\n`;
    return new Response(out, {
      headers: {
        "content-type": "text/vnd.graphviz; charset=utf-8",
        "cache-control": "public, max-age=60",
      },
    });
  }

  if (format === "txt") {
    if (mode === "why") {
      const revs = findDirectReverseDeps(graph, pkg, includeMake);
      if (!exists && revs.length === 0) {
        return new Response(
          `# package ${JSON.stringify(pkg)} not found in KryX dependency graph\n`,
          {
            headers: {
              "content-type": "text/plain; charset=utf-8",
              "cache-control": "public, max-age=60",
            },
          }
        );
      }
      const lines = [
        `# Required By ${pkg} (${revs.length} direct dependent(s))`,
      ];
      for (const rv of revs) {
        const tag = (rv.isMake ? "make" : "runtime").padEnd(8);
        lines.push(
          `${rv.name.padEnd(24)} ${(rv.node.c || "-").padEnd(10)} ${tag} ${SITE_URL}${rv.node.u || ""}`
        );
      }
      return new Response(lines.join("\n") + "\n", {
        headers: {
          "content-type": "text/plain; charset=utf-8",
          "cache-control": "public, max-age=60",
        },
      });
    }
    if (!exists) {
      return new Response(
        `# package ${JSON.stringify(pkg)} not found in KryX dependency graph\n`,
        {
          headers: {
            "content-type": "text/plain; charset=utf-8",
            "cache-control": "public, max-age=60",
          },
        }
      );
    }
    const { lines, total, truncated } = buildDepTreeLines(graph, pkg, includeMake);
    const out = [`# Dependency tree for ${pkg} (${total} unique node(s))`];
    for (const ln of lines) {
      let extra = "";
      if (ln.isMake) extra += " [make]";
      if (ln.node && ln.node.b) extra += " [prebuilt]";
      if (ln.cycle) extra += " (cycle)";
      else if (ln.repeat) extra += " (*)";
      out.push(`${ln.prefix}${ln.name}${extra}`);
    }
    if (truncated) out.push("# ... truncated at 200 lines");
    return new Response(out.join("\n") + "\n", {
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "public, max-age=60",
      },
    });
  }

  let body = `<h1>Dependency Explorer</h1>\n${renderDepsForm(pkg, mode, includeMake)}\n`;
  if (mode === "why") {
    const revs = findDirectReverseDeps(graph, pkg, includeMake);
    if (!exists && revs.length === 0) {
      body += `<p>Package or port <code>${escapeHTML(pkg)}</code> was not found in the dependency graph.</p>\n`;
      body += `<p class="meta-note">Try <a href="/search?q=${encodeURIComponent(pkg)}">searching for ${escapeHTML(pkg)}</a> instead.</p>\n`;
    } else {
      if (exists && node && node.u) {
        body += `<h2>Required By: <a href="${escapeHTML(node.u)}">${escapeHTML(pkg)}</a> <code>${escapeHTML(node.v || "")}</code></h2>\n`;
      } else {
        body += `<h2>Required By: <code>${escapeHTML(pkg)}</code></h2>\n`;
      }
      body += `<p class="meta-note">${revs.length} direct dependent(s) &middot; <a href="/deps?pkg=${encodeURIComponent(pkg)}">View dependency tree</a> &middot; <a href="/deps?pkg=${encodeURIComponent(pkg)}&amp;mode=why&amp;format=txt">Plain text</a></p>\n`;
      if (revs.length === 0) {
        body += `<p>No other indexed packages or ports declare a direct dependency on this package.</p>\n`;
      } else {
        const rows = revs
          .map((rv) => {
            const kind = rv.isMake ? "make" : "runtime";
            return `<tr><td><a href="${escapeHTML(rv.node.u)}">${escapeHTML(rv.name)}</a></td><td><code>${escapeHTML(rv.node.v || "—")}</code></td><td><a href="/ports/${escapeHTML(rv.node.c || "")}/">${escapeHTML(rv.node.c || "")}</a></td><td><span class="kind-tag">${kind}</span></td><td><a href="/deps?pkg=${encodeURIComponent(rv.name)}">tree</a></td></tr>`;
          })
          .join("\n");
        body += `<div class="table-wrap">\n<table>\n<thead>\n<tr><th>Package</th><th>Version</th><th>Collection</th><th>Dependency</th><th>Explore</th></tr>\n</thead>\n<tbody>\n${rows}\n</tbody>\n</table>\n</div>\n`;
      }
    }
  } else if (!exists) {
    body += `<p>Package or port <code>${escapeHTML(pkg)}</code> was not found in the dependency graph.</p>\n`;
    body += `<p class="meta-note">Try <a href="/search?q=${encodeURIComponent(pkg)}">searching for ${escapeHTML(pkg)}</a> instead.</p>\n`;
  } else {
    const { lines, total, truncated } = buildDepTreeLines(graph, pkg, includeMake);
    const makeParam = includeMake ? "&amp;include_make=1" : "";
    body += `<h2>Dependency Tree: <a href="${escapeHTML(node.u)}">${escapeHTML(pkg)}</a> <code>${escapeHTML(node.v || "")}</code></h2>\n`;
    body += `<p class="meta-note">${total} unique node(s) &middot; <a href="/deps?pkg=${encodeURIComponent(pkg)}&amp;mode=why">Required By (why)</a> &middot; <a href="/deps?pkg=${encodeURIComponent(pkg)}${makeParam}&amp;format=txt">Plain text</a> &middot; <a href="/deps?pkg=${encodeURIComponent(pkg)}${makeParam}&amp;format=dot">Graphviz DOT</a></p>\n`;
    const renderedLines = lines
      .map((ln) => {
        let s = escapeHTML(ln.prefix);
        if (ln.node && ln.node.u) {
          s += `<a href="${escapeHTML(ln.node.u)}">${escapeHTML(ln.name)}</a>`;
        } else {
          s += escapeHTML(ln.name);
        }
        if (ln.node && ln.node.v) s += ` (${escapeHTML(ln.node.v)})`;
        if (ln.isMake) s += " [make]";
        if (ln.node && ln.node.b) s += " [prebuilt]";
        if (ln.cycle) s += " (cycle)";
        else if (ln.repeat) s += " (*)";
        return s;
      })
      .join("\n");
    body += `<pre class="dep-tree"><code>${renderedLines}${truncated ? "\n... truncated at 200 lines" : ""}</code></pre>\n`;
  }

  const html = renderPage({
    title: `Dependencies: ${pkg}`,
    description: `Dependency graph for ${pkg} in KryX Linux.`,
    canonical: "/deps/",
    activeNav: "ports",
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
