package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SiteContext holds site-wide configuration for page rendering.
type SiteContext struct {
	SiteURL   string
	BuildTime time.Time
}

// PageMeta describes HTML head and navigation metadata for a single page.
type PageMeta struct {
	Title       string
	Description string
	ActiveNav   string // home, download, docs, packages, ports, news, status
	Canonical   string // Path starting with "/"
}

// ReleaseInfo mirrors data/release.json.
type ReleaseInfo struct {
	Version            string            `json:"version"`
	ReleaseDate        string            `json:"release_date"`
	Arch               string            `json:"arch"`
	SignifyKeyID       string            `json:"signify_key_id"`
	SignifyFingerprint string            `json:"signify_fingerprint"`
	Artifacts          []ReleaseArtifact `json:"artifacts"`
}

// ReleaseArtifact describes one downloadable image or tarball.
type ReleaseArtifact struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Arch   string `json:"arch"`
	Size   string `json:"size"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type navItem struct {
	Key   string
	Label string
	Href  string
}

// Strictly 8 top-level navigation items (usable inline in lynx/w3m/links).
var topNavItems = []navItem{
	{Key: "home", Label: "Home", Href: "/"},
	{Key: "download", Label: "Download", Href: "/download/"},
	{Key: "docs", Label: "Docs", Href: "/docs/"},
	{Key: "packages", Label: "Packages", Href: "/packages/"},
	{Key: "ports", Label: "Ports", Href: "/ports/"},
	{Key: "news", Label: "News", Href: "/news/"},
	{Key: "status", Label: "Status", Href: "/status/"},
	{Key: "github", Label: "GitHub", Href: "https://github.com/kryx-linux"},
}

func renderPageShell(site SiteContext, meta PageMeta, mainHTML string) string {
	fullTitle := "KryX Linux"
	if meta.Title != "" && meta.Title != "KryX Linux" {
		fullTitle = meta.Title + " — KryX Linux"
	}
	desc := defaultStr(meta.Description, "KryX Linux — CRUX-inspired, binary-first, minimal Linux distribution.")
	canonicalURL := strings.TrimRight(site.SiteURL, "/") + meta.Canonical

	var nav strings.Builder
	for i, item := range topNavItems {
		if i > 0 {
			nav.WriteString("\n      ")
		}
		cur := ""
		if item.Key == meta.ActiveNav {
			cur = ` aria-current="page"`
		}
		fmt.Fprintf(&nav, `<a href="%s"%s>%s</a>`, html.EscapeString(item.Href), cur, html.EscapeString(item.Label))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <meta name="description" content="%s">
  <link rel="canonical" href="%s">
  <link rel="icon" type="image/svg+xml" href="/kryx.svg">
  <link rel="alternate" type="application/rss+xml" title="KryX Linux News" href="/feed.xml">
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
        %s
      </nav>
    </div>
  </header>
  <main id="main">
%s
  </main>
  <footer class="site-footer">
    <div class="footer-inner">
      <div class="footer-links">
        <a href="/search/">Search</a>
        <a href="/man/">Man Pages</a>
        <a href="/install.txt">install.txt</a>
        <a href="/SHA256SUMS.txt">SHA256SUMS.txt</a>
        <a href="/kryx-release.pub">kryx-release.pub</a>
        <a href="/packages.txt">packages.txt</a>
        <a href="/ports.txt">ports.txt</a>
        <a href="/feed.xml">RSS Feed</a>
        <a href="https://github.com/kryx-linux">GitHub</a>
      </div>
      <p>KryX Linux &mdash; licensed under <a href="https://github.com/kryx-linux/website/blob/main/LICENSE">GPL-3.0</a> &middot; 0 bytes of client-side JavaScript &middot; built %s UTC.</p>
    </div>
  </footer>
</body>
</html>
`,
		html.EscapeString(fullTitle),
		html.EscapeString(desc),
		html.EscapeString(canonicalURL),
		nav.String(),
		mainHTML,
		site.BuildTime.UTC().Format("2006-01-02 15:04"),
	)
}

func writeHTMLPage(dstPath string, site SiteContext, meta PageMeta, mainHTML string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dstPath, []byte(renderPageShell(site, meta, mainHTML)), 0o644)
}

