// Shared helpers for KryX Linux Cloudflare Pages Functions.
// Prefixed with "_" so Cloudflare Pages never exposes this file as an HTTP route.
// All server-side functions live outside public/ so 0 bytes of client-side JS are served.

export const SITE_URL = "https://kryx.anrix.org";

export const VALID_SCOPES = [
  ["all", "All"],
  ["packages", "Packages"],
  ["ports", "Ports"],
  ["docs", "Handbook"],
  ["man", "Man Pages"],
  ["news", "News"],
];

export const PORT_COLLECTIONS = [
  "kpm-tools",
  "kernel",
  "system",
  "core",
  "opt",
  "xorg",
  "contrib",
  "compat-32",
];

const TOP_NAV = [
  ["home", "Home", "/"],
  ["download", "Download", "/download/"],
  ["docs", "Docs", "/docs/"],
  ["packages", "Packages", "/packages/"],
  ["ports", "Ports", "/ports/"],
  ["news", "News", "/news/"],
  ["status", "Status", "/status/"],
  ["github", "GitHub", "https://github.com/kryx-linux"],
];

export function escapeHTML(str) {
  return String(str ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function safeNextPath(raw) {
  const s = String(raw || "").trim();
  if (!s || !s.startsWith("/") || s.startsWith("//")) return "/";
  if (/[\r\n\\]/.test(s)) return "/";
  return s;
}

export function renderPage({
  title,
  description = "KryX Linux — CRUX-inspired, binary-first, minimal Linux distribution.",
  canonical = "/",
  activeNav = "",
  bodyHTML = "",
  nextPath = "",
}) {
  const fullTitle =
    title && title !== "KryX Linux" ? `${title} — KryX Linux` : "KryX Linux";
  const canonicalURL = `${SITE_URL}${canonical.startsWith("/") ? canonical : "/" + canonical}`;
  const safeNext = safeNextPath(nextPath || canonical);
  const encNext = encodeURIComponent(safeNext);

  const navHTML = TOP_NAV.map(([key, label, href]) => {
    const cur = key === activeNav ? ' aria-current="page"' : "";
    return `<a href="${escapeHTML(href)}"${cur}>${escapeHTML(label)}</a>`;
  }).join("\n        ");

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHTML(fullTitle)}</title>
  <meta name="description" content="${escapeHTML(description)}">
  <link rel="canonical" href="${escapeHTML(canonicalURL)}">
  <link rel="icon" type="image/svg+xml" href="/kryx.svg">
  <link rel="alternate" type="application/rss+xml" title="KryX Linux News" href="/feed.xml">
  <link rel="search" type="application/opensearchdescription+xml" title="KryX Linux" href="/opensearch.xml">
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <a class="skip-link" href="#main">Skip to content</a>
  <header class="site-header">
    <div class="header-inner">
      <div class="brand-row">
        <a class="brand-link" href="/">
          <img class="brand-logo" src="/kryx.svg" alt="" width="24" height="21">
          <span>KryX Linux</span>
        </a>
      </div>
      <nav class="site-nav" aria-label="Primary">
        ${navHTML}
      </nav>
    </div>
  </header>
  <main id="main">
${bodyHTML}
  </main>
  <footer class="site-footer">
    <div class="footer-inner">
      <div class="footer-links">
        <a href="/search/">Search</a>
        <a href="/browse">Browse</a>
        <a href="/deps/">Deps</a>
        <a href="/man/">Man Pages</a>
        <a href="/install.txt">install.txt</a>
        <a href="/SHA256SUMS.txt">SHA256SUMS.txt</a>
        <a href="/kryx-release.pub">kryx-release.pub</a>
        <a href="/packages.txt">packages.txt</a>
        <a href="/ports.txt">ports.txt</a>
        <a href="/feed.xml">RSS Feed</a>
        <a href="https://github.com/kryx-linux">GitHub</a>
      </div>
      <div class="footer-meta">
        <p>KryX Linux &mdash; licensed under <a href="https://github.com/kryx-linux/website/blob/main/LICENSE">GPL-3.0</a> &middot; 0 bytes of client-side JavaScript.</p>
        <p class="theme-switcher">Theme: <a href="/theme?mode=auto&amp;next=${escapeHTML(encNext)}">Auto</a> &middot; <a href="/theme?mode=light&amp;next=${escapeHTML(encNext)}">Light</a> &middot; <a href="/theme?mode=dark&amp;next=${escapeHTML(encNext)}">Dark</a></p>
      </div>
    </div>
  </footer>
</body>
</html>
`;
}
