package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkdownAutoTOCAndAnchors(t *testing.T) {
	md := `---
title: Test Guide
description: Testing markdown parser
toc: true
---

# Test Guide

Intro with **bold**, *italic*, ` + "`inline_code`" + `, and [link](/docs/).

## Section One

Paragraph text.

### Sub Section

` + "```sh\nkpm sync\n```" + `

## Section Two

| Col A | Col B |
| :--- | :--- |
| ` + "`kpm`" + ` | fast |
`
	doc := ParseMarkdown(md, true)
	if doc.Meta.Title != "Test Guide" {
		t.Fatalf("unexpected title: %q", doc.Meta.Title)
	}
	if !strings.Contains(doc.HTML, `<nav class="toc"`) {
		t.Fatalf("expected auto-TOC in rendered HTML: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `id="section-one"`) || !strings.Contains(doc.HTML, `class="heading-anchor"`) {
		t.Fatalf("expected heading anchor links: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `<code>inline_code</code>`) {
		t.Fatalf("expected inline code preserved verbatim: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, `<table>`) {
		t.Fatalf("expected rendered table: %s", doc.HTML)
	}
}

func TestBuildSiteEndToEndAndZeroJS(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "public")

	err = BuildSite(BuildOptions{
		OutDir:     outDir,
		SiteURL:    "https://kryx.anrix.org",
		ContentDir: filepath.Join(repoRoot, "content"),
		StaticDir:  filepath.Join(repoRoot, "static"),
		DataDir:    filepath.Join(repoRoot, "data"),
		ManDirs:    []string{filepath.Join(repoRoot, "data", "man")},
		Offline:    true,
		BuildTime:  time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildSite failed: %v", err)
	}

	required := []string{
		"index.html",
		"style.css",
		"kryx.svg",
		"download/index.html",
		"docs/index.html",
		"docs/installation.html",
		"docs/kpm.html",
		"docs/building-ports.html",
		"docs/packaging-deb.html",
		"docs/init-systems.html",
		"docs/depgraph.html",
		"docs/configuration.html",
		"packages/index.html",
		"packages/kpm.html",
		"packages/wlroots.html",
		"ports/index.html",
		"ports/kpm-tools/index.html",
		"ports/kpm-tools/kpm.html",
		"man/index.html",
		"man/kpm.8.html",
		"man/kpm-index.8.html",
		"man/kryx-src.8.html",
		"man/ports.8.html",
		"news/index.html",
		"status/index.html",
		"feed.xml",
		"sitemap.xml",
		"robots.txt",
		"opensearch.xml",
		"404.html",
		"install.txt",
		"SHA256SUMS.txt",
		"kryx-release.pub",
		"packages.txt",
		"ports.txt",
		"search-index.json",
		"search/index.html",
		"deps-graph.json",
		"deps/index.html",
	}
	for _, rel := range required {
		p := filepath.Join(outDir, filepath.FromSlash(rel))
		fi, err := os.Stat(p)
		if err != nil || fi.Size() == 0 {
			t.Fatalf("expected non-empty output %s (err=%v)", rel, err)
		}
	}

	// Enforce strict 0-byte JavaScript invariant across every file in outDir.
	err = filepath.WalkDir(outDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".js") {
			t.Fatalf("forbidden JavaScript file found in output: %s", path)
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			lower := strings.ToLower(string(data))
			if strings.Contains(lower, "<script") {
				t.Fatalf("forbidden <script> tag found in %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Validate RSS 2.0 XML syntax.
	feedBytes, err := os.ReadFile(filepath.Join(outDir, "feed.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed rssDocument
	if err := xml.Unmarshal(feedBytes, &parsed); err != nil {
		t.Fatalf("invalid feed.xml: %v", err)
	}
	if len(parsed.Channel.Items) < 3 {
		t.Fatalf("expected at least 3 RSS items, got %d", len(parsed.Channel.Items))
	}

	// Validate sitemap.xml syntax and coverage.
	sitemapBytes, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var sm sitemapURLSet
	if err := xml.Unmarshal(sitemapBytes, &sm); err != nil {
		t.Fatalf("invalid sitemap.xml: %v", err)
	}
	if len(sm.URLs) < 20 {
		t.Fatalf("expected >= 20 sitemap URLs, got %d", len(sm.URLs))
	}
	if !strings.Contains(string(sitemapBytes), "https://kryx.anrix.org/docs/init-systems.html") {
		t.Fatalf("expected init-systems doc in sitemap.xml")
	}

	// Verify Void-style Handbook sidebar & pager on new guide.
	initDocHTML, err := os.ReadFile(filepath.Join(outDir, "docs", "init-systems.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(initDocHTML), `class="docs-sidebar"`) || !strings.Contains(string(initDocHTML), `class="docs-pager"`) {
		t.Fatalf("expected Void-style docs-sidebar and docs-pager in docs/init-systems.html")
	}

	// Verify zero heavy bold (font-weight: 700) in style.css.
	cssBytes, err := os.ReadFile(filepath.Join(outDir, "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cssBytes), "font-weight: 700") {
		t.Fatalf("forbidden font-weight: 700 found in style.css")
	}

	// Verify strictly 8 top-level navigation items on index.html.
	homeHTML, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	navStart := strings.Index(string(homeHTML), `<nav class="site-nav"`)
	navEnd := strings.Index(string(homeHTML)[navStart:], `</nav>`)
	navBlock := string(homeHTML)[navStart : navStart+navEnd]
	if got := strings.Count(navBlock, "<a "); got != 8 {
		t.Fatalf("expected strictly 8 top-nav links, got %d:\n%s", got, navBlock)
	}
	if !strings.Contains(string(homeHTML), `action="/search"`) {
		t.Fatalf("expected zero-JS search form on homepage")
	}

	// Verify search-index.json parses and contains expected kinds.
	rawIdx, err := os.ReadFile(filepath.Join(outDir, "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []SearchEntry
	if err := json.Unmarshal(rawIdx, &entries); err != nil {
		t.Fatalf("invalid search-index.json: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("expected >= 10 search entries, got %d", len(entries))
	}

	// Verify server-side zero-JS search handler (lynx/w3m/curl compatible).
	handler := NewSiteHandler(outDir, SiteContext{
		SiteURL:   "https://kryx.anrix.org",
		BuildTime: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	})

	// 1. HTML query matching binary package + docs
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search?q=wlroots&scope=packages", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /search?q=wlroots, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/packages/wlroots.html") {
		t.Fatalf("expected wlroots package link in search results:\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Fatalf("forbidden <script> tag in /search response")
	}

	// 2. Plain-text query for curl / terminal users
	recTxt := httptest.NewRecorder()
	reqTxt := httptest.NewRequest(http.MethodGet, "/search?q=kpm&format=txt", nil)
	handler.ServeHTTP(recTxt, reqTxt)
	if recTxt.Code != http.StatusOK {
		t.Fatalf("expected 200 for /search?q=kpm&format=txt, got %d", recTxt.Code)
	}
	if ct := recTxt.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain content-type, got %q", ct)
	}
	if !strings.Contains(recTxt.Body.String(), "https://kryx.anrix.org/packages/kpm.html") {
		t.Fatalf("expected plain-text kpm URL, got:\n%s", recTxt.Body.String())
	}

	// 3. XSS escaping on hostile query parameter
	recXSS := httptest.NewRecorder()
	reqXSS := httptest.NewRequest(http.MethodGet, "/search?q=%3Cscript%3Ealert(1)%3C/script%3E", nil)
	handler.ServeHTTP(recXSS, reqXSS)
	if recXSS.Code != http.StatusOK {
		t.Fatalf("expected 200 for escaped query, got %d", recXSS.Code)
	}
	if strings.Contains(strings.ToLower(recXSS.Body.String()), "<script") {
		t.Fatalf("unescaped <script> leaked into /search HTML:\n%s", recXSS.Body.String())
	}

	// 4. Empty query serves static /search/index.html
	recEmpty := httptest.NewRecorder()
	reqEmpty := httptest.NewRequest(http.MethodGet, "/search", nil)
	handler.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusOK || !strings.Contains(recEmpty.Body.String(), "Example Queries") {
		t.Fatalf("expected static search page on empty query, got status=%d", recEmpty.Code)
	}

	// 5. Dependency Explorer (/deps) HTML, reverse-deps (why), plain-text, Graphviz DOT, and XSS
	recDep := httptest.NewRecorder()
	handler.ServeHTTP(recDep, httptest.NewRequest(http.MethodGet, "/deps?pkg=wlroots", nil))
	if recDep.Code != http.StatusOK || !strings.Contains(recDep.Body.String(), "Dependency Tree:") || strings.Contains(strings.ToLower(recDep.Body.String()), "<script") {
		t.Fatalf("unexpected /deps?pkg=wlroots response: code=%d", recDep.Code)
	}
	recDepTxt := httptest.NewRecorder()
	handler.ServeHTTP(recDepTxt, httptest.NewRequest(http.MethodGet, "/deps?pkg=wlroots&format=txt", nil))
	if recDepTxt.Code != http.StatusOK || !strings.HasPrefix(recDepTxt.Header().Get("Content-Type"), "text/plain") || !strings.Contains(recDepTxt.Body.String(), "# Dependency tree for wlroots") {
		t.Fatalf("unexpected /deps txt response: code=%d body=%s", recDepTxt.Code, recDepTxt.Body.String())
	}
	recDepDot := httptest.NewRecorder()
	handler.ServeHTTP(recDepDot, httptest.NewRequest(http.MethodGet, "/deps?pkg=wlroots&format=dot", nil))
	if recDepDot.Code != http.StatusOK || !strings.Contains(recDepDot.Body.String(), "digraph kryx_deps") {
		t.Fatalf("unexpected /deps dot response: code=%d body=%s", recDepDot.Code, recDepDot.Body.String())
	}
	recWhy := httptest.NewRecorder()
	handler.ServeHTTP(recWhy, httptest.NewRequest(http.MethodGet, "/deps?pkg=wayland&mode=why", nil))
	if recWhy.Code != http.StatusOK || !strings.Contains(recWhy.Body.String(), "Required By:") || !strings.Contains(recWhy.Body.String(), "wlroots") {
		t.Fatalf("unexpected /deps why response: code=%d body=%s", recWhy.Code, recWhy.Body.String())
	}
	recDepXSS := httptest.NewRecorder()
	handler.ServeHTTP(recDepXSS, httptest.NewRequest(http.MethodGet, "/deps?pkg=%3Cscript%3Ealert(1)%3C/script%3E", nil))
	if strings.Contains(strings.ToLower(recDepXSS.Body.String()), "<script") {
		t.Fatalf("unescaped <script> leaked into /deps HTML")
	}

	// 6. Filterable package/port browser (/browse)
	recBrowse := httptest.NewRecorder()
	handler.ServeHTTP(recBrowse, httptest.NewRequest(http.MethodGet, "/browse?kind=prebuilt&per=10&page=1", nil))
	if recBrowse.Code != http.StatusOK || !strings.Contains(recBrowse.Body.String(), "/packages/wlroots.html") {
		t.Fatalf("unexpected /browse HTML response: code=%d", recBrowse.Code)
	}
	recBrowseTxt := httptest.NewRecorder()
	handler.ServeHTTP(recBrowseTxt, httptest.NewRequest(http.MethodGet, "/browse?col=kpm-tools&format=txt", nil))
	if recBrowseTxt.Code != http.StatusOK || !strings.Contains(recBrowseTxt.Body.String(), "/packages/kpm.html") {
		t.Fatalf("unexpected /browse txt response: code=%d body=%s", recBrowseTxt.Code, recBrowseTxt.Body.String())
	}

	// 7. Zero-JS theme switcher (/theme + cookie middleware + open-redirect guard)
	recTheme := httptest.NewRecorder()
	handler.ServeHTTP(recTheme, httptest.NewRequest(http.MethodGet, "/theme?mode=dark&next=/docs/", nil))
	if recTheme.Code != http.StatusFound || recTheme.Header().Get("Location") != "/docs/" {
		t.Fatalf("expected 302 redirect to /docs/, got code=%d loc=%q", recTheme.Code, recTheme.Header().Get("Location"))
	}
	if !strings.Contains(recTheme.Header().Get("Set-Cookie"), "kryx_theme=dark") {
		t.Fatalf("expected kryx_theme=dark cookie, got %q", recTheme.Header().Get("Set-Cookie"))
	}
	recRedirectGuard := httptest.NewRecorder()
	handler.ServeHTTP(recRedirectGuard, httptest.NewRequest(http.MethodGet, "/theme?mode=dark&next=//evil.example/pwn", nil))
	if recRedirectGuard.Header().Get("Location") != "/" {
		t.Fatalf("open redirect was not blocked: got %q", recRedirectGuard.Header().Get("Location"))
	}
	recDarkHome := httptest.NewRecorder()
	reqDarkHome := httptest.NewRequest(http.MethodGet, "/", nil)
	reqDarkHome.AddCookie(&http.Cookie{Name: "kryx_theme", Value: "dark"})
	handler.ServeHTTP(recDarkHome, reqDarkHome)
	if !strings.Contains(recDarkHome.Body.String(), `<html lang="en" data-theme="dark">`) {
		t.Fatalf("expected data-theme=\"dark\" injected into HTML response")
	}
	if !strings.Contains(recDarkHome.Header().Get("Vary"), "Cookie") {
		t.Fatalf("expected Vary: Cookie header when theme cookie active")
	}
}

func TestLoadReleaseInfoLiveSync(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(repoRoot, "data", "release.json")

	const mockHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tag_name": "2026.12-rolling",
				"published_at": "2026-12-01T08:00:00Z",
				"assets": [
					{
						"name": "kryx-linux-x86_64.iso",
						"size": 452984832,
						"browser_download_url": "https://example.invalid/kryx-linux-x86_64.iso"
					},
					{
						"name": "SHA256SUMS.txt",
						"size": 128,
						"browser_download_url": "` + ts.URL + `/SHA256SUMS.txt"
					}
				]
			}`))
		case "/SHA256SUMS.txt":
			_, _ = w.Write([]byte(mockHash + "  kryx-linux-x86_64.iso\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Offline skips HTTP fetch.
	relOff, _, err := LoadReleaseInfo(fallbackPath, ts.URL+"/releases/latest", true)
	if err != nil {
		t.Fatal(err)
	}
	if relOff.Version == "2026.12-rolling" {
		t.Fatalf("offline mode should not fetch mock release")
	}

	// Online updates version, date, size, URL, and SHA-256 checksum.
	relLive, src, err := LoadReleaseInfo(fallbackPath, ts.URL+"/releases/latest", false)
	if err != nil {
		t.Fatal(err)
	}
	if relLive.Version != "2026.12-rolling" || relLive.ReleaseDate != "2026-12-01" {
		t.Fatalf("unexpected live release metadata: %+v (src=%s)", relLive, src)
	}
	foundISO := false
	for _, a := range relLive.Artifacts {
		if a.Name == "kryx-linux-x86_64.iso" {
			foundISO = true
			if a.SHA256 != mockHash || a.URL != "https://example.invalid/kryx-linux-x86_64.iso" || !strings.Contains(a.Size, "MiB") {
				t.Fatalf("unexpected synced ISO artifact: %+v", a)
			}
		}
	}
	if !foundISO {
		t.Fatalf("expected kryx-linux-x86_64.iso artifact in synced release")
	}
}

func TestDiscoverPortsSiblingReposAndTarGz(t *testing.T) {
	ws := t.TempDir()
	writePkg := func(rel, content string) {
		t.Helper()
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writePkg("ports/kpm-tools/kpm/Pkgfile", "# Description: kpm\nname=kpm\nversion=0.6.0\nrelease=1\n")
	writePkg("ports/kernel/linux6.18/Pkgfile", "# Description: kernel\nname=kernel\nversion=6.18.64\nrelease=2\n")
	writePkg("core/zlib/Pkgfile", "# Description: zlib\nname=zlib\nversion=1.4\nrelease=1\n")
	writePkg("opt/grub2/Pkgfile", "# Description: grub2\nname=grub2\nversion=2.16\nrelease=1\n")

	byCol, byName := DiscoverPorts(filepath.Join(ws, "ports"), RepoIndex{}, true)
	if len(byCol["kpm-tools"]) != 1 || len(byCol["kernel"]) != 1 || len(byCol["core"]) != 1 || len(byCol["opt"]) != 1 {
		t.Fatalf("unexpected sibling discovery counts: kpm-tools=%d kernel=%d core=%d opt=%d",
			len(byCol["kpm-tools"]), len(byCol["kernel"]), len(byCol["core"]), len(byCol["opt"]))
	}
	if byName["kernel"].Collection != "kernel" || byName["zlib"].Collection != "core" {
		t.Fatalf("unexpected collection mapping: %+v", byName)
	}

	// Verify tarball streaming extraction for both single-collection and multi-collection GitHub archives.
	makeTarGz := func(files map[string]string) *bytes.Buffer {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for name, body := range files {
			hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return &buf
	}

	coreTar := makeTarGz(map[string]string{
		"core-main/bash/Pkgfile": "# Description: GNU Bash\nname=bash\nversion=5.3\nrelease=1\n",
		"core-main/README.md":    "ignore me",
	})
	gotCore, err := extractPortsFromTarGz(coreTar, "core", nil)
	if err != nil || len(gotCore["core"]) != 1 || gotCore["core"][0].Name != "bash" {
		t.Fatalf("extractPortsFromTarGz(core) failed: got=%+v err=%v", gotCore, err)
	}

	metaTar := makeTarGz(map[string]string{
		"ports-main/kpm-tools/kpm/Pkgfile":      "# Description: kpm\nname=kpm\nversion=0.6.0\nrelease=1\n",
		"ports-main/kernel/linux6.18/Pkgfile":   "# Description: kernel\nname=kernel\nversion=6.18.64\nrelease=2\n",
		"ports-main/ignored/foo/Pkgfile":        "# Description: skip\nname=foo\nversion=1\nrelease=1\n",
	})
	gotMeta, err := extractPortsFromTarGz(metaTar, "", map[string]bool{"kpm-tools": true, "kernel": true})
	if err != nil || len(gotMeta["kpm-tools"]) != 1 || len(gotMeta["kernel"]) != 1 || len(gotMeta["ignored"]) != 0 {
		t.Fatalf("extractPortsFromTarGz(meta) failed: got=%+v err=%v", gotMeta, err)
	}
}