func renderDownloadArtifactsTable(rel ReleaseInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<h2>Current Artifacts (<code>%s</code> &middot; %s)</h2>\n",
		html.EscapeString(rel.Version), html.EscapeString(rel.ReleaseDate))
	sb.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Artifact</th><th>Type</th><th>Arch</th><th>Size</th><th>SHA-256</th></tr>\n</thead>\n<tbody>\n")
	for _, a := range rel.Artifacts {
		shortSum := a.SHA256
		if len(shortSum) > 16 && !strings.Contains(shortSum, " ") {
			shortSum = shortSum[:16] + "…"
		}
		fmt.Fprintf(&sb, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td><code>%s</code></td><td>%s</td><td><code title=\"%s\">%s</code></td></tr>\n",
			html.EscapeString(a.URL),
			html.EscapeString(a.Name),
			html.EscapeString(a.Kind),
			html.EscapeString(a.Arch),
			html.EscapeString(a.Size),
			html.EscapeString(a.SHA256),
			html.EscapeString(shortSum),
		)
	}
	sb.WriteString("</tbody>\n</table>\n</div>\n")
	fmt.Fprintf(&sb, "<p>Full checksum manifest: <a href=\"/SHA256SUMS.txt\">SHA256SUMS.txt</a> &middot; Public signing key: <a href=\"/kryx-release.pub\">kryx-release.pub</a> (Key ID <code>%s</code>)</p>\n",
		html.EscapeString(rel.SignifyKeyID))
	return sb.String()
}

func generatePlainTextEndpoints(outDir string, site SiteContext, rel ReleaseInfo) error {
	// 1. /SHA256SUMS.txt
	var sums strings.Builder
	fmt.Fprintf(&sums, "# KryX Linux %s (%s) SHA-256 Checksums\n", rel.Version, rel.ReleaseDate)
	fmt.Fprintf(&sums, "# Verify: sha256sum -c --ignore-missing SHA256SUMS.txt\n")
	for _, a := range rel.Artifacts {
		if strings.Contains(a.SHA256, " ") {
			continue
		}
		fmt.Fprintf(&sums, "%s  %s\n", a.SHA256, a.Name)
	}
	if err := os.WriteFile(filepath.Join(outDir, "SHA256SUMS.txt"), []byte(sums.String()), 0o644); err != nil {
		return err
	}

	// 2. /install.txt (concise terminal guide for live ISO / curl | less)
	installTxt := fmt.Sprintf(`KRYX LINUX — LIVE ISO / TERMINAL INSTALLATION GUIDE (%s)
Website: %s
Signing Key ID: %s (%s)
========================================================================

1. PARTITION & MOUNT (UEFI example on /dev/nvme0n1)
   cfdisk /dev/nvme0n1
   mkfs.fat -F32 /dev/nvme0n1p1
   mkfs.ext4 /dev/nvme0n1p2
   mount /dev/nvme0n1p2 /mnt
   mkdir -p /mnt/boot/efi
   mount /dev/nvme0n1p1 /mnt/boot/efi

2A. FROM KRYX LIVE ISO (kryx-linux-x86_64.iso, GRUB BIOS + UEFI)
   # /etc/kpm/keys/kryx-release.pub is pre-configured in the live image
   setup /mnt

2B. OR BOOTSTRAP FROM ANY X86_64 LIVE LINUX ISO
   curl -fsSLO %s/SHA256SUMS.txt
   curl -fsSLO %s/kryx-release.pub
   curl -fsSLO https://github.com/kryx-linux/iso/releases/latest/download/kryx-rootfs-x86_64-rolling.tar.xz
   sha256sum -c --ignore-missing SHA256SUMS.txt
   tar -xpf kryx-rootfs-x86_64-rolling.tar.xz -C /mnt
   setup-repo --root /mnt --key %s/kryx-release.pub --verify
   kpm --root=/mnt install --deps kpm kernel grub2 grub2-efi efibootmgr

3. CHROOT & VERIFY SYSTEM HEALTH
   setup-chroot /mnt
   kpm doctor
   kpm check --files
   kpm check --broken

4. CONFIGURE BASE SYSTEM & GRUB BOOTLOADER
   passwd
   echo "kryx" > /etc/hostname
   ln -sf /usr/share/zoneinfo/UTC /etc/localtime
   # Edit /etc/fstab, then install GRUB:
   grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=KryX
   # (Legacy BIOS: grub-install --target=i386-pc /dev/sda)
   grub-mkconfig -o /boot/grub/grub.cfg

5. OPTIONAL: SYNC LOCAL PORTS TREE
   ports check
   ports -u

6. REBOOT
   exit
   umount -R /mnt
   reboot

Full handbook: %s/docs/installation.html
`,
		rel.Version,
		strings.TrimRight(site.SiteURL, "/"),
		rel.SignifyKeyID,
		rel.SignifyFingerprint,
		strings.TrimRight(site.SiteURL, "/"),
		strings.TrimRight(site.SiteURL, "/"),
		strings.TrimRight(site.SiteURL, "/"),
		strings.TrimRight(site.SiteURL, "/"),
	)
	return os.WriteFile(filepath.Join(outDir, "install.txt"), []byte(installTxt), 0o644)
}

func renderStatusPage(site SiteContext, idx RepoIndex, indexSource string, byCol map[string][]PortRecipe, rel ReleaseInfo, manCount int) string {
	totalPorts := 0
	for _, col := range portCollectionsOrder {
		totalPorts += len(byCol[col])
	}

	var sb strings.Builder
	sb.WriteString("<h1>Repository &amp; Build Status</h1>\n")
	sb.WriteString("<p>Live snapshot of KryX Linux's <code>rolling-x86_64</code> binary repository, ports collections, signing key identity, and CI pipelines.</p>\n")

	sb.WriteString("<h2>Repository Summary</h2>\n<div class=\"table-wrap\">\n<table>\n<tbody>\n")
	fmt.Fprintf(&sb, "<tr><th>Binary Packages Indexed</th><td>%d (<a href=\"/packages/\">browse</a>)</td></tr>\n", len(idx.Packages))
	fmt.Fprintf(&sb, "<tr><th>Port Recipes Indexed</th><td>%d (<a href=\"/ports/\">browse</a>)</td></tr>\n", totalPorts)
	fmt.Fprintf(&sb, "<tr><th>Web Man Pages</th><td>%d (<a href=\"/man/\">browse</a>)</td></tr>\n", manCount)
	fmt.Fprintf(&sb, "<tr><th>Release Channel</th><td><code>rolling-x86_64</code> (%s)</td></tr>\n", html.EscapeString(rel.Version))
	fmt.Fprintf(&sb, "<tr><th>Index Source</th><td><code>%s</code></td></tr>\n", html.EscapeString(indexSource))
	fmt.Fprintf(&sb, "<tr><th>Last Site Sync (UTC)</th><td><code>%s</code></td></tr>\n", site.BuildTime.UTC().Format(time.RFC3339))
	sb.WriteString("</tbody>\n</table>\n</div>\n")

	sb.WriteString("<h2>Signing Key Identity</h2>\n<div class=\"table-wrap\">\n<table>\n<tbody>\n")
	fmt.Fprintf(&sb, "<tr><th>Key Format</th><td><code>kryx-ed25519-pub-v1</code> (Ed25519 / <code>kpm</code>)</td></tr>\n")
	fmt.Fprintf(&sb, "<tr><th>Key ID</th><td><code>%s</code></td></tr>\n", html.EscapeString(rel.SignifyKeyID))
	fmt.Fprintf(&sb, "<tr><th>Fingerprint</th><td><code>%s</code></td></tr>\n", html.EscapeString(rel.SignifyFingerprint))
	sb.WriteString("<tr><th>Public Key URL</th><td><a href=\"/kryx-release.pub\">https://kryx.anrix.org/kryx-release.pub</a></td></tr>\n")
	sb.WriteString("</tbody>\n</table>\n</div>\n")

	sb.WriteString("<h2>GitHub Actions CI Status</h2>\n")
	sb.WriteString("<div class=\"ci-badges\">\n")
	sb.WriteString("<a href=\"https://github.com/kryx-linux/kpm/actions\"><img src=\"https://github.com/kryx-linux/kpm/actions/workflows/ci.yml/badge.svg\" alt=\"kpm CI status\"></a>\n")
	sb.WriteString("<a href=\"https://github.com/kryx-linux/ports/actions\"><img src=\"https://github.com/kryx-linux/ports/actions/workflows/build-publish.yml/badge.svg\" alt=\"ports matrix build status\"></a>\n")
	sb.WriteString("<a href=\"https://github.com/kryx-linux/iso/actions\"><img src=\"https://github.com/kryx-linux/iso/actions/workflows/iso.yml/badge.svg\" alt=\"ISO and base image build status\"></a>\n")
	sb.WriteString("</div>\n")

	sb.WriteString("<h2>Collection Breakdown</h2>\n<div class=\"table-wrap\">\n<table>\n<thead>\n<tr><th>Collection</th><th>Port Recipes</th><th>Link</th></tr>\n</thead>\n<tbody>\n")
	for _, col := range portCollectionsOrder {
		fmt.Fprintf(&sb, "<tr><td><code>%s</code></td><td>%d</td><td><a href=\"/ports/%s/\">/ports/%s/</a></td></tr>\n",
			html.EscapeString(col), len(byCol[col]), html.EscapeString(col), html.EscapeString(col))
	}
	sb.WriteString("</tbody>\n</table>\n</div>\n")

	return sb.String()
}

func renderHomePage(rel ReleaseInfo, idx RepoIndex, portsByCol map[string][]PortRecipe, newsPosts []NewsPost) string {
	totalPorts := 0
	for _, col := range portCollectionsOrder {
		totalPorts += len(portsByCol[col])
	}

	var sb strings.Builder
	sb.WriteString("<section class=\"hero\">\n")
	sb.WriteString("  <div class=\"hero-emblem\"><img src=\"/kryx.svg\" alt=\"KryX Linux\" width=\"96\" height=\"84\"></div>\n")
	sb.WriteString("  <div class=\"hero-copy\">\n")
	sb.WriteString("    <h1>KryX Linux</h1>\n")
	sb.WriteString("    <p class=\"hero-lead\">A lightweight, CRUX-inspired rolling Linux distribution for x86_64 with binary-first packages, BSD-style ports, and a static pure-Go toolchain.</p>\n")
	sb.WriteString("    <div class=\"hero-actions\">\n")
	sb.WriteString("      <a class=\"btn btn-primary\" href=\"/download/\">Download</a>\n")
	sb.WriteString("      <a class=\"btn\" href=\"/docs/\">Handbook</a>\n")
	sb.WriteString("      <a class=\"btn\" href=\"/packages/\">Packages</a>\n")
	sb.WriteString("      <a class=\"btn\" href=\"/ports/\">Ports</a>\n")
	sb.WriteString("    </div>\n")
	sb.WriteString(renderSearchForm("", "all", "Search packages, ports, docs, man pages..."))
	sb.WriteString("  </div>\n")
	sb.WriteString("</section>\n\n")

	sb.WriteString("<div class=\"home-grid\">\n")

	// Left column: Overview, features, and quick start
	sb.WriteString("  <section class=\"home-section\">\n")
	sb.WriteString("    <h2>Features</h2>\n")
	sb.WriteString("    <div class=\"feature-grid\">\n")
	sb.WriteString("      <div class=\"feature-card\">\n")
	sb.WriteString("        <h3>Binary-First with kpm</h3>\n")
	sb.WriteString("        <p>Install SHA-256 and signify-verified <code>.pkg.tar.zst</code> archives immediately with unified Tarjan/Kahn dependency resolution and ABI verification.</p>\n")
	sb.WriteString("      </div>\n")
	sb.WriteString("      <div class=\"feature-card\">\n")
	sb.WriteString("        <h3>Integrated Pure-Go Ports</h3>\n")
	sb.WriteString("        <p>Sync collections over git, rsync, httpup, or tarballs natively via <code>ports -u</code> and build from clean <code>Pkgfile</code> recipes on demand.</p>\n")
	sb.WriteString("      </div>\n")
	sb.WriteString("      <div class=\"feature-card\">\n")
	sb.WriteString("        <h3>CRUX Parity &amp; Dual Init</h3>\n")
	sb.WriteString("        <p>Full <code>pkgutils</code> multi-call compatibility (<code>pkgadd</code>, <code>pkgrm</code>, <code>pkginfo</code>, <code>pkgmk</code>, <code>rejmerge</code>) with CRUX BSD-style <code>rc</code> or opt-in <code>dinit</code>.</p>\n")
	sb.WriteString("      </div>\n")
	sb.WriteString("      <div class=\"feature-card\">\n")
	sb.WriteString("        <h3>Static Toolchain</h3>\n")
	sb.WriteString("        <p><code>kpm</code>, <code>kpm-index</code>, and <code>kryx-src</code> compile statically with zero runtime dependencies, bootstrapping cleanly from any x86_64 live ISO.</p>\n")
	sb.WriteString("      </div>\n")
	sb.WriteString("    </div>\n\n")

	sb.WriteString("    <h2>Quick Start</h2>\n")
	sb.WriteString("    <p>Sync the binary index, search, and install a package with dependencies:</p>\n")
	sb.WriteString("    <pre><code>kpm sync\nkpm search wlroots\nkpm install --deps wlroots</code></pre>\n")
	sb.WriteString("    <p>Compile directly from ports using prebuilt build dependencies (<code># Make depends on:</code>):</p>\n")
	sb.WriteString("    <pre><code>ports -u\nkpm build --deps --sync --install zig</code></pre>\n")
	sb.WriteString("    <p>Installing from a live ISO without a graphical browser?</p>\n")
	sb.WriteString("    <pre><code>curl -s https://kryx.anrix.org/install.txt | less</code></pre>\n")
	sb.WriteString("  </section>\n\n")

	// Right column: Latest News + Snapshot summary
	sb.WriteString("  <aside class=\"home-section\">\n")
	sb.WriteString("    <h2>Latest News</h2>\n")
	sb.WriteString("    <ul class=\"news-list\">\n")
	maxNews := len(newsPosts)
	if maxNews > 6 {
		maxNews = 6
	}
	for _, p := range newsPosts[:maxNews] {
		sb.WriteString("      <li>")
		if p.Date != "" {
			fmt.Fprintf(&sb, "<span class=\"news-date\">%s</span>", html.EscapeString(p.Date))
		}
		fmt.Fprintf(&sb, "<a href=\"/news/%s.html\">%s</a></li>\n",
			html.EscapeString(p.Slug), html.EscapeString(p.Title))
	}
	sb.WriteString("    </ul>\n")
	sb.WriteString("    <p class=\"meta-note\"><a href=\"/news/\">All news</a> &middot; <a href=\"/feed.xml\">RSS feed</a></p>\n")

	sb.WriteString("    <div class=\"stat-box\">\n")
	sb.WriteString("      <h3>Repository Snapshot</h3>\n")
	sb.WriteString("      <ul>\n")
	fmt.Fprintf(&sb, "        <li>Channel: <code>rolling-x86_64</code> (%s)</li>\n", html.EscapeString(rel.Version))
	fmt.Fprintf(&sb, "        <li>Binary packages: <a href=\"/packages/\">%d indexed</a> (<a href=\"/packages.txt\">txt</a>)</li>\n", len(idx.Packages))
	fmt.Fprintf(&sb, "        <li>Port recipes: <a href=\"/ports/\">%d across %d collections</a> (<a href=\"/ports.txt\">txt</a>)</li>\n", totalPorts, len(portCollectionsOrder))
	sb.WriteString("        <li>Documentation: <a href=\"/docs/\">Handbook</a> &middot; <a href=\"/man/\">Man Pages</a></li>\n")
	sb.WriteString("        <li>Signing key: <a href=\"/kryx-release.pub\">kryx-release.pub</a> (<a href=\"/status/\">verify</a>)</li>\n")
	sb.WriteString("      </ul>\n")
	sb.WriteString("    </div>\n")
	sb.WriteString("  </aside>\n")

	sb.WriteString("</div>\n")
	return sb.String()
}
